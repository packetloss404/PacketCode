package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/provider"
)

func TestProviderFactoriesFromConfig_AddsCustomOpenAICompatibleProvider(t *testing.T) {
	keyless := false
	cfg := config.Default()
	cfg.Providers["localai"] = config.ProviderConfig{
		Type:           "openai_compatible",
		BaseURL:        "http://localhost:8080/v1",
		DisplayName:    "LocalAI",
		APIKeyRequired: &keyless,
		DefaultModel:   "local-model",
	}

	factories := providerFactoriesFromConfig(cfg)
	factory, ok := factories["localai"]
	require.True(t, ok)

	prov := factory("")
	assert.Equal(t, "localai", prov.Slug())
	assert.Equal(t, "LocalAI", prov.Name())
	assert.True(t, providerRequiresAPIKey(cfg, "openai"))
	assert.False(t, providerRequiresAPIKey(cfg, "localai"))
}

func TestProviderFactoriesFromConfig_SkipsNonCustomUnknownProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["typo"] = config.ProviderConfig{DefaultModel: "m"}

	factories := providerFactoriesFromConfig(cfg)
	_, ok := factories["typo"]
	assert.False(t, ok)
}

func TestProviderFactoriesFromConfig_SugarFollowsTriStateActivation(t *testing.T) {
	t.Run("fresh install is inactive", func(t *testing.T) {
		_, ok := providerFactoriesFromConfig(config.Default())["sugar"]
		assert.False(t, ok)
	})
	t.Run("existing Sugar config remains active", func(t *testing.T) {
		cfg := config.Default()
		cfg.Providers["sugar"] = config.ProviderConfig{DefaultModel: "sugar/conduit"}
		_, ok := providerFactoriesFromConfig(cfg)["sugar"]
		assert.True(t, ok)
	})
	t.Run("explicit false wins over existing config", func(t *testing.T) {
		cfg := config.Default()
		cfg.Providers["sugar"] = config.ProviderConfig{DefaultModel: "sugar/conduit"}
		disabled := false
		cfg.Sugar.Enabled = &disabled
		_, ok := providerFactoriesFromConfig(cfg)["sugar"]
		assert.False(t, ok)
	})
	t.Run("explicitly disabled built-in permits a custom Sugar slug", func(t *testing.T) {
		cfg := config.Default()
		disabled := false
		keyless := false
		cfg.Sugar.Enabled = &disabled
		cfg.Providers["sugar"] = config.ProviderConfig{
			Type: "openai_compatible", BaseURL: "http://localhost:8080/v1",
			DefaultModel: "custom-model", APIKeyRequired: &keyless,
		}
		factory, ok := providerFactoriesFromConfig(cfg)["sugar"]
		require.True(t, ok)
		assert.False(t, providerRequiresAPIKey(cfg, "sugar"))
		assert.Equal(t, "sugar", factory("").Slug())
	})
}

func TestProviderFactoriesFromConfig_DoesNotOverrideBuiltInSlug(t *testing.T) {
	keyRequired := false
	cfg := config.Default()
	cfg.Providers["openai"] = config.ProviderConfig{
		Type:           "openai_compatible",
		BaseURL:        "http://localhost:8080/v1",
		APIKeyRequired: &keyRequired,
	}

	factories := providerFactoriesFromConfig(cfg)
	prov := factories["openai"]("sk-test")

	assert.Equal(t, "openai", prov.Slug())
	assert.Equal(t, "OpenAI", prov.Name())
	assert.True(t, providerRequiresAPIKey(cfg, "openai"))
}

func TestProviderFactoriesFromConfig_AppliesCodexReasoningEffort(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["codex"] = config.ProviderConfig{
		DefaultModel:    "gpt-5.6-sol",
		ReasoningEffort: "high",
	}

	prov := providerFactoriesFromConfig(cfg)["codex"]("")
	controller, ok := prov.(provider.ReasoningEffortController)
	require.True(t, ok)
	assert.Equal(t, "high", controller.ReasoningEffort("gpt-5.6-sol"))
}
