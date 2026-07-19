package workflow

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/packetcode/packetcode/internal/jobs"
)

// DefaultWorkflowMaxAgents bounds how many agents a single run may spawn. It
// guards against a run consuming the whole jobs.Manager MaxTotal budget. A run
// that would exceed it fails with a clear error before spawning.
const DefaultWorkflowMaxAgents = 16

// defaultJobWaitTimeout is the per-agent WaitForJob budget. Cancellation (via
// Engine.Cancel → jobs.Cancel) makes a live child terminal, which unblocks the
// wait well before this fires; the ceiling only bounds a genuinely stuck job.
const defaultJobWaitTimeout = 30 * time.Minute

// terminalDrainTimeout bounds the second wait after a timed-out child has
// been cancelled. Well-behaved workers transition immediately; the bound
// prevents a provider that ignores context cancellation from hanging a run.
const terminalDrainTimeout = 5 * time.Second

// Engine orchestrates workflow runs over a jobs.Manager. It owns no
// agent-execution machinery: every agent is spawned as an ordinary background
// job and joined via WaitForJob.
type Engine struct {
	jobs *jobs.Manager

	mu    sync.Mutex
	runs  map[string]*Run
	order []string // run ids, oldest-first
	subs  []func(RunUpdate)

	maxAgents   int
	tokenBudget int
	waitTimeout time.Duration
}

// NewEngine constructs an Engine bound to the given jobs.Manager.
func NewEngine(m *jobs.Manager) *Engine {
	return &Engine{
		jobs:        m,
		runs:        map[string]*Run{},
		maxAgents:   DefaultWorkflowMaxAgents,
		waitTimeout: defaultJobWaitTimeout,
	}
}

// SetMaxAgents overrides the per-run agent cap. Values <= 0 are ignored.
// SetTokenBudget sets an aggregate input+output boundary budget per workflow
// run. Zero disables the budget. A concurrently running fan-out step may
// finish above the boundary; no later step is spawned after the boundary is
// observed.
func (e *Engine) SetTokenBudget(n int) {
	if n < 0 {
		n = 0
	}
	e.mu.Lock()
	e.tokenBudget = n
	e.mu.Unlock()
}

func (e *Engine) SetMaxAgents(n int) {
	if n <= 0 {
		return
	}
	e.mu.Lock()
	e.maxAgents = n
	e.mu.Unlock()
}

// Subscribe registers a callback fired on every run state change. Callbacks
// run on their own goroutine so a slow subscriber cannot block the driver.
func (e *Engine) Subscribe(fn func(RunUpdate)) {
	if fn == nil {
		return
	}
	e.mu.Lock()
	e.subs = append(e.subs, fn)
	e.mu.Unlock()
}

// Run is the live, mutable state of a workflow run. It is returned from Start
// for callers that want the id; UI code should consume RunSnapshot instead.
type Run struct {
	ID         string
	Workflow   string
	StartedAt  time.Time
	FinishedAt time.Time

	mu        sync.Mutex
	state     RunState
	err       error
	phases    []PhaseResult
	cancel    context.CancelFunc
	cancelled bool
	children  map[string]struct{}
}

// Start validates wf, registers a new run, and launches its driver goroutine.
// The driver context is a child of ctx; cancelling ctx (or calling
// Engine.Cancel) aborts the run.
func (e *Engine) Start(ctx context.Context, wf Workflow) (*Run, error) {
	if e.jobs == nil {
		return nil, fmt.Errorf("workflow: no jobs manager configured")
	}
	if err := validate(wf); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	driverCtx, cancel := context.WithCancel(ctx)

	run := &Run{
		ID:        newRunID(),
		Workflow:  wf.Name,
		StartedAt: time.Now().UTC(),
		state:     RunPending,
		cancel:    cancel,
		children:  make(map[string]struct{}),
	}
	// Pre-build the phase/step skeleton so snapshots have structure before
	// any agent has been spawned.
	for _, ph := range wf.Phases {
		pr := PhaseResult{Name: ph.Name}
		for _, st := range ph.Steps {
			pr.Steps = append(pr.Steps, StepResult{Step: st.Name, Mode: normalizeMode(st.Mode)})
		}
		run.phases = append(run.phases, pr)
	}

	e.mu.Lock()
	e.runs[run.ID] = run
	e.order = append(e.order, run.ID)
	e.mu.Unlock()

	e.emit(run)
	go e.drive(driverCtx, run, wf)
	return run, nil
}

// drive executes the workflow: phases sequentially, steps within a phase
// sequentially, and each step's agents per its mode. It fails fast on the
// first step error unless the phase is marked ContinueOnError.
func (e *Engine) drive(ctx context.Context, run *Run, wf Workflow) {
	run.setState(RunRunning)
	e.emit(run)

	e.mu.Lock()
	maxAgents := e.maxAgents
	tokenBudget := e.tokenBudget
	e.mu.Unlock()

	steps := map[string]string{} // bound step summaries for templating
	spawned := 0
	var firstErr error // first step error seen (fail-fast trigger and run error)

outer:
	for pi, ph := range wf.Phases {
		for si, st := range ph.Steps {
			used := workflowTokens(run)
			if tokenBudget > 0 && used >= tokenBudget {
				firstErr = fmt.Errorf("workflow token budget exhausted: used %d tokens (budget %d)", used, tokenBudget)
				break outer
			}
			if err := ctx.Err(); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				break outer
			}
			sr := e.runStep(ctx, run, pi, si, st, wf.Inputs, steps, &spawned, maxAgents)

			// Expose this step's summaries to later steps.
			steps[st.BindKey()] = summariesOf(sr)

			if sr.Err != nil {
				if firstErr == nil {
					firstErr = sr.Err
				}
				// Fail-fast unless the phase opts into continuing. Either
				// way the run's terminal state reflects the error.
				if !ph.ContinueOnError {
					break outer
				}
			}
		}
	}

	// Determine final state.
	run.mu.Lock()
	cancelled := run.cancelled
	run.mu.Unlock()

	switch {
	case cancelled || ctx.Err() != nil:
		run.finish(RunCancelled, firstErr)
	case firstErr != nil:
		run.finish(RunFailed, firstErr)
	default:
		run.finish(RunCompleted, nil)
	}
	e.emit(run)
}

// runStep executes a single step and records its result into the run at
// [pi][si], emitting a snapshot as spawns land and after the join.
func workflowTokens(run *Run) int {
	run.mu.Lock()
	defer run.mu.Unlock()
	total := 0
	for _, phase := range run.phases {
		for _, step := range phase.Steps {
			for _, result := range step.Agents {
				total += result.InputTokens + result.OutputTokens
			}
		}
	}
	return total
}

func (e *Engine) runStep(ctx context.Context, run *Run, pi, si int, st Step, inputs, steps map[string]string, spawned *int, maxAgents int) StepResult {
	mode := normalizeMode(st.Mode)

	// Determine the fan-out items. StepSingle is one item with an empty
	// template item value.
	var items []string
	if mode == StepParallel {
		items = st.FanOut
		if len(items) == 0 {
			sr := StepResult{Step: st.Name, Mode: mode, Err: fmt.Errorf("step %q: parallel step has no fan-out items", st.Name)}
			run.setStep(pi, si, sr)
			e.emit(run)
			return sr
		}
	} else {
		items = []string{""}
	}

	// Guard the per-run agent budget before spawning anything.
	if *spawned+len(items) > maxAgents {
		sr := StepResult{
			Step: st.Name, Mode: mode,
			Err: fmt.Errorf("workflow agent cap reached: step %q would spawn %d agents, exceeding the limit of %d", st.Name, len(items), maxAgents),
		}
		run.setStep(pi, si, sr)
		e.emit(run)
		return sr
	}

	// Spawn one job per item.
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			results := e.cancelAndDrain(ids)
			sr := StepResult{Step: st.Name, Mode: mode, JobIDs: ids, Agents: results, Err: err}
			run.setStep(pi, si, sr)
			e.emit(run)
			return sr
		}
		prompt, err := renderPrompt(st.Agent.Prompt, inputs, steps, item)
		if err != nil {
			results := e.cancelAndDrain(ids)
			sr := StepResult{Step: st.Name, Mode: mode, JobIDs: ids, Agents: results, Err: fmt.Errorf("step %q: render prompt: %w", st.Name, err)}
			run.setStep(pi, si, sr)
			e.emit(run)
			return sr
		}
		snap, serr := e.jobs.Spawn(jobs.SpawnRequest{
			Prompt:       prompt,
			Provider:     st.Agent.Provider,
			Model:        st.Agent.Model,
			SystemPrompt: st.Agent.SystemPrompt,
			AllowWrite:   st.Agent.AllowWrite,
		})
		if serr != nil {
			results := e.cancelAndDrain(ids)
			sr := StepResult{Step: st.Name, Mode: mode, JobIDs: ids, Agents: results, Err: fmt.Errorf("step %q: spawn failed: %s", st.Name, serr.Error())}
			run.setStep(pi, si, sr)
			e.emit(run)
			return sr
		}
		ids = append(ids, snap.ID)
		*spawned++
		// Registration and Cancel use the same lock. If cancellation won the
		// Spawn-to-registration race, cancel the newly created child now.
		if run.registerChild(snap.ID) {
			results := e.cancelAndDrain(ids)
			sr := StepResult{Step: st.Name, Mode: mode, JobIDs: ids, Agents: results, Err: context.Canceled}
			run.setStep(pi, si, sr)
			e.emit(run)
			return sr
		}
		run.setStep(pi, si, StepResult{Step: st.Name, Mode: mode, JobIDs: append([]string(nil), ids...)})
		e.emit(run)
	}

	// Join all agents concurrently (mirrors jobs spawner_adapter.CollectResults).
	results := e.waitAll(ctx, ids)

	sr := StepResult{Step: st.Name, Mode: mode, JobIDs: ids, Agents: results}
	// A failed or cancelled agent fails the step.
	for _, r := range results {
		if r.State == jobs.StateFailed {
			sr.Err = fmt.Errorf("step %q: agent %s failed: %s", st.Name, r.JobID, firstNonEmpty(r.Error, r.Reason, "unknown error"))
			break
		}
		if r.State == jobs.StateCancelled {
			sr.Err = fmt.Errorf("step %q: agent %s cancelled", st.Name, r.JobID)
			break
		}
	}
	if sr.Err == nil && len(results) != len(ids) {
		sr.Err = fmt.Errorf("step %q: %d of %d agents did not report a result", st.Name, len(ids)-len(results), len(ids))
	}
	run.setStep(pi, si, sr)
	e.emit(run)
	return sr
}

// waitAll waits for every job id concurrently and returns their results in
// the original id order, skipping any that never reported (timeout).
func (e *Engine) waitAll(ctx context.Context, ids []string) []jobs.Result {
	e.mu.Lock()
	timeout := e.waitTimeout
	e.mu.Unlock()

	var mu sync.Mutex
	got := make(map[string]jobs.Result, len(ids))
	failed := make(chan struct{}, 1)
	var wg sync.WaitGroup
	wg.Add(len(ids))
	for _, id := range ids {
		id := id
		go func() {
			defer wg.Done()
			// Keep waiting for the job's terminal transition after the workflow
			// context is cancelled. The coordinator below cancels every child;
			// this independent timeout makes the join a bounded terminal drain.
			waitCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			res, ok := e.jobs.WaitForJobContext(waitCtx, id)
			if !ok {
				select {
				case failed <- struct{}{}:
				default:
				}
				e.jobs.Cancel(id)
				drainCtx, drainCancel := context.WithTimeout(context.Background(), terminalDrainTimeout)
				res, ok = e.jobs.WaitForJobContext(drainCtx, id)
				drainCancel()
				if !ok {
					return
				}
			}
			mu.Lock()
			got[id] = res
			mu.Unlock()
			if res.State == jobs.StateFailed || res.State == jobs.StateCancelled {
				select {
				case failed <- struct{}{}:
				default:
				}
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-failed:
		e.cancelJobs(ids)
		<-done
	case <-ctx.Done():
		e.cancelJobs(ids)
		<-done
	case <-done:
	}

	out := make([]jobs.Result, 0, len(ids))
	for _, id := range ids {
		if r, ok := got[id]; ok {
			out = append(out, r)
		}
	}
	return out
}

// Cancel marks the run cancelled, cancels its driver context, and cancels every
// live child job so any in-flight WaitForJob unblocks. Returns false if the id
// is unknown or already terminal.
func (e *Engine) Cancel(id string) bool {
	e.mu.Lock()
	run, ok := e.runs[id]
	e.mu.Unlock()
	if !ok {
		return false
	}

	run.mu.Lock()
	if run.state.IsTerminal() {
		run.mu.Unlock()
		return false
	}
	run.cancelled = true
	cancel := run.cancel
	ids := run.childIDsLocked()
	run.mu.Unlock()

	for _, jid := range ids {
		e.jobs.Cancel(jid)
	}
	if cancel != nil {
		cancel()
	}
	e.emit(run)
	return true
}

// CancelAll cancels every non-terminal run. Returns the number cancelled.
func (e *Engine) CancelAll() int {
	e.mu.Lock()
	ids := append([]string(nil), e.order...)
	e.mu.Unlock()
	n := 0
	for _, id := range ids {
		if e.Cancel(id) {
			n++
		}
	}
	return n
}

// List returns snapshots of every run, newest-first.
func (e *Engine) List() []RunSnapshot {
	e.mu.Lock()
	runs := make([]*Run, 0, len(e.order))
	for _, id := range e.order {
		if r, ok := e.runs[id]; ok {
			runs = append(runs, r)
		}
	}
	e.mu.Unlock()

	out := make([]RunSnapshot, 0, len(runs))
	for _, r := range runs {
		out = append(out, e.snapshot(r))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}

// Get returns a snapshot of one run.
func (e *Engine) Get(id string) (RunSnapshot, bool) {
	e.mu.Lock()
	run, ok := e.runs[id]
	e.mu.Unlock()
	if !ok {
		return RunSnapshot{}, false
	}
	return e.snapshot(run), true
}

// emit fans a fresh snapshot out to every subscriber on its own goroutine.
func (e *Engine) emit(run *Run) {
	snap := e.snapshot(run)
	e.mu.Lock()
	subs := make([]func(RunUpdate), len(e.subs))
	copy(subs, e.subs)
	e.mu.Unlock()
	for _, fn := range subs {
		fn := fn
		go func() {
			defer func() { _ = recover() }()
			fn(RunUpdate{Run: snap})
		}()
	}
}

// snapshot builds a UI-safe RunSnapshot, fetching live job snapshots from the
// manager so the view can render each agent row.
func (e *Engine) snapshot(run *Run) RunSnapshot {
	run.mu.Lock()
	defer run.mu.Unlock()

	rs := RunSnapshot{
		ID:         run.ID,
		Workflow:   run.Workflow,
		State:      run.state,
		StartedAt:  run.StartedAt,
		FinishedAt: run.FinishedAt,
	}
	if run.err != nil {
		rs.Err = run.err.Error()
	}
	for _, ph := range run.phases {
		ps := PhaseSnapshot{Name: ph.Name}
		for _, st := range ph.Steps {
			ss := StepSnapshot{Name: st.Step, Mode: st.Mode}
			if st.Err != nil {
				ss.Err = st.Err.Error()
			}
			for _, jid := range st.JobIDs {
				as := AgentSnapshot{JobID: jid}
				if snap, ok := e.jobs.Get(jid); ok {
					as.Job = snap
					as.HasJob = true
				}
				ss.Agents = append(ss.Agents, as)
			}
			ps.Steps = append(ps.Steps, ss)
		}
		rs.Phases = append(rs.Phases, ps)
	}
	return rs
}

// ─────────────────────────────────────────────────────────────────────────
// Run helpers (all take run.mu)
// ─────────────────────────────────────────────────────────────────────────

func (r *Run) setState(s RunState) {
	r.mu.Lock()
	r.state = s
	r.mu.Unlock()
}

func (r *Run) finish(s RunState, err error) {
	r.mu.Lock()
	r.state = s
	if err != nil {
		r.err = err
	}
	r.FinishedAt = time.Now().UTC()
	r.mu.Unlock()
}

func (r *Run) setStep(pi, si int, sr StepResult) {
	r.mu.Lock()
	if pi >= 0 && pi < len(r.phases) && si >= 0 && si < len(r.phases[pi].Steps) {
		r.phases[pi].Steps[si] = sr
	}
	r.mu.Unlock()
}

// childIDsLocked returns every registered child. Caller holds r.mu.
func (r *Run) childIDsLocked() []string {
	ids := make([]string, 0, len(r.children))
	for id := range r.children {
		ids = append(ids, id)
	}
	return ids
}

// registerChild returns true when the run was already cancelled. Holding the
// run lock closes the race with Cancel taking its child snapshot.
func (r *Run) registerChild(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.children[id] = struct{}{}
	return r.cancelled
}

func (e *Engine) cancelJobs(ids []string) {
	for _, id := range ids {
		e.jobs.Cancel(id)
	}
}

func (e *Engine) cancelAndDrain(ids []string) []jobs.Result {
	if len(ids) == 0 {
		return nil
	}
	e.cancelJobs(ids)
	ctx, cancel := context.WithTimeout(context.Background(), terminalDrainTimeout)
	defer cancel()

	results := make([]jobs.Result, len(ids))
	present := make([]bool, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		i, id := i, id
		wg.Add(1)
		go func() {
			defer wg.Done()
			if result, ok := e.jobs.WaitForJobContext(ctx, id); ok {
				results[i] = result
				present[i] = true
			}
		}()
	}
	wg.Wait()

	out := make([]jobs.Result, 0, len(ids))
	for i := range results {
		if present[i] {
			out = append(out, results[i])
		}
	}
	return out
}

func normalizeMode(m StepMode) StepMode {
	if m == StepParallel {
		return StepParallel
	}
	return StepSingle
}

func newRunID() string {
	id := uuid.NewString()
	out := make([]byte, 0, 8)
	for i := 0; i < len(id) && len(out) < 8; i++ {
		if id[i] == '-' {
			continue
		}
		out = append(out, id[i])
	}
	return "wf-" + string(out)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// validate performs light structural validation of a workflow spec.
func validate(wf Workflow) error {
	if wf.Name == "" {
		return fmt.Errorf("workflow: missing name")
	}
	if len(wf.Phases) == 0 {
		return fmt.Errorf("workflow %q: no phases", wf.Name)
	}
	for _, ph := range wf.Phases {
		if len(ph.Steps) == 0 {
			return fmt.Errorf("workflow %q: phase %q has no steps", wf.Name, ph.Name)
		}
		for _, st := range ph.Steps {
			if st.Name == "" {
				return fmt.Errorf("workflow %q: phase %q has an unnamed step", wf.Name, ph.Name)
			}
			if normalizeMode(st.Mode) == StepParallel && len(st.FanOut) == 0 {
				return fmt.Errorf("workflow %q: parallel step %q has no fan-out items", wf.Name, st.Name)
			}
		}
	}
	return nil
}
