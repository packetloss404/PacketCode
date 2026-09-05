package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/ui/components/agentview"
)

func TestFirstVisibleProgressStopsThinkingSpinner(t *testing.T) {
	for _, ev := range []agent.AgentEvent{
		{Type: agent.EventTextDelta, Text: "answer"},
		{Type: agent.EventReasoningDelta, Text: "reasoning"},
		{Type: agent.EventToolCallProposed},
	} {
		r := newTestApp(t)
		r.app.spinner.Start("Thinking…")
		r.app.handleAgentEvent(ev)
		if r.app.spinner.Active() {
			t.Fatalf("event %v left generic spinner active", ev.Type)
		}
	}
}

func TestLeftArrowOpensAgentsOnlyFromEmptyIdleInput(t *testing.T) {
	r := newTestApp(t)
	mgr := wireJobsManagerForSlashTest(t, r)
	t.Cleanup(func() { _ = mgr.Shutdown(2 * time.Second) })
	r.app.input.Reset()
	r.app.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if !r.app.agentView.Visible() {
		t.Fatal("left from empty idle input should open Agent View")
	}

	r.app.agentView.Hide()
	r.app.input.SetValue("draft")
	r.app.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if r.app.agentView.Visible() {
		t.Fatal("left while editing text must remain cursor movement")
	}
}

func TestAgentWorkspaceTaskPromptCanClearReturnAndSpawn(t *testing.T) {
	r := newTestApp(t)
	mgr := wireJobsManagerForSlashTest(t, r)
	t.Cleanup(func() { _ = mgr.Shutdown(2 * time.Second) })

	r.app.showAgentView()
	r.app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	r.app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("audit code")})
	if got := r.app.input.Value(); got != "audit code" {
		t.Fatalf("agent task prompt = %q, want %q", got, "audit code")
	}
	r.app.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if got := r.app.input.Value(); got != "" {
		t.Fatalf("Esc should clear agent task draft, got %q", got)
	}
	if !r.app.agentView.Visible() {
		t.Fatal("Esc with a draft should leave Agent View open")
	}

	r.app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	r.app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("review fixtures")})
	r.app.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if got := r.app.input.Value(); got != "" {
		t.Fatalf("spawn should clear agent task prompt, got %q", got)
	}
	if got := len(mgr.List()); got != 1 {
		t.Fatalf("spawned jobs = %d, want 1", got)
	}
	if !r.app.agentView.Visible() {
		t.Fatal("spawning from Agent View should keep the workspace open")
	}

	r.app.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if r.app.agentView.Visible() {
		t.Fatal("Esc from Agent View list should return to chat")
	}
}

func TestAgentWorkspaceListActionsAreNotSwallowedByTaskInput(t *testing.T) {
	r := newTestApp(t)
	mgr := wireJobsManagerForSlashTest(t, r)
	t.Cleanup(func() { _ = mgr.Shutdown(2 * time.Second) })

	_, _ = r.app.handleSpawnCommand([]string{"inspect the renderer"})
	r.app.showAgentView()
	_, cmd := r.app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if got := r.app.input.Value(); got != "" {
		t.Fatalf("list action leaked into task input: %q", got)
	}
	if cmd == nil {
		t.Fatal("peek action did not emit a command")
	}
	if _, ok := cmd().(agentview.PeekMsg); !ok {
		t.Fatalf("peek action emitted %T", cmd())
	}
}
