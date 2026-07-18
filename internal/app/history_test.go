package app

import "testing"

func TestPromptHistory_RecallPrevNext(t *testing.T) {
	r := newTestApp(t)

	r.app.recordHistory("first")
	r.app.recordHistory("second")
	r.app.recordHistory("third")

	// Start typing a new draft, then page back through history.
	r.app.input.SetValue("draft")

	if !r.app.historyPrev() {
		t.Fatal("historyPrev should consume the key")
	}
	if got := r.app.input.Value(); got != "third" {
		t.Fatalf("1st Up = %q, want third", got)
	}
	r.app.historyPrev()
	if got := r.app.input.Value(); got != "second" {
		t.Fatalf("2nd Up = %q, want second", got)
	}
	r.app.historyPrev()
	if got := r.app.input.Value(); got != "first" {
		t.Fatalf("3rd Up = %q, want first", got)
	}
	// At the oldest entry, further Up is consumed but does not wrap.
	r.app.historyPrev()
	if got := r.app.input.Value(); got != "first" {
		t.Fatalf("Up past oldest = %q, want first (no wrap)", got)
	}

	// Page forward; the stashed draft returns after the newest entry.
	r.app.historyNext()
	if got := r.app.input.Value(); got != "second" {
		t.Fatalf("Down = %q, want second", got)
	}
	r.app.historyNext()
	if got := r.app.input.Value(); got != "third" {
		t.Fatalf("Down = %q, want third", got)
	}
	r.app.historyNext()
	if got := r.app.input.Value(); got != "draft" {
		t.Fatalf("Down past newest = %q, want restored draft", got)
	}
	// Not navigating anymore → Down is not consumed.
	if r.app.historyNext() {
		t.Fatal("historyNext should return false when not navigating")
	}
}

func TestPromptHistory_EmptyAndDedup(t *testing.T) {
	r := newTestApp(t)

	// No history yet → Up is not consumed.
	if r.app.historyPrev() {
		t.Fatal("historyPrev should return false with empty history")
	}

	// Blank submissions are not recorded; consecutive duplicates collapse.
	r.app.recordHistory("  ")
	r.app.recordHistory("dup")
	r.app.recordHistory("dup")
	if got := len(r.app.promptHistory); got != 1 {
		t.Fatalf("history length = %d, want 1 (blank skipped, dup collapsed)", got)
	}
}
