package topbar

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopBar_RendersProvider(t *testing.T) {
	m := New()
	m.SetWidth(120)
	m.SetProvider("openai", "OpenAI", "gpt-4.1")
	m.SetContext(50_000, 200_000)
	m.SetProject("my-app", "main")
	m.SetCost(1.24)

	out := m.View()
	assert.Contains(t, out, "packetcode")
	assert.Contains(t, out, "OpenAI")
	assert.Contains(t, out, "gpt-4.1")
	assert.Contains(t, out, "my-app")
}

func TestTopBar_DropsSegmentsWhenNarrow(t *testing.T) {
	m := New()
	m.SetProvider("openai", "OpenAI", "gpt-4.1")
	m.SetContext(50_000, 200_000)
	m.SetProject("very-long-project-name-here", "feature/long-branch-name-here")
	m.SetCost(123.45)

	wide := m.copyWith(120).View()
	narrow := m.copyWith(40).View()

	// Narrow rendering must drop at least one segment (cost or project).
	assert.True(t,
		!strings.Contains(narrow, "$123.45") || !strings.Contains(narrow, "very-long-project"),
		"narrow rendering should shed at least one droppable segment",
	)
	// Wide rendering shouldn't drop those segments.
	assert.Contains(t, wide, "OpenAI")
}

func TestHumanTokens(t *testing.T) {
	cases := map[int]string{
		500:        "500",
		1500:       "1K",
		1_500_000:  "1.5M",
	}
	for in, want := range cases {
		assert.Equal(t, want, humanTokens(in), in)
	}
}

// copyWith returns a copy of m with the width changed — keeps tests
// isolated from each other.
func (m Model) copyWith(width int) Model {
	m.width = width
	return m
}
