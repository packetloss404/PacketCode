package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/mcp"
	"github.com/packetcode/packetcode/internal/provider"
)

func withRunExecutor(t *testing.T, fn func(context.Context, runCommandOptions, io.Writer) (runResult, error)) {
	t.Helper()
	previous := executeRunCommand
	executeRunCommand = func(ctx context.Context, opts runCommandOptions, stderr io.Writer) (runResult, error) {
		return fn(ctx, opts, stderr)
	}
	t.Cleanup(func() { executeRunCommand = previous })
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRunCommand_ParsesFlagsAndPrintsSanitizedFinalResponse(t *testing.T) {
	withRunExecutor(t, func(_ context.Context, opts runCommandOptions, _ io.Writer) (runResult, error) {
		if opts.Provider != "openai" || opts.Model != "gpt-test" || opts.PermissionMode != "auto" || opts.ResumeID != "12345678" {
			t.Fatalf("unexpected options: %+v", opts)
		}
		if opts.Prompt != "fix the tests" {
			t.Fatalf("prompt = %q", opts.Prompt)
		}
		return runResult{Output: "safe\x1b]52;c;ZXZpbA==\a text\n"}, nil
	})

	var stdout, stderr bytes.Buffer
	code := runRunCommand([]string{
		"--provider", "openai", "--model", "gpt-test", "--permission-mode", "auto",
		"--resume", "12345678", "fix", "the", "tests",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "safe text\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunCommand_JSONIsOneVersionedDocument(t *testing.T) {
	withRunExecutor(t, func(_ context.Context, opts runCommandOptions, _ io.Writer) (runResult, error) {
		if !opts.JSON {
			t.Fatal("JSON option was not propagated")
		}
		return runResult{
			SessionID: "session-1", Provider: "openai", Model: "gpt-test", Output: "done",
			Usage: runUsage{InputTokens: 11, OutputTokens: 3, CacheCreationInputTokens: 2, CacheReadInputTokens: 7},
		}, nil
	})

	var stdout, stderr bytes.Buffer
	if code := runRunCommand([]string{"--json", "say", "hello"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var got runResult
	decoder := json.NewDecoder(&stdout)
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("decode JSON: %v; output = %q", err, stdout.String())
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout contained more than one JSON value: %v", err)
	}
	if got.SchemaVersion != 1 || !got.OK || got.SessionID != "session-1" || got.Output != "done" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if got.Usage.InputTokens != 11 || got.Usage.CacheReadInputTokens != 7 {
		t.Fatalf("unexpected usage: %+v", got.Usage)
	}
}

func TestRunCommand_ExitCodesAndDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "general", err: errors.New("provider failed"), want: runExitError},
		{name: "approval", err: errRunApprovalUnavailable, want: runExitApprovalUnavailable},
		{name: "cancel", err: context.Canceled, want: runExitCanceled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withRunExecutor(t, func(context.Context, runCommandOptions, io.Writer) (runResult, error) {
				return runResult{}, tc.err
			})
			var stdout, stderr bytes.Buffer
			if code := runRunCommand([]string{"prompt"}, &stdout, &stderr); code != tc.want {
				t.Fatalf("exit = %d, want %d", code, tc.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("plain error leaked to stdout: %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), tc.err.Error()) {
				t.Fatalf("stderr = %q, want error %q", stderr.String(), tc.err)
			}
		})
	}
}

func TestRunCommand_JSONErrorRemainsOneDocument(t *testing.T) {
	withRunExecutor(t, func(context.Context, runCommandOptions, io.Writer) (runResult, error) {
		return runResult{SessionID: "session-1", Provider: "openai", Model: "gpt-test"}, errRunApprovalUnavailable
	})
	var stdout, stderr bytes.Buffer
	if code := runRunCommand([]string{"--json", "prompt"}, &stdout, &stderr); code != runExitApprovalUnavailable {
		t.Fatalf("exit = %d", code)
	}
	var got runResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v; output = %q", err, stdout.String())
	}
	if got.OK || got.Error != errRunApprovalUnavailable.Error() || got.SchemaVersion != 1 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if !strings.Contains(stderr.String(), errRunApprovalUnavailable.Error()) {
		t.Fatalf("missing stderr diagnostic: %q", stderr.String())
	}
}

func TestRunCommand_PlainOutputWriteFailureIsAnError(t *testing.T) {
	withRunExecutor(t, func(_ context.Context, _ runCommandOptions, _ io.Writer) (runResult, error) {
		return runResult{Output: "finished"}, nil
	})
	var stderr bytes.Buffer
	code := runRunCommand([]string{"do", "work"}, failingWriter{err: errors.New("broken pipe")}, &stderr)
	if code != runExitError {
		t.Fatalf("exit code = %d, want %d", code, runExitError)
	}
	if !strings.Contains(stderr.String(), "write response: broken pipe") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunCommand_HelpAndUsageErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runRunCommand([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help exit = %d", code)
	}
	if !strings.Contains(stdout.String(), "packetcode run") || !strings.Contains(stdout.String(), "--permission-mode") {
		t.Fatalf("incomplete help: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runRunCommand(nil, &stdout, &stderr); code != runExitUsage {
		t.Fatalf("missing prompt exit = %d", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "prompt is required") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}

	stderr.Reset()
	if code := runRunCommand([]string{"--not-a-flag", "prompt"}, &stdout, &stderr); code != runExitUsage {
		t.Fatalf("bad flag exit = %d", code)
	}

	stderr.Reset()
	if code := runRunCommand([]string{"--permission-mode", "anything-goes", "prompt"}, &stdout, &stderr); code != runExitUsage {
		t.Fatalf("bad permission mode exit = %d", code)
	}
}

func TestCollectRunEvents_ReturnsFinalTurnAndPerRunUsage(t *testing.T) {
	events := make(chan agent.AgentEvent, 8)
	events <- agent.AgentEvent{Type: agent.EventTextDelta, Text: "I will inspect"}
	events <- agent.AgentEvent{Type: agent.EventUsageUpdate, Usage: provider.Usage{
		InputTokens: 10, OutputTokens: 2, CacheReadInputTokens: 6,
	}}
	events <- agent.AgentEvent{Type: agent.EventToolCallProposed}
	events <- agent.AgentEvent{Type: agent.EventTextDelta, Text: "fixed"}
	events <- agent.AgentEvent{Type: agent.EventTextDelta, Text: " it"}
	events <- agent.AgentEvent{Type: agent.EventUsageUpdate, Usage: provider.Usage{
		InputTokens: 14, OutputTokens: 3, CacheCreationInputTokens: 4, CacheReadInputTokens: 8,
	}}
	events <- agent.AgentEvent{Type: agent.EventDone}
	close(events)

	output, usage, err := collectRunEvents(context.Background(), events)
	if err != nil {
		t.Fatal(err)
	}
	if output != "fixed it" {
		t.Fatalf("output = %q", output)
	}
	if usage != (runUsage{InputTokens: 24, OutputTokens: 5, CacheCreationInputTokens: 4, CacheReadInputTokens: 14}) {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestCollectRunEvents_TerminalFailures(t *testing.T) {
	t.Run("agent error", func(t *testing.T) {
		events := make(chan agent.AgentEvent, 1)
		events <- agent.AgentEvent{Type: agent.EventError, Error: errors.New("stream failed")}
		close(events)
		_, _, err := collectRunEvents(context.Background(), events)
		if err == nil || err.Error() != "stream failed" {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("early close", func(t *testing.T) {
		events := make(chan agent.AgentEvent)
		close(events)
		_, _, err := collectRunEvents(context.Background(), events)
		if err == nil || !strings.Contains(err.Error(), "without a terminal event") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := collectRunEvents(ctx, make(chan agent.AgentEvent))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestNonInteractiveApprover_RejectsAndRecordsAttempt(t *testing.T) {
	approver := &nonInteractiveApprover{}
	decision := approver.Approve(context.Background(), agent.ApprovalRequest{})
	if decision.Approved || !approver.Asked() || decision.Reason != errRunApprovalUnavailable.Error() {
		t.Fatalf("decision = %+v, asked = %v", decision, approver.Asked())
	}
}

func TestWriteRunMCPReportsSurfacesStartupFailures(t *testing.T) {
	var out bytes.Buffer
	writeRunMCPReports(&out, []mcp.StartupReport{
		{Name: "files", Status: "running", ToolCount: 3, PID: 42},
		{Name: "browser", Status: "failed", Err: "handshake timeout"},
	})
	got := out.String()
	for _, want := range []string{
		"packetcode run: mcp files: 3 tools, pid 42",
		"packetcode run: mcp browser: failed — handshake timeout",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("startup reports %q do not contain %q", got, want)
		}
	}
}
