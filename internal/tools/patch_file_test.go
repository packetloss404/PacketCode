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

func TestPatchFile_RejectsInvalidUTF8(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "binary.dat")
	require.NoError(t, os.WriteFile(target, []byte{0xff, 0xfe, 'x'}, 0o644))

	tool := NewPatchFileTool(root, NoopBackupManager())
	body, _ := json.Marshal(map[string]any{
		"path":    "binary.dat",
		"patches": []map[string]string{{"search": "x", "replace": "y"}},
	})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content, "non-UTF-8")

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, []byte{0xff, 0xfe, 'x'}, got)
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

// TestPatchFile_PatchOpJSONWireFormat pins the rename from unexported
// patchOp to exported PatchOp: the wire format must still accept the
// lowercase {"search":..., "replace":...} literals the LLM produces.
func TestPatchFile_PatchOpJSONWireFormat(t *testing.T) {
	raw := []byte(`[{"search":"x","replace":"y"},{"search":"a","replace":"b"}]`)
	var ops []PatchOp
	require.NoError(t, json.Unmarshal(raw, &ops))
	require.Len(t, ops, 2)
	assert.Equal(t, "x", ops[0].Search)
	assert.Equal(t, "y", ops[0].Replace)
	assert.Equal(t, "a", ops[1].Search)
	assert.Equal(t, "b", ops[1].Replace)

	// Marshal-roundtrip keeps the same lowercase tags.
	out, err := json.Marshal(ops)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"search":"x"`)
	assert.Contains(t, string(out), `"replace":"b"`)
}

func TestPatchFile_PreviewPatchDiff_Valid(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("hello world\n"), 0o644))
	tool := NewPatchFileTool(root, NoopBackupManager())
	unified, err := tool.PreviewPatchDiff("a.txt", []PatchOp{{Search: "hello world", Replace: "hi there"}})
	require.NoError(t, err)
	assert.Contains(t, unified, "-hello world")
	assert.Contains(t, unified, "+hi there")
	assert.Contains(t, unified, "a.txt (current)")
	assert.Contains(t, unified, "a.txt (proposed)")
}

func TestPatchFile_PreviewPatchDiff_EmptyPatches(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("x\n"), 0o644))
	tool := NewPatchFileTool(root, NoopBackupManager())
	_, err := tool.PreviewPatchDiff("a.txt", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one patch is required")
}

func TestPatchFile_PreviewPatchDiff_EmptySearch(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("x\n"), 0o644))
	tool := NewPatchFileTool(root, NoopBackupManager())
	_, err := tool.PreviewPatchDiff("a.txt", []PatchOp{{Search: "", Replace: "y"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "patch #1 has empty search string")
}

func TestPatchFile_PreviewPatchDiff_SearchNotFound(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("foo\n"), 0o644))
	tool := NewPatchFileTool(root, NoopBackupManager())
	_, err := tool.PreviewPatchDiff("a.txt", []PatchOp{{Search: "bar", Replace: "baz"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPatchFile_PreviewPatchDiff_SearchAmbiguous(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("dup\ndup\n"), 0o644))
	tool := NewPatchFileTool(root, NoopBackupManager())
	_, err := tool.PreviewPatchDiff("a.txt", []PatchOp{{Search: "dup", Replace: "uniq"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "matches 2 times")
}

func TestPatchFile_PreviewPatchDiff_RejectsInvalidUTF8(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "binary.dat")
	require.NoError(t, os.WriteFile(target, []byte{0xff, 0xfe, 'x'}, 0o644))
	tool := NewPatchFileTool(root, NoopBackupManager())
	_, err := tool.PreviewPatchDiff("binary.dat", []PatchOp{{Search: "x", Replace: "y"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-UTF-8")
}

func TestPatchFile_PreviewPatchDiff_PathTraversal(t *testing.T) {
	tool := NewPatchFileTool(t.TempDir(), NoopBackupManager())
	_, err := tool.PreviewPatchDiff("../escape.txt", []PatchOp{{Search: "x", Replace: "y"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside project root")
}

func TestPatchFile_PreviewPatchDiff_NonexistentFile(t *testing.T) {
	tool := NewPatchFileTool(t.TempDir(), NoopBackupManager())
	_, err := tool.PreviewPatchDiff("missing.txt", []PatchOp{{Search: "x", Replace: "y"}})
	require.Error(t, err)
}

func TestPatchFile_PreviewPatchDiff_DoesNotWrite(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("hello\n"), 0o644))
	tool := NewPatchFileTool(root, NoopBackupManager())
	_, err := tool.PreviewPatchDiff("a.txt", []PatchOp{{Search: "hello", Replace: "bye"}})
	require.NoError(t, err)
	got, _ := os.ReadFile(target)
	assert.Equal(t, "hello\n", string(got), "preview must not mutate the file")
}

func TestPatchFile_PreviewPatchDiff_MultiplePatches(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("alpha\nbeta\n"), 0o644))
	tool := NewPatchFileTool(root, NoopBackupManager())
	unified, err := tool.PreviewPatchDiff("a.txt", []PatchOp{
		{Search: "alpha", Replace: "ALPHA"},
		{Search: "beta", Replace: "BETA"},
	})
	require.NoError(t, err)
	assert.Contains(t, unified, "-alpha")
	assert.Contains(t, unified, "+ALPHA")
	assert.Contains(t, unified, "-beta")
	assert.Contains(t, unified, "+BETA")
}
