package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchFile_AppliesAndReturnsDiff(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("hello world\nhow are you\n"), 0o644))

	tool := NewPatchFileTool(root, NoopBackupManager())
	body, _ := json.Marshal(map[string]any{
		"path": "a.txt",
		"patches": []map[string]string{
			{"search": "hello world", "replace": "hi there"},
		},
	})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Contains(t, res.Content, "+hi there")
	assert.Contains(t, res.Content, "-hello world")

	got, _ := os.ReadFile(target)
	assert.Equal(t, "hi there\nhow are you\n", string(got))
}

func TestPatchFile_FailsWhenSearchMissing(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("foo\n"), 0o644))

	tool := NewPatchFileTool(root, NoopBackupManager())
	body, _ := json.Marshal(map[string]any{
		"path":    "a.txt",
		"patches": []map[string]string{{"search": "bar", "replace": "baz"}},
	})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content, "not found")
}

func TestPatchFile_FailsWhenSearchAmbiguous(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("dup\ndup\n"), 0o644))

	tool := NewPatchFileTool(root, NoopBackupManager())
	body, _ := json.Marshal(map[string]any{
		"path":    "a.txt",
		"patches": []map[string]string{{"search": "dup", "replace": "uniq"}},
	})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content, "matches 2 times")
}

func TestPatchFile_AppliesMultipleSequentially(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("alpha\nbeta\n"), 0o644))

	tool := NewPatchFileTool(root, NoopBackupManager())
	body, _ := json.Marshal(map[string]any{
		"path": "a.txt",
		"patches": []map[string]string{
			{"search": "alpha", "replace": "ALPHA"},
			{"search": "beta", "replace": "BETA"},
		},
	})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.False(t, res.IsError)
	got, _ := os.ReadFile(target)
	assert.Equal(t, "ALPHA\nBETA\n", string(got))
}

func TestPatchFile_RequiresApproval(t *testing.T) {
	tool := NewPatchFileTool(t.TempDir(), nil)
	assert.True(t, tool.RequiresApproval())
}
