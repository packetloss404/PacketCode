package computers

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/packetcode/packetcode/internal/procrun"
)

// LocalBackend preserves PacketCode's existing project-root confinement while
// satisfying the same contract as an SSH computer.
type LocalBackend struct {
	root     string
	realRoot string
}

func NewLocalBackend(root string) (*LocalBackend, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("local backend: resolve root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("local backend: resolve root symlinks: %w", err)
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		if err != nil {
			return nil, fmt.Errorf("local backend: inspect root: %w", err)
		}
		return nil, fmt.Errorf("local backend: root is not a directory: %s", root)
	}
	// Preserve the caller's lexical spelling (notably Windows short paths)
	// for backups and user-facing results. Individual resolutions still use
	// EvalSymlinks before trusting a path.
	return &LocalBackend{root: filepath.Clean(abs), realRoot: filepath.Clean(resolved)}, nil
}

func (b *LocalBackend) ComputerID() string { return "local" }
func (b *LocalBackend) Kind() Kind         { return KindLocal }
func (b *LocalBackend) Root() string       { return b.root }
func (b *LocalBackend) Close() error       { return nil }

func (b *LocalBackend) Resolve(_ context.Context, name string, forWrite bool) (string, error) {
	if forWrite {
		return b.resolveWrite(name)
	}
	return b.resolveExisting(name)
}

func (b *LocalBackend) resolveExisting(name string) (string, error) {
	candidate, err := localCandidate(b.root, name)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	if !localPathWithin(b.realRoot, resolved) {
		return "", fmt.Errorf("path %q resolves outside project root", name)
	}
	return resolved, nil
}

func (b *LocalBackend) resolveWrite(name string) (string, error) {
	candidate, err := localCandidate(b.root, name)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(candidate)
	if info, statErr := os.Stat(parent); statErr == nil && !info.IsDir() {
		return "", fmt.Errorf("parent is not a directory: %s", parent)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}
	if resolved, statErr := filepath.EvalSymlinks(candidate); statErr == nil {
		if !localPathWithin(b.realRoot, resolved) {
			return "", fmt.Errorf("path %q resolves outside project root", name)
		}
		return candidate, nil
	} else if !os.IsNotExist(statErr) {
		return "", statErr
	}

	ancestor := parent
	for {
		resolved, statErr := filepath.EvalSymlinks(ancestor)
		if statErr == nil {
			if info, infoErr := os.Stat(ancestor); infoErr == nil && !info.IsDir() {
				return "", fmt.Errorf("parent is not a directory: %s", ancestor)
			}
			if !localPathWithin(b.realRoot, resolved) {
				return "", fmt.Errorf("path %q has an ancestor outside project root", name)
			}
			return candidate, nil
		}
		if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("path %q has no existing ancestor", name)
		}
		ancestor = parent
	}
}

func localCandidate(root, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		name = "."
	}
	candidate := name
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate = filepath.Clean(candidate)
	if !localPathWithin(root, candidate) {
		return "", fmt.Errorf("path outside project root: %s", name)
	}
	return candidate, nil
}

func localPathWithin(root, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func (b *LocalBackend) ReadFile(ctx context.Context, name string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := b.resolveExisting(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}

func (b *LocalBackend) WriteFile(ctx context.Context, name string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := b.resolveWrite(name)
	if err != nil {
		return err
	}
	dir := filepath.Dir(resolved)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	// Re-resolve after mkdir so a concurrently introduced symlink cannot turn
	// directory creation into a write outside the root.
	resolved, err = b.resolveWrite(name)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(resolved); statErr == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(resolved), ".write.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, resolved); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func (b *LocalBackend) ReadDir(ctx context.Context, name string) ([]FileEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved, err := b.resolveExisting(name)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", name)
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, err
	}
	out := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, infoErr
		}
		out = append(out, FileEntry{Name: entry.Name(), Size: info.Size(), Mode: info.Mode(), IsDir: entry.IsDir()})
	}
	return out, nil
}

func (b *LocalBackend) Execute(ctx context.Context, command, cwd string, output io.Writer) (ExecResult, error) {
	resolved := b.root
	var err error
	if strings.TrimSpace(cwd) != "" && cwd != "." {
		resolved, err = b.resolveExisting(cwd)
		if err != nil {
			return ExecResult{}, err
		}
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = resolved
	cmd.Stdout = output
	cmd.Stderr = output
	teardown := procrun.ConfigureTreeCancelRecorder(cmd)
	runErr := cmd.Run()
	if runErr == nil {
		return ExecResult{ExitCode: 0}, nil
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		return ExecResult{ExitCode: exitErr.ExitCode()}, nil
	}
	if ctx.Err() != nil {
		// Cancelled. Report what the teardown managed, not merely that one
		// was asked for: the caller has to tell a user whether the command
		// actually stopped.
		res := ExecResult{ExitCode: -1}
		if outcome, ran := teardown(); ran {
			res.Teardown = &outcome
		}
		return res, nil
	}
	return ExecResult{}, runErr
}
