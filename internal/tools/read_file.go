package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/packetcode/packetcode/internal/computers"
)

const readFileSchema = `{
  "type": "object",
  "properties": {
    "path":       { "type": "string", "description": "File path relative to the project root" },
    "start_line": { "type": "integer", "description": "First line to include (1-indexed, inclusive). Optional." },
    "end_line":   { "type": "integer", "description": "Last line to include (1-indexed, inclusive). Optional. Output is capped at 400 lines." }
  },
  "required": ["path"]
}`

const (
	maxReadFileLines      = 400
	maxReadFileLineLength = 1024 * 1024
)

type ReadFileTool struct {
	Root       string
	Backend    computers.RuntimeBackend
	backendErr error
}

func NewReadFileTool(root string) *ReadFileTool {
	backend, err := computers.NewLocalBackend(root)
	return &ReadFileTool{Root: root, Backend: backend, backendErr: err}
}

func NewReadFileToolWithBackend(backend computers.RuntimeBackend) *ReadFileTool {
	if backend == nil {
		return &ReadFileTool{backendErr: fmt.Errorf("runtime backend is nil")}
	}
	return &ReadFileTool{Root: backend.Root(), Backend: backend}
}

func (*ReadFileTool) Name() string            { return "read_file" }
func (*ReadFileTool) RequiresApproval() bool  { return false }
func (*ReadFileTool) Schema() json.RawMessage { return json.RawMessage(readFileSchema) }
func (*ReadFileTool) Description() string {
	return "Read a file from the project. Optional start_line/end_line restrict the slice returned. Output is line-numbered and capped at 400 lines."
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
	if t.backendErr != nil {
		return ToolResult{Content: fmt.Sprintf("read_file: %s", t.backendErr), IsError: true}, nil
	}
	data, err := t.Backend.ReadFile(ctx, p.Path)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("read_file: %s", err), IsError: true}, nil
	}

	start, requestedEnd := 1, 0
	if p.StartLine > 0 {
		start = p.StartLine
	}
	if p.EndLine > 0 {
		requestedEnd = p.EndLine
	}
	if requestedEnd > 0 && start > requestedEnd {
		return ToolResult{
			Content: fmt.Sprintf("read_file: start_line (%d) is past end_line (%d)", start, requestedEnd),
			IsError: true,
		}, nil
	}
	if requestedEnd == 0 || requestedEnd-start+1 > maxReadFileLines {
		requestedEnd = start + maxReadFileLines - 1
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), maxReadFileLineLength)
	var lines []string
	total := 0
	for scanner.Scan() {
		total++
		line := scanner.Text()
		if !utf8.ValidString(line) {
			return ToolResult{
				Content: fmt.Sprintf("read_file: %s appears to be binary or invalid UTF-8; refusing to render", p.Path),
				IsError: true,
			}, nil
		}
		if total >= start && total <= requestedEnd {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return ToolResult{Content: fmt.Sprintf("read_file: %s", err), IsError: true}, nil
	}
	if start > total {
		return ToolResult{
			Content: fmt.Sprintf("read_file: start_line (%d) is past end of file (%d lines)", start, total),
			IsError: true,
		}, nil
	}

	end := start + len(lines) - 1
	truncated := requestedEnd < total
	var b strings.Builder
	fmt.Fprintf(&b, "%s (lines %d-%d of %d)\n", p.Path, start, end, total)
	for i, line := range lines {
		fmt.Fprintf(&b, "%5d | %s\n", start+i, line)
	}
	if truncated {
		fmt.Fprintf(&b, "... output truncated at %d lines; use start_line/end_line for another slice\n", maxReadFileLines)
	}
	return ToolResult{
		Content: b.String(),
		Metadata: map[string]any{
			"path":        p.Path,
			"total_lines": total,
			"start_line":  start,
			"end_line":    end,
			"truncated":   truncated,
		},
	}, nil
}
