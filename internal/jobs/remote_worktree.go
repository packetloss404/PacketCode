package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/packetcode/packetcode/internal/computers"
)

const remoteGitTimeout = 30 * time.Second

// openRemoteBackend obtains a fresh backend owned by this worker and verifies
// that an opener did not accidentally return a backend for another computer.
func (m *Manager) openRemoteBackend(ctx context.Context, ws Workspace) (computers.RuntimeBackend, error) {
	m.mu.RLock()
	opener := m.cfg.OpenBackend
	m.mu.RUnlock()
	if opener == nil {
		return nil, fmt.Errorf("no backend opener is configured for %s", workspaceLabel(ws))
	}
	backend, err := opener(ctx, ws)
	if err != nil {
		return nil, err
	}
	if backend == nil {
		return nil, fmt.Errorf("backend opener returned nil for %s", workspaceLabel(ws))
	}
	if backend.ComputerID() != ws.ComputerID {
		_ = backend.Close()
		return nil, fmt.Errorf("backend computer mismatch: got %s, want %s", backend.ComputerID(), ws.ComputerID)
	}
	if backend.Kind() != computers.KindSSH {
		_ = backend.Close()
		return nil, fmt.Errorf("backend for %s is %s, want ssh", ws.ComputerName, backend.Kind())
	}
	return backend, nil
}

// prepareRemoteBackend opens the registered root for a read-only job. A
// write job first creates a dedicated git worktree from committed HEAD, closes
// the bootstrap connection, and opens a new backend rooted at the worktree.
// There is deliberately no fallback to the primary remote checkout.
func (m *Manager) prepareRemoteBackend(ctx context.Context, j *Job) (computers.RuntimeBackend, error) {
	ws := workspaceOfJob(j, m.cfg.Root)
	backend, err := m.openRemoteBackend(ctx, ws)
	if err != nil {
		return nil, err
	}
	if !j.AllowWrite {
		return backend, nil
	}

	// Job.ID is intentionally short for the UI. Do not reuse its 32-bit-ish
	// collision space for durable names on a shared server: bind the remote
	// branch and path to the full subsession identity instead.
	created, err := createRemoteWorktree(ctx, backend, remoteWorktreeToken(j))
	if err != nil {
		_ = backend.Close()
		m.setWorktree(j, worktreeInfo{Root: ws.WorkingDir, Note: err.Error()})
		return nil, err
	}
	m.setWorktree(j, created)
	_ = backend.Close()

	worktreeWS := ws
	worktreeWS.WorkingDir = created.Root
	worktreeBackend, err := m.openRemoteBackend(ctx, worktreeWS)
	if err != nil {
		return nil, fmt.Errorf("open remote worktree %s: %w", created.Root, err)
	}
	return worktreeBackend, nil
}

func remoteWorktreeToken(j *Job) string {
	if j == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(j.SessionID + "\x00" + j.ID))
	return hex.EncodeToString(sum[:])[:24]
}

func createRemoteWorktree(ctx context.Context, backend computers.RuntimeBackend, id string) (worktreeInfo, error) {
	info := worktreeInfo{Root: backend.Root()}
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, "\x00\r\n/") {
		return info, fmt.Errorf("invalid remote worktree job id %q", id)
	}

	repoRoot, err := remoteCommandOutput(ctx, backend, "git rev-parse --show-toplevel")
	if err != nil {
		return info, fmt.Errorf("remote worktree repo check: %w", err)
	}
	repoRoot = strings.TrimSpace(repoRoot)
	if !path.IsAbs(repoRoot) || strings.ContainsAny(repoRoot, "\x00\r\n") {
		return info, fmt.Errorf("remote git returned invalid repository root %q", repoRoot)
	}
	workspaceRel, err := remoteRepoRelative(repoRoot, backend.Root())
	if err != nil {
		return info, err
	}
	base, err := remoteCommandOutput(ctx, backend, "git rev-parse --verify 'HEAD^{commit}'")
	if err != nil {
		return info, fmt.Errorf("remote worktree base ref: %w", err)
	}
	base = strings.TrimSpace(base)
	if base == "" || strings.ContainsAny(base, "\x00\r\n \t") {
		return info, fmt.Errorf("remote git returned invalid HEAD %q", base)
	}
	home, err := remoteCommandOutput(ctx, backend, `printf '%s\n' "$HOME"`)
	if err != nil {
		return info, fmt.Errorf("remote home lookup: %w", err)
	}
	home = strings.TrimSpace(home)
	if !path.IsAbs(home) || strings.ContainsAny(home, "\x00\r\n") {
		return info, fmt.Errorf("remote shell returned invalid HOME %q", home)
	}

	branch := "packetcode-job-" + id
	if _, err := remoteCommandOutput(ctx, backend, "git check-ref-format --branch "+quotePOSIX(branch)); err != nil {
		return info, fmt.Errorf("remote worktree branch: %w", err)
	}
	repoKey := remoteRepoWorktreeKey(repoRoot)
	parent := path.Join(home, ".packetcode", "worktrees", repoKey)
	worktreePath := path.Join(parent, id)
	packetDir := path.Join(home, ".packetcode")
	worktreesDir := path.Join(packetDir, "worktrees")
	if _, err := remoteCommandOutput(ctx, backend,
		"umask 077 && test ! -L "+quotePOSIX(packetDir)+" && test ! -L "+quotePOSIX(worktreesDir)+" && test ! -L "+quotePOSIX(parent)+
			" && mkdir -p -- "+quotePOSIX(parent)+
			" && test ! -L "+quotePOSIX(packetDir)+" && test ! -L "+quotePOSIX(worktreesDir)+" && test ! -L "+quotePOSIX(parent)+
			" && chmod 700 -- "+quotePOSIX(packetDir)+" "+quotePOSIX(worktreesDir)+" "+quotePOSIX(parent)+
			" && test ! -e "+quotePOSIX(worktreePath)+" && test ! -L "+quotePOSIX(worktreePath)+" && mkdir -m 700 -- "+quotePOSIX(worktreePath),
	); err != nil {
		return info, fmt.Errorf("remote worktree path: %w", err)
	}

	command := "git -C " + quotePOSIX(repoRoot) + " worktree add -b " + quotePOSIX(branch) + " " + quotePOSIX(worktreePath) + " " + quotePOSIX(base)
	if _, err := remoteCommandOutput(ctx, backend, command); err != nil {
		// Remove only the exact empty directory created above. rmdir refuses
		// non-empty paths, so a partial worktree is preserved for inspection.
		_, _ = remoteCommandOutput(context.Background(), backend, "rmdir -- "+quotePOSIX(worktreePath))
		return info, fmt.Errorf("remote git worktree add: %w", err)
	}
	jobRoot := worktreePath
	if workspaceRel != "." {
		jobRoot = path.Join(worktreePath, workspaceRel)
	}
	return worktreeInfo{Root: jobRoot, Path: worktreePath, Branch: branch, Base: base}, nil
}

func remoteRepoRelative(repoRoot, workspaceRoot string) (string, error) {
	repoRoot = path.Clean(repoRoot)
	workspaceRoot = path.Clean(workspaceRoot)
	if workspaceRoot == repoRoot {
		return ".", nil
	}
	prefix := strings.TrimSuffix(repoRoot, "/") + "/"
	if !strings.HasPrefix(workspaceRoot, prefix) {
		return "", fmt.Errorf("remote workspace root %s is outside git repository %s", workspaceRoot, repoRoot)
	}
	rel := strings.TrimPrefix(workspaceRoot, prefix)
	if rel == "" || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("invalid remote workspace path relative to repository: %s", rel)
	}
	return rel, nil
}

func remoteCommandOutput(parent context.Context, backend computers.RuntimeBackend, command string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, remoteGitTimeout)
	defer cancel()
	var out bytes.Buffer
	result, err := backend.Execute(ctx, command, ".", &out)
	if err != nil {
		return strings.TrimSpace(out.String()), err
	}
	if ctx.Err() != nil {
		return strings.TrimSpace(out.String()), ctx.Err()
	}
	if result.ExitCode != 0 {
		detail := strings.TrimSpace(out.String())
		if detail == "" {
			detail = fmt.Sprintf("exit %d", result.ExitCode)
		}
		return detail, fmt.Errorf("%s", detail)
	}
	return strings.TrimSpace(out.String()), nil
}

func quotePOSIX(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func remoteRepoWorktreeKey(repoRoot string) string {
	sum := sha256.Sum256([]byte(path.Clean(repoRoot)))
	return hex.EncodeToString(sum[:])[:12]
}

// closeBackendOnCancel ensures a remote transport does not outlive its job
// context. Close implementations must be idempotent; the worker also defers a
// normal Close for the non-cancelled path.
func closeBackendOnCancel(ctx context.Context, backend computers.RuntimeBackend) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			_ = backend.Close()
		case <-stop:
		}
	}()
	var once bool
	return func() {
		if !once {
			once = true
			close(stop)
		}
		<-done
	}
}
