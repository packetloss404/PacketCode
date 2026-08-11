package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSugarAndConduitConfigEnvironmentPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[sugar]
cache_mode = "auto"
cache_retention = "5m"
privacy = "standard"

[conduit]
shadow_enabled = false
timeout_ms = 900
capsule_max_bytes = 4096
`), 0o600))
	t.Setenv("PACKETCODE_SUGAR_CACHE_MODE", "off")
	t.Setenv("PACKETCODE_SUGAR_CACHE_RETENTION", "1h")
	t.Setenv("PACKETCODE_SUGAR_PRIVACY", "zdr_required")
	t.Setenv("PACKETCODE_CONDUIT_SHADOW", "true")

	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, "off", cfg.Sugar.CacheMode)
	assert.Equal(t, "1h", cfg.Sugar.CacheRetention)
	assert.Equal(t, "zdr_required", cfg.Sugar.Privacy)
	assert.True(t, cfg.Conduit.ShadowEnabled)
	assert.Equal(t, 900, cfg.Conduit.TimeoutMS)
	assert.Equal(t, 4096, cfg.Conduit.CapsuleMaxBytes)
}

func TestSugarAndConduitConfigRejectsInvalidEnvironment(t *testing.T) {
	t.Setenv("PACKETCODE_SUGAR_PRIVACY", "maybe")
	_, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml"))
	require.ErrorContains(t, err, "sugar.privacy")
}
