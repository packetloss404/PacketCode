// Package tools defines the agent's executable tool set: read_file,
// write_file, patch_file, execute_command, search_codebase, list_directory.
//
// Each tool is a self-contained Tool implementation. The agent loop wires
// the registry into the LLM provider as a list of ToolDefinition values
// and dispatches incoming tool calls back to Execute().
//
// Approval semantics: any Tool with RequiresApproval()==true must be gated
// by the TUI's approval prompt before Execute is called. The Tool itself
// does not enforce this — the caller does.
package tools

import (
	"context"
	"encoding/json"
)

// Tool is the contract implemented by every agent-callable action.
type Tool interface {
	Name() string
	Description() string
	// Schema returns the JSON Schema document describing the tool's
	// parameters. The agent loop forwards this to LLM providers as the
	// tool's parameter definition.
	Schema() json.RawMessage
	// Execute runs the tool. params is the JSON arguments object emitted
	// by the LLM. The returned ToolResult is sent back to the LLM as a
	// tool-role message.
	Execute(ctx context.Context, params json.RawMessage) (ToolResult, error)
	// RequiresApproval reports whether this tool's invocations must be
	// confirmed by the user before Execute is called. Read-only tools
	// (read_file, search_codebase, list_directory) return false; anything
	// that mutates the filesystem or shells out returns true.
	RequiresApproval() bool
}

// ToolResult is what gets serialized back to the LLM. Content should be a
// human-readable string the model can reason about; IsError flags model
// errors (file not found, command failed) so the LLM can adjust its plan.
type ToolResult struct {
	Content  string
	IsError  bool
	Metadata map[string]any
}
