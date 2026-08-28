// Package agentview renders a selectable overview of background-agent jobs.
//
// The component is presentation-only: callers pass jobs.Snapshot values in and
// listen for typed Bubble Tea messages when the user asks to close, peek, open,
// cancel, or inject a job result.
package agentview

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	jobspkg "github.com/packetcode/packetcode/internal/jobs"
	"github.com/packetcode/packetcode/internal/ui/theme"
)

const (
	StateQueued    = "queued"
	StateRunning   = "running"
	StateCompleted = "completed"
	StateFailed    = "failed"
	StateCancelled = "cancelled"
	StateAbandoned = "abandoned"
)

// Job is the display projection Agent View needs. App code can map this from
// jobs.Snapshot without coupling this component to manager internals.
type Job struct {
	ID, ParentJobID, Prompt, Provider, Model string
	ComputerID, ComputerName, WorkingDir     string
	State                                    string
	AbandonCause                             string
	ResultStatus                             string
	// Todos is the agent's own plan, when it kept one. Carried as plain
	// counts plus the live item rather than the whole list: a job row has one
	// line to spare, and "what is it doing now" is the question the list is
	// being read to answer.
	TodosTotal, TodosDone            int
	TodoCurrent                      string
	Summary, Error                   string
	LastActivity, LastMessage        string
	WorktreePath, WorktreeBranch     string
	WorktreeBase, WorktreeNote       string
	Artifacts                        []jobspkg.Artifact
	CreatedAt, UpdatedAt, FinishedAt time.Time
	// Tokens are cumulative API usage across all turns of this background
	// job. They are billing totals, not foreground context-window occupancy.
	Tokens                    struct{ Input, Output int }
	CostUSD                   float64
	Depth                     int
	NeedsInput, NeedsApproval bool
}

// CloseMsg is emitted when the user dismisses the agent view.
type CloseMsg struct{}

// PeekMsg is emitted when the user requests a lightweight preview of a job.
type PeekMsg struct{ JobID string }

// OpenMsg is emitted when the user requests the full job view.
type OpenMsg struct{ JobID string }

// CancelMsg is emitted when the user requests cancellation for a job.
type CancelMsg struct{ JobID string }

// InjectMsg is emitted when the user requests injecting a job result into the
// foreground conversation context.
type InjectMsg struct{ JobID string }

// IgnoreMsg is emitted when the user dismisses a terminal job result without
// injecting it into the foreground conversation.
type IgnoreMsg struct{ JobID string }

type group int

// Ordinals matter: rebuildRows keeps every group ≤ groupCompleted on screen
// even when empty, and hides the rest. groupAbandoned belongs on the hidden
// side — an always-visible "Abandoned" heading would imply lost work is a
// routine part of the lifecycle.
const (
	groupNeedsInput group = iota
	groupActive
	groupCompleted
	groupFailed
	groupCancelled
	groupAbandoned
)

type rowKind int

const (
	rowHeader rowKind = iota
	rowJob
)

type row struct {
	kind  rowKind
	group group
	job   Job
}

// Model is the Agent View list component. It follows the existing value-return
// Update convention used by the other Bubble Tea components in this repo.
type Model struct {
	visible      bool
	title        string
	jobs         []Job
	rows         []row
	cursor       int
	scrollOffset int
	width        int
	height       int
}

// New returns an empty, hidden Agent View.
func New() Model {
	return Model{title: "Agent View", cursor: -1}
}

// Show makes the component visible and replaces its job list. It accepts either
// []Job or []jobs.Snapshot so callers can use the component directly from the
// manager while tests can build lightweight display rows.
func (m *Model) Show(items any) {
	m.visible = true
	m.SetJobs(items)
}

// Hide closes the component. It is safe to call when already hidden.
func (m *Model) Hide() { m.visible = false }

// Visible reports whether the component should currently be rendered.
func (m Model) Visible() bool { return m.visible }

// Resize stores terminal dimensions for row clipping and scrolling.
func (m *Model) Resize(w, h int) {
	m.width = w
	m.height = h
	m.ensureCursorVisible()
}

// SetJobs replaces the displayed jobs. Selection is preserved by job ID when
// possible, otherwise it moves to the first selectable row.
func (m *Model) SetJobs(items any) {
	selected := m.SelectedID()
	jobs := normalizeJobs(items)
	m.jobs = append(m.jobs[:0], jobs...)
	m.rebuildRows()
	if selected != "" && m.selectID(selected) {
		return
	}
	m.cursor = m.firstJobRow()
	m.ensureCursorVisible()
}

func normalizeJobs(items any) []Job {
	switch v := items.(type) {
	case nil:
		return nil
	case []Job:
		out := make([]Job, len(v))
		copy(out, v)
		return out
	case []jobspkg.Snapshot:
		out := make([]Job, len(v))
		for i, s := range v {
			out[i] = fromSnapshot(s)
		}
		return out
	default:
		return nil
	}
}

func fromSnapshot(s jobspkg.Snapshot) Job {
	j := Job{
		ID:             s.ID,
		ParentJobID:    s.ParentJobID,
		Prompt:         s.Prompt,
		Provider:       s.Provider,
		Model:          s.Model,
		ComputerID:     s.ComputerID,
		ComputerName:   s.ComputerName,
		WorkingDir:     s.WorkingDir,
		State:          s.State.String(),
		AbandonCause:   string(s.AbandonCause),
		TodosTotal:     len(s.Todos),
		TodosDone:      countTodosDone(s.Todos),
		TodoCurrent:    currentTodo(s.Todos),
		ResultStatus:   s.ResultStatus.String(),
		Summary:        s.Summary,
		Error:          s.Error,
		LastActivity:   s.LastActivity,
		LastMessage:    s.LastMessage,
		WorktreePath:   s.WorktreePath,
		WorktreeBranch: s.WorktreeBranch,
		WorktreeBase:   s.WorktreeBase,
		WorktreeNote:   s.WorktreeNote,
		Artifacts:      s.Artifacts,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
		FinishedAt:     s.FinishedAt,
		CostUSD:        s.CostUSD,
		Depth:          s.Depth,
		NeedsInput:     s.NeedsInput,
		NeedsApproval:  s.NeedsApproval,
	}
	j.Tokens.Input = s.Tokens.Input
	j.Tokens.Output = s.Tokens.Output
	return j
}

// SelectedID returns the ID under the cursor, or "" when there is no selectable
// row.
func (m Model) SelectedID() string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return ""
	}
	r := m.rows[m.cursor]
	if r.kind != rowJob {
		return ""
	}
	return r.job.ID
}

func (m Model) selectedJob() (Job, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return Job{}, false
	}
	r := m.rows[m.cursor]
	if r.kind != rowJob {
		return Job{}, false
	}
	return r.job, true
}

// Update handles list navigation and emits action messages for the selected
// job.
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
		m.cursor = m.firstJobRow()
		m.ensureCursorVisible()
		return m, nil
	case "end", "G":
		m.cursor = m.lastJobRow()
		m.ensureCursorVisible()
		return m, nil
	case "pgup":
		m.page(-1)
		return m, nil
	case "pgdown", " ":
		m.page(1)
		return m, nil
	case "p", "P":
		return m, m.emitForSelection(func(id string) tea.Msg { return PeekMsg{JobID: id} })
	case "enter", "o", "O":
		return m, m.emitForSelection(func(id string) tea.Msg { return OpenMsg{JobID: id} })
	case "c", "C":
		if job, ok := m.selectedJob(); !ok || !canCancel(job) {
			return m, nil
		}
		return m, m.emitForSelection(func(id string) tea.Msg { return CancelMsg{JobID: id} })
	case "i", "I":
		if job, ok := m.selectedJob(); !ok || !canDecideResult(job) {
			return m, nil
		}
		return m, m.emitForSelection(func(id string) tea.Msg { return InjectMsg{JobID: id} })
	case "x", "X", "d", "D":
		if job, ok := m.selectedJob(); !ok || !canDecideResult(job) {
			return m, nil
		}
		return m, m.emitForSelection(func(id string) tea.Msg { return IgnoreMsg{JobID: id} })
	}
	return m, nil
}

// View renders the grouped job table. Returns "" while hidden so callers can
// wire it into an overlay slot without extra branching.
func (m Model) View() string {
	if !m.visible {
		return ""
	}
	w := m.modalWidth()
	innerW := w - 2
	if innerW < 20 {
		innerW = 20
	}

	waiting, working, completed := m.groupCounts()
	title := theme.StyleAccent.Bold(true).Render("⚡ packetcode agents")
	meta := theme.StyleSecondary.Render(fmt.Sprintf("%d awaiting input · %d working · %d completed", waiting, working, completed))
	header := title + "\n" + meta
	body := m.renderRows(innerW)
	footer := theme.StyleDim.Render(m.footerText())

	content := strings.Join([]string{header, "", body, "", footer}, "\n")
	return lipgloss.NewStyle().Padding(0, 1).Width(w).Render(content)
}

func (m Model) groupCounts() (waiting, working, completed int) {
	for _, job := range m.jobs {
		switch groupForJob(job) {
		case groupNeedsInput:
			waiting++
		case groupActive:
			working++
		default:
			completed++
		}
	}
	return waiting, working, completed
}

func (m Model) footerText() string {
	parts := []string{"↑/↓ move", "p peek", "enter open", "n new task"}
	if job, ok := m.selectedJob(); ok {
		if canCancel(job) {
			parts = append(parts, "c cancel")
		}
		if canDecideResult(job) {
			parts = append(parts, "i inject", "x ignore")
		}
	}
	parts = append(parts, "Esc return")
	return strings.Join(parts, " · ")
}

func emit(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}

func (m Model) emitForSelection(fn func(string) tea.Msg) tea.Cmd {
	id := m.SelectedID()
	if id == "" {
		return nil
	}
	return emit(fn(id))
}

func (m *Model) rebuildRows() {
	groups := map[group][]Job{}
	for _, j := range m.jobs {
		g := groupForJob(j)
		groups[g] = append(groups[g], j)
	}

	// Every group must be listed: a group missing from this slice has no rows
	// built for it, so its jobs disappear from the view entirely.
	order := []group{groupNeedsInput, groupActive, groupCompleted, groupFailed, groupCancelled, groupAbandoned}
	m.rows = m.rows[:0]
	for _, g := range order {
		items := groups[g]
		// Claude Code keeps the three primary lifecycle sections visible even
		// when empty, which makes the screen stable as agents move between them.
		if len(items) == 0 && g > groupCompleted {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			return newestTime(items[i]).After(newestTime(items[j]))
		})
		m.rows = append(m.rows, row{kind: rowHeader, group: g})
		for _, job := range items {
			m.rows = append(m.rows, row{kind: rowJob, group: g, job: job})
		}
	}
	if len(m.rows) == 0 {
		m.cursor = -1
		m.scrollOffset = 0
	}
}

func groupForJob(j Job) group {
	if j.NeedsInput || j.NeedsApproval {
		return groupNeedsInput
	}
	return groupForState(j.State)
}

func groupForState(s string) group {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case StateCompleted, "done", "success", "succeeded":
		return groupCompleted
	case StateFailed, "error":
		return groupFailed
	case StateCancelled, "canceled":
		return groupCancelled
	case StateAbandoned:
		return groupAbandoned
	default:
		return groupActive
	}
}

func groupLabel(g group) string {
	switch g {
	case groupNeedsInput:
		return "Needs input"
	case groupActive:
		return "Working"
	case groupCompleted:
		return "Completed"
	case groupFailed:
		return "Failed"
	case groupCancelled:
		return "Cancelled"
	case groupAbandoned:
		return "Abandoned"
	default:
		return "Jobs"
	}
}

func groupDescription(g group) string {
	switch g {
	case groupNeedsInput:
		return "Agents with a question or permission decision wait here"
	case groupActive:
		return "Agents actively working in the background"
	case groupCompleted:
		return "Finished agents wait here for review or injection"
	case groupFailed:
		return "Agents that stopped with an error"
	case groupCancelled:
		return "Agents stopped before completion"
	case groupAbandoned:
		// Spelled out because the two plausible guesses are both wrong: these
		// agents were neither cancelled nor finished, and none was resumed.
		return "Agents whose outcome is unknown; they were not resumed"
	default:
		return ""
	}
}

func (m *Model) selectID(id string) bool {
	if id == "" {
		return false
	}
	for i, r := range m.rows {
		if r.kind == rowJob && r.job.ID == id {
			m.cursor = i
			m.ensureCursorVisible()
			return true
		}
	}
	return false
}

func (m Model) firstJobRow() int {
	for i, r := range m.rows {
		if r.kind == rowJob {
			return i
		}
	}
	return -1
}

func (m Model) lastJobRow() int {
	for i := len(m.rows) - 1; i >= 0; i-- {
		if m.rows[i].kind == rowJob {
			return i
		}
	}
	return -1
}

func (m *Model) move(delta int) {
	if m.cursor < 0 {
		m.cursor = m.firstJobRow()
		m.ensureCursorVisible()
		return
	}
	for i := m.cursor + delta; i >= 0 && i < len(m.rows); i += delta {
		if m.rows[i].kind == rowJob {
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
		w = 88
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
	if h > 32 {
		h = 32
	}
	if h < 8 {
		h = 8
	}
	return h
}

func (m Model) listHeight() int {
	// Leave room for the two-line header, footer, input, and bottom status.
	h := m.modalHeight() - 10
	if h < 1 {
		return 1
	}
	return h
}

func (m Model) renderRows(w int) string {
	h := m.listHeight()
	lines := make([]string, 0, h)
	for i := m.scrollOffset; i < len(m.rows) && len(lines) < h; i++ {
		r := m.rows[i]
		rendered := ""
		if r.kind == rowHeader {
			rendered = m.renderHeader(r.group, w)
		} else {
			rendered = m.renderJobRow(r.job, i == m.cursor, w)
		}
		for _, line := range strings.Split(rendered, "\n") {
			if len(lines) >= h {
				break
			}
			lines = append(lines, line)
		}
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderHeader(g group, w int) string {
	label := theme.StylePrimary.Bold(true).Render(groupLabel(g))
	desc := theme.StyleDim.Render(groupDescription(g))
	return label + "\n " + truncate(desc, max(0, w-1))
}

func (m Model) renderJobRow(j Job, selected bool, w int) string {
	cursor := "  "
	if selected {
		cursor = "▶ "
	}
	icon := "✻"
	iconStyle := theme.StyleAccent
	switch groupForJob(j) {
	case groupNeedsInput:
		icon, iconStyle = "●", theme.StyleWarning
	case groupCompleted:
		icon, iconStyle = "✓", theme.StyleSuccess
	case groupFailed:
		icon, iconStyle = "!", theme.StyleError
	case groupCancelled:
		icon, iconStyle = "×", theme.StyleSecondary
	case groupAbandoned:
		// A question mark, never the completed check or the cancelled cross:
		// the glyph has to read as "we do not know how this ended".
		icon, iconStyle = "?", theme.StyleWarning
	}
	prompt := truncate(strings.TrimSpace(rowMessage(j)), max(8, w-30))
	if prompt == "" {
		prompt = "(no prompt)"
	}
	age := roundedAge(j)
	lead := cursor + iconStyle.Render(icon) + " " + theme.StyleAccent.Render(j.ID) + "  " + theme.StylePrimary.Render(prompt)
	space := max(1, w-lipgloss.Width(lead)-lipgloss.Width(age))
	line := truncate(lead+strings.Repeat(" ", space)+theme.StyleDim.Render(age), w)
	details := fmt.Sprintf("    %s · %s · %s · api %d/%d · $%.4f", targetLabel(j), providerLabel(j), statusBadge(j), j.Tokens.Input, j.Tokens.Output, j.CostUSD)
	line += "\n" + theme.StyleDim.Render(truncate(details, w))
	// The agent's own plan, when it kept one. This is the only place a
	// background job can say what it is part-way through, so it belongs
	// with the row rather than behind the transcript.
	if todos := todoLine(j); todos != "" {
		line += "\n" + theme.StyleDim.Render(truncate(todos, w))
	}
	if selected {
		line = lipgloss.NewStyle().Background(theme.BaseSurfaceBright).Render(line)
	}
	if wt := worktreeMessage(j); wt != "" {
		line += "\n" + theme.StyleDim.Render("  "+truncate(wt, max(0, w-2)))
	}
	if digest := jobspkg.ArtifactDigest(j.Artifacts); digest != "" {
		line += "\n" + theme.StyleDim.Render("  artifacts: "+truncate(digest, max(0, w-13)))
	}
	return line
}

func statusBadge(j Job) string {
	switch {
	case j.NeedsApproval:
		return "approval"
	case j.NeedsInput:
		return "input"
	}
	// Abandonment outranks both the result-handling status and the last
	// activity label. Those describe what someone did with the result or what
	// the agent was last seen doing; neither may stand in for an outcome we
	// never observed.
	if groupForState(j.State) == groupAbandoned {
		badge := StateAbandoned
		if cause := abandonCauseLabel(j); cause != "" {
			badge += " (" + cause + ")"
		}
		// Keep a final handling label alongside the outcome. Dropping it
		// would invite a user who already injected or ignored this result to
		// do it a second time. The outcome still leads: the handling label
		// says what was done with the result, not whether the work finished.
		switch status := strings.ToLower(strings.TrimSpace(j.ResultStatus)); status {
		case "injected", "ignored", "consumed":
			badge += " · " + status
		}
		return badge
	}
	switch strings.ToLower(strings.TrimSpace(j.ResultStatus)) {
	case "consumed":
		return "consumed"
	case "injected":
		return "injected"
	case "ignored":
		return "ignored"
	case "seen":
		return "seen"
	case "pending":
		if groupForState(j.State) != groupActive {
			if strings.EqualFold(strings.TrimSpace(j.State), StateCancelled) {
				return "cancelled"
			}
			if strings.EqualFold(strings.TrimSpace(j.State), StateFailed) && strings.TrimSpace(j.Error) == "" && strings.TrimSpace(j.Summary) == "" {
				return "failed"
			}
			return "ready"
		}
	}
	activity := strings.ToLower(strings.TrimSpace(j.LastActivity))
	if activity != "" {
		return activity
	}
	return strings.ToLower(strings.TrimSpace(j.State))
}

// abandonCauseLabel returns the cause to show alongside the abandoned state,
// or "" when there is nothing to add. "unknown" is dropped deliberately: the
// state already says the outcome is unknown, so repeating it as a cause adds
// no information and reads like a second, distinct fact.
func abandonCauseLabel(j Job) string {
	cause := strings.ToLower(strings.TrimSpace(j.AbandonCause))
	if cause == "" || cause == string(jobspkg.AbandonCauseUnknown) {
		return ""
	}
	return cause
}

// canCancel is false for every terminal state, abandoned included. There is
// nothing left to stop, and offering the key implies the job is still live.
func canCancel(j Job) bool {
	return groupForState(j.State) == groupActive
}

// canDecideResult is true for abandoned jobs. Whatever partial work and error
// text they carry is still the user's to inject or ignore; without a decision
// the result would sit in the view forever with no way to clear it.
func canDecideResult(j Job) bool {
	if groupForState(j.State) == groupActive {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(j.ResultStatus)) {
	case "", "pending", "seen":
		if strings.EqualFold(strings.TrimSpace(j.State), StateCancelled) &&
			strings.TrimSpace(j.Summary) == "" &&
			strings.TrimSpace(j.Error) == "" {
			return false
		}
		return true
	default:
		return false
	}
}

func providerLabel(j Job) string {
	if j.Provider == "" {
		return j.Model
	}
	if j.Model == "" {
		return j.Provider
	}
	return j.Provider + "/" + j.Model
}

func targetLabel(j Job) string {
	if strings.TrimSpace(j.ComputerName) == "" {
		return "local"
	}
	return "computer:" + j.ComputerName
}

func roundedAge(j Job) string {
	if j.CreatedAt.IsZero() {
		return "0s"
	}
	ref := time.Now()
	if !j.FinishedAt.IsZero() {
		ref = j.FinishedAt
	}
	return shortDuration(ref.Sub(j.CreatedAt))
}

func newestTime(j Job) time.Time {
	if !j.UpdatedAt.IsZero() {
		return j.UpdatedAt
	}
	if !j.FinishedAt.IsZero() {
		return j.FinishedAt
	}
	return j.CreatedAt
}

func rowMessage(j Job) string {
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

func worktreeMessage(j Job) string {
	if j.WorktreePath != "" {
		parts := []string{"worktree: " + j.WorktreePath}
		if j.WorktreeBranch != "" {
			parts = append(parts, "branch "+j.WorktreeBranch)
		}
		if j.WorktreeBase != "" {
			parts = append(parts, "base "+j.WorktreeBase)
		}
		return strings.Join(parts, " · ")
	}
	if j.WorktreeNote != "" {
		return "worktree unavailable: " + j.WorktreeNote
	}
	return ""
}

func nonEmpty(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func shortDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d / time.Minute)
	s := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm%02ds", m, s)
}

// truncate clips s to w display columns, preserving any ANSI styling it
// carries.
//
// Counting runes is wrong for these rows. Every styled segment carries ~15-20
// runes of invisible SGR escape, so a row only ~40 columns wide counts as ~150
// runes; clipping at w runes cut it far short of the terminal width and landed
// mid-escape. The callers already size their columns with lipgloss.Width, so
// measuring the final string any other way contradicts the layout that
// produced it. Display width is also the right measure for wide runes, which
// rune counting silently under-measures.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// countTodosDone counts completed entries.
func countTodosDone(items []jobspkg.TodoItem) int {
	n := 0
	for _, item := range items {
		if item.Status == jobspkg.TodoCompleted {
			n++
		}
	}
	return n
}

// currentTodo names the item the agent says it is on. It prefers the
// in_progress entry — todo_write allows only one — and falls back to the first
// pending one, because a list with nothing in progress is usually a plan the
// agent has written but not yet started.
func currentTodo(items []jobspkg.TodoItem) string {
	for _, item := range items {
		if item.Status == jobspkg.TodoInProgress {
			return item.Content
		}
	}
	for _, item := range items {
		if item.Status == jobspkg.TodoPending {
			return item.Content
		}
	}
	return ""
}

// todoLine renders the one-line plan summary, or "" when there is no plan to
// show. A finished list still reports its counts: "3/3 done" is the useful
// closing statement, and suppressing it would make a completed plan look like
// no plan at all.
func todoLine(j Job) string {
	if j.TodosTotal == 0 {
		return ""
	}
	line := fmt.Sprintf("    todos %d/%d", j.TodosDone, j.TodosTotal)
	if j.TodoCurrent != "" {
		line += " · " + j.TodoCurrent
	}
	return line
}
