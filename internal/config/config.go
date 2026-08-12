// Package config loads, persists, and exposes packetcode's user configuration.
//
// The on-disk format is TOML at ~/.packetcode/config.toml with 0600 perms.
// API keys may be overridden at runtime via env vars of the form
// PACKETCODE_<SLUG>_API_KEY (e.g. PACKETCODE_OPENAI_API_KEY).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Default         DefaultConfig              `toml:"default"`
	Providers       map[string]ProviderConfig  `toml:"providers"`
	Behavior        BehaviorConfig             `toml:"behavior"`
	Permissions     PermissionConfig           `toml:"permissions"`
	MCP             map[string]MCPServerConfig `toml:"mcp"`
	StatusLine      StatusLineConfig           `toml:"statusline"`
	Hooks           HooksConfig                `toml:"hooks"`
	ACP             ACPConfig                  `toml:"acp"`
	PacketComputers PacketComputersConfig      `toml:"packet_computers"`
	Sugar           SugarConfig                `toml:"sugar"`
	Conduit         ConduitConfig              `toml:"conduit"`
}

// ACPConfig controls the optional Agent Client Protocol stdio server. Enabled
// is a pointer so existing configurations remain enabled when absent.
type ACPConfig struct {
	Enabled         *bool `toml:"enabled,omitempty"`
	enabledOverride *bool
}

func (c ACPConfig) IsEnabled() bool {
	if c.enabledOverride != nil {
		return *c.enabledOverride
	}
	return c.Enabled == nil || *c.Enabled
}

// PacketComputersConfig controls PacketCode's optional local/SSH computer
// registry and remote execution surface. Enabled is a pointer so existing
// configurations remain enabled when the setting is absent.
type PacketComputersConfig struct {
	Enabled         *bool `toml:"enabled,omitempty"`
	enabledOverride *bool
}

func (c PacketComputersConfig) IsEnabled() bool {
	if c.enabledOverride != nil {
		return *c.enabledOverride
	}
	return c.Enabled == nil || *c.Enabled
}

// SugarConfig controls Packetcode's private cache/governor envelope. Sugar
// may enforce stricter workspace policy server-side; these values are requests,
// never a way for the client to weaken that policy.
type SugarConfig struct {
	Enabled         *bool  `toml:"enabled,omitempty"`
	CacheMode       string `toml:"cache_mode"`
	CacheRetention  string `toml:"cache_retention"`
	Privacy         string `toml:"privacy"`
	enabledOverride *bool
}

// SugarExplicitlyDisabled reports whether the user or environment selected a
// hard-off state. Explicit commands such as `sugar login` may activate the
// nil/automatic state, but must respect this state.
func (c *Config) SugarExplicitlyDisabled() bool {
	if c == nil {
		return false
	}
	if c.Sugar.enabledOverride != nil {
		return !*c.Sugar.enabledOverride
	}
	return c.Sugar.Enabled != nil && !*c.Sugar.Enabled
}

// SugarIsEnabled resolves Sugar's tri-state activation. Explicit true/false
// wins. When absent, existing Sugar users stay enabled, while a fresh install
// remains inert until `sugar login` deliberately activates it.
func (c *Config) SugarIsEnabled() bool {
	if c == nil {
		return false
	}
	if c.Sugar.enabledOverride != nil {
		return *c.Sugar.enabledOverride
	}
	if c.Sugar.Enabled != nil {
		return *c.Sugar.Enabled
	}
	if strings.EqualFold(strings.TrimSpace(c.Default.Provider), "sugar") {
		return true
	}
	provider, configured := c.Providers["sugar"]
	return configured && !provider.IsOpenAICompatible()
}

// SugarUsesCustomProvider reports the one supported built-in slug escape:
// explicit built-in disable plus an OpenAI-compatible [providers.sugar].
func (c *Config) SugarUsesCustomProvider() bool {
	if c == nil || !c.SugarExplicitlyDisabled() {
		return false
	}
	provider, configured := c.Providers["sugar"]
	return configured && provider.IsOpenAICompatible()
}

// ConduitConfig controls the optional, decision-only shadow runtime. It is
// disabled by default and never changes the live provider or model choice.
type ConduitConfig struct {
	ShadowEnabled   bool `toml:"shadow_enabled"`
	TimeoutMS       int  `toml:"timeout_ms"`
	CapsuleMaxBytes int  `toml:"capsule_max_bytes"`
}

// ConduitIsEnabled resolves the child setting together with its Sugar parent
// without mutating either stored value.
func (c *Config) ConduitIsEnabled() bool {
	return c != nil && c.SugarIsEnabled() && c.Conduit.ShadowEnabled
}

// MCPServerConfig is the per-server entry for [mcp.<name>] in the user's
// config.toml. The map key is the server name; this struct holds the
// command, args, env, and lifecycle knobs.
//
// Enabled is a *bool so the absent / nil state means "default on" — set
// `enabled = false` explicitly to opt out.
type MCPServerConfig struct {
	Command    string            `toml:"command"`
	Args       []string          `toml:"args,omitempty"`
	Env        map[string]string `toml:"env,omitempty"`
	EnvFrom    []string          `toml:"env_from,omitempty"`
	Enabled    *bool             `toml:"enabled,omitempty"`
	TimeoutSec int               `toml:"timeout_sec,omitempty"`
}

// IsEnabled returns true when the user has not explicitly disabled the
// server. nil pointer → enabled.
func (c MCPServerConfig) IsEnabled() bool { return c.Enabled == nil || *c.Enabled }

type DefaultConfig struct {
	Provider string `toml:"provider"`
	Model    string `toml:"model"`
}

type ProviderConfig struct {
	Type            string                `toml:"type,omitempty"`
	APIKey          string                `toml:"api_key"`
	APIKeyEnv       string                `toml:"api_key_env,omitempty"`
	APIKeyRequired  *bool                 `toml:"api_key_required,omitempty"`
	DefaultModel    string                `toml:"default_model"`
	ReasoningEffort string                `toml:"reasoning_effort,omitempty"`
	Host            string                `toml:"host,omitempty"` // Ollama only
	BaseURL         string                `toml:"base_url,omitempty"`
	DisplayName     string                `toml:"display_name,omitempty"`
	BrandColor      string                `toml:"brand_color,omitempty"`
	Headers         map[string]string     `toml:"headers,omitempty"`
	Models          []ProviderModelConfig `toml:"models,omitempty"`

	// Ollama-only tuning. All optional — omitted means packetcode's smart
	// defaults (auto-sized context, 30m keep-alive, the model's own default
	// temperature), so a stock local install needs none of these.
	NumCtx      int      `toml:"num_ctx,omitempty"`     // fixed context window; 0 = auto-size per request
	KeepAlive   string   `toml:"keep_alive,omitempty"`  // e.g. "30m", "-1" (pin), "0" (unload now)
	Temperature *float64 `toml:"temperature,omitempty"` // nil = leave to the model
}

// ProviderModelConfig is an optional static model entry for custom
// OpenAI-compatible providers whose /models endpoint is unavailable or
// incomplete.
type ProviderModelConfig struct {
	ID            string  `toml:"id"`
	DisplayName   string  `toml:"display_name,omitempty"`
	ContextWindow int     `toml:"context_window,omitempty"`
	SupportsTools *bool   `toml:"supports_tools,omitempty"`
	InputPer1M    float64 `toml:"input_per_1m,omitempty"`
	OutputPer1M   float64 `toml:"output_per_1m,omitempty"`
}

// IsOpenAICompatible reports whether this provider is a user-defined
// OpenAI-compatible endpoint.
func (c ProviderConfig) IsOpenAICompatible() bool {
	t := strings.ToLower(strings.TrimSpace(c.Type))
	return t == "openai_compatible" || t == "openai-compatible"
}

// IsKeylessProvider reports whether a built-in provider authenticates without
// an API key: ollama (a local server) and codex (a ChatGPT subscription whose
// OAuth tokens live in ~/.codex/auth.json). Central so the many key-prompt,
// setup, picker, and doctor call sites stay consistent.
func IsKeylessProvider(slug string) bool {
	return slug == "ollama" || slug == "codex"
}

// RequiresAPIKey reports whether packetcode should require a key before
// registering or validating this provider.
func (c ProviderConfig) RequiresAPIKey(slug string) bool {
	if IsKeylessProvider(slug) {
		return false
	}
	if isReservedHostedProvider(slug) {
		return true
	}
	if c.APIKeyRequired != nil {
		return *c.APIKeyRequired
	}
	return true
}

// ProviderRequiresAPIKey resolves key policy with feature-gate context. The
// built-in Sugar provider always requires its hosted credential, while an
// explicitly enabled custom provider reusing that slug honors
// api_key_required like every other OpenAI-compatible endpoint.
func (c *Config) ProviderRequiresAPIKey(slug string) bool {
	if c == nil {
		return !IsKeylessProvider(slug)
	}
	provider, configured := c.Providers[slug]
	if !configured {
		return !IsKeylessProvider(slug)
	}
	if slug == "sugar" && c.SugarUsesCustomProvider() {
		if provider.APIKeyRequired != nil {
			return *provider.APIKeyRequired
		}
		return true
	}
	return provider.RequiresAPIKey(slug)
}

func isReservedHostedProvider(slug string) bool {
	switch slug {
	case "sugar", "openai", "anthropic", "gemini", "minimax", "deepseek", "grok", "mistral", "openrouter":
		return true
	default:
		return false
	}
}

type BehaviorConfig struct {
	TrustMode            bool `toml:"trust_mode"`
	AutoCompactThreshold int  `toml:"auto_compact_threshold"`
	MaxInputRows         int  `toml:"max_input_rows"`

	// Provider request resilience. Total attempts (incl. the first) for a
	// streaming request on transient errors; 0 means use the default of 3.
	ProviderMaxRetries int `toml:"provider_max_retries"`

	// Abort a provider stream that goes silent for this many seconds; 0
	// means use the default of 60.
	ProviderStallTimeout int `toml:"provider_stall_timeout"`

	// Background agents (see docs/feature-background-agents.md).
	BackgroundMaxConcurrent   int    `toml:"background_max_concurrent"`
	BackgroundMaxDepth        int    `toml:"background_max_depth"`
	BackgroundMaxTotal        int    `toml:"background_max_total"`
	BackgroundDefaultProvider string `toml:"background_default_provider"`
	BackgroundDefaultModel    string `toml:"background_default_model"`
	BackgroundTokenBudget     int    `toml:"background_token_budget"`
	WorkflowTokenBudget       int    `toml:"workflow_token_budget"`
}

// PermissionConfig controls the approval policy applied to tool calls.
type PermissionConfig struct {
	// Profile names the active built-in or custom profile. Built-ins:
	// balanced/ask, safe/read_only, edit/accept_edits, full/trusted.
	Profile string `toml:"profile,omitempty"`
	// Profiles maps custom profile names to tool-action maps. Supported
	// actions are allow, ask, and deny. Keys are tool names, "default",
	// or "mcp" for all MCP tool aliases.
	Profiles map[string]PermissionProfile `toml:"profiles,omitempty"`
	// Rules are ordered explicit overrides. Later rules win when more
	// than one rule matches a tool call.
	Rules []PermissionRule `toml:"rules,omitempty"`

	// Legacy inline overrides from the early permissions draft. Keep
	// parsing them so existing local configs do not break.
	Default string            `toml:"default,omitempty"`
	Tools   map[string]string `toml:"tools,omitempty"`
}

type PermissionProfile map[string]string

type PermissionRule struct {
	Tool          string   `toml:"tool,omitempty"`
	Action        string   `toml:"action"`
	Command       string   `toml:"command,omitempty"`
	CommandPrefix []string `toml:"command_prefix,omitempty"`
	Reason        string   `toml:"reason,omitempty"`
}

// StatusLineConfig declares an optional command that renders the bottom
// status line. The command receives a JSON snapshot on stdin and packetcode
// renders its stdout when it exits successfully.
type StatusLineConfig struct {
	Command    string `toml:"command"`
	Enabled    *bool  `toml:"enabled,omitempty"`
	TimeoutSec int    `toml:"timeout_sec,omitempty"`
}

func (c StatusLineConfig) IsEnabled() bool {
	return c.Command != "" && (c.Enabled == nil || *c.Enabled)
}

// HooksConfig contains user-defined shell hooks. Use TOML arrays of tables:
// [[hooks.user_prompt_submit]], [[hooks.pre_tool_use]], [[hooks.post_tool_use]].
type HooksConfig struct {
	UserPromptSubmit []HookConfig `toml:"user_prompt_submit"`
	PreToolUse       []HookConfig `toml:"pre_tool_use"`
	PostToolUse      []HookConfig `toml:"post_tool_use"`
}

// HookConfig describes one shell command hook. Matcher applies only to
// tool hooks; empty or "*" matches every tool, otherwise it must equal the
// tool name.
type HookConfig struct {
	Command    string `toml:"command"`
	Matcher    string `toml:"matcher,omitempty"`
	Enabled    *bool  `toml:"enabled,omitempty"`
	TimeoutSec int    `toml:"timeout_sec,omitempty"`
}

func (c HookConfig) IsEnabled() bool {
	return c.Command != "" && (c.Enabled == nil || *c.Enabled)
}

// Load reads ~/.packetcode/config.toml and returns the parsed config.
// If the file does not exist, returns Default() — the caller can use
// IsFirstRun() to distinguish a fresh install from a returning user.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}
	return LoadFrom(path)
}

// LoadFrom reads config from an explicit path. Exposed for testing and
// for callers that want to point at a non-default file.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config: %w", err)
		}
		data = nil
	}
	cfg := Default()
	if len(data) > 0 {
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	if cfg.MCP == nil {
		cfg.MCP = map[string]MCPServerConfig{}
	}
	if cfg.Permissions.Tools == nil {
		cfg.Permissions.Tools = map[string]string{}
	}
	if cfg.Permissions.Profiles == nil {
		cfg.Permissions.Profiles = map[string]PermissionProfile{}
	}
	if err := applyIntegrationEnvironment(cfg); err != nil {
		return nil, err
	}
	if err := validateSugarAndConduit(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyIntegrationEnvironment(cfg *Config) error {
	if value, ok := os.LookupEnv("PACKETCODE_ACP_ENABLED"); ok {
		value = strings.TrimSpace(value)
		if value != "" {
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("PACKETCODE_ACP_ENABLED must be true or false")
			}
			cfg.ACP.enabledOverride = &enabled
		}
	}
	if value, ok := os.LookupEnv("PACKETCODE_PACKET_COMPUTERS_ENABLED"); ok {
		value = strings.TrimSpace(value)
		if value != "" {
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("PACKETCODE_PACKET_COMPUTERS_ENABLED must be true or false")
			}
			cfg.PacketComputers.enabledOverride = &enabled
		}
	}
	if value, ok := os.LookupEnv("PACKETCODE_SUGAR_ENABLED"); ok {
		value = strings.TrimSpace(value)
		if value != "" {
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("PACKETCODE_SUGAR_ENABLED must be true or false")
			}
			cfg.Sugar.enabledOverride = &enabled
		}
	}
	if !cfg.SugarIsEnabled() {
		// Sugar owns both the cache envelope and Conduit shadow runtime. Keep
		// every subordinate surface inert when the parent integration is off.
		// Preserve the stored child setting so a later unrelated config save
		// cannot erase the user's preference; runtime consumers resolve it
		// together with the parent gate.
		return nil
	}
	if value, ok := os.LookupEnv("PACKETCODE_SUGAR_CACHE_MODE"); ok {
		cfg.Sugar.CacheMode = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("PACKETCODE_SUGAR_CACHE_RETENTION"); ok {
		cfg.Sugar.CacheRetention = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("PACKETCODE_SUGAR_PRIVACY"); ok {
		cfg.Sugar.Privacy = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("PACKETCODE_CONDUIT_SHADOW"); ok {
		enabled, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("PACKETCODE_CONDUIT_SHADOW must be true or false")
		}
		cfg.Conduit.ShadowEnabled = enabled
	}
	return nil
}

func validateSugarAndConduit(cfg *Config) error {
	if !cfg.SugarIsEnabled() {
		return nil
	}
	switch cfg.Sugar.CacheMode {
	case "auto", "off":
	default:
		return fmt.Errorf("sugar.cache_mode must be auto or off")
	}
	switch cfg.Sugar.CacheRetention {
	case "provider_default", "5m", "30m", "1h":
	default:
		return fmt.Errorf("sugar.cache_retention must be provider_default, 5m, 30m, or 1h")
	}
	switch cfg.Sugar.Privacy {
	case "standard", "zdr_required":
	default:
		return fmt.Errorf("sugar.privacy must be standard or zdr_required")
	}
	if cfg.Conduit.TimeoutMS < 100 || cfg.Conduit.TimeoutMS > 30_000 {
		return fmt.Errorf("conduit.timeout_ms must be between 100 and 30000")
	}
	if cfg.Conduit.CapsuleMaxBytes < 2_048 || cfg.Conduit.CapsuleMaxBytes > 64*1024 {
		return fmt.Errorf("conduit.capsule_max_bytes must be between 2048 and 65536")
	}
	return nil
}

// Save writes the config to ~/.packetcode/config.toml atomically with 0600 perms.
// Atomic = write to temp file in the same directory, then rename.
func (c *Config) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	return c.SaveTo(path)
}

// SaveTo writes the config to an explicit path. Same atomic semantics as Save.
func (c *Config) SaveTo(path string) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config.*.toml.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(buf.String()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp config: %w", err)
	}
	return nil
}

// SetProviderKey records an API key for the named provider and persists.
func (c *Config) SetProviderKey(slug, apiKey string) error {
	if c.Providers == nil {
		c.Providers = map[string]ProviderConfig{}
	}
	p := c.Providers[slug]
	p.APIKey = apiKey
	c.Providers[slug] = p
	return c.Save()
}

// GetProviderKey returns the API key for a provider. The env var
// PACKETCODE_<SLUG>_API_KEY takes precedence over the on-disk value.
// Returns empty string if neither is set.
func (c *Config) GetProviderKey(slug string) string {
	envKey := c.ProviderAPIKeyEnvName(slug)
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	if p, ok := c.Providers[slug]; ok {
		return p.APIKey
	}
	return ""
}

// ProviderAPIKeyEnvName returns the environment variable that overrides
// the configured API key for slug. Custom providers can set api_key_env;
// otherwise slugs are normalized to PACKETCODE_<SLUG>_API_KEY.
func (c *Config) ProviderAPIKeyEnvName(slug string) string {
	if c != nil {
		if p, ok := c.Providers[slug]; ok && strings.TrimSpace(p.APIKeyEnv) != "" {
			return strings.TrimSpace(p.APIKeyEnv)
		}
	}
	return DefaultProviderAPIKeyEnvName(slug)
}

// DefaultProviderAPIKeyEnvName normalizes provider slugs into shell-safe
// PACKETCODE_*_API_KEY variable names.
func DefaultProviderAPIKeyEnvName(slug string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(slug) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		name = "CUSTOM"
	}
	return fmt.Sprintf("PACKETCODE_%s_API_KEY", name)
}

// IsFirstRun reports whether the config file is missing on disk.
func IsFirstRun() bool {
	path, err := ConfigPath()
	if err != nil {
		return true
	}
	_, err = os.Stat(path)
	return os.IsNotExist(err)
}
