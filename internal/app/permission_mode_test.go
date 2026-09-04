package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/testwait"
	"github.com/packetcode/packetcode/internal/tools"
)

func TestNextPermMode_CycleOrder(t *testing.T) {
	// normal → accept-edits → auto → plan → normal
	if got := nextPermMode(modeNormal); got != modeAcceptEdits {
		t.Fatalf("normal → %v, want accept-edits", got)
	}
	if got := nextPermMode(modeAcceptEdits); got != modeAuto {
		t.Fatalf("accept-edits → %v, want auto", got)
	}
	if got := nextPermMode(modeAuto); got != modePlan {
		t.Fatalf("auto → %v, want plan", got)
	}
	if got := nextPermMode(modePlan); got != modeNormal {
		t.Fatalf("plan → %v, want normal", got)
	}
}

func TestCyclePermissionMode_WalksProfilesAndPlan(t *testing.T) {
	r := newTestApp(t)

	r.app.applyPermMode(modeNormal)
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileAsk {
		t.Fatalf("normal profile = %v, want ask", got)
	}

	// normal → accept-edits (profile "edit").
	r.app.cyclePermissionMode()
	if got := r.app.currentPermMode(); got != modeAcceptEdits {
		t.Fatalf("after 1st cycle = %v, want accept-edits", got)
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileEdit {
		t.Fatalf("accept-edits profile = %v, want edit", got)
	}

	// accept-edits → auto (profile "auto").
	r.app.cyclePermissionMode()
	if got := r.app.currentPermMode(); got != modeAuto {
		t.Fatalf("after 2nd cycle = %v, want auto", got)
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileAuto {
		t.Fatalf("auto profile = %v, want auto", got)
	}

	// auto → plan (planMode on, profile "safe").
	r.app.cyclePermissionMode()
	if got := r.app.currentPermMode(); got != modePlan {
		t.Fatalf("after 3rd cycle = %v, want plan", got)
	}
	if !r.app.planMode {
		t.Fatal("plan mode flag should be set")
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileSafe {
		t.Fatalf("plan profile = %v, want safe", got)
	}

	// plan → normal (flag cleared, profile "ask").
	r.app.cyclePermissionMode()
	if got := r.app.currentPermMode(); got != modeNormal {
		t.Fatalf("after 4th cycle = %v, want normal", got)
	}
	if r.app.planMode {
		t.Fatal("plan mode flag should be cleared")
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileAsk {
		t.Fatalf("normal profile = %v, want ask", got)
	}
}

func TestCyclePermissionMode_WhileStreamingUpdatesLivePolicy(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)
	r.app.streaming = true

	r.app.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := r.app.currentPermMode(); got != modeAcceptEdits {
		t.Fatalf("first streaming cycle = %v, want accept-edits", got)
	}
	r.app.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := r.app.currentPermMode(); got != modeAuto {
		t.Fatalf("second streaming cycle = %v, want auto", got)
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileAuto {
		t.Fatalf("live profile = %v, want auto", got)
	}
}

func TestCyclePermissionMode_ReevaluatesVisibleApproval(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)
	r.app.streaming = true
	tool := tools.NewExecuteCommandTool(r.tmp)
	call := provider.ToolCall{ID: "call-1", Name: tool.Name(), Arguments: `{"command":"go test ./..."}`}
	req := agent.ApprovalRequest{Tool: tool, ToolCall: call, Params: json.RawMessage(call.Arguments)}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	decisionCh := make(chan agent.ApprovalDecision, 1)
	go func() { decisionCh <- r.app.approver.Approve(ctx, req) }()

	showApprovalWhenPending(t, r.app)
	r.app.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab}) // manual -> accept edits; shell still asks
	if !r.app.approval.Visible() {
		t.Fatal("accept-edits should keep a shell approval visible")
	}
	r.app.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab}) // accept edits -> auto; shell is allowed
	if r.app.approval.Visible() {
		t.Fatal("auto mode should resolve and hide the existing shell approval")
	}
	decision := waitDecision(t, decisionCh)
	if !decision.Approved {
		t.Fatalf("live auto-mode decision = %+v, want approved", decision)
	}
}

func TestCyclePermissionMode_AdvancesToNextQueuedApproval(t *testing.T) {
	r := newTestApp(t)
	r.app.applyPermMode(modeNormal)
	r.app.streaming = true
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	writeTool := tools.NewWriteFileTool(r.tmp, nil)
	writeCall := provider.ToolCall{ID: "write-1", Name: writeTool.Name(), Arguments: `{"path":"a.txt","content":"x"}`}
	writeDecision := make(chan agent.ApprovalDecision, 1)
	go func() {
		writeDecision <- r.app.approver.Approve(ctx, agent.ApprovalRequest{
			Tool: writeTool, ToolCall: writeCall, Params: json.RawMessage(writeCall.Arguments),
		})
	}()
	showApprovalWhenPending(t, r.app)

	shellTool := tools.NewExecuteCommandTool(r.tmp)
	shellCall := provider.ToolCall{ID: "shell-2", Name: shellTool.Name(), Arguments: `{"command":"go test ./..."}`}
	shellDecision := make(chan agent.ApprovalDecision, 1)
	go func() {
		shellDecision <- r.app.approver.Approve(ctx, agent.ApprovalRequest{
			Tool: shellTool, ToolCall: shellCall, Params: json.RawMessage(shellCall.Arguments),
		})
	}()
	testwait.For(t, time.Second, "second approval reached the queue", func() bool {
		return r.app.approver.QueueDepth() == 2
	})

	r.app.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab}) // manual -> accept edits
	if decision := waitDecision(t, writeDecision); !decision.Approved {
		t.Fatalf("write decision = %+v, want approved", decision)
	}
	if !r.app.approval.Visible() {
		t.Fatal("queued shell approval should become visible")
	}
	if view := r.app.approval.View(); !strings.Contains(view, "go test ./...") {
		t.Fatalf("next approval was not the queued shell command:\n%s", view)
	}

	r.app.approver.ResolveID(r.app.approvalID, agent.ApprovalDecision{Approved: false, Reason: "test cleanup"})
	if decision := waitDecision(t, shellDecision); decision.Approved {
		t.Fatalf("shell cleanup decision = %+v, want rejected", decision)
	}
}

func TestCyclePermissionMode_BypassIsOutOfCycleAndExitsToNormal(t *testing.T) {
	r := newTestApp(t)

	// Bypass is reached deliberately (as /trust on does) — full profile.
	r.app.applyPermMode(modeBypass)
	if got := r.app.currentPermMode(); got != modeBypass {
		t.Fatalf("mode = %v, want bypass", got)
	}
	if got := r.app.currentPermissionPolicy().Profile(); got != permissions.ProfileFull {
		t.Fatalf("bypass profile = %v, want full", got)
	}

	// The forward cycle never yields bypass.
	for m := modeNormal; m <= modePlan; m++ {
		if nextPermMode(m) == modeBypass {
			t.Fatalf("cycle from %v produced bypass; it must be out of the cycle", m)
		}
	}

	// Shift+Tab from bypass drops back to normal (never a trap).
	r.app.cyclePermissionMode()
	if got := r.app.currentPermMode(); got != modeNormal {
		t.Fatalf("bypass should cycle to normal, got %v", got)
	}
}
