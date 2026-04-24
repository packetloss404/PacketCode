package tools

// BackupManager is the contract write_file and patch_file use to snapshot
// a file before modifying it. The session package provides the concrete
// implementation; defining the interface here keeps the tools package
// import-free of session and avoids a dependency cycle.
type BackupManager interface {
	// Backup snapshots the file at filePath. Returns nil silently if the
	// file does not exist (a fresh write doesn't need a backup).
	Backup(filePath string) error
}

type rollbackBackupManager interface {
	RollbackBackup(filePath string) error
}

func rollbackBackup(backups BackupManager, filePath string) {
	if rb, ok := backups.(rollbackBackupManager); ok {
		_ = rb.RollbackBackup(filePath)
	}
}

// noopBackup is a BackupManager that does nothing — useful in tests and
// in code paths where backups are explicitly unwanted.
type noopBackup struct{}

func (noopBackup) Backup(string) error { return nil }

// NoopBackupManager returns a BackupManager that performs no work. Tests
// for write_file and patch_file use this; production wires in the real
// session.BackupManager.
func NoopBackupManager() BackupManager { return noopBackup{} }
