package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/testwait"
	"github.com/packetcode/packetcode/internal/tools"
)

type approverTestTool struct{}

func (approverTestTool) Name() string            { return "test_tool" }
func (approverTestTool) Description() string     { return "test" }
func (approverTestTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (approverTestTool) RequiresApproval() bool  { return true }
func (approverTestTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{}, nil
}

func TestUIApproverRoutesDecisionToVisibleRequest(t *testing.T) {
	u := newUIApprover()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dec1 := make(chan agent.ApprovalDecision, 1)
	go func() {
		dec1 <- u.Approve(ctx, approvalReq("first"))
	}()

	first := waitPendingApproval(t, u)
	if first.req.ToolCall.ID != "first" {
		t.Fatalf("first pending id = %q, want first", first.req.ToolCall.ID)
	}

	dec2 := make(chan agent.ApprovalDecision, 1)
	go func() {
		dec2 <- u.Approve(ctx, approvalReq("second"))
	}()

	if _, ok := u.Next(); ok {
		t.Fatalf("second request surfaced while first approval was active")
	}
	if !u.ResolveID(first.id, agent.ApprovalDecision{Approved: true, Reason: "first decision"}) {
		t.Fatal("resolving the displayed envelope reported no delivery")
	}
	got1 := waitDecision(t, dec1)
	if !got1.Approved || got1.Reason != "first decision" {
		t.Fatalf("first decision = %+v", got1)
	}

	second := waitPendingApproval(t, u)
	if second.req.ToolCall.ID != "second" {
		t.Fatalf("second pending id = %q, want second", second.req.ToolCall.ID)
	}
	// The first envelope's id must be inert now: a late decision carrying it
	// is exactly the "answer landed on the wrong request" failure.
	if u.ResolveID(first.id, agent.ApprovalDecision{Approved: true, Reason: "stale"}) {
		t.Fatal("a stale envelope id resolved the request that replaced it")
	}
	if !u.ResolveID(second.id, agent.ApprovalDecision{Approved: false, Reason: "second decision"}) {
		t.Fatal("resolving the second envelope reported no delivery")
	}
	got2 := waitDecision(t, dec2)
	if got2.Approved || got2.Reason != "second decision" {
		t.Fatalf("second decision = %+v", got2)
	}
	select {
	case dec := <-dec1:
		t.Fatalf("first caller received a second decision %+v", dec)
	default:
	}
}

func TestUIApproverNotifiesInsteadOfRequiringIdlePolling(t *testing.T) {
	u := newUIApprover()
	notified := make(chan struct{}, 1)
	u.SetNotify(func() { notified <- struct{}{} })
	decisions := make(chan agent.ApprovalDecision, 1)

	go func() {
		decisions <- u.Approve(context.Background(), approvalReq("event-driven"))
	}()

	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("approval did not notify the UI")
	}
	next := waitPendingApproval(t, u)
	if next.req.ToolCall.ID != "event-driven" {
		t.Fatalf("pending id = %q, want event-driven", next.req.ToolCall.ID)
	}
	u.ResolveID(next.id, agent.ApprovalDecision{Approved: true})
	if got := waitDecision(t, decisions); !got.Approved {
		t.Fatalf("decision = %+v, want approved", got)
	}
}

func TestUIApproverPermissionPolicyAllowAndDeny(t *testing.T) {
	u := newUIApprover()
	u.SetPermissionPolicy(permissions.DefaultPolicy().WithRule("test_tool", permissions.ActionDeny))
	denied := u.Approve(context.Background(), approvalReq("deny"))
	if denied.Approved || denied.Reason == "" {
		t.Fatalf("denied decision = %+v", denied)
	}
	if _, ok := u.Next(); ok {
		t.Fatalf("denied policy request should not reach approval queue")
	}

	u.SetPermissionPolicy(permissions.DefaultPolicy().WithRule("test_tool", permissions.ActionAllow))
	allowed := u.Approve(context.Background(), approvalReq("allow"))
	if !allowed.Approved || string(allowed.EditedParams) != `{}` {
		t.Fatalf("allowed decision = %+v", allowed)
	}
	if _, ok := u.Next(); ok {
		t.Fatalf("allowed policy request should not reach approval queue")
	}
}

func TestUIApproverExplicitPromptIgnoresLivePolicyAndAutoTrust(t *testing.T) {
	u := newUIApprover()
	u.SetPermissionPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileFull))
	u.SetTrust(true)
	decisions := make(chan agent.ApprovalDecision, 1)

	go func() {
		decisions <- u.PromptApproval(context.Background(), approvalReq("snapshot-ask"))
	}()

	next := waitPendingApproval(t, u)
	if next.req.ToolCall.ID != "snapshot-ask" {
		t.Fatalf("pending id = %q, want snapshot-ask", next.req.ToolCall.ID)
	}
	if next.origin != originBackground {
		t.Fatalf("PromptApproval origin = %v, want background", next.origin)
	}
	if u.ResolveActiveByPolicy(next.id) {
		t.Fatal("live policy must not resolve an explicit snapshot-bound prompt")
	}
	u.ResolveID(next.id, agent.ApprovalDecision{Approved: false, Reason: "explicit rejection"})
	if got := waitDecision(t, decisions); got.Approved || got.Reason != "explicit rejection" {
		t.Fatalf("decision = %+v, want explicit rejection", got)
	}
}

func TestUIApproverExplicitPromptHonorsLiveDeny(t *testing.T) {
	u := newUIApprover()
	notified := make(chan struct{}, 1)
	u.SetNotify(func() { notified <- struct{}{} })
	decisions := make(chan agent.ApprovalDecision, 1)
	go func() {
		decisions <- u.PromptApproval(context.Background(), approvalReq("snapshot-revoked"))
	}()
	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("approval did not reach the queue")
	}

	u.SetPermissionPolicy(permissions.DefaultPolicy().WithRule("test_tool", permissions.ActionDeny))
	if _, ok := u.Next(); ok {
		t.Fatal("live deny should resolve the snapshot-bound prompt without displaying it")
	}
	if got := waitDecision(t, decisions); got.Approved || got.Reason == "" {
		t.Fatalf("decision = %+v, want policy denial", got)
	}
}

func approvalReq(id string) agent.ApprovalRequest {
	return agent.ApprovalRequest{
		Tool: approverTestTool{},
		ToolCall: provider.ToolCall{
			ID:        id,
			Name:      "test_tool",
			Arguments: `{}`,
		},
		Params: json.RawMessage(`{}`),
	}
}

func waitPendingApproval(t *testing.T, u *uiApprover) pendingApproval {
	t.Helper()
	var out pendingApproval
	testwait.For(t, time.Second, "approval reached the queue", func() bool {
		next, ok := u.Next()
		if ok {
			out = next
		}
		return ok
	})
	return out
}

func waitDecision(t *testing.T, ch <-chan agent.ApprovalDecision) agent.ApprovalDecision {
	t.Helper()
	select {
	case dec := <-ch:
		return dec
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for approval decision")
		return agent.ApprovalDecision{}
	}
}
