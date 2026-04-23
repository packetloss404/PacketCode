package statusline

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/config"
)

func TestRunner_RenderPassesJSONOnStdin(t *testing.T) {
	command := "read input; case \"$input\" in *gpt-test*) printf custom-status;; *) exit 1;; esac"
	if runtime.GOOS == "windows" {
		command = "$data = $input | Out-String; if ($data -match 'gpt-test') { 'custom-status' } else { exit 1 }"
	}
	r := New(config.StatusLineConfig{Command: command, TimeoutSec: 2}, t.TempDir())
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
