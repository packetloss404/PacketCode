package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/jobs"
)

func TestRenderPrompt_Interpolation(t *testing.T) {
	inputs := map[string]string{"topic": "widgets", "depth": "deep"}
	steps := map[string]string{"gather": "found three things"}

	out, err := renderPrompt("study {{.inputs.topic}} ({{.inputs.depth}}) using {{.steps.gather}} for {{.item}}", inputs, steps, "phase-x")
	require.NoError(t, err)
	require.Equal(t, "study widgets (deep) using found three things for phase-x", out)
}

func TestRenderPrompt_MissingKeysAreEmpty(t *testing.T) {
	out, err := renderPrompt("a={{.inputs.nope}} b={{.steps.absent}} c={{.item}}", nil, nil, "")
	require.NoError(t, err)
	require.Equal(t, "a= b= c=", out)
}

func TestRenderPrompt_ParseError(t *testing.T) {
	_, err := renderPrompt("{{.inputs.topic", map[string]string{"topic": "x"}, nil, "")
	require.Error(t, err)
}

func TestSummariesOf(t *testing.T) {
	sr := StepResult{
		Agents: []jobs.Result{
			{Summary: "one"},
			{Summary: ""},
			{Error: "boom"},
			{Summary: "two"},
		},
	}
	got := summariesOf(sr)
	require.Equal(t, "one\n\n(error) boom\n\ntwo", got)
}
