package app

import (
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/ui/theme"
)

// permMode is the Claude Code-style permission mode surfaced beneath the
// input and cycled with Shift+Tab. It is a view over two pieces of existing
// state — the read-only "plan mode" flag and the permission policy's active
// profile — not a new source of truth:
//
//	modeNormal      → profile "ask"   (mutating tools prompt)
//	modeAcceptEdits → profile "edit"  (file edits auto-approve; shell prompts)
//	modePlan        → planMode + "safe" (read-only; propose a plan)
//	modeBypass      → profile "full"  (everything auto-approves; via /trust on)
type permMode int

const (
	modeNormal permMode = iota
	modeAcceptEdits
	modePlan
	modeBypass
)

// currentPermMode derives the active mode from the plan flag and the policy
// profile. Plan mode wins because it layers a read-only profile plus a
// per-turn instruction on top of whatever profile it replaced.
func (a *App) currentPermMode() permMode {
	if a.planMode {
		return modePlan
	}
	switch a.currentPermissionPolicy().Profile() {
	case permissions.ProfileEdit:
		return modeAcceptEdits
	case permissions.ProfileFull:
		return modeBypass
	default:
		return modeNormal
	}
}

// nextPermMode is the Shift+Tab cycle: normal → accept-edits → plan → normal.
// Bypass is deliberately not in the forward cycle (it is the dangerous,
// stickier mode reached via /trust on); from bypass, Shift+Tab drops back to
// normal so the user can always step out of it.
func nextPermMode(m permMode) permMode {
	switch m {
	case modeNormal:
		return modeAcceptEdits
	case modeAcceptEdits:
		return modePlan
	case modePlan:
		return modeNormal
	case modeBypass:
		return modeNormal
	default:
		return modeNormal
	}
}

// cyclePermissionMode advances to the next mode on Shift+Tab. It is a no-op
// while a turn is streaming — mirroring the /plan guard, since flipping the
// profile (or the plan instruction) mid-turn would disrupt in-flight tool
// approvals. The bottom indicator reflects the unchanged mode, so the key
// simply does nothing until the turn settles.
func (a *App) cyclePermissionMode() {
	if a.streaming {
		return
	}
	a.applyPermMode(nextPermMode(a.currentPermMode()))
}

// applyPermMode transitions to target, reusing the same primitives as the
// /plan and /trust commands so the bookkeeping (planPrevProfile for /plan
// off, preTrustPolicy for /trust off) stays consistent no matter which path
// changed the mode. setPermissionPolicy refreshes the top bar.
func (a *App) applyPermMode(target permMode) {
	if target == a.currentPermMode() {
		return
	}
	// Leaving plan mode: drop the flag before repointing the profile.
	if a.planMode && target != modePlan {
		a.planMode = false
	}
	switch target {
	case modeNormal:
		a.preTrustPolicy = nil
		a.setPermissionPolicy(a.currentPermissionPolicy().WithProfile(permissions.ProfileAsk))
	case modeAcceptEdits:
		a.preTrustPolicy = nil
		a.setPermissionPolicy(a.currentPermissionPolicy().WithProfile(permissions.ProfileEdit))
	case modePlan:
		a.planPrevProfile = a.currentPermissionPolicy().Profile()
		a.preTrustPolicy = nil
		a.setPermissionPolicy(a.currentPermissionPolicy().WithProfile(permissions.ProfileSafe))
		a.planMode = true
		a.refreshTopBar()
	case modeBypass:
		if a.currentPermissionPolicy().Profile() != permissions.ProfileFull {
			a.preTrustPolicy = a.currentPermissionPolicy()
		}
		a.setPermissionPolicy(a.currentPermissionPolicy().WithProfile(permissions.ProfileFull))
	}
}

// permModeHint renders the one-line indicator shown directly beneath the
// input, in the style of Claude Code's mode footer. Normal mode still shows a
// dim affordance so the Shift+Tab shortcut is discoverable.
func (a *App) permModeHint() string {
	dim := theme.StyleDim
	switch a.currentPermMode() {
	case modeAcceptEdits:
		return theme.StyleSuccess.Render("⏵⏵ accept edits on") + dim.Render("  ·  shift+tab to cycle")
	case modePlan:
		return theme.StyleAccent.Render("⏸ plan mode on") + dim.Render("  ·  shift+tab to cycle")
	case modeBypass:
		return theme.StyleWarning.Render("⏵⏵ bypass permissions on") + dim.Render("  ·  shift+tab or /trust off to exit")
	default:
		return dim.Render("shift+tab to cycle mode  ·  normal → accept edits → plan")
	}
}
