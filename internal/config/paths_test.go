package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveHomeDir_DefaultAndAbsoluteOverride(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	t.Setenv(HomeEnv, "")

	got, err := ResolveHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(userHome, ".packetcode"), got)
	assert.Equal(t, "default", HomeSource())

	override := filepath.Join(t.TempDir(), "PacketCode State")
	t.Setenv(HomeEnv, override)
	got, err = ResolveHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(override), got)
	assert.Equal(t, "environment", HomeSource())
}

func TestResolveHomeDir_RejectsRelativeOverrideWithoutTouchingDefault(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	t.Setenv(HomeEnv, filepath.Join("relative", "packetcode"))

	_, err := HomeDir()
	require.ErrorContains(t, err, "must be an absolute path")
	_, statErr := os.Stat(filepath.Join(userHome, ".packetcode"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHomeDir_OverrideIsolatedFromLegacyDefault(t *testing.T) {
	userHome := t.TempDir()
	legacy := filepath.Join(userHome, ".packetcode")
	require.NoError(t, os.MkdirAll(legacy, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "config.toml"), []byte("legacy"), 0o600))

	override := filepath.Join(t.TempDir(), "isolated")
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	t.Setenv(HomeEnv, override)

	got, err := HomeDir()
	require.NoError(t, err)
	assert.Equal(t, override, got)
	_, err = os.Stat(filepath.Join(override, "config.toml"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	contents, err := os.ReadFile(filepath.Join(legacy, "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, "legacy", string(contents))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(override)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

// TestThemePath_UnderHomeDir pins the returned theme path to
// `<home>/.packetcode/theme.toml`. `t.Setenv` on both HOME and
// USERPROFILE keeps the test cross-platform (Windows prefers
// USERPROFILE; Unix prefers HOME).
// TestMCPLogPath asserts the log path resolves under the packetcode
// home directory and that the home directory is created on demand.
func TestMCPLogPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	got, err := MCPLogPath("git")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".packetcode", "mcp-git.log"), got)
}

func TestMCPLogPath_RejectsUnsafeName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	for _, name := range []string{"../evil", `foo\\bar`, "", "name.with.dot"} {
		_, err := MCPLogPath(name)
		require.Error(t, err, "name %q", name)
	}
}

func TestThemePath_UnderHomeDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	got, err := ThemePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".packetcode", "theme.toml"), got)
}

func TestCommandsDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	user, err := UserCommandsDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".packetcode", "commands"), user)

	project := ProjectCommandsDir(filepath.Join(dir, "work"))
	assert.Equal(t, filepath.Join(dir, "work", ".packetcode", "commands"), project)
}
