package jobs

import (
	"context"
	"fmt"

	"github.com/packetcode/packetcode/internal/agent"
)

// jobApprover is the per-job adapter around the main session's Approver.
//
// Approval policy (see docs/feature-background-agents.md):
//   - When AllowWrite=false, every approval-gated tool call is rejected
//     immediately with "background job is read-only". This is a defence in
//     depth alongside buildJobToolRegistry which doesn't even register
//     destructive tools in that mode.
//   - When AllowWrite=true, requests are forwarded to the parent approver
//     (typically the main session's uiApprover) with the tool name
//     prefixed by "[job:<id>]" so the user can tell where the prompt
//     originated.
type jobApprover struct {
	parent     agent.Approver
	jobID      string
	allowWrite bool
}

// NewJobApprover constructs the per-job Approver wrapper. parent is the
// main session's approver; jobID identifies the spawning job in
// annotated approval prompts.
func NewJobApprover(parent agent.Approver, jobID string, allowWrite bool) agent.Approver {
	return &jobApprover{parent: parent, jobID: jobID, allowWrite: allowWrite}
}

func (j *jobApprover) Approve(ctx context.Context, req agent.ApprovalRequest) agent.ApprovalDecision {
	if !j.allowWrite {
		return agent.ApprovalDecision{
			Approved: false,
			Reason:   "background job is read-only",
		}
	}
	if j.parent == nil {
		// No parent approver wired — be conservative.
		return agent.ApprovalDecision{
			Approved: false,
			Reason:   "no parent approver available",
		}
	}
	annotated := req
	annotated.ToolCall.Name = fmt.Sprintf("[job:%s] %s", j.jobID, req.ToolCall.Name)
	return j.parent.Approve(ctx, annotated)
}
