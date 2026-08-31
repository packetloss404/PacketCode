package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/packetcode/packetcode/internal/compat"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// config.toml is the one format the contract does not refuse, and this is why:
// it is a file a person typed, packetcode never rewrites it, so there is
// nothing to corrupt -- and refusing to start because they once ran a newer
// build is a worse failure than the one being prevented.
func TestLoadFrom_NewerSchemaIsReportedNotFatal(t *testing.T) {
	path := writeConfig(t, "schema_version = 99\n\n[default]\nprovider = \"openai\"\n")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("a newer config must still load: %v", err)
	}
	if cfg.Default.Provider != "openai" {
		t.Fatalf("settings this build understands were dropped: %q", cfg.Default.Provider)
	}
	problems := cfg.CompatProblems()
	if len(problems) == 0 {
		t.Fatal("a newer schema loaded in silence")
	}
	joined := strings.Join(problems, "\n")
	for _, want := range []string{"99", "ignored"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("problem does not mention %q:\n%s", want, joined)
		}
	}
}

// The setting that does nothing, which is how someone spends an afternoon.
// BurntSushi ignores unknown keys, which is correct -- an older build must not
// fail on an option it has never heard of -- but silent is not the same as
// reported.
func TestLoadFrom_UnknownKeysAreNamed(t *testing.T) {
	cfg, err := LoadFrom(writeConfig(t,
		"[default]\nprovider = \"openai\"\nnot_a_setting = true\n"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	joined := strings.Join(cfg.CompatProblems(), "\n")
	if !strings.Contains(joined, "not_a_setting") {
		t.Fatalf("an unrecognised key was not named:\n%s", joined)
	}
	// Singular reads naturally; the plural branch is exercised below.
	if !strings.Contains(joined, "it was ignored") {
		t.Fatalf("singular did not agree:\n%s", joined)
	}
}

func TestLoadFrom_ManyUnknownKeysAreCappedAndCounted(t *testing.T) {
	body := "[default]\nprovider = \"openai\"\n"
	for _, k := range []string{"aa", "bb", "cc", "dd", "ee", "ff", "gg"} {
		body += k + " = 1\n"
	}
	cfg, err := LoadFrom(writeConfig(t, body))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	joined := strings.Join(cfg.CompatProblems(), "\n")
	if !strings.Contains(joined, "and 2 more") {
		t.Fatalf("the remainder was not counted:\n%s", joined)
	}
	if !strings.Contains(joined, "they were ignored") {
		t.Fatalf("plural did not agree:\n%s", joined)
	}
}

// The overwhelmingly common case: a config with no schema_version at all,
// written before the field existed. Refusing or nagging about those would
// guard a case that has not happened yet at the cost of every case that has.
func TestLoadFrom_AbsentSchemaVersionIsSilent(t *testing.T) {
	cfg, err := LoadFrom(writeConfig(t,
		"[default]\nprovider = \"openai\"\nmodel = \"gpt-5.6-sol\"\n"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if problems := cfg.CompatProblems(); len(problems) != 0 {
		t.Fatalf("an ordinary config was complained about: %v", problems)
	}
	if cfg.SchemaVersion != 0 {
		t.Fatalf("SchemaVersion = %d, want 0 for a file that does not set it", cfg.SchemaVersion)
	}
}

// The current version is not newer than itself.
func TestLoadFrom_CurrentSchemaVersionIsSilent(t *testing.T) {
	cfg, err := LoadFrom(writeConfig(t,
		"schema_version = 1\n\n[default]\nprovider = \"openai\"\n"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if problems := cfg.CompatProblems(); len(problems) != 0 {
		t.Fatalf("the current schema version was complained about: %v", problems)
	}
	if compat.ConfigVersion != 1 {
		t.Fatalf("this test hard-codes schema_version 1; compat.ConfigVersion is now %d",
			compat.ConfigVersion)
	}
}

// A missing file is not a compatibility problem.
func TestLoadFrom_MissingFileHasNoProblems(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if problems := cfg.CompatProblems(); len(problems) != 0 {
		t.Fatalf("a missing config produced problems: %v", problems)
	}
}
