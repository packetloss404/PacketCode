package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// resolveInRoot resolves a tool-supplied path against root, returning the
// cleaned absolute path. It rejects:
//   - absolute paths that escape root,
//   - relative paths whose resolved form lies outside root (`..` traversal).
//
// Symlinks are *not* resolved here — that's a deliberate choice because
// resolving them would force every tool to handle the case where the file
// doesn't exist yet (Lstat-vs-Stat semantics differ across OS). The risk
// of a malicious symlink inside a user's own project is tolerable; the
// risk of a malicious LLM injecting `../../../etc/passwd` is not.
func resolveInRoot(root, suppliedPath string) (string, error) {
	if suppliedPath == "" {
		return "", fmt.Errorf("path is empty")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}

	candidate := suppliedPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, candidate)
	}
	candidate = filepath.Clean(candidate)

	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil {
		return "", fmt.Errorf("path outside project root: %s", suppliedPath)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path outside project root: %s", suppliedPath)
	}
	return candidate, nil
}
