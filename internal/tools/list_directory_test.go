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

func setupListTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "lib"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# top"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "lib", "util.go"), []byte("package lib"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(root, "node_modules", "x"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "node_modules", "x", "junk.js"), []byte("noise"), 0o644))
	return root
}

func TestListDirectory_NonRecursiveDefault(t *testing.T) {
	tool := NewListDirectoryTool(setupListTree(t))
	res, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Contains(t, res.Content, "src/")
	assert.Contains(t, res.Content, "README.md")
	assert.NotContains(t, res.Content, "main.go", "non-recursive should not list nested files")
	assert.NotContains(t, res.Content, "node_modules")
}

func TestListDirectory_Recursive(t *testing.T) {
	tool := NewListDirectoryTool(setupListTree(t))
	body, _ := json.Marshal(map[string]any{"recursive": true, "max_depth": 5})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.Contains(t, res.Content, "main.go")
	assert.Contains(t, res.Content, "util.go")
	assert.NotContains(t, res.Content, "junk.js")
}

func TestListDirectory_OnFileReturnsError(t *testing.T) {
	root := setupListTree(t)
	tool := NewListDirectoryTool(root)
	body, _ := json.Marshal(map[string]any{"path": "README.md"})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content, "not a directory")
}

func TestListDirectory_RejectsTraversal(t *testing.T) {
	tool := NewListDirectoryTool(t.TempDir())
	body, _ := json.Marshal(map[string]any{"path": "../"})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.True(t, res.IsError)
}
