package app

import (
	"testing"

	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
)

func TestCommandPrefixFromArgs(t *testing.T) {
	cases := []struct {
		args string
		want []string
	}{
		{`{"command":"go test ./..."}`, []string{"go", "test"}},
		{`{"command":"git status"}`, []string{"git", "status"}},
		{`{"command":"ls -la"}`, []string{"ls"}},
		{`{"command":"echo hi"}`, []string{"echo"}},
		{`{"command":""}`, nil},
		{`not json`, nil},
	}
	for _, c := range cases {
		got := commandPrefixFromArgs(c.args)
		if len(got) != len(c.want) {
			t.Fatalf("args %q => %v, want %v", c.args, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("args %q => %v, want %v", c.args, got, c.want)
			}
		}
	}
}

func TestRememberApproval_CommandScopedRule(t *testing.T) {
	r := newTestApp(t)
	// "Always allow" a `go test` command.
	r.app.rememberApproval(provider.ToolCall{Name: "execute_command", Arguments: `{"command":"go test ./..."}`})

	pol := r.app.currentPermissionPolicy()
	// A future `go test` command is now auto-allowed…
	allow := pol.Decide(permissions.Request{
		ToolName:         "execute_command",
		RequiresApproval: true,
		Params:           []byte(`{"command":"go test ./internal/..."}`),
	})
	if allow.Decision != permissions.DecisionAllow {
		t.Fatalf("go test should be auto-allowed, got %v", allow.Decision)
	}
	// …but an unrelated command is not.
	rm := pol.Decide(permissions.Request{
		ToolName:         "execute_command",
		RequiresApproval: true,
		Params:           []byte(`{"command":"rm -rf /"}`),
	})
	if rm.Decision == permissions.DecisionAllow {
		t.Fatalf("unrelated command must not be auto-allowed, got %v", rm.Decision)
	}
}

func TestRememberApproval_ToolScopedRule(t *testing.T) {
	r := newTestApp(t)
	r.app.rememberApproval(provider.ToolCall{Name: "write_file", Arguments: `{"path":"a.go"}`})
	d := r.app.currentPermissionPolicy().Decide(permissions.Request{ToolName: "write_file", RequiresApproval: true})
	if d.Decision != permissions.DecisionAllow {
		t.Fatalf("write_file should be auto-allowed after remember, got %v", d.Decision)
	}
}
