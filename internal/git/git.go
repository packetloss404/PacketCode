// Package git wraps the handful of git read-only operations the top bar
// needs (branch, repo root, in-repo check). All operations shell out to
// the system `git`. If git isn't installed, every function returns the
// "not in a repo" zero value rather than an error — the UI degrades
// gracefully on machines without git.
package git

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// IsRepo reports whether dir is inside a git working tree.
func IsRepo(dir string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	return cmd.Run() == nil
}

// Branch returns the current branch name (e.g. "main", "feature/auth"),
// or empty string if dir isn't a repo, git isn't installed, or HEAD is
// detached.
func Branch(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RepoRoot returns the absolute path of the repo root for dir, or dir
// itself if it isn't in a repo.
func RepoRoot(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return dir
	}
	return strings.TrimSpace(string(out))
}
