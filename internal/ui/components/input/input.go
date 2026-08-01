// Package input is the bottom-anchored multi-line text entry. Enter submits;
// portable alternate bindings insert newlines while slash and file completion
// are coordinated by the parent App.
package input

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/packetcode/packetcode/internal/ui/theme"
)

// SubmitMsg is emitted when the user hits Enter on a non-empty buffer.
// The App routes it to agent.Run().
type SubmitMsg struct{ Text string }

type Model struct {
	ta      textarea.Model
	focused bool
	width   int
	height  int
	maxRows int
}

func New() Model {
	ta := textarea.New()
	ta.Placeholder = "Ask packetcode anything... (/ commands · @ files)"
	ta.CharLimit = 0
	ta.MaxHeight = 4
	ta.ShowLineNumbers = false
	ta.Prompt = "❯ "
	ta.SetHeight(1)
	// Enter is owned by this component as the submit key. Keep explicit
	// newline fallbacks in the textarea keymap: modern terminals commonly
	// encode Shift+Enter as Ctrl+J. Alt+Enter is an additional fallback when
	// the terminal reports Alt distinctly; backslash-Enter works everywhere.
	ta.KeyMap.InsertNewline.SetKeys("ctrl+j", "alt+enter")

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
	m.height = height
	m.ta.SetWidth(max(1, width-2))
	m.syncHeight()
}

func (m *Model) Focus()        { m.focused = true; m.ta.Focus() }
func (m *Model) Blur()         { m.focused = false; m.ta.Blur() }
func (m *Model) Reset()        { m.ta.Reset(); m.syncHeight() }
func (m *Model) Value() string { return m.ta.Value() }

// SetMaxRows applies the configured upper bound for the expanding composer.
func (m *Model) SetMaxRows(rows int) {
	if rows <= 0 {
		return
	}
	m.maxRows = rows
	m.ta.MaxHeight = rows
	m.syncHeight()
}

// AtFirstLine reports whether the caret is on the first visual line of the
// buffer. Used by prompt-history recall so Up only pages into history when
// the caret is at the top, otherwise it moves between lines as usual.
func (m *Model) AtFirstLine() bool {
	info := m.ta.LineInfo()
	return m.ta.Line() == 0 && info.RowOffset == 0
}

// AtLastLine reports whether the caret is on the last visual line of the
// buffer — the mirror of AtFirstLine for Down.
func (m *Model) AtLastLine() bool {
	info := m.ta.LineInfo()
	return m.ta.Line() >= m.ta.LineCount()-1 && info.RowOffset >= info.Height-1
}

// SetValue replaces the textarea contents and moves the caret to the
// end. Used by the slash-command autocomplete popup to swap in the
// chosen "/<verb> " prefix without the user losing their typing flow.
func (m *Model) SetValue(s string) {
	m.ta.SetValue(s)
	m.ta.CursorEnd()
	m.syncHeight()
}

// CursorByteOffset returns the caret position as a byte offset into Value.
// textarea exposes its logical row and visual-line offsets separately; this
// method folds them back into the full buffer so callers can edit the token at
// the caret instead of assuming the caret is always at the end.
func (m *Model) CursorByteOffset() int {
	value := m.ta.Value()
	lines := strings.Split(value, "\n")
	row := m.ta.Line()
	if row < 0 {
		return 0
	}
	if row >= len(lines) {
		return len(value)
	}

	offset := 0
	for i := 0; i < row; i++ {
		offset += len(lines[i]) + 1 // include the newline separator
	}
	info := m.ta.LineInfo()
	col := info.StartColumn + info.ColumnOffset
	runes := []rune(lines[row])
	if col < 0 {
		col = 0
	}
	if col > len(runes) {
		col = len(runes)
	}
	return offset + len(string(runes[:col]))
}

// ValueBeforeCursor is the portion of Value to the left of the caret.
func (m *Model) ValueBeforeCursor() string {
	value := m.ta.Value()
	offset := m.CursorByteOffset()
	if offset < 0 || offset > len(value) {
		return value
	}
	return value[:offset]
}

// ReplaceMention splices "@<path> " over the byte range [start, end) in the
// buffer, used by the @-file autocomplete to swap the token under the caret
// for the chosen relative path. Text after the token is preserved and the
// caret lands immediately after the inserted mention (and one existing or
// inserted separator), so completion remains natural in the middle of a
// multiline draft. Out-of-range bounds are ignored defensively.
func (m *Model) ReplaceMention(start, end int, path string) {
	v := m.ta.Value()
	if start < 0 || end > len(v) || start > end {
		return
	}
	replacement := "@" + path
	suffix := v[end:]
	cursor := start + len(replacement)
	if suffix == "" {
		replacement += " "
		cursor++
	} else if r, size := utf8.DecodeRuneInString(suffix); unicode.IsSpace(r) {
		// Reuse the separator already present and move across exactly one rune.
		cursor += size
	} else {
		replacement += " "
		cursor++
	}
	updated := v[:start] + replacement + suffix
	m.ta.SetValue(updated)
	m.setCursorByteOffset(cursor)
	m.syncHeight()
}

// setCursorByteOffset restores the textarea caret after SetValue, which
// otherwise always leaves it at the end of the buffer.
func (m *Model) setCursorByteOffset(offset int) {
	value := m.ta.Value()
	if offset < 0 {
		offset = 0
	}
	if offset > len(value) {
		offset = len(value)
	}
	prefix := value[:offset]
	targetRow := strings.Count(prefix, "\n")
	lastNewline := strings.LastIndex(prefix, "\n")
	linePrefix := prefix
	if lastNewline >= 0 {
		linePrefix = prefix[lastNewline+1:]
	}
	targetCol := utf8.RuneCountInString(linePrefix)

	// SetValue leaves the caret on the final logical row. CursorUp traverses
	// visual wraps as well, so bound the loop while walking to targetRow.
	for steps := 0; m.ta.Line() > targetRow && steps <= m.ta.Length()+m.ta.LineCount(); steps++ {
		m.ta.CursorUp()
	}
	m.ta.SetCursor(targetCol)
}

// Update runs the textarea's own logic and intercepts Enter to fire SubmitMsg.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok && m.focused {
		switch km.String() {
		case "enter":
			text := m.ta.Value()
			cursor := m.CursorByteOffset()
			if cursor > 0 && cursor <= len(text) && text[cursor-1] == '\\' {
				// Claude-compatible universal fallback: a trailing backslash
				// escapes Enter into a newline even when the terminal cannot
				// distinguish Shift+Enter.
				updated := text[:cursor-1] + "\n" + text[cursor:]
				m.ta.SetValue(updated)
				m.setCursorByteOffset(cursor)
				m.syncHeight()
				return m, nil
			}
			if text != "" {
				cmd := func() tea.Msg { return SubmitMsg{Text: text} }
				return m, cmd
			}
			return m, nil
		}
		// Shift+Enter (reported as Ctrl+J by many terminals) and Alt+Enter
		// insert a newline by falling through to the textarea keymap.
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
	width := m.width
	if width <= 0 {
		width = 80
	}
	return style.Width(width).Render(body)
}

// ViewBlurred renders a copy with inactive focus styling while an overlay owns
// the keyboard. The live composer remains focused, so dismissing the overlay
// restores typing without an extra state transition or cursor jump.
func (m Model) ViewBlurred() string {
	m.focused = false
	m.ta.Blur()
	return m.View()
}

// ViewWithPlaceholder renders a copy with a context-specific placeholder.
// Agent View uses this to mirror Claude Code's "describe a task" prompt
// without mutating the normal chat placeholder or the underlying buffer.
func (m Model) ViewWithPlaceholder(placeholder string) string {
	m.ta.Placeholder = placeholder
	return m.View()
}

func (m *Model) syncHeight() {
	rows := 0
	contentWidth := m.width - 4 // input width minus prompt and breathing room
	for _, line := range strings.Split(m.ta.Value(), "\n") {
		visualRows := 1
		if contentWidth > 0 {
			visualRows = max(1, (runewidth.StringWidth(line)+contentWidth-1)/contentWidth)
		}
		rows += visualRows
	}
	if rows < 1 {
		rows = 1
	}
	if m.maxRows <= 0 {
		m.maxRows = 4
	}
	limit := m.maxRows
	if m.height > 0 && m.height < limit+2 {
		limit = max(1, m.height-2)
	}
	m.ta.MaxHeight = limit
	if rows > limit {
		rows = limit
	}
	m.ta.SetHeight(rows)
}
