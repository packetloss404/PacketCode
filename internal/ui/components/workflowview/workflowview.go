// Package workflowview renders a selectable overview of workflow runs and the
// agents inside them. It clones the presentation approach of the agentview
// component: callers pass workflow.RunSnapshot values in and listen for typed
// Bubble Tea messages when the user asks to close, open an agent transcript, or
// cancel a run.
//
// Layout is a flat, scrollable list built from the run → phase → step → agent
// tree. Run header rows are selectable (for cancel); agent rows are selectable
// (for open). Phase and step rows are non-selectable separators.
package workflowview

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	jobspkg "github.com/packetcode/packetcode/internal/jobs"
	"github.com/packetcode/packetcode/internal/ui/theme"
	"github.com/packetcode/packetcode/internal/workflow"
)

// CloseMsg is emitted when the user dismisses the workflow view.
type CloseMsg struct{}

// OpenMsg is emitted when the user requests the full transcript for an agent.
type OpenMsg struct{ JobID string }

// CancelMsg is emitted when the user requests cancellation of a run.
type CancelMsg struct{ RunID string }

type rowKind int

const (
	rowRun rowKind = iota
	rowPhase
	rowStep
	rowAgent
	rowError
)

type row struct {
	kind     rowKind
	runID    string
	runState workflow.RunState
	jobID    string
	text     string
	job      jobspkg.Snapshot
	hasJob   bool
	depth    int
}

// Model is the workflow view list component. It follows the value-return
// Update convention used by the other Bubble Tea components in this repo.
type Model struct {
	visible      bool
	title        string
	runs         []workflow.RunSnapshot
	rows         []row
	cursor       int
	scrollOffset int
	width        int
	height       int
	focusRun     string // run id to scroll to on the next SetRuns
}

// New returns an empty, hidden workflow view.
func New() Model {
	return Model{title: "Workflows", cursor: -1}
}

// Show makes the component visible and replaces its run list.
func (m *Model) Show(runs []workflow.RunSnapshot) {
	m.visible = true
	m.SetRuns(runs)
}

// ShowFocused shows the view scrolled to the given run id.
func (m *Model) ShowFocused(runs []workflow.RunSnapshot, runID string) {
	m.focusRun = runID
	m.Show(runs)
}

// Hide closes the component. Safe to call when already hidden.
func (m *Model) Hide() { m.visible = false }

// Visible reports whether the component should currently be rendered.
func (m Model) Visible() bool { return m.visible }

// Resize stores terminal dimensions for row clipping and scrolling.
func (m *Model) Resize(w, h int) {
	m.width = w
	m.height = h
	m.ensureCursorVisible()
}

// SetRuns replaces the displayed runs, preserving selection by row identity
// when possible.
func (m *Model) SetRuns(runs []workflow.RunSnapshot) {
	selKind, selID := m.selectionKey()
	m.runs = append(m.runs[:0], runs...)
	m.rebuildRows()
	if m.focusRun != "" && m.selectRun(m.focusRun) {
		m.focusRun = ""
		return
	}
	if selID != "" && m.selectByKey(selKind, selID) {
		return
	}
	m.cursor = m.firstSelectableRow()
	m.ensureCursorVisible()
}

func (m Model) selectionKey() (rowKind, string) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return rowRun, ""
	}
	r := m.rows[m.cursor]
	switch r.kind {
	case rowAgent:
		return rowAgent, r.jobID
	case rowRun:
		return rowRun, r.runID
	}
	return rowRun, ""
}

func (m *Model) selectByKey(kind rowKind, id string) bool {
	for i, r := range m.rows {
		if r.kind != kind {
			continue
		}
		if kind == rowAgent && r.jobID == id {
			m.cursor = i
			m.ensureCursorVisible()
			return true
		}
		if kind == rowRun && r.runID == id {
			m.cursor = i
			m.ensureCursorVisible()
			return true
		}
	}
	return false
}

func (m *Model) selectRun(id string) bool {
	return m.selectByKey(rowRun, id)
}

// Update handles list navigation and emits action messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.visible {
		return m, nil
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "esc", "q", "Q":
		m.visible = false
		return m, emit(CloseMsg{})
	case "up", "k", "ctrl+p":
		m.move(-1)
		return m, nil
	case "down", "j", "ctrl+n":
		m.move(1)
		return m, nil
	case "home", "g":
		m.cursor = m.firstSelectableRow()
		m.ensureCursorVisible()
		return m, nil
	case "end", "G":
		m.cursor = m.lastSelectableRow()
		m.ensureCursorVisible()
		return m, nil
	case "pgup":
		m.page(-1)
		return m, nil
	case "pgdown", " ":
		m.page(1)
		return m, nil
	case "enter", "o", "O":
		if r, ok := m.selectedRow(); ok && r.kind == rowAgent && r.jobID != "" {
			return m, emit(OpenMsg{JobID: r.jobID})
		}
		return m, nil
	case "c", "C":
		if r, ok := m.selectedRow(); ok {
			if r.kind == rowRun && !r.runState.IsTerminal() {
				return m, emit(CancelMsg{RunID: r.runID})
			}
			if r.kind == rowAgent && r.runID != "" {
				// Cancelling from an agent row cancels its parent run.
				return m, emit(CancelMsg{RunID: r.runID})
			}
		}
		return m, nil
	}
	return m, nil
}

func (m Model) selectedRow() (row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return row{}, false
	}
	return m.rows[m.cursor], true
}

// View renders the run/phase/step/agent tree.
func (m Model) View() string {
	if !m.visible {
		return ""
	}
	w := m.modalWidth()
	innerW := w - 2
	if innerW < 20 {
		innerW = 20
	}

	title := theme.StyleAccent.Bold(true).Render("⚡ packetcode workflows")
	meta := theme.StyleSecondary.Render(fmt.Sprintf("%d runs · fan-out agents and ordered phases", len(m.runs)))
	header := title + "\n" + meta
	body := m.renderRows(innerW)
	footer := theme.StyleDim.Render(m.footerText())

	content := strings.Join([]string{header, "", body, "", footer}, "\n")
	return lipgloss.NewStyle().Padding(0, 1).Width(w).Render(content)
}

func (m Model) footerText() string {
	parts := []string{"↑/↓ move"}
	if r, ok := m.selectedRow(); ok {
		if r.kind == rowAgent {
			parts = append(parts, "enter open agent")
		}
		if (r.kind == rowRun || r.kind == rowAgent) && !r.runState.IsTerminal() {
			parts = append(parts, "c cancel run")
		}
	}
	parts = append(parts, "Esc close")
	return strings.Join(parts, " · ")
}

func emit(msg tea.Msg) tea.Cmd { return func() tea.Msg { return msg } }

func (m *Model) rebuildRows() {
	m.rows = m.rows[:0]
	for _, run := range m.runs {
		m.rows = append(m.rows, row{
			kind:     rowRun,
			runID:    run.ID,
			runState: run.State,
			text:     runHeaderText(run),
		})
		if strings.TrimSpace(run.Err) != "" {
			m.rows = append(m.rows, row{kind: rowError, runID: run.ID, text: "error: " + run.Err, depth: 1})
		}
		for _, ph := range run.Phases {
			m.rows = append(m.rows, row{kind: rowPhase, runID: run.ID, text: ph.Name, depth: 1})
			for _, st := range ph.Steps {
				m.rows = append(m.rows, row{kind: rowStep, runID: run.ID, text: stepHeaderText(st), depth: 2})
				if strings.TrimSpace(st.Err) != "" {
					m.rows = append(m.rows, row{kind: rowError, runID: run.ID, text: st.Err, depth: 3})
				}
				for _, ag := range st.Agents {
					role := ""
					if ag.Role == "verifier" {
						role = fmt.Sprintf("verifier a%d · ", ag.Attempt)
					} else if ag.Attempt > 1 {
						role = fmt.Sprintf("work a%d · ", ag.Attempt)
					}
					m.rows = append(m.rows, row{
						kind:     rowAgent,
						runID:    run.ID,
						runState: run.State,
						jobID:    ag.JobID,
						job:      ag.Job,
						hasJob:   ag.HasJob,
						depth:    3,
						text:     role,
					})
				}
			}
		}
	}
	if len(m.rows) == 0 {
		m.cursor = -1
		m.scrollOffset = 0
	}
}

func runHeaderText(run workflow.RunSnapshot) string {
	name := run.Workflow
	if name == "" {
		name = "(workflow)"
	}
	return fmt.Sprintf("%s  %s", run.ID, name)
}

func stepHeaderText(st workflow.StepSnapshot) string {
	mode := string(st.Mode)
	if mode == "" {
		mode = "single"
	}
	verification := string(st.Verification)
	if verification == "" {
		verification = string(workflow.VerificationUnverified)
	}
	meta := []string{mode, verification}
	if st.Attempts > 1 {
		meta = append(meta, fmt.Sprintf("%d attempts", st.Attempts))
	}
	return fmt.Sprintf("%s [%s]", st.Name, strings.Join(meta, " · "))
}

func isSelectable(r row) bool { return r.kind == rowRun || r.kind == rowAgent }

func (m Model) firstSelectableRow() int {
	for i, r := range m.rows {
		if isSelectable(r) {
			return i
		}
	}
	return -1
}

func (m Model) lastSelectableRow() int {
	for i := len(m.rows) - 1; i >= 0; i-- {
		if isSelectable(m.rows[i]) {
			return i
		}
	}
	return -1
}

func (m *Model) move(delta int) {
	if m.cursor < 0 {
		m.cursor = m.firstSelectableRow()
		m.ensureCursorVisible()
		return
	}
	for i := m.cursor + delta; i >= 0 && i < len(m.rows); i += delta {
		if isSelectable(m.rows[i]) {
			m.cursor = i
			m.ensureCursorVisible()
			return
		}
	}
}

func (m *Model) page(direction int) {
	steps := m.listHeight() / 2
	if steps < 1 {
		steps = 1
	}
	for i := 0; i < steps; i++ {
		before := m.cursor
		m.move(direction)
		if m.cursor == before {
			break
		}
	}
}

func (m *Model) ensureCursorVisible() {
	h := m.listHeight()
	if h <= 0 || m.cursor < 0 {
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
	maxOffset := len(m.rows) - h
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
}

func (m Model) modalWidth() int {
	w := m.width
	if w <= 0 {
		w = 96
	}
	if w < 44 {
		w = 44
	}
	return w
}

func (m Model) modalHeight() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	if h > 34 {
		h = 34
	}
	if h < 8 {
		h = 8
	}
	return h
}

func (m Model) listHeight() int {
	h := m.modalHeight() - 8
	if h < 1 {
		return 1
	}
	return h
}

func (m Model) renderRows(w int) string {
	h := m.listHeight()
	if len(m.rows) == 0 {
		lines := make([]string, 0, h)
		msg := theme.StyleDim.Render("no workflow runs - /workflows run <name>")
		pad := h / 2
		for i := 0; i < pad; i++ {
			lines = append(lines, "")
		}
		lines = append(lines, lipgloss.PlaceHorizontal(w, lipgloss.Center, msg))
		for len(lines) < h {
			lines = append(lines, "")
		}
		return strings.Join(lines, "\n")
	}

	rows := make([]string, 0, h)
	end := m.scrollOffset + h
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.scrollOffset; i < end; i++ {
		rows = append(rows, m.renderRow(m.rows[i], i == m.cursor, w))
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

func (m Model) renderRow(r row, selected bool, w int) string {
	switch r.kind {
	case rowRun:
		return m.renderRunRow(r, selected, w)
	case rowPhase:
		return m.renderPhaseRow(r, w)
	case rowStep:
		return m.renderStepRow(r, w)
	case rowError:
		return indentDim("  "+strings.Repeat("  ", r.depth)+"⚠ "+r.text, w)
	case rowAgent:
		return m.renderAgentRow(r, selected, w)
	}
	return ""
}

func (m Model) renderRunRow(r row, selected bool, w int) string {
	cursor := "  "
	if selected {
		cursor = "▶ "
	}
	state := renderRunState(r.runState)
	line := cursor + theme.StyleAccent.Bold(true).Render(padOrTrunc(r.text, 40)) + " " + state
	line = truncate(line, w)
	if selected {
		line = lipgloss.NewStyle().Background(theme.BaseSurfaceBright).Render(line)
	}
	return line
}

func (m Model) renderPhaseRow(r row, w int) string {
	label := "  phase " + r.text + " "
	line := theme.StyleSecondary.Bold(true).Render(label)
	pad := max(0, w-lipgloss.Width(label))
	return line + theme.StyleDim.Render(strings.Repeat("─", pad))
}

func (m Model) renderStepRow(r row, w int) string {
	return theme.StyleSecondary.Render("    step " + truncate(r.text, max(0, w-9)))
}

func (m Model) renderAgentRow(r row, selected bool, w int) string {
	cursor := "  "
	if selected {
		cursor = "▶ "
	}
	indent := "      "
	if !r.hasJob {
		line := cursor + indent + theme.StyleDim.Render(r.text+"agent "+padOrTrunc(shortID(r.jobID), 8)+" (pending)")
		return truncate(line, w)
	}
	j := r.job
	id := theme.StyleAccent.Render(padOrTrunc(shortID(j.ID), 8))
	state := renderJobState(j.State.String(), 10)
	prov := providerLabel(j.Provider, j.Model)
	// Background tokens are cumulative input/output API usage, not the
	// foreground conversation's context-window occupancy.
	tokens := fmt.Sprintf("api %d/%d", j.Tokens.Input, j.Tokens.Output)
	cost := fmt.Sprintf("$%.4f", j.CostUSD)

	fixedW := lipgloss.Width(cursor) + len(indent) + 8 + 1 + 10 + 1 + 20 + 1 + 11 + 1 + 8 + 1
	msgW := w - fixedW
	if msgW < 8 {
		msgW = 8
	}
	role := rRoleLabel(r)
	msg := truncate(strings.TrimSpace(role+jobMessage(j)), msgW)

	line := cursor + indent + strings.Join([]string{
		id,
		state,
		padOrTrunc(prov, 20),
		padOrTrunc(tokens, 11),
		padOrTrunc(cost, 8),
		msg,
	}, " ")
	line = truncate(line, w)
	if selected {
		line = lipgloss.NewStyle().Background(theme.BaseSurfaceBright).Render(line)
	}
	return line
}

func rRoleLabel(r row) string {
	// Role/attempt live on AgentSnapshot, but rows intentionally contain only
	// rendering data. rebuildRows prefixes them into text for pending rows and
	// stores the same prefix in this lightweight field.
	return r.text
}

func indentDim(s string, w int) string {
	return theme.StyleDim.Render(truncate(s, w))
}

// ─────────────────────────────────────────────────────────────────────────
// Rendering helpers (cloned from agentview)
// ─────────────────────────────────────────────────────────────────────────

func renderRunState(s workflow.RunState) string {
	label := padOrTrunc(string(s), 10)
	switch s {
	case workflow.RunRunning:
		return lipgloss.NewStyle().Foreground(theme.Info).Render(label)
	case workflow.RunPending:
		return lipgloss.NewStyle().Foreground(theme.Warning).Render(label)
	case workflow.RunCompleted:
		return lipgloss.NewStyle().Foreground(theme.Success).Render(label)
	case workflow.RunFailed:
		return lipgloss.NewStyle().Foreground(theme.Error).Render(label)
	case workflow.RunCancelled:
		return theme.StyleSecondary.Render(label)
	default:
		return theme.StyleDim.Render(label)
	}
}

func renderJobState(s string, w int) string {
	state := strings.ToLower(strings.TrimSpace(s))
	if state == "" {
		state = "unknown"
	}
	label := padOrTrunc(state, w)
	switch state {
	case "running":
		return lipgloss.NewStyle().Foreground(theme.Info).Render(label)
	case "queued":
		return lipgloss.NewStyle().Foreground(theme.Warning).Render(label)
	case "completed", "done", "success", "succeeded":
		return lipgloss.NewStyle().Foreground(theme.Success).Render(label)
	case "failed", "error":
		return lipgloss.NewStyle().Foreground(theme.Error).Render(label)
	case "cancelled", "canceled":
		return theme.StyleSecondary.Render(label)
	default:
		return theme.StyleDim.Render(label)
	}
}

func providerLabel(provider, model string) string {
	if provider == "" {
		return model
	}
	if model == "" {
		return provider
	}
	return provider + "/" + model
}

func jobMessage(j jobspkg.Snapshot) string {
	if j.NeedsApproval {
		return "needs approval: " + nonEmpty(j.LastMessage, j.Prompt)
	}
	if j.NeedsInput {
		return "needs input: " + nonEmpty(j.LastMessage, j.Prompt)
	}
	if strings.EqualFold(strings.TrimSpace(j.LastMessage), "started") && j.Prompt != "" {
		return j.Prompt
	}
	if j.LastMessage != "" {
		return j.LastMessage
	}
	if j.Summary != "" {
		return j.Summary
	}
	if j.Error != "" {
		return j.Error
	}
	return j.Prompt
}

func nonEmpty(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func padOrTrunc(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		if w <= 1 {
			return string(r[:w])
		}
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

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
