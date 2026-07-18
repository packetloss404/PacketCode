// Package input is the bottom-anchored multi-line text entry. Enter
// submits, Shift+Enter inserts a newline, / on an empty buffer is the
// hook a future autocomplete component would attach to.
package input

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/ui/theme"
)

// SubmitMsg is emitted when the user hits Enter on a non-empty buffer.
// The App routes it to agent.Run().
type SubmitMsg struct{ Text string }

type Model struct {
	ta      textarea.Model
	focused bool
	width   int
	maxRows int
}

func New() Model {
	ta := textarea.New()
	ta.Placeholder = "Ask packetcode anything... (/ commands · @ files)"
	ta.CharLimit = 0
	ta.MaxHeight = 4
	ta.ShowLineNumbers = false
	ta.Prompt = "> " // Claude Code-style prompt inside the input box
	ta.SetHeight(1)

	ta.FocusedStyle.Base = lipgloss.NewStyle().Foreground(theme.TextPrimary)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(theme.TextDim)
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(theme.AccentPrimary)

	ta.BlurredStyle.Base = lipgloss.NewStyle().Foreground(theme.TextSecondary)
	ta.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(theme.TextDim)

	ta.Focus()
	return Model{ta: ta, focused: true, maxRows: 4}
}

func (m *Model) Resize(width, height int) {
	m.width = width
	m.ta.SetWidth(width - 4)
	if height > 0 && height < m.maxRows+2 {
		m.maxRows = max(1, height-2)
	}
	m.syncHeight()
}

func (m *Model) Focus()        { m.focused = true; m.ta.Focus() }
func (m *Model) Blur()         { m.focused = false; m.ta.Blur() }
func (m *Model) Reset()        { m.ta.Reset(); m.syncHeight() }
func (m *Model) Value() string { return m.ta.Value() }

// AtFirstLine reports whether the caret is on the first visual line of the
// buffer. Used by prompt-history recall so Up only pages into history when
// the caret is at the top, otherwise it moves between lines as usual.
func (m *Model) AtFirstLine() bool { return m.ta.Line() == 0 }

// AtLastLine reports whether the caret is on the last visual line of the
// buffer — the mirror of AtFirstLine for Down.
func (m *Model) AtLastLine() bool { return m.ta.Line() >= m.ta.LineCount()-1 }

// SetValue replaces the textarea contents and moves the caret to the
// end. Used by the slash-command autocomplete popup to swap in the
// chosen "/<verb> " prefix without the user losing their typing flow.
func (m *Model) SetValue(s string) {
	m.ta.SetValue(s)
	m.ta.CursorEnd()
	m.syncHeight()
}

// ReplaceMention splices "@<path> " over the byte range [start, end) in the
// buffer, used by the @-file autocomplete to swap the partially-typed
// "@query" token for the chosen relative path. The caret lands at the end of
// the buffer: the popup is only ever open while the active token ends at the
// caret, so end is the caret and everything after it is preserved. Out-of-
// range bounds are ignored (defensive; the App always passes a valid span).
func (m *Model) ReplaceMention(start, end int, path string) {
	v := m.ta.Value()
	if start < 0 || end > len(v) || start > end {
		return
	}
	m.ta.SetValue(v[:start] + "@" + path + " " + v[end:])
	m.ta.CursorEnd()
	m.syncHeight()
}

// Update runs the textarea's own logic and intercepts Enter to fire SubmitMsg.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok && m.focused {
		switch km.Type {
		case tea.KeyEnter:
			text := m.ta.Value()
			if text != "" {
				cmd := func() tea.Msg { return SubmitMsg{Text: text} }
				return m, cmd
			}
			return m, nil
		}
		// Shift+Enter inserts a newline by falling through to the textarea.
	}
	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	m.syncHeight()
	return m, cmd
}

func (m Model) View() string {
	style := theme.StyleInputIdle
	if m.focused {
		style = theme.StyleInputFocused
	}
	body := m.ta.View()
	width := m.width - 2
	if width <= 0 {
		width = 80
	}
	return style.Width(width).Render(body)
}

func (m *Model) syncHeight() {
	rows := strings.Count(m.ta.Value(), "\n") + 1
	if rows < 1 {
		rows = 1
	}
	if m.maxRows <= 0 {
		m.maxRows = 4
	}
	if rows > m.maxRows {
		rows = m.maxRows
	}
	m.ta.SetHeight(rows)
}
