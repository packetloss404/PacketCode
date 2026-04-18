package jobs

import (
	"path/filepath"
)

// resolveAbsoluteForLock turns a (possibly relative) path into a
// canonical absolute path keyed by the project root. The result is used
// only as a map key for path-locking — it does NOT validate that the
// path stays inside root (the underlying tool already does that).
func resolveAbsoluteForLock(root, p string) (string, error) {
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	if root == "" {
		// Best-effort: fall back to working-dir-relative.
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", err
		}
		return filepath.Clean(abs), nil
	}
	return filepath.Clean(filepath.Join(root, p)), nil
}
