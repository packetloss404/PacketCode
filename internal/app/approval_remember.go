package app

import (
	"encoding/json"
	"strings"

	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
)

// rememberApproval installs a session permission rule from the approval
// prompt's "always allow" choice, so the same action isn't asked again this
// session. For execute_command it scopes to the command family (first one or
// two tokens) rather than allowing every command; for other tools it allows the
// tool by name.
func (a *App) rememberApproval(call provider.ToolCall) {
	base := a.currentPermissionPolicy()
	if call.Name == "execute_command" {
		if prefix := commandPrefixFromArgs(call.Arguments); len(prefix) > 0 {
			a.preTrustPolicy = nil
			a.setPermissionPolicy(base.WithCommandPrefixRule(prefix, permissions.DecisionAllow))
			a.conversation.AppendSystem("won't ask again for `" + strings.Join(prefix, " ") + " …` commands this session (/permissions to review)")
			return
		}
	}
	a.preTrustPolicy = nil
	a.setPermissionPolicy(base.WithRule(call.Name, permissions.DecisionAllow))
	a.conversation.AppendSystem("won't ask again for " + call.Name + " this session (/permissions to review)")
}

// commandPrefixFromArgs extracts a command-family prefix from an
// execute_command tool call's arguments. It returns the first token, plus the
// second when the first is a common multiplexer (go, git, npm, cargo, …) whose
// subcommand meaningfully narrows the scope (e.g. ["go","test"]).
func commandPrefixFromArgs(args string) []string {
	var parsed struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return nil
	}
	fields := strings.Fields(parsed.Command)
	if len(fields) == 0 {
		return nil
	}
	if len(fields) >= 2 && multiplexerCommands[fields[0]] {
		return fields[:2]
	}
	return fields[:1]
}

// multiplexerCommands are tools whose first argument is a subcommand worth
// including in the allow-rule prefix so "always" scopes to e.g. `go test`
// rather than all of `go`.
var multiplexerCommands = map[string]bool{
	"go": true, "git": true, "npm": true, "pnpm": true, "yarn": true,
	"cargo": true, "make": true, "docker": true, "kubectl": true,
	"python": true, "python3": true, "pip": true, "pip3": true, "uv": true,
	"node": true, "bun": true, "gh": true, "brew": true,
}
