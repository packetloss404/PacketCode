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
	workingDir string
}

// NewLoader constructs a Loader rooted at workingDir for project-scoped specs.
func NewLoader(workingDir string) *Loader {
	if strings.TrimSpace(workingDir) == "" {
		workingDir = "."
	}
	return &Loader{workingDir: workingDir}
}

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
	name = strings.TrimSpace(name)
	if name == "" {
		return Workflow{}, false
	}
	// Project dir first, then user dir (highest precedence first).
	for _, dir := range l.dirsByPrecedence() {
		path := filepath.Join(dir, name+".toml")
		if _, err := os.Stat(path); err == nil {
			if wf, err := loadTOMLWorkflow(path); err == nil {
				return wf, true
			}
		}
	}
	if wf, ok := builtinWorkflows()[name]; ok {
		return wf, true
	}
	return Workflow{}, false
}

// dirs returns the workflow directories in load order (user, then project).
func (l *Loader) dirs() []string {
	var dirs []string
	if home, err := config.HomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "workflows"))
	}
	dirs = append(dirs, filepath.Join(l.workingDir, ".packetcode", "workflows"))
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
	Name   string            `toml:"name"`
	Inputs map[string]string `toml:"inputs"`
	Phases []tomlPhase       `toml:"phases"`
}

type tomlPhase struct {
	Name            string     `toml:"name"`
	ContinueOnError bool       `toml:"continue_on_error"`
	Steps           []tomlStep `toml:"steps"`
}

// tomlStep flattens the AgentSpec fields onto the step for spec ergonomics.
type tomlStep struct {
	Name         string   `toml:"name"`
	Mode         string   `toml:"mode"`
	Bind         string   `toml:"bind"`
	FanOut       []string `toml:"fan_out"`
	Prompt       string   `toml:"prompt"`
	Provider     string   `toml:"provider"`
	Model        string   `toml:"model"`
	SystemPrompt string   `toml:"system_prompt"`
	AllowWrite   bool     `toml:"allow_write"`
}

func loadTOMLWorkflow(path string) (Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Workflow{}, err
	}
	var tw tomlWorkflow
	if err := toml.Unmarshal(data, &tw); err != nil {
		return Workflow{}, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if tw.Name == "" {
		// Default the name to the filename stem so `run <file>` works.
		tw.Name = strings.TrimSuffix(filepath.Base(path), ".toml")
	}
	wf := Workflow{Name: tw.Name, Inputs: tw.Inputs}
	for _, tp := range tw.Phases {
		ph := Phase{Name: tp.Name, ContinueOnError: tp.ContinueOnError}
		for _, ts := range tp.Steps {
			ph.Steps = append(ph.Steps, Step{
				Name:   ts.Name,
				Mode:   parseMode(ts.Mode),
				Bind:   ts.Bind,
				FanOut: ts.FanOut,
				Agent: AgentSpec{
					Prompt:       ts.Prompt,
					Provider:     ts.Provider,
					Model:        ts.Model,
					SystemPrompt: ts.SystemPrompt,
					AllowWrite:   ts.AllowWrite,
				},
			})
		}
		wf.Phases = append(wf.Phases, ph)
	}
	if err := validate(wf); err != nil {
		return Workflow{}, err
	}
	return wf, nil
}

func parseMode(s string) StepMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "parallel", "fanout", "fan-out", "fan_out":
		return StepParallel
	default:
		return StepSingle
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
		Name: "review",
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
