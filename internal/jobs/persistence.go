package jobs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// persistedJob is the on-disk shape for ~/.packetcode/jobs/<id>.json.
// Mirrors Job but uses a stable JSON form so Bucket B/C and future
// versions can decode it without depending on Go field order.
type persistedJob struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	ParentJobID  string    `json:"parent_job_id,omitempty"`
	Prompt       string    `json:"prompt"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	State        string    `json:"state"`
	CreatedAt    time.Time `json:"created_at"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	FinishedAt   time.Time `json:"finished_at,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	Error        string    `json:"error,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	Depth        int       `json:"depth"`
	AllowWrite   bool      `json:"allow_write"`
}

func toPersisted(j *Job) persistedJob {
	return persistedJob{
		ID:           j.ID,
		SessionID:    j.SessionID,
		ParentJobID:  j.ParentJobID,
		Prompt:       j.Prompt,
		Provider:     j.Provider,
		Model:        j.Model,
		State:        j.State.String(),
		CreatedAt:    j.CreatedAt,
		StartedAt:    j.StartedAt,
		FinishedAt:   j.FinishedAt,
		Summary:      j.Summary,
		Error:        j.Error,
		Reason:       j.Reason,
		InputTokens:  j.InputTokens,
		OutputTokens: j.OutputTokens,
		CostUSD:      j.CostUSD,
		Depth:        j.Depth,
		AllowWrite:   j.AllowWrite,
	}
}

func parseState(s string) State {
	switch s {
	case "queued":
		return StateQueued
	case "running":
		return StateRunning
	case "completed":
		return StateCompleted
	case "failed":
		return StateFailed
	case "cancelled":
		return StateCancelled
	}
	return StateFailed
}

func fromPersisted(p persistedJob) *Job {
	return &Job{
		ID:           p.ID,
		SessionID:    p.SessionID,
		ParentJobID:  p.ParentJobID,
		Prompt:       p.Prompt,
		Provider:     p.Provider,
		Model:        p.Model,
		State:        parseState(p.State),
		CreatedAt:    p.CreatedAt,
		StartedAt:    p.StartedAt,
		FinishedAt:   p.FinishedAt,
		Summary:      p.Summary,
		Error:        p.Error,
		Reason:       p.Reason,
		InputTokens:  p.InputTokens,
		OutputTokens: p.OutputTokens,
		CostUSD:      p.CostUSD,
		Depth:        p.Depth,
		AllowWrite:   p.AllowWrite,
	}
}

// saveSnapshot persists a Job to <jobsDir>/<id>.json with atomic
// temp-file-then-rename semantics, mirroring session.Manager.Save.
func saveSnapshot(jobsDir string, j *Job) error {
	if jobsDir == "" {
		return nil
	}
	if err := os.MkdirAll(jobsDir, 0o700); err != nil {
		return fmt.Errorf("save job: ensure dir: %w", err)
	}
	data, err := json.MarshalIndent(toPersisted(j), "", "  ")
	if err != nil {
		return fmt.Errorf("save job: marshal: %w", err)
	}
	final := filepath.Join(jobsDir, j.ID+".json")
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

// loadOrphaned scans jobsDir for any persisted jobs that were Queued or
// Running when the previous app instance exited, rewrites them as
// Cancelled with reason "previous app exit", and returns the count plus
// the resurrected Jobs (so callers can hydrate the in-memory map). The
// resurrected jobs are already in a terminal state.
func loadOrphaned(jobsDir string) ([]*Job, error) {
	if jobsDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(jobsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("load orphans: %w", err)
	}
	var resurrected []*Job
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
			continue
		}
		var p persistedJob
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		state := parseState(p.State)
		if state != StateQueued && state != StateRunning {
			continue
		}
		// Rewrite as Cancelled with the orphan reason.
		j := fromPersisted(p)
		j.State = StateCancelled
		j.Reason = "previous app exit"
		if j.FinishedAt.IsZero() {
			j.FinishedAt = time.Now().UTC()
		}
		if err := saveSnapshot(jobsDir, j); err == nil {
			resurrected = append(resurrected, j)
		}
	}
	return resurrected, nil
}
