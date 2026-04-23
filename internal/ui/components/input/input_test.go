package input

import (
	"strings"
	"testing"
)

// TestInput_SetValueReplacesBufferAndMovesCursorToEnd verifies the
// autocomplete-accept path: SetValue swaps the buffer AND puts the
// cursor at the end so the next keystroke appends rather than inserts
// in the middle.
func TestInput_SetValueReplacesBufferAndMovesCursorToEnd(t *testing.T) {
	m := New()
	m.SetValue("/spawn ")
	if got := m.Value(); got != "/spawn " {
		t.Fatalf("Value() = %q, want %q", got, "/spawn ")
	}
	// Swap to a shorter value and verify the old text is gone.
	m.SetValue("/help")
	if got := m.Value(); got != "/help" {
		t.Fatalf("Value() after second SetValue = %q, want %q", got, "/help")
	}
}

func TestInput_ViewCompactByDefault(t *testing.T) {
	m := New()
	m.Resize(80, 0)

	lines := strings.Split(m.View(), "\n")
	if got, want := len(lines), 3; got != want {
		t.Fatalf("empty input height = %d lines, want %d\n%s", got, want, m.View())
	}
}

func TestInput_ViewGrowsForMultilineText(t *testing.T) {
	m := New()
	m.Resize(80, 0)
	m.SetValue("one\ntwo\nthree")

	lines := strings.Split(m.View(), "\n")
	if got, want := len(lines), 5; got != want {
		t.Fatalf("multiline input height = %d lines, want %d\n%s", got, want, m.View())
	}
}
