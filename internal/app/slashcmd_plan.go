package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/permissions"
)

// planModeInstruction is prepended to each turn while plan mode is on. It steers
// the model to research and propose a plan instead of trying (and being denied)
// to edit — the read_only policy already blocks mutations; this makes the
// behavior intentional rather than a wall of denials.
const planModeInstruction = "[Plan mode: research only. Do NOT edit files or run state-changing commands — those are disabled. Investigate using read-only tools, then present a concise, numbered plan of the changes you would make and stop for approval. The user will disable plan mode to let you execute.]\n\n"

// handlePlanCommand toggles read-only "plan mode". Bare /plan toggles; /plan on
// and /plan off are explicit.
func (a *App) handlePlanCommand(args []string) (tea.Model, tea.Cmd) {
	want := !a.planMode // bare toggle
	if len(args) > 0 {
		switch args[0] {
		case "on":
			want = true
		case "off", "done":
			want = false
		default:
			a.conversation.AppendSystem("usage: /plan [on|off]")
			return a, nil
		}
	}

	if want == a.planMode {
		if a.planMode {
			a.conversation.AppendSystem("plan mode is already on — /plan off to approve and execute")
		} else {
			a.conversation.AppendSystem("plan mode is already off")
		}
		return a, nil
	}
	if a.streaming {
		a.conversation.AppendSystem("finish or cancel the current turn (Ctrl+C) before changing plan mode")
		return a, nil
	}

	if want {
		a.planPrevProfile = a.currentPermissionPolicy().Profile()
		if a.planPrevProfile != permissions.ProfileFull {
			a.preTrustPolicy = nil
		}
		a.setPermissionPolicy(a.currentPermissionPolicy().WithProfile(permissions.ProfileSafe))
		a.planMode = true
		a.refreshTopBar()
		a.conversation.AppendSystem("plan mode ON — read-only. The model will research and propose a plan; edits and commands are disabled. Type /plan off to approve and execute.")
		return a, nil
	}

	a.setPermissionPolicy(a.currentPermissionPolicy().WithProfile(a.planPrevProfile))
	a.planMode = false
	a.refreshTopBar()
	a.conversation.AppendSystem("plan mode OFF — editing enabled (profile: " + permissions.ProfileConfigName(a.planPrevProfile) + ")")
	return a, nil
}
