package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/permissions"
)

func (a *App) handlePermissionsCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		a.conversation.AppendSystem(renderPermissionPolicy(a.currentPermissionPolicy(), a.approver.IsTrusted()))
		return a, nil
	}
	switch args[0] {
	case "profiles":
		a.conversation.AppendSystem(renderPermissionProfiles())
		return a, nil
	case "reset":
		if len(args) != 1 {
			a.conversation.AppendSystem("permissions: reset does not take arguments")
			return a, nil
		}
		policy := a.permissionBase
		if policy == nil {
			policy = permissions.DefaultPolicy()
		}
		a.planMode = false
		a.preTrustPolicy = nil
		a.activeSkillGrant = nil
		if a.approver != nil {
			a.approver.SetTrust(false)
		}
		a.setSessionPermissionPolicy(policy)
		a.conversation.AppendSystem("permission policy reset to startup configuration; session rules revoked")
		return a, nil
	case "profile", "use":
		if len(args) != 2 {
			a.conversation.AppendSystem("permissions: profile requires one value")
			return a, nil
		}
		profile, err := permissions.ParseProfile(args[1])
		if err != nil {
			a.conversation.AppendSystem("permissions: " + err.Error())
			return a, nil
		}
		base := a.sessionPermissionPolicy()
		if profile == permissions.ProfileFull {
			restore := base
			if a.planMode {
				restore = restore.WithProfile(a.planPrevProfile)
			}
			if restore.Profile() != permissions.ProfileFull {
				a.preTrustPolicy = restore
			}
			base = restore
		} else {
			a.preTrustPolicy = nil
		}
		a.planMode = false
		policy := base.WithProfile(profile)
		a.setSessionPermissionPolicy(policy)
		a.conversation.AppendSystem("permission profile: " + permissions.ProfileConfigName(profile) + " (session)")
		return a, nil
	case "explain":
		if len(args) != 2 {
			a.conversation.AppendSystem("permissions: explain requires one tool name")
			return a, nil
		}
		a.conversation.AppendSystem(a.explainPermission(args[1]))
		return a, nil
	case "rule":
		if len(args) != 3 {
			a.conversation.AppendSystem("permissions: rule requires <tool|pattern> <ask|allow|deny>")
			return a, nil
		}
		action, err := permissions.ParseAction(args[2])
		if err != nil {
			a.conversation.AppendSystem("permissions: " + err.Error())
			return a, nil
		}
		base := a.sessionPermissionPolicy()
		policy := base.WithRule(args[1], action)
		if action == permissions.DecisionAsk {
			a.revokeSkillGrants(args[1])
		}
		if base.Profile() == permissions.ProfileFull {
			a.preTrustPolicy = a.trustOffPolicy().WithRule(args[1], action)
		} else {
			a.preTrustPolicy = nil
		}
		a.setSessionPermissionPolicy(policy)
		a.conversation.AppendSystem(fmt.Sprintf("permission rule: %s = %s (session)", args[1], action))
		return a, nil
	default:
		a.conversation.AppendSystem(fmt.Sprintf("permissions: unknown subcommand %q (want profiles, profile, use, explain, rule, or reset)", args[0]))
		return a, nil
	}
}

func (a *App) currentPermissionPolicy() *permissions.Policy {
	if a.permissionPolicy != nil {
		return a.permissionPolicy
	}
	if a.approver != nil && a.approver.PermissionPolicy() != nil {
		return a.approver.PermissionPolicy()
	}
	return permissions.DefaultPolicy()
}

func (a *App) setPermissionPolicy(policy *permissions.Policy) {
	if policy == nil {
		policy = permissions.DefaultPolicy()
	}
	a.permissionPolicy = policy
	if a.approver != nil {
		a.approver.SetPermissionPolicy(policy)
	}
	if a.agent != nil {
		a.agent.SetPolicy(policy)
	}
	if a.jobs != nil {
		// A background job can outlive this turn. Its initial policy must not
		// capture a foreground skill's temporary pre-approval.
		a.jobs.SetPermissionPolicy(a.sessionPermissionPolicy())
	}
	a.refreshTopBar()
}

func renderPermissionPolicy(policy *permissions.Policy, trust bool) string {
	lines := []string{"Permission policy"}
	if policy == nil {
		policy = permissions.DefaultPolicy()
	}
	lines = append(lines, policy.SummaryLines()...)
	trustState := "off"
	if trust {
		trustState = "on"
	}
	lines = append(lines, "trust_mode: "+trustState)
	return strings.Join(lines, "\n")
}

func renderPermissionProfiles() string {
	lines := []string{"Permission profiles"}
	for _, profile := range permissions.Profiles() {
		lines = append(lines, fmt.Sprintf("%s: %s", permissions.ProfileConfigName(profile), permissions.ProfileSummary(profile)))
	}
	return strings.Join(lines, "\n")
}

func (a *App) explainPermission(toolName string) string {
	requiresApproval := true
	if a.deps.Tools != nil {
		if tool, ok := a.deps.Tools.Get(toolName); ok {
			requiresApproval = tool.RequiresApproval()
		}
	}
	result := a.currentPermissionPolicy().Decide(permissions.Request{
		ToolName:         toolName,
		RequiresApproval: requiresApproval,
	})
	return fmt.Sprintf("permission explain: %s -> %s (%s)", toolName, result.Decision, result.Reason)
}
