package jobs

import (
	"fmt"
	"path/filepath"
	"strings"
)

// worktreeBinding is a worktree root this Manager created, together with the
// computer it lives on. The computer travels with the path because a remote
// worktree only exists on that host: handing its POSIX path to a local job
// would point every tool at a directory that is not there.
type worktreeBinding struct {
	ComputerID string
	Root       string
}

// recordWorktreeRoot remembers the root a job was actually given after its
// worktree was created. Job carries WorktreePath (the checkout's top level)
// but not the root, and for a project opened below the repository root the
// two differ — see createWorktree. A verifier needs the root, so that the
// relative paths in a step prompt mean the same thing to both agents.
func (m *Manager) recordWorktreeRoot(j *Job, root string) {
	if j == nil || strings.TrimSpace(root) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.worktreeRoots[j.ID] = worktreeBinding{ComputerID: j.ComputerID, Root: root}
}

// resolveVerifyRoot turns SpawnRequest.VerifyWorktreeOf into the root the new
// job will run in. It is the whole trust boundary for the field, so it is
// deliberately narrow:
//
//   - A write job is refused. The point of the root is that a verifier can
//     read the work it judges; a writable one could edit that work instead.
//   - The path comes from this Manager's own record of worktrees it created.
//     Nothing a caller supplies is used as a path, so the field cannot name
//     an arbitrary directory.
//   - The workspace must match the recorded one. A worktree on a Packet
//     Computer is not reachable from a local job, and vice versa.
//   - A local root is re-validated against the worktrees directory, so a
//     record whose directory was replaced by a symlink since it was written
//     does not become a way out of ~/.packetcode/worktrees.
//
// An id with no record returns ok=false and no error: a read-only work step
// never gets a worktree, and its verifier correctly keeps the ordinary root.
func (m *Manager) resolveVerifyRoot(req SpawnRequest, ws Workspace) (worktreeBinding, bool, *SpawnError) {
	target := strings.TrimSpace(req.VerifyWorktreeOf)
	if target == "" {
		return worktreeBinding{}, false, nil
	}
	if req.AllowWrite {
		return worktreeBinding{}, false, &SpawnError{
			Code:   "verify_root_denied",
			Reason: "a job that may write cannot be rooted in the worktree it verifies",
		}
	}

	m.mu.RLock()
	binding, ok := m.worktreeRoots[target]
	m.mu.RUnlock()
	if !ok {
		return worktreeBinding{}, false, nil
	}
	if binding.ComputerID != ws.ComputerID {
		return worktreeBinding{}, false, &SpawnError{
			Code: "verify_root_denied",
			Reason: fmt.Sprintf(
				"job %s worktree lives on %s; this job runs on %s",
				target, computerLabel(binding.ComputerID), computerLabel(ws.ComputerID),
			),
		}
	}
	if binding.ComputerID == "" {
		if err := m.validateLocalVerifyRoot(binding.Root); err != nil {
			return worktreeBinding{}, false, &SpawnError{Code: "verify_root_denied", Reason: err.Error()}
		}
	}
	return binding, true, nil
}

// verifyRootFor reports the read-only root resolved for j at spawn time.
func (m *Manager) verifyRootFor(jobID string) (worktreeBinding, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	binding, ok := m.verifyRoots[jobID]
	return binding, ok
}

// validateLocalVerifyRoot re-checks a recorded local root before it is used.
// Spawn validates too, but a queued job can sit for minutes: the check that
// matters is the one taken immediately before the tools are pointed at the
// directory.
func (m *Manager) validateLocalVerifyRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("verifier root is empty")
	}
	if err := validateExistingDir(root, "verifier root"); err != nil {
		return err
	}
	worktreesDir, err := filepath.EvalSymlinks(m.worktreesDir())
	if err != nil {
		return fmt.Errorf("verifier worktrees dir: %w", err)
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("verifier root: %w", err)
	}
	if err := ensureChildPath(worktreesDir, real); err != nil {
		return fmt.Errorf("verifier root is not a packetcode worktree: %s", root)
	}
	return nil
}

func computerLabel(computerID string) string {
	if computerID == "" {
		return "this machine"
	}
	return "computer " + computerID
}
