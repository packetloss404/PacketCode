package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/permissions"
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
		if a.sessionPermissionPolicy().Profile() == permissions.ProfileFull {
			state = "on"
		}
		a.conversation.AppendSystem("trust mode: " + state)
		return a, nil
	}
	if value {
		restore := a.sessionPermissionPolicy()
		if a.planMode {
			restore = restore.WithProfile(a.planPrevProfile)
			a.planMode = false
		}
		if restore.Profile() != permissions.ProfileFull {
			a.preTrustPolicy = restore
		}
		a.approver.SetTrust(false)
		a.setSessionPermissionPolicy(restore.WithProfile(permissions.ProfileFull))
		a.conversation.AppendSystem("trust mode enabled — prompted tools will auto-approve unless policy denies them")
	} else {
		if a.sessionPermissionPolicy().Profile() != permissions.ProfileFull && !a.approver.IsTrusted() && a.preTrustPolicy == nil {
			a.conversation.AppendSystem("trust mode already disabled")
			return a, nil
		}
		a.approver.SetTrust(false)
		restore := a.trustOffPolicy()
		a.preTrustPolicy = nil
		a.setSessionPermissionPolicy(restore)
		a.conversation.AppendSystem("trust mode disabled — restored permission profile: " + permissions.ProfileConfigName(restore.Profile()))
	}
	return a, nil
}

func (a *App) trustOffPolicy() *permissions.Policy {
	restore := a.preTrustPolicy
	if restore == nil {
		restore = a.permissionBase
	}
	if restore == nil {
		restore = a.sessionPermissionPolicy().WithProfile(permissions.ProfileAsk)
	}
	if restore.Profile() == permissions.ProfileFull {
		restore = restore.WithProfile(permissions.ProfileAsk)
	}
	return restore
}
