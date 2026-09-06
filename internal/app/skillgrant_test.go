package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/skills"
	"github.com/packetcode/packetcode/internal/tools"
)

// grantTool is a registered tool a grant can name.
type grantTool struct{ name string }

func (g *grantTool) Name() string            { return g.name }
func (g *grantTool) Description() string     { return "test tool" }
func (g *grantTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (g *grantTool) RequiresApproval() bool  { return true }
func (g *grantTool) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{}, nil
}

func grantRig(t *testing.T, toolNames ...string) *testAppRig {
	t.Helper()
	r := newTestApp(t)
	reg := tools.NewRegistry()
	for _, n := range toolNames {
		reg.Register(&grantTool{name: n})
	}
	r.app.deps.Tools = reg
	return r
}

// userSkill builds a trusted skill whose allowed-tools list is the one a
// SKILL.md would carry, parsed the same way -- so a test naming Bash(gh:*)
// exercises the real translation rather than a hand-built grant.
func userSkill(name string, allowed ...string) skills.Skill {
	_, fm := skills.ParseFrontmatterFields(
		"---" + "\n" + "description: d" + "\n" + "allowed-tools: " +
			strings.Join(allowed, ", ") + "\n" + "---" + "\n" + "body" + "\n")
	return skills.Skill{Name: name, Source: skills.SourceUser, Invocation: fm}
}

func decisionFor(a *App, tool string) permissions.Decision {
	return a.currentPermissionPolicy().Decide(permissions.Request{
		ToolName: tool, RequiresApproval: true,
	}).Decision
}

// The grant converts Ask to Allow for the named tool, for this turn.
func TestSkillGrant_WidensForTheTurn(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))
	require.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "execute_command"))

	note := r.app.applySkillGrant(userSkill("deploy", "execute_command"))
	assert.Contains(t, note, "execute_command")
	assert.Equal(t, permissions.DecisionAllow, decisionFor(r.app, "execute_command"),
		"the grant did not take effect")
}

// An explicit deny is a floor. A skill must not be able to lift one, or
// `allowed-tools` becomes a way to undo a decision the user made deliberately.
func TestSkillGrant_CannotOverrideAnExplicitDeny(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(
		permissions.DefaultPolicy().WithRule("execute_command", permissions.DecisionDeny))
	require.Equal(t, permissions.DecisionDeny, decisionFor(r.app, "execute_command"))

	r.app.applySkillGrant(userSkill("deploy", "execute_command"))

	assert.Equal(t, permissions.DecisionDeny, decisionFor(r.app, "execute_command"),
		"a skill lifted an explicit deny")
}

// A widening that outlived its turn would be indistinguishable from a
// permanent policy change the user never made.
func TestSkillGrant_ReleasedWhenTheTurnEnds(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))

	r.app.applySkillGrant(userSkill("deploy", "execute_command"))
	require.Equal(t, permissions.DecisionAllow, decisionFor(r.app, "execute_command"))

	note := r.app.releaseSkillGrant()
	assert.Contains(t, note, "ended with the turn")
	assert.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "execute_command"),
		"the grant outlived its turn")
	// Releasing twice is not an error and does not disturb the policy.
	assert.Empty(t, r.app.releaseSkillGrant())
	assert.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "execute_command"))
}

// A project skill's request is refused at the skills layer; the App reports it
// rather than quietly granting nothing.
func TestSkillGrant_ProjectSkillIsRefusedAndReported(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))

	note := r.app.applySkillGrant(skills.Skill{
		Name:       "deploy",
		Source:     skills.SourceProject,
		Invocation: skills.Frontmatter{AllowedTools: []skills.ToolGrant{{Tool: "execute_command"}}},
	})

	assert.Contains(t, note, "not honoured for a project skill")
	assert.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "execute_command"),
		"a project skill widened the policy")
	assert.Nil(t, r.app.activeSkillGrant)
}

// Claude Code's tool names are not packetcode's, so a skill written for that
// ecosystem lands here naming tools that do not exist. Granting the
// closest-looking one would be guessing about exactly the thing that must not
// be guessed.
func TestSkillGrant_UnknownToolGrantsNothingAndSaysSo(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))

	note := r.app.applySkillGrant(userSkill("ported", "Bash"))

	assert.Contains(t, note, "Bash")
	assert.Contains(t, note, "not a packetcode tool")
	assert.Nil(t, r.app.activeSkillGrant, "an unknown name still created a grant")
	assert.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "execute_command"),
		"an unknown name widened some other tool")
}

// A mix grants the real one and reports the rest.
func TestSkillGrant_MixedNames(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))

	note := r.app.applySkillGrant(userSkill("ported", "Bash", "execute_command"))

	assert.Equal(t, permissions.DecisionAllow, decisionFor(r.app, "execute_command"))
	assert.Contains(t, note, "Bash")
	assert.True(t, strings.Contains(note, "pre-approved"), "the honoured half was not reported: %q", note)
}

// The teardown restores what was in force, not a recomputed default: the user
// may have changed the profile during the same turn.
func TestSkillGrant_ReleaseRestoresTheCapturedPolicy(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(
		permissions.DefaultPolicy().WithRule("read_file", permissions.DecisionDeny))

	r.app.applySkillGrant(userSkill("deploy", "execute_command"))
	r.app.releaseSkillGrant()

	assert.Equal(t, permissions.DecisionDeny, decisionFor(r.app, "read_file"),
		"an unrelated rule was lost when the grant was released")
}

// decisionForCommand asks the policy about a shell command, which is what a
// scoped grant is actually about.
func decisionForCommand(a *App, command string) permissions.Decision {
	params, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		panic(err)
	}
	return a.currentPermissionPolicy().Decide(permissions.Request{
		ToolName: "execute_command", RequiresApproval: true, Params: params,
	}).Decision
}

// The case the whole feature exists for: a skill published for the ecosystem
// says it runs gh, and packetcode pre-approves gh -- not the shell.
func TestSkillGrant_CommandPrefixWidensOnlyThatCommand(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))
	require.Equal(t, permissions.DecisionAsk, decisionForCommand(r.app, "gh pr list"))

	note := r.app.applySkillGrant(userSkill("companion-clis", "Bash(gh:*)"))

	assert.Contains(t, note, "pre-approved")
	assert.Equal(t, permissions.DecisionAllow, decisionForCommand(r.app, "gh pr list"),
		"the command the skill named was not pre-approved")
	assert.Equal(t, permissions.DecisionAsk, decisionForCommand(r.app, "rm -rf ."),
		"a scoped grant widened the whole shell")
	assert.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "execute_command"),
		"a scoped grant became a bare tool grant")
}

// A prefix rule describes one simple command. It must never authorize a larger
// program that merely starts with the right word.
func TestSkillGrant_CommandPrefixDoesNotAuthorizeACompoundProgram(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))

	r.app.applySkillGrant(userSkill("companion-clis", "Bash(gh:*)"))

	for _, command := range []string{"gh pr list && rm -rf .", "gh pr list; rm -rf .", "gh $(rm -rf .)"} {
		assert.Equal(t, permissions.DecisionAsk, decisionForCommand(r.app, command),
			"a prefix grant authorized %q", command)
	}
}

// The exact form matches the program byte-for-byte and nothing near it.
func TestSkillGrant_ExactCommandGrant(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))

	r.app.applySkillGrant(userSkill("tidy", "execute_command(git status)"))

	assert.Equal(t, permissions.DecisionAllow, decisionForCommand(r.app, "git status"))
	assert.Equal(t, permissions.DecisionAsk, decisionForCommand(r.app, "git status --short"))
}

// An explicit deny is a floor for a scoped grant too, and a deny the policy
// cannot evaluate escalates rather than falling through.
func TestSkillGrant_ScopedGrantCannotLiftADeny(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(permissions.DefaultPolicy().
		WithCommandPrefixRule([]string{"gh", "pr", "merge"}, permissions.DecisionDeny))

	r.app.applySkillGrant(userSkill("companion-clis", "Bash(gh:*)"))

	assert.Equal(t, permissions.DecisionDeny, decisionForCommand(r.app, "gh pr merge 12"),
		"a skill lifted an explicit deny")
	assert.Equal(t, permissions.DecisionAllow, decisionForCommand(r.app, "gh pr list"),
		"the deny floor swallowed the rest of the grant")
}

// The grant is torn down with the turn, scope and all.
func TestSkillGrant_ScopedGrantReleasedWhenTheTurnEnds(t *testing.T) {
	r := grantRig(t, "execute_command")
	r.app.setPermissionPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))

	r.app.applySkillGrant(userSkill("companion-clis", "Bash(gh:*)"))
	require.Equal(t, permissions.DecisionAllow, decisionForCommand(r.app, "gh pr list"))

	r.app.releaseSkillGrant()

	assert.Equal(t, permissions.DecisionAsk, decisionForCommand(r.app, "gh pr list"),
		"a scoped grant outlived its turn")
}

// A scope this file cannot turn into a rule grants nothing rather than falling
// back to the bare tool. Not reachable from a file on disk -- the parser
// refuses such a scope first -- so it is built by hand.
func TestSkillGrant_InexpressibleScopeGrantsNothing(t *testing.T) {
	r := grantRig(t, "write_file")
	r.app.setPermissionPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))

	note := r.app.applySkillGrant(skills.Skill{
		Name:   "ported",
		Source: skills.SourceUser,
		Invocation: skills.Frontmatter{AllowedTools: []skills.ToolGrant{
			{Tool: "write_file", CommandPrefix: []string{"src"}},
		}},
	})

	assert.Contains(t, note, "nothing was granted")
	assert.Nil(t, r.app.activeSkillGrant)
	assert.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "write_file"),
		"a scope that could not be expressed became a grant of the whole tool")
}
