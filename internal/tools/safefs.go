package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveInRoot resolves a tool-supplied path against root, returning the
// cleaned absolute path. It rejects lexical traversal and symlink/junction
// traversal through the existing path prefix.
func resolveInRoot(root, suppliedPath string) (string, error) {
	abs, err := resolveLexicalInRoot(root, suppliedPath)
	if err != nil {
		return "", err
	}
	if err := validateExistingPrefixInRoot(root, abs); err != nil {
		return "", err
	}
	return abs, nil
}

// resolveExistingInRoot resolves a path that must already exist and rejects
// symlink/junction targets outside root.
func resolveExistingInRoot(root, suppliedPath string) (string, error) {
	abs, err := resolveLexicalInRoot(root, suppliedPath)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("resolve root symlinks: %w", err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	if !pathWithinRoot(rootReal, real) {
		return "", fmt.Errorf("path outside project root: %s", suppliedPath)
	}
	return real, nil
}

// resolveWritePath resolves a path intended for creation or replacement. It
// validates the final target if it exists and validates the parent chain so
// writes cannot create files through a symlink/junction outside root.
func resolveWritePath(root, suppliedPath string) (string, error) {
	abs, err := resolveLexicalInRoot(root, suppliedPath)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(abs); err == nil {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("resolve root: %w", err)
		}
		rootReal, err := filepath.EvalSymlinks(rootAbs)
		if err != nil {
			return "", fmt.Errorf("resolve root symlinks: %w", err)
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", err
		}
		if !pathWithinRoot(rootReal, real) {
			return "", fmt.Errorf("path outside project root: %s", suppliedPath)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(abs)
	if err := validateExistingPrefixInRoot(root, parent); err != nil {
		return "", err
	}
	if info, err := os.Stat(parent); err == nil && !info.IsDir() {
		return "", fmt.Errorf("parent is not a directory: %s", parent)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return abs, nil
}

func resolveLexicalInRoot(root, suppliedPath string) (string, error) {
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

	if !pathWithinRoot(rootAbs, candidate) {
		return "", fmt.Errorf("path outside project root: %s", suppliedPath)
	}
	return candidate, nil
}

func validateExistingPrefixInRoot(root, path string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve root symlinks: %w", err)
	}
	cur := filepath.Clean(path)
	for {
		if _, err := os.Lstat(cur); err == nil {
			real, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return err
			}
			if !pathWithinRoot(rootReal, real) {
				return fmt.Errorf("path outside project root: %s", path)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			return fmt.Errorf("path outside project root: %s", path)
		}
		cur = parent
	}
}

func pathWithinRoot(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
