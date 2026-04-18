package tools

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shellEcho emits a portable echo invocation for the test command.
// Windows cmd.exe uses double quotes around the echoed string differently
// than POSIX sh, so we keep both branches simple.
func shellEcho(s string) string {
	if runtime.GOOS == "windows" {
		return "echo " + s
	}
	return "echo '" + s + "'"
}

func TestExecuteCommand_RunsAndCapturesStdout(t *testing.T) {
	tool := NewExecuteCommandTool(t.TempDir())
	body, _ := json.Marshal(map[string]any{"command": shellEcho("hello-world")})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Contains(t, res.Content, "hello-world")
	assert.Contains(t, res.Content, "[exit 0]")
}

func TestExecuteCommand_NonZeroExit(t *testing.T) {
	cmd := "exit 7"
	if runtime.GOOS == "windows" {
		cmd = "cmd /C exit 7"
	}
	tool := NewExecuteCommandTool(t.TempDir())
	body, _ := json.Marshal(map[string]any{"command": cmd})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content, "[exit 7]")
}

func TestExecuteCommand_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep semantics differ on Windows; covered indirectly via context handling")
	}
	tool := NewExecuteCommandTool(t.TempDir())
	body, _ := json.Marshal(map[string]any{
		"command":     "sleep 5",
		"timeout_sec": 1,
	})
	start := time.Now()
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content, "timed out")
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestExecuteCommand_RejectsCWDOutsideRoot(t *testing.T) {
	tool := NewExecuteCommandTool(t.TempDir())
	body, _ := json.Marshal(map[string]any{
		"command": shellEcho("hi"),
		"cwd":     "../escape",
	})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content, "outside project root")
}

func TestExecuteCommand_RequiresApproval(t *testing.T) {
	tool := NewExecuteCommandTool(t.TempDir())
	assert.True(t, tool.RequiresApproval())
}
