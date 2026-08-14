package permissions

import (
	"encoding/json"
	"testing"

	"github.com/packetcode/packetcode/internal/config"
)

func TestPolicy_DefaultProfilePromptsDestructiveAllowsRead(t *testing.T) {
	p := Must(config.PermissionConfig{})

	assertDecision(t, p, Request{ToolName: "read_file"}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "list_symbols"}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "find_definition"}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "find_references"}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "get_diagnostics"}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "collect_agent_results", RequiresApproval: true}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "write_file", RequiresApproval: true}, DecisionAsk)
	assertDecision(t, p, Request{ToolName: "execute_command", RequiresApproval: true}, DecisionAsk)
	assertDecision(t, p, Request{ToolName: "filesystem__write_file", RequiresApproval: true}, DecisionAsk)
}

func TestPolicy_FullProfileAllowsAllExceptExplicitDeny(t *testing.T) {
	p := Must(config.PermissionConfig{
		Profile: string(ProfileFull),
		Rules: []config.PermissionRule{{
			Tool:   "execute_command",
			Action: string(DecisionDeny),
			Reason: "shell disabled",
		}},
	})

	assertDecision(t, p, Request{ToolName: "write_file", RequiresApproval: true}, DecisionAllow)
	res := p.Decide(Request{ToolName: "execute_command", RequiresApproval: true})
	if res.Decision != DecisionDeny || res.Reason != "shell disabled" {
		t.Fatalf("deny rule = %+v", res)
	}
}

func TestPolicy_SafeProfileDeniesDestructiveAndAllowsRead(t *testing.T) {
	p := Must(config.PermissionConfig{Profile: string(ProfileSafe)})

	assertDecision(t, p, Request{ToolName: "search_codebase"}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "find_references"}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "get_diagnostics"}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "collect_agent_results", RequiresApproval: true}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "patch_file", RequiresApproval: true}, DecisionDeny)
	assertDecision(t, p, Request{ToolName: "spawn_agent", RequiresApproval: true}, DecisionDeny)
	assertDecision(t, p, Request{ToolName: "filesystem__read_file", RequiresApproval: true}, DecisionDeny)
}

func TestPolicy_RuleSpecificityAndOrder(t *testing.T) {
	p := Must(config.PermissionConfig{
		Profile: string(ProfileAsk),
		Rules: []config.PermissionRule{
			{Tool: "mcp:*", Action: string(DecisionAsk)},
			{Tool: "filesystem__read_file", Action: string(DecisionAllow)},
		},
	})

	assertDecision(t, p, Request{ToolName: "filesystem__read_file", RequiresApproval: true}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "filesystem__write_file", RequiresApproval: true}, DecisionAsk)
}

func TestPolicy_CommandPrefixMatchesFields(t *testing.T) {
	p := Must(config.PermissionConfig{
		Profile: string(ProfileFull),
		Rules: []config.PermissionRule{{
			Tool:          "execute_command",
			Action:        string(DecisionAsk),
			CommandPrefix: []string{"git", "status"},
		}},
	})

	params, _ := json.Marshal(map[string]any{"command": "git status --short"})
	assertDecision(t, p, Request{ToolName: "execute_command", RequiresApproval: true, Params: params}, DecisionAsk)

	params, _ = json.Marshal(map[string]any{"command": "git status-rm"})
	assertDecision(t, p, Request{ToolName: "execute_command", RequiresApproval: true, Params: params}, DecisionAllow)

	for _, command := range []string{
		"git status && echo chained",
		"git status; echo chained",
		"git status | sh",
		"git status > /tmp/result",
		"git status $(malicious)",
		"git status\necho chained",
	} {
		params, _ = json.Marshal(map[string]any{"command": command})
		assertDecision(t, p, Request{ToolName: "execute_command", RequiresApproval: true, Params: params}, DecisionAllow)
	}
}

// A deny floor must hold against a command that merely wraps the denied
// invocation. Allow-direction prefix matching refuses to match anything but a
// simple command; reusing that refusal on a deny rule turned "cannot prove it
// matches" into "does not match", so every command below used to fall through
// to the auto profile and run.
func TestPolicy_DenyFloorHoldsAcrossCompoundCommands(t *testing.T) {
	p := denyPushPolicy()

	for _, command := range []string{
		"git push origin main",
		"git push origin main; :",
		"true && git push origin main",
		"echo start; git push origin main",
		"ls | git push origin main",
		"echo $(git push origin main)",
		"GIT_SSH_COMMAND=ssh git push origin main",
		"git push origin main\necho done",
	} {
		assertCommandDecision(t, p, command, DecisionDeny)
	}
}

// Indirection through an interpreter or a script cannot be resolved from the
// command string, so the policy escalates to an approval prompt instead of
// answering "not denied".
func TestPolicy_DenyFloorEscalatesUnprovableIndirection(t *testing.T) {
	p := denyPushPolicy()

	for _, command := range []string{
		"sh -c 'git push origin main'",
		"bash -lc \"git push origin main\"",
		"env FOO=bar sh -c 'git push'",
		"xargs git",
		"./deploy.sh",
		"scripts/release.sh --force",
	} {
		assertCommandDecision(t, p, command, DecisionAsk)
	}
}

// Escalation must not become noise: a compound command that provably has
// nothing to do with the deny rule still runs without a prompt.
func TestPolicy_DenyFloorLeavesUnrelatedCommandsAlone(t *testing.T) {
	p := denyPushPolicy()

	for _, command := range []string{
		"ls -la | wc -l",
		"echo one && echo two",
		"git status --short",
		"git pushed-branch-report",
		"cat notes.txt > /tmp/out",
	} {
		assertCommandDecision(t, p, command, DecisionAllow)
	}
}

// Escalation only ever tightens. A profile that already asks or denies is not
// loosened by an indeterminate deny floor.
func TestPolicy_DenyFloorEscalationNeverWeakens(t *testing.T) {
	ask := Must(config.PermissionConfig{
		Profile: string(ProfileAsk),
		Rules: []config.PermissionRule{{
			Tool:          "execute_command",
			Action:        string(DecisionDeny),
			CommandPrefix: []string{"git", "push"},
		}},
	})
	assertCommandDecision(t, ask, "sh -c 'ls'", DecisionAsk)

	tool := Must(config.PermissionConfig{
		Profile: string(ProfileAuto),
		Rules: []config.PermissionRule{{
			Tool:   "execute_command",
			Action: string(DecisionDeny),
			Reason: "shell disabled",
		}},
	})
	assertCommandDecision(t, tool, "sh -c 'anything'", DecisionDeny)
}

func denyPushPolicy() *Policy {
	return Must(config.PermissionConfig{
		Profile: string(ProfileAuto),
		Rules: []config.PermissionRule{{
			Tool:          "execute_command",
			Action:        string(DecisionDeny),
			CommandPrefix: []string{"git", "push"},
			Reason:        "no pushes",
		}},
	})
}

func assertCommandDecision(t *testing.T, p *Policy, command string, want Decision) {
	t.Helper()
	params, err := json.Marshal(map[string]any{"command": command})
	if err != nil {
		t.Fatalf("marshal command %q: %v", command, err)
	}
	got := p.Decide(Request{ToolName: "execute_command", RequiresApproval: true, Params: params})
	if got.Decision != want {
		t.Fatalf("Decide(%q) = %s (%s), want %s", command, got.Decision, got.Reason, want)
	}
}

func TestPolicy_SafeProfileIsNonReadOnlySafetyFloor(t *testing.T) {
	p := Must(config.PermissionConfig{
		Profile: string(ProfileSafe),
		Rules: []config.PermissionRule{
			{Tool: "execute_command", Action: string(DecisionAllow)},
			{Tool: "mcp:*", Action: string(DecisionAsk)},
		},
	}).WithRule("write_file", DecisionAllow)

	for _, tool := range []string{"execute_command", "write_file", "filesystem__read_file"} {
		assertDecision(t, p, Request{ToolName: tool, RequiresApproval: true}, DecisionDeny)
	}
}

func TestPolicy_ConfiguredDenyIsFloorForLaterSessionAllow(t *testing.T) {
	p := Must(config.PermissionConfig{
		Profile: string(ProfileFull),
		Rules: []config.PermissionRule{{
			Tool:   "execute_command",
			Action: string(DecisionDeny),
		}},
	}).WithRule("execute_command", DecisionAllow)

	assertDecision(t, p, Request{ToolName: "execute_command", RequiresApproval: true}, DecisionDeny)
}

func TestPolicy_EmptySessionCommandRulesAreNoOps(t *testing.T) {
	p := Must(config.PermissionConfig{Profile: string(ProfileSafe)})
	p = p.WithCommandRule("", DecisionAllow)
	p = p.WithCommandPrefixRule(nil, DecisionAllow)
	params, _ := json.Marshal(map[string]any{"command": "echo harmless"})
	assertDecision(t, p, Request{ToolName: "execute_command", RequiresApproval: true, Params: params}, DecisionDeny)
}

func TestPolicy_CustomProfileAndRules(t *testing.T) {
	p := Must(config.PermissionConfig{
		Profile: "balanced-plus",
		Profiles: map[string]config.PermissionProfile{
			"balanced-plus": {
				"default":         "ask",
				"read_file":       "allow",
				"search_codebase": "allow",
				"list_directory":  "allow",
				"mcp":             "ask",
			},
		},
		Rules: []config.PermissionRule{{
			Tool:    "execute_command",
			Action:  "deny",
			Command: "rm -rf *",
			Reason:  "destructive delete",
		}},
	})

	assertDecision(t, p, Request{ToolName: "read_file"}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "filesystem__read_file", RequiresApproval: true}, DecisionAsk)
	params, _ := json.Marshal(map[string]any{"command": "rm -rf *"})
	res := p.Decide(Request{ToolName: "execute_command", RequiresApproval: true, Params: params})
	if res.Decision != DecisionDeny || res.Reason != "destructive delete" {
		t.Fatalf("exact command deny = %+v", res)
	}
}

func TestPolicy_CustomProfileDefaultDenyIsTrueFallback(t *testing.T) {
	p := Must(config.PermissionConfig{
		Profile: "locked",
		Profiles: map[string]config.PermissionProfile{
			"locked": {
				"default":   "deny",
				"read_file": "allow",
			},
		},
	})

	assertDecision(t, p, Request{ToolName: "read_file"}, DecisionAllow)
	assertDecision(t, p, Request{ToolName: "search_codebase"}, DecisionDeny)
	assertDecision(t, p, Request{ToolName: "execute_command", RequiresApproval: true}, DecisionDeny)
}

func TestPolicy_MapBackedRulesAreDeterministicAndSpecificWins(t *testing.T) {
	cfg := config.PermissionConfig{
		Profile: "custom",
		Profiles: map[string]config.PermissionProfile{
			"custom": {
				"default":          "ask",
				"mcp":              "allow",
				"server__danger":   "deny",
				"filesystem__safe": "allow",
			},
		},
		Tools: map[string]string{
			"filesystem__*":      "allow",
			"filesystem__danger": "deny",
		},
	}
	for i := 0; i < 50; i++ {
		p := Must(cfg)
		assertDecision(t, p, Request{ToolName: "filesystem__read_file", RequiresApproval: true}, DecisionAllow)
		assertDecision(t, p, Request{ToolName: "filesystem__danger", RequiresApproval: true}, DecisionDeny)
		assertDecision(t, p, Request{ToolName: "server__read", RequiresApproval: true}, DecisionAllow)
		assertDecision(t, p, Request{ToolName: "server__danger", RequiresApproval: true}, DecisionDeny)
	}
}

func assertDecision(t *testing.T, p *Policy, req Request, want Decision) {
	t.Helper()
	if got := p.Decide(req); got.Decision != want {
		t.Fatalf("Decide(%+v) = %+v, want %s", req, got, want)
	}
}
