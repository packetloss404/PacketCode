package jobs

import (
	"encoding/json"
	"fmt"
	"github.com/packetcode/packetcode/internal/atomicfile"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/packetcode/packetcode/internal/computers"
	"github.com/packetcode/packetcode/internal/tools"
)

// jobFormatVersion is bumped when the on-disk job shape changes
// incompatibly. Readers refuse a newer record rather than silently
// misinterpreting it, matching computers.registryVersion. Records written
// before versioning existed decode as 0 and remain readable.
const jobFormatVersion = 1

// UnreadableRecord names a job file that could not be loaded. Losing a job
// quietly is the failure mode this exists to prevent: a job that was
// abandoned must be reported as such, never as nothing at all.
type UnreadableRecord struct {
	Path   string
	Reason string
}

// persistedJob is the on-disk shape for ~/.packetcode/jobs/<id>.json.
// Mirrors Job but uses a stable JSON form so future versions can decode
// it without depending on Go field order.
type persistedJob struct {
	FormatVersion     int               `json:"format_version"`
	ID                string            `json:"id"`
	SessionID         string            `json:"session_id"`
	ParentJobID       string            `json:"parent_job_id,omitempty"`
	Prompt            string            `json:"prompt"`
	Provider          string            `json:"provider"`
	Model             string            `json:"model"`
	State             string            `json:"state"`
	Seq               int64             `json:"seq,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at,omitempty"`
	StartedAt         time.Time         `json:"started_at,omitempty"`
	FinishedAt        time.Time         `json:"finished_at,omitempty"`
	LastActivity      string            `json:"last_activity,omitempty"`
	LastMessage       string            `json:"last_message,omitempty"`
	NeedsInput        bool              `json:"needs_input,omitempty"`
	NeedsApproval     bool              `json:"needs_approval,omitempty"`
	Summary           string            `json:"summary,omitempty"`
	Error             string            `json:"error,omitempty"`
	Reason            string            `json:"reason,omitempty"`
	AbandonCause      string            `json:"abandon_cause,omitempty"`
	CancelRequest     string            `json:"cancel_request,omitempty"`
	InputTokens       int               `json:"input_tokens"`
	OutputTokens      int               `json:"output_tokens"`
	CostUSD           float64           `json:"cost_usd"`
	Depth             int               `json:"depth"`
	AllowWrite        bool              `json:"allow_write"`
	ComputerID        string            `json:"computer_id,omitempty"`
	ComputerName      string            `json:"computer_name,omitempty"`
	WorkingDir        string            `json:"working_dir,omitempty"`
	WorkspaceIdentity string            `json:"workspace_identity,omitempty"`
	OwnerRoot         string            `json:"owner_root,omitempty"`
	ComputerPolicy    *computers.Policy `json:"computer_policy,omitempty"`
	ResultStatus      string            `json:"result_status,omitempty"`
	Artifacts         []Artifact        `json:"artifacts,omitempty"`
	WorktreePath      string            `json:"worktree_path,omitempty"`
	WorktreeBranch    string            `json:"worktree_branch,omitempty"`
	WorktreeBase      string            `json:"worktree_base,omitempty"`
	WorktreeNote      string            `json:"worktree_note,omitempty"`
	Todos             []TodoItem        `json:"todos,omitempty"`
	Recovered         bool              `json:"recovered,omitempty"`
	ResubmitOf        string            `json:"resubmit_of,omitempty"`
	ResubmittedAs     string            `json:"resubmitted_as,omitempty"`
}

func toPersisted(j *Job) persistedJob {
	return persistedJob{
		FormatVersion:     jobFormatVersion,
		ID:                j.ID,
		SessionID:         j.SessionID,
		ParentJobID:       j.ParentJobID,
		Prompt:            j.Prompt,
		Provider:          j.Provider,
		Model:             j.Model,
		State:             j.State.String(),
		Seq:               j.Seq,
		CreatedAt:         j.CreatedAt,
		UpdatedAt:         j.UpdatedAt,
		StartedAt:         j.StartedAt,
		FinishedAt:        j.FinishedAt,
		LastActivity:      j.LastActivity,
		LastMessage:       j.LastMessage,
		NeedsInput:        j.NeedsInput,
		NeedsApproval:     j.NeedsApproval,
		Summary:           j.Summary,
		Error:             j.Error,
		Reason:            j.Reason,
		AbandonCause:      persistedAbandonCause(j),
		CancelRequest:     string(normalizeCancelRequest(j.CancelRequest)),
		InputTokens:       j.InputTokens,
		OutputTokens:      j.OutputTokens,
		CostUSD:           j.CostUSD,
		Depth:             j.Depth,
		AllowWrite:        j.AllowWrite,
		ComputerID:        j.ComputerID,
		ComputerName:      j.ComputerName,
		WorkingDir:        j.WorkingDir,
		WorkspaceIdentity: j.WorkspaceIdentity,
		OwnerRoot:         j.OwnerRoot,
		ComputerPolicy:    persistedComputerPolicy(j),
		ResultStatus:      normalizeResultStatus(j.ResultStatus).String(),
		Artifacts:         cloneArtifacts(j.Artifacts),
		WorktreePath:      j.WorktreePath,
		WorktreeBranch:    j.WorktreeBranch,
		WorktreeBase:      j.WorktreeBase,
		WorktreeNote:      j.WorktreeNote,
		Todos:             j.Todos(),
		Recovered:         j.Recovered,
		ResubmitOf:        j.ResubmitOf,
		ResubmittedAs:     j.ResubmittedAs,
	}
}

func parseState(s string) State {
	state, _ := parseKnownState(s)
	return state
}

// parseKnownState reports whether the stored state is one this build
// understands. An unrecognised state is not silently flattened to failed:
// a record naming a state we cannot interpret is reported as unreadable so
// the job surfaces as a problem rather than as a wrong answer.
func parseKnownState(s string) (State, bool) {
	switch s {
	case "queued":
		return StateQueued, true
	case "running":
		return StateRunning, true
	case "completed":
		return StateCompleted, true
	case "failed":
		return StateFailed, true
	case "cancelled":
		return StateCancelled, true
	case "abandoned":
		return StateAbandoned, true
	}
	return StateFailed, false
}

func parseResultStatus(s string) ResultStatus {
	switch ResultStatus(s) {
	case ResultStatusPending, ResultStatusSeen, ResultStatusIgnored, ResultStatusInjected, ResultStatusConsumed:
		return ResultStatus(s)
	default:
		return ResultStatusPending
	}
}

func fromPersisted(p persistedJob) *Job {
	updatedAt := p.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = p.FinishedAt
	}
	if updatedAt.IsZero() {
		updatedAt = p.StartedAt
	}
	if updatedAt.IsZero() {
		updatedAt = p.CreatedAt
	}
	return &Job{
		ID:                p.ID,
		SessionID:         p.SessionID,
		ParentJobID:       p.ParentJobID,
		Prompt:            p.Prompt,
		Provider:          p.Provider,
		Model:             p.Model,
		State:             parseState(p.State),
		Seq:               p.Seq,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         updatedAt,
		StartedAt:         p.StartedAt,
		FinishedAt:        p.FinishedAt,
		LastActivity:      p.LastActivity,
		LastMessage:       p.LastMessage,
		NeedsInput:        p.NeedsInput,
		NeedsApproval:     p.NeedsApproval,
		Summary:           p.Summary,
		Error:             p.Error,
		Reason:            p.Reason,
		AbandonCause:      abandonCauseFromPersisted(p),
		CancelRequest:     normalizeCancelRequest(CancelRequest(p.CancelRequest)),
		InputTokens:       p.InputTokens,
		OutputTokens:      p.OutputTokens,
		CostUSD:           p.CostUSD,
		Depth:             p.Depth,
		AllowWrite:        p.AllowWrite,
		ComputerID:        p.ComputerID,
		ComputerName:      p.ComputerName,
		WorkingDir:        p.WorkingDir,
		WorkspaceIdentity: p.WorkspaceIdentity,
		OwnerRoot:         p.OwnerRoot,
		ComputerPolicy:    computerPolicyFromPersisted(p.ComputerPolicy),
		ResultStatus:      parseResultStatus(p.ResultStatus),
		Artifacts:         cloneArtifacts(p.Artifacts),
		WorktreePath:      p.WorktreePath,
		WorktreeBranch:    p.WorktreeBranch,
		WorktreeBase:      p.WorktreeBase,
		WorktreeNote:      p.WorktreeNote,
		Recovered:         p.Recovered,
		ResubmitOf:        p.ResubmitOf,
		ResubmittedAs:     p.ResubmittedAs,
		// Seeded rather than left nil: a recovered job's plan is evidence of
		// what it was part-way through, which is exactly what the reader of an
		// abandoned job wants to see.
		todos: todoStoreFrom(p.Todos),
	}
}

// todoStoreFrom rebuilds a store from persisted items. Invalid entries are
// dropped rather than rejected: unlike the job's State, a malformed todo says
// nothing about what happened to the work, so losing the whole record over one
// bad line would trade real evidence for a cosmetic detail.
func todoStoreFrom(items []TodoItem) *tools.TodoStore {
	store := tools.NewTodoStore()
	if len(items) == 0 {
		return store
	}
	kept := make([]TodoItem, 0, len(items))
	for _, item := range items {
		if item.Content == "" {
			continue
		}
		switch item.Status {
		case tools.TodoPending, tools.TodoInProgress, tools.TodoCompleted:
			kept = append(kept, item)
		}
	}
	store.Replace(kept)
	return store
}

// persistedAbandonCause writes a cause only for abandoned jobs. Stamping one
// on any other state would leave a claim about an outcome that was in fact
// confirmed, and omitempty keeps every existing record byte-identical.
func persistedAbandonCause(j *Job) string {
	if j == nil || j.State != StateAbandoned {
		return ""
	}
	return string(normalizeAbandonCause(j.AbandonCause))
}

// abandonCauseFromPersisted mirrors the write side. A record that is not
// abandoned carries no cause, and an abandoned record with an absent or
// unreadable cause reads back as Unknown — which is the truthful answer, not
// a fallback.
func abandonCauseFromPersisted(p persistedJob) AbandonCause {
	if parseState(p.State) != StateAbandoned {
		return ""
	}
	return normalizeAbandonCause(AbandonCause(p.AbandonCause))
}

func persistedComputerPolicy(j *Job) *computers.Policy {
	if j == nil || j.ComputerID == "" {
		return nil
	}
	policy := j.ComputerPolicy
	return &policy
}

func computerPolicyFromPersisted(policy *computers.Policy) computers.Policy {
	if policy == nil {
		return computers.Policy{}
	}
	return *policy
}

// saveSnapshot persists a Job to <jobsDir>/<id>.json with atomic
// temp-file-then-rename semantics, mirroring session.Manager.Save.
func saveSnapshot(jobsDir string, j *Job) error {
	return savePersistedSnapshot(jobsDir, toPersisted(j))
}

func savePersistedSnapshot(jobsDir string, p persistedJob) error {
	if jobsDir == "" {
		return nil
	}
	if err := os.MkdirAll(jobsDir, 0o700); err != nil {
		return fmt.Errorf("save job: ensure dir: %w", err)
	}
	final := filepath.Join(jobsDir, p.ID+".json")
	if existing, ok := readPersistedJob(final); ok {
		if existing.FormatVersion > jobFormatVersion {
			return fmt.Errorf(
				"save job %s: on-disk record version %d is newer than this build supports (%d)",
				p.ID, existing.FormatVersion, jobFormatVersion,
			)
		}
		if p.Seq > 0 && existing.Seq > p.Seq {
			return nil
		}
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("save job: marshal: %w", err)
	}
	// fsynced before the rename, for the same reason as a session file: a job
	// record that exists and is empty after a crash is the state a restart
	// reconciles from, and it would reconcile from nothing.
	if err := atomicfile.Write(final, data, 0o600, ".job.*.json.tmp"); err != nil {
		return fmt.Errorf("save job: %w", err)
	}
	return nil
}

func readPersistedJob(path string) (persistedJob, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return persistedJob{}, false
	}
	var p persistedJob
	if err := json.Unmarshal(data, &p); err != nil {
		return persistedJob{}, false
	}
	return p, true
}

// decodeRecordFile reads and validates one record without writing anything.
// Both the loader and the read-only inspector go through it so a record that
// one of them calls unreadable is never quietly accepted by the other.
func decodeRecordFile(path string) (persistedJob, State, *UnreadableRecord) {
	data, err := os.ReadFile(path)
	if err != nil {
		return persistedJob{}, StateFailed, &UnreadableRecord{Path: path, Reason: err.Error()}
	}
	var p persistedJob
	if err := json.Unmarshal(data, &p); err != nil {
		return persistedJob{}, StateFailed, &UnreadableRecord{Path: path, Reason: "malformed job record: " + err.Error()}
	}
	if p.FormatVersion > jobFormatVersion {
		return persistedJob{}, StateFailed, &UnreadableRecord{
			Path: path,
			Reason: fmt.Sprintf("job record version %d is newer than this build supports (%d)",
				p.FormatVersion, jobFormatVersion),
		}
	}
	state, known := parseKnownState(p.State)
	if !known {
		return persistedJob{}, StateFailed, &UnreadableRecord{
			Path:   path,
			Reason: fmt.Sprintf("unrecognised job state %q", p.State),
		}
	}
	return p, state, nil
}

// InspectRecords reports how many job records in jobsDir this build can read
// and which it cannot, without loading, reconciling, or writing anything.
//
// The read-only guarantee is the entire point. NewManager reconciles: it
// rewrites records left queued or running by a previous exit and saves them
// back. A diagnostic that went through NewManager would therefore mark a
// live instance's in-flight jobs as abandoned merely by being run, and the
// owner-root scoping does not help when the diagnostic shares that root.
// Callers that only want to report on the directory must use this.
func InspectRecords(jobsDir string) (readable int, unreadable []UnreadableRecord, err error) {
	if jobsDir == "" {
		return 0, nil, nil
	}
	entries, err := os.ReadDir(jobsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil, nil
		}
		return 0, nil, fmt.Errorf("inspect jobs: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if strings.HasPrefix(e.Name(), ".job.") {
			continue
		}
		if _, _, bad := decodeRecordFile(filepath.Join(jobsDir, e.Name())); bad != nil {
			unreadable = append(unreadable, *bad)
			continue
		}
		readable++
	}
	return readable, unreadable, nil
}

// loadOrphaned scans jobsDir for any persisted jobs that were Queued or
// Running when the previous app instance exited and rewrites them with
// reason "previous app exit": Running becomes Abandoned (its outcome was
// never witnessed), Queued becomes Cancelled (it provably never ran).
// Returns the resurrected Jobs so callers can hydrate the in-memory map.
// The resurrected jobs are already in a terminal state and are never
// resumed — resubmitting one creates a new job.
func loadOrphaned(jobsDir string) ([]*Job, error) {
	_, recovered, _, err := loadPersistedJobs(jobsDir, "")
	return recovered, err
}

// loadPersistedJobs reads every job record in jobsDir. ownerRoot is the
// project root of the calling instance; records created by an instance
// rooted elsewhere are left strictly alone, because the state directory is
// shared across projects and another live instance may still be running
// them. An empty ownerRoot disables scoping.
//
// Records that cannot be read are returned rather than skipped: a job file
// that vanishes without a word is indistinguishable from a job that never
// existed, which is the one thing job reporting must never do.
func loadPersistedJobs(jobsDir, ownerRoot string) ([]*Job, []*Job, []UnreadableRecord, error) {
	if jobsDir == "" {
		return nil, nil, nil, nil
	}
	entries, err := os.ReadDir(jobsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, fmt.Errorf("load jobs: %w", err)
	}
	var loaded []*Job
	var recovered []*Job
	var unreadable []UnreadableRecord
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		// Skip our own temp-file pattern from interrupted writes.
		if strings.HasPrefix(e.Name(), ".job.") {
			continue
		}
		path := filepath.Join(jobsDir, e.Name())
		p, state, bad := decodeRecordFile(path)
		if bad != nil {
			unreadable = append(unreadable, *bad)
			continue
		}
		if (state == StateQueued || state == StateRunning) && !ownedByRoot(p, ownerRoot) {
			// Another instance created this job and may still be running it.
			// Rewriting it here would report someone else's live work as
			// abandoned and make it eligible for a duplicate resubmit.
			continue
		}
		j := fromPersisted(p)
		if state == StateQueued || state == StateRunning {
			// A queued job never left the semaphore, so "it did not run" is
			// a confirmed outcome and Cancelled is honest. A running job had
			// started work that nothing here witnessed the end of, so the
			// only truthful verdict is Abandoned — packetcode does not resume
			// it and must not claim it was cancelled on purpose.
			if state == StateRunning {
				j.State = StateAbandoned
				j.AbandonCause = AbandonCauseAppExit
			} else {
				j.State = StateCancelled
			}
			j.Recovered = true
			j.Reason = "previous app exit"
			if j.FinishedAt.IsZero() {
				j.FinishedAt = time.Now().UTC()
			}
			if j.UpdatedAt.IsZero() || j.FinishedAt.After(j.UpdatedAt) {
				j.UpdatedAt = j.FinishedAt
			}
			j.LastActivity = j.State.String()
			j.LastMessage = j.Reason
			j.NeedsInput = false
			j.NeedsApproval = false
			if err := saveSnapshot(jobsDir, j); err != nil {
				unreadable = append(unreadable, UnreadableRecord{
					Path:   path,
					Reason: "could not record abandonment: " + err.Error(),
				})
				continue
			}
			recovered = append(recovered, j)
		}
		loaded = append(loaded, j)
	}
	return loaded, recovered, unreadable, nil
}

// ownedByRoot reports whether a record belongs to the instance rooted at
// ownerRoot. Records written before ownership was tracked carry no root and
// are treated as owned, so upgrading does not strand existing jobs.
func ownedByRoot(p persistedJob, ownerRoot string) bool {
	return ownerRoot == "" || p.OwnerRoot == "" || p.OwnerRoot == ownerRoot
}
