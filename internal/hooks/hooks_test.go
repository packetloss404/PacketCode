package hooks

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/config"
)

func TestRunUserPromptSubmit_CollectsStdout(t *testing.T) {
	command := "input=$(cat); case \"$input\" in *hello*) printf injected-context;; *) exit 1;; esac"
	timeoutSec := 2
	if runtime.GOOS == "windows" {
		command = "$data = [Console]::In.ReadToEnd(); if ($data -match 'hello') { 'injected-context' } else { exit 1 }"
		timeoutSec = 5
	}
	if runtime.GOOS == "windows" {
		timeoutSec = 120 // DIAGNOSTIC: measure the real cost, do not truncate it
	}
	r := New(config.HooksConfig{
		UserPromptSubmit: []config.HookConfig{{Command: command, TimeoutSec: timeoutSec}},
	}, t.TempDir())

	start := time.Now()
	out, err := r.RunUserPromptSubmit(context.Background(), PromptPayload{Prompt: "hello"})
	fmt.Fprintf(os.Stderr, "DIAG first-hook-spawn elapsed=%s err=%v\n", time.Since(start).Round(time.Millisecond), err)
	require.NoError(t, err)
	assert.Equal(t, "injected-context", out)

	for i := 0; i < 5; i++ {
		s := time.Now()
		_, err := r.RunUserPromptSubmit(context.Background(), PromptPayload{Prompt: "hello"})
		fmt.Fprintf(os.Stderr, "DIAG warm-hook-spawn[%d] elapsed=%s err=%v\n", i, time.Since(s).Round(time.Millisecond), err)
	}
}

func TestRunPreToolUse_MatcherCanBlock(t *testing.T) {
	command := "echo blocked >&2; exit 7"
	if runtime.GOOS == "windows" {
		command = "Write-Error blocked; exit 7"
	}
	blockTimeout := 2
	if runtime.GOOS == "windows" {
		blockTimeout = 120 // DIAGNOSTIC
	}
	r := New(config.HooksConfig{
		PreToolUse: []config.HookConfig{{Matcher: "execute_command", Command: command, TimeoutSec: blockTimeout}},
	}, t.TempDir())

	blockStart := time.Now()
	_, err := r.RunPreToolUse(context.Background(), ToolPayload{ToolName: "execute_command"})
	fmt.Fprintf(os.Stderr, "DIAG matcher-block-spawn elapsed=%s\n", time.Since(blockStart).Round(time.Millisecond))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")

	_, err = r.RunPreToolUse(context.Background(), ToolPayload{ToolName: "read_file"})
	require.NoError(t, err)
}

func TestRunPreToolUse_TimeoutMessage(t *testing.T) {
	command := "sleep 5"
	if runtime.GOOS == "windows" {
		command = "Start-Sleep -Seconds 5"
	}
	r := New(config.HooksConfig{
		PreToolUse: []config.HookConfig{{Matcher: "execute_command", Command: command, TimeoutSec: 1}},
	}, t.TempDir())

	start := time.Now()
	_, err := r.RunPreToolUse(context.Background(), ToolPayload{ToolName: "execute_command"})
	fmt.Fprintf(os.Stderr, "DIAG timeout-path elapsed=%s err=%v\n", time.Since(start).Round(time.Millisecond), err)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out after 1s")
	assert.Contains(t, err.Error(), "process tree cancellation requested")
}

func TestRunPostToolUse_TruncatesStdoutAndStderr(t *testing.T) {
	command := "(yes o | head -c 70000); (yes e | head -c 70000 >&2); exit 3"
	if runtime.GOOS == "windows" {
		command = "$out = 'o' * 70000; $err = 'e' * 70000; [Console]::Out.Write($out); [Console]::Error.Write($err); exit 3"
	}
	truncTimeout := 5
	if runtime.GOOS == "windows" {
		truncTimeout = 120 // DIAGNOSTIC
	}
	r := New(config.HooksConfig{
		PostToolUse: []config.HookConfig{{Matcher: "execute_command", Command: command, TimeoutSec: truncTimeout}},
	}, t.TempDir())

	truncStart := time.Now()
	out, err := r.RunPostToolUse(context.Background(), ToolPayload{ToolName: "execute_command"})
	fmt.Fprintf(os.Stderr, "DIAG truncate-spawn elapsed=%s\n", time.Since(truncStart).Round(time.Millisecond))
	require.NoError(t, err)
	assert.Contains(t, out, "stdout truncated at 64KB")
	assert.Contains(t, out, "stderr truncated at 64KB")
	assert.Less(t, len(out), 140*1024)
	assert.False(t, strings.Contains(out, strings.Repeat("o", 70000)))
}
