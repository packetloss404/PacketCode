package app

import (
	"strings"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/session"
)

// With nothing saved, /resume says so rather than opening an empty overlay the
// user then has to dismiss.
func TestResume_NoSavedSessionsReportsInsteadOfOpening(t *testing.T) {
	rig := newTestApp(t)
	// The rig's only session is the current one; deleting it would leave no
	// manager, so instead assert against a manager pointed at an empty dir.
	rig.app.deps.Sessions = session.NewManager(t.TempDir())

	rig.app.handleResumeCommand(nil)

	if rig.app.picker.Visible() {
		t.Fatal("no sessions to choose from, so no picker should open")
	}
	if !strings.Contains(rig.app.conversation.View(), "no saved sessions") {
		t.Fatalf("expected an explanation, got:\n%s", rig.app.conversation.View())
	}
}

// The bare form opens a picker over saved sessions.
func TestResume_OpensAPickerOverSavedSessions(t *testing.T) {
	rig := newTestApp(t)
	if _, err := rig.app.deps.Sessions.New("fake", "fake-model"); err != nil {
		t.Fatalf("New: %v", err)
	}

	rig.app.handleResumeCommand(nil)

	if !rig.app.picker.Visible() {
		t.Fatal("/resume should open the session picker")
	}
	if got := rig.app.picker.ID(); got != "session" {
		t.Fatalf("picker ID = %q, want %q — the select handler keys on it", got, "session")
	}
}

// An id argument skips the picker and behaves like /sessions resume, so the
// two spellings agree.
func TestResume_WithAnUnknownIDReportsIt(t *testing.T) {
	rig := newTestApp(t)
	rig.app.handleResumeCommand([]string{"nosuchid"})

	if rig.app.picker.Visible() {
		t.Fatal("an explicit id should not open the picker")
	}
	out := rig.app.conversation.View()
	if !strings.Contains(out, "resume:") {
		t.Fatalf("failure should be attributed to resume, got:\n%s", out)
	}
}

// The detail line answers what someone scanning the list is asking: how long
// ago, how big, and on what model.
func TestSessionItems_DescribeEachSessionAndMarkTheCurrentOne(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	items := sessionItems([]session.Summary{
		{ID: "aaaaaaaabbbbbbbb", Name: "auth refactor", UpdatedAt: now.Add(-2 * time.Hour),
			Provider: "openai", Model: "gpt-5", MessageCount: 14},
		{ID: "ccccccccdddddddd", UpdatedAt: now.Add(-30 * time.Minute), MessageCount: 3},
	}, "ccccccccdddddddd", now)

	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Label != "auth refactor" {
		t.Fatalf("a named session should show its name, got %q", items[0].Label)
	}
	for _, want := range []string{"ago", "14 msg", "openai/gpt-5"} {
		if !strings.Contains(items[0].Detail, want) {
			t.Fatalf("detail %q missing %q", items[0].Detail, want)
		}
	}
	if items[1].Label != shortID("ccccccccdddddddd") {
		t.Fatalf("an unnamed session should fall back to its short id, got %q", items[1].Label)
	}
	if items[1].Marker == "" {
		t.Fatal("the current session should be marked, not hidden")
	}
	if items[0].Marker != "" {
		t.Fatal("only the current session carries the marker")
	}
	// A session with no provider metadata must not render a bare separator.
	if strings.Contains(items[1].Detail, "· /") || strings.HasSuffix(items[1].Detail, "·") {
		t.Fatalf("detail should omit absent provider/model cleanly, got %q", items[1].Detail)
	}
}
