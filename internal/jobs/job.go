// Package jobs owns the lifecycle of background ("spawned") agents.
//
// A Job is a fully isolated mini-agent: its own session.Manager, its own
// session.BackupManager, its own tools.Registry (cloned from the main
// one), and its own context.CancelFunc. The Manager governs concurrency
// (sem-bounded MaxConcurrent), recursion (Depth ≤ MaxDepth), the lifetime
// cap (MaxTotal), persistence to ~/.packetcode/jobs/, and the asynchronous
// fan-out of state-transition Snapshots to the UI.
//
// See docs/feature-background-agents.md for the full design.
package jobs

import (
	"time"

	"github.com/packetcode/packetcode/internal/computers"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/tools"
)

// State enumerates the lifecycle of a Job. Terminal states (Completed,
// Failed, Cancelled, Abandoned) never transition further.
//
// Cancelled and Abandoned are deliberately distinct. Cancelled means the
// stop was asked for and the outcome is known: either a human cancelled it
// or it never started. Abandoned means work had begun and packetcode cannot
// confirm how it ended — the honest verdict when a transport dies or the app
// exits mid-run. Flattening the second into the first would report a
// confirmed cancellation that nobody confirmed.
//
// Persisted as the String() value, never the int, so appending here is safe
// for existing records on disk.
type State int

const (
	StateQueued    State = iota // accepted, not yet running (concurrency limit)
	StateRunning                // worker goroutine started, agent loop active
	StateCompleted              // agent emitted EventDone
	StateFailed                 // EventError or panic
	StateCancelled              // stop was requested and the job is known to have stopped
	StateAbandoned              // work began; packetcode cannot confirm the outcome
)

// String renders a State for logs and the /jobs panel.
func (s State) String() string {
	switch s {
	case StateQueued:
		return "queued"
	case StateRunning:
		return "running"
	case StateCompleted:
		return "completed"
	case StateFailed:
		return "failed"
	case StateCancelled:
		return "cancelled"
	case StateAbandoned:
		return "abandoned"
	default:
		return "unknown"
	}
}

// IsTerminal reports whether s is a terminal state. The worker stops
// publishing snapshots once a job reaches a terminal state.
func (s State) IsTerminal() bool {
	switch s {
	case StateCompleted, StateFailed, StateCancelled, StateAbandoned:
		return true
	}
	return false
}

// IsSuccess reports whether s is a terminal state that finished its work.
// Callers deciding "did this agent succeed?" must use this rather than
// testing for known failure states: an allowlist of failures silently
// reports every state added later as a success.
func (s State) IsSuccess() bool { return s == StateCompleted }

// AbandonCause records why a job was abandoned. It is a string enum for the
// same reason ComputerPolicy's approval axis is: the safe value must survive
// an absent JSON field, and "" decodes to CauseUnknown rather than to a
// confident claim.
type AbandonCause string

const (
	// AbandonCauseUnknown is the default. It says only that work began and
	// the outcome was never confirmed.
	AbandonCauseUnknown AbandonCause = "unknown"
	// AbandonCauseAppExit is set when packetcode itself stopped: a shutdown
	// that cancelled a running job, or reconciliation of a record left
	// active by an unclean exit.
	AbandonCauseAppExit AbandonCause = "app-exit"
	// AbandonCauseTransportLost is set when the job's transport died while
	// work was in flight. On SSH this is the case packetcode explicitly
	// cannot resolve: a detached remote descendant may still be running.
	AbandonCauseTransportLost AbandonCause = "transport-lost"
)

// normalizeAbandonCause coerces an unrecognised cause to Unknown rather than
// rejecting the record. The cause is descriptive detail hung off the state;
// coercing it loses nothing that was ever claimed, because "unknown" is
// exactly what an unreadable cause means. The State itself is not treated
// this way — see parseKnownState, which reports rather than coerces.
func normalizeAbandonCause(c AbandonCause) AbandonCause {
	switch c {
	case AbandonCauseUnknown, AbandonCauseAppExit, AbandonCauseTransportLost:
		return c
	default:
		return AbandonCauseUnknown
	}
}

// CancelRequest records that a stop was explicitly asked for, and by whom.
// It is stamped and persisted *before* the context is cancelled, because the
// context itself carries no cause: user cancel, /cancel all, app shutdown,
// and a dead transport all surface as an identical context.Canceled. Without
// this field the worker cannot tell a deliberate cancellation from a loss.
type CancelRequest string

const (
	CancelRequestNone     CancelRequest = ""         // nobody asked; a stop here is a loss
	CancelRequestUser     CancelRequest = "user"     // /cancel <id> or /cancel all
	CancelRequestShutdown CancelRequest = "shutdown" // the app is exiting
)

// snapshotAbandonCause exposes a cause only for abandoned jobs. Normalizing
// unconditionally would stamp "unknown" onto every completed and cancelled
// job, so the UI would show a reason-for-abandonment on work that was never
// abandoned.
func snapshotAbandonCause(j *Job) AbandonCause {
	if j == nil || j.State != StateAbandoned {
		return ""
	}
	return normalizeAbandonCause(j.AbandonCause)
}

// normalizeCancelRequest coerces an unrecognised request to None. An
// unreadable request is not evidence that anyone asked to stop, and None is
// the value that keeps the worker honest: with no recorded request, a stop
// is classified as a loss rather than as a confirmed cancellation.
func normalizeCancelRequest(c CancelRequest) CancelRequest {
	switch c {
	case CancelRequestUser, CancelRequestShutdown:
		return c
	default:
		return CancelRequestNone
	}
}

// ResultStatus records how a terminal job result has been handled after
// completion. Pending/seen results remain available for an explicit
// Agent View decision; ignored/injected/consumed results are final.
type ResultStatus string

const (
	ResultStatusPending  ResultStatus = "pending"
	ResultStatusSeen     ResultStatus = "seen"
	ResultStatusIgnored  ResultStatus = "ignored"
	ResultStatusInjected ResultStatus = "injected"
	ResultStatusConsumed ResultStatus = "consumed"
)

func (s ResultStatus) String() string {
	if s == "" {
		return string(ResultStatusPending)
	}
	return string(s)
}

func normalizeResultStatus(s ResultStatus) ResultStatus {
	switch s {
	case ResultStatusPending, ResultStatusSeen, ResultStatusIgnored, ResultStatusInjected, ResultStatusConsumed:
		return s
	default:
		return ResultStatusPending
	}
}

// Job is the in-memory record for a single background agent run. The
// Manager owns the canonical Job; UI/test code should consume Snapshots
// to avoid sharing mutable state.
type Job struct {
	ID            string // 8-char short id, also the subsession suffix
	SessionID     string // full id of the job's underlying session.Session
	ParentJobID   string // "" when spawned from the main session
	Prompt        string // initial user message
	Provider      string // slug; may differ from main session
	Model         string // model id under that provider
	State         State
	CreatedAt     time.Time
	StartedAt     time.Time
	FinishedAt    time.Time
	UpdatedAt     time.Time
	Summary       string        // short result summary surfaced into main convo
	Error         string        // populated on StateFailed and on StateAbandoned
	Reason        string        // free-form; "previous app exit" / "app shutdown" / etc.
	AbandonCause  AbandonCause  // why the outcome is unknown; set with StateAbandoned
	CancelRequest CancelRequest // durable record that a stop was asked for
	LastActivity  string        // concise activity label for dashboards
	LastMessage   string        // latest human-visible text/result snippet
	NeedsInput    bool          // true while a job is blocked on user action
	NeedsApproval bool          // true while a job is blocked on tool approval
	Seq           int64         // monotonic snapshot sequence for stale-update guards
	InputTokens   int
	OutputTokens  int
	// Cache counts are subsets of InputTokens, kept so the cost estimate
	// can price them at the cached rate.
	CacheReadTokens     int
	CacheCreationTokens int
	CostUSD             float64
	Depth               int                // 0 for main-spawned, parent.Depth+1 otherwise
	Transcript          []provider.Message // snapshot taken when state becomes terminal
	AllowWrite          bool               // tracks whether destructive tools were enabled
	ComputerID          string             // stable Packet Computer id; empty for local jobs
	ComputerName        string             // display name captured at spawn time
	WorkingDir          string             // immutable local or remote workspace root
	WorkspaceIdentity   string             // immutable endpoint+root identity for resubmit safety
	OwnerRoot           string             // project root of the instance that created the job
	ComputerPolicy      computers.Policy   // conservative per-computer policy captured at spawn
	ResultStatus        ResultStatus       // pending/seen/ignored/injected/consumed after terminal result exists
	Artifacts           []Artifact         // bounded structured refs captured from tool execution
	WorktreePath        string             // per-job git worktree root when write isolation is active
	WorktreeBranch      string             // branch checked out by the worktree
	WorktreeBase        string             // base ref/SHA used to create the worktree
	WorktreeNote        string             // fallback or setup note when no worktree was created

	// Reconcile lineage. A job left active by a previous process exit is
	// rewritten as Abandoned with cause app-exit and marked Recovered; it is
	// never resumed. Resubmitting one creates a brand-new job, and the two
	// records are linked in both directions so neither pretends the old run
	// continued. Records written before the Abandoned state existed carry
	// Cancelled + Recovered and are still honoured on read.
	Recovered     bool   // reconciled from a previous app exit
	ResubmitOf    string // id of the recovered job this job was resubmitted from
	ResubmittedAs string // id of the new job created from this recovered job

	// todos is the job's own todo_write list. Allocated once at spawn (or
	// seeded from disk on reload) and never reassigned, so the worker
	// goroutine writing through the tool and the manager reading it under
	// m.mu are both safe: TodoStore guards its own contents and List returns
	// a copy. It is a pointer rather than a slice so the tool and the Job
	// observe the same list without the manager having to be told about
	// every write.
	todos *tools.TodoStore
}

// Todos returns the job's current todo list, or nil when it has none.
func (j *Job) Todos() []TodoItem {
	if j == nil {
		return nil
	}
	return j.todos.List()
}

// TodoItem and the todo statuses are re-exported so UI code can render a
// job's plan without importing internal/tools, which owns the tool rather than
// the job record.
type TodoItem = tools.TodoItem

const (
	TodoPending    = tools.TodoPending
	TodoInProgress = tools.TodoInProgress
	TodoCompleted  = tools.TodoCompleted
)

// Snapshot is a safe-to-copy projection of Job for UI consumption. It
// shares no mutable state with the underlying Job — Manager produces a
// fresh Snapshot on every state transition.
type Snapshot struct {
	ID, ParentJobID, Prompt, Provider, Model, Summary, Error string
	ComputerID, ComputerName, WorkingDir                     string
	WorkspaceIdentity                                        string
	LastActivity, LastMessage                                string
	State                                                    State
	Todos                                                    []TodoItem
	AbandonCause                                             AbandonCause
	ResultStatus                                             ResultStatus
	CreatedAt, StartedAt, FinishedAt, UpdatedAt              time.Time
	Tokens                                                   struct{ Input, Output int }
	CostUSD                                                  float64
	Depth                                                    int
	NeedsInput, NeedsApproval, AllowWrite                    bool
	Seq                                                      int64
	Artifacts                                                []Artifact
	WorktreePath, WorktreeBranch, WorktreeBase, WorktreeNote string
	Recovered                                                bool
	ResubmitOf, ResubmittedAs                                string
}

// AwaitingApproval, AwaitingAnswer and Blocked split the one "this job is
// waiting on a human" bit into the two things it can actually mean.
//
// A pending tool approval sets NeedsApproval alone; NeedsInput is reserved for
// "the agent asked you something", which is the signal the background-question
// feature needs a place for. AwaitingAnswer is deliberately the *residue*: it
// becomes true the moment a writer sets NeedsInput without NeedsApproval, and
// stays false until one does, so no caller has to be updated when that happens.
// (Until audit patch P05 every writer set both, so it was never true.)
func (s Snapshot) AwaitingApproval() bool { return s.NeedsApproval }

// AwaitingAnswer reports a job blocked on something other than a tool
// approval — a question for the user.
func (s Snapshot) AwaitingAnswer() bool { return s.NeedsInput && !s.NeedsApproval }

// Blocked reports whether the job is waiting on a human at all. Callers that
// group or count "needs input" want this, not either field on its own.
func (s Snapshot) Blocked() bool { return s.NeedsInput || s.NeedsApproval }

// snapshotOf builds a Snapshot from a Job. Caller must hold the Manager's
// read lock (or otherwise know the Job is not being mutated).
func snapshotOf(j *Job) Snapshot {
	s := Snapshot{
		ID:                j.ID,
		ParentJobID:       j.ParentJobID,
		Prompt:            j.Prompt,
		Provider:          j.Provider,
		Model:             j.Model,
		Summary:           j.Summary,
		Error:             j.Error,
		State:             j.State,
		Todos:             j.Todos(),
		AbandonCause:      snapshotAbandonCause(j),
		ResultStatus:      normalizeResultStatus(j.ResultStatus),
		CreatedAt:         j.CreatedAt,
		StartedAt:         j.StartedAt,
		FinishedAt:        j.FinishedAt,
		UpdatedAt:         j.UpdatedAt,
		CostUSD:           j.CostUSD,
		Depth:             j.Depth,
		LastActivity:      j.LastActivity,
		LastMessage:       j.LastMessage,
		NeedsInput:        j.NeedsInput,
		NeedsApproval:     j.NeedsApproval,
		AllowWrite:        j.AllowWrite,
		ComputerID:        j.ComputerID,
		ComputerName:      j.ComputerName,
		WorkingDir:        j.WorkingDir,
		WorkspaceIdentity: j.WorkspaceIdentity,
		Seq:               j.Seq,
		Artifacts:         cloneArtifacts(j.Artifacts),
		WorktreePath:      j.WorktreePath,
		WorktreeBranch:    j.WorktreeBranch,
		WorktreeBase:      j.WorktreeBase,
		WorktreeNote:      j.WorktreeNote,
		Recovered:         j.Recovered,
		ResubmitOf:        j.ResubmitOf,
		ResubmittedAs:     j.ResubmittedAs,
	}
	s.Tokens.Input = j.InputTokens
	s.Tokens.Output = j.OutputTokens
	return s
}
