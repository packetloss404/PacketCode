package hooks

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/testwait"
)

// TestMain spawns the hook interpreter once before any test's budget is
// running, because on Windows the first one costs about twenty-five times what
// every later one does and that cost has nothing to do with what these tests
// assert.
//
// Measured on four GitHub `windows-latest` runners: the first
// `powershell -Command "exit 0"` in a job took 4.33s, 4.66s, 4.63s and 4.87s,
// while every subsequent one took 0.16-0.19s and a bare `cmd.exe` CreateProcess
// at the same instant took 15-38ms. So the machine was not busy, the stdin
// plumbing was not slow (no-stdin 170ms, stdin attached 175ms, full hook script
// 180ms) and internal/hooks added nothing (184ms with the tree-cancel wiring,
// 194ms end to end through Runner). It is Windows PowerShell's own start-up --
// faulting in and JITing the assemblies behind System.Management.Automation
// from a cold image -- paid once per machine rather than once per spawn.
//
// Whichever test spawned first absorbed all of it. That was
// TestRunUserPromptSubmit_CollectsStdout, whose 5s budget sat 0.1-0.7s above a
// 4.6s constant, so whether it passed was decided by noise: it failed roughly
// two runs in three on CI and never once on a developer machine, which is the
// one result a test must never produce. Paying the cost here makes every
// budget in this file mean the same thing, instead of loading the whole
// per-machine cost onto whichever test happens to run first.
func TestMain(m *testing.M) {
	warmShellInterpreter()
	os.Exit(m.Run())
}

// warmShellInterpreter runs the cheapest possible command through the same
// shellCommand the hooks use, so the warm-up covers whatever interpreter
// production actually picks rather than a copy of that choice that can drift.
//
// Its result is deliberately ignored. It asserts nothing, and a machine where
// it fails is one where the tests below should report the problem themselves
// against their own scaled budgets -- not one where the suite refuses to start.
func warmShellInterpreter() {
	ctx, cancel := context.WithTimeout(context.Background(), testwait.Timeout(5*time.Second))
	defer cancel()
	cmd := shellCommand(ctx, "exit 0")
	cmd.Stdin = strings.NewReader("")
	_ = cmd.Run()
}

func TestRunUserPromptSubmit_CollectsStdout(t *testing.T) {
	command := "input=$(cat); case \"$input\" in *hello*) printf injected-context;; *) exit 1;; esac"
	if runtime.GOOS == "windows" {
		command = "$data = [Console]::In.ReadToEnd(); if ($data -match 'hello') { 'injected-context' } else { exit 1 }"
	}
	r := New(config.HooksConfig{
		UserPromptSubmit: []config.HookConfig{{Command: command, TimeoutSec: testwait.Seconds(2 * time.Second)}},
	}, t.TempDir())

	out, err := r.RunUserPromptSubmit(context.Background(), PromptPayload{Prompt: "hello"})
	require.NoError(t, err)
	assert.Equal(t, "injected-context", out)
}

func TestRunPreToolUse_MatcherCanBlock(t *testing.T) {
	command := "echo blocked >&2; exit 7"
	if runtime.GOOS == "windows" {
		command = "Write-Error blocked; exit 7"
	}
	// Scaled for the same reason as the hook above, and with less room to
	// spare than the 2 looks: on the runners measured this hook took 1.0-1.7s
	// against its two-second budget, because Write-Error builds and formats a
	// full ErrorRecord and so costs several times a bare spawn. It survived
	// only because the test above it absorbed the interpreter's start-up.
	r := New(config.HooksConfig{
		PreToolUse: []config.HookConfig{{Matcher: "execute_command", Command: command, TimeoutSec: testwait.Seconds(2 * time.Second)}},
	}, t.TempDir())

	_, err := r.RunPreToolUse(context.Background(), ToolPayload{ToolName: "execute_command"})
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
	// Not scaled, unlike every other budget in this file, and not an oversight.
	// Here the timeout is the subject rather than the scaffolding: the test
	// asserts the hook gives up at one second instead of waiting out the
	// five-second sleep, so a scaled budget would assert nothing. Scaling the
	// three-second ceiling would be worse still -- past five seconds it can no
	// longer tell a cancelled hook from one that ran to completion.
	//
	// It does not need the slack either. Measured on the same runs that put the
	// hook above 0.6s over its budget, this path took 1.02-1.05s, because the
	// deadline starts at Run and fires while the interpreter is still starting:
	// what it measures is when cancellation happened, not how long a spawn took.
	r := New(config.HooksConfig{
		PreToolUse: []config.HookConfig{{Matcher: "execute_command", Command: command, TimeoutSec: 1}},
	}, t.TempDir())

	start := time.Now()
	_, err := r.RunPreToolUse(context.Background(), ToolPayload{ToolName: "execute_command"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out after 1s")
	assert.Contains(t, err.Error(), "process tree cancellation requested")
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestRunPostToolUse_TruncatesStdoutAndStderr(t *testing.T) {
	command := "(yes o | head -c 70000); (yes e | head -c 70000 >&2); exit 3"
	if runtime.GOOS == "windows" {
		command = "$out = 'o' * 70000; $err = 'e' * 70000; [Console]::Out.Write($out); [Console]::Error.Write($err); exit 3"
	}
	r := New(config.HooksConfig{
		PostToolUse: []config.HookConfig{{Matcher: "execute_command", Command: command, TimeoutSec: testwait.Seconds(5 * time.Second)}},
	}, t.TempDir())

	out, err := r.RunPostToolUse(context.Background(), ToolPayload{ToolName: "execute_command"})
	require.NoError(t, err)
	assert.Contains(t, out, "stdout truncated at 64KB")
	assert.Contains(t, out, "stderr truncated at 64KB")
	assert.Less(t, len(out), 140*1024)
	assert.False(t, strings.Contains(out, strings.Repeat("o", 70000)))
}
