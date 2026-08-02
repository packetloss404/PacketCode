package app

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSSHComputerArgs(t *testing.T) {
	computer, err := parseSSHComputerArgs([]string{
		"production", "deploy@[2001:db8::10]", "/srv/apps/widget",
		"--fingerprint", "SHA256:abc123", "--port", "2222", "--identity", `C:\keys\deploy`,
	})
	require.NoError(t, err)
	assert.Equal(t, "production", computer.Name)
	assert.Equal(t, "deploy", computer.SSHUser)
	assert.Equal(t, "2001:db8::10", computer.SSHHost)
	assert.Equal(t, 2222, computer.SSHPort)
	assert.Equal(t, "/srv/apps/widget", computer.ProjectRoots[0])
	assert.Equal(t, "SHA256:abc123", computer.SSHHostFingerprint)
	assert.Equal(t, `C:\keys\deploy`, computer.SSHIdentityFile)
}

func TestParseSSHComputerArgsRequiresPinnedHostAndAbsoluteRoot(t *testing.T) {
	_, err := parseSSHComputerArgs([]string{"prod", "deploy@host", "relative", "--fingerprint", "SHA256:key"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute POSIX path")

	_, err = parseSSHComputerArgs([]string{"prod", "deploy@host", "/srv/app", "--port", "22"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--fingerprint must be")
}

func TestComputersCommandsRegisterDuplicateAndConfirmedRemove(t *testing.T) {
	rig := newTestApp(t)
	t.Setenv("PACKETCODE_HOME", filepath.Join(rig.tmp, "packetcode-home"))
	args := []string{
		"ssh", "production", "deploy@example.com", "/srv/app",
		"--fingerprint", "SHA256:approved-key",
	}
	rig.app.handleComputersCommand(args)
	registry, err := loadComputerRegistry()
	require.NoError(t, err)
	_, ok := registry.Get("production")
	require.True(t, ok)

	rig.app.handleComputersCommand(args)
	convContains(t, rig.app, `name "production" is already registered`)

	rig.app.handleComputersCommand([]string{"remove", "production"})
	registry, err = loadComputerRegistry()
	require.NoError(t, err)
	_, ok = registry.Get("production")
	require.True(t, ok, "remove without --yes must preserve the record")

	rig.app.handleComputersCommand([]string{"remove", "production", "--yes"})
	registry, err = loadComputerRegistry()
	require.NoError(t, err)
	_, ok = registry.Get("production")
	assert.False(t, ok)
}
