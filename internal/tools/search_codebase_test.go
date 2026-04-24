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

func setupSearchTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() { greet() }\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "util.go"), []byte("package main\n\nfunc greet() {}\n"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(root, "node_modules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "node_modules", "noise.go"), []byte("greet here too"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "more.txt"), []byte("greet in text\n"), 0o644))
	return root
}

func TestSearchCodebase_FindsMatches(t *testing.T) {
	tool := NewSearchCodebaseTool(setupSearchTree(t))
	body, _ := json.Marshal(map[string]any{"pattern": "greet"})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Contains(t, res.Content, "greet")
	assert.NotContains(t, res.Content, "node_modules", "skipped dirs must not appear in results")
}

func TestSearchCodebase_NoMatches(t *testing.T) {
	tool := NewSearchCodebaseTool(setupSearchTree(t))
	body, _ := json.Marshal(map[string]any{"pattern": "definitely-not-here-xyz"})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.Contains(t, res.Content, "No matches")
}

func TestSearchCodebase_GlobFilter(t *testing.T) {
	tool := NewSearchCodebaseTool(setupSearchTree(t))
	body, _ := json.Marshal(map[string]any{"pattern": "greet", "file_glob": "*.go"})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.NotContains(t, res.Content, ".txt", "glob should restrict to .go files")
}

func TestSearchCodebase_GoFallbackGlobMatchesRelativePath(t *testing.T) {
	tool := NewSearchCodebaseTool(setupSearchTree(t))
	tool.rgOnce.Do(func() { tool.rgPath = "" })
	body, _ := json.Marshal(map[string]any{"pattern": "greet", "file_glob": "**/*.go"})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.Contains(t, res.Content, "main.go")
	assert.NotContains(t, res.Content, ".txt", "glob should restrict to .go files")
}

func TestSearchCodebase_EmptyPattern(t *testing.T) {
	tool := NewSearchCodebaseTool(t.TempDir())
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"pattern":""}`))
	require.NoError(t, err)
	assert.True(t, res.IsError)
}
