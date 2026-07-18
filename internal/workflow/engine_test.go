package workflow

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/jobs"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/tools"
)

// fakeProvider is a minimal streaming provider for engine tests. Each
// ChatCompletion emits a short text reply then EventDone — unless the request
// prompt contains "FAIL", in which case it emits EventError (which the jobs
// worker turns into StateFailed). When holdOpen is set it blocks until the
// context is cancelled, so tests can exercise cancellation.
type fakeProvider struct {
	holdOpen bool
	spawns   int32
}

func (f *fakeProvider) Name() string                                  { return "fake" }
func (f *fakeProvider) Slug() string                                  { return "fake" }
func (f *fakeProvider) BrandColor() lipgloss.Color                    { return lipgloss.Color("#000000") }
func (f *fakeProvider) ValidateKey(_ context.Context, _ string) error { return nil }
func (f *fakeProvider) ListModels(_ context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "fake-model"}}, nil
}
func (f *fakeProvider) Pricing(string) (float64, float64) { return 1.0, 5.0 }
func (f *fakeProvider) ContextWindow(string) int          { return 100_000 }
func (f *fakeProvider) SupportsTools(string) bool         { return true }

func (f *fakeProvider) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	atomic.AddInt32(&f.spawns, 1)
	prompt := lastUserContent(req)
	ch := make(chan provider.StreamEvent, 4)
	go func() {
		defer close(ch)
		if f.holdOpen {
			select {
			case ch <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: "(running)"}:
			case <-ctx.Done():
				return
			}
			<-ctx.Done()
			return
		}
		if strings.Contains(prompt, "FAIL") {
			ch <- provider.StreamEvent{Type: provider.EventError, Error: errors.New("scripted failure")}
			return
		}
		ch <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: "summary for: " + prompt}
		ch <- provider.StreamEvent{Type: provider.EventDone, Usage: &provider.Usage{InputTokens: 5, OutputTokens: 2}}
	}()
	return ch, nil
}

func lastUserContent(req provider.ChatRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == provider.RoleUser {
			return req.Messages[i].Content
		}
	}
	if len(req.Messages) > 0 {
		return req.Messages[len(req.Messages)-1].Content
	}
	return ""
}

func newTestManager(t *testing.T, prov provider.Provider, opts ...func(*jobs.Config)) *jobs.Manager {
	t.Helper()
	reg := provider.NewRegistry()
	reg.Register(prov)
	require.NoError(t, reg.SetActive(prov.Slug(), "fake-model"))

	tally := filepath.Join(t.TempDir(), "tally.json")
	tr, err := cost.NewTracker(tally, func(string, string) (float64, float64) { return 1.0, 5.0 })
	require.NoError(t, err)

	cfg := jobs.Config{
		Registry:      reg,
		Tools:         tools.NewRegistry(),
		SessionsDir:   t.TempDir(),
		BackupsDir:    t.TempDir(),
		JobsDir:       t.TempDir(),
		CostTracker:   tr,
		PricingFor:    func(string, string) (float64, float64) { return 1.0, 5.0 },
		MaxConcurrent: 8,
		MaxDepth:      2,
		MaxTotal:      64,
		Approver:      agent.AutoApprove(),
		Root:          t.TempDir(),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	mgr, _, err := jobs.NewManager(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Shutdown(2 * time.Second) })
	return mgr
}

func waitRun(t *testing.T, e *Engine, id string, want RunState, timeout time.Duration) RunSnapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if snap, ok := e.Get(id); ok && snap.State == want {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	snap, _ := e.Get(id)
	t.Fatalf("run %s did not reach %s within %s (state=%s err=%s)", id, want, timeout, snap.State, snap.Err)
	return RunSnapshot{}
}

// Test: a parallel step spawns N agents and joins them into StepResult.Agents.
func TestEngine_ParallelFanOut(t *testing.T) {
	prov := &fakeProvider{}
	mgr := newTestManager(t, prov)
	e := NewEngine(mgr)

	wf := Workflow{
		Name: "fan",
		Phases: []Phase{{
			Name: "p1",
			Steps: []Step{{
				Name:   "review",
				Mode:   StepParallel,
				Bind:   "review",
				FanOut: []string{"a", "b", "c"},
				Agent:  AgentSpec{Prompt: "review {{.item}}"},
			}},
		}},
	}
	run, err := e.Start(context.Background(), wf)
	require.NoError(t, err)

	snap := waitRun(t, e, run.ID, RunCompleted, 5*time.Second)
	require.Len(t, snap.Phases, 1)
	require.Len(t, snap.Phases[0].Steps, 1)
	require.Len(t, snap.Phases[0].Steps[0].Agents, 3, "3 fan-out agents")
	require.EqualValues(t, 3, atomic.LoadInt32(&prov.spawns))
}

// Test: sequential phases run in order, and a later step sees the prior step's
// summaries via {{.steps.<bind>}}.
func TestEngine_SequentialPhasesAndBinding(t *testing.T) {
	prov := &fakeProvider{}
	mgr := newTestManager(t, prov)
	e := NewEngine(mgr)

	wf := Workflow{
		Name:   "seq",
		Inputs: map[string]string{"topic": "widgets"},
		Phases: []Phase{
			{Name: "gather", Steps: []Step{{
				Name:  "gather",
				Mode:  StepSingle,
				Bind:  "gather",
				Agent: AgentSpec{Prompt: "gather about {{.inputs.topic}}"},
			}}},
			{Name: "synth", Steps: []Step{{
				Name:  "synth",
				Mode:  StepSingle,
				Agent: AgentSpec{Prompt: "synthesize: {{.steps.gather}}"},
			}}},
		},
	}
	run, err := e.Start(context.Background(), wf)
	require.NoError(t, err)

	snap := waitRun(t, e, run.ID, RunCompleted, 5*time.Second)
	require.Len(t, snap.Phases, 2)

	// The synth agent's prompt should have embedded the gather summary.
	synthAgents := snap.Phases[1].Steps[0].Agents
	require.Len(t, synthAgents, 1)
	require.Contains(t, synthAgents[0].Job.Prompt, "synthesize:")
	require.Contains(t, synthAgents[0].Job.Prompt, "summary for: gather about widgets")
}

// Test: a failing agent fails the run (fail-fast) and later phases don't run.
func TestEngine_FailFast(t *testing.T) {
	prov := &fakeProvider{}
	mgr := newTestManager(t, prov)
	e := NewEngine(mgr)

	wf := Workflow{
		Name: "failing",
		Phases: []Phase{
			{Name: "p1", Steps: []Step{{
				Name:  "boom",
				Mode:  StepSingle,
				Agent: AgentSpec{Prompt: "please FAIL now"},
			}}},
			{Name: "p2", Steps: []Step{{
				Name:  "never",
				Mode:  StepSingle,
				Agent: AgentSpec{Prompt: "should not run"},
			}}},
		},
	}
	run, err := e.Start(context.Background(), wf)
	require.NoError(t, err)

	snap := waitRun(t, e, run.ID, RunFailed, 5*time.Second)
	require.NotEmpty(t, snap.Err)
	// The second phase's step should have spawned no agents.
	require.Empty(t, snap.Phases[1].Steps[0].Agents, "fail-fast must skip later phases")
	require.EqualValues(t, 1, atomic.LoadInt32(&prov.spawns))
}

// Test: ContinueOnError lets the run proceed past a failing step.
func TestEngine_ContinueOnError(t *testing.T) {
	prov := &fakeProvider{}
	mgr := newTestManager(t, prov)
	e := NewEngine(mgr)

	wf := Workflow{
		Name: "resilient",
		Phases: []Phase{{
			Name:            "p1",
			ContinueOnError: true,
			Steps: []Step{
				{Name: "boom", Mode: StepSingle, Agent: AgentSpec{Prompt: "FAIL here"}},
				{Name: "ok", Mode: StepSingle, Agent: AgentSpec{Prompt: "this one works"}},
			},
		}},
	}
	run, err := e.Start(context.Background(), wf)
	require.NoError(t, err)

	// The run still fails overall (a step errored) but both steps ran.
	snap := waitRun(t, e, run.ID, RunFailed, 5*time.Second)
	require.Len(t, snap.Phases[0].Steps[1].Agents, 1, "second step ran despite first failing")
}

// Test: Cancel stops a run whose agents are still running.
func TestEngine_Cancel(t *testing.T) {
	prov := &fakeProvider{holdOpen: true}
	mgr := newTestManager(t, prov)
	e := NewEngine(mgr)

	wf := Workflow{
		Name: "long",
		Phases: []Phase{{
			Name: "p1",
			Steps: []Step{{
				Name:   "hold",
				Mode:   StepParallel,
				FanOut: []string{"x", "y"},
				Agent:  AgentSpec{Prompt: "hold {{.item}}"},
			}},
		}},
	}
	run, err := e.Start(context.Background(), wf)
	require.NoError(t, err)

	// Wait until agents are spawned, then cancel.
	require.Eventually(t, func() bool {
		snap, ok := e.Get(run.ID)
		return ok && len(snap.Phases[0].Steps[0].Agents) == 2
	}, 3*time.Second, 5*time.Millisecond)

	require.True(t, e.Cancel(run.ID))
	waitRun(t, e, run.ID, RunCancelled, 5*time.Second)
}

// Test: the WorkflowMaxAgents guard trips with a clear error.
func TestEngine_MaxAgentsGuard(t *testing.T) {
	prov := &fakeProvider{}
	mgr := newTestManager(t, prov)
	e := NewEngine(mgr)
	e.SetMaxAgents(2)

	wf := Workflow{
		Name: "greedy",
		Phases: []Phase{{
			Name: "p1",
			Steps: []Step{{
				Name:   "many",
				Mode:   StepParallel,
				FanOut: []string{"a", "b", "c", "d"},
				Agent:  AgentSpec{Prompt: "do {{.item}}"},
			}},
		}},
	}
	run, err := e.Start(context.Background(), wf)
	require.NoError(t, err)

	snap := waitRun(t, e, run.ID, RunFailed, 5*time.Second)
	require.Contains(t, snap.Err, "agent cap")
	require.EqualValues(t, 0, atomic.LoadInt32(&prov.spawns), "guard trips before any spawn")
}

// Test: Subscribe receives run updates including a terminal transition.
func TestEngine_SubscribeEmits(t *testing.T) {
	prov := &fakeProvider{}
	mgr := newTestManager(t, prov)
	e := NewEngine(mgr)

	var (
		mu     sync.Mutex
		states []RunState
	)
	e.Subscribe(func(u RunUpdate) {
		mu.Lock()
		states = append(states, u.Run.State)
		mu.Unlock()
	})

	wf := Workflow{
		Name: "sub",
		Phases: []Phase{{Name: "p1", Steps: []Step{{
			Name: "one", Mode: StepSingle, Agent: AgentSpec{Prompt: "hi"},
		}}}},
	}
	run, err := e.Start(context.Background(), wf)
	require.NoError(t, err)
	waitRun(t, e, run.ID, RunCompleted, 5*time.Second)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return containsRunState(states, RunCompleted)
	}, 2*time.Second, 5*time.Millisecond)
}

func containsRunState(s []RunState, want RunState) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
