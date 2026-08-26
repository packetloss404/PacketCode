// Package workflow is a Go-native multi-agent workflow engine layered over
// the existing background-jobs manager (internal/jobs). It orchestrates a
// Workflow — a sequence of Phases, each holding Steps that run either a
// single agent or a parallel fan-out of agents — by spawning jobs on the
// jobs.Manager and joining their results. The engine adds no agent-execution
// machinery of its own: every agent is an ordinary background job.
//
// Workflows support sequential phases, single-agent steps, parallel fan-out,
// and optional fail-closed verification with bounded retries. Every retry and
// verifier is an ordinary background job, so the existing job limits,
// permissions, token accounting, cancellation, and transcript surfaces remain
// authoritative.
package workflow

import (
	"time"

	"github.com/packetcode/packetcode/internal/jobs"
)

// StepMode selects how a Step is executed.
type StepMode string

const (
	// StepSingle runs the step's Agent once.
	StepSingle StepMode = "single"
	// StepParallel spawns one agent per FanOut item, concurrently, and
	// joins their results.
	StepParallel StepMode = "parallel"
)

const (
	// CurrentSchemaVersion is the only workflow TOML schema Packetcode accepts.
	// A file with a newer version is refused rather than best-effort decoded.
	CurrentSchemaVersion  = 1
	CurrentVerdictVersion = 1

	// PassContractV1 names the structured verdict contract supported by the
	// verifier. The model must return the tagged JSON object documented in
	// docs/workflows.md; missing or malformed verdicts fail closed.
	PassContractV1 = "packetcode-workflow-verdict-v1"
)

// AgentSpec describes a single agent invocation. Prompt is a text/template
// rendered against the workflow inputs, prior bound step summaries, and (for
// fan-out) the current {{.item}}. Provider/Model/SystemPrompt are optional
// overrides; empty values fall through to the jobs.Manager defaults.
type AgentSpec struct {
	Prompt       string
	Provider     string
	Model        string
	SystemPrompt string
	AllowWrite   bool
}

// VerifySpec declares the independent agent that decides whether a completed
// step attempt passes. Provider and Model are required in versioned TOML so a
// verifier never silently changes when the foreground provider changes.
type VerifySpec struct {
	Prompt       string
	Provider     string
	Model        string
	SystemPrompt string
	PassContract string
}

// RetrySpec bounds how many additional work attempts may run after a verifier
// failure. Max=0 means the initial attempt only.
type RetrySpec struct {
	Max int
}

// Step is one unit of work within a Phase.
type Step struct {
	Name   string
	Mode   StepMode
	Agent  AgentSpec
	FanOut []string // items for StepParallel; each renders {{.item}}
	Bind   string   // name to bind this step's summaries under (default: Name)

	// Verify is optional. A missing verifier leaves the step explicitly
	// unverified; it is never rendered as passed. Retry applies only when a
	// verifier is present.
	Verify *VerifySpec
	Retry  RetrySpec
}

// BindKey returns the name under which this step's summaries are exposed to
// later steps via {{.steps.<key>}}. Defaults to the step Name.
func (s Step) BindKey() string {
	if s.Bind != "" {
		return s.Bind
	}
	return s.Name
}

// Phase is a group of Steps that run in declaration order. When
// ContinueOnError is true a failing step does not abort the run; otherwise
// the run fails fast at the first step error.
type Phase struct {
	Name            string
	Steps           []Step
	ContinueOnError bool
}

// Workflow is a named, reusable orchestration spec.
type Workflow struct {
	SchemaVersion int
	Name          string
	Inputs        map[string]string
	Phases        []Phase
}

// RunOptions controls execution placement without changing the reusable
// workflow definition. Computer is a registered Packet Computer name (or an
// empty string to inherit the jobs.Manager's active workspace default).
// Placement deliberately lives outside schema-versioned TOML: every work
// attempt, retry, and verifier in one run executes against the same target.
type RunOptions struct {
	Computer string
}

// VerificationState is the user-visible verification outcome for a step.
type VerificationState string

const (
	VerificationUnverified VerificationState = "unverified"
	VerificationPending    VerificationState = "pending"
	VerificationPassed     VerificationState = "passed"
	VerificationFailed     VerificationState = "failed"
)

// AttemptResult records one work attempt and its optional verifier. Agents are
// the work jobs; Verifier is kept separate so the live view can label it.
type AttemptResult struct {
	Number        int
	Agents        []jobs.Result
	JobIDs        []string
	Verifier      *jobs.Result
	VerifierJobID string
	Verdict       VerificationState
	Reason        string
	Err           error
}

// StepResult is the terminal outcome of a Step: the joined agent results
// (one per fan-out item, or a single element for StepSingle) plus any error
// that failed the step.
type StepResult struct {
	Step         string
	Mode         StepMode
	Agents       []jobs.Result // work agents from the final attempt
	JobIDs       []string      // work job ids from the final attempt
	Attempts     []AttemptResult
	Verification VerificationState
	VerifyReason string
	Err          error
}

// PhaseResult holds the StepResults for one phase.
type PhaseResult struct {
	Name  string
	Steps []StepResult
}

// RunState enumerates the lifecycle of a Run.
type RunState string

const (
	RunPending   RunState = "pending"
	RunRunning   RunState = "running"
	RunCompleted RunState = "completed"
	RunFailed    RunState = "failed"
	RunCancelled RunState = "cancelled"
)

// IsTerminal reports whether the run has reached a final state.
func (s RunState) IsTerminal() bool {
	switch s {
	case RunCompleted, RunFailed, RunCancelled:
		return true
	default:
		return false
	}
}

// ─────────────────────────────────────────────────────────────────────────
// UI-safe snapshots
// ─────────────────────────────────────────────────────────────────────────

// AgentSnapshot is a UI-safe projection of one agent within a step. Job is
// the live jobs.Snapshot (fetched from the manager) so the view can reuse the
// agent-view rendering; HasJob is false when the manager no longer knows the
// id. A spawn failure never reaches here — it is recorded on the step as
// StepSnapshot.Err, because no agent row exists for a job that never spawned.
type AgentSnapshot struct {
	JobID   string
	Job     jobs.Snapshot
	HasJob  bool
	Role    string // "work" or "verifier"
	Attempt int
}

// StepSnapshot is a UI-safe projection of a Step.
type StepSnapshot struct {
	Name         string
	Mode         StepMode
	Agents       []AgentSnapshot
	Attempts     int
	Verification VerificationState
	VerifyReason string
	Err          string
}

// PhaseSnapshot is a UI-safe projection of a Phase.
type PhaseSnapshot struct {
	Name  string
	Steps []StepSnapshot
}

// RunSnapshot is a safe-to-copy projection of a Run for UI consumption. The
// engine produces a fresh RunSnapshot on every state transition.
type RunSnapshot struct {
	ID           string
	Workflow     string
	State        RunState
	Err          string
	Phases       []PhaseSnapshot
	StartedAt    time.Time
	FinishedAt   time.Time
	Computer     string // requested selector; empty means manager default
	ComputerID   string // immutable resolved target id, populated after spawn
	ComputerName string // resolved friendly name, populated after spawn
	WorkingDir   string // canonical resolved workspace root
}

// TargetLabel returns a compact human-facing placement label. Before the
// first job resolves, an explicit Computer selector remains useful; an empty
// label means the jobs.Manager's default workspace has not resolved yet.
func (s RunSnapshot) TargetLabel() string {
	if s.ComputerName != "" {
		return s.ComputerName
	}
	if s.Computer != "" {
		return s.Computer
	}
	if s.ComputerID != "" {
		return s.ComputerID
	}
	return ""
}

// RunUpdate is the payload delivered to Subscribe callbacks on every run
// state change.
type RunUpdate struct {
	Run RunSnapshot
}
