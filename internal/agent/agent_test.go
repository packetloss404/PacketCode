package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/provider"
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
	turns       [][]provider.StreamEvent
	turnIdx     int32
	chatCount   int32
	lastRequest provider.ChatRequest
}

func (s *scriptedProvider) Name() string                                                 { return "scripted" }
func (s *scriptedProvider) Slug() string                                                 { return "scripted" }
func (s *scriptedProvider) BrandColor() lipgloss.Color                                   { return lipgloss.Color("#000000") }
func (s *scriptedProvider) ValidateKey(_ context.Context, _ string) error                { return nil }
func (s *scriptedProvider) ListModels(_ context.Context) ([]provider.Model, error)       { return nil, nil }
func (s *scriptedProvider) Pricing(string) (float64, float64)                            { return 1.0, 5.0 }
func (s *scriptedProvider) ContextWindow(string) int                                     { return 100_000 }
func (s *scriptedProvider) SupportsTools(string) bool                                    { return true }

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

func (r *recordingTool) Name() string             { return r.name }
func (r *recordingTool) Description() string      { return "test tool" }
func (r *recordingTool) Schema() json.RawMessage  { return json.RawMessage(`{"type":"object"}`) }
func (r *recordingTool) RequiresApproval() bool   { return r.approval }
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
	// 2000 chars / 4 = ~500 tokens; in a 1000-token window that's 50%.
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
