package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/tools"
)

// repeatingProvider proposes the same tool call on every turn — the shape of a
// model stuck on a call that never succeeds. argsFor lets a test vary the
// arguments the model asks for; stopAfter turns the stream into a plain text
// answer once enough tool turns have gone by, which is how the
// legitimate-repeat tests reach a clean end.
type repeatingProvider struct {
	toolName  string
	argsFor   func(turn int) string
	stopAfter int
	chatCount int32
}

func (p *repeatingProvider) Name() string                              { return "repeating" }
func (p *repeatingProvider) Slug() string                              { return "repeating" }
func (p *repeatingProvider) BrandColor() lipgloss.Color                { return lipgloss.Color("#000000") }
func (p *repeatingProvider) ValidateKey(context.Context, string) error { return nil }
func (p *repeatingProvider) Pricing(string) (float64, float64)         { return 0, 0 }
func (p *repeatingProvider) ContextWindow(string) int                  { return 100_000 }
func (p *repeatingProvider) SupportsTools(string) bool                 { return true }
func (p *repeatingProvider) ListModels(context.Context) ([]provider.Model, error) {
	return nil, nil
}

func (p *repeatingProvider) ChatCompletion(_ context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	turn := int(atomic.AddInt32(&p.chatCount, 1))
	ch := make(chan provider.StreamEvent, 4)
	if p.stopAfter > 0 && turn > p.stopAfter {
		ch <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: "finished"}
		ch <- provider.StreamEvent{Type: provider.EventDone}
		close(ch)
		return ch, nil
	}
	args := `{"path":"missing.txt"}`
	if p.argsFor != nil {
		args = p.argsFor(turn)
	}
	ch <- provider.StreamEvent{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: fmt.Sprintf("c%d", turn), Name: p.toolName}}
	ch <- provider.StreamEvent{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: args}}
	ch <- provider.StreamEvent{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}}
	ch <- provider.StreamEvent{Type: provider.EventDone}
	close(ch)
	return ch, nil
}

// progressingTool answers differently every call — a poll watching a file a
// build is still writing.
type progressingTool struct {
	name  string
	calls int32
}

func (p *progressingTool) Name() string            { return p.name }
func (p *progressingTool) Description() string     { return "progressing test tool" }
func (p *progressingTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (p *progressingTool) RequiresApproval() bool  { return false }
func (p *progressingTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	n := atomic.AddInt32(&p.calls, 1)
	return tools.ToolResult{Content: fmt.Sprintf("%d bytes written so far", n)}, nil
}

// rewritingApprover approves everything but replaces the arguments, standing in
// for a user who edits a proposed call before letting it run.
type rewritingApprover struct{ params string }

func (r rewritingApprover) Approve(context.Context, ApprovalRequest) ApprovalDecision {
	return ApprovalDecision{Approved: true, EditedParams: json.RawMessage(r.params)}
}

func errorFrom(events []AgentEvent) error {
	for _, ev := range events {
		if ev.Type == EventError {
			return ev.Error
		}
	}
	return nil
}

func TestAgent_AbortsIdenticalRepeatedToolCall(t *testing.T) {
	prov := &repeatingProvider{toolName: "read_file"}
	rt := &recordingTool{name: "read_file", result: tools.ToolResult{Content: "no such file", IsError: true}}
	a, _, _ := newAgentRig(t, prov, rt)

	evs := collect(a.Run(context.Background(), "read the file"))

	err := errorFrom(evs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loop detected")
	assert.Contains(t, err.Error(), "read_file", "the abort must name the repeated call")
	assert.NotContains(t, err.Error(), "exceeded")
	// Threshold 5 means the sixth identical turn trips it — far short of the
	// 25 iterations the old cap would have burned.
	assert.Equal(t, int32(6), atomic.LoadInt32(&prov.chatCount))
	assert.Equal(t, int32(6), atomic.LoadInt32(&rt.executed))
}

func TestAgent_RepeatedCallWithChangingOutputIsNotALoop(t *testing.T) {
	// The regression that would make this feature hated: an identical call is
	// legitimate for as long as its answer keeps changing.
	prov := &repeatingProvider{toolName: "read_file", stopAfter: 12}
	pt := &progressingTool{name: "read_file"}
	a, _, _ := newAgentRig(t, prov, pt)

	evs := collect(a.Run(context.Background(), "watch the build"))

	require.NoError(t, errorFrom(evs))
	assert.Equal(t, int32(12), atomic.LoadInt32(&pt.calls))
	assert.Equal(t, EventDone, evs[len(evs)-1].Type)
}

func TestAgent_LoopDetectionSignsExecutedArguments(t *testing.T) {
	// The model varies its arguments every turn, so the pre-approval view looks
	// like fresh work; the approver rewrites them all to the same thing. Signing
	// what actually ran is what makes the loop visible.
	prov := &repeatingProvider{
		toolName: "write_file",
		argsFor:  func(turn int) string { return fmt.Sprintf(`{"attempt":%d}`, turn) },
	}
	rt := &recordingTool{name: "write_file", approval: true, result: tools.ToolResult{Content: "written"}}
	a, _, _ := newAgentRig(t, prov, rt)
	a.SetApprover(rewritingApprover{params: `{"attempt":0}`})

	err := errorFrom(collect(a.Run(context.Background(), "write it")))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "loop detected")
	assert.Equal(t, int32(6), atomic.LoadInt32(&rt.executed))
}

func TestAgent_LoopDetectionDisabledRunsToTheIterationCap(t *testing.T) {
	prov := &repeatingProvider{toolName: "read_file"}
	rt := &recordingTool{name: "read_file"}
	a, _, _ := newAgentRig(t, prov, rt)
	a.loopDetection = LoopDetectionConfig{Disabled: true}

	err := errorFrom(collect(a.Run(context.Background(), "read the file")))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeded 25 tool iterations")
}

func TestAgent_LoopWindowIsPerRun(t *testing.T) {
	// Two runs of the same stuck call: the second must start from an empty
	// window, or a fresh user message would inherit the previous one's history.
	prov := &repeatingProvider{toolName: "read_file"}
	rt := &recordingTool{name: "read_file", result: tools.ToolResult{Content: "no such file"}}
	a, _, _ := newAgentRig(t, prov, rt)

	require.Error(t, errorFrom(collect(a.Run(context.Background(), "first"))))
	assert.Equal(t, int32(6), atomic.LoadInt32(&rt.executed))

	require.Error(t, errorFrom(collect(a.Run(context.Background(), "second"))))
	assert.Equal(t, int32(12), atomic.LoadInt32(&rt.executed), "second run must get its own full window")
}

func TestLoopDetector_ToolFreeTurnsAreSkipped(t *testing.T) {
	d := newLoopDetector(LoopDetectionConfig{})
	stuck := []toolObservation{{name: "read_file", arguments: `{"p":1}`, content: "missing"}}

	for i := 0; i < defaultLoopThreshold; i++ {
		require.NoError(t, d.observe(stuck))
		require.NoError(t, d.observe(nil), "a turn with no tool calls carries no signal")
	}
	require.Len(t, d.recent, defaultLoopThreshold)
	assert.Error(t, d.observe(stuck))
}

func TestLoopDetector_RepeatsAgeOutOfTheWindow(t *testing.T) {
	d := newLoopDetector(LoopDetectionConfig{})
	stuck := []toolObservation{{name: "read_file", arguments: `{"p":1}`, content: "missing"}}

	for i := 0; i < defaultLoopThreshold; i++ {
		require.NoError(t, d.observe(stuck))
	}
	for i := 0; i < defaultLoopWindowTurns-defaultLoopThreshold; i++ {
		require.NoError(t, d.observe([]toolObservation{{name: "read_file", arguments: fmt.Sprintf(`{"p":%d}`, i), content: "ok"}}))
	}
	// The oldest repeat has slid out of view, so this one is only the fifth.
	assert.NoError(t, d.observe(stuck))
}

func TestLoopDetector_ParallelCallOrderDoesNotMatter(t *testing.T) {
	forward := []toolObservation{
		{name: "read_file", arguments: `{"p":1}`, content: "x"},
		{name: "grep", arguments: `{"q":"z"}`, content: "y"},
	}
	reversed := []toolObservation{forward[1], forward[0]}
	assert.Equal(t, fingerprintTurn(forward).signature, fingerprintTurn(reversed).signature)
}

func TestLoopDetector_FieldBoundariesAreUnambiguous(t *testing.T) {
	// Without the length prefixes these two turns would hash identically.
	left := []toolObservation{{name: "read", arguments: "file", content: "x"}}
	right := []toolObservation{{name: "readfile", arguments: "", content: "x"}}
	assert.NotEqual(t, fingerprintTurn(left).signature, fingerprintTurn(right).signature)
}

func TestLoopDetector_ThresholdIsConfigurable(t *testing.T) {
	d := newLoopDetector(LoopDetectionConfig{WindowTurns: 4, Threshold: 2})
	stuck := []toolObservation{{name: "read_file", arguments: `{}`, content: "missing"}}

	require.NoError(t, d.observe(stuck))
	require.NoError(t, d.observe(stuck))
	assert.Error(t, d.observe(stuck))
}
