package app

import (
	"testing"

	"github.com/packetcode/packetcode/internal/permissions"
)

func TestPlanMode_TogglesReadOnlyAndRestores(t *testing.T) {
	r := newTestApp(t)
	prev := r.app.currentPermissionPolicy().Profile()

	r.app.handleSlashCommand("plan", []string{"on"}, "/plan on")
	if !r.app.planMode {
		t.Fatal("plan mode should be on")
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileSafe {
		t.Fatalf("plan mode must force read_only, got %v", got)
	}

	r.app.handleSlashCommand("plan", []string{"off"}, "/plan off")
	if r.app.planMode {
		t.Fatal("plan mode should be off")
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != prev {
		t.Fatalf("profile not restored: got %v want %v", got, prev)
	}
}

func TestPlanMode_BareToggle(t *testing.T) {
	r := newTestApp(t)
	r.app.handleSlashCommand("plan", nil, "/plan")
	if !r.app.planMode {
		t.Fatal("bare /plan should enter plan mode")
	}
	r.app.handleSlashCommand("plan", nil, "/plan")
	if r.app.planMode {
		t.Fatal("bare /plan again should exit plan mode")
	}
}
