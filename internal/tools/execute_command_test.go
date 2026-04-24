package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
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
	assert.Contains(t, res.Content, "process tree cancellation requested")
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

// TestExecuteCommand_ContextCancelKillsProcess proves that cancelling
// the ctx handed to Execute tears down the underlying process within
// 1s. Round 5 relies on this: Ctrl+C at the App layer cancels the turn
// ctx, which the agent passes through to tool.Execute, which must kill
// anything mid-flight. Skipped on Windows — `timeout /t 30` has
// tortured stdout semantics under cmd.exe and the payoff doesn't
// justify the platform dance.
func TestExecuteCommand_ContextCancelKillsProcess(t *testing.T) {
	tool := NewExecuteCommandTool(t.TempDir())
	command := "sleep 30"
	if runtime.GOOS == "windows" {
		command = `powershell -NoProfile -Command "Start-Sleep -Seconds 30"`
	}
	body, _ := json.Marshal(map[string]any{"command": command})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	res, err := tool.Execute(ctx, body)
	elapsed := time.Since(start)

	require.NoError(t, err, "Execute should swallow the killed-process error into a ToolResult")
	assert.True(t, res.IsError, "cancelled run should be flagged as an error")
	assert.Contains(t, res.Content, "canceled")
	assert.NotContains(t, res.Content, "[exit 0]")
	assert.Less(t, elapsed, 1*time.Second, "Execute must return within 1s of ctx cancel; took %s", elapsed)
}

func TestExecuteCommand_NonZeroExitIsNotCancellation(t *testing.T) {
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
	assert.NotContains(t, res.Content, "canceled")
	assert.NotContains(t, res.Content, "timed out")
}

func TestExecuteCommand_TruncatesCapturedOutput(t *testing.T) {
	root := t.TempDir()
	bigFile := root + string(os.PathSeparator) + "big.txt"
	require.NoError(t, os.WriteFile(bigFile, []byte(strings.Repeat("x", 120000)), 0o600))
	command := "cat big.txt"
	if runtime.GOOS == "windows" {
		command = "type big.txt"
	}
	tool := NewExecuteCommandTool(root)
	body, _ := json.Marshal(map[string]any{"command": command})
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Contains(t, res.Content, "output truncated at 100KB")
	assert.Less(t, len(res.Content), 106*1024)
	assert.Equal(t, true, res.Metadata["truncated"])
}

func TestExecuteCommand_CancelsPOSIXProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows descendant enumeration is environment-dependent; taskkill path is covered by fast cancel test")
	}
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep unavailable")
	}
	root := t.TempDir()
	pidFile := root + string(os.PathSeparator) + "child.pid"
	command := "sleep 30 & printf %s $! > " + strconv.Quote(pidFile) + "; wait"
	tool := NewExecuteCommandTool(root)
	body, _ := json.Marshal(map[string]any{"command": command})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan ToolResult, 1)
	go func() {
		res, err := tool.Execute(ctx, body)
		require.NoError(t, err)
		done <- res
	}()

	var pidBytes []byte
	require.Eventually(t, func() bool {
		var err error
		pidBytes, err = os.ReadFile(pidFile)
		return err == nil && strings.TrimSpace(string(pidBytes)) != ""
	}, time.Second, 20*time.Millisecond)
	cancel()
	res := <-done
	assert.True(t, res.IsError)

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	require.NoError(t, err)
	assert.Eventually(t, func() bool {
		return exec.Command("kill", "-0", strconv.Itoa(pid)).Run() != nil
	}, time.Second, 20*time.Millisecond, "child process should be killed with the shell process group")
}
