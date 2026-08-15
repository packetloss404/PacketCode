package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
)

func TestServerNativeAgentPermissionAndEvents(t *testing.T) {
	workspace := t.TempDir()
	factory := &nativeTestFactory{dir: t.TempDir(), provider: &scriptedProvider{}}
	client := newTestClient(t, factory)
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 99, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	result := object(t, initialized["result"])
	assert.Equal(t, json.Number("1"), result["protocolVersion"])
	capabilities := object(t, result["agentCapabilities"])
	assert.Equal(t, true, capabilities["loadSession"])

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "session/new",
		"params": map[string]any{"cwd": "relative/path", "mcpServers": []any{}},
	})
	badCWD := client.receiveID(2)
	assert.Equal(t, json.Number("-32602"), object(t, badCWD["error"])["code"])

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "session/new",
		"params": map[string]any{"cwd": workspace, "mcpServers": []any{}},
	})
	newSession := client.receiveID(3)
	sessionID := object(t, newSession["result"])["sessionId"].(string)
	require.NotEmpty(t, sessionID)

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "session/prompt",
		"params": map[string]any{
			"sessionId": sessionID,
			"prompt": []any{
				map[string]any{"type": "text", "text": "Run the test tool"},
				map[string]any{"type": "resource_link", "uri": "file:///tmp/context.txt", "name": "context.txt"},
			},
		},
	})

	messages := make([]map[string]any, 0, 12)
	var permission map[string]any
	for permission == nil {
		msg := client.receive()
		messages = append(messages, msg)
		if msg["method"] == "session/request_permission" {
			permission = msg
		}
	}
	require.Equal(t, "packetcode-permission-1", permission["id"])
	permissionParams := object(t, permission["params"])
	permissionTool := object(t, permissionParams["toolCall"])
	assert.Equal(t, "call-42", permissionTool["toolCallId"])
	assert.Equal(t, "execute", permissionTool["kind"])

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": permission["id"],
		"result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "allow_once"}},
	})
	var promptResponse map[string]any
	for promptResponse == nil {
		msg := client.receive()
		messages = append(messages, msg)
		if idEqual(msg["id"], 4) {
			promptResponse = msg
		}
	}
	assert.Equal(t, "end_turn", object(t, promptResponse["result"])["stopReason"])
	assert.Equal(t, 2, factory.provider.callCount())
	assert.Equal(t, 1, factory.executions())

	updates := updatesFrom(t, messages)
	assertUpdate(t, updates, "plan", "", "in_progress")
	assertUpdate(t, updates, "tool_call", "call-42", "pending")
	assertUpdate(t, updates, "tool_call_update", "call-42", "in_progress")
	assertUpdate(t, updates, "tool_call_update", "call-42", "completed")
	assertUpdate(t, updates, "agent_message_chunk", "", "")
	assertUpdate(t, updates, "plan", "", "completed")

	client.send(map[string]any{"jsonrpc": "2.0", "id": 5, "method": "session/load", "params": map[string]any{}})
	load := client.receiveID(5)
	assert.Equal(t, json.Number("-32602"), object(t, load["error"])["code"])
	client.send(map[string]any{"jsonrpc": "2.0", "id": 6, "method": "packetcode/unknown", "params": map[string]any{}})
	unknown := client.receiveID(6)
	assert.Equal(t, json.Number("-32601"), object(t, unknown["error"])["code"])
}

// replayFactory answers resume requests with canned history so tests can
// assert the session/load replay wire behavior without a real provider.
type replayFactory struct {
	history []provider.Message
	mu      sync.Mutex
	lastCfg SessionConfig
}

func (f *replayFactory) NewSession(_ context.Context, cfg SessionConfig, _ agent.Approver) (*Runtime, error) {
	f.mu.Lock()
	f.lastCfg = cfg
	f.mu.Unlock()
	if cfg.SessionID == "" {
		return &Runtime{ID: "fresh-session", Runner: blockingRunner{}}, nil
	}
	if cfg.SessionID == "missing-session" {
		return nil, fmt.Errorf("load session %s: file does not exist", cfg.SessionID)
	}
	return &Runtime{ID: cfg.SessionID, Runner: blockingRunner{}, History: f.history}, nil
}

func (f *replayFactory) lastConfig() SessionConfig {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastCfg
}

func TestServerLoadSessionReplaysHistoryBeforeResponse(t *testing.T) {
	workspace := t.TempDir()
	factory := &replayFactory{history: []provider.Message{
		{Role: provider.RoleSystem, Content: "you are packetcode"},
		{Role: provider.RoleUser, Content: "hello"},
		{Role: provider.RoleAssistant, Content: "checking", ToolCalls: []provider.ToolCall{{ID: "call-1", Name: "read_file", Arguments: "{}"}}},
		{Role: provider.RoleTool, ToolCallID: "call-1", Name: "read_file", Content: "package main"},
		{Role: provider.RoleAssistant, Content: ""}, // tool-only turn: no text to replay
		{Role: provider.RoleAssistant, Content: "all done"},
	}}
	client := newTestClient(t, factory)
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	assert.Equal(t, true, capabilities["loadSession"])

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": "load", "method": "session/load",
		"params": map[string]any{"sessionId": "resumed-1", "cwd": workspace, "mcpServers": []any{}},
	})

	// Every replay update must arrive strictly before the session/load result.
	type replayed struct{ kind, text string }
	var got []replayed
	messageIDs := map[string]bool{}
	var response map[string]any
	for response == nil {
		msg := client.receive()
		if update, ok := sessionUpdate(msg); ok {
			params := object(t, msg["params"])
			assert.Equal(t, "resumed-1", params["sessionId"])
			content := object(t, update["content"])
			got = append(got, replayed{update["sessionUpdate"].(string), content["text"].(string)})
			id, _ := update["messageId"].(string)
			assert.NotEmpty(t, id, "replayed chunks carry a messageId")
			messageIDs[id] = true
			continue
		}
		if idEqual(msg["id"], "load") {
			response = msg
		}
	}
	assert.Len(t, messageIDs, 3, "each replayed message gets its own messageId")
	require.Nil(t, response["error"], "session/load should succeed: %v", response["error"])
	assert.Equal(t, map[string]any{}, object(t, response["result"]))
	assert.Equal(t, []replayed{
		{"user_message_chunk", "hello"},
		{"agent_message_chunk", "checking"},
		{"agent_message_chunk", "all done"},
	}, got, "replay is text turns only, in stored order")

	cfg := factory.lastConfig()
	assert.Equal(t, "resumed-1", cfg.SessionID)
	assert.Equal(t, workspace, cfg.CWD)

	// The resumed session is registered: prompting it works.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": "prompt", "method": "session/prompt",
		"params": map[string]any{"sessionId": "resumed-1", "prompt": []any{map[string]any{"type": "text", "text": "continue"}}},
	})
	for {
		msg := client.receive()
		if update, ok := sessionUpdate(msg); ok && update["sessionUpdate"] == "tool_call" {
			break
		}
	}
	client.send(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]any{"sessionId": "resumed-1"}})
	for {
		msg := client.receive()
		if idEqual(msg["id"], "prompt") {
			break
		}
	}

	// Re-loading an idle session on the same connection replaces its runtime
	// and replays again — clients switch between sessions and come back.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": "again", "method": "session/load",
		"params": map[string]any{"sessionId": "resumed-1", "cwd": workspace, "mcpServers": []any{}},
	})
	replayCount := 0
	var again map[string]any
	for again == nil {
		msg := client.receive()
		if update, ok := sessionUpdate(msg); ok {
			kind, _ := update["sessionUpdate"].(string)
			if kind == "user_message_chunk" || kind == "agent_message_chunk" {
				replayCount++
			}
			continue
		}
		if idEqual(msg["id"], "again") {
			again = msg
		}
	}
	require.Nil(t, again["error"], "re-load of an idle session should succeed")
	assert.Equal(t, 3, replayCount, "second load replays the full transcript again")
}

func TestServerLoadSessionWhileActivePromptIsBusy(t *testing.T) {
	workspace := t.TempDir()
	factory := &replayFactory{history: []provider.Message{{Role: provider.RoleUser, Content: "hi"}}}
	client := newTestClient(t, factory)
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	client.receiveID(1)

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": "load", "method": "session/load",
		"params": map[string]any{"sessionId": "resumed-busy", "cwd": workspace, "mcpServers": []any{}},
	})
	var loaded map[string]any
	for loaded == nil {
		msg := client.receive()
		if idEqual(msg["id"], "load") {
			loaded = msg
		}
	}
	require.Nil(t, loaded["error"])

	// Blocking prompt keeps the session active; a load during it must be busy.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": "prompt", "method": "session/prompt",
		"params": map[string]any{"sessionId": "resumed-busy", "prompt": []any{map[string]any{"type": "text", "text": "go"}}},
	})
	for {
		msg := client.receive()
		if update, ok := sessionUpdate(msg); ok && update["sessionUpdate"] == "tool_call" {
			break
		}
	}
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": "busy-load", "method": "session/load",
		"params": map[string]any{"sessionId": "resumed-busy", "cwd": workspace, "mcpServers": []any{}},
	})
	var busy map[string]any
	for busy == nil {
		msg := client.receive()
		if idEqual(msg["id"], "busy-load") {
			busy = msg
		}
	}
	assert.Equal(t, json.Number("-32000"), object(t, busy["error"])["code"])

	client.send(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]any{"sessionId": "resumed-busy"}})
	for {
		msg := client.receive()
		if idEqual(msg["id"], "prompt") {
			assert.Equal(t, "cancelled", object(t, msg["result"])["stopReason"])
			break
		}
	}
}

func TestServerLoadSessionFactoryErrorIsInternal(t *testing.T) {
	workspace := t.TempDir()
	client := newTestClient(t, &replayFactory{})
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	client.receiveID(1)

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "session/load",
		"params": map[string]any{"sessionId": "missing-session", "cwd": workspace, "mcpServers": []any{}},
	})
	reply := client.receiveID(2)
	errObj := object(t, reply["error"])
	assert.Equal(t, json.Number("-32603"), errObj["code"])
	assert.Contains(t, errObj["message"], "load PacketCode session")

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "session/load",
		"params": map[string]any{"sessionId": "x", "cwd": "relative/path", "mcpServers": []any{}},
	})
	badCWD := client.receiveID(3)
	assert.Equal(t, json.Number("-32602"), object(t, badCWD["error"])["code"])
}

type staticSessionLister struct {
	summaries []SessionSummary
	err       error
}

func (l *staticSessionLister) ListSessions() ([]SessionSummary, error) {
	return l.summaries, l.err
}

func TestServerSessionsListExtension(t *testing.T) {
	updated := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	lister := &staticSessionLister{summaries: []SessionSummary{{
		SessionID: "abc-123", Name: "wire acp", UpdatedAt: updated,
		Provider: "codex", Model: "gpt-5.3", WorkingDir: "/proj/gui",
		MessageCount: 4, CostUSD: 1.25,
	}}}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetSessionLister(lister) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	extension := object(t, capabilities["_packetcode"])
	assert.Equal(t, true, extension["sessionsList"])

	client.send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "_packetcode/sessions/list", "params": map[string]any{}})
	reply := client.receiveID(2)
	sessions, ok := object(t, reply["result"])["sessions"].([]any)
	require.True(t, ok)
	require.Len(t, sessions, 1)
	entry := object(t, sessions[0])
	assert.Equal(t, "abc-123", entry["sessionId"])
	assert.Equal(t, "wire acp", entry["name"])
	assert.Equal(t, "codex", entry["provider"])
	assert.Equal(t, "/proj/gui", entry["workingDir"])
	assert.Equal(t, json.Number("4"), entry["messageCount"])
}

func TestServerSessionsListWithoutListerIsMethodNotFound(t *testing.T) {
	client := newTestClient(t, blockingFactory{})
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	extension := object(t, capabilities["_packetcode"])
	assert.Equal(t, false, extension["sessionsList"])

	client.send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "_packetcode/sessions/list", "params": map[string]any{}})
	reply := client.receiveID(2)
	assert.Equal(t, json.Number("-32601"), object(t, reply["error"])["code"])
}

type recordingSessionRenamer struct {
	mu    sync.Mutex
	calls [][2]string
	err   error
}

func (r *recordingSessionRenamer) RenameSession(id, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, [2]string{id, name})
	return r.err
}

func (r *recordingSessionRenamer) recorded() [][2]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][2]string(nil), r.calls...)
}

func TestServerSessionsRenameExtension(t *testing.T) {
	renamer := &recordingSessionRenamer{}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetSessionRenamer(renamer) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	extension := object(t, capabilities["_packetcode"])
	assert.Equal(t, true, extension["sessionsRename"])
	assert.Equal(t, false, extension["sessionsList"])

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/sessions/rename",
		"params": map[string]any{"sessionId": "abc-123", "name": "wire acp titles"},
	})
	reply := client.receiveID(2)
	require.Nil(t, reply["error"], "rename should succeed: %v", reply["error"])
	assert.Equal(t, [][2]string{{"abc-123", "wire acp titles"}}, renamer.recorded())
}

func TestServerSessionsRenameValidation(t *testing.T) {
	renamer := &recordingSessionRenamer{}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetSessionRenamer(renamer) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	client.receiveID(1)

	// Empty name: invalid params, and the renamer is never invoked.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/sessions/rename",
		"params": map[string]any{"sessionId": "abc-123", "name": "   "},
	})
	emptyName := client.receiveID(2)
	assert.Equal(t, json.Number("-32602"), object(t, emptyName["error"])["code"])

	// Empty session id: invalid params as well.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "_packetcode/sessions/rename",
		"params": map[string]any{"sessionId": "", "name": "titled"},
	})
	emptyID := client.receiveID(3)
	assert.Equal(t, json.Number("-32602"), object(t, emptyID["error"])["code"])
	assert.Empty(t, renamer.recorded())
}

func TestServerSessionsRenameUnknownSessionIsInternal(t *testing.T) {
	renamer := &recordingSessionRenamer{err: fmt.Errorf("load session: file does not exist")}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetSessionRenamer(renamer) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	client.receiveID(1)

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/sessions/rename",
		"params": map[string]any{"sessionId": "missing", "name": "titled"},
	})
	reply := client.receiveID(2)
	errObj := object(t, reply["error"])
	assert.Equal(t, json.Number("-32603"), errObj["code"])
	assert.Contains(t, errObj["message"], "rename session")
	assert.Contains(t, errObj["message"], "file does not exist")
}

func TestServerSessionsRenameWithoutRenamerIsMethodNotFound(t *testing.T) {
	client := newTestClient(t, blockingFactory{})
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	extension := object(t, capabilities["_packetcode"])
	assert.Equal(t, false, extension["sessionsRename"])

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/sessions/rename",
		"params": map[string]any{"sessionId": "abc-123", "name": "titled"},
	})
	reply := client.receiveID(2)
	assert.Equal(t, json.Number("-32601"), object(t, reply["error"])["code"])
}

type staticModelCatalog struct {
	models []ModelOption
	err    error
}

func (c *staticModelCatalog) ListModels() ([]ModelOption, error) {
	return c.models, c.err
}

func TestServerModelsListExtension(t *testing.T) {
	catalog := &staticModelCatalog{models: []ModelOption{
		{Provider: "codex", Model: "gpt-5.3", Default: true},
		{Provider: "anthropic", Model: "claude-fable-5"},
	}}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetModelCatalog(catalog) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	extension := object(t, capabilities["_packetcode"])
	assert.Equal(t, true, extension["modelsList"])
	assert.Equal(t, false, extension["sessionsList"])

	client.send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "_packetcode/models/list", "params": map[string]any{}})
	reply := client.receiveID(2)
	models, ok := object(t, reply["result"])["models"].([]any)
	require.True(t, ok)
	require.Len(t, models, 2)
	first := object(t, models[0])
	assert.Equal(t, "codex", first["provider"])
	assert.Equal(t, "gpt-5.3", first["model"])
	assert.Equal(t, true, first["default"])
	second := object(t, models[1])
	assert.Equal(t, "anthropic", second["provider"])
	assert.Equal(t, "claude-fable-5", second["model"])
	assert.Equal(t, false, second["default"])
}

func TestServerModelsListWithoutCatalogIsMethodNotFound(t *testing.T) {
	client := newTestClient(t, blockingFactory{})
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	extension := object(t, capabilities["_packetcode"])
	assert.Equal(t, false, extension["modelsList"])

	client.send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "_packetcode/models/list", "params": map[string]any{}})
	reply := client.receiveID(2)
	assert.Equal(t, json.Number("-32601"), object(t, reply["error"])["code"])
}

// recordingFactory captures the SessionConfig session/new hands to the
// factory and rejects providers and permission modes outside its allow-lists
// the way the production factory does.
type recordingFactory struct {
	knownProviders []string

	mu      sync.Mutex
	configs []SessionConfig
}

func (f *recordingFactory) NewSession(_ context.Context, cfg SessionConfig, _ agent.Approver) (*Runtime, error) {
	if cfg.Provider != "" {
		known := false
		for _, slug := range f.knownProviders {
			if slug == cfg.Provider {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("%w %q", ErrUnknownProvider, cfg.Provider)
		}
	}
	if cfg.PermissionMode != "" {
		known := false
		for _, mode := range PermissionModes {
			if mode == cfg.PermissionMode {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("%w %q", ErrUnknownPermissionMode, cfg.PermissionMode)
		}
	}
	f.mu.Lock()
	f.configs = append(f.configs, cfg)
	id := fmt.Sprintf("recorded-session-%d", len(f.configs))
	f.mu.Unlock()
	return &Runtime{ID: id, Runner: blockingRunner{}}, nil
}

func (f *recordingFactory) recorded() []SessionConfig {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SessionConfig(nil), f.configs...)
}

func TestServerNewSessionPacketcodeOverride(t *testing.T) {
	workspace := t.TempDir()
	factory := &recordingFactory{knownProviders: []string{"anthropic", "ollama"}}
	client := newTestClient(t, factory)
	defer client.close()

	client.send(map[string]any{"jsonrpc": "2.0", "id": "init", "method": "initialize", "params": map[string]any{"protocolVersion": 1}})
	client.receiveID("init")

	// No override: the factory sees empty Provider/Model.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "session/new",
		"params": map[string]any{"cwd": workspace, "mcpServers": []any{}},
	})
	plain := client.receiveID(1)
	require.NotEmpty(t, object(t, plain["result"])["sessionId"])

	// Override: provider and model reach the factory verbatim.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "session/new",
		"params": map[string]any{
			"cwd": workspace, "mcpServers": []any{},
			"_packetcode": map[string]any{"provider": "anthropic", "model": "claude-fable-5"},
		},
	})
	overridden := client.receiveID(2)
	require.NotEmpty(t, object(t, overridden["result"])["sessionId"])

	// Unknown provider: invalid-params, and the factory records no session.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "session/new",
		"params": map[string]any{
			"cwd": workspace, "mcpServers": []any{},
			"_packetcode": map[string]any{"provider": "nonexistent", "model": "whatever"},
		},
	})
	rejected := client.receiveID(3)
	assert.Equal(t, json.Number("-32602"), object(t, rejected["error"])["code"])

	configs := factory.recorded()
	require.Len(t, configs, 2)
	assert.Equal(t, "", configs[0].Provider)
	assert.Equal(t, "", configs[0].Model)
	assert.Equal(t, "anthropic", configs[1].Provider)
	assert.Equal(t, "claude-fable-5", configs[1].Model)
}

func TestServerNewSessionPermissionModeOverride(t *testing.T) {
	workspace := t.TempDir()
	factory := &recordingFactory{knownProviders: []string{"anthropic"}}
	client := newTestClient(t, factory)
	defer client.close()

	client.send(map[string]any{"jsonrpc": "2.0", "id": "init", "method": "initialize", "params": map[string]any{"protocolVersion": 1}})
	initialized := client.receiveID("init")
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	extension := object(t, capabilities["_packetcode"])
	advertised, ok := extension["permissionModes"].([]any)
	require.True(t, ok, "initialize must advertise _packetcode.permissionModes")
	modes := make([]string, 0, len(advertised))
	for _, mode := range advertised {
		modes = append(modes, mode.(string))
	}
	assert.Equal(t, []string{"ask", "accept-edits", "auto", "read-only", "bypass"}, modes)

	// No override: the factory sees an empty PermissionMode.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "session/new",
		"params": map[string]any{"cwd": workspace, "mcpServers": []any{}},
	})
	plain := client.receiveID(1)
	require.NotEmpty(t, object(t, plain["result"])["sessionId"])

	// Override: the mode reaches the factory verbatim, alone or alongside a
	// provider/model override.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "session/new",
		"params": map[string]any{
			"cwd": workspace, "mcpServers": []any{},
			"_packetcode": map[string]any{"permissionMode": "accept-edits"},
		},
	})
	overridden := client.receiveID(2)
	require.NotEmpty(t, object(t, overridden["result"])["sessionId"])

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "session/new",
		"params": map[string]any{
			"cwd": workspace, "mcpServers": []any{},
			"_packetcode": map[string]any{"provider": "anthropic", "model": "claude-fable-5", "permissionMode": "bypass"},
		},
	})
	combined := client.receiveID(3)
	require.NotEmpty(t, object(t, combined["result"])["sessionId"])

	// Unknown mode: invalid-params, and the factory records no session.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "session/new",
		"params": map[string]any{
			"cwd": workspace, "mcpServers": []any{},
			"_packetcode": map[string]any{"permissionMode": "yolo"},
		},
	})
	rejected := client.receiveID(4)
	assert.Equal(t, json.Number("-32602"), object(t, rejected["error"])["code"])

	configs := factory.recorded()
	require.Len(t, configs, 3)
	assert.Equal(t, "", configs[0].PermissionMode)
	assert.Equal(t, "accept-edits", configs[1].PermissionMode)
	assert.Equal(t, "bypass", configs[2].PermissionMode)
	assert.Equal(t, "anthropic", configs[2].Provider)
	assert.Equal(t, "claude-fable-5", configs[2].Model)
}

func TestServerCancelReturnsTerminalCancelledAndFailsOpenTool(t *testing.T) {
	workspace := t.TempDir()
	factory := blockingFactory{}
	client := newTestClient(t, factory)
	defer client.close()

	client.send(map[string]any{"jsonrpc": "2.0", "id": "init", "method": "initialize", "params": map[string]any{"protocolVersion": 1}})
	client.receiveID("init")
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": "new", "method": "session/new",
		"params": map[string]any{"cwd": workspace, "mcpServers": []any{}},
	})
	sessionID := object(t, client.receiveID("new")["result"])["sessionId"].(string)
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": "prompt", "method": "session/prompt",
		"params": map[string]any{"sessionId": sessionID, "prompt": []any{map[string]any{"type": "text", "text": "wait"}}},
	})

	// Wait until the runner has emitted its pending tool so cancellation also
	// exercises terminalization of in-flight tool state.
	for {
		msg := client.receive()
		if update, ok := sessionUpdate(msg); ok && update["sessionUpdate"] == "tool_call" {
			break
		}
	}
	client.send(map[string]any{
		"jsonrpc": "2.0", "method": "session/cancel",
		"params": map[string]any{"sessionId": sessionID},
	})
	seenFailed := false
	for {
		msg := client.receive()
		if update, ok := sessionUpdate(msg); ok && update["sessionUpdate"] == "tool_call_update" {
			if update["toolCallId"] == "blocked-call" && update["status"] == "failed" {
				seenFailed = true
			}
		}
		if idEqual(msg["id"], "prompt") {
			assert.Equal(t, "cancelled", object(t, msg["result"])["stopReason"])
			break
		}
	}
	assert.True(t, seenFailed, "cancelled prompt should terminalize its open tool call")
}

func TestServerCancelDominatesLateAllow(t *testing.T) {
	workspace := t.TempDir()
	factory := &nativeTestFactory{dir: t.TempDir(), provider: &scriptedProvider{}}
	client := newTestClient(t, factory)
	defer client.close()

	client.send(map[string]any{"jsonrpc": "2.0", "id": "init", "method": "initialize", "params": map[string]any{"protocolVersion": 1}})
	client.receiveID("init")
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": "new", "method": "session/new",
		"params": map[string]any{"cwd": workspace, "mcpServers": []any{}},
	})
	sessionID := object(t, client.receiveID("new")["result"])["sessionId"].(string)
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": "prompt", "method": "session/prompt",
		"params": map[string]any{"sessionId": sessionID, "prompt": []any{map[string]any{"type": "text", "text": "Run the test tool"}}},
	})
	var permission map[string]any
	for permission == nil {
		msg := client.receive()
		if msg["method"] == "session/request_permission" {
			permission = msg
		}
	}
	client.send(map[string]any{
		"jsonrpc": "2.0", "method": "session/cancel",
		"params": map[string]any{"sessionId": sessionID},
	})
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": permission["id"],
		"result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "allow_once"}},
	})
	for {
		msg := client.receive()
		if idEqual(msg["id"], "prompt") {
			assert.Equal(t, "cancelled", object(t, msg["result"])["stopReason"])
			break
		}
	}
	assert.Equal(t, 0, factory.executions(), "a late allow must not execute after cancellation")
}

type nativeTestFactory struct {
	dir      string
	provider *scriptedProvider
	mu       sync.Mutex
	executed int
}

func (f *nativeTestFactory) NewSession(_ context.Context, cfg SessionConfig, approver agent.Approver) (*Runtime, error) {
	reg := provider.NewRegistry()
	reg.Register(f.provider)
	requireModel := "fake-model"
	if err := reg.SetActive(f.provider.Slug(), requireModel); err != nil {
		return nil, err
	}
	toolRegistry := tools.NewRegistry()
	toolRegistry.Register(&approvalTestTool{run: func() {
		f.mu.Lock()
		f.executed++
		f.mu.Unlock()
	}})
	manager := session.NewManager(filepath.Join(f.dir, "sessions"))
	created, err := manager.New(f.provider.Slug(), requireModel)
	if err != nil {
		return nil, err
	}
	runner := agent.New(agent.Config{
		Registry: reg, Tools: toolRegistry, Session: manager, Approver: approver,
		Policy: permissions.Must(config.PermissionConfig{Profile: "ask"}),
	})
	return &Runtime{ID: created.ID, Runner: runner}, nil
}

func (f *nativeTestFactory) executions() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.executed
}

type scriptedProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *scriptedProvider) Name() string                              { return "Fake" }
func (p *scriptedProvider) Slug() string                              { return "fake" }
func (p *scriptedProvider) BrandColor() lipgloss.Color                { return lipgloss.Color("#000000") }
func (p *scriptedProvider) ValidateKey(context.Context, string) error { return nil }
func (p *scriptedProvider) ListModels(context.Context) ([]provider.Model, error) {
	return []provider.Model{{ID: "fake-model", ContextWindow: 1000, SupportsTools: true}}, nil
}
func (p *scriptedProvider) Pricing(string) (float64, float64) { return 0, 0 }
func (p *scriptedProvider) ContextWindow(string) int          { return 1000 }
func (p *scriptedProvider) SupportsTools(string) bool         { return true }
func (p *scriptedProvider) ChatCompletion(ctx context.Context, _ provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	events := make(chan provider.StreamEvent, 5)
	if call == 1 {
		events <- provider.StreamEvent{Type: provider.EventToolCallStart, ToolCall: &provider.ToolCallDelta{Index: 0, ID: "call-42", Name: "execute_command"}}
		events <- provider.StreamEvent{Type: provider.EventToolCallDelta, ToolCall: &provider.ToolCallDelta{Index: 0, ArgumentsDelta: `{"command":"echo test"}`}}
		events <- provider.StreamEvent{Type: provider.EventToolCallEnd, ToolCall: &provider.ToolCallDelta{Index: 0}}
		events <- provider.StreamEvent{Type: provider.EventDone}
	} else {
		events <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: "complete"}
		events <- provider.StreamEvent{Type: provider.EventDone}
	}
	close(events)
	return events, nil
}

func (p *scriptedProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type approvalTestTool struct{ run func() }

func (t *approvalTestTool) Name() string        { return "execute_command" }
func (t *approvalTestTool) Description() string { return "fake command" }
func (t *approvalTestTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`)
}
func (t *approvalTestTool) RequiresApproval() bool { return true }
func (t *approvalTestTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	if t.run != nil {
		t.run()
	}
	return tools.ToolResult{Content: "command completed"}, nil
}

type blockingFactory struct{}

func (blockingFactory) NewSession(context.Context, SessionConfig, agent.Approver) (*Runtime, error) {
	return &Runtime{ID: "blocking-session", Runner: blockingRunner{}}, nil
}

type blockingRunner struct{}

func (blockingRunner) Run(ctx context.Context, _ string) <-chan agent.AgentEvent {
	events := make(chan agent.AgentEvent, 2)
	go func() {
		defer close(events)
		events <- agent.AgentEvent{Type: agent.EventToolCallProposed, ToolCall: provider.ToolCall{ID: "blocked-call", Name: "execute_command", Arguments: `{}`}}
		<-ctx.Done()
		events <- agent.AgentEvent{Type: agent.EventError, Error: ctx.Err()}
	}()
	return events
}

type testClient struct {
	t      *testing.T
	in     *io.PipeWriter
	lines  chan map[string]any
	errors chan error
	done   chan error
}

func newTestClient(t *testing.T, factory SessionFactory) *testClient {
	t.Helper()
	return newTestClientConfigured(t, factory, nil)
}

func newTestClientConfigured(t *testing.T, factory SessionFactory, configure func(*Server)) *testClient {
	t.Helper()
	serverIn, clientIn := io.Pipe()
	clientOut, serverOut := io.Pipe()
	client := &testClient{t: t, in: clientIn, lines: make(chan map[string]any, 32), errors: make(chan error, 1), done: make(chan error, 1)}
	server := NewServer(serverIn, serverOut, io.Discard, factory, "test")
	if configure != nil {
		configure(server)
	}
	go func() {
		client.done <- server.Serve(context.Background())
		_ = serverOut.Close()
	}()
	go func() {
		scanner := bufio.NewScanner(clientOut)
		for scanner.Scan() {
			decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
			decoder.UseNumber()
			var msg map[string]any
			if err := decoder.Decode(&msg); err != nil {
				client.errors <- err
				return
			}
			client.lines <- msg
		}
		client.errors <- scanner.Err()
	}()
	return client
}

func (c *testClient) send(value any) {
	c.t.Helper()
	data, err := json.Marshal(value)
	require.NoError(c.t, err)
	data = append(data, '\n')
	_, err = c.in.Write(data)
	require.NoError(c.t, err)
}

func (c *testClient) receive() map[string]any {
	c.t.Helper()
	select {
	case msg := <-c.lines:
		return msg
	case err := <-c.errors:
		require.NoError(c.t, err)
		return nil
	case <-time.After(3 * time.Second):
		c.t.Fatal("timed out waiting for ACP message")
		return nil
	}
}

func (c *testClient) receiveID(id any) map[string]any {
	c.t.Helper()
	for {
		msg := c.receive()
		if idEqual(msg["id"], id) {
			return msg
		}
	}
}

func (c *testClient) close() {
	c.t.Helper()
	_ = c.in.Close()
	select {
	case err := <-c.done:
		require.NoError(c.t, err)
	case <-time.After(3 * time.Second):
		c.t.Fatal("ACP server did not stop after stdin closed")
	}
}

func object(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	require.True(t, ok, "expected object, got %T", value)
	return result
}

func idEqual(got, want any) bool {
	return fmt.Sprint(got) == fmt.Sprint(want)
}

func sessionUpdate(msg map[string]any) (map[string]any, bool) {
	if msg["method"] != "session/update" {
		return nil, false
	}
	params, ok := msg["params"].(map[string]any)
	if !ok {
		return nil, false
	}
	update, ok := params["update"].(map[string]any)
	return update, ok
}

func updatesFrom(t *testing.T, messages []map[string]any) []map[string]any {
	t.Helper()
	updates := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		if update, ok := sessionUpdate(msg); ok {
			updates = append(updates, update)
		}
	}
	return updates
}

func assertUpdate(t *testing.T, updates []map[string]any, updateType, callID, status string) {
	t.Helper()
	for _, update := range updates {
		if update["sessionUpdate"] != updateType {
			continue
		}
		if callID != "" && update["toolCallId"] != callID {
			continue
		}
		if status != "" {
			if updateType == "plan" {
				entries, ok := update["entries"].([]any)
				if !ok || len(entries) == 0 || object(t, entries[0])["status"] != status {
					continue
				}
			} else if update["status"] != status {
				continue
			}
		}
		return
	}
	t.Errorf("missing %s update for call %q with status %q", updateType, callID, status)
}
