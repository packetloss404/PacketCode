// Package atomicfile writes a file so that a crash leaves either the previous
// contents or the new ones, never a mixture and never an empty file.
//
// Both internal/session and internal/jobs already wrote through a temp file and
// a rename, and both called that "atomic". Rename is atomic for *visibility* —
// a reader sees one name or the other, never a half-written one — but without
// an fsync the rename can reach the disk before the data it points at. A crash
// in that window leaves a file that exists, has the right name, and contains
// nothing. For a session that is the conversation; for a job record it is the
// state a restart reconciles from.
//
// So the claim was half true, in the half that is easy to test and not the half
// that matters when the machine loses power.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Write writes data to path via a temp file in the same directory, fsynced
// before the rename.
//
// tmpPattern is passed to os.CreateTemp, so it should carry a leading dot and
// a `.tmp` suffix — callers that scan the directory need to recognise and skip
// a leftover.
func Write(path string, data []byte, perm os.FileMode, tmpPattern string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, tmpPattern)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp: %w", err)
	}
	// Before the close, not after: this is the step that makes the rename
	// point at bytes that are actually on the disk.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	syncDir(dir)
	return nil
}

// syncDir flushes the directory entry so the rename itself survives a crash,
// and not merely the bytes it points at.
//
// Best-effort and deliberately unchecked. On POSIX this is the documented way
// to make a rename durable. On Windows a directory cannot be opened for the
// same purpose and the call fails; that is not an error worth failing a write
// over, because the data fsync above has already done the part that prevents
// an empty file.
func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
