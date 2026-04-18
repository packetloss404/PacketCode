package jobs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
)

// writeInitialSubSession persists a fresh session.Session for the job
// under its deterministic SessionID so that session.Manager.Load(id)
// can adopt it as Current. Mirrors the atomic temp-file-then-rename
// pattern session.Manager.Save uses internally.
func writeInitialSubSession(sessionsDir string, j *Job) error {
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return fmt.Errorf("subsession: mkdir: %w", err)
	}
	now := time.Now().UTC()
	s := session.Session{
		ID:        j.SessionID,
		Name:      "job-" + j.ID,
		CreatedAt: now,
		UpdatedAt: now,
		Provider:  j.Provider,
		Model:     j.Model,
		Messages:  []provider.Message{},
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("subsession: marshal: %w", err)
	}
	final := filepath.Join(sessionsDir, j.SessionID+".json")
	tmp, err := os.CreateTemp(sessionsDir, ".session.*.json.tmp")
	if err != nil {
		return fmt.Errorf("subsession: temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("subsession: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("subsession: close: %w", err)
	}
	if err := os.Rename(tmpPath, final); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("subsession: rename: %w", err)
	}
	return nil
}
