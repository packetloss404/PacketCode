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
	require.Equal(t, CurrentSchemaVersion, wf.SchemaVersion)
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
schema_version = 1
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
schema_version = 1
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

func TestLoader_PCH3SchemaFixtures(t *testing.T) {
	t.Run("valid verifier", func(t *testing.T) {
		wf, err := loadTOMLWorkflow(filepath.Join("testdata", "valid-verifier.toml"))
		require.NoError(t, err)
		require.Equal(t, CurrentSchemaVersion, wf.SchemaVersion)
		step := wf.Phases[0].Steps[0]
		require.NotNil(t, step.Verify)
		require.Equal(t, PassContractV1, step.Verify.PassContract)
		require.Equal(t, 2, step.Retry.Max)
	})

	t.Run("missing verifier remains explicitly unverified", func(t *testing.T) {
		wf, err := loadTOMLWorkflow(filepath.Join("testdata", "unverified.toml"))
		require.NoError(t, err)
		require.Nil(t, wf.Phases[0].Steps[0].Verify)
	})

	t.Run("unknown newer version is refused", func(t *testing.T) {
		_, err := loadTOMLWorkflow(filepath.Join("testdata", "future-version.toml"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported schema_version 2")
	})
}

func TestLoader_RequiresVersionAndRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.toml")
	require.NoError(t, os.WriteFile(missing, []byte(`
name = "missing"
[[phases]]
name = "p"
[[phases.steps]]
name = "s"
prompt = "hi"
`), 0o644))
	_, err := loadTOMLWorkflow(missing)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing schema_version")

	unknown := filepath.Join(dir, "unknown.toml")
	require.NoError(t, os.WriteFile(unknown, []byte(`
schema_version = 1
name = "unknown"
mystery = true
[[phases]]
name = "p"
[[phases.steps]]
name = "s"
prompt = "hi"
`), 0o644))
	_, err = loadTOMLWorkflow(unknown)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown field")

	badMode := filepath.Join(dir, "bad-mode.toml")
	require.NoError(t, os.WriteFile(badMode, []byte(`
schema_version = 1
name = "bad-mode"
[[phases]]
name = "p"
[[phases.steps]]
name = "s"
mode = "concurrent-ish"
prompt = "hi"
`), 0o644))
	_, err = loadTOMLWorkflow(badMode)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported mode")
}
