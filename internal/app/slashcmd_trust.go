package app

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleTrustCommand toggles or reports the session's trust mode. With
// trust mode on, destructive tool calls auto-approve; with it off, the
// approval modal is raised. Escalation in either direction is immediate
// — the user is only ever mutating their own session's behaviour.
func (a *App) handleTrustCommand(args []string) (tea.Model, tea.Cmd) {
	set, value, err := parseTrustArgs(args)
	if err != nil {
		a.conversation.AppendSystem("trust: " + err.Error())
		return a, nil
	}
	if !set {
		state := "off"
		if a.approver.IsTrusted() {
			state = "on"
		}
		a.conversation.AppendSystem("trust mode: " + state)
		return a, nil
	}
	a.approver.SetTrust(value)
	if value {
		a.conversation.AppendSystem("trust mode enabled — destructive tools will auto-approve")
	} else {
		a.conversation.AppendSystem("trust mode disabled — destructive tools will prompt")
	}
	return a, nil
}
