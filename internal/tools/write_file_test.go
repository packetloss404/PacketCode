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

type recordingBackup struct {
	calls []string
}

func (r *recordingBackup) Backup(p string) error {
	r.calls = append(r.calls, p)
	return nil
}

func TestWriteFile_NewFile(t *testing.T) {
	root := t.TempDir()
	bk := &recordingBackup{}
	tool := NewWriteFileTool(root, bk)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"new/dir/hello.go","content":"package hello\n"}`))
	require.NoError(t, err)
	assert.False(t, res.IsError)

	got, err := os.ReadFile(filepath.Join(root, "new/dir/hello.go"))
	require.NoError(t, err)
	assert.Equal(t, "package hello\n", string(got))
	assert.Len(t, bk.calls, 1, "backup is called once even for nonexistent file")
}

func TestWriteFile_OverwriteCallsBackup(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "main.go")
	require.NoError(t, os.WriteFile(target, []byte("// old\n"), 0o644))

	bk := &recordingBackup{}
	tool := NewWriteFileTool(root, bk)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"main.go","content":"// new\n"}`))
	require.NoError(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "// new\n", string(got))
	assert.Equal(t, []string{target}, bk.calls)
}

func TestWriteFile_RequiresApproval(t *testing.T) {
	tool := NewWriteFileTool(t.TempDir(), nil)
	assert.True(t, tool.RequiresApproval())
}

func TestWriteFile_PathValidation(t *testing.T) {
	tool := NewWriteFileTool(t.TempDir(), nil)
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"../escape.txt","content":"x"}`))
	require.NoError(t, err)
	assert.True(t, res.IsError)
}
