package app

import (
	"context"
	"sync"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/tools"
)

// uiApprover is the bridge between the agent's blocking Approver call
// and the TUI's event-loop-driven approval prompt.
//
// Mechanism: Approve() pushes the request onto pendingCh and blocks on
// resultCh. The App's tea.Update() reads pendingCh, raises the approval
// modal, waits for the user to hit y/n, and posts the decision back via
// resultCh. Both sides are size-1 channels — the agent loop is serial,
// so there's never more than one approval in flight.
type uiApprover struct {
	pendingCh chan agent.ApprovalRequest
	resultCh  chan agent.ApprovalDecision

	mu        sync.Mutex
	autoTrust bool // when true, every Approve returns Approved without prompting
}

func newUIApprover() *uiApprover {
	return &uiApprover{
		pendingCh: make(chan agent.ApprovalRequest, 1),
		resultCh:  make(chan agent.ApprovalDecision, 1),
	}
}

func (u *uiApprover) Approve(ctx context.Context, req agent.ApprovalRequest) agent.ApprovalDecision {
	u.mu.Lock()
	trusted := u.autoTrust
	u.mu.Unlock()
	if trusted {
		return agent.ApprovalDecision{Approved: true, EditedParams: req.Params}
	}

	select {
	case u.pendingCh <- req:
	case <-ctx.Done():
		return agent.ApprovalDecision{Approved: false, Reason: "cancelled"}
	}
	select {
	case dec := <-u.resultCh:
		return dec
	case <-ctx.Done():
		return agent.ApprovalDecision{Approved: false, Reason: "cancelled"}
	}
}

// Pending returns the next pending request without blocking. Returns
// (zero, false) if the queue is empty. The App polls this from its
// Update loop.
func (u *uiApprover) Pending() (agent.ApprovalRequest, bool) {
	select {
	case r := <-u.pendingCh:
		return r, true
	default:
		return agent.ApprovalRequest{}, false
	}
}

// Resolve posts the user's decision back to the waiting Approve() call.
func (u *uiApprover) Resolve(decision agent.ApprovalDecision) {
	select {
	case u.resultCh <- decision:
	default:
	}
}

// SetTrust toggles trust mode. When enabled, future Approve() calls
// return immediately without raising the modal.
func (u *uiApprover) SetTrust(trust bool) {
	u.mu.Lock()
	u.autoTrust = trust
	u.mu.Unlock()
}

// IsTrusted reports trust mode.
func (u *uiApprover) IsTrusted() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.autoTrust
}

// describeRequest is a small helper for the conversation log.
func describeRequest(t tools.Tool, c provider.ToolCall) string {
	return t.Name() + ": " + c.Arguments
}
