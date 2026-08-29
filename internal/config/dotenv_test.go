package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// isolateHome points the packetcode data home at scratch space.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv(HomeEnv, home)
	return home
}

func TestParseDotEnvLine(t *testing.T) {
	for _, tc := range []struct {
		name, line, wantName, wantValue string
		wantOK                          bool
	}{
		{"plain", "KEY=value", "KEY", "value", true},
		{"spaces around equals", "KEY = value", "KEY", "value", true},
		{"export prefix", "export KEY=value", "KEY", "value", true},
		{"double quoted", `KEY="value"`, "KEY", "value", true},
		{"single quoted", "KEY='value'", "KEY", "value", true},
		{"empty value", "KEY=", "KEY", "", true},
		{"comment", "# KEY=value", "", "", false},
		{"blank", "   ", "", "", false},
		{"no equals", "KEY value", "", "", false},
		{"no name", "=value", "", "", false},
		// A trailing comment on an unquoted value is a comment.
		{"trailing comment", "KEY=value # note", "KEY", "value", true},
		// Quoting is how you keep one. Stripping it from a quoted value would
		// silently truncate a key that legitimately contains " #".
		{"hash inside quotes", `KEY="value # not a comment"`, "KEY", "value # not a comment", true},
		// A key is an opaque token. Escapes are not interpreted, and an
		// unmatched quote is part of the value rather than half-eaten.
		{"backslash kept", `KEY=a\nb`, "KEY", `a\nb`, true},
		{"unmatched quote", `KEY="value`, "KEY", `"value`, true},
		{"inner equals", "KEY=a=b", "KEY", "a=b", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name, value, ok := parseDotEnvLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if name != tc.wantName || value != tc.wantValue {
				t.Fatalf("got (%q, %q), want (%q, %q)", name, value, tc.wantName, tc.wantValue)
			}
		})
	}
}

// A project .env wins over the user one for a name both define -- that is what
// a project file is for -- and the origin says which file answered.
func TestLoadDotEnv_ProjectOverridesUserAndRecordsOrigin(t *testing.T) {
	home := isolateHome(t)
	project := t.TempDir()
	writeFile(t, filepath.Join(home, DotEnvFile), "SHARED=from-user\nONLY_USER=u\n")
	writeFile(t, filepath.Join(project, DotEnvFile), "SHARED=from-project\nONLY_PROJECT=p\n")

	d, problems := LoadDotEnv(project)
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	v, from, ok := d.Lookup("SHARED")
	if !ok || v != "from-project" {
		t.Fatalf("SHARED = %q (ok=%v), want from-project", v, ok)
	}
	if !strings.HasPrefix(from, project) {
		t.Fatalf("origin = %q, want the project file", from)
	}
	if v, _, _ := d.Lookup("ONLY_USER"); v != "u" {
		t.Fatalf("user-only name did not survive: %q", v)
	}
	if v, _, _ := d.Lookup("ONLY_PROJECT"); v != "p" {
		t.Fatalf("project-only name missing: %q", v)
	}
}

// Missing files are the normal case, not an error.
func TestLoadDotEnv_MissingFilesAreSilent(t *testing.T) {
	isolateHome(t)
	d, problems := LoadDotEnv(t.TempDir())
	if len(problems) != 0 {
		t.Fatalf("absent .env reported problems: %v", problems)
	}
	if len(d.Names()) != 0 {
		t.Fatalf("expected nothing loaded, got %v", d.Names())
	}
}

// A .env that exists and did nothing is the failure worth naming: the user put
// a key in a file and packetcode behaved as if they had not.
func TestLoadDotEnv_UnreadableFileIsReported(t *testing.T) {
	isolateHome(t)
	project := t.TempDir()
	// A directory where a file should be: readable stat, unreadable content.
	if err := os.MkdirAll(filepath.Join(project, DotEnvFile), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, problems := LoadDotEnv(project)
	if len(problems) == 0 {
		t.Fatal("a .env that could not be read was silent")
	}
}

// Running from the packetcode home must not read the same file twice and
// report every key as project-scoped.
func TestLoadDotEnv_HomeAsWorkingDirReadsOnce(t *testing.T) {
	home := isolateHome(t)
	writeFile(t, filepath.Join(home, DotEnvFile), "KEY=v\n")

	d, problems := LoadDotEnv(home)
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	_, from, ok := d.Lookup("KEY")
	if !ok {
		t.Fatal("KEY did not load")
	}
	if from != filepath.Join(home, DotEnvFile) {
		t.Fatalf("origin = %q, want the single home file", from)
	}
}

// The precedence that matters: a real environment variable is what someone
// deliberately set for this run, and a stale .env silently overriding it is
// the failure that makes dotenv loaders infuriating.
func TestProviderKeyWithOrigin_Precedence(t *testing.T) {
	isolateHome(t)
	cfg := &Config{Providers: map[string]ProviderConfig{
		"openai": {APIKey: "from-config"},
	}}
	envName := cfg.ProviderAPIKeyEnvName("openai")

	// config.toml only.
	key, origin := cfg.ProviderKeyWithOrigin("openai")
	if key != "from-config" || origin.Source != KeySourceConfig {
		t.Fatalf("config tier: %q %+v", key, origin)
	}

	// .env beats config.toml.
	d := &DotEnv{
		values: map[string]string{envName: "from-dotenv"},
		origin: map[string]string{envName: "/somewhere/.env"},
	}
	cfg.SetDotEnv(d)
	key, origin = cfg.ProviderKeyWithOrigin("openai")
	if key != "from-dotenv" || origin.Source != KeySourceDotEnv {
		t.Fatalf("dotenv tier: %q %+v", key, origin)
	}
	if !strings.Contains(origin.Describe(), "/somewhere/.env") {
		t.Fatalf("Describe does not name the file: %q", origin.Describe())
	}

	// A real environment variable beats both.
	t.Setenv(envName, "from-env")
	key, origin = cfg.ProviderKeyWithOrigin("openai")
	if key != "from-env" || origin.Source != KeySourceEnv {
		t.Fatalf("env tier: %q %+v", key, origin)
	}

	// Nothing anywhere.
	empty := &Config{}
	key, origin = empty.ProviderKeyWithOrigin("deepseek")
	if key != "" || origin.Source != KeySourceNone {
		t.Fatalf("empty: %q %+v", key, origin)
	}
	if origin.Describe() != "not set" {
		t.Fatalf("Describe = %q", origin.Describe())
	}
}

// An empty value in .env is not a key. Treating "" as set would shadow a real
// key in config.toml with nothing.
func TestProviderKeyWithOrigin_EmptyDotEnvValueDoesNotShadowConfig(t *testing.T) {
	isolateHome(t)
	cfg := &Config{Providers: map[string]ProviderConfig{"openai": {APIKey: "from-config"}}}
	envName := cfg.ProviderAPIKeyEnvName("openai")
	cfg.SetDotEnv(&DotEnv{
		values: map[string]string{envName: "   "},
		origin: map[string]string{envName: "/x/.env"},
	})
	key, origin := cfg.ProviderKeyWithOrigin("openai")
	if key != "from-config" || origin.Source != KeySourceConfig {
		t.Fatalf("an empty .env value shadowed config.toml: %q %+v", key, origin)
	}
}

// .env must never reach the process environment. packetcode runs shell
// commands on the model's behalf; a file that exists to hold API keys is the
// last thing an arbitrary subprocess should inherit.
func TestLoadDotEnv_DoesNotTouchTheProcessEnvironment(t *testing.T) {
	isolateHome(t)
	project := t.TempDir()
	const name = "PACKETCODE_DOTENV_LEAK_PROBE"
	writeFile(t, filepath.Join(project, DotEnvFile), name+"=leaked\n")

	if _, problems := LoadDotEnv(project); len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if got := os.Getenv(name); got != "" {
		t.Fatalf("%s reached the process environment with %q", name, got)
	}
}

func TestLoadDotEnv_RefusesAnOversizedFile(t *testing.T) {
	isolateHome(t)
	project := t.TempDir()
	// Many short lines, not one long one: a single oversized line is caught by
	// the scanner's own buffer limit, which would let this pass with the size
	// cap deleted. Short lines isolate the cap.
	var b strings.Builder
	for i := 0; b.Len() <= maxDotEnvBytes; i++ {
		fmt.Fprintf(&b, "K%06d=0123456789012345678901234567890123456789\n", i)
	}
	writeFile(t, filepath.Join(project, DotEnvFile), b.String())

	_, problems := LoadDotEnv(project)
	if len(problems) == 0 {
		t.Fatal("an oversized .env was read without complaint")
	}
}
