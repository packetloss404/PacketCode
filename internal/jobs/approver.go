package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/packetcode/packetcode/internal/agent"
)

// jobApprover is the per-job adapter around the main session's Approver.
//
// Approval policy (see docs/feature-background-agents.md):
//   - When AllowWrite=false, approval-gated destructive tool calls are
//     rejected immediately with "background job is read-only". spawn_agent
//     is allowed through so read-only jobs can delegate read-only children
//     until the depth cap is reached.
//   - When AllowWrite=true, requests are forwarded to the parent approver
//     (typically the main session's uiApprover) with the tool name
//     prefixed by "[job:<id>]" so the user can tell where the prompt
//     originated. The prefix is a label; the job id is also handed to the
//     parent as data via jobApprovalPrompter, because the parent uses origin
//     to decide what a foreground Ctrl+C is allowed to reject.
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
	if req.ToolCall.Name == "spawn_agent" && !j.allowWrite {
		return agent.ApprovalDecision{
			Approved:     true,
			EditedParams: cloneParams(req.Params),
		}
	}
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
	// Preferred: the parent records which job asked. Recovering that by
	// parsing the "[job:<id>] " display prefix back out of the tool name is
	// fragile — the prefix exists to be read by a human, and a request whose
	// origin is misread is one a foreground Ctrl+C will cancel by mistake.
	if prompter, ok := j.parent.(jobApprovalPrompter); ok {
		return prompter.PromptJobApproval(ctx, j.jobID, annotated)
	}
	if prompter, ok := j.parent.(agent.ApprovalPrompter); ok {
		return prompter.PromptApproval(ctx, annotated)
	}
	return j.parent.Approve(ctx, annotated)
}

// jobApprovalPrompter is implemented by parent approvers that can record the
// originating job alongside the request. Declared here rather than in
// internal/agent because it is this package's requirement of its parent, and
// Go interface satisfaction is structural: the main session's approver
// implements it without either package importing the other.
type jobApprovalPrompter interface {
	PromptJobApproval(ctx context.Context, jobID string, req agent.ApprovalRequest) agent.ApprovalDecision
}

func cloneParams(params json.RawMessage) json.RawMessage {
	if len(params) == 0 {
		return nil
	}
	out := make(json.RawMessage, len(params))
	copy(out, params)
	return out
}
