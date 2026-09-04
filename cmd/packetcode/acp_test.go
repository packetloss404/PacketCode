package main

import (
	"bytes"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/acp"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
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

func TestPacketACPFactoryPermissionCeiling(t *testing.T) {
	cfg := config.Default()
	// Operator explicitly restricted the server: clients may narrow but
	// never escalate past it.
	cfg.Permissions.Profile = "read-only"
	factory := &packetACPFactory{
		cfg: cfg, provider: "anthropic", model: "claude-fable-5",
		ceiling:     serverPermissionCeiling(cfg),
		sessionsDir: t.TempDir(), backupsDir: t.TempDir(),
	}

	_, err := factory.NewSession(t.Context(), acp.SessionConfig{
		CWD: t.TempDir(), PermissionMode: "bypass",
	}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, acp.ErrPermissionModeDenied)

	// Matching the ceiling is allowed: policy resolution succeeds and the
	// factory proceeds past permission handling (it fails later on provider
	// construction, not on the mode).
	_, err = factory.NewSession(t.Context(), acp.SessionConfig{
		CWD: t.TempDir(), PermissionMode: "read-only",
	}, nil)
	if err != nil {
		assert.NotErrorIs(t, err, acp.ErrPermissionModeDenied)
		assert.NotErrorIs(t, err, acp.ErrUnknownPermissionMode)
	}

	assert.Equal(t, []string{"read-only"}, allowedPermissionModes(serverPermissionCeiling(cfg)))
}

func TestServerPermissionCeilingDefaults(t *testing.T) {
	// Default config: the local user is the operator; no restriction.
	cfg := config.Default()
	cfg.Permissions.Profile = ""
	assert.Equal(t, permissions.ProfileFull, serverPermissionCeiling(cfg))
	assert.Equal(t, acp.PermissionModes, allowedPermissionModes(permissions.ProfileFull))

	// Explicit ask profile caps at ask.
	cfg.Permissions.Profile = "ask"
	assert.Equal(t, permissions.ProfileAsk, serverPermissionCeiling(cfg))
	assert.Equal(t, []string{"ask", "read-only"}, allowedPermissionModes(permissions.ProfileAsk))

	// Trust mode is already maximal.
	cfg.Behavior.TrustMode = true
	assert.Equal(t, permissions.ProfileFull, serverPermissionCeiling(cfg))

	// Custom profiles have unknown shape: unrestricted.
	cfg.Behavior.TrustMode = false
	cfg.Permissions.Profile = "my-custom-profile"
	assert.Equal(t, permissions.ProfileFull, serverPermissionCeiling(cfg))
}

// TestPacketACPFactoryMCPServersFallback pins the absent-vs-empty decision at
// the factory boundary: a client that says nothing about MCP inherits the
// operator's [mcp.<name>] blocks (the TUI's fleet), while a client that sends
// an explicit list — empty or not — gets exactly that list.
func TestPacketACPFactoryMCPServersFallback(t *testing.T) {
	cfg := config.Default()
	cfg.MCP = map[string]config.MCPServerConfig{
		"github": {Command: "gh-mcp", Args: []string{"serve"}},
		"fs":     {Command: "fs-mcp"},
	}

	// Absent: the agent's own configuration, alphabetically ordered like the
	// TUI's fleet, and NOT flagged as a client choice.
	servers, clientSupplied := mcpServersForSession(acp.SessionConfig{}, cfg)
	assert.False(t, clientSupplied)
	require.Len(t, servers, 2)
	assert.Equal(t, "fs", servers[0].Name)
	assert.Equal(t, "github", servers[1].Name)
	assert.Equal(t, []string{"serve"}, servers[1].Args)

	// Explicit empty: the client owns the fleet and asked for none. The
	// operator's config must NOT leak back in.
	servers, clientSupplied = mcpServersForSession(
		acp.SessionConfig{MCPServersSet: true, MCPServers: []acp.MCPServer{}}, cfg)
	assert.True(t, clientSupplied)
	assert.Empty(t, servers)

	// Explicit non-empty: exactly the client's list wins over the config.
	servers, clientSupplied = mcpServersForSession(acp.SessionConfig{
		MCPServersSet: true,
		MCPServers: []acp.MCPServer{
			{Name: "client-only", Command: "/opt/tool", Args: []string{"-x"}, Env: map[string]string{"K": "v"}},
		},
	}, cfg)
	assert.True(t, clientSupplied)
	require.Len(t, servers, 1)
	assert.Equal(t, "client-only", servers[0].Name)
	assert.Equal(t, "/opt/tool", servers[0].Command)
	assert.True(t, servers[0].Enabled)

	// A disabled [mcp.<name>] block still reaches the manager, which reports
	// it "disabled" rather than starting it.
	disabled := false
	cfg.MCP = map[string]config.MCPServerConfig{"off": {Command: "x", Enabled: &disabled}}
	servers, _ = mcpServersForSession(acp.SessionConfig{}, cfg)
	require.Len(t, servers, 1)
	assert.False(t, servers[0].Enabled)
}

// TestPacketACPFactoryNewSessionMCPFailureSemantics pins the deliberate split
// at the public factory boundary. NewSession now delegates MCP construction to
// packetRuntime, so testing the old startMCP helper alone would let the two
// paths drift without catching the regression.
func TestPacketACPFactoryNewSessionMCPFailureSemantics(t *testing.T) {
	broken := filepath.Join(t.TempDir(), "definitely-not-an-executable")
	newConfig := func() *config.Config {
		cfg := config.Default()
		cfg.Default.Provider = "ollama"
		cfg.Default.Model = "test-model"
		cfg.Providers["ollama"] = config.ProviderConfig{DefaultModel: "test-model"}
		return cfg
	}
	newFactory := func(cfg *config.Config, log io.Writer) *packetACPFactory {
		return &packetACPFactory{
			cfg: cfg, provider: "ollama", model: "test-model", log: log,
			sessionsDir: t.TempDir(), backupsDir: t.TempDir(),
		}
	}

	// Agent defaults: log and continue, so one bad config block cannot make
	// every ACP session uncreatable. The failed status must survive the shared
	// runtime mapping and its Close function must be safe to call repeatedly.
	cfg := newConfig()
	cfg.MCP = map[string]config.MCPServerConfig{"broken": {Command: broken, TimeoutSec: 2}}
	var logs bytes.Buffer
	factory := newFactory(cfg, &logs)
	degraded, err := factory.NewSession(t.Context(), acp.SessionConfig{CWD: t.TempDir()}, nil)
	require.NoError(t, err)
	require.NotNil(t, degraded)
	require.NotNil(t, degraded.Close)
	require.Len(t, degraded.MCPServers, 1)
	assert.Equal(t, "broken", degraded.MCPServers[0].Name)
	assert.Equal(t, "failed", degraded.MCPServers[0].Status)
	assert.Equal(t, "agent", degraded.MCPServers[0].Source)
	assert.NotEmpty(t, degraded.MCPServers[0].Error)
	assert.Contains(t, logs.String(), "mcp broken: failed")
	require.NoError(t, degraded.Close())
	require.NoError(t, degraded.Close(), "runtime cleanup must be idempotent")

	// Client-requested: the client named this server for this session, so
	// silently dropping it would hand the agent a fleet it did not ask for.
	factory = newFactory(newConfig(), io.Discard)
	rejected, err := factory.NewSession(t.Context(), acp.SessionConfig{
		CWD:           t.TempDir(),
		MCPServersSet: true,
		MCPServers:    []acp.MCPServer{{Name: "broken", Command: broken}},
	}, nil)
	require.Error(t, err)
	assert.Nil(t, rejected)
	assert.Contains(t, err.Error(), `MCP server "broken" failed to start`)

	// No servers at all still returns an empty (not nil) status list so the
	// mcp/list extension reports "none" rather than "unknown", and the shared
	// runtime still supplies the cleanup hook it owns.
	factory = newFactory(newConfig(), io.Discard)
	none, err := factory.NewSession(t.Context(), acp.SessionConfig{CWD: t.TempDir()}, nil)
	require.NoError(t, err)
	require.NotNil(t, none)
	assert.NotNil(t, none.MCPServers)
	assert.Empty(t, none.MCPServers)
	require.NotNil(t, none.Close)
	require.NoError(t, none.Close())
}

func TestPacketMCPListerReportsConfiguredServers(t *testing.T) {
	cfg := config.Default()
	disabled := false
	cfg.MCP = map[string]config.MCPServerConfig{
		"github": {Command: "gh-mcp"},
		"off":    {Command: "x", Enabled: &disabled},
	}
	servers, err := (&packetMCPLister{cfg: cfg}).ListMCPServers()
	require.NoError(t, err)
	require.Len(t, servers, 2)
	assert.Equal(t, "github", servers[0].Name)
	assert.Equal(t, "configured", servers[0].Status)
	assert.Equal(t, "agent", servers[0].Source)
	assert.Equal(t, "gh-mcp", servers[0].Command)
	assert.Equal(t, "off", servers[1].Name)
	assert.Equal(t, "disabled", servers[1].Status)

	// No [mcp] blocks: an empty list, never nil-shaped surprises.
	empty, err := (&packetMCPLister{cfg: config.Default()}).ListMCPServers()
	require.NoError(t, err)
	assert.Empty(t, empty)
}
