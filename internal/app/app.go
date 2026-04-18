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
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/git"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
	"github.com/packetcode/packetcode/internal/ui/components/approval"
	"github.com/packetcode/packetcode/internal/ui/components/conversation"
	"github.com/packetcode/packetcode/internal/ui/components/input"
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

// Deps bundles everything App needs from main(). main() owns the lifecycle
// of these objects; App just borrows them.
type Deps struct {
	Config       *config.Config
	Registry     *provider.Registry
	Tools        *tools.Registry
	Sessions     *session.Manager
	CostTracker  *cost.Tracker
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
	spinner      spinner.Model

	// Agent + bridge.
	agent    *agent.Agent
	approver *uiApprover

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
		spinner:      spinner.New(),
		agent:        a,
		approver:     approver,
	}

	app.refreshTopBar()
	return app, nil
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
		return a.startTurn(msg.Text)

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

	// Delegate to the focused subcomponent.
	var cmds []tea.Cmd
	if a.approval.Visible() {
		var cmd tea.Cmd
		a.approval, cmd = a.approval.Update(msg)
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
	if a.approval.Visible() {
		overlay = a.approval.View()
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
}

func (a *App) startTurn(text string) (tea.Model, tea.Cmd) {
	a.input.Reset()
	a.conversation.AppendUser(text)
	a.streaming = true

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
