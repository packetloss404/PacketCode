package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitAvailable reports whether `git` is on PATH; git-backed tests skip
// themselves on systems without it (mirrors internal/git tests).
func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestListProjectFiles_GitRepo(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "-q", "-b", "main").Run(); err != nil {
		t.Skip("git init failed:", err)
	}

	writeFile(t, filepath.Join(dir, "tracked.go"), "package x\n")
	writeFile(t, filepath.Join(dir, "sub", "nested.txt"), "hi\n")
	writeFile(t, filepath.Join(dir, "untracked.md"), "new\n")
	writeFile(t, filepath.Join(dir, "ignored.log"), "noise\n")
	writeFile(t, filepath.Join(dir, ".gitignore"), "ignored.log\n")

	// Track a couple of files; leave untracked.md and ignored.log unstaged.
	if err := exec.Command("git", "-C", dir, "add", "tracked.go", "sub/nested.txt").Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}

	got := listProjectFiles(dir)

	if !contains(got, "tracked.go") {
		t.Errorf("expected tracked.go in %v", got)
	}
	if !contains(got, "sub/nested.txt") {
		t.Errorf("expected sub/nested.txt in %v", got)
	}
	if !contains(got, "untracked.md") {
		t.Errorf("expected untracked untracked.md in %v", got)
	}
	if contains(got, "ignored.log") {
		t.Errorf("ignored.log should be excluded by .gitignore, got %v", got)
	}
	for _, p := range got {
		if p == ".git" || strings.HasPrefix(p, ".git/") {
			t.Errorf(".git internals should never appear, got %q", p)
		}
	}
}

func TestListProjectFiles_WalkFallback(t *testing.T) {
	dir := t.TempDir() // NOT a git repo → WalkDir fallback

	writeFile(t, filepath.Join(dir, "main.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "docs", "readme.md"), "docs\n")
	writeFile(t, filepath.Join(dir, "node_modules", "left-pad", "index.js"), "module.exports=1\n")
	writeFile(t, filepath.Join(dir, ".git", "config"), "[core]\n")
	writeFile(t, filepath.Join(dir, ".hidden", "secret.txt"), "x\n")

	got := listProjectFiles(dir)

	if !contains(got, "main.go") {
		t.Errorf("expected main.go in %v", got)
	}
	if !contains(got, "docs/readme.md") {
		t.Errorf("expected docs/readme.md in %v", got)
	}
	for _, p := range got {
		if strings.HasPrefix(p, "node_modules/") {
			t.Errorf("node_modules should be skipped, got %q", p)
		}
		if strings.HasPrefix(p, ".git/") {
			t.Errorf(".git should be skipped, got %q", p)
		}
		if strings.HasPrefix(p, ".hidden/") {
			t.Errorf("dotdirs should be skipped, got %q", p)
		}
	}
}

func TestBuildMentionEntries(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub", "file.go"), "package sub\n")

	entries := buildMentionEntries(dir)
	var found bool
	for _, e := range entries {
		if e.Verb == "sub/file.go" {
			found = true
			if e.Usage != "sub/file.go" {
				t.Errorf("Usage = %q, want sub/file.go", e.Usage)
			}
			if e.Desc != "sub" {
				t.Errorf("Desc = %q, want sub", e.Desc)
			}
		}
	}
	if !found {
		t.Fatalf("expected an entry for sub/file.go, got %+v", entries)
	}
}
