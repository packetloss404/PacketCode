package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIFixtureStatesRenderProductionComponents(t *testing.T) {
	wants := map[string]string{
		"user-assistant": "context accounting",
		"thinking":       "Thinking",
		"streaming":      "failure occurs",
		"tool-running":   "Bash(go test",
		"tool-result":    "Read 115 lines",
		"approval":       "Do you want to proceed?",
		"error":          "timed out",
		"cancelled":      "turn cancelled",
		"queued":         "(queued)",
		"compacting":     "Compacting",
		"compacted":      "218000 -> 62400",
		"normal":         "manual mode on",
		"accept-edits":   "accept edits on",
		"auto":           "auto mode on",
		"plan":           "plan mode on",
		"bypass":         "bypass permissions on",
		"agents":         "packetcode agents",
		"workflows":      "packetcode workflows",
	}
	if len(wants) != len(TUIFixtureStates) {
		t.Fatalf("fixture test covers %d states, registry has %d", len(wants), len(TUIFixtureStates))
	}
	for _, state := range TUIFixtureStates {
		t.Run(state, func(t *testing.T) {
			model, err := NewTUIFixture(state)
			if err != nil {
				t.Fatal(err)
			}
			fixture := model.(*tuiFixtureModel)
			_, _ = fixture.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			view := fixture.View()
			if !strings.Contains(view, wants[state]) {
				t.Fatalf("fixture %q missing %q:\n%s", state, wants[state], view)
			}
			if state != "workflows" && (!strings.Contains(view, "❯") || (state != "agents" && !strings.Contains(view, "272K"))) {
				t.Fatalf("fixture %q missing shared input/status chrome:\n%s", state, view)
			}
		})
	}
}

func TestTUIFixtureRejectsUnknownState(t *testing.T) {
	if _, err := NewTUIFixture("not-a-state"); err == nil {
		t.Fatal("expected unknown fixture error")
	}
}
