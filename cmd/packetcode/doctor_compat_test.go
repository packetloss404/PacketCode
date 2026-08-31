package main

import (
	"os"
	"strings"
	"testing"

	"github.com/packetcode/packetcode/internal/config"
)

// doctor is where someone lands when "the option I set does nothing", by which
// point the startup banner has scrolled away. Wiring the report into run()
// alone would leave doctor calling the config file fine while a setting in it
// was being ignored -- the same shape as the bug that once left doctor unable
// to see .env at all.
func TestDoctorReportsIgnoredConfigSettings(t *testing.T) {
	restore := isolateDoctorEnv(t)
	defer restore()

	path, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(strings.TrimSuffix(path, "config.toml"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Written whole rather than appended. schema_version is a top-level key,
	// and appending it after a [table] header puts it inside that table --
	// which is a fact about TOML, not about packetcode, and cost this test a
	// run to remember.
	body := "schema_version = 99\nnot_a_setting = true\n\n" +
		"[default]\nprovider = \"openai\"\nmodel = \"gpt-5.6-sol\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("a config from a newer build must still load: %v", err)
	}
	r := doctorReport{}
	addConfigCompatCheck(&r, loaded)
	check := assertDoctorCheck(t, r, "config.compatibility", doctorWarn)
	for _, want := range []string{"not_a_setting", "99"} {
		if !strings.Contains(check.Detail, want) {
			t.Fatalf("detail does not name %q: %q", want, check.Detail)
		}
	}
	if check.Docs != "docs/compatibility.md" {
		t.Fatalf("check does not point at the contract: %q", check.Docs)
	}
}

func TestDoctorConfigCompatibilityIsOKForAnOrdinaryConfig(t *testing.T) {
	r := doctorReport{}
	addConfigCompatCheck(&r, config.Default())
	assertDoctorCheck(t, r, "config.compatibility", doctorOK)
}
