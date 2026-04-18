package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// handleUndoCommand pops the session's BackupManager stack and restores
// (or deletes, for create-then-undo) the most recent file change. The
// user explicitly approved the original write, so reversing it doesn't
// need a second confirmation.
func (a *App) handleUndoCommand(_ []string) (tea.Model, tea.Cmd) {
	if a.backups == nil {
		a.conversation.AppendSystem("undo: backups not available")
		return a, nil
	}
	path, err := a.backups.Undo()
	if err != nil {
		a.conversation.AppendSystem("undo: " + err.Error())
		return a, nil
	}
	if path == "" {
		a.conversation.AppendSystem("nothing to undo")
		return a, nil
	}
	a.conversation.AppendSystem(fmt.Sprintf("restored %s (depth now: %d)", path, a.backups.Depth()))
	return a, nil
}
