package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/skills"
	"github.com/packetcode/packetcode/internal/tools"
	"github.com/stretchr/testify/require"
)

func TestSkillGrantLifecyclePreservesSessionChanges(t *testing.T) {
	for _, pattern := range []string{"execute_command", "*"} {
		t.Run("ask "+pattern, func(t *testing.T) {
			r := grantRig(t, "execute_command")
			r.app.applySkillGrant(userSkill("deploy", "Bash(gh:*)"))
			require.Equal(t, permissions.DecisionAllow, decisionForCommand(r.app, "gh pr list"))
			r.app.handlePermissionsCommand([]string{"rule", pattern, "ask"})
			require.Equal(t, permissions.DecisionAsk, decisionForCommand(r.app, "gh pr list"))
			r.app.handlePermissionsCommand([]string{"profile", "edit"})
			require.Equal(t, permissions.DecisionAsk, decisionForCommand(r.app, "gh pr list"))
			r.app.releaseSkillGrant()
			require.Equal(t, permissions.DecisionAsk, decisionForCommand(r.app, "gh pr list"))
		})
	}
	t.Run("deny", func(t *testing.T) {
		r := grantRig(t, "execute_command")
		r.app.applySkillGrant(userSkill("deploy", "execute_command"))
		r.app.handlePermissionsCommand([]string{"rule", "execute_command", "deny"})
		r.app.releaseSkillGrant()
		require.Equal(t, permissions.DecisionDeny, decisionFor(r.app, "execute_command"))
	})
	t.Run("reset", func(t *testing.T) {
		r := grantRig(t, "execute_command", "write_file")
		r.app.handlePermissionsCommand([]string{"rule", "write_file", "allow"})
		r.app.applySkillGrant(userSkill("deploy", "execute_command"))
		r.app.handlePermissionsCommand([]string{"reset"})
		r.app.releaseSkillGrant()
		require.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "write_file"))
		require.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "execute_command"))
	})
	t.Run("remember", func(t *testing.T) {
		r := grantRig(t, "execute_command", "write_file")
		r.app.applySkillGrant(userSkill("deploy", "write_file"))
		r.app.rememberApproval(provider.ToolCall{Name: "execute_command", Arguments: `{"command":"git status"}`})
		r.app.releaseSkillGrant()
		require.Equal(t, permissions.DecisionAllow, decisionForCommand(r.app, "git status"))
		require.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "write_file"))
	})
	t.Run("trust snapshot", func(t *testing.T) {
		r := grantRig(t, "execute_command")
		r.app.applySkillGrant(userSkill("deploy", "execute_command"))
		r.app.handleTrustCommand([]string{"on"})
		r.app.releaseSkillGrant()
		r.app.handleTrustCommand([]string{"off"})
		require.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "execute_command"))
	})
	t.Run("explicit allow matches grant", func(t *testing.T) {
		r := grantRig(t, "execute_command")
		r.app.applySkillGrant(userSkill("deploy", "execute_command"))
		r.app.handlePermissionsCommand([]string{"rule", "execute_command", "allow"})
		r.app.releaseSkillGrant()
		require.Equal(t, permissions.DecisionAllow, decisionFor(r.app, "execute_command"))
	})
}

func TestSkillGrantLifecycleQueuedSkillDoesNotWidenCurrentTurn(t *testing.T) {
	r := lifecycleSkillRig(t)
	r.app.streaming = true
	r.app.handleSlashCommand("deploy", nil, "/deploy")
	require.Len(t, r.app.queuedInputs, 1)
	require.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "execute_command"))

	// Ending the unrelated turn starts the queued skill with its own grant.
	r.prov.turns = [][]provider.StreamEvent{{{Type: provider.EventDone}}}
	wireAgent(r, r.prov, &grantTool{name: "execute_command"})
	_, cmd := r.app.Update(agentDoneMsg{})
	require.Equal(t, permissions.DecisionAllow, decisionFor(r.app, "execute_command"))
	pump := newDrainPump(t, r.app, cmd)
	pump.RunUntil(2*time.Second, func() bool { return !r.app.streaming })
	require.False(t, r.app.streaming)
	require.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "execute_command"))
}

func TestSkillGrantLifecycleAutoCompactionDefersGrant(t *testing.T) {
	r := lifecycleSkillRig(t)
	for i := 0; i <= defaultCompactKeep; i++ {
		require.NoError(t, r.sessions.AddMessage(provider.Message{Role: provider.RoleUser, Content: "history"}))
	}
	require.NoError(t, r.sessions.SetContextTokens(90_000))
	r.app.handleSlashCommand("deploy", nil, "/deploy")
	defer r.app.cancelTurn()
	require.Len(t, r.app.queuedInputs, 1)
	require.NotNil(t, r.app.queuedInputs[0].SkillGrant)
	require.Nil(t, r.app.activeSkillGrant)
	require.Equal(t, permissions.DecisionAsk, decisionFor(r.app, "execute_command"))
	r.app.clearQueuedInputs()
	require.Nil(t, r.app.activeSkillGrant)
}

func lifecycleSkillRig(t *testing.T) *testAppRig {
	t.Helper()
	r := grantRig(t, "execute_command")
	home := t.TempDir()
	t.Setenv("PACKETCODE_HOME", home)
	writeInvocableSkill(t, filepath.Join(home, "skills"), "deploy", "---\ndescription: Deploy\nallowed-tools: execute_command\n---\nDeploy it.\n")
	r.app.deps.Skills = skills.Load(r.app.deps.WorkingDir)
	r.app.slashCommands = LoadSlashRegistry(r.app.deps.WorkingDir, r.app.deps.Skills)
	return r
}

func TestSkillGrantLifecycleModelCallbackIsScopedAndAcknowledged(t *testing.T) {
	r := lifecycleSkillRig(t)
	tool := tools.NewSkillTool(r.app.deps.Skills)
	r.app.deps.Tools.Register(tool)
	messages := make(chan tea.Msg, 1)
	r.app.sendMsg = func(msg tea.Msg) { messages <- msg }
	r.app.wireSkillGrants()
	r.app.streaming = true
	r.app.skillTurnID = 10
	args := json.RawMessage(`{"name":"deploy"}`)

	// Background jobs share the tool instance but not the foreground context.
	_, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)
	require.Empty(t, messages)
	require.Nil(t, r.app.activeSkillGrant)

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), skillTurnKey{}, uint64(10)))
	defer cancel()
	done := make(chan struct{})
	go func() { _, _ = tool.Execute(ctx, args); close(done) }()
	var msg tea.Msg
	select {
	case msg = <-messages:
	case <-time.After(2 * time.Second):
		t.Fatal("skill callback never reached the app")
	}
	select {
	case <-done:
		t.Fatal("skill returned before its permission change was applied")
	default:
	}
	r.app.Update(msg)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("skill callback did not receive acknowledgment")
	}
	require.Equal(t, permissions.DecisionAllow, decisionFor(r.app, "execute_command"))
	r.app.releaseSkillGrant()

	// A queued callback becomes inert when cancelled, even before agentDone.
	done = make(chan struct{})
	go func() { _, _ = tool.Execute(ctx, args); close(done) }()
	select {
	case msg = <-messages:
	case <-time.After(2 * time.Second):
		t.Fatal("second callback never reached the app")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not release the acknowledgment wait")
	}
	r.app.Update(msg)
	require.Nil(t, r.app.activeSkillGrant)

	// A stale callback cannot affect a new turn or an idle app.
	r.app.Update(skillLoadedMsg{skill: userSkill("deploy", "execute_command"), turnID: 9})
	r.app.streaming = false
	r.app.Update(skillLoadedMsg{skill: userSkill("deploy", "execute_command"), turnID: 10})
	require.Nil(t, r.app.activeSkillGrant)
}
