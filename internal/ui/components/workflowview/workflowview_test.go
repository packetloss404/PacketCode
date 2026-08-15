package workflowview

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	jobspkg "github.com/packetcode/packetcode/internal/jobs"
	"github.com/packetcode/packetcode/internal/workflow"
)

func sampleRuns() []workflow.RunSnapshot {
	return []workflow.RunSnapshot{{
		ID:       "wf-1",
		Workflow: "review",
		State:    workflow.RunRunning,
		Phases: []workflow.PhaseSnapshot{{
			Name: "review",
			Steps: []workflow.StepSnapshot{{
				Name: "review",
				Mode: workflow.StepParallel,
				Agents: []workflow.AgentSnapshot{
					{JobID: "job-aaaa", HasJob: true, Job: jobspkg.Snapshot{ID: "job-aaaa", State: jobspkg.StateRunning, Provider: "fake", Model: "m"}},
					{JobID: "job-bbbb", HasJob: false},
				},
			}},
		}},
	}}
}

func TestWorkflowView_ShowBuildsRows(t *testing.T) {
	m := New()
	m.Resize(100, 24)
	m.Show(sampleRuns())

	if !m.Visible() {
		t.Fatal("expected visible after Show")
	}
	// Rows: run header, phase, step, 2 agents = 5.
	if len(m.rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(m.rows))
	}
	// Cursor should land on the first selectable row (the run header).
	if r, ok := m.selectedRow(); !ok || r.kind != rowRun {
		t.Fatalf("expected cursor on run row, got %+v", r)
	}
}

func TestWorkflowView_OpenAgentEmitsOpenMsg(t *testing.T) {
	m := New()
	m.Resize(100, 24)
	m.Show(sampleRuns())

	// Move down to the first agent row (run -> agent, phase/step are skipped).
	var cmd tea.Cmd
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	r, ok := m.selectedRow()
	if !ok || r.kind != rowAgent {
		t.Fatalf("expected agent row after moving down, got %+v", r)
	}
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an OpenMsg command")
	}
	msg := cmd()
	open, ok := msg.(OpenMsg)
	if !ok {
		t.Fatalf("expected OpenMsg, got %T", msg)
	}
	if open.JobID != "job-aaaa" {
		t.Fatalf("expected job-aaaa, got %s", open.JobID)
	}
}

func TestWorkflowView_CancelRunEmitsCancelMsg(t *testing.T) {
	m := New()
	m.Resize(100, 24)
	m.Show(sampleRuns())

	// Cursor is on the run header; 'c' cancels the (running) run.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("expected a CancelMsg command")
	}
	cancel, ok := cmd().(CancelMsg)
	if !ok {
		t.Fatalf("expected CancelMsg, got %T", cmd())
	}
	if cancel.RunID != "wf-1" {
		t.Fatalf("expected wf-1, got %s", cancel.RunID)
	}
}

func TestStepHeaderText_ShowsVerificationAndRetries(t *testing.T) {
	got := stepHeaderText(workflow.StepSnapshot{
		Name:         "synthesize",
		Mode:         workflow.StepSingle,
		Attempts:     2,
		Verification: workflow.VerificationPassed,
	})
	if got != "synthesize [single · passed · 2 attempts]" {
		t.Fatalf("stepHeaderText = %q", got)
	}
}

func TestRunHeaderText_ShowsResolvedComputer(t *testing.T) {
	got := runHeaderText(workflow.RunSnapshot{
		ID:           "wf-1",
		Workflow:     "review",
		Computer:     "prod-alias",
		ComputerName: "production",
		WorkingDir:   "/srv/app",
	})
	if got != "wf-1  review  @production" {
		t.Fatalf("runHeaderText = %q", got)
	}
}

func TestWorkflowView_LabelsVerifierRows(t *testing.T) {
	runs := sampleRuns()
	runs[0].Phases[0].Steps[0].Agents = []workflow.AgentSnapshot{{
		JobID:   "verify-1",
		Role:    "verifier",
		Attempt: 2,
	}}
	m := New()
	m.Resize(100, 24)
	m.Show(runs)
	if len(m.rows) < 4 || m.rows[3].text != "verifier a2 · " {
		t.Fatalf("verifier row label missing: %+v", m.rows)
	}
}

func TestRenderJobState_AbandonedIsNeitherDoneNorCancelled(t *testing.T) {
	got := renderJobState(jobspkg.StateAbandoned.String(), 10)
	if !strings.Contains(got, "abandoned") {
		t.Fatalf("renderJobState = %q, want it to contain the abandoned label", got)
	}
	// "abandoned" is 9 runes, so the 10-wide column pads rather than
	// truncating; a truncated "abandone…" would be an easy misread.
	if strings.Contains(got, "…") {
		t.Fatalf("renderJobState = %q, want no truncation at width 10", got)
	}
}

func TestWorkflowView_RendersAbandonedAgentState(t *testing.T) {
	runs := sampleRuns()
	runs[0].Phases[0].Steps[0].Agents = []workflow.AgentSnapshot{{
		JobID:  "job-aaaa",
		HasJob: true,
		Job: jobspkg.Snapshot{
			ID:           "job-aaaa",
			State:        jobspkg.StateAbandoned,
			AbandonCause: jobspkg.AbandonCauseTransportLost,
			Provider:     "fake",
			Model:        "m",
		},
	}}

	m := New()
	m.Resize(120, 24)
	m.Show(runs)

	out := m.View()
	if !strings.Contains(out, "abandoned") {
		t.Fatalf("view missing abandoned state:\n%s", out)
	}
	if strings.Contains(out, "cancelled") || strings.Contains(out, "completed") {
		t.Fatalf("abandoned agent rendered as a cancellation or completion:\n%s", out)
	}
}
