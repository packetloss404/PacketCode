package hooks

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/config"
)

func TestRunUserPromptSubmit_CollectsStdout(t *testing.T) {
	command := "input=$(cat); case \"$input\" in *hello*) printf injected-context;; *) exit 1;; esac"
	if runtime.GOOS == "windows" {
		command = "$data = $input | Out-String; if ($data -match 'hello') { 'injected-context' } else { exit 1 }"
	}
	r := New(config.HooksConfig{
		UserPromptSubmit: []config.HookConfig{{Command: command, TimeoutSec: 2}},
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
	r := New(config.HooksConfig{
		PreToolUse: []config.HookConfig{{Matcher: "execute_command", Command: command, TimeoutSec: 2}},
	}, t.TempDir())

	_, err := r.RunPreToolUse(context.Background(), ToolPayload{ToolName: "execute_command"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")

	_, err = r.RunPreToolUse(context.Background(), ToolPayload{ToolName: "read_file"})
	require.NoError(t, err)
}
