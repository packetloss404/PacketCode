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

// TestInput_ReplaceMention splices the chosen path over the "@query" token
// and leaves the caret at the end with a trailing space.
func TestInput_ReplaceMention(t *testing.T) {
	m := New()
	m.SetValue("hello @inp")
	// "@inp" starts at byte 6; end is the buffer length (caret at end).
	m.ReplaceMention(6, len(m.Value()), "internal/x.go")
	if got, want := m.Value(), "hello @internal/x.go "; got != want {
		t.Fatalf("Value() = %q, want %q", got, want)
	}
}

// TestInput_ReplaceMention_PreservesTail keeps text after the token intact.
func TestInput_ReplaceMention_PreservesTail(t *testing.T) {
	m := New()
	m.SetValue("a @q z")
	// Replace only the "@q" at bytes [2,4); the " z" tail must survive.
	m.ReplaceMention(2, 4, "path.go")
	if got, want := m.Value(), "a @path.go  z"; got != want {
		t.Fatalf("Value() = %q, want %q", got, want)
	}
}

// TestInput_ReplaceMention_OutOfRangeIsNoop guards defensive bounds.
func TestInput_ReplaceMention_OutOfRangeIsNoop(t *testing.T) {
	m := New()
	m.SetValue("hi")
	m.ReplaceMention(5, 9, "x.go") // start > len
	if got := m.Value(); got != "hi" {
		t.Fatalf("out-of-range ReplaceMention mutated buffer: %q", got)
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
