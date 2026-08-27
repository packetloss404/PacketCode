package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/sugar"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
)

type recordingRuntimeHooks struct {
	starts    []sugar.RuntimeRunStart
	events    []sugar.RuntimeEvent
	continues int
	startErr  error
}

func (r *recordingRuntimeHooks) StartRun(_ context.Context, start sugar.RuntimeRunStart) (*sugar.RuntimeRunResponse, error) {
	r.starts = append(r.starts, start)
	if r.startErr != nil {
		return nil, r.startErr
	}
	return &sugar.RuntimeRunResponse{Run: sugar.RuntimeRun{ID: "run_test"}}, nil
}
func (r *recordingRuntimeHooks) EmitEvent(_ context.Context, event sugar.RuntimeEvent) (*sugar.RuntimeEventResponse, error) {
	r.events = append(r.events, event)
	return &sugar.RuntimeEventResponse{Event: sugar.RuntimeEventResult{Seq: event.Seq}}, nil
}
func (r *recordingRuntimeHooks) Continue(context.Context, string, string) (*sugar.RuntimeContinueResponse, error) {
	r.continues++
	return &sugar.RuntimeContinueResponse{Decision: sugar.RuntimeDecision{
		Action: "escalate", WouldEscalate: true, Tier: "frontier", PredictedModel: "some-other-model", ReasonCodes: []string{"validation_failure"},
	}}, nil
}

type conduitTestProvider struct {
	hooks    sugar.RuntimeHooks
	turns    [][]provider.StreamEvent
	requests []provider.ChatRequest
}

func (p *conduitTestProvider) Name() string                                         { return "Sugar test" }
func (p *conduitTestProvider) Slug() string                                         { return sugar.Slug }
func (p *conduitTestProvider) BrandColor() lipgloss.Color                           { return lipgloss.Color("#000") }
func (p *conduitTestProvider) ValidateKey(context.Context, string) error            { return nil }
func (p *conduitTestProvider) ListModels(context.Context) ([]provider.Model, error) { return nil, nil }
func (p *conduitTestProvider) Pricing(string) (float64, float64)                    { return 0, 0 }
func (p *conduitTestProvider) ContextWindow(string) int                             { return 200_000 }
func (p *conduitTestProvider) SupportsTools(string) bool                            { return true }
func (p *conduitTestProvider) RuntimeHooks() sugar.RuntimeHooks                     { return p.hooks }
func (p *conduitTestProvider) ChatCompletion(_ context.Context, request provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	p.requests = append(p.requests, request)
	idx := len(p.requests) - 1
	if idx >= len(p.turns) {
		return nil, errors.New("no scripted turn")
	}
	stream := make(chan provider.StreamEvent, len(p.turns[idx]))
	for _, event := range p.turns[idx] {
		stream <- event
	}
	close(stream)
	return stream, nil
}

func TestConduitShadowLifecycleIsContentFreeAndNeverChangesLiveModel(t *testing.T) {
	hooks := &recordingRuntimeHooks{}
	prov := &conduitTestProvider{hooks: hooks, turns: [][]provider.StreamEvent{
		{
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "c1", Name: "execute_command"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{"command":"go test ./..."}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}},
			{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 1, ID: "c2", Name: "execute_command"}},
			{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 1, ArgumentsDelta: `{"command":"go build ./..."}`}},
			{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 1}},
			{Type: provider.EventDone},
		},
		{{Type: provider.EventTextDelta, TextDelta: "done"}, {Type: provider.EventDone}},
	}}
	tool := &recordingTool{name: "execute_command", result: tools.ToolResult{
		Content:  "API_KEY=super-secret\n--- FAIL: TestThing expected 1 got 2",
		IsError:  true,
		Metadata: map[string]any{"exit_code": 1},
	}}
	registry := provider.NewRegistry()
	registry.Register(prov)
	require.NoError(t, registry.SetActive(sugar.Slug, sugar.DefaultModel))
	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(tool)
	sessions := session.NewManager(t.TempDir())
	_, err := sessions.New(sugar.Slug, sugar.DefaultModel)
	require.NoError(t, err)
	require.NoError(t, sessions.ReplaceMessagesAfterCompaction(nil))
	tracker, err := cost.NewTracker(filepath.Join(t.TempDir(), "tally.json"), nil)
	require.NoError(t, err)
	runner := New(Config{
		Registry: registry, Tools: toolRegistry, Session: sessions, CostTracker: tracker, Approver: AutoApprove(),
		ConduitShadow: ConduitShadowConfig{Enabled: true, Timeout: time.Second, CapsuleMaxBytes: 4096},
		SugarCache:    SugarCacheConfig{Mode: provider.SugarCacheOff, Retention: provider.SugarCache5Minutes, Privacy: provider.SugarPrivacyZDRRequired},
	})

	events := collect(runner.Run(context.Background(), "Fix tests without changing the live model"))
	for _, event := range events {
		require.NotEqual(t, EventError, event.Type, event.Error)
	}
	require.Len(t, hooks.starts, 1, "one shadow run per user turn")
	require.Len(t, hooks.events, 2)
	assert.Equal(t, 1, hooks.events[0].Seq)
	assert.Equal(t, 2, hooks.events[1].Seq)
	assert.Equal(t, sugar.RuntimeValidation, hooks.events[0].Type)
	assert.Equal(t, sugar.RuntimeToolTest, hooks.events[0].ToolCategory)
	assert.Equal(t, sugar.RuntimeToolBuild, hooks.events[1].ToolCategory)
	assert.Equal(t, 2, hooks.continues)
	for _, request := range prov.requests {
		assert.Equal(t, sugar.DefaultModel, request.Model, "shadow recommendation must not alter the live request")
		assert.Nil(t, request.SugarCache, "cache mode off must avoid metadata and fingerprint work")
	}
	_, activeModel := registry.Active()
	assert.Equal(t, sugar.DefaultModel, activeModel)

	wire, err := json.Marshal(hooks.events)
	require.NoError(t, err)
	for _, forbidden := range []string{"content", "capsule", "arguments", "command", "super-secret", "go test"} {
		assert.NotContains(t, strings.ToLower(string(wire)), forbidden)
	}
	capsule := sessions.Current().SpecialistCapsule
	require.NotNil(t, capsule)
	assert.Equal(t, 1, capsule.Generation)
	require.NotEmpty(t, capsule.FailedGates)
	capsuleJSON, err := json.Marshal(capsule)
	require.NoError(t, err)
	assert.NotContains(t, string(capsuleJSON), "super-secret")
	assert.Contains(t, string(capsuleJSON), "shadow recommendation")
}

func TestConduitShadowDisabledOrUnavailablePreservesNormalRun(t *testing.T) {
	for _, test := range []struct {
		name       string
		enabled    bool
		startErr   error
		wantStarts int
	}{
		{name: "disabled", enabled: false, wantStarts: 0},
		{name: "unavailable", enabled: true, startErr: errors.New("endpoint unavailable"), wantStarts: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			hooks := &recordingRuntimeHooks{startErr: test.startErr}
			prov := &conduitTestProvider{hooks: hooks, turns: [][]provider.StreamEvent{{{Type: provider.EventTextDelta, TextDelta: "ok"}, {Type: provider.EventDone}}}}
			registry := provider.NewRegistry()
			registry.Register(prov)
			require.NoError(t, registry.SetActive(sugar.Slug, sugar.DefaultModel))
			sessions := session.NewManager(t.TempDir())
			_, err := sessions.New(sugar.Slug, sugar.DefaultModel)
			require.NoError(t, err)
			runner := New(Config{Registry: registry, Tools: tools.NewRegistry(), Session: sessions, Approver: AutoApprove(), ConduitShadow: ConduitShadowConfig{Enabled: test.enabled, Timeout: time.Second}})
			events := collect(runner.Run(context.Background(), "hello"))
			for _, event := range events {
				require.NotEqual(t, EventError, event.Type, event.Error)
			}
			assert.Len(t, hooks.starts, test.wantStarts)
			assert.Empty(t, hooks.events)
			assert.Equal(t, sugar.DefaultModel, prov.requests[0].Model)
			assert.Nil(t, prov.requests[0].SugarCache, "disabled cache must do no Sugar metadata work")
		})
	}
}

// classifyTool keys off tool names as strings, so a name that drifts from the
// registry fails silently — the call lands in RuntimeToolOther and the shadow
// record reports the turn as touching no files. Pin the names against the
// registry rather than against a hand-copied list.
func TestClassifyToolMatchesRegisteredToolNames(t *testing.T) {
	root := t.TempDir()
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(root))
	registry.Register(tools.NewSearchCodebaseTool(root))
	registry.Register(tools.NewListDirectoryTool(root))
	registry.Register(tools.NewWriteFileTool(root, nil))
	registry.Register(tools.NewPatchFileTool(root, nil))

	for _, registered := range registry.All() {
		name := registered.Name()
		t.Run(name, func(t *testing.T) {
			got := classifyTool(provider.ToolCall{Name: name, Arguments: "{}"})
			assert.Equal(t, sugar.RuntimeToolFile, got,
				"%s is a registered file tool but classifies as %s", name, got)
		})
	}
}

func TestClassifyToolCategorisesShellCommands(t *testing.T) {
	call := func(command string) provider.ToolCall {
		args, err := json.Marshal(map[string]string{"command": command})
		require.NoError(t, err)
		return provider.ToolCall{Name: "execute_command", Arguments: string(args)}
	}
	assert.Equal(t, sugar.RuntimeToolTest, classifyTool(call("go test ./...")))
	assert.Equal(t, sugar.RuntimeToolBuild, classifyTool(call("go build ./...")))
	assert.Equal(t, sugar.RuntimeToolTypecheck, classifyTool(call("go vet ./...")))
	assert.Equal(t, sugar.RuntimeToolShell, classifyTool(call("git status")))
	assert.Equal(t, sugar.RuntimeToolOther, classifyTool(provider.ToolCall{Name: "spawn_agent", Arguments: "{}"}))
}

// blocked() hashes into s.salt, so it must lead with the same inactive-shadow
// guard its siblings use rather than doing work an inactive run discards.
func TestConduitShadowBlockedIsInertWhenInactive(t *testing.T) {
	state := &conduitShadowState{}
	state.blocked(context.Background(), provider.ToolCall{Name: "write_file"}, "denied")
	assert.Empty(t, state.capsule.Evidence)
	assert.Zero(t, state.seq)
}
