package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/hooks"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/sugar"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
)

// ────────────────────────────────────────────────────────────────────────────
// Test fixtures
// ────────────────────────────────────────────────────────────────────────────

// scriptedProvider replays a fixed sequence of stream-event batches, one
// batch per ChatCompletion call. Lets us script multi-turn conversations
// (LLM responds → tool runs → LLM responds again) without an HTTP server.
type scriptedProvider struct {
	slug         string
	turns        [][]provider.StreamEvent
	turnIdx      int32
	chatCount    int32
	lastRequest  provider.ChatRequest
	disableTools bool
}

// gatedToolProvider pauses the first provider turn before emitting a tool call,
// allowing a test to change permission mode while Agent.Run is active.
type gatedToolProvider struct {
	started chan struct{}
	release chan struct{}
	turn    int32
}

func (g *gatedToolProvider) Name() string                              { return "gated" }
func (g *gatedToolProvider) Slug() string                              { return "gated" }
func (g *gatedToolProvider) BrandColor() lipgloss.Color                { return lipgloss.Color("#000000") }
func (g *gatedToolProvider) ValidateKey(context.Context, string) error { return nil }
func (g *gatedToolProvider) Pricing(string) (float64, float64)         { return 0, 0 }
func (g *gatedToolProvider) ContextWindow(string) int                  { return 100_000 }
func (g *gatedToolProvider) SupportsTools(string) bool                 { return true }
func (g *gatedToolProvider) ListModels(context.Context) ([]provider.Model, error) {
	return nil, nil
}
func (g *gatedToolProvider) ChatCompletion(ctx context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	turn := atomic.AddInt32(&g.turn, 1)
	ch := make(chan provider.StreamEvent, 4)
	go func() {
		defer close(ch)
		if turn == 1 {
			close(g.started)
			select {
			case <-g.release:
			case <-ctx.Done():
				ch <- provider.StreamEvent{Type: provider.EventError, Error: ctx.Err()}
				return
			}
			ch <- provider.StreamEvent{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "execute_command"}}
			ch <- provider.StreamEvent{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{}`}}
			ch <- provider.StreamEvent{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}}
			ch <- provider.StreamEvent{Type: provider.EventDone}
			return
		}
		ch <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: "done"}
		ch <- provider.StreamEvent{Type: provider.EventDone}
	}()
	return ch, nil
}

func (s *scriptedProvider) Name() string { return "scripted" }
func (s *scriptedProvider) Slug() string {
	if s.slug != "" {
		return s.slug
	}
	return "scripted"
}
func (s *scriptedProvider) BrandColor() lipgloss.Color                             { return lipgloss.Color("#000000") }
func (s *scriptedProvider) ValidateKey(_ context.Context, _ string) error          { return nil }
func (s *scriptedProvider) ListModels(_ context.Context) ([]provider.Model, error) { return nil, nil }
func (s *scriptedProvider) Pricing(string) (float64, float64)                      { return 1.0, 5.0 }
func (s *scriptedProvider) ContextWindow(string) int                               { return 100_000 }
func (s *scriptedProvider) SupportsTools(string) bool                              { return !s.disableTools }

func (s *scriptedProvider) ChatCompletion(_ context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	atomic.AddInt32(&s.chatCount, 1)
	idx := atomic.AddInt32(&s.turnIdx, 1) - 1
	s.lastRequest = req
	if int(idx) >= len(s.turns) {
		return nil, errors.New("scriptedProvider: no more turns scripted")
	}
	ch := make(chan provider.StreamEvent, len(s.turns[idx]))
	for _, ev := range s.turns[idx] {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// recordingTool exposes whether Execute was called and what params it
// saw. Used to verify the agent dispatches with the LLM-supplied (or
// approver-edited) arguments.
type recordingTool struct {
	name      string
	approval  bool
	executed  int32
	lastInput string
	result    tools.ToolResult
}

func (r *recordingTool) Name() string            { return r.name }
func (r *recordingTool) Description() string     { return "test tool" }
func (r *recordingTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (r *recordingTool) RequiresApproval() bool  { return r.approval }
func (r *recordingTool) Execute(_ context.Context, p json.RawMessage) (tools.ToolResult, error) {
	atomic.AddInt32(&r.executed, 1)
	r.lastInput = string(p)
	res := r.result
	if res.Content == "" {
		res.Content = "ok"
	}
	return res, nil
}

func newAgentRig(t *testing.T, prov provider.Provider, ts ...tools.Tool) (*Agent, *session.Manager, *cost.Tracker) {
	t.Helper()
	reg := provider.NewRegistry()
	reg.Register(prov)
	require.NoError(t, reg.SetActive(prov.Slug(), "scripted-model"))

	tr := tools.NewRegistry()
	for _, tool := range ts {
		tr.Register(tool)
	}

	sessDir := t.TempDir()
	sm := session.NewManager(sessDir)
	_, err := sm.New(prov.Slug(), "scripted-model")
	require.NoError(t, err)

	tally := filepath.Join(t.TempDir(), "tally.json")
	ct, err := cost.NewTracker(tally, func(string, string) (float64, float64) { return 1.0, 5.0 })
	require.NoError(t, err)

	a := New(Config{
		Registry:    reg,
		Tools:       tr,
		Session:     sm,
		CostTracker: ct,
		Approver:    AutoApprove(),
	})
	return a, sm, ct
}

func collect(events <-chan AgentEvent) []AgentEvent {
	var out []AgentEvent
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

// ────────────────────────────────────────────────────────────────────────────
// Tests
// ────────────────────────────────────────────────────────────────────────────

func TestAgent_TokenBudgetStopsAtToolTurnBoundary(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{{
		{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "read", ArgumentsDelta: `{}`}},
		{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
		{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 8, OutputTokens: 2}},
	}}}
	a, _, _ := newAgentRig(t, prov, &recordingTool{name: "read"})
	a.tokenBudget = 10
	events := collect(a.Run(context.Background(), "read"))
	var budgetErr error
	for _, ev := range events {
		if ev.Type == EventError {
			budgetErr = ev.Error
		}
	}
	require.Error(t, budgetErr)
	assert.Contains(t, budgetErr.Error(), "used 10 tokens (budget 10)")
	assert.Equal(t, int32(1), atomic.LoadInt32(&prov.chatCount), "must not begin another provider stream")
}

func TestAgent_TextOnlyTurn(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventTextDelta, TextDelta: "Hello"},
			{Type: provider.EventTextDelta, TextDelta: " there"},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 10, OutputTokens: 2}},
		},
	}}

	a, sm, _ := newAgentRig(t, prov)
	events := collect(a.Run(context.Background(), "hi"))

	var text string
	var sawDone, sawUsage bool
	for _, ev := range events {
		switch ev.Type {
		case EventTextDelta:
			text += ev.Text
		case EventDone:
			sawDone = true
		case EventUsageUpdate:
			sawUsage = true
			assert.Equal(t, 10, ev.Usage.InputTokens)
		}
	}
	assert.Equal(t, "Hello there", text)
	assert.True(t, sawDone)
	assert.True(t, sawUsage)

	cur := sm.Current()
	require.Len(t, cur.Messages, 2, "user + assistant message persisted")
	assert.Equal(t, provider.RoleUser, cur.Messages[0].Role)
	assert.Equal(t, provider.RoleAssistant, cur.Messages[1].Role)
	assert.Equal(t, "Hello there", cur.Messages[1].Content)
	assert.Equal(t, 10, cur.TokenUsage.TotalInput)
}

func TestAgent_DoesNotAttachSugarCacheMetadataToOtherProviders(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{{{Type: provider.EventDone}}}}
	a, sm, _ := newAgentRig(t, prov, &recordingTool{name: "zeta"}, &recordingTool{name: "alpha"})
	a.systemPrompt = "stable system"
	collect(a.Run(context.Background(), "hi"))

	assert.Nil(t, prov.lastRequest.SugarCache)
	assert.NotNil(t, sm.Current())
	require.Len(t, prov.lastRequest.Tools, 2)
	assert.Equal(t, "alpha", prov.lastRequest.Tools[0].Name)
	assert.Equal(t, "zeta", prov.lastRequest.Tools[1].Name)
}

func TestAgent_AttachesStableSessionCacheMetadataToEnabledSugar(t *testing.T) {
	prov := &scriptedProvider{slug: sugar.Slug, turns: [][]provider.StreamEvent{{{Type: provider.EventDone}}}}
	a, sm, _ := newAgentRig(t, prov, &recordingTool{name: "zeta"}, &recordingTool{name: "alpha"})
	a.systemPrompt = "stable system"
	a.sugarCache = SugarCacheConfig{
		Enabled: true, Mode: provider.SugarCacheAuto,
		Retention: provider.SugarCacheProviderDefault, Privacy: provider.SugarPrivacyStandard,
	}
	collect(a.Run(context.Background(), "hi"))

	cache := prov.lastRequest.SugarCache
	require.NotNil(t, cache)
	assert.Equal(t, sm.Current().ID, cache.ConversationID)
	assert.Equal(t, 0, cache.CompactionGeneration)
	assert.Equal(t, 1, cache.StablePrefixMessages)
	assert.Equal(t, provider.SugarCacheAuto, cache.Mode)
	assert.Equal(t, provider.SugarCacheProviderDefault, cache.Retention)
	assert.Equal(t, provider.SugarPrivacyStandard, cache.Privacy)
	assert.Equal(t, provider.CachePrefixFingerprint("stable system", prov.lastRequest.Tools), cache.PrefixFingerprint)
}

func TestAgent_ToolCallApprovedAndExecuted(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			// Turn 1: LLM proposes a tool call.
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "do_thing"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{"x":1}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 5, OutputTokens: 7}},
		},
		{
			// Turn 2: LLM responds to the tool result with text and stops.
			{Type: provider.EventTextDelta, TextDelta: "All done"},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 12, OutputTokens: 3}},
		},
	}}

	rt := &recordingTool{name: "do_thing", approval: true, result: tools.ToolResult{Content: "tool ran"}}
	a, sm, _ := newAgentRig(t, prov, rt)

	evs := collect(a.Run(context.Background(), "do the thing"))

	var sawProposed, sawApproved, sawExecuted bool
	for _, ev := range evs {
		switch ev.Type {
		case EventToolCallProposed:
			sawProposed = true
			assert.Equal(t, "do_thing", ev.ToolCall.Name)
		case EventToolCallApproved:
			sawApproved = true
		case EventToolCallExecuted:
			sawExecuted = true
			assert.Equal(t, "tool ran", ev.ToolResult.Content)
		}
	}
	assert.True(t, sawProposed)
	assert.True(t, sawApproved)
	assert.True(t, sawExecuted)
	assert.Equal(t, int32(1), atomic.LoadInt32(&rt.executed))
	assert.JSONEq(t, `{"x":1}`, rt.lastInput)

	// Session should now have user, assistant(tool_call), tool, assistant(text) = 4 messages.
	cur := sm.Current()
	require.Len(t, cur.Messages, 4)
	assert.Equal(t, provider.RoleUser, cur.Messages[0].Role)
	assert.Equal(t, provider.RoleAssistant, cur.Messages[1].Role)
	require.Len(t, cur.Messages[1].ToolCalls, 1)
	assert.Equal(t, provider.RoleTool, cur.Messages[2].Role)
	assert.Equal(t, "tool ran", cur.Messages[2].Content)
	assert.Equal(t, provider.RoleAssistant, cur.Messages[3].Role)
	assert.Equal(t, "All done", cur.Messages[3].Content)
}

func TestAgent_SetPolicyDuringRunAppliesToLaterToolCall(t *testing.T) {
	prov := &gatedToolProvider{started: make(chan struct{}), release: make(chan struct{})}
	tool := &recordingTool{name: "execute_command", approval: true}
	a, _, _ := newAgentRig(t, prov, tool)
	a.SetApprover(AutoReject("manual mode would reject"))
	a.SetPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))

	events := a.Run(context.Background(), "run tests")
	select {
	case <-prov.started:
	case <-time.After(time.Second):
		t.Fatal("provider turn did not start")
	}
	// This is the live Shift+Tab path at the agent boundary: the policy changes
	// while Run is active, before the model's subsequent tool call is handled.
	a.SetPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAuto))
	close(prov.release)
	_ = collect(events)

	if got := atomic.LoadInt32(&tool.executed); got != 1 {
		t.Fatalf("tool executions = %d, want 1 after live switch to auto", got)
	}
}

// SetApprover is documented as a mid-conversation swap (/trust), so the write
// races the running turn's read in handleToolCall. Under -race this pins that
// the swap is synchronised; it also pins that the swapped-in approver is the
// one actually consulted for the turn's later tool call.
func TestAgent_SetApproverDuringRunAppliesToLaterToolCall(t *testing.T) {
	prov := &gatedToolProvider{started: make(chan struct{}), release: make(chan struct{})}
	tool := &recordingTool{name: "execute_command", approval: true}
	a, _, _ := newAgentRig(t, prov, tool)
	a.SetPolicy(permissions.DefaultPolicy().WithProfile(permissions.ProfileAsk))

	events := a.Run(context.Background(), "run tests")
	select {
	case <-prov.started:
	case <-time.After(time.Second):
		t.Fatal("provider turn did not start")
	}
	a.SetApprover(AutoReject("user switched to manual mid-turn"))
	close(prov.release)

	sawRejected := false
	for _, ev := range collect(events) {
		if ev.Type == EventToolCallRejected {
			sawRejected = true
			assert.Contains(t, ev.Text, "user switched to manual mid-turn")
		}
	}
	assert.True(t, sawRejected, "the approver installed mid-run must be the one consulted")
	if got := atomic.LoadInt32(&tool.executed); got != 0 {
		t.Fatalf("tool executions = %d, want 0 after live switch to reject", got)
	}
}

// A nil approver must not become a nil-panic on the next tool call; New()
// already substitutes AutoReject, and the setter has to hold that line.
func TestAgent_SetApproverNilFallsBackToReject(t *testing.T) {
	a := New(Config{})
	a.SetApprover(nil)
	decision := a.currentApprover().Approve(context.Background(), ApprovalRequest{})
	assert.False(t, decision.Approved)
}

func TestAgent_DropsTextOnToolCallTurn(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventTextDelta, TextDelta: `<|python_tag|>{"path":"main.go"}`},
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "do_thing"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{"path":"main.go"}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventDone},
		},
		{
			{Type: provider.EventTextDelta, TextDelta: "done"},
			{Type: provider.EventDone},
		},
	}}

	rt := &recordingTool{name: "do_thing", result: tools.ToolResult{Content: "tool ran"}}
	a, sm, _ := newAgentRig(t, prov, rt)

	evs := collect(a.Run(context.Background(), "do it"))
	var sawLeakedText bool
	for _, ev := range evs {
		if ev.Type == EventTextDelta && strings.Contains(ev.Text, "<|python_tag|>") {
			sawLeakedText = true
		}
	}
	assert.True(t, sawLeakedText, "text still streams live; UI/session drop it when a tool call follows")

	cur := sm.Current()
	require.Len(t, cur.Messages, 4)
	assert.Equal(t, "", cur.Messages[1].Content)
	require.Len(t, cur.Messages[1].ToolCalls, 1)
	assert.Equal(t, "do_thing", cur.Messages[1].ToolCalls[0].Name)
	assert.Equal(t, "done", cur.Messages[3].Content)
}

func TestAgent_UnsupportedModelOmitsNativeTools(t *testing.T) {
	prov := &scriptedProvider{
		turns: [][]provider.StreamEvent{{
			{Type: provider.EventTextDelta, TextDelta: "plain response"},
			{Type: provider.EventDone},
		}},
	}
	prov.disableTools = true

	rt := &recordingTool{name: "do_thing"}
	a, _, _ := newAgentRig(t, prov, rt)

	_ = collect(a.Run(context.Background(), "hi"))
	assert.Empty(t, prov.lastRequest.Tools)
	require.NotEmpty(t, prov.lastRequest.Messages)
	assert.Equal(t, provider.RoleSystem, prov.lastRequest.Messages[0].Role)
	assert.Contains(t, prov.lastRequest.Messages[0].Content, "Native tool calling is unavailable")
	assert.Contains(t, prov.lastRequest.Messages[0].Content, "scripted-model")
}

func TestAgent_InvalidToolCallArgumentsError(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "do_thing"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{"path":`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventDone},
		},
	}}

	rt := &recordingTool{name: "do_thing"}
	a, _, _ := newAgentRig(t, prov, rt)

	evs := collect(a.Run(context.Background(), "do it"))
	var gotErr error
	for _, ev := range evs {
		if ev.Type == EventError {
			gotErr = ev.Error
		}
	}
	require.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), "invalid JSON")
	assert.Equal(t, int32(0), atomic.LoadInt32(&rt.executed))
}

func TestAgent_ToolCallRejected(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "danger"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 5, OutputTokens: 5}},
		},
		{
			{Type: provider.EventTextDelta, TextDelta: "OK, skipping"},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 10, OutputTokens: 2}},
		},
	}}

	rt := &recordingTool{name: "danger", approval: true}

	reg := provider.NewRegistry()
	reg.Register(prov)
	require.NoError(t, reg.SetActive("scripted", "scripted-model"))
	tr := tools.NewRegistry()
	tr.Register(rt)
	sm := session.NewManager(t.TempDir())
	_, _ = sm.New("scripted", "scripted-model")
	a := New(Config{
		Registry: reg,
		Tools:    tr,
		Session:  sm,
		Approver: AutoReject("nope"),
	})

	evs := collect(a.Run(context.Background(), "be dangerous"))

	var rejected bool
	for _, ev := range evs {
		if ev.Type == EventToolCallRejected {
			rejected = true
		}
	}
	assert.True(t, rejected)
	assert.Equal(t, int32(0), atomic.LoadInt32(&rt.executed), "rejected tool must not be executed")

	// The rejection message ends up in the conversation as a tool-role
	// message so the LLM sees it.
	cur := sm.Current()
	var found bool
	for _, m := range cur.Messages {
		if m.Role == provider.RoleTool && m.Content == "nope" {
			found = true
		}
	}
	assert.True(t, found, "rejection reason should be in session as a tool-role message")
}

func TestAgent_ReadOnlyToolSkipsApproval(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "peek"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 1, OutputTokens: 1}},
		},
		{
			{Type: provider.EventTextDelta, TextDelta: "done"},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 2, OutputTokens: 1}},
		},
	}}

	rt := &recordingTool{name: "peek", approval: false}
	a, _, _ := newAgentRig(t, prov, rt)
	// Use AutoReject so that *if* approval were called, the tool would not
	// run — proves the agent didn't ask for approval.
	a.SetApprover(AutoReject("would be rejected"))

	collect(a.Run(context.Background(), "peek"))

	assert.Equal(t, int32(1), atomic.LoadInt32(&rt.executed), "non-approval tools must run regardless of approver")
}

func TestAgent_UnknownToolReportsError(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "missing"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 5, OutputTokens: 5}},
		},
		{
			{Type: provider.EventTextDelta, TextDelta: "I'll try something else"},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 8, OutputTokens: 3}},
		},
	}}

	a, sm, _ := newAgentRig(t, prov)
	collect(a.Run(context.Background(), "do missing"))

	cur := sm.Current()
	var foundErrMsg bool
	for _, m := range cur.Messages {
		if m.Role == provider.RoleTool && m.Content == "unknown tool: missing" {
			foundErrMsg = true
		}
	}
	assert.True(t, foundErrMsg)
}

func TestAgent_NoActiveProviderErrors(t *testing.T) {
	reg := provider.NewRegistry()
	tr := tools.NewRegistry()
	sm := session.NewManager(t.TempDir())
	_, _ = sm.New("none", "none")

	a := New(Config{
		Registry: reg,
		Tools:    tr,
		Session:  sm,
		Approver: AutoApprove(),
	})

	evs := collect(a.Run(context.Background(), "hi"))
	require.NotEmpty(t, evs)
	last := evs[len(evs)-1]
	assert.Equal(t, EventError, last.Type)
}

func TestAgent_CostTrackerUpdated(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventTextDelta, TextDelta: "hi"},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 1_000_000, OutputTokens: 500_000}},
		},
	}}
	a, sm, ct := newAgentRig(t, prov)
	collect(a.Run(context.Background(), "hi"))

	id := sm.Current().ID
	in, out := ct.SessionTokens(id)
	assert.Equal(t, 1_000_000, in)
	assert.Equal(t, 500_000, out)

	// Pricing in newAgentRig is $1/M in, $5/M out → $1 + $2.50 = $3.50.
	assert.InDelta(t, 3.50, ct.SessionCost(id), 1e-9)
}

func TestAgent_ParallelToolCallsDispatched(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c0", Name: "alpha"}},
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 1, ID: "c1", Name: "beta"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{"a":1}`}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 1, ArgumentsDelta: `{"b":2}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 1}},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 5, OutputTokens: 5}},
		},
		{
			{Type: provider.EventTextDelta, TextDelta: "both done"},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 10, OutputTokens: 2}},
		},
	}}

	alpha := &recordingTool{name: "alpha", approval: false}
	beta := &recordingTool{name: "beta", approval: false}
	a, sm, _ := newAgentRig(t, prov, alpha, beta)

	collect(a.Run(context.Background(), "do both"))

	assert.Equal(t, int32(1), atomic.LoadInt32(&alpha.executed))
	assert.Equal(t, int32(1), atomic.LoadInt32(&beta.executed))

	cur := sm.Current()
	require.Len(t, cur.Messages, 5, "user, assistant(2 tool_calls), tool0, tool1, assistant(text)")
	require.Len(t, cur.Messages[1].ToolCalls, 2)
	assert.Equal(t, "alpha", cur.Messages[1].ToolCalls[0].Name)
	assert.Equal(t, "beta", cur.Messages[1].ToolCalls[1].Name)
}

// ────────────────────────────────────────────────────────────────────────────
// Cancellation — Round 5
// ────────────────────────────────────────────────────────────────────────────

// cancellableProvider hands back a stream channel whose lifetime is
// bounded by the ChatCompletion ctx. The goroutine emits EventError
// (context.Canceled) as soon as ctx is done, mirroring what real
// providers do under the parser-level ctx.Err() guard added in Round 5.
type cancellableProvider struct{}

func (cancellableProvider) Name() string                                         { return "cancellable" }
func (cancellableProvider) Slug() string                                         { return "cancellable" }
func (cancellableProvider) BrandColor() lipgloss.Color                           { return lipgloss.Color("#000000") }
func (cancellableProvider) ValidateKey(context.Context, string) error            { return nil }
func (cancellableProvider) ListModels(context.Context) ([]provider.Model, error) { return nil, nil }
func (cancellableProvider) Pricing(string) (float64, float64)                    { return 1.0, 5.0 }
func (cancellableProvider) ContextWindow(string) int                             { return 100_000 }
func (cancellableProvider) SupportsTools(string) bool                            { return true }

func (cancellableProvider) ChatCompletion(ctx context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 4)
	go func() {
		defer close(ch)
		// Drip a single text delta so the test can see the stream
		// actually started, then block on ctx.
		ch <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: "tick"}
		<-ctx.Done()
		ch <- provider.StreamEvent{Type: provider.EventError, Error: ctx.Err()}
	}()
	return ch, nil
}

// TestAgent_Run_CancelDuringChatCompletion drives a turn against a
// provider that blocks on ctx, cancels the ctx after the first delta,
// and asserts the events channel closes promptly with EventError whose
// cause is context.Canceled. This is the agent-level contract Round 5
// relies on: %w wrapping all the way through oneTurn / run.
func TestAgent_Run_CancelDuringChatCompletion(t *testing.T) {
	a, _, _ := newAgentRig(t, cancellableProvider{})

	ctx, cancel := context.WithCancel(context.Background())
	events := a.Run(ctx, "hang forever")

	// Read the first event to confirm streaming actually started, then
	// cancel.
	first, ok := <-events
	require.True(t, ok, "expected at least one event before cancel")
	assert.Equal(t, EventTextDelta, first.Type)
	cancel()

	deadline := time.After(200 * time.Millisecond)
	var lastMeaningful AgentEvent
	var sawCancelErr bool
	var channelClosed bool
drain:
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				channelClosed = true
				break drain
			}
			lastMeaningful = ev
			if ev.Type == EventError && ev.Error != nil && errors.Is(ev.Error, context.Canceled) {
				sawCancelErr = true
			}
		case <-deadline:
			break drain
		}
	}
	assert.True(t, channelClosed, "events channel must close within 200ms of cancel")
	assert.True(t, sawCancelErr, "last meaningful event should be EventError wrapping context.Canceled; got %+v", lastMeaningful)
}

// blockingApprover blocks Approve on ctx.Done() — i.e. it never returns
// of its own accord. Used to prove the agent unblocks the approver when
// Run's ctx is cancelled.
type blockingApprover struct {
	called int32
}

func (b *blockingApprover) Approve(ctx context.Context, _ ApprovalRequest) ApprovalDecision {
	atomic.AddInt32(&b.called, 1)
	<-ctx.Done()
	return ApprovalDecision{Approved: false, Reason: "cancelled"}
}

type cancelingTool struct {
	started  chan struct{}
	executed int32
}

func (c *cancelingTool) Name() string            { return "slow_tool" }
func (c *cancelingTool) Description() string     { return "test tool" }
func (c *cancelingTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (c *cancelingTool) RequiresApproval() bool  { return false }
func (c *cancelingTool) Execute(ctx context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	atomic.AddInt32(&c.executed, 1)
	close(c.started)
	<-ctx.Done()
	return tools.ToolResult{}, ctx.Err()
}

// TestAgent_Run_CancelDuringApproval drives a turn that reaches the
// approval gate and never resolves, then cancels the ctx. The agent
// should unblock the approver (via ctx), record the rejection, and
// close the events channel promptly.
func TestAgent_Run_CancelDuringApproval(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "danger"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 1, OutputTokens: 1}},
		},
	}}
	rt := &recordingTool{name: "danger", approval: true}
	app := &blockingApprover{}

	reg := provider.NewRegistry()
	reg.Register(prov)
	require.NoError(t, reg.SetActive("scripted", "scripted-model"))
	tr := tools.NewRegistry()
	tr.Register(rt)
	sm := session.NewManager(t.TempDir())
	_, _ = sm.New("scripted", "scripted-model")
	a := New(Config{
		Registry: reg,
		Tools:    tr,
		Session:  sm,
		Approver: app,
	})

	ctx, cancel := context.WithCancel(context.Background())
	events := a.Run(ctx, "be dangerous")

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Drain the channel; require close within 500ms of cancel.
	deadline := time.After(1 * time.Second)
	var channelClosed bool
drain:
	for {
		select {
		case _, ok := <-events:
			if !ok {
				channelClosed = true
				break drain
			}
		case <-deadline:
			break drain
		}
	}
	assert.True(t, channelClosed, "events channel must close once approval unblocks on cancel")
	assert.GreaterOrEqual(t, atomic.LoadInt32(&app.called), int32(1), "approver should have been invoked")
	assert.Equal(t, int32(0), atomic.LoadInt32(&rt.executed), "rejected-on-cancel tool must not execute")
}

func TestAgent_Run_CancelDuringTool(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "slow_tool"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 1, OutputTokens: 1}},
		},
	}}
	ct := &cancelingTool{started: make(chan struct{})}
	hookMarker := filepath.Join(t.TempDir(), "post-hook-ran")

	reg := provider.NewRegistry()
	reg.Register(prov)
	require.NoError(t, reg.SetActive("scripted", "scripted-model"))
	tr := tools.NewRegistry()
	tr.Register(ct)
	sm := session.NewManager(t.TempDir())
	_, _ = sm.New("scripted", "scripted-model")
	a := New(Config{
		Registry: reg,
		Tools:    tr,
		Session:  sm,
		Approver: AutoApprove(),
		Hooks: hooks.New(config.HooksConfig{
			PostToolUse: []config.HookConfig{{
				Matcher:    "slow_tool",
				Command:    touchCommand(hookMarker),
				TimeoutSec: 2,
			}},
		}, t.TempDir()),
	})

	ctx, cancel := context.WithCancel(context.Background())
	events := a.Run(ctx, "run slow tool")

	select {
	case <-ct.started:
	// Generous: this is only the barrier that gets us to the interesting part.
	// The assertions below bound how fast cancellation is observed; a tight
	// bound here just makes the test fail on a loaded machine before it runs.
	case <-time.After(5 * time.Second):
		t.Fatal("tool did not start")
	}
	cancel()

	deadline := time.After(1 * time.Second)
	var channelClosed bool
	var sawCancelErr bool
	var sawExecuted bool
drain:
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				channelClosed = true
				break drain
			}
			if ev.Type == EventError && ev.Error != nil && errors.Is(ev.Error, context.Canceled) {
				sawCancelErr = true
			}
			if ev.Type == EventToolCallExecuted {
				sawExecuted = true
			}
		case <-deadline:
			break drain
		}
	}

	assert.True(t, channelClosed, "events channel must close once tool observes cancel")
	assert.True(t, sawCancelErr, "tool cancellation should propagate as EventError(context.Canceled)")
	assert.False(t, sawExecuted, "cancelled tool must not be reported as a completed tool result")
	assert.Equal(t, int32(1), atomic.LoadInt32(&ct.executed), "tool should have started exactly once")
	assert.NoFileExists(t, hookMarker, "post-tool hook must not run after tool cancellation")

	cur := sm.Current()
	for _, m := range cur.Messages {
		if m.Role == provider.RoleTool {
			t.Fatalf("cancelled tool must not be saved as a tool-role failure message: %+v", m)
		}
	}
}

func touchCommand(path string) string {
	if runtime.GOOS == "windows" {
		return "New-Item -ItemType File -Path '" + strings.ReplaceAll(path, "'", "''") + "' -Force | Out-Null"
	}
	return "touch '" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

func TestContextManager_EstimateAndPercent(t *testing.T) {
	cm := NewContextManager(80)
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "hello world"},
		{Role: provider.RoleAssistant, Content: "hi"},
	}
	tokens := cm.EstimateTokens(msgs)
	assert.Greater(t, tokens, 0)
	assert.Less(t, tokens, 100)

	pct := cm.UsagePercent(msgs, 100)
	assert.GreaterOrEqual(t, pct, 0)
	assert.LessOrEqual(t, pct, 100)

	assert.Equal(t, 0, cm.UsagePercent(msgs, 0), "zero max → unknown → return 0")
}

func TestContextManager_ShouldSuggestCompact(t *testing.T) {
	cm := NewContextManager(50)
	long := make([]byte, 2000)
	for i := range long {
		long[i] = 'x'
	}
	msgs := []provider.Message{{Role: provider.RoleUser, Content: string(long)}}
	// The conservative estimator uses three bytes/token, so this crosses 50%.
	assert.True(t, cm.ShouldSuggestCompact(msgs, 1000))
	assert.False(t, cm.ShouldSuggestCompact(msgs, 10_000))
}

func TestContextManager_CompactPreservesSystemAndTail(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventTextDelta, TextDelta: "summary text"},
			{Type: provider.EventDone},
		},
	}}

	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "you are helpful"},
		{Role: provider.RoleUser, Content: "msg 1"},
		{Role: provider.RoleAssistant, Content: "reply 1"},
		{Role: provider.RoleUser, Content: "msg 2"},
		{Role: provider.RoleAssistant, Content: "reply 2"},
		{Role: provider.RoleUser, Content: "msg 3"},
		{Role: provider.RoleAssistant, Content: "reply 3"},
	}

	cm := NewContextManager(80)
	out, err := cm.Compact(context.Background(), prov, "scripted-model", msgs, 2)
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(out), 3)
	assert.Equal(t, provider.RoleSystem, out[0].Role)
	assert.Equal(t, "you are helpful", out[0].Content)
	assert.Contains(t, out[1].Content, "summary text")
	// Last two messages of the original input must be preserved verbatim.
	tail := out[len(out)-2:]
	assert.Equal(t, "msg 3", tail[0].Content)
	assert.Equal(t, "reply 3", tail[1].Content)
}

func TestContextManager_CompactTailStartingOnToolMessageKeepsGroup(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventTextDelta, TextDelta: "summary text"},
			{Type: provider.EventDone},
		},
	}}

	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "before"},
		{Role: provider.RoleAssistant, Content: "old reply"},
		{Role: provider.RoleUser, Content: "run both tools"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "call-a", Name: "alpha", Arguments: `{}`},
			{ID: "call-b", Name: "beta", Arguments: `{}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "call-a", Name: "alpha", Content: "alpha result"},
		{Role: provider.RoleTool, ToolCallID: "call-b", Name: "beta", Content: "beta result"},
		{Role: provider.RoleAssistant, Content: "done"},
	}

	cm := NewContextManager(80)
	out, err := cm.Compact(context.Background(), prov, "scripted-model", msgs, 2)
	require.NoError(t, err)

	require.Len(t, out, 5)
	assert.Contains(t, out[0].Content, "summary text")
	assert.Equal(t, provider.RoleAssistant, out[1].Role)
	require.Len(t, out[1].ToolCalls, 2)
	assert.Equal(t, provider.RoleTool, out[2].Role)
	assert.Equal(t, "call-a", out[2].ToolCallID)
	assert.Equal(t, provider.RoleTool, out[3].Role)
	assert.Equal(t, "call-b", out[3].ToolCallID)
	assert.Equal(t, "done", out[4].Content)
}

func TestAgentBuildMessagesKeepsToolProjectionImmutableAcrossTurns(t *testing.T) {
	sm := session.NewManager(t.TempDir())
	_, err := sm.New("scripted", "scripted-model")
	require.NoError(t, err)
	large := strings.Repeat("old", 30000)
	require.NoError(t, sm.AddMessage(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "old", Name: "run", Arguments: `{}`}}}))
	require.NoError(t, sm.AddMessage(provider.Message{Role: provider.RoleTool, ToolCallID: "old", Name: "run", Content: large}))

	a := New(Config{Session: sm})
	first := a.buildMessages()
	require.Len(t, first, 2)
	projected := first[1].Content
	assert.Contains(t, projected, "tool result truncated")

	require.NoError(t, sm.AddMessage(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "new", Name: "run", Arguments: `{}`}}}))
	require.NoError(t, sm.AddMessage(provider.Message{Role: provider.RoleTool, ToolCallID: "new", Name: "run", Content: "recent"}))
	second := a.buildMessages()
	require.Len(t, second, 4)
	assert.Equal(t, projected, second[1].Content, "an older result's model-facing bytes must not change")

	stored := sm.Current()
	require.NotNil(t, stored)
	assert.Equal(t, large, stored.Messages[1].Content, "full local/UI result must remain available")
	assert.Equal(t, projected, stored.Messages[1].ModelContent)
}

func TestContextManagerCompactUsesProjectionButPreservesFullTail(t *testing.T) {
	sm := session.NewManager(t.TempDir())
	_, err := sm.New("scripted", "scripted-model")
	require.NoError(t, err)
	large := strings.Repeat("head", 12000) + "SECRET_MIDDLE_SENTINEL" + strings.Repeat("tail", 12000)
	require.NoError(t, sm.AddMessage(provider.Message{Role: provider.RoleUser, Content: "run the tool"}))
	require.NoError(t, sm.AddMessage(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "run", Arguments: `{}`}}}))
	require.NoError(t, sm.AddMessage(provider.Message{Role: provider.RoleTool, ToolCallID: "call-1", Name: "run", Content: large}))
	require.NoError(t, sm.AddMessage(provider.Message{Role: provider.RoleAssistant, Content: "tool complete"}))
	require.NoError(t, sm.AddMessage(provider.Message{Role: provider.RoleUser, Content: "next"}))

	prov := &scriptedProvider{turns: [][]provider.StreamEvent{{
		{Type: provider.EventTextDelta, TextDelta: "summary"},
		{Type: provider.EventDone},
	}}}
	_, _, err = NewContextManager(80).CompactWithUsage(context.Background(), prov, "scripted-model", sm.Current().Messages, 1)
	require.NoError(t, err)
	require.NotEmpty(t, prov.lastRequest.Messages)
	summaryPrompt := prov.lastRequest.Messages[0].Content
	assert.Contains(t, summaryPrompt, "tool result truncated")
	assert.NotContains(t, summaryPrompt, "SECRET_MIDDLE_SENTINEL")

	// Preparing the model summary must not alter the authoritative local/UI
	// transcript while it creates the projected prompt.
	assert.Equal(t, large, sm.Current().Messages[2].Content)
}

func TestAgent_BuildMessagesNormalizesSplitToolCallGroups(t *testing.T) {
	sm := session.NewManager(t.TempDir())
	_, err := sm.New("scripted", "scripted-model")
	require.NoError(t, err)
	require.NoError(t, sm.ReplaceMessages([]provider.Message{
		{Role: provider.RoleUser, Content: "before"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
			{ID: "call-a", Name: "alpha", Arguments: `{}`},
			{ID: "call-b", Name: "beta", Arguments: `{}`},
		}},
		{Role: provider.RoleTool, ToolCallID: "call-a", Name: "alpha", Content: "alpha result"},
		{Role: provider.RoleUser, Content: "interrupted"},
		{Role: provider.RoleTool, ToolCallID: "call-b", Name: "beta", Content: "beta result"},
		{Role: provider.RoleAssistant, Content: "after"},
	}))

	a := New(Config{Session: sm})
	msgs := a.buildMessages()

	require.Len(t, msgs, 3)
	assert.Equal(t, "before", msgs[0].Content)
	assert.Equal(t, "interrupted", msgs[1].Content)
	assert.Equal(t, "after", msgs[2].Content)
	for _, msg := range msgs {
		assert.NotEqual(t, provider.RoleTool, msg.Role)
		assert.Empty(t, msg.ToolCalls)
	}
}

// Interleaved-thinking models (MiniMax M2.x/M3) reason between tool calls and
// require their own chain to be replayed on the next request; stripping it
// degrades multi-turn tool use. The visible content of a tool-calling turn is
// deliberately dropped, so reasoning has to be persisted in its own field and
// carried into the follow-up request.
func TestAgent_PersistsReasoningAcrossToolCall(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventReasoningDelta, TextDelta: "I need to "},
			{Type: provider.EventReasoningDelta, TextDelta: "run the tool"},
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "do_thing"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventDone},
		},
		{
			{Type: provider.EventTextDelta, TextDelta: "All done"},
			{Type: provider.EventDone},
		},
	}}
	rt := &recordingTool{name: "do_thing", approval: false}
	a, sm, _ := newAgentRig(t, prov, rt)

	collect(a.Run(context.Background(), "do the thing"))

	cur := sm.Current()
	require.GreaterOrEqual(t, len(cur.Messages), 2)
	assistant := cur.Messages[1]
	require.Equal(t, provider.RoleAssistant, assistant.Role)
	require.Len(t, assistant.ToolCalls, 1)
	assert.Equal(t, "I need to run the tool", assistant.Reasoning,
		"reasoning must be persisted on a tool-calling turn, not discarded with the content")

	// The follow-up request must carry it back to the model.
	var replayed string
	for _, m := range prov.lastRequest.Messages {
		if m.Role == provider.RoleAssistant && m.Reasoning != "" {
			replayed = m.Reasoning
		}
	}
	assert.Equal(t, "I need to run the tool", replayed,
		"the reasoning chain must be replayed on the next request")
}

// Reasoning must never be folded into the assistant's visible text.
func TestAgent_ReasoningIsNotVisibleContent(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventReasoningDelta, TextDelta: "thinking out loud"},
			{Type: provider.EventTextDelta, TextDelta: "Here is the answer."},
			{Type: provider.EventDone},
		},
	}}
	a, sm, _ := newAgentRig(t, prov)

	collect(a.Run(context.Background(), "question"))

	cur := sm.Current()
	require.Len(t, cur.Messages, 2)
	assert.Equal(t, "Here is the answer.", cur.Messages[1].Content)
	assert.Equal(t, "thinking out loud", cur.Messages[1].Reasoning)
}

// A deny rule blocks a tool that does not require approval.
//
// This is the property that made the unused ToolDecider seam look load-bearing:
// if the policy were only consulted on the approval path, a deny rule could not
// reach a tool whose RequiresApproval is false, and the agent would run it. The
// agent consults the policy itself, before and independently of approval, so it
// does — and removing the seam took nothing with it.
func TestAgent_PolicyDeniesAToolThatDoesNotRequireApproval(t *testing.T) {
	prov := &scriptedProvider{turns: [][]provider.StreamEvent{{
		{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "read", ArgumentsDelta: `{}`}},
		{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
		{Type: provider.EventDone},
	}, {
		{Type: provider.EventTextDelta, TextDelta: "ok"},
		{Type: provider.EventDone},
	}}}
	// approval: false — nothing would ever prompt for this tool.
	tool := &recordingTool{name: "read", approval: false}
	a, _, _ := newAgentRig(t, prov, tool)
	// An approver that would wave anything through, so a pass could only come
	// from the policy being skipped rather than from the approver agreeing.
	a.SetApprover(AutoApprove())
	a.SetPolicy(permissions.DefaultPolicy().WithRule("read", permissions.DecisionDeny))

	events := collect(a.Run(context.Background(), "read the file"))

	if got := atomic.LoadInt32(&tool.executed); got != 0 {
		t.Fatalf("a denied tool ran %d times; the policy was not consulted for a "+
			"tool that does not require approval", got)
	}
	rejected := false
	for _, ev := range events {
		if ev.Type == EventToolCallRejected {
			rejected = true
			if !strings.Contains(ev.Text, "permission denied") {
				t.Fatalf("rejection does not name the policy: %q", ev.Text)
			}
		}
	}
	if !rejected {
		t.Fatal("no rejection event was emitted for a denied tool")
	}
}
