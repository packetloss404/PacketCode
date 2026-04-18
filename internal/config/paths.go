package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// dirName is the name of packetcode's config/state directory under $HOME.
const dirName = ".packetcode"

// HomeDir returns ~/.packetcode, creating it (with 0700) if it does not exist.
func HomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate user home: %w", err)
	}
	dir := filepath.Join(home, dirName)
	if err := EnsureDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// ConfigPath returns ~/.packetcode/config.toml.
// The directory is created if missing; the file is not.
func ConfigPath() (string, error) {
	dir, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// SessionsDir returns ~/.packetcode/sessions/, creating it if missing.
func SessionsDir() (string, error) {
	dir, err := HomeDir()
	if err != nil {
		return "", err
	}
	sessions := filepath.Join(dir, "sessions")
	if err := EnsureDir(sessions); err != nil {
		return "", err
	}
	return sessions, nil
}

// BackupsDir returns ~/.packetcode/backups/, creating it if missing.
func BackupsDir() (string, error) {
	dir, err := HomeDir()
	if err != nil {
		return "", err
	}
	backups := filepath.Join(dir, "backups")
	if err := EnsureDir(backups); err != nil {
		return "", err
	}
	return backups, nil
}

// JobsDir returns ~/.packetcode/jobs/, creating it if missing.
// Background-job metadata snapshots live here; see internal/jobs.
func JobsDir() (string, error) {
	dir, err := HomeDir()
	if err != nil {
		return "", err
	}
	jobs := filepath.Join(dir, "jobs")
	if err := EnsureDir(jobs); err != nil {
		return "", err
	}
	return jobs, nil
}

// CostTallyPath returns ~/.packetcode/cost-tally.json.
// The directory is created if missing; the file is not.
func CostTallyPath() (string, error) {
	dir, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cost-tally.json"), nil
}

// ThemePath returns ~/.packetcode/theme.toml.
// The directory is created if missing; the file is not. When the file
// is absent, the theme loader falls back to the built-in Terminal Noir
// palette.
func ThemePath() (string, error) {
	dir, err := HomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "theme.toml"), nil
}

// EnsureDir creates the directory (with parents) at 0700 if it does not exist.
// On Windows the perm bits are best-effort; the OS does not enforce POSIX modes.
func EnsureDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create dir %s: %w", path, err)
	}
	return nil
}
