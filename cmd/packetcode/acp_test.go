package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/acp"
	"github.com/packetcode/packetcode/internal/config"
)

func TestServerModelCatalogEnumeratesConfiguredProviders(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {DefaultModel: "claude-fable-5"},
		"ollama":    {DefaultModel: "qwen3:14b"},
		"unknownco": {DefaultModel: "mystery"}, // no factory: excluded
		"acme": {
			Type:         "openai_compatible",
			BaseURL:      "https://models.acme.test/v1",
			DefaultModel: "acme-large",
			Models: []config.ProviderModelConfig{
				{ID: "acme-large"}, // duplicate of the default: deduplicated
				{ID: "acme-mini"},
			},
		},
	}
	catalog := &packetModelCatalog{cfg: cfg, activeProvider: "anthropic", activeModel: "claude-fable-5"}

	models, err := catalog.ListModels()
	require.NoError(t, err)

	expected := []acp.ModelOption{
		{Provider: "anthropic", Model: "claude-fable-5", Default: true},
		{Provider: "acme", Model: "acme-large"},
		{Provider: "acme", Model: "acme-mini"},
		{Provider: "ollama", Model: "qwen3:14b"},
	}
	assert.Equal(t, expected, models)
}

func TestServerModelCatalogIncludesCLIOverrideAbsentFromConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"anthropic": {DefaultModel: "claude-fable-5"},
	}
	catalog := &packetModelCatalog{cfg: cfg, activeProvider: "openai", activeModel: "gpt-5.3-mini"}

	models, err := catalog.ListModels()
	require.NoError(t, err)

	expected := []acp.ModelOption{
		{Provider: "openai", Model: "gpt-5.3-mini", Default: true},
		{Provider: "anthropic", Model: "claude-fable-5"},
	}
	assert.Equal(t, expected, models)
}
