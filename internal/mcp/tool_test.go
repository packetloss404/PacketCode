package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMcpTool_AdaptsNameAsProviderSafe asserts the adapter exposes
// a provider-safe public name while retaining the original MCP tool for
// execution.
func TestMcpTool_AdaptsNameAsProviderSafe(t *testing.T) {
	stub := makeBasicStub(t, "fs", []ServerTool{
		{Name: "read_file", Description: "read a file", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}, nil)
	defer stub.Stop()
	cli, err := NewClientWithStub("fs", stub, stubInfo, 5)
	require.NoError(t, err)
	defer cli.Close(time.Second)

	mt := NewMcpTool(cli, cli.Tools()[0])
	assert.Equal(t, "fs__read_file", mt.Name())
	assert.Equal(t, "read a file", mt.Description())
	assert.True(t, mt.RequiresApproval())
}

func TestMcpTool_SafeNameSanitizesAndCaps(t *testing.T) {
	got := safeToolName("my.server", "read/file")
	assert.Equal(t, "my_server__read_file", got)

	long := safeToolName(strings.Repeat("server", 20), strings.Repeat("tool", 20))
	assert.LessOrEqual(t, len(long), 64)
	assert.NotContains(t, long, ".")
	assert.NotContains(t, long, "/")
}

// TestMcpTool_Execute_DeadClient asserts that calls against a dead
// client surface as an IsError ToolResult, not as a Go-level error.
func TestMcpTool_Execute_DeadClient(t *testing.T) {
	stub := makeBasicStub(t, "fs", []ServerTool{{Name: "x"}}, nil)
	cli, err := NewClientWithStub("fs", stub, stubInfo, 5)
	require.NoError(t, err)
	mt := NewMcpTool(cli, cli.Tools()[0])

	stub.CloseStdout()
	// Wait briefly for the reader to register EOF.
	for i := 0; i < 50 && cli.IsAlive(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	require.False(t, cli.IsAlive())

	res, err := mt.Execute(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, res.Content, "MCP server")
	assert.Contains(t, res.Content, "exited")
	stub.Stop()
}

// TestMcpTool_Execute_FlattensContent asserts:
//   - text items are joined with '\n'
//   - non-text items render as "[<type> content omitted]"
func TestMcpTool_Execute_FlattensContent(t *testing.T) {
	stub := makeBasicStub(t, "srv", []ServerTool{{Name: "many"}}, map[string]StubHandler{
		"tools/call": func(_ json.RawMessage) (any, *ErrorObj) {
			return map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "hello"},
					{"type": "image"},
					{"type": "text", "text": "world"},
				},
				"isError": false,
			}, nil
		},
	})
	defer stub.Stop()
	cli, err := NewClientWithStub("srv", stub, stubInfo, 5)
	require.NoError(t, err)
	defer cli.Close(time.Second)
	mt := NewMcpTool(cli, cli.Tools()[0])

	res, err := mt.Execute(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "hello\n[image content omitted]\nworld", res.Content)
}

// TestMcpTool_Execute_NullParams asserts that a literal `null` (or
// empty) params blob is rewritten to `{}` before being forwarded.
func TestMcpTool_Execute_NullParams(t *testing.T) {
	captured := atomic.Value{}
	stub := makeBasicStub(t, "srv", []ServerTool{{Name: "t"}}, map[string]StubHandler{
		"tools/call": func(params json.RawMessage) (any, *ErrorObj) {
			var p struct {
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(params, &p)
			captured.Store(string(p.Arguments))
			return map[string]any{"content": []any{}, "isError": false}, nil
		},
	})
	defer stub.Stop()
	cli, err := NewClientWithStub("srv", stub, stubInfo, 5)
	require.NoError(t, err)
	defer cli.Close(time.Second)
	mt := NewMcpTool(cli, cli.Tools()[0])

	_, err = mt.Execute(context.Background(), json.RawMessage(`null`))
	require.NoError(t, err)
	assert.Equal(t, "{}", captured.Load())

	_, err = mt.Execute(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "{}", captured.Load())
}

// TestMcpTool_Execute_CtxCancellation asserts that ctx cancellation
// surfaces as a Go error so the agent loop can abort the turn.
func TestMcpTool_Execute_CtxCancellation(t *testing.T) {
	hold := make(chan struct{})
	stub := makeBasicStub(t, "srv", []ServerTool{{Name: "blocky"}}, map[string]StubHandler{
		"tools/call": func(_ json.RawMessage) (any, *ErrorObj) {
			<-hold
			return map[string]any{"content": []any{}, "isError": false}, nil
		},
	})
	defer func() { close(hold); stub.Stop() }()

	cli, err := NewClientWithStub("srv", stub, stubInfo, 5)
	require.NoError(t, err)
	defer cli.Close(time.Second)
	mt := NewMcpTool(cli, cli.Tools()[0])

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err = mt.Execute(ctx, json.RawMessage(`{}`))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "context canceled"),
		"unexpected error: %v", err)
}

// TestMcpTool_Schema_PassesThrough asserts the inputSchema is forwarded
// to callers verbatim.
func TestMcpTool_Schema_PassesThrough(t *testing.T) {
	custom := json.RawMessage(`{"type":"object","properties":{"x":{"type":"integer"}},"required":["x"]}`)
	stub := makeBasicStub(t, "srv", []ServerTool{
		{Name: "t", InputSchema: custom},
	}, nil)
	defer stub.Stop()
	cli, err := NewClientWithStub("srv", stub, stubInfo, 5)
	require.NoError(t, err)
	defer cli.Close(time.Second)
	mt := NewMcpTool(cli, cli.Tools()[0])
	assert.JSONEq(t, string(custom), string(mt.Schema()))
}
