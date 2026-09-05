package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationProblems_CleanDefaultConfig(t *testing.T) {
	cfg := Default()
	assert.Empty(t, cfg.ValidationProblems())
}

func TestValidationProblems_NamesMissingEnvFromVariable(t *testing.T) {
	t.Setenv("PACKETCODE_TEST_PRESENT", "1")
	cfg := Default()
	cfg.MCP["gh"] = MCPServerConfig{Command: "gh-mcp", EnvFrom: []string{"PACKETCODE_TEST_PRESENT", "PACKETCODE_TEST_DEFINITELY_ABSENT"}}
	problems := cfg.ValidationProblems()
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "[mcp.gh]")
	assert.Contains(t, problems[0], "PACKETCODE_TEST_DEFINITELY_ABSENT")
	assert.NotContains(t, problems[0], "PACKETCODE_TEST_PRESENT")
}

func TestValidationProblems_MCPShape(t *testing.T) {
	cfg := Default()
	cfg.MCP["bad name"] = MCPServerConfig{Command: "x"}
	cfg.MCP["empty"] = MCPServerConfig{}
	off := false
	cfg.MCP["disabled"] = MCPServerConfig{Enabled: &off}
	problems := strings.Join(cfg.ValidationProblems(), "\n")
	assert.Contains(t, problems, "[mcp.bad name]")
	assert.Contains(t, problems, "[mcp.empty] is enabled but has no command")
	assert.NotContains(t, problems, "[mcp.disabled]")
}

func TestValidationProblems_ProviderKeyEnvAndBaseURL(t *testing.T) {
	cfg := Default()
	cfg.Providers["lab"] = ProviderConfig{Type: "openai_compatible", BaseURL: "ftp://x", APIKeyEnv: "PACKETCODE_TEST_LAB_TOKEN"}
	problems := strings.Join(cfg.ValidationProblems(), "\n")
	assert.Contains(t, problems, "[providers.lab] base_url must use http or https")
	assert.Contains(t, problems, "api_key_env names PACKETCODE_TEST_LAB_TOKEN")

	t.Setenv("PACKETCODE_TEST_LAB_TOKEN", "k")
	cfg.Providers["lab"] = ProviderConfig{Type: "openai_compatible", BaseURL: "https://lab.example/v1", APIKeyEnv: "PACKETCODE_TEST_LAB_TOKEN"}
	assert.Empty(t, cfg.ValidationProblems())
}

func TestValidationProblems_DefaultProviderWithoutKeyNamesTheVariable(t *testing.T) {
	t.Setenv("PACKETCODE_OPENAI_API_KEY", "")
	cfg := Default()
	cfg.Default.Provider = "openai"
	cfg.Default.Model = "gpt-5.6-sol"
	problems := strings.Join(cfg.ValidationProblems(), "\n")
	assert.Contains(t, problems, "[default] provider openai has no API key")
	assert.Contains(t, problems, "PACKETCODE_OPENAI_API_KEY")

	cfg.Providers["openai"] = ProviderConfig{APIKey: "sk-test"}
	assert.Empty(t, cfg.ValidationProblems())

	cfg.Default.Provider = "nosuch"
	problems = strings.Join(cfg.ValidationProblems(), "\n")
	assert.Contains(t, problems, `[default] provider "nosuch" is not a built-in provider`)
}

func TestValidationProblems_KeylessDefaultsAreClean(t *testing.T) {
	for _, slug := range []string{"ollama", "codex"} {
		cfg := Default()
		cfg.Default.Provider = slug
		assert.Empty(t, cfg.ValidationProblems(), slug)
	}
}

func TestValidationProblems_BehaviorRanges(t *testing.T) {
	cfg := Default()
	cfg.Behavior.AutoCompactThreshold = 120
	cfg.Behavior.ProviderMaxRetries = -1
	cfg.Behavior.BackgroundMaxDepth = -2
	problems := strings.Join(cfg.ValidationProblems(), "\n")
	assert.Contains(t, problems, "auto_compact_threshold 120")
	assert.Contains(t, problems, "provider_max_retries -1")
	assert.Contains(t, problems, "background_max_depth -2")
}
