package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/config"
)

func TestACPDisabledFailsBeforeStartingProtocol(t *testing.T) {
	home := t.TempDir()
	t.Setenv(config.HomeEnv, home)
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.toml"), []byte("[acp]\nenabled = false\n"), 0o600))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runACPCommand(nil, strings.NewReader(""), &stdout, &stderr)

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout.String(), "disabled ACP must not emit protocol output")
	assert.Contains(t, stderr.String(), "ACP integration is disabled")
}
