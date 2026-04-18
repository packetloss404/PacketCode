package autocomplete

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// testEntries returns a small fixed entry list the tests share. Mirrors
// the shape buildAutocompleteEntries produces but is independent of
// keymap.SlashCommands so this file stays app-free.
func testEntries() []Entry {
	return []Entry{
		{Verb: "spawn", Usage: "/spawn <prompt>", Desc: "Spawn a background agent"},
		{Verb: "jobs", Usage: "/jobs", Desc: "List background jobs"},
		{Verb: "cancel", Usage: "/cancel <id|all>", Desc: "Cancel a job"},
		{Verb: "provider", Usage: "/provider [slug]", Desc: "List providers or switch active"},
		{Verb: "model", Usage: "/model [id]", Desc: "List models or switch active"},
		{Verb: "sessions", Usage: "/sessions", Desc: "List sessions (resume|delete subcommands)"},
		{Verb: "undo", Usage: "/undo", Desc: "Undo the most recent file change"},
		{Verb: "compact", Usage: "/compact [--keep N]", Desc: "Summarise older messages to reclaim context"},
		{Verb: "cost", Usage: "/cost", Desc: "Show cost breakdown (reset --yes to clear)"},
		{Verb: "trust", Usage: "/trust [on|off]", Desc: "Toggle auto-approval of destructive tools"},
		{Verb: "help", Usage: "/help", Desc: "Show this help message"},
		{Verb: "clear", Usage: "/clear", Desc: "Clear the transcript pane"},
	}
}

// newSized returns an opened Model with a terminal-like width set so
// View() renders something non-trivial.
func newSized(filter string) Model {
	m := New(testEntries())
	m.SetWidth(80)
	m.Open(filter)
	return m
}

func TestAutocomplete_NewHidden(t *testing.T) {
	m := New(testEntries())
	if m.Visible() {
		t.Fatalf("New() should return a hidden model")
	}
	if m.View() != "" {
		t.Fatalf("hidden View() must be empty, got %q", m.View())
	}
}

func TestAutocomplete_OpenEmptyFilterShowsAllEntries(t *testing.T) {
	m := New(testEntries())
	m.SetWidth(80)
	m.Open("")
	if !m.Visible() {
		t.Fatalf("Open should flip visible=true")
	}
	if got, want := m.Count(), len(testEntries()); got != want {
		t.Fatalf("Count = %d, want %d", got, want)
	}
}

func TestAutocomplete_OpenWithFilterNarrows(t *testing.T) {
	m := newSized("sp")
	if m.Count() != 1 {
		t.Fatalf("Count = %d, want 1 (spawn only)", m.Count())
	}
	if got := m.SelectedVerb(); got != "spawn" {
		t.Fatalf("SelectedVerb = %q, want spawn", got)
	}
}

func TestAutocomplete_PrefixMatchesBeatSubstring(t *testing.T) {
	// "co" prefixes "compact" and "cost" (tier 1) but also appears
	// inside "reclaim context" (cost's desc -- wait: "compact" has
	// "reclaim context" too). More direct: "c" matches verbs c*
	// first, then any desc containing c. With "co", tier 1 is
	// {compact, cost}; anything in tier 2 must come after them.
	m := newSized("co")
	if m.Count() == 0 {
		t.Fatalf("Count should be > 0")
	}
	// First two rows must be the verb-prefix hits, alphabetical.
	first := m.entries[m.filtered[0]].Verb
	if first != "compact" {
		t.Fatalf("first row = %q, want compact (prefix tier)", first)
	}
	// Cursor should start on the best match.
	if got := m.SelectedVerb(); got != "compact" {
		t.Fatalf("SelectedVerb = %q, want compact", got)
	}
}

func TestAutocomplete_AlphabeticalWithinTier(t *testing.T) {
	// Tier 1 ("c" prefix): cancel, clear, compact, cost → alphabetical.
	m := newSized("c")
	want := []string{"cancel", "clear", "compact", "cost"}
	if m.Count() < len(want) {
		t.Fatalf("Count = %d, want >= %d", m.Count(), len(want))
	}
	for i, w := range want {
		got := m.entries[m.filtered[i]].Verb
		if got != w {
			t.Fatalf("row %d = %q, want %q (alphabetical)", i, got, w)
		}
	}
}

func TestAutocomplete_NoMatchesHidesView(t *testing.T) {
	m := newSized("zzz-nothing-matches")
	if m.Count() != 0 {
		t.Fatalf("Count = %d, want 0", m.Count())
	}
	// Still visible (App controls visibility), but View is empty so
	// the layout silently drops the row.
	if !m.Visible() {
		t.Fatalf("Visible should still be true — App controls close")
	}
	if m.View() != "" {
		t.Fatalf("View() with zero matches must be empty")
	}
}

func TestAutocomplete_CloseHides(t *testing.T) {
	m := newSized("")
	m.Close()
	if m.Visible() {
		t.Fatalf("Visible should be false after Close")
	}
	if m.View() != "" {
		t.Fatalf("View() after Close must be empty")
	}
}

func TestAutocomplete_SetFilterResetsCursor(t *testing.T) {
	m := newSized("")
	// Walk cursor down a few rows.
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.cursor == 0 {
		t.Fatalf("precondition: cursor should have moved")
	}
	m.SetFilter("h")
	if m.cursor != 0 {
		t.Fatalf("SetFilter should reset cursor to 0, got %d", m.cursor)
	}
}

func TestAutocomplete_UpDownCursor(t *testing.T) {
	m := newSized("")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("down once: cursor = %d, want 1", m.cursor)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Fatalf("up once: cursor = %d, want 0", m.cursor)
	}
	// Up at 0 stays at 0.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Fatalf("up at top: cursor = %d, want 0 (clamped)", m.cursor)
	}
	// Walk to the bottom; another down clamps.
	for i := 0; i < 100; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.cursor != m.Count()-1 {
		t.Fatalf("down past end: cursor = %d, want %d", m.cursor, m.Count()-1)
	}
}

func TestAutocomplete_CtrlNCtrlP(t *testing.T) {
	m := newSized("")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if m.cursor != 1 {
		t.Fatalf("ctrl+n: cursor = %d, want 1", m.cursor)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if m.cursor != 0 {
		t.Fatalf("ctrl+p: cursor = %d, want 0", m.cursor)
	}
}

func TestAutocomplete_CtrlJCtrlK(t *testing.T) {
	m := newSized("")
	// Ctrl+J is represented via runes? Bubbletea treats ctrl+j as a
	// distinct key. Use a constructed KeyMsg.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if m.cursor != 1 {
		t.Fatalf("ctrl+j: cursor = %d, want 1", m.cursor)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.cursor != 0 {
		t.Fatalf("ctrl+k: cursor = %d, want 0", m.cursor)
	}
}

func TestAutocomplete_UpdateIgnoredWhenHidden(t *testing.T) {
	m := New(testEntries())
	m.SetWidth(80)
	before := m.cursor
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != before {
		t.Fatalf("Update while hidden should be a no-op")
	}
}

func TestAutocomplete_UpdateIgnoresNonKeyMsgs(t *testing.T) {
	m := newSized("")
	before := m.cursor
	// Drive some non-key message through Update.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.cursor != before {
		t.Fatalf("non-key message must not move cursor")
	}
}

func TestAutocomplete_UpdateDoesNotHandleTabEnterEsc(t *testing.T) {
	m := newSized("")
	before := m.cursor
	for _, km := range []tea.KeyMsg{
		{Type: tea.KeyTab},
		{Type: tea.KeyEnter},
		{Type: tea.KeyEsc},
	} {
		m, _ = m.Update(km)
	}
	if m.cursor != before {
		t.Fatalf("Tab/Enter/Esc must be ignored by the component (App handles them)")
	}
	if !m.Visible() {
		t.Fatalf("Esc must not hide the component — App controls visibility")
	}
}

func TestAutocomplete_SelectedVerbEmptyWhenNoMatches(t *testing.T) {
	m := newSized("zzz-nothing")
	if got := m.SelectedVerb(); got != "" {
		t.Fatalf("SelectedVerb with no matches = %q, want empty", got)
	}
}

func TestAutocomplete_SelectedVerbMatchesCursor(t *testing.T) {
	m := newSized("")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := m.SelectedVerb()
	want := m.entries[m.filtered[1]].Verb
	if got != want {
		t.Fatalf("SelectedVerb after down = %q, want %q", got, want)
	}
}

func TestAutocomplete_ViewContainsUsageAndDesc(t *testing.T) {
	m := newSized("spawn")
	out := m.View()
	if !strings.Contains(out, "/spawn") {
		t.Fatalf("View should contain usage /spawn; got:\n%s", out)
	}
	if !strings.Contains(out, "background agent") {
		t.Fatalf("View should contain description; got:\n%s", out)
	}
}

func TestAutocomplete_ViewShowsCursorRowHighlight(t *testing.T) {
	m := newSized("")
	out := m.View()
	// The marker glyph lives on the cursor row.
	if !strings.Contains(out, "▶") {
		t.Fatalf("View should render cursor marker ▶ on the cursor row; got:\n%s", out)
	}
}

func TestAutocomplete_ViewMoreFooterWhenOverflow(t *testing.T) {
	// Empty filter → 12 entries, max rows = 6 → 6 hidden → "+6 more".
	m := newSized("")
	out := m.View()
	if !strings.Contains(out, "more") {
		t.Fatalf("View with > 6 rows should include an overflow footer; got:\n%s", out)
	}
}

func TestAutocomplete_WidthClamp(t *testing.T) {
	m := New(testEntries())
	// Terminal too narrow → clamp floor is 40.
	m.SetWidth(10)
	if got := m.clampedWidth(); got != 40 {
		t.Fatalf("clampedWidth(10) = %d, want 40", got)
	}
	// Terminal wide → clamp ceiling is 60.
	m.SetWidth(200)
	if got := m.clampedWidth(); got != 60 {
		t.Fatalf("clampedWidth(200) = %d, want 60", got)
	}
	// Mid-range — terminal width minus 4.
	m.SetWidth(50)
	if got := m.clampedWidth(); got != 46 {
		t.Fatalf("clampedWidth(50) = %d, want 46", got)
	}
}

func TestAutocomplete_SetFilterStripsLeadingSlash(t *testing.T) {
	// Both "/sp" and "sp" should converge to the same filtered list:
	// the leading "/" is stripped inside rebuild() before comparison.
	a := newSized("sp")
	b := newSized("/sp")
	if a.Count() != b.Count() {
		t.Fatalf("Count with slash vs without differs: %d vs %d", b.Count(), a.Count())
	}
	if a.SelectedVerb() != b.SelectedVerb() {
		t.Fatalf("SelectedVerb differs: %q vs %q", b.SelectedVerb(), a.SelectedVerb())
	}
}
