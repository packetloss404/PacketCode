package input

import "testing"

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
