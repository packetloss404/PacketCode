package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/acp"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
)

func TestServerUsageReaderReadsPersistedSession(t *testing.T) {
	dir := t.TempDir()
	manager := session.NewManager(dir)
	created, err := manager.New("anthropic", "claude-fable-5")
	require.NoError(t, err)
	// Two stream completions: totals accumulate, context tracks the last one.
	require.NoError(t, manager.UpdateUsage(provider.Usage{InputTokens: 1000, OutputTokens: 200}, 3, 15))
	require.NoError(t, manager.UpdateUsage(provider.Usage{InputTokens: 1500, OutputTokens: 300}, 3, 15))

	reader := &packetUsageReader{dir: dir}
	usage, err := reader.ReadUsage(created.ID)
	require.NoError(t, err)
	assert.Equal(t, acp.SessionUsage{
		ContextTokens: 1800, // last request's prompt + completion
		TotalInput:    2500,
		TotalOutput:   500,
		CostUSD:       2500*3.0/1_000_000 + 500*15.0/1_000_000,
	}, usage)

	_, err = reader.ReadUsage("no-such-session")
	require.Error(t, err)
}

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

func TestPacketACPFactoryProviderOverrides(t *testing.T) {
	cfg := config.Default()
	cfg.Providers = map[string]config.ProviderConfig{
		"ollama": {}, // present but no default_model
	}
	factory := &packetACPFactory{
		cfg: cfg, provider: "anthropic", model: "claude-fable-5",
		sessionsDir: t.TempDir(), backupsDir: t.TempDir(),
	}

	// Unknown provider override must surface the errors.Is-matchable sentinel
	// so the ACP server answers -32602 instead of a generic internal error.
	_, err := factory.NewSession(t.Context(), acp.SessionConfig{
		CWD: t.TempDir(), Provider: "definitely-unknown",
	}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, acp.ErrUnknownProvider)

	// A provider override without a model consults that provider's
	// default_model; when it has none, session creation fails loudly rather
	// than silently reusing the previous provider's model.
	_, err = factory.NewSession(t.Context(), acp.SessionConfig{
		CWD: t.TempDir(), Provider: "ollama",
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no model is configured for provider "ollama"`)
}
