// Package app is the top-level Bubble Tea model. It orchestrates the
// composed UI components and translates their messages into agent and
// session actions.
//
// The flow is straightforward:
//   1. User types in the input bar → Enter → SubmitMsg.
//   2. App runs agent.Run(), which returns a channel of AgentEvent.
//   3. A goroutine forwards each AgentEvent to the Bubble Tea program
//      via Send(). Update() routes them to the conversation pane.
//   4. When the agent needs approval, the uiApprover bridge posts the
//      pending request, App raises the approval modal, the user hits y/n,
//      and the decision is sent back to the agent.
package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/git"
	"github.com/packetcode/packetcode/internal/jobs"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
	"github.com/packetcode/packetcode/internal/ui/components/approval"
	"github.com/packetcode/packetcode/internal/ui/components/conversation"
	"github.com/packetcode/packetcode/internal/ui/components/input"
	jobs_ui "github.com/packetcode/packetcode/internal/ui/components/jobs"
	"github.com/packetcode/packetcode/internal/ui/components/spinner"
	"github.com/packetcode/packetcode/internal/ui/components/topbar"
	"github.com/packetcode/packetcode/internal/ui/layout"
)

// agentEventMsg wraps a single agent.AgentEvent so we can route it
// through the Bubble Tea Update loop.
type agentEventMsg struct{ ev agent.AgentEvent }

// agentDoneMsg signals the agent's event channel has closed.
type agentDoneMsg struct{}

// pollApproverMsg fires periodically so the App can pick up pending
// approval requests posted by the agent goroutine.
type pollApproverMsg struct{}

// tickTopbarMsg updates the duration counter in the top bar.
type tickTopbarMsg struct{}

// jobUpdateMsg is dispatched from the jobs.Manager Subscribe callback
// (which runs in its own goroutine) into the Bubble Tea Update loop via
// tea.Program.Send. The App uses it to refresh the top bar and, on
// terminal transitions, append a system message describing the outcome.
type jobUpdateMsg struct{ Snap jobs.Snapshot }

// Deps bundles everything App needs from main(). main() owns the lifecycle
// of these objects; App just borrows them.
type Deps struct {
	Config       *config.Config
	Registry     *provider.Registry
	Tools        *tools.Registry
	Sessions     *session.Manager
	CostTracker  *cost.Tracker
	Jobs         *jobs.Manager
	WorkingDir   string
	SystemPrompt string
	Version      string // shown on the welcome splash; e.g. "v1" or "v0.1.0"
}

type App struct {
	deps Deps

	// UI components.
	topbar       topbar.Model
	conversation conversation.Model
	input        input.Model
	approval     approval.Model
	jobsPanel    jobs_ui.Model
	spinner      spinner.Model

	// Agent + bridge.
	agent    *agent.Agent
	approver *uiApprover

	// Background-agents manager. Non-nil when deps.Jobs is set. All
	// job-related UI code paths guard on `a.jobs != nil`.
	jobs *jobs.Manager

	// sendMsg is the tea.Program.Send bridge set by the host (main.go)
	// after tea.NewProgram so callbacks originating off the Bubble Tea
	// thread (notably the jobs.Manager Subscribe callback) can deliver
	// messages into Update. Nil-safe: if unset, async updates are
	// silently dropped (sync code paths still work).
	sendMsg func(tea.Msg)

	width    int
	height   int
	streaming bool
	err      string
}

// New constructs the App and registers the active provider/model from
// config. Returns an error if no provider is configured (caller should
// run the setup flow first).
func New(deps Deps) (*App, error) {
	if deps.Registry == nil || deps.Tools == nil || deps.Sessions == nil {
		return nil, fmt.Errorf("app: missing required dependencies")
	}
	if deps.WorkingDir == "" {
		deps.WorkingDir = "."
	}

	approver := newUIApprover()
	if deps.Config != nil && deps.Config.Behavior.TrustMode {
		approver.SetTrust(true)
	}

	a := agent.New(agent.Config{
		Registry:     deps.Registry,
		Tools:        deps.Tools,
		Session:      deps.Sessions,
		CostTracker:  deps.CostTracker,
		Approver:     approver,
		SystemPrompt: deps.SystemPrompt,
	})

	conv := conversation.New()
	if deps.Version != "" {
		conv.SetVersion(deps.Version)
	} else {
		conv.SetVersion("v1")
	}

	app := &App{
		deps:         deps,
		topbar:       topbar.New(),
		conversation: conv,
		input:        input.New(),
		approval:     approval.New(),
		jobsPanel:    jobs_ui.New(),
		spinner:      spinner.New(),
		agent:        a,
		approver:     approver,
		jobs:         deps.Jobs,
	}

	if deps.Jobs != nil {
		// Fan every snapshot transition from the manager into Update.
		// The callback runs off the Bubble Tea thread; sendMsg is set
		// by the host after tea.NewProgram (see main.go).
		deps.Jobs.Subscribe(func(snap jobs.Snapshot) {
			if app.sendMsg != nil {
				app.sendMsg(jobUpdateMsg{Snap: snap})
			}
		})
	}

	app.refreshTopBar()
	return app, nil
}

// SetSendFunc wires the tea.Program.Send bridge. Host (main.go) calls
// this between tea.NewProgram and prog.Run so off-thread callbacks (the
// jobs.Manager subscriber) can post messages into the Update loop.
func (a *App) SetSendFunc(fn func(tea.Msg)) {
	a.sendMsg = fn
}

// Approver returns the App's uiApprover so the host can inject it as the
// jobs.Manager parent approver. Hidden behind the agent.Approver
// interface because that's what jobs.Manager wants.
func (a *App) Approver() agent.Approver {
	return a.approver
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(
		pollApprover(),
		tickTopbar(),
	)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.resize(msg.Width, msg.Height)
		return a, nil

	case tea.KeyMsg:
		return a.handleKey(msg)

	case input.SubmitMsg:
		// Slash commands are UI-side concerns: they don't hit the LLM.
		// Intercept them before startTurn so /spawn, /jobs, /cancel etc.
		// take effect immediately without invoking agent.Run.
		if cmd, args, ok := ParseSlashCommand(msg.Text); ok {
			return a.handleSlashCommand(cmd, args, msg.Text)
		}
		return a.startTurn(msg.Text)

	case jobUpdateMsg:
		return a.handleJobUpdate(msg.Snap)

	case agentEventMsg:
		return a.handleAgentEvent(msg.ev)

	case agentEventBatch:
		return a.reentrantHandle(msg)

	case agentDoneMsg:
		a.streaming = false
		a.spinner.Stop()
		a.conversation.FinaliseAgent()
		return a, nil

	case approval.ResultMsg:
		switch msg.Result {
		case approval.Approved:
			a.approver.Resolve(agent.ApprovalDecision{Approved: true})
		case approval.Rejected:
			a.approver.Resolve(agent.ApprovalDecision{Approved: false, Reason: "user rejected"})
		}
		return a, nil

	case pollApproverMsg:
		if req, ok := a.approver.Pending(); ok {
			a.approval.Show(req.Tool, req.ToolCall)
			a.approval.SetWidth(a.width)
		}
		return a, pollApprover()

	case tickTopbarMsg:
		a.refreshTopBar()
		return a, tickTopbar()
	}

	// Delegate to the focused subcomponent. The approval prompt wins
	// (it blocks the agent loop); the jobs transcript modal is next
	// (keyboard input while open should scroll the transcript, not
	// the input bar); otherwise the conversation + input consume.
	var cmds []tea.Cmd
	if a.approval.Visible() {
		var cmd tea.Cmd
		a.approval, cmd = a.approval.Update(msg)
		cmds = append(cmds, cmd)
	} else if a.jobsPanel.Visible() {
		var cmd tea.Cmd
		a.jobsPanel, cmd = a.jobsPanel.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		var cmd tea.Cmd
		a.input, cmd = a.input.Update(msg)
		cmds = append(cmds, cmd)
		a.conversation, cmd = a.conversation.Update(msg)
		cmds = append(cmds, cmd)
	}
	if a.spinner.Active() {
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

func (a *App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if a.streaming {
			// Streaming: cancel current generation by stopping spinner;
			// proper cancellation context-plumbing is a follow-up.
			a.spinner.Stop()
			a.streaming = false
			return a, nil
		}
		return a, tea.Quit
	case "ctrl+l":
		fresh := conversation.New()
		fresh.SetVersion(a.deps.Version)
		a.conversation = fresh
		// Conversation will pick up its size on the next View() call.
		return a, nil
	}
	if a.approval.Visible() {
		var cmd tea.Cmd
		a.approval, cmd = a.approval.Update(msg)
		return a, cmd
	}
	if a.jobsPanel.Visible() {
		var cmd tea.Cmd
		a.jobsPanel, cmd = a.jobsPanel.Update(msg)
		return a, cmd
	}
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	return a, cmd
}

func (a *App) View() string {
	if a.width <= 0 || a.height <= 0 {
		return ""
	}

	// Render the chrome regions first so we can measure their actual
	// rendered heights — those vary with multi-line input, narrow-mode
	// shedding in the status bar, etc.
	status := a.topbar.View()
	in := a.input.View()
	overlay := ""
	// Overlay precedence: the approval prompt is the most urgent
	// (blocks the agent loop), the jobs transcript modal is
	// user-opened and should hide the spinner while open, and the
	// spinner fills the slot only when nothing else wants it.
	if a.approval.Visible() {
		overlay = a.approval.View()
	} else if a.jobsPanel.Visible() {
		overlay = a.jobsPanel.View()
	} else if a.spinner.Active() {
		overlay = a.spinner.View()
	}

	statusH := lipgloss.Height(status)
	inputH := lipgloss.Height(in)
	overlayH := 0
	if overlay != "" {
		overlayH = lipgloss.Height(overlay)
	}

	bodyH := a.height - statusH - inputH - overlayH
	if bodyH < 3 {
		bodyH = 3
	}

	// Now that we know the exact body height, size the conversation
	// pane (which owns its own viewport) to match. Rendering after this
	// resize guarantees no content gets clipped by post-hoc trimming —
	// the previous bug that ate the user's first message.
	a.conversation.Resize(a.width, bodyH)
	body := a.conversation.View()

	return layout.Frame(body, overlay, in, status)
}

// ────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ────────────────────────────────────────────────────────────────────────────

// resize stores the new terminal dimensions and propagates width to
// components that wrap text. Heights are recomputed in View() based on
// the rendered chrome — this avoids the trap of guessing border + padding
// row counts and getting clipped content.
func (a *App) resize(w, h int) {
	a.width = w
	a.height = h
	a.topbar.SetWidth(w)
	a.input.Resize(w, 0)
	a.approval.SetWidth(w)
	// Give the jobs modal a generous but not fullscreen body budget —
	// header + footer + border eat a few lines in the component
	// itself. Matches the approval prompt's "full-width, modest
	// height" shape.
	modalH := h - 8
	if modalH < 8 {
		modalH = 8
	}
	a.jobsPanel.Resize(w, modalH)
	// Conversation height is set in View() after measuring chrome.
}

func (a *App) refreshTopBar() {
	if prov, modelID := a.deps.Registry.Active(); prov != nil {
		a.topbar.SetProvider(prov.Slug(), prov.Name(), modelID)

		// Context window from active model. We keep this best-effort —
		// providers like Ollama report 0 (unknown).
		ctxMax := prov.ContextWindow(modelID)
		used := 0
		if cur := a.deps.Sessions.Current(); cur != nil {
			used = cur.TokenUsage.TotalInput
		}
		a.topbar.SetContext(used, ctxMax)
	}

	root := a.deps.WorkingDir
	a.topbar.SetProject(filepath.Base(root), git.Branch(root))

	if a.deps.CostTracker != nil {
		a.topbar.SetCost(a.deps.CostTracker.TotalCost())
	}

	// The ⚙ N jobs counter reflects StateQueued + StateRunning jobs. We
	// pass 0 when no manager is wired so the segment stays hidden in
	// configurations where background agents are disabled. ActiveCount
	// is lock-guarded, so calling it on every refreshTopBar tick (15s)
	// and every job state transition is cheap.
	if a.jobs != nil {
		a.topbar.SetJobs(a.jobs.ActiveCount())
	} else {
		a.topbar.SetJobs(0)
	}
}

func (a *App) startTurn(text string) (tea.Model, tea.Cmd) {
	a.input.Reset()
	a.conversation.AppendUser(text)
	a.streaming = true

	// Inject any ready background job results into the session as
	// RoleUser context messages so the LLM sees them as part of this
	// turn's input. Per spec: "[Background job <id> result]\n<summary>".
	a.injectPendingJobResults()

	prov, modelID := a.deps.Registry.Active()
	providerSlug := ""
	if prov != nil {
		providerSlug = prov.Slug()
	}

	// Spin off the agent run on a background goroutine. We forward each
	// AgentEvent to the Bubble Tea program via Send(); the message
	// arrives in Update as an agentEventMsg.
	ctx := context.Background()
	stream := a.agent.Run(ctx, text)

	go func() {
		defer func() { _ = providerSlug; _ = modelID }()
		// We can't call Program.Send here without the program reference.
		// Instead we accumulate events into a channel that Update polls
		// via tea.Cmd. Simpler approach: read from the stream using a
		// blocking tea.Cmd that fires once per event.
	}()

	return a, readAgentEvent(stream)
}

func (a *App) handleAgentEvent(ev agent.AgentEvent) (tea.Model, tea.Cmd) {
	prov, modelID := a.deps.Registry.Active()
	providerSlug := ""
	if prov != nil {
		providerSlug = prov.Slug()
	}

	switch ev.Type {
	case agent.EventTextDelta:
		if !a.spinner.Active() {
			// First token arrived → silence the spinner.
			a.spinner.Stop()
		}
		a.conversation.AppendAgentText(modelID, providerSlug, ev.Text)

	case agent.EventToolCallProposed:
		a.conversation.AppendToolCall(ev.ToolCall.Name, ev.ToolCall.Arguments)

	case agent.EventToolCallExecuted:
		a.conversation.CompleteToolCall(ev.ToolCall.Name, ev.ToolResult)

	case agent.EventToolCallRejected:
		a.conversation.AppendSystem(fmt.Sprintf("✗ rejected %s", ev.ToolCall.Name))

	case agent.EventUsageUpdate:
		a.refreshTopBar()

	case agent.EventDone:
		// EventDone is the channel-close signal at the agent level. The
		// channel close itself produces agentDoneMsg.

	case agent.EventError:
		a.conversation.AppendSystem("error: " + ev.Error.Error())
	}
	return a, nil
}

// readAgentEvent reads one event from the agent's channel and converts
// it to a tea.Msg. Returns agentDoneMsg when the channel closes.
// Recursive: every time we deliver an event we schedule another read.
func readAgentEvent(stream <-chan agent.AgentEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-stream
		if !ok {
			return agentDoneMsg{}
		}
		return agentEventBatch{first: ev, rest: stream}
	}
}

// agentEventBatch is a self-rescheduling cursor over the agent stream.
// When Update receives one, it dispatches `first` and schedules another
// read of `rest`. This keeps the Bubble Tea event loop responsive while
// preserving event order.
type agentEventBatch struct {
	first agent.AgentEvent
	rest  <-chan agent.AgentEvent
}

// Wire agentEventBatch into Update as if it were agentEventMsg, then
// schedule the next read.
func (a *App) reentrantHandle(b agentEventBatch) (tea.Model, tea.Cmd) {
	model, cmd := a.handleAgentEvent(b.first)
	next := readAgentEvent(b.rest)
	if cmd == nil {
		return model, next
	}
	return model, tea.Batch(cmd, next)
}

// injectPendingJobResults drains any ready background-job results from
// the manager and appends each as a RoleUser message to the main
// session. The agent's next ChatRequest picks them up via
// buildMessages. We deliberately use RoleUser (not RoleSystem) so
// providers that disallow multi-system messages still accept the
// payload — the existing system prompt already holds slot 0.
func (a *App) injectPendingJobResults() {
	if a.jobs == nil {
		return
	}
	results := a.jobs.DrainResults(32)
	if len(results) == 0 {
		return
	}
	for _, r := range results {
		summary := strings.TrimSpace(r.Summary)
		if summary == "" {
			summary = "(no summary)"
		}
		body := fmt.Sprintf("[Background job %s result]\n%s", r.JobID, summary)
		_ = a.deps.Sessions.AddMessage(provider.Message{
			Role:    provider.RoleUser,
			Content: body,
		})
	}
}

// handleJobUpdate is the UI-side handler for a jobs.Snapshot transition.
// Refreshes the top bar counter and, on terminal states, appends a
// system message summarising the outcome.
func (a *App) handleJobUpdate(snap jobs.Snapshot) (tea.Model, tea.Cmd) {
	a.refreshTopBar()
	if snap.State.IsTerminal() {
		a.conversation.AppendSystem(formatTerminalJobLine(snap))
	}
	return a, nil
}

// formatTerminalJobLine renders a single-line inline notification for a
// job that has just reached a terminal state. Matches the spec:
//   [job:7f3a — done · 12s · gemini/2.5-flash · $0.0031]
//   14 call sites in 8 files; …
func formatTerminalJobLine(snap jobs.Snapshot) string {
	label := "done"
	switch snap.State {
	case jobs.StateFailed:
		label = "failed"
	case jobs.StateCancelled:
		label = "cancelled"
	case jobs.StateCompleted:
		label = "done"
	}
	dur := time.Duration(0)
	if !snap.StartedAt.IsZero() && !snap.FinishedAt.IsZero() {
		dur = snap.FinishedAt.Sub(snap.StartedAt)
	}
	prov := snap.Provider
	if snap.Model != "" {
		if prov != "" {
			prov += "/" + snap.Model
		} else {
			prov = snap.Model
		}
	}
	head := fmt.Sprintf("[job:%s — %s · %s · %s · $%.4f]",
		snap.ID, label, roundedDuration(dur), prov, snap.CostUSD)
	body := strings.TrimSpace(snap.Summary)
	if snap.State == jobs.StateFailed && snap.Error != "" {
		if body != "" {
			body += "\n"
		}
		body += "error: " + snap.Error
	}
	if body == "" {
		return head
	}
	return head + "\n" + body
}

// roundedDuration renders a duration as a short "12s" / "1m03s" string
// for the one-line terminal-job notification. We round to the nearest
// second so output doesn't drift between runs.
func roundedDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d / time.Minute)
	s := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm%02ds", m, s)
}

// handleSlashCommand dispatches a parsed slash command from the input
// line. Returns the tea.Model / tea.Cmd pair so the caller can thread
// it back through Update.
func (a *App) handleSlashCommand(cmd string, args []string, original string) (tea.Model, tea.Cmd) {
	a.input.Reset()
	if a.jobs == nil {
		a.conversation.AppendSystem("background jobs are disabled (no jobs.Manager wired)")
		return a, nil
	}

	switch cmd {
	case "spawn":
		return a.handleSpawnCommand(args)
	case "jobs":
		return a.handleJobsCommand(args)
	case "cancel":
		return a.handleCancelCommand(args)
	}
	// ParseSlashCommand only returns ok=true for the three names above,
	// so this branch is unreachable — but render a friendly fallback
	// for forward compatibility.
	a.conversation.AppendSystem("unknown command: " + original)
	return a, nil
}

func (a *App) handleSpawnCommand(args []string) (tea.Model, tea.Cmd) {
	provSlug, modelID, allowWrite, prompt, err := ParseSpawnFlags(args)
	if err != nil {
		a.conversation.AppendSystem("spawn: " + err.Error())
		return a, nil
	}
	snap, spawnErr := a.jobs.Spawn(jobs.SpawnRequest{
		Prompt:      prompt,
		Provider:    provSlug,
		Model:       modelID,
		ParentJobID: "",
		ParentDepth: 0,
		AllowWrite:  allowWrite,
	})
	if spawnErr != nil {
		a.conversation.AppendSystem(fmt.Sprintf("spawn failed: %s", spawnErr.Error()))
		return a, nil
	}
	prov := snap.Provider
	if snap.Model != "" {
		if prov != "" {
			prov += "/" + snap.Model
		} else {
			prov = snap.Model
		}
	}
	a.conversation.AppendSystem(fmt.Sprintf("[job:%s queued — %s] %s", snap.ID, prov, snap.Prompt))
	// Reflect the new job on the top bar immediately. The Subscribe
	// fanout will do this too, but asynchronously on a goroutine —
	// bumping the counter here is synchronous and matches the user's
	// mental model (they typed the command, they see the counter).
	a.refreshTopBar()
	return a, nil
}

func (a *App) handleJobsCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		a.conversation.AppendSystem(renderJobsTable(a.jobs.List()))
		return a, nil
	}
	id := args[0]
	snap, ok := a.jobs.Get(id)
	if !ok {
		a.conversation.AppendSystem(fmt.Sprintf("[job:%s not found]", id))
		return a, nil
	}
	transcript, _ := a.jobs.Transcript(id)
	// Bucket C: /jobs <id> opens the transcript modal. Previously the
	// detail was rendered as an inline system message via
	// renderJobDetail — kept in the package for tests that assert
	// against its format, but no longer wired into the user flow.
	a.jobsPanel.Show(snap, transcript)
	return a, nil
}

func (a *App) handleCancelCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		a.conversation.AppendSystem("cancel: missing job id (or 'all')")
		return a, nil
	}
	target := args[0]
	if target == "all" {
		n := a.jobs.CancelAll()
		a.conversation.AppendSystem(fmt.Sprintf("[cancelled %d jobs]", n))
		a.refreshTopBar()
		return a, nil
	}
	if a.jobs.Cancel(target) {
		a.conversation.AppendSystem(fmt.Sprintf("[job:%s — cancellation requested]", target))
	} else {
		a.conversation.AppendSystem(fmt.Sprintf("[job:%s not found or already terminal]", target))
	}
	a.refreshTopBar()
	return a, nil
}

// renderJobsTable returns a monospace ASCII table of snapshots.
// Newest-first; prompt truncated to 50 chars.
func renderJobsTable(snaps []jobs.Snapshot) string {
	if len(snaps) == 0 {
		return "no background jobs"
	}
	// jobs.Manager.List() already sorts newest-first; still, be defensive
	// if a subset is passed in.
	sort.SliceStable(snaps, func(i, j int) bool {
		return snaps[i].CreatedAt.After(snaps[j].CreatedAt)
	})
	var b strings.Builder
	b.WriteString("ID    STATE      PROV/MODEL              AGE    TOK(IN/OUT)  PROMPT\n")
	now := time.Now()
	for _, s := range snaps {
		prov := s.Provider
		if s.Model != "" {
			if prov != "" {
				prov += "/" + s.Model
			} else {
				prov = s.Model
			}
		}
		age := roundedDuration(now.Sub(s.CreatedAt))
		tok := fmt.Sprintf("%d/%d", s.Tokens.Input, s.Tokens.Output)
		prompt := s.Prompt
		if len(prompt) > 50 {
			prompt = prompt[:47] + "..."
		}
		fmt.Fprintf(&b, "%-5s %-10s %-23s %-6s %-12s %s\n",
			trunc(s.ID, 5), trunc(s.State.String(), 10), trunc(prov, 23), age, trunc(tok, 12), prompt)
	}
	return strings.TrimRight(b.String(), "\n")
}

// trunc returns s truncated to n runes (not bytes), adding nothing — just
// clips. Used for table-cell formatting.
func trunc(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func pollApprover() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return pollApproverMsg{}
	})
}

func tickTopbar() tea.Cmd {
	return tea.Tick(15*time.Second, func(time.Time) tea.Msg {
		return tickTopbarMsg{}
	})
}
