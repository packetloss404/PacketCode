package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/packetcode/packetcode/internal/procrun"
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
	maxExecOutputBytes = 100 * 1024
)

type ExecuteCommandTool struct {
	Root string
}

func NewExecuteCommandTool(root string) *ExecuteCommandTool {
	return &ExecuteCommandTool{Root: root}
}

func (*ExecuteCommandTool) Name() string            { return "execute_command" }
func (*ExecuteCommandTool) RequiresApproval() bool  { return true }
func (*ExecuteCommandTool) Schema() json.RawMessage { return json.RawMessage(executeCommandSchema) }
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
		resolved, err := resolveExistingInRoot(t.Root, p.CWD)
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

	out := procrun.NewBoundedBuffer(maxExecOutputBytes)
	cmd.Stdout = out
	cmd.Stderr = out
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	timedOut := cmdCtx.Err() == context.DeadlineExceeded
	canceled := cmdCtx.Err() == context.Canceled
	truncated := out.Truncated()
	outBytes := out.Bytes()

	var b strings.Builder
	fmt.Fprintf(&b, "$ %s\n", p.Command)
	if cwd != t.Root {
		fmt.Fprintf(&b, "(cwd: %s)\n", cwd)
	}
	if len(outBytes) > 0 {
		b.Write(outBytes)
		if !strings.HasSuffix(string(outBytes), "\n") {
			b.WriteByte('\n')
		}
	}
	if truncated {
		b.WriteString("...[output truncated at 100KB]...\n")
	}
	switch {
	case timedOut:
		fmt.Fprintf(&b, "[timed out after %s; process tree cancellation requested]\n", timeout)
	case canceled:
		b.WriteString("[canceled; process tree cancellation requested]\n")
	case exitCode == 0:
		b.WriteString("[exit 0]")
	default:
		fmt.Fprintf(&b, "[exit %d]", exitCode)
	}

	isError := timedOut || canceled || exitCode != 0
	return ToolResult{
		Content: b.String(),
		IsError: isError,
		Metadata: map[string]any{
			"exit_code": exitCode,
			"timed_out": timedOut,
			"canceled":  canceled,
			"truncated": truncated,
			"cwd":       cwd,
		},
	}, nil
}

// buildShellCommand picks the right invocation per OS. We deliberately use
// the shell rather than direct argv splitting so the LLM can use pipes,
// redirects, env-var expansion, etc. — closer to what a developer would type.
func buildShellCommand(ctx context.Context, command string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	procrun.ConfigureTreeCancel(cmd)
	return cmd
}
