package jobs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/packetcode/packetcode/internal/computers"
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
	}
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
	tmp, err := os.CreateTemp(jobsDir, ".job.*.json.tmp")
	if err != nil {
		return fmt.Errorf("save job: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("save job: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("save job: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, final); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("save job: rename: %w", err)
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

// loadOrphaned scans jobsDir for any persisted jobs that were Queued or
// Running when the previous app instance exited, rewrites them as
// Cancelled with reason "previous app exit", and returns the count plus
// the resurrected Jobs (so callers can hydrate the in-memory map). The
// resurrected jobs are already in a terminal state.
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
		data, err := os.ReadFile(path)
		if err != nil {
			unreadable = append(unreadable, UnreadableRecord{Path: path, Reason: err.Error()})
			continue
		}
		var p persistedJob
		if err := json.Unmarshal(data, &p); err != nil {
			unreadable = append(unreadable, UnreadableRecord{Path: path, Reason: "malformed job record: " + err.Error()})
			continue
		}
		if p.FormatVersion > jobFormatVersion {
			unreadable = append(unreadable, UnreadableRecord{
				Path: path,
				Reason: fmt.Sprintf("job record version %d is newer than this build supports (%d)",
					p.FormatVersion, jobFormatVersion),
			})
			continue
		}
		state, known := parseKnownState(p.State)
		if !known {
			unreadable = append(unreadable, UnreadableRecord{
				Path:   path,
				Reason: fmt.Sprintf("unrecognised job state %q", p.State),
			})
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
			j.State = StateCancelled
			j.Recovered = true
			j.Reason = "previous app exit"
			if j.FinishedAt.IsZero() {
				j.FinishedAt = time.Now().UTC()
			}
			if j.UpdatedAt.IsZero() || j.FinishedAt.After(j.UpdatedAt) {
				j.UpdatedAt = j.FinishedAt
			}
			j.LastActivity = "cancelled"
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
