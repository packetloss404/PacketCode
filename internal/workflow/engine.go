package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/packetcode/packetcode/internal/jobs"
	"github.com/packetcode/packetcode/internal/provider"
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

// SetMaxAgents overrides the per-run agent cap. Values <= 0 are ignored.
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
	Computer   string // requested selector; empty inherits manager default
	ComputerID string
	// ComputerName and WorkingDir are resolved from the first spawned job.
	ComputerName string
	WorkingDir   string

	mu        sync.Mutex
	state     RunState
	err       error
	phases    []PhaseResult
	cancel    context.CancelFunc
	cancelled bool
	children  map[string]struct{}
}

// Start validates wf, registers a new run, and launches its driver goroutine
// against the jobs.Manager's default workspace. It remains the compatibility
// entry point for callers that do not need explicit placement.
func (e *Engine) Start(ctx context.Context, wf Workflow) (*Run, error) {
	return e.StartWithOptions(ctx, wf, RunOptions{})
}

// StartWithOptions starts a workflow with run-level execution placement. The
// selector is forwarded to every work attempt, retry, and verifier; the first
// queued job stamps the run with resolved target identity and working dir.
func (e *Engine) StartWithOptions(ctx context.Context, wf Workflow, opts RunOptions) (*Run, error) {
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
		Computer:  strings.TrimSpace(opts.Computer),
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
			verification := VerificationUnverified
			if st.Verify != nil {
				verification = VerificationPending
			}
			pr.Steps = append(pr.Steps, StepResult{
				Step: st.Name, Mode: normalizeMode(st.Mode), Verification: verification,
			})
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
			if err := workflowBudgetError(run, tokenBudget); err != nil {
				// Guarded like the ctx.Err() check below it. In a
				// ContinueOnError phase firstErr may already hold the failure
				// the user actually needs to see; overwriting it reports
				// "token budget exhausted" for a run that really broke for
				// another reason two steps earlier.
				if firstErr == nil {
					firstErr = err
				}
				break outer
			}
			if err := ctx.Err(); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				break outer
			}
			sr := e.runStep(ctx, run, pi, si, st, wf.Inputs, steps, &spawned, maxAgents, tokenBudget)

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

// workflowTokens sums the input+output tokens every agent in the run has
// reported so far, across every attempt and verifier.
func workflowTokens(run *Run) int {
	run.mu.Lock()
	defer run.mu.Unlock()
	total := 0
	for _, phase := range run.phases {
		for _, step := range phase.Steps {
			if len(step.Attempts) == 0 {
				for _, result := range step.Agents {
					total += result.InputTokens + result.OutputTokens
				}
				continue
			}
			for _, attempt := range step.Attempts {
				for _, result := range attempt.Agents {
					total += result.InputTokens + result.OutputTokens
				}
				if attempt.Verifier != nil {
					total += attempt.Verifier.InputTokens + attempt.Verifier.OutputTokens
				}
			}
		}
	}
	return total
}

// runStep executes a single step and records its result into the run at
// [pi][si], emitting a snapshot as spawns land and after the join.
func (e *Engine) runStep(
	ctx context.Context,
	run *Run,
	pi, si int,
	st Step,
	inputs, steps map[string]string,
	spawned *int,
	maxAgents, tokenBudget int,
) StepResult {
	mode := normalizeMode(st.Mode)
	verification := VerificationUnverified
	if st.Verify != nil {
		verification = VerificationPending
	}
	sr := StepResult{Step: st.Name, Mode: mode, Verification: verification}
	publish := func() { e.publishStep(run, pi, si, sr) }

	maxAttempts := st.Retry.Max + 1
	lastVerifyReason := ""
	for attemptNumber := 1; attemptNumber <= maxAttempts; attemptNumber++ {
		if err := workflowBudgetError(run, tokenBudget); err != nil {
			sr.Err = err
			if st.Verify != nil {
				sr.Verification = VerificationFailed
				sr.VerifyReason = err.Error()
			}
			publish()
			return sr
		}

		progress := func(current AttemptResult) {
			preview := cloneStepResult(sr)
			preview.Attempts = append(preview.Attempts, cloneAttemptResult(current))
			preview.Agents = append([]jobs.Result(nil), current.Agents...)
			preview.JobIDs = append([]string(nil), current.JobIDs...)
			e.publishStep(run, pi, si, preview)
		}
		attempt := e.runWorkAttempt(
			ctx, run, st, inputs, steps, attemptNumber, lastVerifyReason,
			spawned, maxAgents, progress,
		)
		sr.Attempts = append(sr.Attempts, attempt)
		sr.Agents = append([]jobs.Result(nil), attempt.Agents...)
		sr.JobIDs = append([]string(nil), attempt.JobIDs...)
		if attempt.Err != nil {
			sr.Err = attempt.Err
			if st.Verify != nil {
				sr.Verification = VerificationFailed
				sr.VerifyReason = "work attempt failed before verification: " + attempt.Err.Error()
			}
			publish()
			return sr
		}

		if st.Verify == nil {
			sr.Verification = VerificationUnverified
			publish()
			return sr
		}

		// Work usage is now known. Refuse to launch a verifier when the work
		// attempt already exhausted the approved aggregate budget.
		if err := workflowBudgetError(run, tokenBudget); err != nil {
			sr.Err = err
			sr.Verification = VerificationFailed
			sr.VerifyReason = err.Error()
			publish()
			return sr
		}

		workEvidence := verifierEvidenceOf(attempt.Agents)
		if strings.TrimSpace(workEvidence) == "" {
			workEvidence = "(the work agents returned no evidence)"
		}
		verifierResult, verifierID, verifierErr := e.runVerifier(
			ctx, run, st, inputs, steps, workEvidence, attemptNumber,
			spawned, maxAgents,
			func(id string) {
				sr.Attempts[len(sr.Attempts)-1].VerifierJobID = id
				publish()
			},
		)
		current := &sr.Attempts[len(sr.Attempts)-1]
		current.VerifierJobID = verifierID
		if verifierID != "" {
			resultCopy := verifierResult
			current.Verifier = &resultCopy
		}

		verdict := VerificationFailed
		reason := ""
		if verifierErr != nil {
			reason = verifierErr.Error()
		} else {
			verdictText := e.verifierOutput(verifierResult)
			parsed, parsedReason, parseErr := ParseVerdict(st.Verify.PassContract, verdictText)
			verdict = parsed
			reason = parsedReason
			if parseErr != nil {
				reason = parseErr.Error()
			}
		}
		current.Verdict = verdict
		current.Reason = reason
		sr.Verification = verdict
		sr.VerifyReason = reason
		publish()

		if verdict == VerificationPassed {
			return sr
		}
		lastVerifyReason = firstNonEmpty(reason, "verifier returned fail")
		if attemptNumber < maxAttempts {
			sr.Verification = VerificationPending
			sr.VerifyReason = fmt.Sprintf("attempt %d failed verification: %s; retrying", attemptNumber, lastVerifyReason)
			publish()
			continue
		}

		sr.Err = fmt.Errorf(
			"step %q: verification failed after %d attempt(s): %s",
			st.Name, attemptNumber, lastVerifyReason,
		)
		publish()
		return sr
	}

	// maxAttempts is always at least one after validation; retain a defensive
	// fail-closed terminal path for programmatic specs.
	sr.Err = fmt.Errorf("step %q: no workflow attempt executed", st.Name)
	publish()
	return sr
}

func (e *Engine) runWorkAttempt(
	ctx context.Context,
	run *Run,
	st Step,
	inputs, steps map[string]string,
	attemptNumber int,
	previousVerifyReason string,
	spawned *int,
	maxAgents int,
	onProgress func(AttemptResult),
) AttemptResult {
	mode := normalizeMode(st.Mode)
	attempt := AttemptResult{Number: attemptNumber, Verdict: VerificationUnverified}

	var items []string
	if mode == StepParallel {
		items = st.FanOut
		if len(items) == 0 {
			attempt.Err = fmt.Errorf("step %q: parallel step has no fan-out items", st.Name)
			return attempt
		}
	} else {
		items = []string{""}
	}
	if *spawned+len(items) > maxAgents {
		attempt.Err = fmt.Errorf(
			"workflow agent cap reached: step %q attempt %d would spawn %d agents, exceeding the limit of %d",
			st.Name, attemptNumber, len(items), maxAgents,
		)
		return attempt
	}

	ids := make([]string, 0, len(items))
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			attempt.JobIDs = append([]string(nil), ids...)
			attempt.Agents = e.cancelAndDrain(ids)
			attempt.Err = err
			return attempt
		}
		prompt, err := renderPrompt(st.Agent.Prompt, inputs, steps, item)
		if err != nil {
			attempt.JobIDs = append([]string(nil), ids...)
			attempt.Agents = e.cancelAndDrain(ids)
			attempt.Err = fmt.Errorf("step %q: render prompt: %w", st.Name, err)
			return attempt
		}
		if attemptNumber > 1 {
			prompt += fmt.Sprintf(
				"\n\nThis is retry attempt %d. The previous verifier failed the work: %s\nAddress that evidence before returning a revised result.",
				attemptNumber, previousVerifyReason,
			)
		}
		snap, spawnErr := e.jobs.Spawn(jobs.SpawnRequest{
			Prompt:       prompt,
			Provider:     st.Agent.Provider,
			Model:        st.Agent.Model,
			Computer:     run.Computer,
			SystemPrompt: st.Agent.SystemPrompt,
			AllowWrite:   st.Agent.AllowWrite,
		})
		if spawnErr != nil {
			attempt.JobIDs = append([]string(nil), ids...)
			attempt.Agents = e.cancelAndDrain(ids)
			attempt.Err = fmt.Errorf("step %q: spawn failed: %s", st.Name, spawnErr.Error())
			return attempt
		}
		run.bindWorkspace(snap.ComputerID, snap.ComputerName, snap.WorkingDir)
		ids = append(ids, snap.ID)
		(*spawned)++
		attempt.JobIDs = append([]string(nil), ids...)
		if run.registerChild(snap.ID) {
			attempt.Agents = e.cancelAndDrain(ids)
			attempt.Err = context.Canceled
			return attempt
		}
		if onProgress != nil {
			onProgress(attempt)
		}
	}

	attempt.Agents = e.waitAll(ctx, ids)
	attempt.JobIDs = append([]string(nil), ids...)
	for _, result := range attempt.Agents {
		if err := e.agentOutcomeError(st.Name, "agent "+result.JobID, result); err != nil {
			attempt.Err = err
			break
		}
	}
	if attempt.Err == nil && len(attempt.Agents) != len(ids) {
		attempt.Err = fmt.Errorf("step %q: %d of %d agents did not report a result", st.Name, len(ids)-len(attempt.Agents), len(ids))
	}
	if onProgress != nil {
		onProgress(attempt)
	}
	return attempt
}

func (e *Engine) runVerifier(
	ctx context.Context,
	run *Run,
	st Step,
	inputs, steps map[string]string,
	workSummary string,
	attemptNumber int,
	spawned *int,
	maxAgents int,
	onStarted func(string),
) (jobs.Result, string, error) {
	if st.Verify == nil {
		return jobs.Result{}, "", fmt.Errorf("step %q: verifier is not configured", st.Name)
	}
	if *spawned+1 > maxAgents {
		return jobs.Result{}, "", fmt.Errorf(
			"workflow agent cap reached: verifier for step %q attempt %d would exceed the limit of %d",
			st.Name, attemptNumber, maxAgents,
		)
	}
	prompt, err := renderVerifyPrompt(st.Verify.Prompt, inputs, steps, workSummary, attemptNumber)
	if err != nil {
		return jobs.Result{}, "", fmt.Errorf("step %q: render verifier prompt: %w", st.Name, err)
	}
	prompt += fmt.Sprintf("\n\nCompleted work for attempt %d:\n%s\n\n%s", attemptNumber, workSummary, verifierContractInstruction(st.Verify.PassContract))
	snap, spawnErr := e.jobs.Spawn(jobs.SpawnRequest{
		Prompt:       prompt,
		Provider:     st.Verify.Provider,
		Model:        st.Verify.Model,
		Computer:     run.Computer,
		SystemPrompt: st.Verify.SystemPrompt,
		AllowWrite:   false,
	})
	if spawnErr != nil {
		return jobs.Result{}, "", fmt.Errorf("step %q: verifier spawn failed: %s", st.Name, spawnErr.Error())
	}
	run.bindWorkspace(snap.ComputerID, snap.ComputerName, snap.WorkingDir)
	(*spawned)++
	if run.registerChild(snap.ID) {
		e.cancelAndDrain([]string{snap.ID})
		return jobs.Result{}, snap.ID, context.Canceled
	}
	if onStarted != nil {
		onStarted(snap.ID)
	}
	results := e.waitAll(ctx, []string{snap.ID})
	if len(results) != 1 {
		return jobs.Result{}, snap.ID, fmt.Errorf("step %q: verifier did not report a result", st.Name)
	}
	result := results[0]
	if err := e.agentOutcomeError(st.Name, "verifier", result); err != nil {
		return result, snap.ID, err
	}
	return result, snap.ID, nil
}

// agentOutcomeError reports why a joined agent result is not a success, and
// nil when it is. The gate is IsSuccess rather than a list of known failure
// states: with a failure list, every terminal state added later falls through
// as a pass and the run advances on work that never succeeded. who is the
// caller's label for the agent ("agent <id>" or "verifier").
func (e *Engine) agentOutcomeError(stepName, who string, result jobs.Result) error {
	if result.State.IsSuccess() {
		return nil
	}
	switch result.State {
	case jobs.StateFailed:
		return fmt.Errorf("step %q: %s failed: %s", stepName, who, firstNonEmpty(result.Error, result.Reason, "unknown error"))
	case jobs.StateCancelled:
		return fmt.Errorf("step %q: %s cancelled", stepName, who)
	case jobs.StateAbandoned:
		cause := ""
		if c := e.abandonCause(result.JobID); c != "" {
			cause = fmt.Sprintf(" (cause: %s)", c)
		}
		return fmt.Errorf("step %q: %s abandoned%s: the outcome could not be confirmed: %s",
			stepName, who, cause, firstNonEmpty(result.Error, result.Reason, "no detail recorded"))
	default:
		return fmt.Errorf("step %q: %s did not succeed: state %s", stepName, who, result.State)
	}
}

// abandonCause reads the cause recorded alongside StateAbandoned. jobs.Result
// carries the state but not the cause, so the job snapshot is the only place
// to ask why the outcome is unknown.
func (e *Engine) abandonCause(jobID string) jobs.AbandonCause {
	snap, ok := e.jobs.Get(jobID)
	if !ok {
		return ""
	}
	return snap.AbandonCause
}

func (e *Engine) verifierOutput(result jobs.Result) string {
	if transcript, ok := e.jobs.Transcript(result.JobID); ok {
		for i := len(transcript) - 1; i >= 0; i-- {
			if transcript[i].Role == provider.RoleAssistant && strings.TrimSpace(transcript[i].Content) != "" {
				return transcript[i].Content
			}
		}
	}
	return result.Summary
}

func workflowBudgetError(run *Run, tokenBudget int) error {
	if tokenBudget <= 0 {
		return nil
	}
	used := workflowTokens(run)
	if used < tokenBudget {
		return nil
	}
	return fmt.Errorf("workflow token budget exhausted: used %d tokens (budget %d)", used, tokenBudget)
}

func (e *Engine) publishStep(run *Run, pi, si int, sr StepResult) {
	run.setStep(pi, si, cloneStepResult(sr))
	e.emit(run)
}

func cloneStepResult(sr StepResult) StepResult {
	out := sr
	out.Agents = append([]jobs.Result(nil), sr.Agents...)
	out.JobIDs = append([]string(nil), sr.JobIDs...)
	out.Attempts = make([]AttemptResult, len(sr.Attempts))
	for i := range sr.Attempts {
		out.Attempts[i] = cloneAttemptResult(sr.Attempts[i])
	}
	return out
}

func cloneAttemptResult(attempt AttemptResult) AttemptResult {
	out := attempt
	out.Agents = append([]jobs.Result(nil), attempt.Agents...)
	out.JobIDs = append([]string(nil), attempt.JobIDs...)
	if attempt.Verifier != nil {
		verifier := *attempt.Verifier
		out.Verifier = &verifier
	}
	return out
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
			// Any non-success terminal state trips fail-fast, including one
			// added after this was written: an unconfirmed outcome is not a
			// reason to leave sibling agents burning budget.
			if !res.State.IsSuccess() {
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
		ID:           run.ID,
		Workflow:     run.Workflow,
		State:        run.state,
		StartedAt:    run.StartedAt,
		FinishedAt:   run.FinishedAt,
		Computer:     run.Computer,
		ComputerID:   run.ComputerID,
		ComputerName: run.ComputerName,
		WorkingDir:   run.WorkingDir,
	}
	if run.err != nil {
		rs.Err = run.err.Error()
	}
	for _, ph := range run.phases {
		ps := PhaseSnapshot{Name: ph.Name}
		for _, st := range ph.Steps {
			ss := StepSnapshot{
				Name:         st.Step,
				Mode:         st.Mode,
				Attempts:     len(st.Attempts),
				Verification: st.Verification,
				VerifyReason: st.VerifyReason,
			}
			if st.Err != nil {
				ss.Err = st.Err.Error()
			}
			if len(st.Attempts) == 0 {
				for _, jid := range st.JobIDs {
					ss.Agents = append(ss.Agents, e.agentSnapshot(jid, "work", 1))
				}
			} else {
				for _, attempt := range st.Attempts {
					for _, jid := range attempt.JobIDs {
						ss.Agents = append(ss.Agents, e.agentSnapshot(jid, "work", attempt.Number))
					}
					if attempt.VerifierJobID != "" {
						ss.Agents = append(ss.Agents, e.agentSnapshot(attempt.VerifierJobID, "verifier", attempt.Number))
					}
				}
			}
			ps.Steps = append(ps.Steps, ss)
		}
		rs.Phases = append(rs.Phases, ps)
	}
	return rs
}

func (e *Engine) agentSnapshot(jobID, role string, attempt int) AgentSnapshot {
	as := AgentSnapshot{JobID: jobID, Role: role, Attempt: attempt}
	if snap, ok := e.jobs.Get(jobID); ok {
		as.Job = snap
		as.HasJob = true
	}
	return as
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

// bindWorkspace records the immutable placement resolved by jobs.Manager.
// Parallel fan-out jobs should all resolve identically; once a non-empty
// field is stamped it is never replaced by a later snapshot.
func (r *Run) bindWorkspace(computerID, computerName, workingDir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ComputerID == "" {
		r.ComputerID = computerID
	}
	if r.ComputerName == "" {
		r.ComputerName = computerName
	}
	if r.WorkingDir == "" {
		r.WorkingDir = workingDir
	}
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

// Validate performs structural and template validation without spawning any
// agents. SchemaVersion zero is accepted only for programmatically-created
// workflows retained for Go API compatibility; TOML files must declare the
// current version in Loader.loadTOMLWorkflow.
func Validate(wf Workflow) error {
	if wf.SchemaVersion != 0 && wf.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("workflow: unsupported schema_version %d (current: %d)", wf.SchemaVersion, CurrentSchemaVersion)
	}
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
			if st.Mode != "" && st.Mode != StepSingle && st.Mode != StepParallel {
				return fmt.Errorf("workflow %q: step %q has unsupported mode %q", wf.Name, st.Name, st.Mode)
			}
			if normalizeMode(st.Mode) == StepParallel && len(st.FanOut) == 0 {
				return fmt.Errorf("workflow %q: parallel step %q has no fan-out items", wf.Name, st.Name)
			}
			if _, err := renderPrompt(st.Agent.Prompt, wf.Inputs, nil, "item"); err != nil {
				return fmt.Errorf("workflow %q: step %q prompt template: %w", wf.Name, st.Name, err)
			}
			if st.Retry.Max < 0 {
				return fmt.Errorf("workflow %q: step %q retry.max must be >= 0", wf.Name, st.Name)
			}
			if st.Verify == nil {
				if st.Retry.Max > 0 {
					return fmt.Errorf("workflow %q: step %q retry.max requires a verify block", wf.Name, st.Name)
				}
				continue
			}
			if strings.TrimSpace(st.Verify.Prompt) == "" {
				return fmt.Errorf("workflow %q: step %q verify.prompt is required", wf.Name, st.Name)
			}
			if strings.TrimSpace(st.Verify.Provider) == "" {
				return fmt.Errorf("workflow %q: step %q verify.provider is required", wf.Name, st.Name)
			}
			if strings.TrimSpace(st.Verify.Model) == "" {
				return fmt.Errorf("workflow %q: step %q verify.model is required", wf.Name, st.Name)
			}
			if st.Verify.PassContract != PassContractV1 {
				return fmt.Errorf(
					"workflow %q: step %q unsupported verify.pass_contract %q (supported: %s)",
					wf.Name, st.Name, st.Verify.PassContract, PassContractV1,
				)
			}
			if _, err := renderVerifyPrompt(st.Verify.Prompt, wf.Inputs, nil, "result", 1); err != nil {
				return fmt.Errorf("workflow %q: step %q verifier prompt template: %w", wf.Name, st.Name, err)
			}
		}
	}
	return nil
}

func validate(wf Workflow) error { return Validate(wf) }
