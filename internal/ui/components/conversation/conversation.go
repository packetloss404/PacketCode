// Package conversation renders the transcript of a session. Each
// finalised message (user turn, assistant reply, tool call + result,
// system note) is committed to the terminal's native scrollback via
// tea.Println; the component itself only holds a single "pending" slot
// for the message currently being streamed or awaiting a tool result,
// which the App renders into its live region below the topbar.
//
// Design:
//   - Append* / Complete* / Finalise* mutate state and push a rendered
//     string onto the internal emits queue.
//   - The App calls DrainEmits() each Update tick and wraps each entry
//     in tea.Println so they land in terminal scrollback above the live
//     region.
//   - PendingView() returns the live-region render of whatever is
//     streaming or awaiting a tool result.
//   - View() is retained for tests: concatenates queued emits + pending.
package conversation

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/tools"
	"github.com/packetcode/packetcode/internal/ui/components/diff"
	"github.com/packetcode/packetcode/internal/ui/components/welcome"
	"github.com/packetcode/packetcode/internal/ui/theme"
)

// MessageKind discriminates how a Message renders.
type MessageKind int

const (
	KindUser MessageKind = iota
	KindAgent
	KindSystem
	KindError
	KindToolCall
)

// Message is the conversation's atomic display unit. Tool calls and tool
// results are merged into a single block so the output reads linearly.
type Message struct {
	Kind    MessageKind
	Author  string
	Color   lipgloss.Color
	Content string

	// Reasoning is the model's streamed reasoning summary (Codex/Responses),
	// rendered dim above Content in a KindAgent bubble. Display-only.
	Reasoning string

	// Queued marks an optimistic user message submitted while a turn is still
	// running; rendered dim with a "(queued)" marker.
	Queued bool

	// ToolCall fields populated when Kind == KindToolCall.
	ToolName   string
	ToolArgs   string
	ToolResult string
	IsError    bool

	// ToolCallID is the provider-assigned call id for the in-flight tool
	// call. Used to route incremental output chunks (EventToolOutputChunk)
	// to the correct pending block when a long-running command streams its
	// stdout/stderr before the final result lands.
	ToolCallID string
	// LiveOutput accumulates streamed stdout/stderr for a running command
	// while it executes. It is a *preview only*: it is shown in the live
	// region until CompleteToolCall commits the authoritative ToolResult,
	// at which point LiveOutput is discarded so the committed result is the
	// single rendered copy (no duplication).
	LiveOutput string
}

// maxLiveOutput bounds the streamed-preview buffer held in the pending
// slot. The producer (Part 1) keeps the bounded buffer for the final
// result; here we only need enough to show recent progress in the live
// region, so we cap and keep the tail. Generous enough for a screenful of
// build/test output, small enough that re-rendering the live region every
// throttle tick stays cheap.
const maxLiveOutput = 8192

// Model is the conversation state: a pending in-flight message (if any)
// and a queue of rendered emits awaiting DrainEmits.
type Model struct {
	width   int
	height  int
	version string

	pending        *Message
	welcomePrinted bool

	// emits is the FIFO queue of rendered strings awaiting DrainEmits.
	// Production: drained each Update cycle and each entry becomes a
	// tea.Println that commits to terminal scrollback.
	emits []string
	// seen mirrors every entry that has ever passed through emit(), kept
	// for test harnesses that want to assert against the cumulative
	// transcript (production code never reads it). Unbounded growth is
	// acceptable because /clear replaces the whole Model.
	seen []string
}

// New constructs an empty conversation.
func New() Model {
	return Model{}
}

// SetVersion sets the version label used on the welcome splash.
func (m *Model) SetVersion(v string) { m.version = v }

// IsEmpty reports whether nothing has been emitted yet and no message is
// pending. Used by tests and by /clear to decide whether a splash is
// needed.
func (m *Model) IsEmpty() bool {
	return m.pending == nil && len(m.seen) == 0
}

// Resize records the terminal dimensions. Width is used by render
// helpers for wrapping; height is retained for API compatibility with
// the previous viewport-based model but is otherwise unused.
func (m *Model) Resize(width, height int) {
	m.width = width
	m.height = height
}

// EmitWelcomeSplash pushes the one-shot welcome splash onto the emits
// queue so the App's DrainEmits wrapper commits it to scrollback via
// tea.Println on the next Update cycle. No-op once already emitted, or
// when width is not yet known (defer until first WindowSizeMsg).
func (m *Model) EmitWelcomeSplash() {
	if m.welcomePrinted || m.width <= 0 {
		return
	}
	m.welcomePrinted = true
	m.emit(welcome.RenderInline(m.width, m.version))
}

// AppendUser commits a user message to scrollback.
func (m *Model) AppendUser(content string) {
	m.emit(renderMessage(Message{
		Kind:    KindUser,
		Author:  "You",
		Color:   theme.AccentPrimary,
		Content: content,
	}, m.contentWidth()))
}

// AppendQueuedUser commits an optimistic user message while another
// foreground operation is still running. The App later sends the same
// text without emitting a duplicate user bubble.
func (m *Model) AppendQueuedUser(content string) {
	m.emit(renderMessage(Message{
		Kind:    KindUser,
		Content: content,
		Queued:  true,
	}, m.contentWidth()))
}

// AppendAgentText appends a streaming chunk to the pending agent
// message, creating it if absent. Not committed yet — the live region
// shows the in-progress render via PendingView.
func (m *Model) AppendAgentText(model, providerSlug, chunk string) {
	if m.pending != nil && m.pending.Kind == KindAgent {
		m.pending.Content += chunk
		return
	}
	m.flushPending()
	m.pending = &Message{
		Kind:    KindAgent,
		Author:  fmt.Sprintf("packetcode (%s)", model),
		Color:   theme.ProviderColor(providerSlug),
		Content: chunk,
	}
}

// AppendAgentReasoning appends a streaming chunk of the model's reasoning
// summary to the pending agent message, creating it if absent. Reasoning
// arrives before (and possibly interleaved with) the answer text; both live in
// the same pending block, with reasoning rendered dim above the answer.
func (m *Model) AppendAgentReasoning(model, providerSlug, chunk string) {
	if m.pending != nil && m.pending.Kind == KindAgent {
		m.pending.Reasoning += chunk
		return
	}
	m.flushPending()
	m.pending = &Message{
		Kind:      KindAgent,
		Author:    fmt.Sprintf("packetcode (%s)", model),
		Color:     theme.ProviderColor(providerSlug),
		Reasoning: chunk,
	}
}

// FinaliseAgent commits the pending agent message (if any) to
// scrollback. Called after agent.EventDone.
func (m *Model) FinaliseAgent() {
	if m.pending != nil && m.pending.Kind == KindAgent {
		m.emit(renderMessage(*m.pending, m.contentWidth()))
		m.pending = nil
	}
}

// AppendToolCall starts a pending tool call. Awaits CompleteToolCall.
// If another message is pending, it is flushed first.
func (m *Model) AppendToolCall(toolName, args string) {
	m.AppendToolCallWithID(toolName, args, "")
}

// AppendToolCallWithID is AppendToolCall plus the provider call id, so
// later EventToolOutputChunk events can be routed to this exact pending
// block via AppendToolOutput. The plain AppendToolCall is retained for
// callers (and tests) that do not have a call id.
func (m *Model) AppendToolCallWithID(toolName, args, callID string) {
	if m.pending != nil && m.pending.Kind == KindAgent {
		m.pending = nil
	} else {
		m.flushPending()
	}
	m.pending = &Message{
		Kind:       KindToolCall,
		ToolName:   toolName,
		ToolArgs:   args,
		ToolCallID: callID,
	}
}

// AppendToolOutput appends a streamed stdout/stderr chunk to the pending
// tool-call block identified by callID, for live display while a
// long-running command runs. It is a no-op unless a tool call is pending
// and either its id matches callID or it has no id recorded yet (the
// latter tolerates producers that omit the id, or a call proposed before
// the id was known). Returns true if the chunk was applied — the App uses
// this to decide whether a re-render is worth scheduling.
//
// The accumulated preview is tail-bounded by maxLiveOutput; the
// authoritative, fully bounded result still arrives via CompleteToolCall.
func (m *Model) AppendToolOutput(callID, chunk string) bool {
	if chunk == "" {
		return false
	}
	if m.pending == nil || m.pending.Kind != KindToolCall {
		return false
	}
	if m.pending.ToolCallID != "" && callID != "" && m.pending.ToolCallID != callID {
		return false
	}
	if m.pending.ToolCallID == "" && callID != "" {
		// Adopt the id from the first chunk so subsequent chunks route
		// strictly by id.
		m.pending.ToolCallID = callID
	}
	m.pending.LiveOutput += chunk
	if len(m.pending.LiveOutput) > maxLiveOutput {
		m.pending.LiveOutput = m.pending.LiveOutput[len(m.pending.LiveOutput)-maxLiveOutput:]
	}
	return true
}

// CompleteToolCall fills in the tool result and commits the tool-call
// block to scrollback. Matches by name against the pending tool call.
// Silently no-ops if there's no matching pending call.
func (m *Model) CompleteToolCall(toolName string, res tools.ToolResult) {
	if m.pending == nil || m.pending.Kind != KindToolCall || m.pending.ToolName != toolName {
		return
	}
	m.pending.ToolResult = res.Content
	m.pending.IsError = res.IsError
	// Drop the streamed preview: the committed ToolResult is the single
	// authoritative copy. renderToolCall prefers ToolResult over
	// LiveOutput anyway, but clearing keeps the committed Message clean.
	m.pending.LiveOutput = ""
	m.emit(renderMessage(*m.pending, m.contentWidth()))
	m.pending = nil
}

// AppendSystem commits a system note to scrollback.
func (m *Model) AppendSystem(content string) {
	m.emit(renderMessage(Message{Kind: KindSystem, Content: content}, m.contentWidth()))
}

// AppendError commits an actionable failure using the same assistant marker as
// Claude Code, but in the semantic error color. Routine informational notes
// continue to use AppendSystem's dim dot marker.
func (m *Model) AppendError(content string) {
	m.emit(renderMessage(Message{Kind: KindError, Content: content}, m.contentWidth()))
}

// PendingView renders the current pending message for the live region.
// Returns "" when nothing is pending.
func (m Model) PendingView() string {
	if m.pending == nil {
		return ""
	}
	return renderMessage(*m.pending, m.contentWidth())
}

// DrainEmits returns the FIFO queue of finalised rendered messages and
// clears it. The App wraps each entry in tea.Println to commit to
// terminal scrollback.
func (m *Model) DrainEmits() []string {
	out := m.emits
	m.emits = nil
	return out
}

// View returns the cumulative transcript (every committed message ever
// emitted, whether or not it has been drained for tea.Println) plus the
// current pending message. Retained for test harnesses that snapshot
// the full conversation; production uses DrainEmits + tea.Println for
// committed content and PendingView for the live region.
func (m Model) View() string {
	if len(m.seen) == 0 && m.pending == nil {
		return ""
	}
	parts := make([]string, 0, len(m.seen)+1)
	parts = append(parts, m.seen...)
	if m.pending != nil {
		parts = append(parts, renderMessage(*m.pending, m.contentWidth()))
	}
	return strings.Join(parts, "\n")
}

// Update consumes Bubble Tea messages. Inline rendering relies on native
// terminal scrollback — no in-app scroll keys, no viewport — so this is
// a no-op. Kept so the component still participates in Bubble Tea's
// Update routing without needing a special case in the App.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) { return m, nil }

// emit pushes a rendered string onto the FIFO queue. No-op for empty
// strings (e.g. system message with empty content). A trailing blank line is
// appended so committed messages are separated by a gap (Claude Code style)
// rather than stacked flush against each other.
func (m *Model) emit(rendered string) {
	if rendered == "" {
		return
	}
	rendered += "\n"
	m.emits = append(m.emits, rendered)
	m.seen = append(m.seen, rendered)
}

// flushPending commits any pending message to scrollback. Used when a
// new pending slot is about to overwrite the current one (e.g. a tool
// call proposed while agent text was still streaming).
func (m *Model) flushPending() {
	if m.pending == nil {
		return
	}
	m.emit(renderMessage(*m.pending, m.contentWidth()))
	m.pending = nil
}

// contentWidth is the effective render width for a message bubble —
// terminal width minus a small gutter. Falls back to a sane default
// before the first WindowSizeMsg arrives.
func (m Model) contentWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	return w - 2
}

// ────────────────────────────────────────────────────────────────────────────
// Per-message rendering
// ────────────────────────────────────────────────────────────────────────────

func renderMessage(msg Message, width int) string {
	switch msg.Kind {
	case KindUser:
		// Flat "❯ text" — no bordered box (Claude Code style).
		if msg.Queued {
			return flatBlock("❯", theme.TextDim, theme.StyleDim.Render(msg.Content+"  (queued)"), width)
		}
		return flatBlock("❯", theme.AccentPrimary, theme.StylePrimary.Render(msg.Content), width)
	case KindAgent:
		out := ""
		if strings.TrimSpace(msg.Reasoning) != "" {
			out = flatBlock("✻", theme.TextDim, theme.StyleDim.Italic(true).Render("Thinking… "+strings.TrimRight(msg.Reasoning, "\n")), width)
		}
		if strings.TrimSpace(msg.Content) != "" {
			answer := flatBlock("⏺", msg.Color, msg.Content, width)
			if out != "" {
				out += "\n" + answer
			} else {
				out = answer
			}
		}
		return out
	case KindSystem:
		if msg.Content == "" {
			return ""
		}
		// Dim, prefixed, no box.
		return flatBlock("·", theme.TextDim, theme.StyleDim.Render(msg.Content), width)
	case KindError:
		if msg.Content == "" {
			return ""
		}
		return flatBlock("⏺", theme.Error, theme.StyleError.Render(msg.Content), width)
	case KindToolCall:
		return renderToolCall(msg, width)
	}
	return ""
}

// flatBlock renders body with a colored marker on the first line and any
// continuation lines indented to align under the text — the flat,
// bordered-box-free style Claude Code uses. body may already carry styling; it
// is wrapped to the available width.
func flatBlock(marker string, markerColor lipgloss.Color, body string, width int) string {
	m := lipgloss.NewStyle().Foreground(markerColor).Render(marker)
	mw := lipgloss.Width(marker)
	indent := strings.Repeat(" ", mw+1)
	avail := width - mw - 1
	if avail < 10 {
		avail = 10
	}
	wrapped := lipgloss.NewStyle().Width(avail).Render(body)
	lines := strings.Split(wrapped, "\n")
	for i, l := range lines {
		if i == 0 {
			lines[i] = m + " " + l
		} else {
			lines[i] = indent + l
		}
	}
	return strings.Join(lines, "\n")
}

func renderToolCall(msg Message, width int) string {
	head := lipgloss.NewStyle().Foreground(theme.AccentSecondary).Render("⏺") + " " +
		theme.StylePrimary.Render(toolDisplay(msg.ToolName, msg.ToolArgs))
	parts := []string{head}

	var body string
	switch {
	case msg.ToolResult != "":
		body = renderToolResultBody(msg, width-6)
	case msg.LiveOutput != "":
		// Live streaming preview shown only while the command runs and no
		// committed result exists yet. Dimmed so it reads as in-progress;
		// replaced wholesale by the result block on CompleteToolCall.
		body = theme.StyleDim.Render(strings.TrimRight(msg.LiveOutput, "\n"))
	}
	if body != "" {
		// "⎿" tree connector on the first result line (Claude Code style),
		// continuation indented under it.
		conn := theme.StyleDim.Render("⎿")
		for i, l := range strings.Split(body, "\n") {
			if i == 0 {
				parts = append(parts, "  "+conn+" "+l)
			} else {
				parts = append(parts, "     "+l)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func toolDisplay(name, args string) string {
	var fields map[string]any
	_ = json.Unmarshal([]byte(args), &fields)
	field := func(keys ...string) string {
		for _, key := range keys {
			if value, ok := fields[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		return ""
	}
	display, detail := name, ""
	switch name {
	case "read_file":
		display, detail = "Read", field("path")
	case "write_file":
		display, detail = "Write", field("path")
	case "patch_file":
		display, detail = "Update", field("path")
	case "execute_command":
		display, detail = "Bash", field("command")
	case "search_codebase":
		display, detail = "Search", field("query", "pattern")
	case "list_directory":
		display, detail = "List", field("path")
	case "list_symbols":
		display, detail = "Symbols", field("path")
	case "find_definition":
		display, detail = "Definition", field("symbol", "name")
	case "find_references":
		display, detail = "References", field("symbol", "name")
	case "get_diagnostics":
		display, detail = "Diagnostics", field("path")
	case "spawn_agent":
		display, detail = "Agent", field("prompt")
	case "collect_agent_results":
		display, detail = "Collect", field("scope")
	default:
		detail = strings.TrimSpace(args)
	}
	if detail == "" {
		return display
	}
	return display + theme.StyleDim.Render("("+truncate(detail, 120)+")")
}

func renderToolResultBody(msg Message, width int) string {
	if msg.IsError {
		return theme.StyleError.Render(msg.ToolResult)
	}
	if msg.ToolName == "patch_file" {
		if rendered, ok := tryRenderDiffResult(msg.ToolResult, width); ok {
			return rendered
		}
	}
	return msg.ToolResult
}

// tryRenderDiffResult looks for a unified-diff marker inside a tool
// result and, if found, renders everything after it via the diff
// component.
func tryRenderDiffResult(content string, width int) (string, bool) {
	idx := strings.Index(content, "--- ")
	if idx < 0 {
		idx = strings.Index(content, "@@ ")
	}
	if idx < 0 {
		return "", false
	}
	prefix := strings.TrimRight(content[:idx], "\n")
	m, err := diff.Parse(content[idx:])
	if err != nil || m.Empty() {
		return "", false
	}
	m = m.SetWidth(width).SetMaxRows(200)
	out := m.View()
	if prefix != "" {
		return theme.StyleDim.Render(prefix) + "\n" + out, true
	}
	return out, true
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
