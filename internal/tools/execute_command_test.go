package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/procrun"
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
	tool := NewExecuteCommandTool(t.TempDir())
	command := "sleep 5"
	if runtime.GOOS == "windows" {
		command = "ping -n 6 127.0.0.1 >NUL"
	}
	body, _ := json.Marshal(map[string]any{
		"command":     command,
		"timeout_sec": 1,
	})
	start := time.Now()
	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content, "timed out")
	// A real teardown ran, so the line must report what it achieved rather
	// than the nil-outcome wording. Asserting the old "cancellation
	// requested" text is what hid the fact that the evidence never reached
	// here at all: killing the tree makes the child exit non-zero, and the
	// backend used to return that ExitError before it recorded the teardown.
	assert.NotContains(t, res.Content, "outcome unknown",
		"the teardown evidence must reach the caller")
	assert.Regexp(t, `process tree (stopped|NOT confirmed stopped)`, res.Content)
	teardown, ok := res.Metadata["teardown"].(map[string]any)
	require.True(t, ok, "metadata must carry structured teardown evidence, got %#v", res.Metadata["teardown"])
	assert.NotEmpty(t, teardown["method"])
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

func TestExecuteCommand_DescriptionAndSchemaMentionRuntimeSafety(t *testing.T) {
	tool := NewExecuteCommandTool(t.TempDir())
	assert.Contains(t, tool.Description(), "Requires user approval")
	assert.Contains(t, tool.Description(), "Output is truncated past 100KB")

	var schema map[string]any
	require.NoError(t, json.Unmarshal(tool.Schema(), &schema))
	props := schema["properties"].(map[string]any)
	command := props["command"].(map[string]any)
	desc := command["description"].(string)
	if runtime.GOOS == "windows" {
		assert.Contains(t, desc, "cmd /C")
		assert.Contains(t, desc, "PowerShell")
		assert.Contains(t, desc, "WSL")
		assert.Contains(t, desc, "Git Bash")
	} else {
		assert.Contains(t, desc, "sh -c")
	}
}

// TestExecuteCommand_ContextCancelKillsProcess proves that cancelling
// the ctx handed to Execute promptly tears down the underlying process.
// Round 5 relies on this: Ctrl+C at the App layer cancels the turn
// ctx, which the agent passes through to tool.Execute, which must kill
// anything mid-flight.
func TestExecuteCommand_ContextCancelKillsProcess(t *testing.T) {
	tool := NewExecuteCommandTool(t.TempDir())
	command := "sleep 30"
	if runtime.GOOS == "windows" {
		command = "ping -n 31 127.0.0.1 >NUL"
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
	assert.Less(t, elapsed, 1500*time.Millisecond, "Execute must return promptly after ctx cancel; took %s", elapsed)
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

// --- output streaming (producer side) ---
//
// These tests pin the OBSERVABLE behavior the live feed must not disturb: a
// slow, multi-line command runs to completion with all output captured, the
// bounded cap survives high-volume output, and cancellation still tears the
// process down promptly. TestExecuteCommand_StreamsIncrementally below covers
// the sink itself.

// slowMultiLineCommand emits several lines spaced out in time so that a true
// incremental streamer would deliver early lines well before process exit.
func slowMultiLineCommand() string {
	if runtime.GOOS == "windows" {
		// Each ping -n 2 waits ~1s; echo between them produces interleaved lines.
		return "echo line1 & ping -n 2 127.0.0.1 >NUL & echo line2 & ping -n 2 127.0.0.1 >NUL & echo line3"
	}
	return "printf 'line1\\n'; sleep 0.3; printf 'line2\\n'; sleep 0.3; printf 'line3\\n'"
}

// TestExecuteCommand_SlowMultiLineCapturesAll verifies that a slow, multi-line
// command completes successfully and that every emitted line is present in the
// final captured output. This is the producer-side guarantee the streaming
// round must not regress: streaming the lines incrementally must still leave the
// full (uncapped-here) content in the returned ToolResult.
func TestExecuteCommand_SlowMultiLineCapturesAll(t *testing.T) {
	tool := NewExecuteCommandTool(t.TempDir())
	body, _ := json.Marshal(map[string]any{"command": slowMultiLineCommand()})

	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.False(t, res.IsError, "slow multi-line command should exit 0: %s", res.Content)
	for _, line := range []string{"line1", "line2", "line3"} {
		assert.Contains(t, res.Content, line, "captured output must include every streamed line")
	}
	assert.Contains(t, res.Content, "[exit 0]")
	assert.Equal(t, false, res.Metadata["truncated"], "small multi-line output must not be truncated")
}

// TestExecuteCommand_StreamingPreservesBoundedCap verifies that high-volume
// output is still truncated at the 100KB cap. The streaming round delivers
// chunks to the UI as they arrive, but the FINAL tool result must remain
// bounded — the streamed view is unbounded, the captured/returned buffer is not.
func TestExecuteCommand_StreamingPreservesBoundedCap(t *testing.T) {
	root := t.TempDir()
	bigFile := root + string(os.PathSeparator) + "big.txt"
	// 120KB of data > the 100KB cap. Written line-wise so a streaming producer
	// would emit many chunks before hitting the cap.
	require.NoError(t, os.WriteFile(bigFile, []byte(strings.Repeat("packetcode-stream-line\n", 6000)), 0o600))
	command := "cat big.txt"
	if runtime.GOOS == "windows" {
		command = "type big.txt"
	}
	tool := NewExecuteCommandTool(root)
	body, _ := json.Marshal(map[string]any{"command": command})

	res, err := tool.Execute(context.Background(), body)
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Contains(t, res.Content, "output truncated at 100KB", "bounded cap must survive streaming")
	assert.Equal(t, true, res.Metadata["truncated"])
	// Content is the cap plus a small fixed header/footer; well under 106KB.
	assert.Less(t, len(res.Content), 106*1024, "returned content must stay bounded even under high-volume output")
}

// TestExecuteCommand_StreamingStillCancels verifies cancellation continues to
// work for a slow command. Streaming output incrementally must not break the
// Round 2 guarantee that a cancelled ctx promptly kills the process tree.
func TestExecuteCommand_StreamingStillCancels(t *testing.T) {
	tool := NewExecuteCommandTool(t.TempDir())
	// A long, slow producer: would stream many lines if allowed to run.
	command := "for i in 1 2 3 4 5 6 7 8 9 10; do printf 'tick %d\\n' \"$i\"; sleep 1; done"
	if runtime.GOOS == "windows" {
		command = "for /L %i in (1,1,10) do (echo tick %i & ping -n 2 127.0.0.1 >NUL)"
	}
	body, _ := json.Marshal(map[string]any{"command": command})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	res, err := tool.Execute(ctx, body)
	elapsed := time.Since(start)

	require.NoError(t, err, "Execute should fold the killed-process error into a ToolResult")
	assert.True(t, res.IsError, "cancelled streaming run should be flagged as an error")
	assert.Contains(t, res.Content, "canceled")
	assert.NotContains(t, res.Content, "[exit 0]")
	assert.Less(t, elapsed, 2*time.Second, "cancellation must return promptly while streaming; took %s", elapsed)
}

// recordingSink captures every chunk with the time it arrived. WriteChunk runs
// on the goroutine draining the child's pipe, so it must be safe to call
// concurrently with the test's own reads.
type recordingSink struct {
	mu     sync.Mutex
	chunks []string
	firstA time.Time
}

func (r *recordingSink) WriteChunk(chunk string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.firstA.IsZero() {
		r.firstA = time.Now()
	}
	r.chunks = append(r.chunks, chunk)
}

func (r *recordingSink) snapshot() ([]string, time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.chunks...), r.firstA
}

// TestExecuteCommand_StreamsIncrementally proves the sink sees output WHILE the
// command is still running, not in one burst at exit. The command prints a line,
// then idles for seconds, then prints again; if chunks were only delivered on
// exit, the first chunk would land at (essentially) the moment Execute returns.
//
// Deliberately asserted as a MINIMUM gap rather than a maximum latency: machine
// load can only stretch the interval between the first line and process exit,
// so this direction cannot fail spuriously under a busy CPU.
func TestExecuteCommand_StreamsIncrementally(t *testing.T) {
	command := "printf 'line1\n'; sleep 2; printf 'line2\n'"
	if runtime.GOOS == "windows" {
		// ping -n 4 waits ~3s between the two echoes.
		command = "echo line1& ping -n 4 127.0.0.1 >NUL& echo line2"
	}
	tool := NewExecuteCommandTool(t.TempDir())
	body, _ := json.Marshal(map[string]any{"command": command})
	sink := &recordingSink{}

	res, err := tool.ExecuteStreaming(context.Background(), body, sink)
	returnedAt := time.Now()
	require.NoError(t, err)
	require.False(t, res.IsError, res.Content)

	chunks, firstAt := sink.snapshot()
	require.NotEmpty(t, chunks, "sink must receive live output")
	require.False(t, firstAt.IsZero())
	assert.Contains(t, chunks[0], "line1", "the first chunk must be the first line, not the whole run")
	assert.GreaterOrEqual(t, returnedAt.Sub(firstAt), 500*time.Millisecond,
		"first chunk arrived %s before Execute returned; that is a single end-of-run flush, not streaming",
		returnedAt.Sub(firstAt))

	streamed := strings.Join(chunks, "")
	assert.Contains(t, streamed, "line1")
	assert.Contains(t, streamed, "line2")
	// The sink is additive: the bounded, model-facing result is unchanged.
	assert.Contains(t, res.Content, "line1")
	assert.Contains(t, res.Content, "line2")
	assert.Contains(t, res.Content, "[exit 0]")
}

// A nil sink must be exactly Execute: same result, no panic on the chunker.
func TestExecuteCommand_NilSinkMatchesExecute(t *testing.T) {
	tool := NewExecuteCommandTool(t.TempDir())
	body, _ := json.Marshal(map[string]any{"command": shellEcho("nil-sink")})
	res, err := tool.ExecuteStreaming(context.Background(), body, nil)
	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Contains(t, res.Content, "nil-sink")
	assert.Contains(t, res.Content, "[exit 0]")
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

// The old wording said cancellation was "requested", which is true in every
// case and so distinguishes nothing: it read identically whether the process
// had stopped or was still running. These pin the three states apart.
func TestDescribeTeardown_SeparatesConfirmedFromUnknown(t *testing.T) {
	cases := []struct {
		name    string
		outcome *procrun.KillOutcome
		want    []string
		absent  []string
	}{
		{
			name:    "no teardown ran",
			outcome: nil,
			want:    []string{"outcome unknown"},
			absent:  []string{"NOT confirmed"},
		},
		{
			name:    "job object contained the tree",
			outcome: &procrun.KillOutcome{Method: procrun.KillMethodJobObject, Confirmed: true},
			want:    []string{"tree stopped (", "job-object"},
			absent:  []string{"NOT confirmed", "unknown"},
		},
		{
			name: "survivors are named, not summarised away",
			outcome: &procrun.KillOutcome{
				Method:    procrun.KillMethodTreeWalk,
				Survivors: []int{4242, 4243},
			},
			want:   []string{"NOT confirmed", "tree-walk", "4242", "4243"},
			absent: []string{"tree stopped ("},
		},
		{
			name: "an unconfirmed remote teardown keeps its reason",
			outcome: &procrun.KillOutcome{
				Method: procrun.KillMethodNone,
				Reason: "sshd may ignore channel signals",
			},
			want:   []string{"NOT confirmed", "sshd may ignore channel signals"},
			absent: []string{"tree stopped ("},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := describeTeardown(tc.outcome)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("describeTeardown = %q, want it to contain %q", got, want)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(got, absent) {
					t.Fatalf("describeTeardown = %q, want it NOT to contain %q", got, absent)
				}
			}
		})
	}
}

// A nil outcome must stay nil in metadata. Emitting a zero value would report
// "not confirmed" for commands that were never cancelled at all.
func TestTeardownMetadata_NilStaysNil(t *testing.T) {
	if got := teardownMetadata(nil); got != nil {
		t.Fatalf("teardownMetadata(nil) = %v, want nil", got)
	}
	meta := teardownMetadata(&procrun.KillOutcome{
		Method:    procrun.KillMethodProcessGroup,
		Survivors: []int{7},
		Reason:    "why",
	})
	if meta["method"] != "process-group" || meta["confirmed"] != false {
		t.Fatalf("unexpected metadata: %v", meta)
	}
	if meta["reason"] != "why" {
		t.Fatalf("reason dropped: %v", meta)
	}
	if _, ok := meta["survivors"]; !ok {
		t.Fatalf("survivors dropped: %v", meta)
	}
}
