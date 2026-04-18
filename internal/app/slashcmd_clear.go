package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/ui/components/conversation"
)

// handleClearCommand resets the conversation pane to the welcome splash
// state. Does NOT touch the session file on disk — only the in-memory
// display. Mirrors the existing Ctrl+L key binding so the two paths
// can't drift.
func (a *App) handleClearCommand(_ []string) (tea.Model, tea.Cmd) {
	fresh := conversation.New()
	if a.deps.Version != "" {
		fresh.SetVersion(a.deps.Version)
	} else {
		fresh.SetVersion("v1")
	}
	a.conversation = fresh
	// View() will size the viewport on the next render.
	return a, nil
}
