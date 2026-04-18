package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFrom_MissingFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, "", cfg.Default.Provider)
	assert.Equal(t, 80, cfg.Behavior.AutoCompactThreshold)
	assert.Equal(t, 10, cfg.Behavior.MaxInputRows)
	assert.False(t, cfg.Behavior.TrustMode)
	assert.NotNil(t, cfg.Providers)
}

func TestLoadFrom_ParsesValidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	contents := `
[default]
provider = "openai"
model = "gpt-4.1"

[providers.openai]
api_key = "sk-test"
default_model = "gpt-4.1"

[providers.ollama]
host = "http://localhost:11434"
default_model = "qwen2.5-coder:14b"

[behavior]
trust_mode = true
auto_compact_threshold = 75
max_input_rows = 8
`
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))

	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.Default.Provider)
	assert.Equal(t, "gpt-4.1", cfg.Default.Model)
	assert.Equal(t, "sk-test", cfg.Providers["openai"].APIKey)
	assert.Equal(t, "http://localhost:11434", cfg.Providers["ollama"].Host)
	assert.True(t, cfg.Behavior.TrustMode)
	assert.Equal(t, 75, cfg.Behavior.AutoCompactThreshold)
	assert.Equal(t, 8, cfg.Behavior.MaxInputRows)
}

func TestSaveTo_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	original := Default()
	original.Default.Provider = "gemini"
	original.Default.Model = "gemini-2.5-pro"
	original.Providers["gemini"] = ProviderConfig{
		APIKey:       "AI-test",
		DefaultModel: "gemini-2.5-pro",
	}
	original.Behavior.TrustMode = true

	require.NoError(t, original.SaveTo(path))

	loaded, err := LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, original.Default, loaded.Default)
	assert.Equal(t, original.Providers["gemini"], loaded.Providers["gemini"])
	assert.Equal(t, original.Behavior, loaded.Behavior)
}

func TestSaveTo_FilePermissions0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not enforced on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	require.NoError(t, Default().SaveTo(path))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestGetProviderKey_EnvVarOverride(t *testing.T) {
	cfg := Default()
	cfg.Providers["openai"] = ProviderConfig{APIKey: "from-config"}

	t.Setenv("PACKETCODE_OPENAI_API_KEY", "from-env")

	assert.Equal(t, "from-env", cfg.GetProviderKey("openai"))
}

func TestGetProviderKey_FallsBackToConfig(t *testing.T) {
	cfg := Default()
	cfg.Providers["openai"] = ProviderConfig{APIKey: "from-config"}

	t.Setenv("PACKETCODE_OPENAI_API_KEY", "")

	assert.Equal(t, "from-config", cfg.GetProviderKey("openai"))
}

func TestGetProviderKey_MissingProviderReturnsEmpty(t *testing.T) {
	cfg := Default()
	t.Setenv("PACKETCODE_GEMINI_API_KEY", "")
	assert.Equal(t, "", cfg.GetProviderKey("gemini"))
}

func TestSetProviderKey_PersistsAndUpdates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // Windows

	// Ensure no env var override interferes.
	t.Setenv("PACKETCODE_OPENAI_API_KEY", "")

	cfg, err := Load()
	require.NoError(t, err)

	require.NoError(t, cfg.SetProviderKey("openai", "sk-new-key"))

	reloaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "sk-new-key", reloaded.GetProviderKey("openai"))
}

func TestIsFirstRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	assert.True(t, IsFirstRun(), "fresh temp home should be first run")

	require.NoError(t, Default().Save())

	assert.False(t, IsFirstRun(), "after Save the config should exist")
}

func TestEnsureDir_CreatesNested(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")
	require.NoError(t, EnsureDir(nested))

	info, err := os.Stat(nested)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}
