package app

import (
	"context"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/packetcode/packetcode/internal/ui/components/autocomplete"
)

// fileIndexWalkCap bounds the fallback WalkDir so a giant non-repo tree can't
// stall the UI thread or balloon memory. Git repos are already bounded by
// ls-files.
const fileIndexWalkCap = 20000

// fileIndexSkipDirs are directory names never descended into by the WalkDir
// fallback: VCS internals and the usual heavy vendored/build trees. Dotdirs
// (any name starting with ".") are skipped separately.
var fileIndexSkipDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
	"dist":         {},
	"build":        {},
	"target":       {},
	".venv":        {},
}

// listProjectFiles returns project-relative, slash-separated paths for the
// files under root that make sense to @-mention. It prefers git's own view
// of the working tree (tracked + untracked, honouring .gitignore) and falls
// back to a bounded filesystem walk when root isn't a repo or git isn't
// available.
func listProjectFiles(root string) []string {
	if files, ok := gitListFiles(root); ok {
		return files
	}
	return walkListFiles(root)
}

// ListProjectFiles is the exported form of listProjectFiles, for callers
// outside the TUI that need the same @-mention candidate set — notably the
// ACP _packetcode/project/files extension in cmd/packetcode. One
// implementation means the desktop client's @ menu and the TUI's agree on
// which files exist and which are ignored.
func ListProjectFiles(root string) []string {
	return listProjectFiles(root)
}

// gitListFiles shells out to `git ls-files` for the cached + untracked set
// with .gitignore applied. ok is false when git isn't installed, root isn't
// a repo, or the command errors/times out, signalling the caller to fall
// back to a filesystem walk.
func gitListFiles(root string) (files []string, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", root,
		"ls-files", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, filepath.ToSlash(line))
	}
	return files, true
}

// walkListFiles is the non-git fallback: a bounded WalkDir that skips VCS
// internals, common vendored/build trees, dotdirs, and obvious binaries.
func walkListFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than aborting the walk
		}
		if len(files) >= fileIndexWalkCap {
			return filepath.SkipAll
		}
		name := d.Name()
		if d.IsDir() {
			if p == root {
				return nil
			}
			if _, skip := fileIndexSkipDirs[name]; skip {
				return filepath.SkipDir
			}
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if isBinaryName(name) {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files
}

// isBinaryName reports whether a filename's extension marks it as an obvious
// binary we shouldn't offer as an @-mention. Cheap extension check only — the
// submit-time expander does the authoritative NUL-byte sniff.
func isBinaryName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".webp", ".tiff",
		".pdf", ".zip", ".gz", ".tar", ".tgz", ".bz2", ".xz", ".7z", ".rar",
		".exe", ".dll", ".so", ".dylib", ".a", ".o", ".class", ".jar",
		".bin", ".dat", ".wasm", ".mp3", ".mp4", ".wav", ".mov", ".avi",
		".mkv", ".flac", ".ogg", ".woff", ".woff2", ".ttf", ".otf", ".eot":
		return true
	}
	return false
}

// buildMentionEntries turns the project's file list into autocomplete rows.
// Verb and Usage are the relative path (Verb is the completion payload spliced
// into the buffer; Usage is the label shown); Desc is the containing directory
// so identically-named files in different folders stay distinguishable.
func buildMentionEntries(root string) []autocomplete.Entry {
	paths := listProjectFiles(root)
	entries := make([]autocomplete.Entry, 0, len(paths))
	for _, rel := range paths {
		entries = append(entries, autocomplete.Entry{
			Verb:  rel,
			Usage: rel,
			Desc:  filepath.Dir(rel),
		})
	}
	return entries
}
