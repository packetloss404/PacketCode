// Package spinner is a Braille-frame "thinking" indicator.
//
// It's a thin wrapper around bubbles/spinner so the rest of the codebase
// only depends on our theme tokens, not Charm's animation defaults.
package spinner

import (
	bspinner "github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/ui/theme"
)

// brailleFrames cycles through the design system's specified spinner.
var brailleFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type Model struct {
	inner  bspinner.Model
	label  string
	active bool
}

func New() Model {
	s := bspinner.New()
	s.Spinner = bspinner.Spinner{
		Frames: brailleFrames,
		FPS:    80_000_000, // 80ms cadence per the design spec.
	}
	s.Style = lipgloss.NewStyle().Foreground(theme.AccentPrimary)
	return Model{inner: s, label: "Thinking..."}
}

// Start activates the spinner and returns a tick command. Callers must
// dispatch the returned Cmd from their Update.
func (m *Model) Start(label string) tea.Cmd {
	if label != "" {
		m.label = label
	}
	m.active = true
	return m.inner.Tick
}

// Stop halts the animation. Subsequent View() calls render empty.
func (m *Model) Stop() { m.active = false }

func (m Model) Active() bool { return m.active }

// Update consumes the spinner tick. Returns a Cmd that schedules the
// next tick (or nil when stopped).
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	var cmd tea.Cmd
	m.inner, cmd = m.inner.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if !m.active {
		return ""
	}
	return m.inner.View() + " " + theme.StyleDim.Render(m.label)
}
