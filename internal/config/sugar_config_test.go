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
[default]
provider = "sugar"
model = "sugar/conduit"

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
	t.Setenv("PACKETCODE_SUGAR_ENABLED", "true")
	t.Setenv("PACKETCODE_SUGAR_PRIVACY", "maybe")
	_, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml"))
	require.ErrorContains(t, err, "sugar.privacy")
}

func TestAutomaticSugarActivationPreservesExistingConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[providers.sugar]
api_key = "existing"
default_model = "sugar/conduit"
`), 0o600))

	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	assert.True(t, cfg.SugarIsEnabled())
	assert.False(t, cfg.SugarExplicitlyDisabled())
}

func TestAutomaticInactiveSugarSkipsSubordinateEnvironment(t *testing.T) {
	t.Setenv("PACKETCODE_SUGAR_PRIVACY", "invalid-while-inactive")
	t.Setenv("PACKETCODE_CONDUIT_SHADOW", "invalid-while-inactive")

	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml"))
	require.NoError(t, err)
	assert.False(t, cfg.SugarIsEnabled())
	assert.False(t, cfg.Conduit.ShadowEnabled)
}

func TestInactiveSugarPreservesStoredConduitPreference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[conduit]
shadow_enabled = true
`), 0o600))

	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	assert.False(t, cfg.SugarIsEnabled())
	assert.True(t, cfg.Conduit.ShadowEnabled, "effective inactivity must not rewrite the stored child setting")
	assert.False(t, cfg.ConduitIsEnabled())
}

func TestExplicitDisableAllowsCustomSugarProviderWithoutActivatingBuiltIn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[sugar]
enabled = false

[providers.sugar]
type = "openai_compatible"
base_url = "http://localhost:8080/v1"
default_model = "custom-model"
`), 0o600))

	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	assert.False(t, cfg.SugarIsEnabled())
	assert.True(t, cfg.SugarUsesCustomProvider())
	assert.False(t, cfg.Conduit.ShadowEnabled)
}

func TestOptionalIntegrationsResolveSafeDefaults(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml"))
	require.NoError(t, err)
	assert.True(t, cfg.PacketComputers.IsEnabled())
	assert.True(t, cfg.ACP.IsEnabled())
	assert.False(t, cfg.SugarIsEnabled(), "fresh installs leave Sugar inactive")
	assert.False(t, cfg.SugarExplicitlyDisabled(), "automatic inactivity still allows explicit login")
	assert.False(t, cfg.Conduit.ShadowEnabled)
}

func TestDisabledSugarMakesSubordinateSettingsInert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[sugar]
enabled = false
cache_mode = "invalid-while-disabled"
cache_retention = "invalid-while-disabled"
privacy = "invalid-while-disabled"

[conduit]
shadow_enabled = true
timeout_ms = 1
capsule_max_bytes = 1
`), 0o600))
	t.Setenv("PACKETCODE_CONDUIT_SHADOW", "not-a-boolean")

	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	assert.False(t, cfg.SugarIsEnabled())
	assert.True(t, cfg.SugarExplicitlyDisabled())
	assert.True(t, cfg.Conduit.ShadowEnabled, "the parent gate suppresses execution without erasing the stored preference")
	assert.False(t, cfg.ConduitIsEnabled())
}

func TestIntegrationEnvironmentOverridesConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[packet_computers]
enabled = true

[acp]
enabled = true

[sugar]
enabled = true
`), 0o600))
	t.Setenv("PACKETCODE_PACKET_COMPUTERS_ENABLED", "false")
	t.Setenv("PACKETCODE_ACP_ENABLED", "false")
	t.Setenv("PACKETCODE_SUGAR_ENABLED", "false")

	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	assert.False(t, cfg.PacketComputers.IsEnabled())
	assert.False(t, cfg.ACP.IsEnabled())
	assert.False(t, cfg.SugarIsEnabled())
	assert.True(t, cfg.SugarExplicitlyDisabled())
}

func TestIntegrationEnvironmentOverridesAreNotPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[packet_computers]
enabled = true

[acp]
enabled = true

[sugar]
enabled = true
`), 0o600))
	t.Setenv("PACKETCODE_PACKET_COMPUTERS_ENABLED", "false")
	t.Setenv("PACKETCODE_ACP_ENABLED", "false")
	t.Setenv("PACKETCODE_SUGAR_ENABLED", "false")

	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	assert.False(t, cfg.PacketComputers.IsEnabled())
	assert.False(t, cfg.ACP.IsEnabled())
	assert.False(t, cfg.SugarIsEnabled())

	saved := filepath.Join(t.TempDir(), "saved.toml")
	require.NoError(t, cfg.SaveTo(saved))
	t.Setenv("PACKETCODE_PACKET_COMPUTERS_ENABLED", "")
	t.Setenv("PACKETCODE_ACP_ENABLED", "")
	t.Setenv("PACKETCODE_SUGAR_ENABLED", "")

	reloaded, err := LoadFrom(saved)
	require.NoError(t, err)
	assert.True(t, reloaded.PacketComputers.IsEnabled())
	assert.True(t, reloaded.ACP.IsEnabled())
	assert.True(t, reloaded.SugarIsEnabled())
}

func TestIntegrationEnvironmentRejectsInvalidBooleans(t *testing.T) {
	t.Run("acp", func(t *testing.T) {
		t.Setenv("PACKETCODE_ACP_ENABLED", "sometimes")
		_, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml"))
		require.ErrorContains(t, err, "PACKETCODE_ACP_ENABLED must be true or false")
	})
	t.Run("packet computers", func(t *testing.T) {
		t.Setenv("PACKETCODE_PACKET_COMPUTERS_ENABLED", "sometimes")
		_, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml"))
		require.ErrorContains(t, err, "PACKETCODE_PACKET_COMPUTERS_ENABLED must be true or false")
	})
	t.Run("sugar", func(t *testing.T) {
		t.Setenv("PACKETCODE_SUGAR_ENABLED", "sometimes")
		_, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml"))
		require.ErrorContains(t, err, "PACKETCODE_SUGAR_ENABLED must be true or false")
	})
}
