package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeRawSession puts a session file on disk exactly as given, bypassing the
// writer -- which is the point, since the writer will not produce the shape
// under test.
func writeRawSession(t *testing.T, dir, id string, raw map[string]any) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func futureSession(id string) map[string]any {
	return map[string]any{
		"format_version": currentFormatVersion + 1,
		"id":             id,
		"name":           "from a newer build",
		"created_at":     time.Now().UTC(),
		"updated_at":     time.Now().UTC(),
		"messages":       []any{},
		// A field this build has never heard of, which is the thing that gets
		// destroyed if the refusal is missing.
		"something_new": "must survive",
	}
}

// The failure this exists to stop, stated as the damage rather than the rule.
//
// A newer session decoded cleanly -- encoding/json discards fields it does not
// know -- and migrateSession, seeing a version above its own, changed nothing
// and said nothing. The session loaded looking entirely normal, and the next
// message wrote it back: everything the newer build had written was gone, from
// a file the user never touched, with no error at any point.
func TestLoad_RefusesASessionFromANewerBuild(t *testing.T) {
	dir := t.TempDir()
	id := "01920000-0000-7000-8000-000000000001"
	path := writeRawSession(t, dir, id, futureSession(id))

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	m := NewManager(dir)
	if _, err := m.Load(id); err == nil {
		t.Fatal("a session from a newer build loaded; an older build must refuse it")
	} else if !strings.Contains(err.Error(), "newer than this build") {
		t.Fatalf("refusal does not explain itself: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("the file was rewritten by a build that refused to read it:\n%s", after)
	}
	if !strings.Contains(string(after), "must survive") {
		t.Fatal("the unknown field was destroyed")
	}
}

// Reported, not silently skipped and not listed. Offering it would put a
// session in /resume that refuses to open; hiding it entirely is the failure
// ListWithProblems already exists to prevent.
func TestListWithProblems_ReportsANewerSession(t *testing.T) {
	dir := t.TempDir()
	id := "01920000-0000-7000-8000-000000000002"
	writeRawSession(t, dir, id, futureSession(id))

	summaries, problems, err := NewManager(dir).ListWithProblems()
	if err != nil {
		t.Fatalf("ListWithProblems: %v", err)
	}
	for _, s := range summaries {
		if s.ID == id {
			t.Fatal("a session that cannot be opened was offered in the listing")
		}
	}
	if len(problems) != 1 {
		t.Fatalf("problems = %v, want one", problems)
	}
	if !strings.Contains(problems[0], "newer than this build") {
		t.Fatalf("problem does not say why: %s", problems[0])
	}
}

// An ordinary session is untouched by any of this.
func TestLoad_CurrentAndOlderSessionsStillLoad(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	s, err := m.New("openai", "gpt-5.6-sol")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.FormatVersion != currentFormatVersion {
		t.Fatalf("a new session was written at version %d, want %d", s.FormatVersion, currentFormatVersion)
	}
	if _, err := m.Load(s.ID); err != nil {
		t.Fatalf("a current session did not load: %v", err)
	}

	// Version 0 is what pre-versioning files decode as, and it must still be
	// read and migrated forward -- refusing the older direction would strand
	// every conversation anyone already has.
	old := "01920000-0000-7000-8000-000000000003"
	raw := futureSession(old)
	raw["format_version"] = 0
	writeRawSession(t, dir, old, raw)
	loaded, err := m.Load(old)
	if err != nil {
		t.Fatalf("an unversioned session did not load: %v", err)
	}
	if loaded.FormatVersion != currentFormatVersion {
		t.Fatalf("migration did not bump the version: got %d", loaded.FormatVersion)
	}
}

// The write chokepoint, independent of Load. Every save goes through here, so
// this is where "never write over a newer session" is guaranteed rather than
// merely intended.
func TestWriteSessionFile_RefusesANewerInMemorySession(t *testing.T) {
	dir := t.TempDir()
	s := &Session{
		ID:            "01920000-0000-7000-8000-000000000004",
		FormatVersion: currentFormatVersion + 1,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	err := writeSessionFile(dir, s)
	if err == nil {
		t.Fatal("wrote a session whose version this build does not support")
	}
	if !strings.Contains(err.Error(), "newer than this build") {
		t.Fatalf("refusal does not explain itself: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, s.ID+".json")); statErr == nil {
		t.Fatal("the refused write still created a file")
	}
}
