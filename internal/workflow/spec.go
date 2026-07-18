// Package workflow is a Go-native multi-agent workflow engine layered over
// the existing background-jobs manager (internal/jobs). It orchestrates a
// Workflow — a sequence of Phases, each holding Steps that run either a
// single agent or a parallel fan-out of agents — by spawning jobs on the
// jobs.Manager and joining their results. The engine adds no agent-execution
// machinery of its own: every agent is an ordinary background job.
//
// Phase 2 scope: sequential phases, single-agent steps, and parallel fan-out
// steps, plus a live view. Pipelines (multi-stage chains) and adversarial
// verification are deferred to a later phase; the Step type declares the
// (currently unused) Stages/Verify fields as clean extension points.
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

// Step is one unit of work within a Phase.
type Step struct {
	Name   string
	Mode   StepMode
	Agent  AgentSpec
	FanOut []string // items for StepParallel; each renders {{.item}}
	Bind   string   // name to bind this step's summaries under (default: Name)

	// Stages and Verify are declared for later phases (pipelines +
	// adversarial verify) and are unused in Phase 2. They are safe to set
	// in a spec but the engine ignores them.
	Stages []string
	Verify bool
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
	Name   string
	Inputs map[string]string
	Phases []Phase
}

// StepResult is the terminal outcome of a Step: the joined agent results
// (one per fan-out item, or a single element for StepSingle) plus any error
// that failed the step.
type StepResult struct {
	Step   string
	Mode   StepMode
	Agents []jobs.Result
	JobIDs []string
	Err    error
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
// agent-view rendering; Err, when non-empty, is a spawn error recorded before
// any job existed.
type AgentSnapshot struct {
	JobID  string
	Job    jobs.Snapshot
	HasJob bool
	Err    string
}

// StepSnapshot is a UI-safe projection of a Step.
type StepSnapshot struct {
	Name   string
	Mode   StepMode
	Agents []AgentSnapshot
	Err    string
}

// PhaseSnapshot is a UI-safe projection of a Phase.
type PhaseSnapshot struct {
	Name  string
	Steps []StepSnapshot
}

// RunSnapshot is a safe-to-copy projection of a Run for UI consumption. The
// engine produces a fresh RunSnapshot on every state transition.
type RunSnapshot struct {
	ID         string
	Workflow   string
	State      RunState
	Err        string
	Phases     []PhaseSnapshot
	StartedAt  time.Time
	FinishedAt time.Time
}

// RunUpdate is the payload delivered to Subscribe callbacks on every run
// state change.
type RunUpdate struct {
	Run RunSnapshot
}
