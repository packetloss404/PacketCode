package input

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if got, want := m.Value(), "a @path.go z"; got != want {
		t.Fatalf("Value() = %q, want %q", got, want)
	}
	if got, want := m.CursorByteOffset(), len("a @path.go "); got != want {
		t.Fatalf("caret = %d, want %d", got, want)
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
	if strings.Contains(m.View(), "│") || !strings.Contains(m.View(), "─") {
		t.Fatalf("input should use horizontal rules without side borders:\n%s", m.View())
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

func TestInput_ConfiguredMaxRowsAndSoftWrapGrowth(t *testing.T) {
	m := New()
	m.SetMaxRows(6)
	m.Resize(24, 0)
	m.SetValue("this is a long prompt that should occupy several visual rows")
	if got := len(strings.Split(m.View(), "\n")); got <= 3 {
		t.Fatalf("soft-wrapped input did not grow: %d lines\n%s", got, m.View())
	}

	m.SetValue("1\n2\n3\n4\n5\n6")
	if got, want := len(strings.Split(m.View(), "\n")), 8; got != want {
		t.Fatalf("configured input height = %d lines, want %d", got, want)
	}
}

func TestInput_MaxRowsRecoversAfterTemporarySmallResize(t *testing.T) {
	m := New()
	m.SetMaxRows(6)
	m.Resize(80, 5)
	m.SetValue("1\n2\n3\n4\n5\n6")
	if got, want := len(strings.Split(m.View(), "\n")), 5; got != want {
		t.Fatalf("small-terminal input height = %d lines, want %d", got, want)
	}

	m.Resize(80, 40)
	if got, want := len(strings.Split(m.View(), "\n")), 8; got != want {
		t.Fatalf("restored input height = %d lines, want %d", got, want)
	}
}

func TestInput_ViewWithPlaceholderDoesNotMutateDefault(t *testing.T) {
	m := New()
	m.Resize(80, 0)
	if got := m.ViewWithPlaceholder("describe a task"); !strings.Contains(got, "describe a task") {
		t.Fatalf("custom placeholder missing:\n%s", got)
	}
	if got := m.View(); !strings.Contains(got, "Ask packetcode anything") {
		t.Fatalf("default placeholder was mutated:\n%s", got)
	}
}

func TestInput_MultilineFallbacksDoNotSubmit(t *testing.T) {
	for name, key := range map[string]tea.KeyMsg{
		"ctrl+j":    {Type: tea.KeyCtrlJ},
		"alt+enter": {Type: tea.KeyEnter, Alt: true},
	} {
		t.Run(name, func(t *testing.T) {
			m := New()
			m.SetValue("one")
			updated, cmd := m.Update(key)
			if got, want := updated.Value(), "one\n"; got != want {
				t.Fatalf("Value() = %q, want %q", got, want)
			}
			if cmd != nil {
				if _, ok := cmd().(SubmitMsg); ok {
					t.Fatal("newline shortcut submitted the prompt")
				}
			}
		})
	}
}

func TestInput_BackslashEnterInsertsNewlineAtCaret(t *testing.T) {
	m := New()
	m.SetValue("one\\two")
	for range len("two") {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got, want := updated.Value(), "one\ntwo"; got != want {
		t.Fatalf("Value() = %q, want %q", got, want)
	}
	if cmd != nil {
		t.Fatal("backslash-enter should not submit")
	}
}

func TestInput_CursorByteOffsetTracksMiddleOfMultilineUnicode(t *testing.T) {
	m := New()
	m.SetValue("first\nαβ @partial tail")
	for range len([]rune(" @partial tail")) {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	}
	if got, want := m.ValueBeforeCursor(), "first\nαβ"; got != want {
		t.Fatalf("ValueBeforeCursor() = %q, want %q", got, want)
	}
}
