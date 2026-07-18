package workflowview

import (
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
