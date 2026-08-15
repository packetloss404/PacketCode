package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/acp"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/session"
)

func TestServerSessionRenamerPersistsSanitizedName(t *testing.T) {
	dir := t.TempDir()
	created, err := session.NewManager(dir).New("anthropic", "claude-fable-5")
	require.NoError(t, err)

	renamer := &packetSessionRenamer{dir: dir}
	require.NoError(t, renamer.RenameSession(created.ID, "Fix The Login Bug"))

	// Read back through a fresh manager: the rename must be on disk, run
	// through sanitizeName (lowercase, hyphenated).
	summaries, err := session.NewManager(dir).List()
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "fix-the-login-bug", summaries[0].Name)

	// Unknown sessions surface the load error to the ACP layer (-32603).
	require.Error(t, renamer.RenameSession("does-not-exist", "anything"))
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

func TestPacketACPFactoryPermissionModeOverrides(t *testing.T) {
	cfg := config.Default()
	serverPolicy, err := permissions.FromConfig(cfg)
	require.NoError(t, err)
	factory := &packetACPFactory{
		cfg: cfg, provider: "anthropic", model: "claude-fable-5", policy: serverPolicy,
		sessionsDir: t.TempDir(), backupsDir: t.TempDir(),
	}

	// Unknown mode override must surface the errors.Is-matchable sentinel so
	// the ACP server answers -32602 instead of a generic internal error, and it
	// must fail before any session state is created.
	_, err = factory.NewSession(t.Context(), acp.SessionConfig{
		CWD: t.TempDir(), PermissionMode: "yolo",
	}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, acp.ErrUnknownPermissionMode)

	// An empty mode keeps the server-wide policy instance untouched.
	policy, err := factory.sessionPolicy("")
	require.NoError(t, err)
	assert.Same(t, serverPolicy, policy)

	// Each advertised wire mode builds a per-session policy on the matching
	// profile.
	expected := map[string]permissions.Profile{
		"ask":          permissions.ProfileAsk,
		"accept-edits": permissions.ProfileEdit,
		"auto":         permissions.ProfileAuto,
		"read-only":    permissions.ProfileSafe,
		"bypass":       permissions.ProfileFull,
	}
	for mode, profile := range expected {
		policy, err := factory.sessionPolicy(mode)
		require.NoError(t, err, "mode %s", mode)
		assert.Equal(t, profile, policy.Profile(), "mode %s", mode)
		assert.NotSame(t, serverPolicy, policy, "mode %s must build its own policy", mode)
	}

	// The per-session policy wins over an inline default rule and trust mode,
	// matching the --permission-mode CLI override semantics.
	cfgLoose := config.Default()
	cfgLoose.Permissions.Default = "allow"
	cfgLoose.Behavior.TrustMode = true
	factoryLoose := &packetACPFactory{
		cfg: cfgLoose, provider: "anthropic", model: "claude-fable-5",
		sessionsDir: t.TempDir(), backupsDir: t.TempDir(),
	}
	strict, err := factoryLoose.sessionPolicy("read-only")
	require.NoError(t, err)
	assert.Equal(t, permissions.ProfileSafe, strict.Profile())
	for _, rule := range strict.Rules() {
		assert.NotEqual(t, "inline default", rule.Reason, "inline default rule must be cleared for per-session modes")
	}
}
