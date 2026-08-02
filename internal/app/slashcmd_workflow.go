package app

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/packetcode/packetcode/internal/workflow"
)

// handleWorkflowCommand dispatches /workflows (alias /workflow):
//
//	/workflows                 open the live run view
//	/workflows list            list saved specs and active runs
//	/workflows validate <name> validate a spec without executing it
//	/workflows run [--computer <name>] <name>  start a workflow by name
//	/workflows stop [id|all]   cancel a run (or every run)
//	/workflows <id>            open the view focused on a run
func (a *App) handleWorkflowCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) > 0 && strings.EqualFold(args[0], "validate") {
		return a.handleWorkflowValidate(args[1:])
	}
	if a.workflow == nil {
		a.conversation.AppendSystem("workflows: engine is disabled (no workflow.Engine wired)")
		return a, nil
	}

	if len(args) == 0 {
		a.workflowView.Show(a.workflow.List())
		return a, nil
	}

	switch strings.ToLower(args[0]) {
	case "list", "ls":
		return a.handleWorkflowList()
	case "run", "start":
		return a.handleWorkflowRun(args[1:])
	case "stop", "cancel":
		return a.handleWorkflowStop(args[1:])
	default:
		// Treat a bare argument as a run id to open in the view.
		return a.handleWorkflowOpen(args[0])
	}
}

func (a *App) handleWorkflowValidate(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		a.conversation.AppendSystem("workflows validate: missing name")
		return a, nil
	}
	wf, err := a.workflowLoader.Load(args[0])
	if err != nil {
		a.conversation.AppendSystem("workflows validate: " + err.Error())
		return a, nil
	}
	steps := 0
	verified := 0
	retries := 0
	for _, phase := range wf.Phases {
		for _, step := range phase.Steps {
			steps++
			if step.Verify != nil {
				verified++
				retries += step.Retry.Max
			}
		}
	}
	a.conversation.AppendSystem(fmt.Sprintf(
		"workflows validate: %s is valid (schema v%d, %d phases, %d steps, %d verified, %d unverified, retry cap %d)",
		wf.Name, wf.SchemaVersion, len(wf.Phases), steps, verified, steps-verified, retries,
	))
	return a, nil
}

func (a *App) handleWorkflowList() (tea.Model, tea.Cmd) {
	var b strings.Builder
	b.WriteString("workflows:")

	specs := a.workflowLoader.List()
	if len(specs) == 0 {
		b.WriteString("\n  (no saved workflows)")
	} else {
		b.WriteString("\n  saved: " + strings.Join(specs, ", "))
	}

	runs := a.workflow.List()
	if len(runs) == 0 {
		b.WriteString("\n  no runs yet — /workflows run <name>")
	} else {
		b.WriteString("\n  runs:")
		for _, r := range runs {
			b.WriteString(fmt.Sprintf("\n    %s  %-14s %-10s target=%s", r.ID, r.Workflow, r.State, workflowRunTarget(r)))
			if strings.TrimSpace(r.Err) != "" {
				b.WriteString(" — " + firstLine(r.Err))
			}
		}
	}
	a.conversation.AppendSystem(b.String())
	return a, nil
}

func (a *App) handleWorkflowRun(args []string) (tea.Model, tea.Cmd) {
	name, opts, inputArgs, parseErr := parseWorkflowRunArgs(args)
	if parseErr != nil {
		a.conversation.AppendSystem("workflows run: " + parseErr.Error())
		return a, nil
	}
	if name == "" {
		names := a.workflowLoader.List()
		a.conversation.AppendSystem("workflows run: missing name. available: " + strings.Join(names, ", "))
		return a, nil
	}
	wf, err := a.workflowLoader.Load(name)
	if err != nil {
		a.conversation.AppendSystem("workflows run: " + err.Error())
		return a, nil
	}

	// Overlay key=value overrides; quote multiword values, e.g.
	// /workflows run review target="the staged diff".
	if overrides := parseInputOverrides(inputArgs); len(overrides) > 0 {
		if wf.Inputs == nil {
			wf.Inputs = map[string]string{}
		}
		for k, v := range overrides {
			wf.Inputs[k] = v
		}
	}

	run, err := a.workflow.StartWithOptions(context.Background(), wf, opts)
	if err != nil {
		a.conversation.AppendSystem("workflows run: " + err.Error())
		return a, nil
	}
	target := "active workspace"
	if opts.Computer != "" {
		target = opts.Computer
	}
	a.conversation.AppendSystem(fmt.Sprintf("[workflow:%s started — %s · target %s] %s", run.ID, wf.Name, target, workflowShape(wf)))
	a.workflowView.ShowFocused(a.workflow.List(), run.ID)
	return a, nil
}

func (a *App) handleWorkflowStop(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 || strings.EqualFold(args[0], "all") {
		n := a.workflow.CancelAll()
		a.conversation.AppendSystem(fmt.Sprintf("workflows stop: cancelled %d run(s)", n))
		if a.workflowView.Visible() {
			a.workflowView.SetRuns(a.workflow.List())
		}
		return a, nil
	}
	id := args[0]
	if a.workflow.Cancel(id) {
		a.conversation.AppendSystem(fmt.Sprintf("[workflow:%s — cancellation requested]", id))
	} else {
		a.conversation.AppendSystem(fmt.Sprintf("[workflow:%s not found or already terminal]", id))
	}
	if a.workflowView.Visible() {
		a.workflowView.SetRuns(a.workflow.List())
	}
	return a, nil
}

func (a *App) handleWorkflowOpen(id string) (tea.Model, tea.Cmd) {
	if _, ok := a.workflow.Get(id); !ok {
		a.conversation.AppendSystem(fmt.Sprintf("[workflow:%s not found]", id))
		return a, nil
	}
	a.workflowView.ShowFocused(a.workflow.List(), id)
	return a, nil
}

func (a *App) handleWorkflowCancel(id string) (tea.Model, tea.Cmd) {
	if a.workflow == nil {
		return a, nil
	}
	if a.workflow.Cancel(id) {
		a.conversation.AppendSystem(fmt.Sprintf("[workflow:%s — cancellation requested]", id))
	}
	if a.workflowView.Visible() {
		a.workflowView.SetRuns(a.workflow.List())
	}
	return a, nil
}

// handleWorkflowUpdate reacts to a run state change delivered off-thread. On a
// terminal transition it appends a one-line system message summarising the run.
func (a *App) handleWorkflowUpdate(run workflow.RunSnapshot) (tea.Model, tea.Cmd) {
	if !run.State.IsTerminal() {
		return a, nil
	}
	if a.workflowTerminalSeen == nil {
		a.workflowTerminalSeen = map[string]bool{}
	}
	if a.workflowTerminalSeen[run.ID] {
		return a, nil
	}
	a.workflowTerminalSeen[run.ID] = true

	switch run.State {
	case workflow.RunCompleted:
		a.conversation.AppendSystem(fmt.Sprintf("[workflow:%s — %s completed]", run.ID, run.Workflow))
	case workflow.RunFailed:
		a.conversation.AppendSystem(fmt.Sprintf("[workflow:%s — %s failed: %s]", run.ID, run.Workflow, firstLine(run.Err)))
	case workflow.RunCancelled:
		a.conversation.AppendSystem(fmt.Sprintf("[workflow:%s — %s cancelled]", run.ID, run.Workflow))
	}
	return a, nil
}

// parseInputOverrides parses key=value tokens. Quoted values split by the
// slash parser are reassembled until the closing quote.
func parseInputOverrides(args []string) map[string]string {
	out := map[string]string{}
	for i := 0; i < len(args); i++ {
		k, v, ok := strings.Cut(args[i], "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			continue
		}
		if len(v) > 0 && (v[0] == '\'' || v[0] == '"') {
			quote := v[0]
			v = v[1:]
			for !strings.HasSuffix(v, string(quote)) && i+1 < len(args) {
				i++
				v += " " + args[i]
			}
			v = strings.TrimSuffix(v, string(quote))
		}
		out[k] = strings.TrimSpace(v)
	}
	return out
}

// parseWorkflowRunArgs keeps placement flags separate from workflow inputs.
// Flags precede the workflow name so existing `run <name> key=value` syntax
// remains byte-for-byte compatible and a workflow input cannot accidentally
// consume a placement option.
func parseWorkflowRunArgs(args []string) (string, workflow.RunOptions, []string, error) {
	var opts workflow.RunOptions
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "--") {
		switch args[i] {
		case "--computer":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", opts, nil, fmt.Errorf("--computer requires a value")
			}
			opts.Computer = strings.TrimSpace(args[i+1])
			i += 2
		default:
			return "", opts, nil, fmt.Errorf("unknown flag %s", args[i])
		}
	}
	if i >= len(args) {
		return "", opts, nil, nil
	}
	name := args[i]
	inputArgs := args[i+1:]
	for _, arg := range inputArgs {
		if strings.HasPrefix(arg, "--") {
			return "", opts, nil, fmt.Errorf("placement flags must precede the workflow name")
		}
	}
	return name, opts, inputArgs, nil
}

func workflowRunTarget(run workflow.RunSnapshot) string {
	if label := run.TargetLabel(); label != "" {
		return label
	}
	if run.WorkingDir != "" {
		return "local"
	}
	return "default"
}

// workflowShape renders a compact phase/step outline for the start message.
func workflowShape(wf workflow.Workflow) string {
	parts := make([]string, 0, len(wf.Phases))
	for _, ph := range wf.Phases {
		steps := make([]string, 0, len(ph.Steps))
		for _, st := range ph.Steps {
			if st.Mode == workflow.StepParallel {
				steps = append(steps, fmt.Sprintf("%s×%d", st.Name, len(st.FanOut)))
			} else {
				steps = append(steps, st.Name)
			}
		}
		parts = append(parts, ph.Name+"("+strings.Join(steps, ",")+")")
	}
	return strings.Join(parts, " → ")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
