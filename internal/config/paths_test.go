package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestThemePath_UnderHomeDir pins the returned theme path to
// `<home>/.packetcode/theme.toml`. `t.Setenv` on both HOME and
// USERPROFILE keeps the test cross-platform (Windows prefers
// USERPROFILE; Unix prefers HOME).
func TestThemePath_UnderHomeDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	got, err := ThemePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".packetcode", "theme.toml"), got)
}
