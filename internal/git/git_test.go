package git

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// gitAvailable returns true if `git` is on PATH. Tests that depend on
// real git operations skip themselves on systems without it.
func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func TestIsRepo_NotARepo(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	assert.False(t, IsRepo(t.TempDir()))
}

func TestBranch_NotARepo(t *testing.T) {
	assert.Equal(t, "", Branch(t.TempDir()))
}

func TestRepoRoot_NotARepoReturnsDir(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, dir, RepoRoot(dir))
}

func TestIsRepo_FreshInit(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	cmd := exec.Command("git", "-C", dir, "init", "-q", "-b", "main")
	if err := cmd.Run(); err != nil {
		t.Skip("git init failed:", err)
	}
	assert.True(t, IsRepo(dir))
	// On a freshly-initialised repo with no commit, branch --show-current
	// can return the configured initial branch ("main") or empty string
	// depending on git version. Accept either.
	branch := Branch(dir)
	assert.Contains(t, []string{"", "main"}, branch)
}
