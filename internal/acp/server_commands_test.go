package acp

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- _packetcode/commands/list -------------------------------------------

type staticCommandCatalog struct {
	mu       sync.Mutex
	commands []CommandInfo
	err      error
	cwds     []string
}

func (c *staticCommandCatalog) ListCommands(cwd string) ([]CommandInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cwds = append(c.cwds, cwd)
	return c.commands, c.err
}

func (c *staticCommandCatalog) recordedCWDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.cwds...)
}

func TestServerCommandsListExtension(t *testing.T) {
	catalog := &staticCommandCatalog{commands: []CommandInfo{
		{Name: "audit", Description: "Security-review the diff", Source: "user"},
		{Name: "deploy", Description: "Ship to an environment", Source: "project", ArgumentHint: "[arguments]", Body: "Deploy to $ARGUMENTS."},
	}}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetCommandCatalog(catalog) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	extension := object(t, capabilities["_packetcode"])
	assert.Equal(t, true, extension["commandsList"])
	assert.Equal(t, false, extension["projectFiles"])

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/commands/list",
		"params": map[string]any{"cwd": "/proj/gui"},
	})
	reply := client.receiveID(2)
	require.Nil(t, reply["error"], "commands/list should succeed: %v", reply["error"])
	commands, ok := object(t, reply["result"])["commands"].([]any)
	require.True(t, ok)
	require.Len(t, commands, 2)

	first := object(t, commands[0])
	assert.Equal(t, "audit", first["name"])
	assert.Equal(t, "Security-review the diff", first["description"])
	assert.Equal(t, "user", first["source"])
	_, hasHint := first["argumentHint"]
	assert.False(t, hasHint, "argumentHint must be omitted when empty")

	second := object(t, commands[1])
	assert.Equal(t, "deploy", second["name"])
	assert.Equal(t, "project", second["source"])
	assert.Equal(t, "[arguments]", second["argumentHint"])
	_, hasBody := second["body"]
	assert.False(t, hasBody, "Body is server-side only and must not be serialized")

	// The client's cwd reaches the catalog so project commands can be found.
	assert.Equal(t, []string{"/proj/gui"}, catalog.recordedCWDs())
}

func TestServerCommandsListWithoutCwdIsAllowed(t *testing.T) {
	catalog := &staticCommandCatalog{}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetCommandCatalog(catalog) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	client.receiveID(1)

	// A missing cwd is not an error: it scopes the answer to user commands.
	client.send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "_packetcode/commands/list", "params": map[string]any{}})
	reply := client.receiveID(2)
	require.Nil(t, reply["error"])
	// A nil catalog slice must serialize as [] rather than null.
	commands, ok := object(t, reply["result"])["commands"].([]any)
	require.True(t, ok, "commands must be a JSON array, got %#v", object(t, reply["result"])["commands"])
	assert.Empty(t, commands)
	assert.Equal(t, []string{""}, catalog.recordedCWDs())
}

func TestServerCommandsListErrorIsInternal(t *testing.T) {
	catalog := &staticCommandCatalog{err: fmt.Errorf("read commands dir: permission denied")}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetCommandCatalog(catalog) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	client.receiveID(1)

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/commands/list",
		"params": map[string]any{"cwd": "/proj"},
	})
	errObj := object(t, client.receiveID(2)["error"])
	assert.Equal(t, json.Number("-32603"), errObj["code"])
	assert.Contains(t, errObj["message"], "list commands")
	assert.Contains(t, errObj["message"], "permission denied")
}

func TestServerCommandsListWithoutCatalogIsMethodNotFound(t *testing.T) {
	client := newTestClient(t, blockingFactory{})
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	extension := object(t, capabilities["_packetcode"])
	assert.Equal(t, false, extension["commandsList"])

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/commands/list",
		"params": map[string]any{"cwd": "/proj"},
	})
	assert.Equal(t, json.Number("-32601"), object(t, client.receiveID(2)["error"])["code"])
}

// --- slash expansion in session/prompt ------------------------------------

func TestServerExpandSlashCommand(t *testing.T) {
	server := NewServer(strings.NewReader(""), io.Discard, io.Discard, blockingFactory{}, "test")
	catalog := &staticCommandCatalog{commands: []CommandInfo{
		{Name: "deploy", Source: "project", Body: "Deploy to $ARGUMENTS and report the result."},
		{Name: "audit", Source: "user", Body: "Security-review the working tree."},
	}}
	server.SetCommandCatalog(catalog)

	// Arguments substitute into $ARGUMENTS.
	assert.Equal(t, "Deploy to staging and report the result.",
		server.expandSlashCommand("/deploy staging", "/proj"))
	// A bare invocation leaves $ARGUMENTS empty rather than literal.
	assert.Equal(t, "Deploy to  and report the result.",
		server.expandSlashCommand("/deploy", "/proj"))
	// A body with no placeholder ignores trailing text.
	assert.Equal(t, "Security-review the working tree.",
		server.expandSlashCommand("/audit please", "/proj"))
	// Unknown verbs, absolute paths, and multi-line prompts pass through.
	assert.Equal(t, "/unknown thing", server.expandSlashCommand("/unknown thing", "/proj"))
	assert.Equal(t, "/usr/bin/env is missing", server.expandSlashCommand("/usr/bin/env is missing", "/proj"))
	assert.Equal(t, "/deploy staging\nand hurry", server.expandSlashCommand("/deploy staging\nand hurry", "/proj"))
	assert.Equal(t, "no slash here", server.expandSlashCommand("no slash here", "/proj"))

	// The session's cwd is what scopes discovery.
	recorded := catalog.recordedCWDs()
	require.NotEmpty(t, recorded)
	for _, cwd := range recorded {
		assert.Equal(t, "/proj", cwd)
	}
}

func TestServerExpandSlashCommandWithoutCatalogIsInert(t *testing.T) {
	server := NewServer(strings.NewReader(""), io.Discard, io.Discard, blockingFactory{}, "test")
	assert.Equal(t, "/deploy staging", server.expandSlashCommand("/deploy staging", "/proj"))
}

func TestServerExpandSlashCommandSurvivesCatalogError(t *testing.T) {
	server := NewServer(strings.NewReader(""), io.Discard, io.Discard, blockingFactory{}, "test")
	server.SetCommandCatalog(&staticCommandCatalog{err: fmt.Errorf("boom")})
	// A broken catalog must never swallow the user's prompt.
	assert.Equal(t, "/deploy staging", server.expandSlashCommand("/deploy staging", "/proj"))
}

// --- _packetcode/project/files --------------------------------------------

type staticProjectFileIndex struct {
	mu    sync.Mutex
	files []string
	err   error
	calls [][3]any
}

func (i *staticProjectFileIndex) SearchFiles(cwd, query string, limit int) ([]string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.calls = append(i.calls, [3]any{cwd, query, limit})
	return i.files, i.err
}

func (i *staticProjectFileIndex) recorded() [][3]any {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([][3]any(nil), i.calls...)
}

func TestServerProjectFilesExtension(t *testing.T) {
	index := &staticProjectFileIndex{files: []string{"src/App.tsx", "src/components/Composer.tsx"}}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetProjectFileIndex(index) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	extension := object(t, capabilities["_packetcode"])
	assert.Equal(t, true, extension["projectFiles"])
	assert.Equal(t, false, extension["commandsList"])

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/project/files",
		"params": map[string]any{"cwd": "/proj/gui", "query": "compo", "limit": 5},
	})
	reply := client.receiveID(2)
	require.Nil(t, reply["error"], "project/files should succeed: %v", reply["error"])
	files, ok := object(t, reply["result"])["files"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"src/App.tsx", "src/components/Composer.tsx"}, files)
	assert.Equal(t, [][3]any{{"/proj/gui", "compo", 5}}, index.recorded())
}

func TestServerProjectFilesLimitDefaultsAndCaps(t *testing.T) {
	index := &staticProjectFileIndex{}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetProjectFileIndex(index) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	client.receiveID(1)

	// Absent limit takes the default.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/project/files",
		"params": map[string]any{"cwd": "/proj"},
	})
	reply := client.receiveID(2)
	// A nil result slice must serialize as [] rather than null.
	files, ok := object(t, reply["result"])["files"].([]any)
	require.True(t, ok, "files must be a JSON array")
	assert.Empty(t, files)

	// Negative limits also take the default.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "_packetcode/project/files",
		"params": map[string]any{"cwd": "/proj", "limit": -4},
	})
	client.receiveID(3)

	// An oversized limit is clamped to the ceiling.
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "_packetcode/project/files",
		"params": map[string]any{"cwd": "/proj", "limit": 100000},
	})
	client.receiveID(4)

	assert.Equal(t, [][3]any{
		{"/proj", "", defaultProjectFilesLimit},
		{"/proj", "", defaultProjectFilesLimit},
		{"/proj", "", maxProjectFilesLimit},
	}, index.recorded())
}

func TestServerProjectFilesRequiresCwd(t *testing.T) {
	index := &staticProjectFileIndex{}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetProjectFileIndex(index) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	client.receiveID(1)

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/project/files",
		"params": map[string]any{"cwd": "   ", "query": "x"},
	})
	errObj := object(t, client.receiveID(2)["error"])
	assert.Equal(t, json.Number("-32602"), errObj["code"])
	assert.Equal(t, "cwd is required", errObj["message"])
	assert.Empty(t, index.recorded())
}

func TestServerProjectFilesErrorIsInternal(t *testing.T) {
	index := &staticProjectFileIndex{err: fmt.Errorf("stat: no such directory")}
	client := newTestClientConfigured(t, blockingFactory{}, func(s *Server) { s.SetProjectFileIndex(index) })
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	client.receiveID(1)

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/project/files",
		"params": map[string]any{"cwd": "/nope"},
	})
	errObj := object(t, client.receiveID(2)["error"])
	assert.Equal(t, json.Number("-32603"), errObj["code"])
	assert.Contains(t, errObj["message"], "search project files")
	assert.Contains(t, errObj["message"], "no such directory")
}

func TestServerProjectFilesWithoutIndexIsMethodNotFound(t *testing.T) {
	client := newTestClient(t, blockingFactory{})
	defer client.close()

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := client.receiveID(1)
	capabilities := object(t, object(t, initialized["result"])["agentCapabilities"])
	extension := object(t, capabilities["_packetcode"])
	assert.Equal(t, false, extension["projectFiles"])

	client.send(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "_packetcode/project/files",
		"params": map[string]any{"cwd": "/proj"},
	})
	assert.Equal(t, json.Number("-32601"), object(t, client.receiveID(2)["error"])["code"])
}
