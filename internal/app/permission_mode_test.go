package app

import (
	"testing"

	"github.com/packetcode/packetcode/internal/permissions"
)

func TestNextPermMode_CycleOrder(t *testing.T) {
	// normal → accept-edits → plan → normal
	if got := nextPermMode(modeNormal); got != modeAcceptEdits {
		t.Fatalf("normal → %v, want accept-edits", got)
	}
	if got := nextPermMode(modeAcceptEdits); got != modePlan {
		t.Fatalf("accept-edits → %v, want plan", got)
	}
	if got := nextPermMode(modePlan); got != modeNormal {
		t.Fatalf("plan → %v, want normal", got)
	}
	// Bypass is out of the forward cycle; Shift+Tab drops back to normal.
	if got := nextPermMode(modeBypass); got != modeNormal {
		t.Fatalf("bypass → %v, want normal", got)
	}
}

func TestCyclePermissionMode_WalksProfilesAndPlan(t *testing.T) {
	r := newTestApp(t)

	// Start from a known baseline.
	r.app.applyPermMode(modeNormal)
	if got := r.app.currentPermMode(); got != modeNormal {
		t.Fatalf("baseline mode = %v, want normal", got)
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileAsk {
		t.Fatalf("normal profile = %v, want ask", got)
	}

	// normal → accept-edits (profile "edit").
	r.app.cyclePermissionMode()
	if got := r.app.currentPermMode(); got != modeAcceptEdits {
		t.Fatalf("after 1st cycle = %v, want accept-edits", got)
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileEdit {
		t.Fatalf("accept-edits profile = %v, want edit", got)
	}

	// accept-edits → plan (planMode on, profile "safe").
	r.app.cyclePermissionMode()
	if got := r.app.currentPermMode(); got != modePlan {
		t.Fatalf("after 2nd cycle = %v, want plan", got)
	}
	if !r.app.planMode {
		t.Fatal("plan mode flag should be set")
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileSafe {
		t.Fatalf("plan profile = %v, want safe", got)
	}

	// plan → normal (flag cleared, profile "ask").
	r.app.cyclePermissionMode()
	if got := r.app.currentPermMode(); got != modeNormal {
		t.Fatalf("after 3rd cycle = %v, want normal", got)
	}
	if r.app.planMode {
		t.Fatal("plan mode flag should be cleared")
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileAsk {
		t.Fatalf("normal profile = %v, want ask", got)
	}
}

func TestCyclePermissionMode_NoOpWhileStreaming(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)
	r.app.streaming = true

	r.app.cyclePermissionMode()
	if got := r.app.currentPermMode(); got != modeNormal {
		t.Fatalf("streaming cycle changed mode to %v, want unchanged normal", got)
	}
}

func TestCyclePermissionMode_BypassExitsToNormal(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeBypass)
	if got := r.app.currentPermMode(); got != modeBypass {
		t.Fatalf("mode = %v, want bypass", got)
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileFull {
		t.Fatalf("bypass profile = %v, want full", got)
	}

	r.app.cyclePermissionMode()
	if got := r.app.currentPermMode(); got != modeNormal {
		t.Fatalf("bypass should cycle to normal, got %v", got)
	}
}
