package config

import (
	"os"
	"path/filepath"
	"strings"
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
	assert.Equal(t, "off", cfg.Sugar.EffectiveCacheMode())
	assert.Equal(t, "1h", cfg.Sugar.EffectiveCacheRetention())
	assert.Equal(t, "zdr_required", cfg.Sugar.EffectivePrivacy())
	assert.True(t, cfg.Conduit.ShadowIsEnabled())
	// The stored fields keep what the file said: an environment override is
	// in force for this process, not a setting to be written back.
	assert.Equal(t, "auto", cfg.Sugar.CacheMode)
	assert.False(t, cfg.Conduit.ShadowEnabled)
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
	// Only the variables Sugar actually owns are skipped while it is
	// inactive. Conduit is no longer subordinate, so its own variable is
	// covered by TestConduitShadowEnvIsValidatedEvenWhileSugarIsOff below.
	t.Setenv("PACKETCODE_SUGAR_PRIVACY", "invalid-while-inactive")

	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml"))
	require.NoError(t, err)
	assert.False(t, cfg.SugarIsEnabled())
	assert.False(t, cfg.Conduit.ShadowEnabled)
}

// A malformed Conduit variable is reported whether or not Sugar is on. It used
// to be skipped unparsed behind the Sugar gate, so a typo silently did nothing.
func TestConduitShadowEnvIsValidatedEvenWhileSugarIsOff(t *testing.T) {
	t.Setenv("PACKETCODE_CONDUIT_SHADOW", "invalid-while-inactive")

	_, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PACKETCODE_CONDUIT_SHADOW")
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
	assert.True(t, cfg.Conduit.ShadowEnabled, "effective inactivity must not rewrite the stored setting")
	// Decoupled: the stored preference is now honoured rather than suppressed.
	// Nothing is lost by that, because conduitShadowState.start still refuses
	// to act unless the live provider is Sugar.
	assert.True(t, cfg.ConduitIsEnabled())
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
`), 0o600))

	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	assert.False(t, cfg.SugarIsEnabled())
	assert.True(t, cfg.SugarExplicitlyDisabled())
	// Sugar's own settings stay inert, but Conduit is independent now, so its
	// stored preference is honoured rather than suppressed.
	assert.True(t, cfg.Conduit.ShadowEnabled)
	assert.True(t, cfg.ConduitIsEnabled())
}

// Conduit bounds are validated whether or not Sugar is on. They used to sit
// behind the Sugar gate, so an out-of-range value lay dormant until somebody
// enabled Sugar and only then failed to load.
func TestConduitBoundsAreValidatedWhileSugarIsDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[sugar]
enabled = false

[conduit]
timeout_ms = 1
`), 0o600))

	_, err := LoadFrom(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conduit.timeout_ms")
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

// Conduit is gated on its own. The config-level dependency on Sugar it used to
// carry was redundant with conduitShadowState.start, which refuses to act
// unless the live provider is Sugar -- so the coupling protected nothing and
// only discarded a setting the user had explicitly made.
func TestConduitShadowGateIsIndependentOfSugar(t *testing.T) {
	cfg := Default()
	disabled := false
	cfg.Sugar.Enabled = &disabled
	cfg.Conduit.ShadowEnabled = true

	if !cfg.ConduitIsEnabled() {
		t.Fatal("conduit shadow must stay enabled when Sugar is off")
	}
	if cfg.SugarIsEnabled() {
		t.Fatal("precondition: Sugar should be disabled in this case")
	}

	cfg.Conduit.ShadowEnabled = false
	if cfg.ConduitIsEnabled() {
		t.Fatal("conduit shadow must be off when its own setting is off")
	}
}

// The Conduit variable must be honoured whether or not Sugar is on; it used to
// sit behind an early return that skipped it entirely.
func TestConduitShadowEnvAppliesWhileSugarIsOff(t *testing.T) {
	t.Setenv("PACKETCODE_SUGAR_ENABLED", "false")
	t.Setenv("PACKETCODE_CONDUIT_SHADOW", "true")

	cfg := Default()
	if err := applyIntegrationEnvironment(cfg); err != nil {
		t.Fatalf("applyIntegrationEnvironment: %v", err)
	}
	if cfg.SugarIsEnabled() {
		t.Fatal("Sugar should be disabled by the environment")
	}
	if !cfg.Conduit.ShadowIsEnabled() {
		t.Fatal("PACKETCODE_CONDUIT_SHADOW must apply even while Sugar is off")
	}
}

// An environment override exported for one experiment must not become the
// stored setting the next time something saves the config. /provider,
// /effort, and the API-key picker all call Save(); before this test, one of
// them was enough to make PACKETCODE_CONDUIT_SHADOW=true permanent.
func TestSaveDoesNotPersistEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[sugar]
enabled = true
cache_mode = "auto"
privacy = "standard"

[conduit]
shadow_enabled = false
`), 0o600))
	t.Setenv("PACKETCODE_SUGAR_CACHE_MODE", "off")
	t.Setenv("PACKETCODE_CONDUIT_SHADOW", "true")

	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	require.Equal(t, "off", cfg.Sugar.EffectiveCacheMode())
	require.True(t, cfg.Conduit.ShadowIsEnabled())
	require.NoError(t, cfg.SaveTo(path))

	t.Setenv("PACKETCODE_SUGAR_CACHE_MODE", "")
	os.Unsetenv("PACKETCODE_SUGAR_CACHE_MODE")
	os.Unsetenv("PACKETCODE_CONDUIT_SHADOW")
	reloaded, err := LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, "auto", reloaded.Sugar.EffectiveCacheMode(), "cache_mode override leaked into config.toml")
	assert.False(t, reloaded.Conduit.ShadowIsEnabled(), "shadow override leaked into config.toml")
}

// An exported-but-empty variable is how a shell spells "not set". Failing
// startup over a blank line in someone's profile would be worse than useless,
// but a misspelled value must still be loud: silently reading it as "off" is
// the direction that quietly disables a feature the user asked for.
func TestLookupBoolEnv_DistinguishesUnsetEmptyAndInvalid(t *testing.T) {
	const name = "PACKETCODE_TEST_GATE"

	got, err := lookupBoolEnv(name)
	if err != nil || got != nil {
		t.Fatalf("unset: got (%v, %v), want (nil, nil)", got, err)
	}

	for _, blank := range []string{"", "   ", "\t"} {
		t.Setenv(name, blank)
		got, err = lookupBoolEnv(name)
		if err != nil || got != nil {
			t.Fatalf("blank %q: got (%v, %v), want (nil, nil)", blank, got, err)
		}
	}

	t.Setenv(name, " true ")
	got, err = lookupBoolEnv(name)
	if err != nil || got == nil || !*got {
		t.Fatalf("surrounding space: got (%v, %v), want (true, nil)", got, err)
	}

	t.Setenv(name, "yes-please")
	if _, err = lookupBoolEnv(name); err == nil {
		t.Fatal("an unparseable value must be an error, never a silent false")
	} else if !strings.Contains(err.Error(), name) {
		t.Fatalf("the error must name the variable, got %q", err)
	}
}
