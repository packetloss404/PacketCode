package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BackupManager snapshots files before destructive tools modify them and
// restores them on /undo. Backups are scoped per session — when a session
// is deleted, its entire backup tree goes with it.
//
// The undo stack is in-memory and reset when the manager is constructed.
// That's fine for the MVP: undo only exists within a single packetcode
// session. Persisting the stack across restarts is post-MVP work.
type BackupManager struct {
	root  string // ~/.packetcode/backups/<session-id>
	mu    sync.Mutex
	stack []entry
}

type entry struct {
	timestamp  int64
	original   string // absolute path of original file
	backupPath string // absolute path of .bak under root
	preExisted bool   // false → file was created (undo = delete)
	mode       os.FileMode
}

// NewBackupManager constructs a manager rooted at <backupsDir>/<sessionID>.
// The directory is created on first Backup call, not here, to avoid
// littering the filesystem with empty session dirs.
func NewBackupManager(backupsDir, sessionID string) *BackupManager {
	return &BackupManager{
		root: filepath.Join(backupsDir, sessionID),
	}
}

// Backup snapshots filePath into the session's backup tree. If the file
// does not exist (i.e. about to be created for the first time), we still
// push an entry so /undo can delete the new file.
func (b *BackupManager) Backup(filePath string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	abs, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("backup: resolve path: %w", err)
	}

	if err := os.MkdirAll(b.root, 0o700); err != nil {
		return fmt.Errorf("backup: create dir: %w", err)
	}

	timestamp := time.Now().UnixNano()
	hash := sha256.Sum256([]byte(abs))
	backupName := fmt.Sprintf("%d-%s.bak", timestamp, hex.EncodeToString(hash[:])[:12])
	backupPath := filepath.Join(b.root, backupName)

	src, err := os.Open(abs)
	if err != nil {
		if os.IsNotExist(err) {
			b.stack = append(b.stack, entry{
				timestamp:  timestamp,
				original:   abs,
				backupPath: "",
				preExisted: false,
			})
			return nil
		}
		return fmt.Errorf("backup: open original: %w", err)
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("backup: stat original: %w", err)
	}

	dst, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("backup: create snapshot: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(backupPath)
		return fmt.Errorf("backup: copy: %w", err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("backup: close snapshot: %w", err)
	}

	b.stack = append(b.stack, entry{
		timestamp:  timestamp,
		original:   abs,
		backupPath: backupPath,
		preExisted: true,
		mode:       info.Mode().Perm(),
	})
	return nil
}

// RollbackBackup removes the most recent backup for filePath. Tools call this
// after a failed mutation so failed writes do not become undoable operations.
func (b *BackupManager) RollbackBackup(filePath string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	abs, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("backup rollback: resolve path: %w", err)
	}
	if len(b.stack) == 0 {
		return nil
	}
	last := b.stack[len(b.stack)-1]
	if last.original != abs {
		return nil
	}
	b.stack = b.stack[:len(b.stack)-1]
	if last.backupPath != "" {
		_ = os.Remove(last.backupPath)
	}
	return nil
}

// Undo restores the most recent backup, removing it from the stack.
// Returns the path that was restored (or deleted, in the create-then-undo
// case). Returns ("", nil) when the stack is empty.
func (b *BackupManager) Undo() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.stack) == 0 {
		return "", nil
	}
	last := b.stack[len(b.stack)-1]

	if !last.preExisted {
		// Restoring "no file existed" means deleting the now-existing file.
		if err := os.Remove(last.original); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("undo: remove created file: %w", err)
		}
		b.stack = b.stack[:len(b.stack)-1]
		return last.original, nil
	}

	src, err := os.Open(last.backupPath)
	if err != nil {
		return "", fmt.Errorf("undo: open backup: %w", err)
	}
	defer src.Close()
	dir := filepath.Dir(last.original)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("undo: create parent dir: %w", err)
	}
	dst, err := os.CreateTemp(dir, ".undo.*.tmp")
	if err != nil {
		return "", fmt.Errorf("undo: create temp: %w", err)
	}
	tmpPath := dst.Name()
	if last.mode != 0 {
		if err := dst.Chmod(last.mode); err != nil {
			_ = dst.Close()
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("undo: chmod temp: %w", err)
		}
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("undo: copy back: %w", err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := os.Rename(tmpPath, last.original); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("undo: restore original: %w", err)
	}
	// Remove the backup file we just consumed.
	b.stack = b.stack[:len(b.stack)-1]
	_ = os.Remove(last.backupPath)
	return last.original, nil
}

// Cleanup removes the entire backup directory for this session.
func (b *BackupManager) Cleanup() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stack = nil
	if err := os.RemoveAll(b.root); err != nil {
		return fmt.Errorf("backup cleanup: %w", err)
	}
	return nil
}

// Depth returns the current number of undo-able operations.
func (b *BackupManager) Depth() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.stack)
}
