package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/packetcode/packetcode/internal/tools"
)

// McpTool adapts a single MCP server tool to the tools.Tool interface
// so it can be registered alongside built-in tools and exercised by the
// agent loop.
type McpTool struct {
	client     *Client
	serverName string
	toolName   string
	desc       string
	schema     json.RawMessage
}

// NewMcpTool constructs an adapter for the given (client, serverTool)
// pair. The exposed tool name is always "<server>.<tool>".
func NewMcpTool(c *Client, t ServerTool) *McpTool {
	schema := t.InputSchema
	if len(bytes.TrimSpace(schema)) == 0 {
		// Default to an empty object schema so providers that require a
		// non-null parameter spec are happy.
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return &McpTool{
		client:     c,
		serverName: c.Name(),
		toolName:   t.Name,
		desc:       t.Description,
		schema:     schema,
	}
}

// Name returns "<server>.<tool>" — MCP tools are always prefixed.
func (t *McpTool) Name() string { return t.serverName + "." + t.toolName }

// Description returns the server-supplied description (may be empty).
func (t *McpTool) Description() string { return t.desc }

// Schema returns the inputSchema verbatim from the server.
func (t *McpTool) Schema() json.RawMessage { return t.schema }

// RequiresApproval is always true for MCP tools — the server is
// untrusted and any tool may have side effects. Trust mode auto-approves.
func (t *McpTool) RequiresApproval() bool { return true }

// Execute forwards the call to the underlying client and flattens the
// content array into a tools.ToolResult.
func (t *McpTool) Execute(ctx context.Context, params json.RawMessage) (tools.ToolResult, error) {
	if !t.client.IsAlive() {
		return tools.ToolResult{
			IsError: true,
			Content: fmt.Sprintf("MCP server %q has exited — restart packetcode to reconnect", t.serverName),
		}, nil
	}

	args := params
	trimmed := bytes.TrimSpace(args)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		args = json.RawMessage("{}")
	}

	res, err := t.client.CallTool(ctx, t.toolName, args)
	if err != nil {
		if errors.Is(err, ErrServerExited) {
			return tools.ToolResult{
				IsError: true,
				Content: fmt.Sprintf("MCP server %q has exited — restart packetcode to reconnect", t.serverName),
			}, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return tools.ToolResult{}, err
		}
		return tools.ToolResult{
			IsError: true,
			Content: fmt.Sprintf("%s.%s: %s", t.serverName, t.toolName, err),
		}, nil
	}

	parts := make([]string, 0, len(res.Content))
	for _, item := range res.Content {
		switch item.Type {
		case "text":
			parts = append(parts, item.Text)
		default:
			parts = append(parts, fmt.Sprintf("[%s content omitted]", item.Type))
		}
	}
	return tools.ToolResult{
		Content: strings.Join(parts, "\n"),
		IsError: res.IsError,
	}, nil
}
