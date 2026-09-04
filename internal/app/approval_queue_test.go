package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/jobs"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/testwait"
	"github.com/packetcode/packetcode/internal/tools"
	"github.com/packetcode/packetcode/internal/ui/components/approval"
)

// showApprovalWhenPending drives the same reconciliation the Update loop runs,
// so tests exercise the id binding rather than poking approval.Show directly.
func showApprovalWhenPending(t *testing.T, a *App) {
	t.Helper()
	testwait.For(t, time.Second, "approval became visible", func() bool {
		a.showPendingApproval()
		return a.approval.Visible()
	})
}

// askFromJob queues a background job's approval the way jobs.jobApprover does,
// including the "[job:<id>] " display annotation.
func askFromJob(a *App, ctx context.Context, jobID, callID string) <-chan agent.ApprovalDecision {
	out := make(chan agent.ApprovalDecision, 1)
	req := approvalReq(callID)
	req.ToolCall.Name = "[job:" + jobID + "] test_tool"
	go func() { out <- a.approver.PromptJobApproval(ctx, jobID, req) }()
	return out
}

// TestShowPendingApproval_DeadEnvelopeReleasesTheModal is the head-of-line
// bug: a background job cancelled while its prompt was on screen used to leave
// the modal up forever, blocking the input and every other job behind an
// approval nobody was waiting for.
func TestShowPendingApproval_DeadEnvelopeReleasesTheModal(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)

	// The notify the approver fires is what the App turns into an
	// approvalPendingMsg, so assert on it rather than on a test-only poll.
	notified := make(chan struct{}, 8)
	r.app.approver.SetNotify(func() {
		select {
		case notified <- struct{}{}:
		default:
		}
	})

	doomedCtx, cancelDoomed := context.WithCancel(context.Background())
	doomed := askFromJob(r.app, doomedCtx, "aaaa1111", "doomed")
	showApprovalWhenPending(t, r.app)
	if got := r.app.approval.View(); !strings.Contains(got, "aaaa1111") {
		t.Fatalf("expected the first job's approval on screen:\n%s", got)
	}
	doomedID := r.app.approvalID

	liveCtx, cancelLive := context.WithCancel(context.Background())
	t.Cleanup(cancelLive)
	live := askFromJob(r.app, liveCtx, "bbbb2222", "live")
	testwait.For(t, time.Second, "second job's approval queued", func() bool {
		return r.app.approver.QueueDepth() == 2
	})

	for drained := false; !drained; {
		select {
		case <-notified:
		default:
			drained = true
		}
	}

	cancelDoomed()
	if got := waitDecision(t, doomed); got.Approved || got.Reason != "cancelled" {
		t.Fatalf("cancelled job decision = %+v", got)
	}
	select {
	case <-notified:
	case <-time.After(testwait.Timeout(time.Second)):
		t.Fatal("abandoning the displayed approval did not notify the UI")
	}

	// No user action: just the message that notify produces.
	r.app.Update(approvalPendingMsg{})
	if r.app.approvalID == doomedID {
		t.Fatal("the dead envelope still owns the modal")
	}
	if !r.app.approval.Visible() {
		t.Fatal("the second job's approval should have taken the freed slot")
	}
	if got := r.app.approval.View(); !strings.Contains(got, "bbbb2222") {
		t.Fatalf("expected the second job's approval on screen:\n%s", got)
	}
	if depth := r.app.approver.QueueDepth(); depth != 1 {
		t.Fatalf("queue depth = %d, want 1 (the dead envelope must not be counted)", depth)
	}

	r.app.rejectVisibleApproval("test cleanup")
	waitDecision(t, live)
}

// TestApprovalResult_CannotLandOnADifferentEnvelope pins the sharpest
// correctness property: an answer made about one request must never resolve
// the request that replaced it.
func TestApprovalResult_CannotLandOnADifferentEnvelope(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	first := askFromJob(r.app, firstCtx, "aaaa1111", "first")
	showApprovalWhenPending(t, r.app)

	// The user's keypress. Captured before the prompt is replaced, exactly as
	// a real ResultMsg is: the message is produced by a tea.Cmd and delivered
	// on a later frame.
	staleAnswer := approvalResult(t, r.app, "1")

	secondCtx, cancelSecond := context.WithCancel(context.Background())
	t.Cleanup(cancelSecond)
	second := askFromJob(r.app, secondCtx, "bbbb2222", "second")
	cancelFirst()
	waitDecision(t, first)
	testwait.For(t, time.Second, "second approval took the slot", func() bool {
		r.app.showPendingApproval()
		return r.app.approval.Visible() && strings.Contains(r.app.approval.View(), "bbbb2222")
	})

	r.app.resolveApprovalResult(staleAnswer)

	select {
	case dec := <-second:
		t.Fatalf("a decision made about the first request resolved the second: %+v", dec)
	case <-time.After(50 * time.Millisecond):
	}
	if !r.app.approval.Visible() {
		t.Fatal("the second request should still be waiting for its own answer")
	}

	r.app.rejectVisibleApproval("test cleanup")
	waitDecision(t, second)
}

// TestCtrlC_DuringForegroundTurn_LeavesBackgroundApprovalPending: Ctrl+C
// cancels the user's turn. A background job's approval is not part of it.
func TestCtrlC_DuringForegroundTurn_LeavesBackgroundApprovalPending(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)
	r.app.streaming = true

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	decision := askFromJob(r.app, ctx, "aaaa1111", "background")
	showApprovalWhenPending(t, r.app)
	boundID := r.app.approvalID

	if _, cmd := r.app.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("Ctrl+C during a turn must not quit")
		}
	}

	select {
	case dec := <-decision:
		t.Fatalf("Ctrl+C rejected a background job's approval: %+v", dec)
	case <-time.After(50 * time.Millisecond):
	}
	if !r.app.approval.Visible() || r.app.approvalID != boundID {
		t.Fatal("the background approval should still be displayed and bound")
	}

	r.app.rejectVisibleApproval("test cleanup")
	waitDecision(t, decision)
}

// TestCtrlC_DuringForegroundTurn_RejectsForegroundApproval is the other half:
// the user's own approval is part of the turn Ctrl+C cancels.
func TestCtrlC_DuringForegroundTurn_RejectsForegroundApproval(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)
	r.app.streaming = true

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	decisions := make(chan agent.ApprovalDecision, 1)
	go func() { decisions <- r.app.approver.Approve(ctx, approvalReq("foreground")) }()
	showApprovalWhenPending(t, r.app)

	r.app.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if got := waitDecision(t, decisions); got.Approved || got.Reason != "cancelled" {
		t.Fatalf("foreground decision = %+v, want cancelled", got)
	}
	if r.app.approval.Visible() {
		t.Fatal("the foreground approval should have been dismissed")
	}
}

// TestForegroundApprovalOutranksBackground: the foreground turn is the thing
// the user is waiting on, so its approval is displayed first even though the
// background request arrived earlier.
func TestForegroundApprovalOutranksBackground(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	// Enqueue strictly background-then-foreground, so displaying the
	// foreground one first can only be the ordering rule and never a race.
	queued := make(chan struct{}, 2)
	r.app.approver.SetNotify(func() { queued <- struct{}{} })
	background := askFromJob(r.app, ctx, "aaaa1111", "background")
	<-queued
	foreground := make(chan agent.ApprovalDecision, 1)
	go func() { foreground <- r.app.approver.Approve(ctx, approvalReq("foreground")) }()
	<-queued

	showApprovalWhenPending(t, r.app)
	if r.app.approvalOrigin != originForeground {
		t.Fatal("the foreground approval should be displayed ahead of the background one")
	}
	r.app.rejectVisibleApproval("test cleanup")
	waitDecision(t, foreground)

	showApprovalWhenPending(t, r.app)
	if r.app.approvalOrigin != originBackground {
		t.Fatal("the background approval should follow")
	}
	r.app.rejectVisibleApproval("test cleanup")
	waitDecision(t, background)
}

// TestRememberOnAbandonedApproval_InstallsNoRule: "yes, and don't ask again"
// answered on a prompt whose job has already gone away must not grant standing
// session authority for the tool.
func TestRememberOnAbandonedApproval_InstallsNoRule(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)

	ctx, cancel := context.WithCancel(context.Background())
	decision := askFromJob(r.app, ctx, "aaaa1111", "abandoned")
	showApprovalWhenPending(t, r.app)

	// The user reaches for "2" while the job dies underneath the prompt.
	answer := approvalResult(t, r.app, "2")
	if !answer.Remember {
		t.Fatal("expected the always-allow choice")
	}
	cancel()
	waitDecision(t, decision)

	r.app.resolveApprovalResult(answer)

	got := r.app.currentPermissionPolicy().Decide(permissions.Request{
		ToolName:         "test_tool",
		RequiresApproval: true,
		Params:           json.RawMessage(`{}`),
	}).Decision
	if got != permissions.DecisionAsk {
		t.Fatalf("abandoned approval installed a %s rule for test_tool", got)
	}
}

// TestRememberOnLiveApproval_InstallsRule guards the fix from over-reaching:
// the ordinary path must still remember.
func TestRememberOnLiveApproval_InstallsRule(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	decisions := make(chan agent.ApprovalDecision, 1)
	go func() { decisions <- r.app.approver.Approve(ctx, approvalReq("live")) }()
	showApprovalWhenPending(t, r.app)

	r.app.resolveApprovalResult(approvalResult(t, r.app, "2"))
	if got := waitDecision(t, decisions); !got.Approved {
		t.Fatalf("decision = %+v, want approved", got)
	}
	got := r.app.currentPermissionPolicy().Decide(permissions.Request{
		ToolName:         "test_tool",
		RequiresApproval: true,
		Params:           json.RawMessage(`{}`),
	}).Decision
	if got != permissions.DecisionAllow {
		t.Fatalf("live always-allow produced %s, want allow", got)
	}
}

// TestJobApproverReportsOriginAsData: origin reaches the queue through the
// jobs adapter as a field, not by parsing the display prefix back out.
func TestJobApproverReportsOriginAsData(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	approver := jobs.NewJobApprover(r.app.Approver(), "aaaa1111", true)
	decisions := make(chan agent.ApprovalDecision, 1)
	go func() { decisions <- approver.Approve(ctx, approvalReq("from-job")) }()

	next := waitPendingApproval(t, r.app.approver)
	if next.origin != originBackground {
		t.Fatalf("origin = %v, want background", next.origin)
	}
	if next.jobID != "aaaa1111" {
		t.Fatalf("jobID = %q, want aaaa1111", next.jobID)
	}
	r.app.approver.ResolveID(next.id, agent.ApprovalDecision{Approved: false, Reason: "test cleanup"})
	waitDecision(t, decisions)
}

// TestPlanModeStillRevokesQueuedBackgroundWrites: a snapshot-bound background
// request is not silently broadened by a later foreground policy, but a later
// deny — here the read-only floor of plan mode — must still revoke it, and must
// do so without ever putting it in front of the user.
func TestPlanModeStillRevokesQueuedBackgroundWrites(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	writeTool := tools.NewWriteFileTool(r.tmp, nil)
	call := provider.ToolCall{ID: "w1", Name: "[job:aaaa1111] " + writeTool.Name(), Arguments: `{"path":"a.txt","content":"x"}`}
	queued := make(chan struct{}, 1)
	r.app.approver.SetNotify(func() { queued <- struct{}{} })
	decisions := make(chan agent.ApprovalDecision, 1)
	go func() {
		decisions <- r.app.approver.PromptJobApproval(ctx, "aaaa1111", agent.ApprovalRequest{
			Tool: writeTool, ToolCall: call, Params: json.RawMessage(call.Arguments),
		})
	}()
	<-queued

	r.app.applyPermMode(modePlan)
	if _, ok := r.app.approver.Next(); ok {
		t.Fatal("plan mode should revoke the queued write without displaying it")
	}
	if got := waitDecision(t, decisions); got.Approved || got.Reason == "" {
		t.Fatalf("decision = %+v, want a policy denial", got)
	}
}

// TestBackgroundAskSurvivesForegroundBroadening is the other side of the same
// snapshot rule: turning the foreground policy up must not approve work a
// background job was launched under stricter terms.
func TestBackgroundAskSurvivesForegroundBroadening(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	decisions := askFromJob(r.app, ctx, "aaaa1111", "snapshot")
	showApprovalWhenPending(t, r.app)

	r.app.applyPermMode(modeBypass)
	if r.app.approver.ResolveActiveByPolicy(r.app.approvalID) {
		t.Fatal("a broadened foreground policy approved a running job's request")
	}
	if !r.app.approval.Visible() {
		t.Fatal("the background approval must stay on screen for an explicit answer")
	}

	r.app.rejectVisibleApproval("test cleanup")
	waitDecision(t, decisions)
}

// approvalResult presses one of the prompt's keys and returns the ResultMsg it
// produced, without letting the App consume it.
func approvalResult(t *testing.T, a *App, key string) approval.ResultMsg {
	t.Helper()
	model, cmd := a.approval.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	a.approval = model
	if cmd == nil {
		t.Fatalf("key %q produced no result", key)
	}
	msg, ok := cmd().(approval.ResultMsg)
	if !ok {
		t.Fatalf("key %q produced %T, want an approval result", key, cmd())
	}
	return msg
}
