package picker

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/ui/theme"
)

// Item is a single selectable row. Extra is caller-owned and survives
// the round-trip through SelectMsg so handlers can stash a typed
// payload (e.g. a provider.Model) without reflecting on ID.
type Item struct {
	ID     string
	Label  string
	Detail string
	Marker string         // single-rune visual marker (e.g. "●" for active)
	Color  lipgloss.Color // optional tint for Marker + Label prefix
	Extra  any
}

// Loader produces a fresh item list. Used when the picker opens against
// a network-bound source (model list). Nil loader = synchronous open:
// the caller preloads items via SetItems.
type Loader func(ctx context.Context) ([]Item, error)

// SelectMsg is emitted when the user hits Enter on a row. PickerID is
// the id the caller passed to New(), so a single App-level handler can
// route multiple pickers by switching on it.
type SelectMsg struct {
	PickerID string
	Item     Item
}

// CloseMsg is emitted when the user hits Esc. PickerID identifies the
// originating picker for the same reason as SelectMsg.
type CloseMsg struct{ PickerID string }

// itemsLoadedMsg is the internal message the loader tea.Cmd returns.
// The App routes it back into the picker's Update when the picker is
// visible; the picker populates items / loadErr accordingly.
type itemsLoadedMsg struct {
	pickerID string
	items    []Item
	err      error
}

// Model is the picker modal. Construct with New(); show via Open(...).
// Follows the same value-receiver-on-Update convention as approval and
// jobs so it composes cleanly into App.Update.
type Model struct {
	id           string
	visible      bool
	title        string
	filter       string
	items        []Item
	filtered     []int
	cursor       int
	scrollOffset int
	width        int
	height       int
	loading      bool
	loadErr      error
	loader       Loader
}

// New returns a hidden Model with the given id and title. The id lets
// a single SelectMsg / CloseMsg handler disambiguate multiple pickers
// sharing the same App.
func New(id, title string) Model {
	return Model{id: id, title: title}
}

// Open makes the picker visible. If loader is nil the picker is
// presented synchronously: the caller is expected to have called
// SetItems before Open. Otherwise loading=true is set and a tea.Cmd is
// returned that invokes loader on a background goroutine and emits an
// itemsLoadedMsg when it resolves.
func (m *Model) Open(loader Loader) tea.Cmd {
	m.visible = true
	m.loadErr = nil
	m.filter = ""
	m.cursor = 0
	m.scrollOffset = 0
	if loader == nil {
		m.loading = false
		m.loader = nil
		m.rebuildFiltered()
		return nil
	}
	m.loading = true
	m.loader = loader
	m.items = nil
	m.filtered = nil
	return m.loaderCmd()
}

// loaderCmd wraps the current loader into a tea.Cmd. Extracted so the
// `r` retry key can re-invoke the exact same loader without caller
// bookkeeping.
func (m Model) loaderCmd() tea.Cmd {
	loader := m.loader
	id := m.id
	if loader == nil {
		return nil
	}
	return func() tea.Msg {
		items, err := loader(context.Background())
		return itemsLoadedMsg{pickerID: id, items: items, err: err}
	}
}

// SetItems replaces the current item list and rebuilds the filter
// view. Resets cursor to 0 (callers that want to pre-seed the cursor
// call SetActive after).
func (m *Model) SetItems(items []Item) {
	m.items = items
	m.loading = false
	m.loadErr = nil
	m.cursor = 0
	m.scrollOffset = 0
	m.rebuildFiltered()
}

// SetActive moves the cursor to the row whose ID matches the given
// string (no-op if not found). Called after SetItems to highlight the
// currently active provider/model on open.
func (m *Model) SetActive(id string) {
	if id == "" {
		return
	}
	for i, idx := range m.filtered {
		if m.items[idx].ID == id {
			m.cursor = i
			m.ensureCursorVisible()
			return
		}
	}
}

// Hide dismisses the picker. Safe to call when already hidden.
func (m *Model) Hide() { m.visible = false }

// Visible reports whether the picker is currently on-screen.
func (m Model) Visible() bool { return m.visible }

// ID returns the identifier passed to New. Lets the App disambiguate
// which picker is active when intercepting keys before Update.
func (m Model) ID() string { return m.id }

// CursorID returns the ID of the currently-highlighted row, or ""
// when the filtered list is empty. Useful for tests and for callers
// that want to mirror the cursor somewhere else (e.g. a preview).
func (m Model) CursorID() string {
	if len(m.filtered) == 0 {
		return ""
	}
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return ""
	}
	return m.items[m.filtered[m.cursor]].ID
}

// Loading reports whether the picker is currently waiting for its
// async loader to resolve.
func (m Model) Loading() bool { return m.loading }

// LoadErr returns the last loader error, if any. Nil in the success
// and initial states.
func (m Model) LoadErr() error { return m.loadErr }

// Resize stores the terminal dimensions. The picker computes its
// modal width/height off these each View().
func (m *Model) Resize(w, h int) {
	m.width = w
	m.height = h
	m.ensureCursorVisible()
}

// Update routes input while the picker is visible. Ignored entirely
// when hidden so the App can forward messages unconditionally.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.visible {
		return m, nil
	}
	switch msg := msg.(type) {
	case itemsLoadedMsg:
		if msg.pickerID != m.id {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.loadErr = msg.err
			m.items = nil
			m.filtered = nil
			return m, nil
		}
		m.loadErr = nil
		m.items = msg.items
		m.cursor = 0
		m.scrollOffset = 0
		m.rebuildFiltered()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey is Update's KeyMsg branch, factored out for readability.
func (m Model) handleKey(km tea.KeyMsg) (Model, tea.Cmd) {
	key := km.String()

	// Error-state retry: only "r" (lowercase) re-fires the loader.
	if m.loadErr != nil && key == "r" && m.loader != nil {
		m.loadErr = nil
		m.loading = true
		return m, m.loaderCmd()
	}

	switch key {
	case "esc":
		m.visible = false
		id := m.id
		return m, func() tea.Msg { return CloseMsg{PickerID: id} }
	case "enter":
		if m.loading || m.loadErr != nil {
			return m, nil
		}
		if len(m.filtered) == 0 {
			return m, nil
		}
		item := m.items[m.filtered[m.cursor]]
		m.visible = false
		id := m.id
		return m, func() tea.Msg { return SelectMsg{PickerID: id, Item: item} }
	case "up", "ctrl+p", "ctrl+k":
		if m.cursor > 0 {
			m.cursor--
			m.ensureCursorVisible()
		}
		return m, nil
	case "down", "ctrl+n", "ctrl+j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.ensureCursorVisible()
		}
		return m, nil
	case "pgup":
		step := m.halfPage()
		m.cursor -= step
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureCursorVisible()
		return m, nil
	case "pgdown":
		step := m.halfPage()
		m.cursor += step
		if m.cursor > len(m.filtered)-1 {
			m.cursor = len(m.filtered) - 1
			if m.cursor < 0 {
				m.cursor = 0
			}
		}
		m.ensureCursorVisible()
		return m, nil
	case "home":
		m.cursor = 0
		m.ensureCursorVisible()
		return m, nil
	case "end":
		m.cursor = len(m.filtered) - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureCursorVisible()
		return m, nil
	case "backspace":
		if n := len(m.filter); n > 0 {
			runes := []rune(m.filter)
			m.filter = string(runes[:len(runes)-1])
			m.rebuildFiltered()
			m.clampCursor()
		}
		return m, nil
	case "ctrl+u":
		if m.filter != "" {
			m.filter = ""
			m.rebuildFiltered()
			m.clampCursor()
		}
		return m, nil
	case " ", "space":
		// Space is a legitimate filter rune when typing "gpt 4".
		m.filter += " "
		m.rebuildFiltered()
		m.clampCursor()
		return m, nil
	}

	// Printable rune (no control modifier) → append to filter.
	if km.Type == tea.KeyRunes && len(km.Runes) > 0 {
		if isPrintable(km.Runes) {
			m.filter += string(km.Runes)
			m.rebuildFiltered()
			m.clampCursor()
		}
	}
	return m, nil
}

// isPrintable returns true when every rune is printable and not a
// control character. Guards the "any typed rune is filter input"
// branch from accidentally accepting bell or ESC.
func isPrintable(rs []rune) bool {
	for _, r := range rs {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// halfPage returns the pg-up/pg-down step size based on current height.
func (m Model) halfPage() int {
	// Use the viewport height of the list area — roughly height/2 per
	// the spec, clamped to at least 1 so a tiny terminal still moves.
	h := m.listHeight()
	if h < 2 {
		return 1
	}
	return h / 2
}

// clampCursor pulls cursor into the [0, len(filtered)-1] range and
// re-aligns scrollOffset so it remains visible.
func (m *Model) clampCursor() {
	if len(m.filtered) == 0 {
		m.cursor = 0
		m.scrollOffset = 0
		return
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureCursorVisible()
}

// ensureCursorVisible adjusts scrollOffset so cursor falls inside the
// list window. Direct-render scrolling (no viewport wrapper).
func (m *Model) ensureCursorVisible() {
	h := m.listHeight()
	if h <= 0 {
		m.scrollOffset = 0
		return
	}
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+h {
		m.scrollOffset = m.cursor - h + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	maxOffset := len(m.filtered) - h
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
}

// rebuildFiltered rebuilds the filtered-index slice from the current
// filter string. Called whenever items or filter change.
func (m *Model) rebuildFiltered() {
	m.filtered = m.filtered[:0]
	for i, it := range m.items {
		if matches(it, m.filter) {
			m.filtered = append(m.filtered, i)
		}
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Geometry
// ────────────────────────────────────────────────────────────────────────────

// modalWidth clamps terminal_width/2 into [52, 96].
func (m Model) modalWidth() int {
	w := m.width / 2
	if m.width > 0 && w < 52 {
		w = 52
	}
	if w > 96 {
		w = 96
	}
	if w < 52 {
		w = 52
	}
	if m.width > 0 && w > m.width-2 {
		w = m.width - 2
	}
	if w < 20 {
		w = 20
	}
	return w
}

// modalHeight clamps 3 + filtered + 1 into [8, terminal_height/2]. Error
// and loading states always render at the 8 floor.
func (m Model) modalHeight() int {
	if m.loading || m.loadErr != nil {
		return 8
	}
	base := 3 + len(m.filtered) + 1
	if base < 8 {
		base = 8
	}
	cap := 8
	if m.height > 0 {
		cap = m.height / 2
	}
	if cap < 8 {
		cap = 8
	}
	if base > cap {
		base = cap
	}
	return base
}

// listHeight returns the number of rows available for items inside the
// modal after accounting for the border + padding + title + filter line
// + footer.
func (m Model) listHeight() int {
	// modalHeight includes: title (1) + filter (1) + blank (1) + rows +
	// footer (1). Subtract the fixed 4 lines to get the rows budget.
	h := m.modalHeight() - 4
	if h < 1 {
		h = 1
	}
	return h
}

// ────────────────────────────────────────────────────────────────────────────
// View
// ────────────────────────────────────────────────────────────────────────────

// View renders the modal. Returns "" when hidden so the layout can skip
// the overlay slot.
func (m Model) View() string {
	if !m.visible {
		return ""
	}
	outerW := m.modalWidth()
	innerW := outerW - 4 // border (2) + padding (2)
	if innerW < 10 {
		innerW = 10
	}

	title := theme.StyleAccent.Render(m.title)
	cursorChar := theme.StyleDim.Render("▌")
	filterLine := theme.StylePrimary.Render("> ") + theme.StylePrimary.Render(m.filter) + cursorChar

	footer := theme.StyleDim.Render("↑/↓ move · ⏎ select · Esc close")
	if strings.TrimSpace(m.filter) != "" {
		footer += theme.StyleDim.Render(fmt.Sprintf("  · filtering %d/%d", len(m.filtered), len(m.items)))
	}

	var body string
	switch {
	case m.loading:
		body = m.renderLoading(innerW)
	case m.loadErr != nil:
		body = m.renderError(innerW)
		footer = theme.StyleDim.Render("r retry · Esc close")
	default:
		body = m.renderList(innerW)
	}

	content := strings.Join([]string{title, filterLine, body, footer}, "\n")

	frame := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.AccentPrimary).
		Padding(0, 1).
		Width(innerW + 2)
	return frame.Render(content)
}

// renderLoading centers a "loading…" placeholder vertically in the
// body area.
func (m Model) renderLoading(w int) string {
	h := m.listHeight()
	msg := theme.StyleDim.Render("loading…")
	lines := make([]string, 0, h)
	pad := h / 2
	for i := 0; i < pad; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, lipgloss.PlaceHorizontal(w, lipgloss.Center, msg))
	for i := len(lines); i < h; i++ {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// renderError shows the load error in StyleError. The footer is
// replaced by the caller with "r retry · Esc close".
func (m Model) renderError(w int) string {
	h := m.listHeight()
	msg := theme.StyleError.Render("error: " + m.loadErr.Error())
	lines := make([]string, 0, h)
	pad := h / 2
	for i := 0; i < pad; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, truncate(msg, w))
	for i := len(lines); i < h; i++ {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// renderList builds the item rows using scrollOffset + listHeight as
// the visible window. Cursor row gets a bright-surface background.
func (m Model) renderList(w int) string {
	h := m.listHeight()
	if len(m.filtered) == 0 {
		lines := make([]string, 0, h)
		msg := theme.StyleDim.Render("no matches")
		pad := h / 2
		for i := 0; i < pad; i++ {
			lines = append(lines, "")
		}
		lines = append(lines, lipgloss.PlaceHorizontal(w, lipgloss.Center, msg))
		for i := len(lines); i < h; i++ {
			lines = append(lines, "")
		}
		return strings.Join(lines, "\n")
	}

	// Column budgets: marker column is 4 runes ("▶ ● "), label column
	// is min(24, body/3). Detail uses the rest.
	labelW := w / 3
	if labelW > 24 {
		labelW = 24
	}
	if labelW < 8 {
		labelW = 8
	}
	markerW := 4
	detailW := w - markerW - labelW - 1
	if detailW < 4 {
		detailW = 4
	}

	rows := make([]string, 0, h)
	end := m.scrollOffset + h
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	for i := m.scrollOffset; i < end; i++ {
		idx := m.filtered[i]
		it := m.items[idx]
		rows = append(rows, m.renderRow(it, i == m.cursor, markerW, labelW, detailW))
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

// renderRow formats one list row. cursorOn flips the background tint
// so the selected row reads. The marker column always includes the
// cursor arrow (▶) on the cursor row and the item's own marker (if any).
func (m Model) renderRow(it Item, cursorOn bool, markerW, labelW, detailW int) string {
	marker := "  "
	if cursorOn {
		marker = "▶ "
	}
	itemMarker := it.Marker
	if itemMarker == "" {
		itemMarker = " "
	}
	if it.Color != "" {
		itemMarker = lipgloss.NewStyle().Foreground(it.Color).Render(itemMarker)
	}
	// marker (2) + itemMarker (1) + space (1) = 4 runes.
	markerCol := marker + itemMarker + " "

	labelStyle := theme.StylePrimary
	if it.Color != "" {
		labelStyle = lipgloss.NewStyle().Foreground(it.Color)
	}
	label := padOrTrunc(it.Label, labelW)
	labelCol := labelStyle.Render(label)
	detail := theme.StyleSecondary.Render(alignRight(truncate(it.Detail, detailW), detailW))

	line := markerCol + labelCol + " " + detail
	if cursorOn {
		// Highlight whole line with bright-surface background.
		return lipgloss.NewStyle().Background(theme.BaseSurfaceBright).Render(line)
	}
	return line
}

// padOrTrunc returns s padded with spaces or truncated (with ellipsis)
// to exactly w runes. Used for fixed-width columns.
func padOrTrunc(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		if w >= 1 {
			return string(r[:w-1]) + "…"
		}
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}

// truncate clips s to at most w runes, adding an ellipsis on overflow.
func truncate(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return string(r[:w])
	}
	return string(r[:w-1]) + "…"
}

// alignRight right-aligns s inside a w-wide field. If s is already
// longer than w it's returned as-is (truncate is the caller's job).
func alignRight(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return s
	}
	return strings.Repeat(" ", w-len(r)) + s
}
