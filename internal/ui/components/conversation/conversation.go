// Package conversation renders the scrollable transcript pane: the
// running list of user/assistant/tool messages with their tool-call
// outputs and inline diffs.
package conversation

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/tools"
	"github.com/packetcode/packetcode/internal/ui/components/diff"
	"github.com/packetcode/packetcode/internal/ui/components/welcome"
	"github.com/packetcode/packetcode/internal/ui/theme"
)

// MessageKind discriminates how a Message renders. We intentionally keep
// the discriminator in the conversation package — provider.Role is the
// data model, MessageKind is the visual model.
type MessageKind int

const (
	KindUser MessageKind = iota
	KindAgent
	KindSystem
	KindToolCall
)

// Message is the conversation's atomic display unit. Tool calls and tool
// results are merged into a single ToolCallBlock so the output reads
// linearly.
type Message struct {
	Kind     MessageKind
	Author   string // "You" / "packetcode (gpt-4.1)" / "system"
	Color    lipgloss.Color
	Content  string

	// ToolCall fields populated when Kind == KindToolCall.
	ToolName string
	ToolArgs string
	ToolResult string
	IsError    bool
	Collapsed  bool
}

// Model is the scrollable conversation pane.
type Model struct {
	messages   []Message
	viewport   viewport.Model
	width      int
	height     int
	autoScroll bool
	version    string
}

func New() Model {
	vp := viewport.New(0, 0)
	return Model{
		viewport:   vp,
		autoScroll: true,
	}
}

// SetVersion sets the version string shown on the welcome splash. Called
// once at construction time by the App.
func (m *Model) SetVersion(v string) { m.version = v }

// IsEmpty reports whether the pane is in welcome-splash state. Used by
// the App to decide whether to size for a splash or for the viewport.
func (m *Model) IsEmpty() bool {
	for _, msg := range m.messages {
		if !(msg.Kind == KindSystem && msg.Content == "") {
			return false
		}
	}
	return true
}

// Resize updates the viewport dimensions. Conversation content is re-laid
// out on next View().
func (m *Model) Resize(width, height int) {
	m.width = width
	m.height = height
	m.viewport.Width = width
	m.viewport.Height = height
	m.refresh()
}

// AppendUser adds a user message and scrolls to bottom.
func (m *Model) AppendUser(content string) {
	m.append(Message{
		Kind:    KindUser,
		Author:  "You",
		Color:   theme.AccentPrimary,
		Content: content,
	})
}

// AppendAgentText starts a new agent message or appends to the most
// recent in-progress one. Streaming chunks land here.
func (m *Model) AppendAgentText(model, providerSlug, chunk string) {
	if n := len(m.messages); n > 0 && m.messages[n-1].Kind == KindAgent {
		m.messages[n-1].Content += chunk
		m.refresh()
		return
	}
	m.append(Message{
		Kind:    KindAgent,
		Author:  fmt.Sprintf("packetcode (%s)", model),
		Color:   theme.ProviderColor(providerSlug),
		Content: chunk,
	})
}

// FinaliseAgent forces any subsequent AppendAgentText to start a new
// message instead of growing the previous one. Called after the agent
// emits EventDone.
func (m *Model) FinaliseAgent() {
	// Insert a sentinel non-agent message... actually no: we just check
	// "is the LAST message agent?" so to break the chain, we append an
	// invisible terminator. Simpler: the caller invokes FinaliseAgent
	// and the next AppendAgentText starts fresh because we mark the
	// last agent message as finalised via a tiny content tweak. To keep
	// the implementation honest, we append a no-render sentinel that
	// gets filtered in View.
	if n := len(m.messages); n > 0 && m.messages[n-1].Kind == KindAgent {
		m.append(Message{Kind: KindSystem, Content: ""}) // terminator
	}
}

// AppendToolCall starts a new tool call block (no result yet).
func (m *Model) AppendToolCall(toolName, args string) {
	m.append(Message{
		Kind:     KindToolCall,
		ToolName: toolName,
		ToolArgs: args,
	})
}

// CompleteToolCall fills in the result for the most recent in-progress
// tool call block matching the given name. We match by the most recent
// pending block to handle parallel tool calls in approximate order.
func (m *Model) CompleteToolCall(toolName string, res tools.ToolResult) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Kind != KindToolCall {
			continue
		}
		if m.messages[i].ToolName != toolName || m.messages[i].ToolResult != "" {
			continue
		}
		m.messages[i].ToolResult = res.Content
		m.messages[i].IsError = res.IsError
		m.messages[i].Collapsed = !res.IsError && len(res.Content) > 600
		break
	}
	m.refresh()
}

// AppendSystem renders an inline system note.
func (m *Model) AppendSystem(content string) {
	m.append(Message{Kind: KindSystem, Content: content})
}

func (m *Model) append(msg Message) {
	m.messages = append(m.messages, msg)
	m.refresh()
}

// ToggleCollapseLast flips the collapsed state of the most recent
// completed tool call. Bound to the Tab key on the conversation pane.
func (m *Model) ToggleCollapseLast() {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Kind == KindToolCall && m.messages[i].ToolResult != "" {
			m.messages[i].Collapsed = !m.messages[i].Collapsed
			m.refresh()
			return
		}
	}
}

// Update handles scroll keys (j/k, ↑/↓, gg, G) and viewport mouse
// (disabled — but Bubble Tea routes wheel events through here anyway).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "G":
			m.viewport.GotoBottom()
			m.autoScroll = true
			return m, nil
		case "g":
			m.viewport.GotoTop()
			m.autoScroll = false
			return m, nil
		case "j", "down":
			m.viewport.LineDown(1)
			m.autoScroll = m.viewport.AtBottom()
			return m, nil
		case "k", "up":
			m.viewport.LineUp(1)
			m.autoScroll = false
			return m, nil
		case "tab":
			m.ToggleCollapseLast()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.IsEmpty() {
		return welcome.Render(m.width, m.height, m.version)
	}
	return m.viewport.View()
}

// refresh re-renders all messages into the viewport content. Auto-scrolls
// to the bottom when m.autoScroll is set.
func (m *Model) refresh() {
	if m.width <= 0 {
		return
	}
	var b strings.Builder
	contentWidth := m.width - 2
	for _, msg := range m.messages {
		if msg.Kind == KindSystem && msg.Content == "" {
			continue
		}
		b.WriteString(renderMessage(msg, contentWidth))
		b.WriteString("\n")
	}
	m.viewport.SetContent(b.String())
	if m.autoScroll {
		m.viewport.GotoBottom()
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Per-message rendering
// ────────────────────────────────────────────────────────────────────────────

func renderMessage(msg Message, width int) string {
	switch msg.Kind {
	case KindUser:
		return renderBubble(msg.Author, msg.Color, msg.Content, theme.StyleUserMessage, width)
	case KindAgent:
		return renderBubble(msg.Author, msg.Color, msg.Content, theme.StyleAgentMessage, width)
	case KindSystem:
		return theme.StyleSystemMessage.Render(msg.Content)
	case KindToolCall:
		return renderToolCall(msg, width)
	}
	return ""
}

func renderBubble(author string, color lipgloss.Color, body string, base lipgloss.Style, width int) string {
	header := lipgloss.NewStyle().Foreground(color).Bold(true).Render(author)
	content := header + "\n" + body
	return base.Width(width - 2).Render(content)
}

// renderToolCall renders a tool invocation + (optionally) its result.
// Long results collapse to a single "▶ Expand" line that the user can
// pop open with Tab.
func renderToolCall(msg Message, width int) string {
	header := theme.LabelBadge(msg.ToolName, theme.AccentPrimary)
	args := truncate(msg.ToolArgs, 200)
	parts := []string{header + theme.StyleDim.Render("  "+args)}

	if msg.ToolResult != "" {
		divider := theme.StyleDim.Render(strings.Repeat("─", 24))
		parts = append(parts, divider, renderToolResultBody(msg, width-4))
	}
	return theme.StyleToolCall.Width(width - 2).Render(strings.Join(parts, "\n"))
}

// renderToolResultBody picks the right rendering for the result body
// (error / collapsed / diff / plain). Extracted from renderToolCall so
// the diff path stays testable in isolation.
func renderToolResultBody(msg Message, width int) string {
	if msg.IsError {
		return theme.StyleError.Render(msg.ToolResult)
	}
	if msg.Collapsed {
		lines := strings.Count(msg.ToolResult, "\n") + 1
		return theme.StyleDim.Render(fmt.Sprintf("▶ Output collapsed (%d lines) — press Tab to expand", lines))
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
// component. Anything before the marker (patch_file's "Applied N
// patches" preamble) is preserved dim above the diff so the user
// still sees the summary line.
//
// Conversation uses a 200-row cap — the viewport is scrollable so
// truncation is only about keeping gigantic diffs from hanging
// lipgloss.
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
