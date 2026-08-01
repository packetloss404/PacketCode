package app

import (
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/ui/theme"
)

// permMode is the Claude Code-style permission mode surfaced beneath the
// input. It is a view over two pieces of existing state — the read-only
// "plan mode" flag and the permission policy's active profile — not a new
// source of truth:
//
//	modeNormal      → profile "ask"   (mutating tools prompt)
//	modeAcceptEdits → profile "edit"  (file edits auto; shell prompts)
//	modeAuto        → profile "auto"  (file edits + shell auto; MCP/gated prompt)
//	modePlan        → planMode + "safe" (read-only; propose a plan)
//	modeBypass      → profile "full"  (everything auto; deny rules a floor)
//
// Shift+Tab cycles the first four (normal → accept edits → auto → plan).
// modeBypass is deliberately NOT in the cycle — it is the dangerous "skip
// permissions" mode, entered on purpose via /trust on (which sets the full
// profile), shown in red. Shift+Tab from bypass steps back to normal so it is
// never a trap.
type permMode int

const (
	modeNormal permMode = iota
	modeAcceptEdits
	modeAuto
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
	case permissions.ProfileAuto:
		return modeAuto
	case permissions.ProfileFull:
		return modeBypass
	default:
		return modeNormal
	}
}

// nextPermMode is the Shift+Tab cycle: normal → accept-edits → auto → plan →
// normal. Bypass is not produced by the cycle (see permMode docs).
func nextPermMode(m permMode) permMode {
	switch m {
	case modeNormal:
		return modeAcceptEdits
	case modeAcceptEdits:
		return modeAuto
	case modeAuto:
		return modePlan
	case modePlan:
		return modeNormal
	default:
		return modeNormal
	}
}

// cyclePermissionMode advances to the next mode on Shift+Tab, including while
// a turn is active. The foreground agent reads the current policy at each tool
// action, so a mid-turn change governs subsequent actions. If an approval is
// already visible, re-evaluate it immediately against the new profile. From
// bypass, Shift+Tab steps back to normal (bypass is entered deliberately via
// /trust on, not by cycling).
func (a *App) cyclePermissionMode() {
	if a.currentPermMode() == modeBypass {
		a.applyPermMode(modeNormal)
	} else {
		a.applyPermMode(nextPermMode(a.currentPermMode()))
	}
	if a.approver != nil && a.approver.ResolveActiveByPolicy() {
		a.approval.Hide()
		a.showPendingApproval()
	}
}

// applyPermMode transitions to target, reusing the same primitives as the
// /plan and /trust commands so the bookkeeping (planPrevProfile for /plan
// off, preTrustPolicy for /trust off) stays consistent no matter which path
// changed the mode. setPermissionPolicy refreshes the top bar.
func (a *App) applyPermMode(target permMode) {
	if target == a.currentPermMode() {
		return
	}
	wasPlan := a.planMode
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
	case modeAuto:
		a.preTrustPolicy = nil
		a.setPermissionPolicy(a.currentPermissionPolicy().WithProfile(permissions.ProfileAuto))
	case modePlan:
		a.planPrevProfile = a.currentPermissionPolicy().Profile()
		a.preTrustPolicy = nil
		a.setPermissionPolicy(a.currentPermissionPolicy().WithProfile(permissions.ProfileSafe))
		a.planMode = true
		a.refreshTopBar()
	case modeBypass:
		restore := a.currentPermissionPolicy()
		if wasPlan {
			restore = restore.WithProfile(a.planPrevProfile)
		}
		if restore.Profile() != permissions.ProfileFull {
			a.preTrustPolicy = restore
		}
		a.setPermissionPolicy(restore.WithProfile(permissions.ProfileFull))
	}
}

// permModeHint renders the one-line indicator shown directly beneath the
// input, in the style of Claude Code's mode footer. Normal mode still shows a
// dim affordance so the Shift+Tab shortcut is discoverable. Bypass is red and
// names its own exit, since it is not part of the Shift+Tab cycle.
func (a *App) permModeHint() string {
	return renderPermModeHint(a.currentPermMode())
}

func renderPermModeHint(mode permMode) string {
	dim := theme.StyleDim
	switch mode {
	case modeAcceptEdits:
		return theme.StyleSuccess.Render("⏵⏵ accept edits on") + dim.Render(" (shift+tab to cycle) · ← for agents")
	case modeAuto:
		return theme.StyleWarning.Render("⏵⏵ auto mode on") + dim.Render(" (shift+tab to cycle) · ← for agents")
	case modePlan:
		return theme.StyleAccent.Render("⏸ plan mode on") + dim.Render(" (shift+tab to cycle) · ← for agents")
	case modeBypass:
		return theme.StyleError.Render("⏵⏵ bypass permissions on") + dim.Render(" (shift+tab to cycle) · ← for agents")
	default:
		return theme.StyleSecondary.Render("⏸ manual mode on") + dim.Render(" · ← for agents")
	}
}
