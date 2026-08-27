package app

import (
	"strings"
	"testing"
)

func TestHelpReportsResolvedFeatureGateStates(t *testing.T) {
	rig := newTestApp(t)
	disabled := false
	rig.cfg.PacketComputers.Enabled = &disabled
	rig.cfg.Sugar.Enabled = &disabled
	rig.cfg.ACP.Enabled = &disabled
	rig.cfg.Conduit.ShadowEnabled = true

	help := rig.app.renderHelp()
	for _, want := range []string{
		"Feature gates",
		"packet_computers",
		"PACKETCODE_PACKET_COMPUTERS_ENABLED",
		"sugar",
		"PACKETCODE_SUGAR_ENABLED",
		"PACKETCODE_ACP_ENABLED",
		"conduit shadow",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
	// The first three gates are explicitly off. Conduit is NOT: shadow_enabled
	// is true above, and it no longer inherits Sugar's state, so reporting it
	// as disabled would misdescribe the resolved configuration.
	got := rig.app.featureGateHelpRows()
	for i, row := range got[:3] {
		if !strings.HasPrefix(row.Desc, "disabled") {
			t.Fatalf("gate %d (%s) = %q, want disabled", i, row.Key, row.Desc)
		}
	}
	if !strings.HasPrefix(got[3].Desc, "enabled") {
		t.Fatalf("conduit shadow = %q, want enabled independently of Sugar", got[3].Desc)
	}
}

func TestHelpDefaultsCompatibilityFeatureGatesOnAndShadowOff(t *testing.T) {
	rig := newTestApp(t)
	rows := rig.app.featureGateHelpRows()
	if !strings.HasPrefix(rows[0].Desc, "enabled") || !strings.HasPrefix(rows[1].Desc, "disabled") {
		t.Fatalf("default feature states = %#v, want Packet Computers enabled and fresh Sugar inactive", rows)
	}
	if !strings.HasPrefix(rows[2].Desc, "enabled") {
		t.Fatalf("default ACP state = %#v, want enabled", rows[2])
	}
	if !strings.HasPrefix(rows[3].Desc, "disabled") {
		t.Fatalf("default Conduit state = %#v, want disabled", rows[3])
	}
}
