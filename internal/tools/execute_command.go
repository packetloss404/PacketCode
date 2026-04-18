package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const executeCommandSchema = `{
  "type": "object",
  "properties": {
    "command":     { "type": "string", "description": "Shell command to execute (sh -c on Unix, cmd /C on Windows)." },
    "cwd":         { "type": "string", "description": "Working directory relative to project root. Defaults to project root." },
    "timeout_sec": { "type": "integer", "description": "Maximum execution time in seconds. Default 60, max 600." }
  },
  "required": ["command"]
}`

const (
	defaultExecTimeout = 60 * time.Second
	maxExecTimeout     = 10 * time.Minute
)

type ExecuteCommandTool struct {
	Root string
}

func NewExecuteCommandTool(root string) *ExecuteCommandTool {
	return &ExecuteCommandTool{Root: root}
}

func (*ExecuteCommandTool) Name() string             { return "execute_command" }
func (*ExecuteCommandTool) RequiresApproval() bool   { return true }
func (*ExecuteCommandTool) Schema() json.RawMessage  { return json.RawMessage(executeCommandSchema) }
func (*ExecuteCommandTool) Description() string {
	return "Execute a shell command and capture stdout+stderr. Requires user approval. Output is truncated past 100KB."
}

type executeCommandParams struct {
	Command    string `json:"command"`
	CWD        string `json:"cwd,omitempty"`
	TimeoutSec int    `json:"timeout_sec,omitempty"`
}

func (t *ExecuteCommandTool) Execute(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	var p executeCommandParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return ToolResult{}, fmt.Errorf("execute_command: parse params: %w", err)
	}
	if strings.TrimSpace(p.Command) == "" {
		return ToolResult{Content: "execute_command: command is empty", IsError: true}, nil
	}

	cwd := t.Root
	if p.CWD != "" {
		resolved, err := resolveInRoot(t.Root, p.CWD)
		if err != nil {
			return ToolResult{Content: err.Error(), IsError: true}, nil
		}
		cwd = resolved
	}

	timeout := defaultExecTimeout
	if p.TimeoutSec > 0 {
		timeout = time.Duration(p.TimeoutSec) * time.Second
		if timeout > maxExecTimeout {
			timeout = maxExecTimeout
		}
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := buildShellCommand(cmdCtx, p.Command)
	cmd.Dir = cwd

	out, runErr := cmd.CombinedOutput()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	const maxOutBytes = 100 * 1024
	truncated := false
	if len(out) > maxOutBytes {
		out = out[:maxOutBytes]
		truncated = true
	}

	timedOut := cmdCtx.Err() == context.DeadlineExceeded

	var b strings.Builder
	fmt.Fprintf(&b, "$ %s\n", p.Command)
	if cwd != t.Root {
		fmt.Fprintf(&b, "(cwd: %s)\n", cwd)
	}
	if len(out) > 0 {
		b.Write(out)
		if !strings.HasSuffix(string(out), "\n") {
			b.WriteByte('\n')
		}
	}
	if truncated {
		b.WriteString("...[output truncated at 100KB]...\n")
	}
	switch {
	case timedOut:
		fmt.Fprintf(&b, "[timed out after %s]\n", timeout)
	case exitCode == 0:
		b.WriteString("[exit 0]")
	default:
		fmt.Fprintf(&b, "[exit %d]", exitCode)
	}

	isError := timedOut || exitCode != 0
	return ToolResult{
		Content: b.String(),
		IsError: isError,
		Metadata: map[string]any{
			"exit_code": exitCode,
			"timed_out": timedOut,
			"truncated": truncated,
			"cwd":       cwd,
		},
	}, nil
}

// buildShellCommand picks the right invocation per OS. We deliberately use
// the shell rather than direct argv splitting so the LLM can use pipes,
// redirects, env-var expansion, etc. — closer to what a developer would type.
func buildShellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}
