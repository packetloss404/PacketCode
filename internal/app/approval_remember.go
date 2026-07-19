package app

import (
	"encoding/json"
	"strings"

	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
)

// rememberApproval installs a session permission rule from the approval
// prompt's "always allow" choice. Shell programs are remembered exactly:
// inferring a prefix can authorize additional commands through shell syntax.
func (a *App) rememberApproval(call provider.ToolCall) {
	base := a.currentPermissionPolicy()
	if call.Name == "execute_command" {
		if command, ok := commandFromArgs(call.Arguments); ok {
			a.preTrustPolicy = nil
			a.setPermissionPolicy(base.WithCommandRule(command, permissions.DecisionAllow))
			a.conversation.AppendSystem("won't ask again for this exact command this session (/permissions to review)")
		}
		return
	}
	a.preTrustPolicy = nil
	a.setPermissionPolicy(base.WithRule(call.Name, permissions.DecisionAllow))
	a.conversation.AppendSystem("won't ask again for " + call.Name + " this session (/permissions to review)")
}

func commandFromArgs(args string) (string, bool) {
	var parsed struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &parsed); err != nil || strings.TrimSpace(parsed.Command) == "" {
		return "", false
	}
	return parsed.Command, true
}
