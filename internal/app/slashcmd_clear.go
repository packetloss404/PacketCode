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
	// A fresh model has no width until the next WindowSizeMsg, and its
	// fallback is 80 columns. Size it to the terminal we already know.
	fresh.Resize(a.width, a.height)
	a.conversation = fresh
	// Clear native terminal scrollback as well as the in-memory transcript.
	// The live prompt/status region is repainted immediately by Bubble Tea.
	return a, tea.ClearScreen
}
