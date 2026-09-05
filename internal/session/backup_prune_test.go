package session

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeBackupTree builds a session backup tree under base and stamps every
// entry in it with mod.
//
// The stamping runs deepest-first because creating a child refreshes its
// parent's mtime, so a parent stamped before its children would not stay
// stamped.
func writeBackupTree(t *testing.T, base, sessionID string, files []string, mod time.Time) string {
	t.Helper()
	root := filepath.Join(base, sessionID)
	for _, rel := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte("backup"), 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	var paths []string
	if err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, p)
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	for i := len(paths) - 1; i >= 0; i-- {
		if err := os.Chtimes(paths[i], mod, mod); err != nil {
			t.Fatalf("chtimes %s: %v", paths[i], err)
		}
	}
	return root
}

func requireExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to survive pruning: %v", path, err)
	}
}

func requireGone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be pruned, stat error = %v", path, err)
	}
}

func TestPruneBackups_RemovesStaleTreesOnly(t *testing.T) {
	base := t.TempDir()
	stale := writeBackupTree(t, base, "sess-stale", []string{"src/a/file.bak"}, time.Now().Add(-30*24*time.Hour))
	recent := writeBackupTree(t, base, "sess-recent", []string{"src/file.bak"}, time.Now().Add(-time.Hour))

	// The active session's tree is deliberately older than the window: a
	// resumed session can be months old, and deleting the tree it is about to
	// write into would break undo for the run that is starting.
	current := writeBackupTree(t, base, "sess-current", []string{"src/file.bak"}, time.Now().Add(-90*24*time.Hour))

	if got := PruneBackups(base, "sess-current", 14*24*time.Hour); got != 1 {
		t.Fatalf("PruneBackups removed %d trees, want 1", got)
	}
	requireGone(t, stale)
	requireExists(t, recent)
	requireExists(t, current)
}

// A backup is written into a mirror of the source tree, so the session
// directory's own mtime can be old while the tree is actively in use. Judging
// age by the top directory alone would delete a tree that was just written.
func TestPruneBackups_DeepWriteKeepsTree(t *testing.T) {
	base := t.TempDir()
	root := writeBackupTree(t, base, "sess", []string{"deep/nested/file.bak"}, time.Now().Add(-30*24*time.Hour))
	now := time.Now()
	if err := os.Chtimes(filepath.Join(root, "deep", "nested", "file.bak"), now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if got := PruneBackups(base, "", 14*24*time.Hour); got != 0 {
		t.Fatalf("PruneBackups removed %d trees, want 0", got)
	}
	requireExists(t, root)
}

func TestPruneBackups_ZeroAgePrunesNothing(t *testing.T) {
	base := t.TempDir()
	root := writeBackupTree(t, base, "sess", []string{"file.bak"}, time.Now().Add(-365*24*time.Hour))

	for _, maxAge := range []time.Duration{0, -time.Hour} {
		if got := PruneBackups(base, "", maxAge); got != 0 {
			t.Fatalf("PruneBackups(maxAge=%s) removed %d trees, want 0", maxAge, got)
		}
	}
	requireExists(t, root)
}

// Pruning is best effort: a backups directory that was never created is not an
// error, it is simply nothing to reclaim.
func TestPruneBackups_MissingBaseIsNotAnError(t *testing.T) {
	if got := PruneBackups(filepath.Join(t.TempDir(), "never-created"), "", time.Hour); got != 0 {
		t.Fatalf("PruneBackups removed %d trees, want 0", got)
	}
}

// Only directories are candidates, so a stray file sharing the backups
// directory is left alone however old it is.
func TestPruneBackups_IgnoresPlainFiles(t *testing.T) {
	base := t.TempDir()
	stray := filepath.Join(base, "notes.txt")
	if err := os.WriteFile(stray, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(stray, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if got := PruneBackups(base, "", 14*24*time.Hour); got != 0 {
		t.Fatalf("PruneBackups removed %d trees, want 0", got)
	}
	requireExists(t, stray)
}
