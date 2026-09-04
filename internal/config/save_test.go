package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfigFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}

func readConfigFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// The failure this prevents is permanent and silent: run an older build once,
// let anything call a save path, and every setting written by a newer build is
// gone. LoadFrom already names those keys as compat problems; saving must
// leave them exactly where it found them.
func TestSaveKeepsSettingsThisBuildDoesNotUnderstand(t *testing.T) {
	path := writeConfigFixture(t, `schema_version = 99
warp_factor = 9

[default]
provider = "openai"
model = "gpt-4.1"

[behavior]
trust_mode = false
telepathy = "enabled"

[providers.openai]
api_key = "old"

[quantum]
entanglement = ["a", "b"]
`)
	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	require.NotEmpty(t, cfg.CompatProblems(), "the fixture should look newer than this build")

	require.NoError(t, cfg.setProviderKeyIn(path, "openai", "new"))

	after := readConfigFixture(t, path)
	assert.Contains(t, after, `api_key = "new"`)
	assert.Contains(t, after, "schema_version = 99", "a newer schema declaration was rewritten")
	assert.Contains(t, after, "warp_factor = 9")
	assert.Contains(t, after, `telepathy = "enabled"`)
	assert.Contains(t, after, "[quantum]")
	assert.Contains(t, after, `entanglement = ["a", "b"]`)
}

// config.toml is a file a person edits by hand. An encoder round trip returns
// it sorted, unspaced, and stripped of every comment they wrote.
func TestSaveKeepsCommentsSpacingAndKeyOrder(t *testing.T) {
	original := `# packetcode configuration
# hands off the ordering, it is deliberate

[default]
model    = "gpt-4.1"     # chosen on purpose
provider = "openai"

# work keys live below
[providers.openai]
api_key       = "old"
default_model = "gpt-4.1"
`
	path := writeConfigFixture(t, original)
	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	require.NoError(t, cfg.setProviderKeyIn(path, "openai", "new"))

	after := readConfigFixture(t, path)
	assert.Equal(t, strings.Replace(original, `api_key       = "old"`, `api_key       = "new"`, 1), after)
}

func TestSaveKeepsCRLFLineEndings(t *testing.T) {
	path := writeConfigFixture(t, "[default]\r\nprovider = \"openai\"\r\nmodel = \"gpt-4.1\"\r\n\r\n[providers.openai]\r\napi_key = \"old\"\r\n")
	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	require.NoError(t, cfg.UpdateIn(path, func(c *Config) {
		c.setProvider("openai", func(p *ProviderConfig) {
			p.APIKey = "new"
			p.ReasoningEffort = "high"
		})
	}))

	after := readConfigFixture(t, path)
	assert.Contains(t, after, "reasoning_effort = \"high\"\r\n")
	assert.NotRegexp(t, "[^\r]\n", after, "a lone LF crept into a CRLF file")
}

func TestSaveKeepsAFileThatEndsWithoutANewline(t *testing.T) {
	path := writeConfigFixture(t, "[providers.openai]\napi_key = \"old\"")
	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	require.NoError(t, cfg.UpdateIn(path, func(c *Config) {
		c.setProvider("openai", func(p *ProviderConfig) { p.DefaultModel = "gpt-5" })
	}))

	assert.Equal(t, "[providers.openai]\napi_key = \"old\"\ndefault_model = \"gpt-5\"\n", readConfigFixture(t, path))
	reloaded, err := LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, "gpt-5", reloaded.Providers["openai"].DefaultModel)
}

// Nothing to preserve means the whole struct is written, so a first run gets a
// complete, readable config rather than the two keys setup happened to set.
func TestSaveWritesTheWholeStructWhenThereIsNoFileToPreserve(t *testing.T) {
	for name, seed := range map[string]string{
		"missing file": "",
		"empty file":   "",
		"only blanks":  "\n\n   \n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			if name != "missing file" {
				require.NoError(t, os.WriteFile(path, []byte(seed), 0o600))
			}
			cfg := Default()
			cfg.Default.Provider = "openai"
			require.NoError(t, cfg.SaveTo(path))

			after := readConfigFixture(t, path)
			assert.Contains(t, after, "[behavior]")
			assert.Contains(t, after, "[permissions]")
			assert.Contains(t, after, `provider = "openai"`)
		})
	}
}

// A save that changes nothing must be a no-op on disk, not a rewrite that
// happens to land on the same content.
func TestSaveTouchesNothingWhenNothingChanged(t *testing.T) {
	original := "# untouched\n[default]\nprovider = \"openai\"\nmodel = \"gpt-4.1\"\n"
	path := writeConfigFixture(t, original)
	cfg, err := LoadFrom(path)
	require.NoError(t, err)

	before, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, cfg.SaveTo(path))

	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, original, readConfigFixture(t, path))
	assert.Equal(t, before.ModTime(), after.ModTime(), "an unchanged config was rewritten")
}

// `/effort default` clears the setting. Leaving `reasoning_effort = ""` behind
// would be a value the loader treats differently from an absent key in every
// field that is optional.
func TestSaveClearsASettingThatWentEmpty(t *testing.T) {
	path := writeConfigFixture(t, "[providers.openai]\napi_key = \"k\"\nreasoning_effort = \"high\"\ndefault_model = \"gpt-5\"\n")
	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	require.NoError(t, cfg.setProviderReasoningEffortIn(path, "openai", ""))

	after := readConfigFixture(t, path)
	assert.Equal(t, "[providers.openai]\napi_key = \"k\"\ndefault_model = \"gpt-5\"\n", after)
}

func TestSaveAddsSettingsWithoutDisturbingTheirNeighbours(t *testing.T) {
	path := writeConfigFixture(t, "# top of file\n[behavior]\ntrust_mode = false\n")
	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	require.NoError(t, cfg.setActiveModelIn(path, "openai", "gpt-5"))

	after := readConfigFixture(t, path)
	assert.Contains(t, after, "# top of file")
	assert.Contains(t, after, "[providers.openai]")
	assert.Contains(t, after, `default_model = "gpt-5"`)
	assert.Contains(t, after, "[default]")

	reloaded, err := LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, "openai", reloaded.Default.Provider)
	assert.Equal(t, "gpt-5", reloaded.Default.Model)
	assert.Equal(t, "gpt-5", reloaded.Providers["openai"].DefaultModel)
	assert.False(t, reloaded.Behavior.TrustMode)
}

// The patcher cannot address one element of an array of tables, so a change
// there is refused by name. Writing an approximation would corrupt the file
// this whole change exists to protect.
func TestSaveRefusesAChangeItCannotMakeSafely(t *testing.T) {
	original := `[permissions]
profile = "balanced"

[[permissions.rules]]
tool = "spawn_agent"
action = "deny"
`
	path := writeConfigFixture(t, original)
	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	require.Len(t, cfg.Permissions.Rules, 1)

	err = cfg.UpdateIn(path, func(c *Config) {
		c.Permissions.Rules = append(c.Permissions.Rules, PermissionRule{Tool: "execute_command", Action: "ask"})
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "array of tables")
	assert.Equal(t, original, readConfigFixture(t, path), "a refused save must not touch the file")
}

// Update persists its own mutation and nothing else, which is what makes it
// safe for a caller that shares a *Config with the rest of the process.
func TestUpdatePersistsOnlyItsOwnMutation(t *testing.T) {
	path := writeConfigFixture(t, "[default]\nprovider = \"openai\"\nmodel = \"gpt-4.1\"\n")
	cfg, err := LoadFrom(path)
	require.NoError(t, err)

	// Something elsewhere in the process adjusted a field for this run only.
	cfg.Behavior.TrustMode = true

	require.NoError(t, cfg.setProviderKeyIn(path, "openai", "sk-new"))

	after := readConfigFixture(t, path)
	assert.Contains(t, after, `api_key = "sk-new"`)
	assert.NotContains(t, after, "trust_mode", "Update swept along a field its mutation never touched")
}

// SaveTo keeps its older, broader contract: it persists whatever the in-memory
// config now says, because every existing caller mutates the struct and then
// calls it.
func TestSaveToPersistsInMemoryChanges(t *testing.T) {
	path := writeConfigFixture(t, "[default]\nprovider = \"openai\"\nmodel = \"gpt-4.1\"\n")
	cfg, err := LoadFrom(path)
	require.NoError(t, err)
	cfg.Behavior.TrustMode = true
	require.NoError(t, cfg.SaveTo(path))

	reloaded, err := LoadFrom(path)
	require.NoError(t, err)
	assert.True(t, reloaded.Behavior.TrustMode)
}

// A file that does not parse is not a file to patch. Refusing keeps whatever
// the user typed instead of replacing it with the encoder's idea of it.
func TestSaveRefusesToPatchAFileItCannotParse(t *testing.T) {
	original := "[default\nprovider = \"openai\"\n"
	path := writeConfigFixture(t, original)
	cfg := Default()
	cfg.Default.Provider = "anthropic"

	err := cfg.SaveTo(path)
	require.Error(t, err)
	assert.Equal(t, original, readConfigFixture(t, path))
}

// The verification step is the last thing between a patcher bug and a
// corrupted config, so it is tested against corruption directly rather than
// only through patches that happen to be correct.
func TestVerifyTOMLPatchRefusesAnythingButTheIntendedChange(t *testing.T) {
	before := "[default]\nprovider = \"openai\"\nmodel = \"gpt-4.1\"\n\n[behavior]\ntrust_mode = false\n"
	edits := []tomlEdit{{path: []string{"default", "model"}, value: "gpt-5"}}

	good := "[default]\nprovider = \"openai\"\nmodel = \"gpt-5\"\n\n[behavior]\ntrust_mode = false\n"
	require.NoError(t, verifyTOMLPatch(before, good, edits))

	cases := map[string]struct {
		after   string
		wantErr string
	}{
		"a key was lost": {
			after:   "[default]\nmodel = \"gpt-5\"\n\n[behavior]\ntrust_mode = false\n",
			wantErr: "would drop default.provider",
		},
		"an untouched key changed": {
			after:   "[default]\nprovider = \"anthropic\"\nmodel = \"gpt-5\"\n\n[behavior]\ntrust_mode = false\n",
			wantErr: "would change default.provider",
		},
		"a key appeared": {
			after:   "[default]\nprovider = \"openai\"\nmodel = \"gpt-5\"\n\n[behavior]\ntrust_mode = false\nmax_input_rows = 4\n",
			wantErr: "would add behavior.max_input_rows",
		},
		"the edit did not land": {
			after:   before,
			wantErr: "default.model did not survive",
		},
		"the result does not parse": {
			after:   "[default\nmodel = \"gpt-5\"\n",
			wantErr: "would not parse",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := verifyTOMLPatch(before, tc.after, edits)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
