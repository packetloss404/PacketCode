package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/computers"
)

type memoryRuntimeBackend struct {
	files       map[string][]byte
	lastCommand string
	lastCWD     string
}

func (b *memoryRuntimeBackend) ComputerID() string   { return "pc_test" }
func (b *memoryRuntimeBackend) Kind() computers.Kind { return computers.KindSSH }
func (b *memoryRuntimeBackend) Root() string         { return "/srv/app" }
func (b *memoryRuntimeBackend) Close() error         { return nil }

func (b *memoryRuntimeBackend) Resolve(_ context.Context, name string, _ bool) (string, error) {
	clean := path.Clean(name)
	if clean == ".." || len(clean) >= 3 && clean[:3] == "../" || path.IsAbs(clean) {
		return "", fmt.Errorf("path outside project root: %s", name)
	}
	return path.Join(b.Root(), clean), nil
}

func (b *memoryRuntimeBackend) ReadFile(_ context.Context, name string) ([]byte, error) {
	data, ok := b.files[path.Clean(name)]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return append([]byte(nil), data...), nil
}

func (b *memoryRuntimeBackend) WriteFile(_ context.Context, name string, data []byte) error {
	b.files[path.Clean(name)] = append([]byte(nil), data...)
	return nil
}

func (b *memoryRuntimeBackend) ReadDir(context.Context, string) ([]computers.FileEntry, error) {
	return nil, nil
}

func (b *memoryRuntimeBackend) Execute(_ context.Context, command, cwd string, output io.Writer) (computers.ExecResult, error) {
	b.lastCommand = command
	b.lastCWD = cwd
	_, _ = io.WriteString(output, "remote output\n")
	return computers.ExecResult{ExitCode: 0}, nil
}

func TestPatchFileWithRuntimeBackend(t *testing.T) {
	backend := &memoryRuntimeBackend{files: map[string][]byte{"app.txt": []byte("old\n")}}
	tool := NewPatchFileToolWithBackend(backend)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{
		"path":"app.txt","patches":[{"search":"old","replace":"new"}]
	}`))
	require.NoError(t, err)
	assert.False(t, result.IsError, result.Content)
	assert.Equal(t, "new\n", string(backend.files["app.txt"]))
}

func TestExecuteCommandWithRuntimeBackend(t *testing.T) {
	backend := &memoryRuntimeBackend{files: map[string][]byte{}}
	tool := NewExecuteCommandToolWithBackend(backend)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"command":"go test ./...","cwd":"services/api"}`))
	require.NoError(t, err)
	assert.False(t, result.IsError, result.Content)
	assert.Equal(t, "go test ./...", backend.lastCommand)
	assert.Equal(t, "services/api", backend.lastCWD)
	assert.Contains(t, result.Content, "remote output")
}
