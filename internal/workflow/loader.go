package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/packetcode/packetcode/internal/config"
)

// Loader resolves Workflow specs by name from three sources, in increasing
// precedence: built-in specs, ~/.packetcode/workflows/*.toml, and
// <project>/.packetcode/workflows/*.toml. A project file named foo.toml
// overrides a user file foo.toml, which overrides a built-in named foo.
type Loader struct {
	projectDir string
}

// NewLoader constructs a Loader rooted at workingDir for project-scoped specs.
func NewLoader(workingDir string) *Loader {
	if strings.TrimSpace(workingDir) == "" {
		workingDir = "."
	}
	return &Loader{projectDir: workingDir}
}

// NewRemoteLoader constructs a loader that exposes built-in and user-level
// workflows without probing a project directory on the controller machine.
// A remote POSIX root is not a local filesystem path, and silently loading
// .packetcode/workflows from the directory packetcode happened to launch in
// could apply an unrelated local override to a server. Remote project workflow
// discovery belongs in a backend-aware asynchronous loader.
func NewRemoteLoader() *Loader { return &Loader{} }

// List returns every known workflow name (built-ins plus files), sorted.
func (l *Loader) List() []string {
	seen := map[string]bool{}
	for name := range builtinWorkflows() {
		seen[name] = true
	}
	for _, dir := range l.dirs() {
		for _, name := range tomlNamesIn(dir) {
			seen[name] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Get resolves a workflow by name. Files take precedence over built-ins, with
// project files winning over user files.
func (l *Loader) Get(name string) (Workflow, bool) {
	wf, err := l.Load(name)
	return wf, err == nil
}

// Load resolves a workflow by name and reports errors from the first existing
// file. A malformed higher-precedence file must not silently fall back.
func (l *Loader) Load(name string) (Workflow, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Workflow{}, fmt.Errorf("workflow name is empty")
	}
	// Project dir first, then user dir (highest precedence first).
	for _, dir := range l.dirsByPrecedence() {
		path := filepath.Join(dir, name+".toml")
		if _, err := os.Stat(path); err == nil {
			wf, loadErr := loadTOMLWorkflow(path)
			if loadErr != nil {
				return Workflow{}, loadErr
			}
			return wf, nil
		} else if !os.IsNotExist(err) {
			return Workflow{}, fmt.Errorf("load workflow %q: %w", name, err)
		}
	}
	if wf, ok := builtinWorkflows()[name]; ok {
		return wf, nil
	}
	return Workflow{}, fmt.Errorf("workflow %q not found", name)
}

// dirs returns the workflow directories in load order (user, then project).
func (l *Loader) dirs() []string {
	var dirs []string
	if home, err := config.HomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "workflows"))
	}
	if strings.TrimSpace(l.projectDir) != "" {
		dirs = append(dirs, filepath.Join(l.projectDir, ".packetcode", "workflows"))
	}
	return dirs
}

// dirsByPrecedence returns directories highest-precedence first (project, user).
func (l *Loader) dirsByPrecedence() []string {
	dirs := l.dirs()
	// Reverse so project (appended last) is checked first.
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs
}

func tomlNamesIn(dir string) []string {
	matches, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, strings.TrimSuffix(filepath.Base(m), ".toml"))
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────
// TOML decoding
// ─────────────────────────────────────────────────────────────────────────

type tomlWorkflow struct {
	SchemaVersion int               `toml:"schema_version"`
	Name          string            `toml:"name"`
	Inputs        map[string]string `toml:"inputs"`
	Phases        []tomlPhase       `toml:"phases"`
}

type tomlPhase struct {
	Name            string     `toml:"name"`
	ContinueOnError bool       `toml:"continue_on_error"`
	Steps           []tomlStep `toml:"steps"`
}

// tomlStep flattens the AgentSpec fields onto the step for spec ergonomics.
type tomlStep struct {
	Name         string      `toml:"name"`
	Mode         string      `toml:"mode"`
	Bind         string      `toml:"bind"`
	FanOut       []string    `toml:"fan_out"`
	Prompt       string      `toml:"prompt"`
	Provider     string      `toml:"provider"`
	Model        string      `toml:"model"`
	SystemPrompt string      `toml:"system_prompt"`
	AllowWrite   bool        `toml:"allow_write"`
	Verify       *tomlVerify `toml:"verify"`
	Retry        tomlRetry   `toml:"retry"`
}

type tomlVerify struct {
	Prompt       string `toml:"prompt"`
	Provider     string `toml:"provider"`
	Model        string `toml:"model"`
	SystemPrompt string `toml:"system_prompt"`
	PassContract string `toml:"pass_contract"`
}

type tomlRetry struct {
	Max int `toml:"max"`
}

func loadTOMLWorkflow(path string) (Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Workflow{}, err
	}
	var tw tomlWorkflow
	metadata, err := toml.Decode(string(data), &tw)
	if err != nil {
		return Workflow{}, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		return Workflow{}, fmt.Errorf("parse %s: unknown field(s): %s", filepath.Base(path), strings.Join(keys, ", "))
	}
	if tw.SchemaVersion == 0 {
		return Workflow{}, fmt.Errorf("parse %s: missing schema_version (current: %d)", filepath.Base(path), CurrentSchemaVersion)
	}
	if tw.SchemaVersion != CurrentSchemaVersion {
		return Workflow{}, fmt.Errorf("parse %s: unsupported schema_version %d (current: %d)", filepath.Base(path), tw.SchemaVersion, CurrentSchemaVersion)
	}
	if tw.Name == "" {
		// Default the name to the filename stem so `run <file>` works.
		tw.Name = strings.TrimSuffix(filepath.Base(path), ".toml")
	}
	wf := Workflow{SchemaVersion: tw.SchemaVersion, Name: tw.Name, Inputs: tw.Inputs}
	for _, tp := range tw.Phases {
		ph := Phase{Name: tp.Name, ContinueOnError: tp.ContinueOnError}
		for _, ts := range tp.Steps {
			mode, modeErr := parseMode(ts.Mode)
			if modeErr != nil {
				return Workflow{}, fmt.Errorf("parse %s: step %q: %w", filepath.Base(path), ts.Name, modeErr)
			}
			step := Step{
				Name:   ts.Name,
				Mode:   mode,
				Bind:   ts.Bind,
				FanOut: ts.FanOut,
				Agent: AgentSpec{
					Prompt:       ts.Prompt,
					Provider:     ts.Provider,
					Model:        ts.Model,
					SystemPrompt: ts.SystemPrompt,
					AllowWrite:   ts.AllowWrite,
				},
				Retry: RetrySpec{Max: ts.Retry.Max},
			}
			if ts.Verify != nil {
				step.Verify = &VerifySpec{
					Prompt:       ts.Verify.Prompt,
					Provider:     ts.Verify.Provider,
					Model:        ts.Verify.Model,
					SystemPrompt: ts.Verify.SystemPrompt,
					PassContract: ts.Verify.PassContract,
				}
			}
			ph.Steps = append(ph.Steps, step)
		}
		wf.Phases = append(wf.Phases, ph)
	}
	if err := validate(wf); err != nil {
		return Workflow{}, err
	}
	return wf, nil
}

func parseMode(s string) (StepMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "parallel", "fanout", "fan-out", "fan_out":
		return StepParallel, nil
	case "", "single":
		return StepSingle, nil
	default:
		return "", fmt.Errorf("unsupported mode %q", s)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Built-in workflows
// ─────────────────────────────────────────────────────────────────────────

// builtinWorkflows returns the specs that ship with packetcode, keyed by name.
// A fresh map is returned per call so callers cannot mutate the canonical spec.
func builtinWorkflows() map[string]Workflow {
	return map[string]Workflow{
		"review": reviewWorkflow(),
	}
}

// reviewWorkflow fans a set of review dimensions out to parallel reviewers and
// then synthesizes their findings with a single agent. It works out of the box
// with no config files: /workflows run review.
func reviewWorkflow() Workflow {
	return Workflow{
		SchemaVersion: CurrentSchemaVersion,
		Name:          "review",
		Inputs: map[string]string{
			"target": "the current working tree changes",
		},
		Phases: []Phase{
			{
				Name: "review",
				Steps: []Step{
					{
						Name:   "review",
						Mode:   StepParallel,
						Bind:   "review",
						FanOut: []string{"correctness", "security", "performance", "readability"},
						Agent: AgentSpec{
							Prompt: "You are a focused code reviewer. Review {{.inputs.target}} strictly through " +
								"the lens of {{.item}}. Report concrete findings with file/function references " +
								"and concrete suggestions. Be concise; if you find nothing material for this " +
								"dimension, say so briefly.",
						},
					},
				},
			},
			{
				Name: "synthesis",
				Steps: []Step{
					{
						Name: "synthesize",
						Mode: StepSingle,
						Bind: "synthesis",
						Agent: AgentSpec{
							Prompt: "You are a lead engineer synthesizing a code review of {{.inputs.target}}. " +
								"Below are findings from four specialist reviewers.\n\n{{.steps.review}}\n\n" +
								"Produce a single prioritized review: group overlapping findings, rank by " +
								"severity, and give a clear go / no-go recommendation with the top action items.",
						},
					},
				},
			},
		},
	}
}
