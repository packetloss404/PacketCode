package main

import (
	"testing"

	"github.com/packetcode/packetcode/internal/computers"
)

func TestComputerWorkspaceIdentityBindsEndpointAndRoot(t *testing.T) {
	base := computers.Computer{
		Kind:               computers.KindSSH,
		SSHUser:            "deploy",
		SSHHost:            "example.internal",
		SSHPort:            22,
		SSHHostFingerprint: "SHA256:approved",
		ProjectRoots:       []string{"/srv/app"},
	}
	want := computerWorkspaceIdentity(base)
	if want == "" {
		t.Fatal("identity is empty")
	}

	changed := base
	changed.SSHHostFingerprint = "SHA256:replacement"
	if got := computerWorkspaceIdentity(changed); got == want {
		t.Fatal("host-key change did not change workspace identity")
	}

	changed = base
	changed.ProjectRoots = []string{"/srv/other"}
	if got := computerWorkspaceIdentity(changed); got == want {
		t.Fatal("root change did not change workspace identity")
	}

	changed = base
	changed.Name = "renamed-display-label"
	changed.ID = "pc_renamed"
	if got := computerWorkspaceIdentity(changed); got != want {
		t.Fatal("display name/id should not alter endpoint identity")
	}
}
