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

func userSkill(name string, allowed ...string) skills.Skill {
	return skills.Skill{
		Name:       name,
		Source:     skills.SourceUser,
		Invocation: skills.Frontmatter{AllowedTools: allowed},
	}
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
		Invocation: skills.Frontmatter{AllowedTools: []string{"execute_command"}},
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
