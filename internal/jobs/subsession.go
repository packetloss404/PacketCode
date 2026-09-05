package jobs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/packetcode/packetcode/internal/atomicfile"
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
		ID:                j.SessionID,
		Name:              "job-" + j.ID,
		CreatedAt:         now,
		UpdatedAt:         now,
		Provider:          j.Provider,
		Model:             j.Model,
		ComputerID:        j.ComputerID,
		WorkingDir:        j.WorkingDir,
		WorkspaceIdentity: j.WorkspaceIdentity,
		Messages:          []provider.Message{},
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("subsession: marshal: %w", err)
	}
	final := filepath.Join(sessionsDir, j.SessionID+".json")
	// Same fsync-backed write as the foreground session store: a crash must
	// not leave a correctly named, empty transcript for the job.
	if err := atomicfile.Write(final, data, 0o600, ".session.*.json.tmp"); err != nil {
		return fmt.Errorf("subsession: %w", err)
	}
	return nil
}
