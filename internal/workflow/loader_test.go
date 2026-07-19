package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoader_BuiltinReview(t *testing.T) {
	l := NewLoader(t.TempDir())

	names := l.List()
	require.Contains(t, names, "review")

	wf, ok := l.Get("review")
	require.True(t, ok)
	require.Equal(t, "review", wf.Name)
	require.Len(t, wf.Phases, 2)

	// Phase 1 is a parallel fan-out over dimensions, bound to "review".
	p1 := wf.Phases[0]
	require.Len(t, p1.Steps, 1)
	require.Equal(t, StepParallel, p1.Steps[0].Mode)
	require.Equal(t, "review", p1.Steps[0].BindKey())
	require.NotEmpty(t, p1.Steps[0].FanOut)

	// Phase 2 is a single synthesizer referencing {{.steps.review}}.
	p2 := wf.Phases[1]
	require.Len(t, p2.Steps, 1)
	require.Equal(t, StepSingle, p2.Steps[0].Mode)
	require.Contains(t, p2.Steps[0].Agent.Prompt, "{{.steps.review}}")

	// The built-in is a valid spec.
	require.NoError(t, validate(wf))
}

func TestLoader_ProjectTOMLOverridesAndLoads(t *testing.T) {
	dir := t.TempDir()
	wfDir := filepath.Join(dir, ".packetcode", "workflows")
	require.NoError(t, os.MkdirAll(wfDir, 0o755))

	toml := `
name = "custom"

[inputs]
target = "the repo"

[[phases]]
name = "analyze"

  [[phases.steps]]
  name = "scan"
  mode = "parallel"
  bind = "scan"
  fan_out = ["security", "perf"]
  prompt = "scan {{.inputs.target}} for {{.item}}"

[[phases]]
name = "report"

  [[phases.steps]]
  name = "write"
  mode = "single"
  prompt = "summarize {{.steps.scan}}"
`
	require.NoError(t, os.WriteFile(filepath.Join(wfDir, "custom.toml"), []byte(toml), 0o644))

	l := NewLoader(dir)
	require.Contains(t, l.List(), "custom")

	wf, ok := l.Get("custom")
	require.True(t, ok)
	require.Equal(t, "custom", wf.Name)
	require.Equal(t, "the repo", wf.Inputs["target"])
	require.Len(t, wf.Phases, 2)

	scan := wf.Phases[0].Steps[0]
	require.Equal(t, StepParallel, scan.Mode)
	require.Equal(t, []string{"security", "perf"}, scan.FanOut)
	require.Equal(t, "scan", scan.BindKey())

	write := wf.Phases[1].Steps[0]
	require.Equal(t, StepSingle, write.Mode)
	require.Contains(t, write.Agent.Prompt, "{{.steps.scan}}")
}

func TestLoader_MalformedProjectOverrideDoesNotFallBack(t *testing.T) {
	dir := t.TempDir()
	wfDir := filepath.Join(dir, ".packetcode", "workflows")
	require.NoError(t, os.MkdirAll(wfDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wfDir, "review.toml"), []byte(`[[phases`), 0o644))

	_, err := NewLoader(dir).Load("review")
	require.Error(t, err)
	require.Contains(t, err.Error(), "review.toml")
}

func TestLoader_GetUnknown(t *testing.T) {
	l := NewLoader(t.TempDir())
	_, ok := l.Get("does-not-exist")
	require.False(t, ok)
}

func TestLoader_NameDefaultsToFilename(t *testing.T) {
	dir := t.TempDir()
	wfDir := filepath.Join(dir, ".packetcode", "workflows")
	require.NoError(t, os.MkdirAll(wfDir, 0o755))
	toml := `
[[phases]]
name = "only"
  [[phases.steps]]
  name = "s"
  prompt = "hi"
`
	require.NoError(t, os.WriteFile(filepath.Join(wfDir, "noname.toml"), []byte(toml), 0o644))

	l := NewLoader(dir)
	wf, ok := l.Get("noname")
	require.True(t, ok)
	require.Equal(t, "noname", wf.Name)
}
