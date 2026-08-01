package jobs

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/provider"
)

// Test 13 — read-only mode rejects every approval request, no matter
// what the parent approver would say.
func TestJobApprover_ReadOnlyRejects(t *testing.T) {
	parent := agent.AutoApprove() // would say yes
	app := NewJobApprover(parent, "abc12345", false)

	dec := app.Approve(context.Background(), agent.ApprovalRequest{
		ToolCall: provider.ToolCall{Name: "write_file"},
	})
	assert.False(t, dec.Approved)
	assert.Contains(t, strings.ToLower(dec.Reason), "read-only")
}

func TestJobApprover_ReadOnlyAllowsSpawnAgent(t *testing.T) {
	parent := &fakeApprover{decision: agent.ApprovalDecision{Approved: false, Reason: "should not be called"}}
	app := NewJobApprover(parent, "abc12345", false)

	dec := app.Approve(context.Background(), agent.ApprovalRequest{
		ToolCall: provider.ToolCall{Name: "spawn_agent"},
		Params:   []byte(`{"prompt":"scout"}`),
	})
	assert.True(t, dec.Approved)
	assert.JSONEq(t, `{"prompt":"scout"}`, string(dec.EditedParams))
	assert.Empty(t, parent.snapshotCalls(), "read-only spawn_agent should not route through parent destructive approver")
}

// Test 14 — allowWrite mode forwards to the parent with the tool name
// prefixed by "[job:<id>]".
func TestJobApprover_AnnotatesJobID(t *testing.T) {
	parent := &fakeApprover{decision: agent.ApprovalDecision{Approved: true}}
	app := NewJobApprover(parent, "abc12345", true)

	dec := app.Approve(context.Background(), agent.ApprovalRequest{
		ToolCall: provider.ToolCall{Name: "write_file", ID: "c1"},
	})
	assert.True(t, dec.Approved)

	calls := parent.snapshotCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "[job:abc12345] write_file", calls[0].ToolCall.Name)
	assert.Equal(t, "c1", calls[0].ToolCall.ID, "non-name fields must be untouched")
}

func TestJobApprover_UsesExplicitPromptWhenParentSupportsIt(t *testing.T) {
	parent := &fakePromptApprover{decision: agent.ApprovalDecision{Approved: true}}
	app := NewJobApprover(parent, "abc12345", true)

	dec := app.Approve(context.Background(), agent.ApprovalRequest{
		ToolCall: provider.ToolCall{Name: "execute_command", ID: "c2"},
	})
	assert.True(t, dec.Approved)
	require.Len(t, parent.promptCalls, 1)
	assert.Empty(t, parent.approveCalls)
	assert.Equal(t, "[job:abc12345] execute_command", parent.promptCalls[0].ToolCall.Name)
}

type fakePromptApprover struct {
	approveCalls []agent.ApprovalRequest
	promptCalls  []agent.ApprovalRequest
	decision     agent.ApprovalDecision
}

func (f *fakePromptApprover) Approve(_ context.Context, req agent.ApprovalRequest) agent.ApprovalDecision {
	f.approveCalls = append(f.approveCalls, req)
	return f.decision
}

func (f *fakePromptApprover) PromptApproval(_ context.Context, req agent.ApprovalRequest) agent.ApprovalDecision {
	f.promptCalls = append(f.promptCalls, req)
	return f.decision
}

// TestJobApprover_NoParentRejects verifies the defensive nil-parent
// branch — if no parent approver is wired, the adapter rejects rather
// than panicking.
func TestJobApprover_NoParentRejects(t *testing.T) {
	app := NewJobApprover(nil, "id", true)
	dec := app.Approve(context.Background(), agent.ApprovalRequest{
		ToolCall: provider.ToolCall{Name: "x"},
	})
	assert.False(t, dec.Approved)
}
