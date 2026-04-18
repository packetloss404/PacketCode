package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

const readFileSchema = `{
  "type": "object",
  "properties": {
    "path":       { "type": "string", "description": "File path relative to the project root" },
    "start_line": { "type": "integer", "description": "First line to include (1-indexed, inclusive). Optional." },
    "end_line":   { "type": "integer", "description": "Last line to include (1-indexed, inclusive). Optional." }
  },
  "required": ["path"]
}`

type ReadFileTool struct {
	Root string
}

func NewReadFileTool(root string) *ReadFileTool { return &ReadFileTool{Root: root} }

func (*ReadFileTool) Name() string             { return "read_file" }
func (*ReadFileTool) RequiresApproval() bool   { return false }
func (*ReadFileTool) Schema() json.RawMessage  { return json.RawMessage(readFileSchema) }
func (*ReadFileTool) Description() string {
	return "Read the contents of a file from the project. Optional start_line/end_line restrict the slice returned. Output is prefixed with line numbers."
}

type readFileParams struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

func (t *ReadFileTool) Execute(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
	var p readFileParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return ToolResult{}, fmt.Errorf("read_file: parse params: %w", err)
	}
	abs, err := resolveInRoot(t.Root, p.Path)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}, nil
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("read_file: %s", err), IsError: true}, nil
	}
	if !utf8.Valid(data) {
		return ToolResult{
			Content: fmt.Sprintf("read_file: %s appears to be binary (%d bytes); refusing to render", p.Path, len(data)),
			IsError: true,
		}, nil
	}

	lines := strings.Split(string(data), "\n")
	// Trim a trailing empty line caused by a final '\n' so line counts
	// match what tools like `wc -l` report.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	start := 1
	end := len(lines)
	if p.StartLine > 0 {
		start = p.StartLine
	}
	if p.EndLine > 0 && p.EndLine < end {
		end = p.EndLine
	}
	if start > end {
		return ToolResult{
			Content: fmt.Sprintf("read_file: start_line (%d) is past end_line (%d)", start, end),
			IsError: true,
		}, nil
	}
	if start > len(lines) {
		return ToolResult{
			Content: fmt.Sprintf("read_file: start_line (%d) is past end of file (%d lines)", start, len(lines)),
			IsError: true,
		}, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s (lines %d-%d of %d)\n", p.Path, start, end, len(lines))
	for i := start; i <= end; i++ {
		fmt.Fprintf(&b, "%5d | %s\n", i, lines[i-1])
	}
	return ToolResult{
		Content: b.String(),
		Metadata: map[string]any{
			"path":        p.Path,
			"total_lines": len(lines),
			"start_line":  start,
			"end_line":    end,
		},
	}, nil
}
