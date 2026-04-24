// Package prompt renders a small centred modal that asks the user for a
// single line of text (API keys, quick answers). It lives in the same
// overlay slot as the approval and picker modals.
//
// Submission routes back to the App via SubmitMsg; Esc routes via
// CancelMsg. PromptID and Context let a single App-level handler
// disambiguate multiple prompt use cases.
package prompt

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/ui/theme"
)

// SubmitMsg is emitted when the user presses Enter. Value is the raw
// entered text (not masked even when the input is rendered masked).
type SubmitMsg struct {
	PromptID string
	Context  string
	Value    string
}

// CancelMsg is emitted when the user presses Esc.
type CancelMsg struct {
	PromptID string
	Context  string
}

// Model is a hidden-by-default single-line entry modal. Construct with
// New(); show with Open(...).
type Model struct {
	id      string
	visible bool

	title   string
	desc    string
	context string
	value   []rune
	masked  bool
	errMsg  string

	width  int
	height int
}

// New returns a hidden model keyed by id. The id flows through to
// SubmitMsg/CancelMsg so the App can route multiple prompts through one
// handler.
func New(id string) Model {
	return Model{id: id}
}

// Open shows the prompt. `context` is caller-owned metadata (e.g. a
// provider slug) that rides back in SubmitMsg/CancelMsg. `title` is the
// bold header; `desc` is the subline under it (hint text).
func (m *Model) Open(context, title, desc string, masked bool) {
	m.visible = true
	m.context = context
	m.title = title
	m.desc = desc
	m.masked = masked
	m.value = nil
	m.errMsg = ""
}

// Hide closes the prompt without emitting CancelMsg. Use when the
// caller has already handled the close (e.g. after SubmitMsg).
func (m *Model) Hide() {
	m.visible = false
	m.value = nil
	m.errMsg = ""
}

// Visible reports whether the prompt is on-screen.
func (m Model) Visible() bool { return m.visible }

// Context returns the context string the caller passed to Open. Used by
// handlers that need to know which prompt invocation is active (e.g.
// to cancel validation after the user hits Esc).
func (m Model) Context() string { return m.context }

// SetError replaces the error line. Empty string clears it. Does not
// dismiss the prompt — the user can edit their input and retry.
func (m *Model) SetError(s string) {
	m.errMsg = s
}

// Resize stores the viewport for centred rendering.
func (m *Model) Resize(w, h int) {
	m.width = w
	m.height = h
}

// Update routes input while the prompt is visible.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.visible {
		return m, nil
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "esc":
		id := m.id
		ctx := m.context
		m.Hide()
		return m, func() tea.Msg { return CancelMsg{PromptID: id, Context: ctx} }
	case "enter":
		id := m.id
		ctx := m.context
		value := string(m.value)
		return m, func() tea.Msg { return SubmitMsg{PromptID: id, Context: ctx, Value: value} }
	case "backspace":
		if n := len(m.value); n > 0 {
			m.value = m.value[:n-1]
		}
		return m, nil
	case "ctrl+u":
		m.value = nil
		return m, nil
	}
	if km.Type == tea.KeyRunes && len(km.Runes) > 0 {
		m.value = append(m.value, km.Runes...)
	} else if km.String() == "space" || km.String() == " " {
		m.value = append(m.value, ' ')
	}
	return m, nil
}

// View renders the centred modal. Returns "" when hidden.
func (m Model) View() string {
	if !m.visible {
		return ""
	}
	w := clampModalWidth(m.width)
	contentW := w - 4
	if contentW < 20 {
		contentW = 20
	}

	titleStyle := lipgloss.NewStyle().Foreground(theme.AccentPrimary).Bold(true)
	header := titleStyle.Render(m.title)

	lines := []string{header}
	if m.desc != "" {
		lines = append(lines, "", theme.StyleSecondary.Render(wrap(m.desc, contentW)))
	}

	field := m.renderField(contentW)
	lines = append(lines, "", field)

	if m.errMsg != "" {
		lines = append(lines, "", theme.StyleError.Render(wrap(m.errMsg, contentW)))
	}

	footer := theme.StyleDim.Render("⏎ submit · Esc cancel")
	lines = append(lines, "", footer)

	body := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.AccentPrimary).
		Padding(1, 2).
		Width(w - 4).
		Render(body)

	if m.width <= 0 || m.height <= 0 {
		return box
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// renderField draws the input cell: masked value if configured, a
// trailing cursor block, padded out to the available width.
func (m Model) renderField(width int) string {
	var display string
	if m.masked {
		display = strings.Repeat("•", len(m.value))
	} else {
		display = string(m.value)
	}
	cursor := lipgloss.NewStyle().Reverse(true).Render(" ")
	runes := []rune(display)
	if len(runes) > width-2 {
		// Keep the trailing edge visible so the user sees what they're typing.
		display = string(runes[len(runes)-(width-2):])
	}
	line := display + cursor
	pad := width - lipgloss.Width(line)
	if pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(theme.AccentPrimary).
		Render(line)
}

func clampModalWidth(terminal int) int {
	switch {
	case terminal <= 0:
		return 60
	case terminal < 44:
		return terminal
	case terminal < 80:
		return terminal - 4
	default:
		return 72
	}
}

// wrap is a minimal line-wrap helper so long description text doesn't
// overflow the modal. Splits on whitespace; never breaks mid-word.
func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	var b strings.Builder
	line := 0
	for _, word := range strings.Fields(s) {
		wlen := len([]rune(word))
		switch {
		case line == 0:
			b.WriteString(word)
			line = wlen
		case line+1+wlen > width:
			b.WriteString("\n")
			b.WriteString(word)
			line = wlen
		default:
			b.WriteString(" ")
			b.WriteString(word)
			line += 1 + wlen
		}
	}
	return b.String()
}
