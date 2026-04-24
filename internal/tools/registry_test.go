package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTool struct {
	name        string
	approval    bool
	description string
}

func (s *stubTool) Name() string            { return s.name }
func (s *stubTool) Description() string     { return s.description }
func (s *stubTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (s *stubTool) RequiresApproval() bool  { return s.approval }
func (s *stubTool) Execute(ctx context.Context, params json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "alpha"})
	r.Register(&stubTool{name: "beta"})

	got, ok := r.Get("alpha")
	require.True(t, ok)
	assert.Equal(t, "alpha", got.Name())

	_, ok = r.Get("missing")
	assert.False(t, ok)
}

func TestRegistry_AllSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "zeta"})
	r.Register(&stubTool{name: "alpha"})
	r.Register(&stubTool{name: "mu"})

	names := []string{}
	for _, t := range r.All() {
		names = append(names, t.Name())
	}
	assert.Equal(t, []string{"alpha", "mu", "zeta"}, names)
}

func TestRegistry_DefinitionsTranslate(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubTool{name: "read", description: "read a file"})
	defs := r.Definitions()
	require.Len(t, defs, 1)
	assert.Equal(t, "read", defs[0].Name)
	assert.Equal(t, "read a file", defs[0].Description)
	assert.JSONEq(t, `{"type":"object"}`, string(defs[0].Parameters))
}

func TestResolveInRoot_AcceptsValidPaths(t *testing.T) {
	root := t.TempDir()
	got, err := resolveInRoot(root, "subdir/file.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "subdir", "file.txt"), got)
}

func TestResolveInRoot_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	_, err := resolveInRoot(root, "../escape.txt")
	require.Error(t, err)
	_, err = resolveInRoot(root, "subdir/../../escape.txt")
	require.Error(t, err)
}

func TestResolveInRoot_RejectsAbsoluteOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "leaked.txt")
	_, err := resolveInRoot(root, outside)
	require.Error(t, err)
}

func TestResolveInRoot_AllowsAbsoluteInsideRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "ok.txt")
	got, err := resolveInRoot(root, inside)
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		// Comparison is case-insensitive on Windows but exact path
		// equality is fine since we built `inside` from `root`.
		assert.Equal(t, inside, got)
	} else {
		assert.Equal(t, inside, got)
	}
}

func TestResolveInRoot_EmptyPathRejected(t *testing.T) {
	_, err := resolveInRoot(t.TempDir(), "")
	require.Error(t, err)
}

func TestResolveExistingInRoot_RejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644))
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation not permitted: %v", err)
		}
		require.NoError(t, err)
	}

	_, err := resolveExistingInRoot(root, filepath.Join("link", "secret.txt"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside project root")
}

func TestResolveWritePath_RejectsSymlinkParentEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation not permitted: %v", err)
		}
		require.NoError(t, err)
	}

	_, err := resolveWritePath(root, filepath.Join("link", "created.txt"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside project root")
}
