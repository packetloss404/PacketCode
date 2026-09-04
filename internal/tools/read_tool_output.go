package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/packetcode/packetcode/internal/toolout"
)

// maxEchoedHandle bounds how much of a model-supplied handle is quoted back in
// a miss. The argument is untrusted text; echoing it whole would let a tool
// result be padded with arbitrary model-authored content.
const maxEchoedHandle = 48

// ReadToolOutputTool retrieves more of a tool result that was too large to send
// to the model whole.
//
// It is read-only, never approval-gated, and cannot return an unbounded result:
// the store clamps every page to toolout.MaxPageBytes, so the tool that exists
// to recover truncated output can never itself blow the context budget that
// truncation protects.
type ReadToolOutputTool struct {
	store *toolout.Store
}

// NewReadToolOutputTool builds the tool over store. A nil store is legal and
// makes every lookup a graceful miss, which is what a session with spilling
// disabled should look like to the model.
func NewReadToolOutputTool(store *toolout.Store) *ReadToolOutputTool {
	return &ReadToolOutputTool{store: store}
}

func (t *ReadToolOutputTool) Name() string { return "read_tool_output" }

func (t *ReadToolOutputTool) Description() string {
	return fmt.Sprintf(
		"Reads more of a large tool result that was shown to you truncated. When a tool result contains a packetcode truncation marker naming a handle, call this with that handle to retrieve the omitted bytes. offset is a byte offset into the complete output (the marker states the omitted range); limit is capped at %d bytes per call. Continue from the next_offset reported by each call until it says end of output. Handles belong to this session only and are not files or paths — an unknown or expired handle simply reports that the output is no longer retained, in which case re-run the original tool.",
		toolout.MaxPageBytes,
	)
}

func (t *ReadToolOutputTool) Schema() json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "handle": {
      "type": "string",
      "description": "Opaque handle from a truncation marker, e.g. out_1a2b3c... Not a file path."
    },
    "offset": {
      "type": "integer",
      "description": "Byte offset into the complete tool output. Defaults to 0.",
      "minimum": 0
    },
    "limit": {
      "type": "integer",
      "description": "Maximum bytes to return. Defaults to %d, capped at %d.",
      "minimum": 1
    }
  },
  "required": ["handle"],
  "additionalProperties": false
}`, toolout.DefaultPageBytes, toolout.MaxPageBytes))
}

// RequiresApproval is false: this reads back bytes a tool already produced in
// this session and that the user has already seen in full in the transcript.
// There is nothing new to approve, and prompting would train reflexive
// approval for a no-op.
func (t *ReadToolOutputTool) RequiresApproval() bool { return false }

type readToolOutputParams struct {
	Handle string `json:"handle"`
	Offset int64  `json:"offset"`
	Limit  int    `json:"limit"`
}

func (t *ReadToolOutputTool) Execute(_ context.Context, params json.RawMessage) (ToolResult, error) {
	var p readToolOutputParams
	if err := json.Unmarshal(params, &p); err != nil {
		return ToolResult{IsError: true, Content: "read_tool_output: invalid arguments; pass {\"handle\": \"out_...\", \"offset\": 0}"}, nil
	}
	handle := strings.TrimSpace(p.Handle)
	page, ok := t.store.Read(handle, p.Offset, p.Limit)
	if !ok {
		// A miss is a normal result, not an error: a handle whose output was
		// pruned must not stall a turn, and an unknown handle must look
		// identical to a pruned one so nothing can be probed through it.
		return ToolResult{Content: fmt.Sprintf(
			"tool output %s is no longer retained. Handles expire when the session ends or the buffer recycles, and are not valid across sessions or background agents. Re-run the original tool if you still need its output.",
			quoteHandle(handle),
		)}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[tool output %s: bytes %d-%d of %d", page.Handle, page.Offset, page.Next, page.Total)
	if page.EOF {
		b.WriteString("; end of output]\n")
	} else {
		fmt.Fprintf(&b, "; continue with offset %d]\n", page.Next)
	}
	b.WriteString(page.Text)
	return ToolResult{Content: b.String()}, nil
}

func quoteHandle(handle string) string {
	if handle == "" {
		return "(no handle supplied)"
	}
	if len(handle) > maxEchoedHandle {
		handle = handle[:maxEchoedHandle] + "..."
	}
	return fmt.Sprintf("%q", handle)
}
