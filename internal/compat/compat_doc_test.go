package compat

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// docPath is the published form of this package.
const docPath = "../../docs/compatibility.md"

// A contract that can drift from the code silently is not a contract, it is a
// document. This test is what makes docs/compatibility.md the former: the
// version numbers and rules it publishes cannot go stale without failing the
// build, and the person who bumps a constant is told, at the moment they bump
// it, that the published version now disagrees.
func TestDoc_MatchesFormats(t *testing.T) {
	rows := parseFormatTable(t)

	for _, f := range Formats() {
		row, ok := rows[f.Name]
		if !ok {
			t.Errorf("format %q is not in the table in %s; every persisted format must be published",
				f.Name, docPath)
			continue
		}
		if row.version != f.Version {
			t.Errorf("format %q: doc says version %d, code says %d — bump both or neither",
				f.Name, row.version, f.Version)
		}
		if row.rule != f.Rule.String() {
			t.Errorf("format %q: doc says %q, code says %q",
				f.Name, row.rule, f.Rule.String())
		}
		if row.location != f.Location {
			t.Errorf("format %q: doc says location %q, code says %q",
				f.Name, row.location, f.Location)
		}
	}

	// The other direction. A row for a format that no longer exists is worse
	// than a missing one: it publishes a promise about a file nothing writes.
	known := map[string]bool{}
	for _, f := range Formats() {
		known[f.Name] = true
	}
	for name := range rows {
		if !known[name] {
			t.Errorf("%s publishes format %q, which Formats() does not list", docPath, name)
		}
	}
}

// Every version in the table must also appear in the format changelog. A bump
// with no changelog entry is the failure this catches: the number changes, the
// build keeps working, and nobody can find out what changed about the format
// or whether their files are affected.
func TestDoc_ChangelogCoversEveryVersion(t *testing.T) {
	doc := readDoc(t)
	_, changelog, found := strings.Cut(doc, "## Format changelog")
	if !found {
		t.Fatalf("%s has no format changelog section", docPath)
	}

	for _, f := range Formats() {
		// Either a dedicated heading ("### session 2") or a shared one
		// ("### session 1, job 1, ... — initial").
		want := fmt.Sprintf("%s %d", f.Name, f.Version)
		if !strings.Contains(changelog, want) {
			t.Errorf("format changelog does not mention %q; a version bump needs an entry saying what changed",
				want)
		}
	}
}

func TestTooNew_SaysWhatAndWhatToDo(t *testing.T) {
	err := TooNew("session", 7, 2)
	for _, want := range []string{"session", "7", "2", "newer than this build", "Upgrade packetcode"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("TooNew message is missing %q: %s", want, err)
		}
	}
}

// Rule.String feeds the doc comparison, so an unnamed rule would make the
// comparison pass by matching "unknown" on both sides of nothing.
func TestRule_AllNamed(t *testing.T) {
	for _, r := range []Rule{ReadOlder, ExactOnly, Advisory} {
		if r.String() == "unknown" {
			t.Errorf("rule %d has no name", int(r))
		}
	}
}

type docRow struct {
	location string
	version  int
	rule     string
}

// tableRow matches one row of the Formats table:
// | name | `location` | 1 | rule |
var tableRow = regexp.MustCompile(`^\|\s*([^|]+?)\s*\|\s*` + "`" + `([^|` + "`" + `]+)` + "`" + `\s*\|\s*(\d+)\s*\|\s*([^|]+?)\s*\|$`)

func parseFormatTable(t *testing.T) map[string]docRow {
	t.Helper()
	rows := map[string]docRow{}
	for _, line := range strings.Split(readDoc(t), "\n") {
		m := tableRow.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		version, err := strconv.Atoi(m[3])
		if err != nil {
			continue
		}
		rows[m[1]] = docRow{location: m[2], version: version, rule: m[4]}
	}
	if len(rows) == 0 {
		t.Fatalf("no format rows parsed from %s; the table shape changed and this test "+
			"would otherwise pass by finding nothing", docPath)
	}
	return rows
}

func readDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(docPath))
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}
