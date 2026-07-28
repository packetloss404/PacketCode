package statusline

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/config"
)

// TestSnapshotClaudeCodeCompat verifies the emitted JSON carries both
// packetcode's native fields and the Claude Code-compatible aliases, so a
// statusline script written for Claude Code works unchanged.
func TestSnapshotClaudeCodeCompat(t *testing.T) {
	snap := Snapshot{
		WorkingDir:    "/home/me/proj",
		Model:         ModelInfo{ID: "gpt-5.6-sol"},
		ContextWindow: ContextInfo{Used: 12000, Max: 272000, UsedPercentage: 4},
	}
	raw, err := json.Marshal(snap)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	// Native fields still present.
	assert.Equal(t, "/home/me/proj", got["working_dir"])
	// Claude Code alias for the working directory.
	assert.Equal(t, "/home/me/proj", got["cwd"])

	// model.display_name falls back to the id.
	model := got["model"].(map[string]any)
	assert.Equal(t, "gpt-5.6-sol", model["id"])
	assert.Equal(t, "gpt-5.6-sol", model["display_name"])

	// context_window: native + Claude Code aliases.
	cw := got["context_window"].(map[string]any)
	assert.EqualValues(t, 4, cw["used_percentage"])
	assert.EqualValues(t, 272000, cw["context_window_size"])
	usage := cw["current_usage"].(map[string]any)
	assert.EqualValues(t, 12000, usage["input_tokens"])
	assert.EqualValues(t, 0, usage["cache_read_input_tokens"])
}

// TestModelDisplayNameHonoured confirms an explicit display name wins over the id.
func TestModelDisplayNameHonoured(t *testing.T) {
	raw, err := json.Marshal(ModelInfo{ID: "gpt-5.6-sol", DisplayName: "GPT-5.6-Sol"})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "GPT-5.6-Sol", got["display_name"])
}

func TestRunner_RenderPassesJSONOnStdin(t *testing.T) {
	command := "read input; case \"$input\" in *gpt-test*) printf custom-status;; *) exit 1;; esac"
	timeoutSec := 2
	if runtime.GOOS == "windows" {
		command = "$data = [Console]::In.ReadToEnd(); if ($data -match 'gpt-test') { 'custom-status' } else { exit 1 }"
		timeoutSec = 5
	}
	r := New(config.StatusLineConfig{Command: command, TimeoutSec: timeoutSec}, t.TempDir())
	require.NotNil(t, r)

	out, err := r.Render(context.Background(), Snapshot{
		Provider: ProviderInfo{Slug: "openai", DisplayName: "OpenAI"},
		Model:    ModelInfo{ID: "gpt-test"},
	})
	require.NoError(t, err)
	assert.Equal(t, "custom-status", out)
}

func TestNew_DisabledWithoutCommand(t *testing.T) {
	assert.Nil(t, New(config.StatusLineConfig{}, t.TempDir()))
}

func TestRunner_RenderTimeoutMessage(t *testing.T) {
	command := "sleep 5"
	if runtime.GOOS == "windows" {
		command = "Start-Sleep -Seconds 5"
	}
	r := New(config.StatusLineConfig{Command: command, TimeoutSec: 1}, t.TempDir())
	require.NotNil(t, r)

	start := time.Now()
	_, err := r.Render(context.Background(), Snapshot{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out after 1s")
	assert.Contains(t, err.Error(), "process tree cancellation requested")
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestRunner_RenderTruncatesStdout(t *testing.T) {
	command := "yes s | head -c 70000"
	if runtime.GOOS == "windows" {
		command = "$out = 's' * 70000; [Console]::Out.Write($out)"
	}
	r := New(config.StatusLineConfig{Command: command, TimeoutSec: 5}, t.TempDir())
	require.NotNil(t, r)

	out, err := r.Render(context.Background(), Snapshot{})
	require.NoError(t, err)
	assert.Contains(t, out, "stdout truncated at 64KB")
	assert.Less(t, len(out), 70*1024)
	assert.False(t, strings.Contains(out, strings.Repeat("s", 70000)))
}

func TestRenderDefault_Format(t *testing.T) {
	line := RenderDefault(Snapshot{
		Project:       "packetcode",
		GitBranch:     "main",
		Provider:      ProviderInfo{Slug: "codex", DisplayName: "Codex (ChatGPT)"},
		Model:         ModelInfo{ID: "gpt-5.6-sol"},
		ContextWindow: ContextInfo{Used: 42000, Max: 272000, UsedPercentage: 15},
		Cost:          CostInfo{TotalCostUSD: 0}, // subscription → hidden
		Operation:     OperationInfo{Active: true, Label: "thinking", ElapsedSeconds: 5, QueuedInputs: 1},
	})
	want := "[Codex (ChatGPT)·gpt-5.6-sol] 🟢 15% (42K/272K) | 📂 packetcode | 🌿 main | ◷ thinking 5s (+1 queued)"
	if line != want {
		t.Fatalf("RenderDefault mismatch:\n got: %q\nwant: %q", line, want)
	}

	// Cost shows when > 0; idle hides the operation segment; red at high ctx.
	line2 := RenderDefault(Snapshot{
		WorkingDir:    "/tmp/app",
		Provider:      ProviderInfo{DisplayName: "OpenAI"},
		Model:         ModelInfo{ID: "gpt-5.5"},
		ContextWindow: ContextInfo{Used: 230000, Max: 272000, UsedPercentage: 85},
		Cost:          CostInfo{TotalCostUSD: 1.2345},
	})
	want2 := "[OpenAI·gpt-5.5] 🔴 85% (230K/272K) | 📂 app | 🌿 - | 💲1.23"
	if line2 != want2 {
		t.Fatalf("RenderDefault mismatch:\n got: %q\nwant: %q", line2, want2)
	}
}

func TestRenderDefaultWidth_KeepsCriticalStateOnOneLine(t *testing.T) {
	snap := Snapshot{
		Project:       "packetcode",
		GitBranch:     "feature/very-long-branch-name",
		Provider:      ProviderInfo{DisplayName: "Codex (ChatGPT)"},
		Model:         ModelInfo{ID: "gpt-5.6-sol"},
		ContextWindow: ContextInfo{Used: 42000, Max: 272000, UsedPercentage: 15},
		Jobs:          JobsInfo{Active: 2},
		Operation:     OperationInfo{Active: true, Label: "thinking", ElapsedSeconds: 5},
	}
	line := RenderDefaultWidth(snap, 70)
	assert.NotContains(t, line, "\n")
	assert.LessOrEqual(t, runewidth.StringWidth(line), 70)
	assert.Contains(t, line, "thinking")
	assert.Contains(t, line, "2 agents")
}

func TestRenderDefault_ShowsReasoningEffort(t *testing.T) {
	line := RenderDefault(Snapshot{
		Provider: ProviderInfo{DisplayName: "Codex"},
		Model: ModelInfo{
			ID:              "gpt-5.6-sol",
			ReasoningEffort: "high",
		},
	})
	assert.Contains(t, line, "● high · /effort")
}
