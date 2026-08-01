package app

import (
	"encoding/json"
	"testing"

	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
)

func TestRememberApproval_CommandIsExact(t *testing.T) {
	r := newTestApp(t)
	remembered := `go test "./internal/..."`
	args, err := json.Marshal(map[string]string{"command": remembered})
	if err != nil {
		t.Fatal(err)
	}
	r.app.rememberApproval(provider.ToolCall{Name: "execute_command", Arguments: string(args)})
	pol := r.app.currentPermissionPolicy()

	assert := func(command string, want permissions.Decision) {
		t.Helper()
		params, err := json.Marshal(map[string]string{"command": command})
		if err != nil {
			t.Fatal(err)
		}
		got := pol.Decide(permissions.Request{ToolName: "execute_command", RequiresApproval: true, Params: params}).Decision
		if got != want {
			t.Errorf("command %q: got %s, want %s", command, got, want)
		}
	}
	assert(remembered, permissions.DecisionAllow)
	for _, adversarial := range []string{
		`go test "./internal/..." && echo chained`,
		`go test "./internal/..."; echo chained`,
		`go test "./internal/..." | sh`,
		`go test "./internal/..." > result`,
		`go test "$(malicious)"`,
		`go test ./internal/...`,
		` go test "./internal/..."`,
		`go  test "./internal/..."`,
		`go test "./internal/..." `,
	} {
		assert(adversarial, permissions.DecisionAsk)
	}
}

func TestRememberApproval_InvalidCommandDoesNotAllowExecuteCommand(t *testing.T) {
	for _, args := range []string{`not json`, `{"command":""}`, `{"command":"   "}`} {
		r := newTestApp(t)
		r.app.rememberApproval(provider.ToolCall{Name: "execute_command", Arguments: args})
		params := json.RawMessage(`{"command":"anything"}`)
		got := r.app.currentPermissionPolicy().Decide(permissions.Request{ToolName: "execute_command", RequiresApproval: true, Params: params}).Decision
		if got == permissions.DecisionAllow {
			t.Fatalf("arguments %q installed a tool-wide allow", args)
		}
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

func TestRememberApproval_StripsBackgroundJobAnnotation(t *testing.T) {
	r := newTestApp(t)
	r.app.rememberApproval(provider.ToolCall{Name: "[job:abc12345] write_file", Arguments: `{"path":"a.go"}`})

	decision := r.app.currentPermissionPolicy().Decide(permissions.Request{ToolName: "write_file", RequiresApproval: true})
	if decision.Decision != permissions.DecisionAllow {
		t.Fatalf("write_file should be auto-allowed after background remember, got %v", decision.Decision)
	}
	annotated := r.app.currentPermissionPolicy().Decide(permissions.Request{ToolName: "[job:abc12345] write_file", RequiresApproval: true})
	if annotated.Decision == permissions.DecisionAllow {
		t.Fatal("remembered rule must target the real tool name, not the UI annotation")
	}
}
