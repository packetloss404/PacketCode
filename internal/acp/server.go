// Package acp exposes PacketCode's existing agent loop over the Agent Client
// Protocol v1 newline-delimited JSON-RPC transport.
package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/packetcode/packetcode/internal/agent"
	"github.com/packetcode/packetcode/internal/provider"
)

const ProtocolVersion = 1

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
	codeSessionBusy    = -32000
)

// MCPServer is the stdio MCP configuration supplied by an ACP client.
type MCPServer struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

// SessionConfig contains the workspace-scoped inputs needed to build a native
// PacketCode agent session. Provider, Model, and PermissionMode are optional
// per-session overrides from the session/new "_packetcode" params object;
// empty values mean "use the configured defaults".
type SessionConfig struct {
	CWD        string
	MCPServers []MCPServer
	// SessionID selects the resume path: when non-empty the factory must load
	// the persisted session with this ID (session/load) instead of creating a
	// new one, and populate Runtime.History with its transcript for replay.
	SessionID string
	Provider  string
	Model     string
	// PermissionMode selects the permission profile the session's policy is
	// built from. One of PermissionModes; empty keeps the server-wide policy.
	PermissionMode string
}

// PermissionModes is the wire vocabulary for the session/new "_packetcode"
// permissionMode override, advertised verbatim in the initialize
// agentCapabilities under _packetcode.permissionModes.
var PermissionModes = []string{"ask", "accept-edits", "auto", "read-only", "bypass"}

// ErrUnknownProvider marks a session/new provider override the factory does
// not recognize. Factories wrap it (errors.Is-compatible) so the server can
// answer invalid-params instead of a generic internal error.
var ErrUnknownProvider = errors.New("unknown provider")

// ErrUnknownPermissionMode marks a session/new permissionMode override the
// factory does not recognize. Factories wrap it (errors.Is-compatible) so the
// server can answer invalid-params instead of a generic internal error.
var ErrUnknownPermissionMode = errors.New("unknown permission mode")

// Runner is PacketCode's terminal-independent agent event source.
type Runner interface {
	Run(context.Context, string) <-chan agent.AgentEvent
}

// Runtime owns one independently persisted PacketCode session.
type Runtime struct {
	ID     string
	Runner Runner
	Close  func() error
	// History is the persisted transcript of a resumed session, in order.
	// Only factories answering a SessionConfig.SessionID resume request set
	// it; the server replays user/assistant text turns to the client before
	// answering session/load. Nil for fresh sessions.
	History []provider.Message
}

// SessionFactory lets the protocol package remain independent of provider and
// tool construction. The command package supplies the production factory;
// tests supply deterministic providers.
type SessionFactory interface {
	NewSession(context.Context, SessionConfig, agent.Approver) (*Runtime, error)
}

// SessionSummary is the wire projection served by the _packetcode/sessions/list
// extension method. Fields are additive; clients must tolerate new ones.
type SessionSummary struct {
	SessionID    string    `json:"sessionId"`
	Name         string    `json:"name"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	WorkingDir   string    `json:"workingDir,omitempty"`
	MessageCount int       `json:"messageCount"`
	CostUSD      float64   `json:"costUsd"`
}

// SessionLister supplies persisted session history for the
// _packetcode/sessions/list extension. Optional: when unset the method
// answers method-not-found, matching older servers.
type SessionLister interface {
	ListSessions() ([]SessionSummary, error)
}

// ModelOption is one selectable provider/model pair served by the
// _packetcode/models/list extension. Default marks the pair a new session
// uses when session/new carries no "_packetcode" override.
type ModelOption struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Default  bool   `json:"default"`
}

// ModelCatalog supplies the configured provider/model choices for the
// _packetcode/models/list extension. Optional: when unset the method
// answers method-not-found, matching older servers.
type ModelCatalog interface {
	ListModels() ([]ModelOption, error)
}

// Server is a single ACP stdio connection. It is safe for prompt workers and
// permission responses to use concurrently.
type Server struct {
	in      io.Reader
	out     io.Writer
	log     io.Writer
	factory SessionFactory
	version string
	lister  SessionLister
	catalog ModelCatalog

	writeMu          sync.Mutex
	stateMu          sync.Mutex
	sessions         map[string]*sessionState
	pending          map[string]chan rpcResponse
	initialized      bool
	ctx              context.Context
	wg               sync.WaitGroup
	nextPermissionID atomic.Uint64
	nextMessageID    atomic.Uint64
}

type sessionState struct {
	runtime  *Runtime
	approver *permissionApprover
	cwd      string

	mu        sync.Mutex
	active    bool
	cancelled bool
	cancel    context.CancelFunc
}

type permissionApprover struct {
	server    *Server
	sessionID string
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type rpcResponse struct {
	result json.RawMessage
	err    *rpcError
}

// NewServer constructs a newline-delimited ACP v1 server. stdout must be the
// protocol writer; diagnostics are written only to logWriter.
func NewServer(in io.Reader, out, logWriter io.Writer, factory SessionFactory, version string) *Server {
	if logWriter == nil {
		logWriter = io.Discard
	}
	if version == "" {
		version = "dev"
	}
	return &Server{
		in: in, out: out, log: logWriter, factory: factory, version: version,
		sessions: make(map[string]*sessionState),
		pending:  make(map[string]chan rpcResponse),
	}
}

// SetSessionLister enables the _packetcode/sessions/list extension. Must be
// called before Serve; a nil lister leaves the method unregistered.
func (s *Server) SetSessionLister(l SessionLister) {
	s.lister = l
}

// SetModelCatalog enables the _packetcode/models/list extension. Must be
// called before Serve; a nil catalog leaves the method unregistered.
func (s *Server) SetModelCatalog(c ModelCatalog) {
	s.catalog = c
}

// Serve processes ACP messages until stdin closes or ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	s.ctx = ctx
	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !json.Valid([]byte(line)) {
			s.sendError(nil, codeParseError, "Parse error", nil)
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			s.sendError(nil, codeInvalidRequest, "Invalid Request", nil)
			continue
		}
		if msg.JSONRPC != "2.0" || (msg.Method == "" && len(msg.ID) == 0) {
			s.sendError(idOrNull(msg.ID), codeInvalidRequest, "Invalid Request", nil)
			continue
		}
		if len(msg.ID) > 0 && !validID(msg.ID) {
			s.sendError(nil, codeInvalidRequest, "Invalid Request", nil)
			continue
		}
		if msg.Method == "" {
			s.handleResponse(msg)
			continue
		}
		s.handleRequest(msg)
	}

	s.shutdown()
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read ACP transport: %w", err)
	}
	return nil
}

func (s *Server) handleRequest(msg rpcMessage) {
	if msg.Method != "initialize" {
		s.stateMu.Lock()
		initialized := s.initialized
		s.stateMu.Unlock()
		if !initialized {
			s.replyError(msg, codeInvalidRequest, "initialize must be called first", nil)
			return
		}
	}

	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "session/new":
		s.handleNewSession(msg)
	case "session/load":
		s.handleLoadSession(msg)
	case "session/prompt":
		s.handlePrompt(msg)
	case "session/cancel":
		s.handleCancel(msg)
	case "_packetcode/sessions/list":
		s.handleSessionsList(msg)
	case "_packetcode/models/list":
		s.handleModelsList(msg)
	default:
		s.replyError(msg, codeMethodNotFound, "Method not found", nil)
	}
}

func (s *Server) handleInitialize(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring initialize notification; a request ID is required")
		return
	}
	var params struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := decodeParams(msg.Params, &params); err != nil || params.ProtocolVersion < 1 {
		s.replyError(msg, codeInvalidParams, "protocolVersion must be a positive integer", nil)
		return
	}
	s.stateMu.Lock()
	if s.initialized {
		s.stateMu.Unlock()
		s.replyError(msg, codeInvalidRequest, "initialize may only be called once", nil)
		return
	}
	s.initialized = true
	s.stateMu.Unlock()

	// ACP requires the agent to return its latest supported version when the
	// client's version is unsupported. The client then decides whether to close.
	s.sendResult(msg.ID, map[string]any{
		"protocolVersion": ProtocolVersion,
		"agentCapabilities": map[string]any{
			"loadSession": true,
			"promptCapabilities": map[string]bool{
				"image": false, "audio": false, "embeddedContext": false,
			},
			"mcpCapabilities":     map[string]bool{"http": false, "sse": false},
			"sessionCapabilities": map[string]any{},
			// Vendor extension surface; underscore-prefixed so spec-only
			// clients skip it. sessionsList gates _packetcode/sessions/list;
			// modelsList gates _packetcode/models/list.
			"_packetcode": map[string]any{
				"sessionsList":    s.lister != nil,
				"modelsList":      s.catalog != nil,
				"permissionModes": PermissionModes,
			},
		},
		"agentInfo":   map[string]string{"name": "packetcode", "title": "PacketCode", "version": s.version},
		"authMethods": []any{},
	})
}

func (s *Server) handleSessionsList(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring _packetcode/sessions/list notification; a request ID is required")
		return
	}
	if s.lister == nil {
		s.replyError(msg, codeMethodNotFound, "Method not found", nil)
		return
	}
	summaries, err := s.lister.ListSessions()
	if err != nil {
		s.replyError(msg, codeInternalError, fmt.Sprintf("list sessions: %v", err), nil)
		return
	}
	if summaries == nil {
		summaries = []SessionSummary{}
	}
	s.sendResult(msg.ID, map[string]any{"sessions": summaries})
}

func (s *Server) handleModelsList(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring _packetcode/models/list notification; a request ID is required")
		return
	}
	if s.catalog == nil {
		s.replyError(msg, codeMethodNotFound, "Method not found", nil)
		return
	}
	models, err := s.catalog.ListModels()
	if err != nil {
		s.replyError(msg, codeInternalError, fmt.Sprintf("list models: %v", err), nil)
		return
	}
	if models == nil {
		models = []ModelOption{}
	}
	s.sendResult(msg.ID, map[string]any{"models": models})
}

type wireMCPServer struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Env     []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"env"`
}

func (s *Server) handleNewSession(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring session/new notification; a request ID is required")
		return
	}
	var params struct {
		CWD                   string           `json:"cwd"`
		MCPServers            *[]wireMCPServer `json:"mcpServers"`
		AdditionalDirectories []string         `json:"additionalDirectories"`
		// Vendor extension: optional per-session provider/model override,
		// mirroring the _packetcode capability namespace.
		Packetcode *struct {
			Provider       string `json:"provider"`
			Model          string `json:"model"`
			PermissionMode string `json:"permissionMode"`
		} `json:"_packetcode"`
	}
	if err := decodeParams(msg.Params, &params); err != nil {
		s.replyError(msg, codeInvalidParams, "invalid session/new parameters", nil)
		return
	}
	if !filepath.IsAbs(params.CWD) {
		s.replyError(msg, codeInvalidParams, "cwd must be an absolute path", nil)
		return
	}
	cwd := filepath.Clean(params.CWD)
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		s.replyError(msg, codeInvalidParams, "cwd must be an existing directory", nil)
		return
	}
	if params.MCPServers == nil {
		s.replyError(msg, codeInvalidParams, "mcpServers is required", nil)
		return
	}
	if len(params.AdditionalDirectories) > 0 {
		s.replyError(msg, codeInvalidParams, "additionalDirectories are not supported", nil)
		return
	}
	mcpServers, err := parseMCPServers(*params.MCPServers)
	if err != nil {
		s.replyError(msg, codeInvalidParams, err.Error(), nil)
		return
	}
	if s.factory == nil {
		s.replyError(msg, codeInternalError, "session factory is not configured", nil)
		return
	}
	sessionConfig := SessionConfig{CWD: cwd, MCPServers: mcpServers}
	if params.Packetcode != nil {
		sessionConfig.Provider = strings.TrimSpace(params.Packetcode.Provider)
		sessionConfig.Model = strings.TrimSpace(params.Packetcode.Model)
		sessionConfig.PermissionMode = strings.TrimSpace(params.Packetcode.PermissionMode)
	}
	approver := &permissionApprover{server: s}
	runtime, err := s.factory.NewSession(s.ctx, sessionConfig, approver)
	if err != nil {
		if errors.Is(err, ErrUnknownProvider) || errors.Is(err, ErrUnknownPermissionMode) {
			s.replyError(msg, codeInvalidParams, err.Error(), nil)
			return
		}
		s.replyError(msg, codeInternalError, "create PacketCode session: "+err.Error(), nil)
		return
	}
	if runtime == nil || runtime.ID == "" || runtime.Runner == nil {
		if runtime != nil && runtime.Close != nil {
			_ = runtime.Close()
		}
		s.replyError(msg, codeInternalError, "session factory returned an invalid runtime", nil)
		return
	}
	approver.sessionID = runtime.ID
	state := &sessionState{runtime: runtime, approver: approver, cwd: cwd}
	s.stateMu.Lock()
	if _, exists := s.sessions[runtime.ID]; exists {
		s.stateMu.Unlock()
		if runtime.Close != nil {
			_ = runtime.Close()
		}
		s.replyError(msg, codeInternalError, "session factory returned a duplicate session ID", nil)
		return
	}
	s.sessions[runtime.ID] = state
	s.stateMu.Unlock()
	s.sendResult(msg.ID, map[string]string{"sessionId": runtime.ID})
}

// handleLoadSession resumes a persisted session per ACP session/load: it
// rebuilds the runtime bound to the stored transcript, replays every user and
// assistant text turn to the client as session/update notifications, and only
// then answers the request with an empty result. Tool-call machinery is not
// replayed in v1 — text turns carry the conversation.
func (s *Server) handleLoadSession(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring session/load notification; a request ID is required")
		return
	}
	var params struct {
		SessionID  string           `json:"sessionId"`
		CWD        string           `json:"cwd"`
		MCPServers *[]wireMCPServer `json:"mcpServers"`
	}
	if err := decodeParams(msg.Params, &params); err != nil {
		s.replyError(msg, codeInvalidParams, "invalid session/load parameters", nil)
		return
	}
	if strings.TrimSpace(params.SessionID) == "" {
		s.replyError(msg, codeInvalidParams, "sessionId is required", nil)
		return
	}
	if !filepath.IsAbs(params.CWD) {
		s.replyError(msg, codeInvalidParams, "cwd must be an absolute path", nil)
		return
	}
	cwd := filepath.Clean(params.CWD)
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		s.replyError(msg, codeInvalidParams, "cwd must be an existing directory", nil)
		return
	}
	if params.MCPServers == nil {
		s.replyError(msg, codeInvalidParams, "mcpServers is required", nil)
		return
	}
	mcpServers, err := parseMCPServers(*params.MCPServers)
	if err != nil {
		s.replyError(msg, codeInvalidParams, err.Error(), nil)
		return
	}
	if s.factory == nil {
		s.replyError(msg, codeInternalError, "session factory is not configured", nil)
		return
	}
	// Re-loading a session this connection already holds is allowed — clients
	// switch between sessions and expect a fresh replay each time — unless a
	// prompt is currently running on it. The old runtime is replaced.
	s.stateMu.Lock()
	existing := s.sessions[params.SessionID]
	s.stateMu.Unlock()
	if existing != nil {
		existing.mu.Lock()
		active := existing.active
		existing.mu.Unlock()
		if active {
			s.replyError(msg, codeSessionBusy, "session already has an active prompt", nil)
			return
		}
	}
	approver := &permissionApprover{server: s}
	runtime, err := s.factory.NewSession(s.ctx, SessionConfig{CWD: cwd, MCPServers: mcpServers, SessionID: params.SessionID}, approver)
	if err != nil {
		s.replyError(msg, codeInternalError, "load PacketCode session: "+err.Error(), nil)
		return
	}
	if runtime == nil || runtime.ID == "" || runtime.Runner == nil {
		if runtime != nil && runtime.Close != nil {
			_ = runtime.Close()
		}
		s.replyError(msg, codeInternalError, "session factory returned an invalid runtime", nil)
		return
	}
	if runtime.ID != params.SessionID {
		if runtime.Close != nil {
			_ = runtime.Close()
		}
		s.replyError(msg, codeInternalError, "session factory resumed a different session ID", nil)
		return
	}
	approver.sessionID = runtime.ID
	state := &sessionState{runtime: runtime, approver: approver, cwd: cwd}
	s.stateMu.Lock()
	s.sessions[runtime.ID] = state
	s.stateMu.Unlock()
	// Requests are handled serially, so nothing raced the replacement; the
	// superseded runtime (if any) just needs its resources released.
	if existing != nil && existing.runtime.Close != nil {
		if err := existing.runtime.Close(); err != nil {
			fmt.Fprintf(s.log, "packetcode acp: close superseded session %s: %v\n", runtime.ID, err)
		}
	}

	// ACP requires the full replay to reach the client before the session/load
	// response. Updates and the result share writeMu-serialized writes, so
	// emitting them here in order guarantees that.
	for _, message := range runtime.History {
		var kind string
		switch message.Role {
		case provider.RoleUser:
			kind = "user_message_chunk"
		case provider.RoleAssistant:
			kind = "agent_message_chunk"
		default:
			continue // system prompts and tool traffic are not replayed
		}
		if message.Content == "" {
			continue
		}
		s.sendUpdate(runtime.ID, map[string]any{
			"sessionUpdate": kind,
			"messageId":     fmt.Sprintf("packetcode-message-%d", s.nextMessageID.Add(1)),
			"content":       map[string]string{"type": "text", "text": message.Content},
		})
	}
	s.sendResult(msg.ID, map[string]any{})
}

func parseMCPServers(in []wireMCPServer) ([]MCPServer, error) {
	out := make([]MCPServer, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, server := range in {
		if server.Type != "stdio" {
			return nil, fmt.Errorf("MCP server %q uses unsupported transport %q", server.Name, server.Type)
		}
		if strings.TrimSpace(server.Name) == "" {
			return nil, errors.New("MCP server name is required")
		}
		if _, ok := seen[server.Name]; ok {
			return nil, fmt.Errorf("duplicate MCP server name %q", server.Name)
		}
		seen[server.Name] = struct{}{}
		if !filepath.IsAbs(server.Command) {
			return nil, fmt.Errorf("MCP server %q command must be an absolute path", server.Name)
		}
		env := make(map[string]string, len(server.Env))
		for _, item := range server.Env {
			if item.Name == "" {
				return nil, fmt.Errorf("MCP server %q has an empty environment variable name", server.Name)
			}
			if _, exists := env[item.Name]; exists {
				return nil, fmt.Errorf("MCP server %q repeats environment variable %q", server.Name, item.Name)
			}
			env[item.Name] = item.Value
		}
		out = append(out, MCPServer{Name: server.Name, Command: filepath.Clean(server.Command), Args: append([]string(nil), server.Args...), Env: env})
	}
	return out, nil
}

func (s *Server) handlePrompt(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring session/prompt notification; a request ID is required")
		return
	}
	var params struct {
		SessionID string            `json:"sessionId"`
		Prompt    []json.RawMessage `json:"prompt"`
	}
	if err := decodeParams(msg.Params, &params); err != nil || params.SessionID == "" || params.Prompt == nil {
		s.replyError(msg, codeInvalidParams, "invalid session/prompt parameters", nil)
		return
	}
	prompt, err := promptText(params.Prompt)
	if err != nil {
		s.replyError(msg, codeInvalidParams, err.Error(), nil)
		return
	}
	state := s.session(params.SessionID)
	if state == nil {
		s.replyError(msg, codeInvalidParams, "unknown sessionId", nil)
		return
	}
	state.mu.Lock()
	if state.active {
		state.mu.Unlock()
		s.replyError(msg, codeSessionBusy, "session already has an active prompt", nil)
		return
	}
	turnCtx, cancel := context.WithCancel(s.ctx)
	state.active = true
	state.cancelled = false
	state.cancel = cancel
	state.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runPrompt(turnCtx, msg.ID, params.SessionID, prompt, state)
	}()
}

func promptText(blocks []json.RawMessage) (string, error) {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(block, &header); err != nil {
			return "", errors.New("prompt contains an invalid content block")
		}
		switch header.Type {
		case "text":
			var textBlock struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(block, &textBlock); err != nil {
				return "", errors.New("prompt contains invalid text content")
			}
			parts = append(parts, textBlock.Text)
		case "resource_link":
			var link struct {
				Type        string `json:"type"`
				URI         string `json:"uri"`
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			if err := json.Unmarshal(block, &link); err != nil || link.URI == "" || link.Name == "" {
				return "", errors.New("prompt contains an invalid resource_link")
			}
			reference := fmt.Sprintf("[Resource: %s]\nURI: %s", link.Name, link.URI)
			if link.Description != "" {
				reference += "\nDescription: " + link.Description
			}
			parts = append(parts, reference)
		default:
			return "", fmt.Errorf("unsupported prompt content type %q", header.Type)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

func (s *Server) runPrompt(ctx context.Context, requestID json.RawMessage, sessionID, prompt string, state *sessionState) {
	messageID := fmt.Sprintf("packetcode-message-%d", s.nextMessageID.Add(1))
	planContent := "Process the user prompt"
	s.sendUpdate(sessionID, map[string]any{
		"sessionUpdate": "plan",
		"entries":       []map[string]string{{"content": planContent, "priority": "medium", "status": "in_progress"}},
	})

	openCalls := make(map[string]provider.ToolCall)
	output := make(map[string]string)
	var runErr error
	done := false
	for ev := range state.runtime.Runner.Run(ctx, prompt) {
		switch ev.Type {
		case agent.EventTextDelta:
			s.sendUpdate(sessionID, map[string]any{
				"sessionUpdate": "agent_message_chunk", "messageId": messageID,
				"content": map[string]string{"type": "text", "text": ev.Text},
			})
		case agent.EventReasoningDelta:
			s.sendUpdate(sessionID, map[string]any{
				"sessionUpdate": "agent_thought_chunk", "messageId": messageID,
				"content": map[string]string{"type": "text", "text": ev.Text},
			})
		case agent.EventToolCallProposed:
			openCalls[ev.ToolCall.ID] = ev.ToolCall
			s.sendUpdate(sessionID, toolCallStart(ev.ToolCall))
		case agent.EventToolCallApproved:
			s.sendUpdate(sessionID, map[string]any{
				"sessionUpdate": "tool_call_update", "toolCallId": ev.ToolCall.ID, "status": "in_progress",
			})
		case agent.EventToolCallRejected:
			delete(openCalls, ev.ToolCall.ID)
			s.sendUpdate(sessionID, toolCallResult(ev.ToolCall.ID, ev.Text, true, map[string]any{"rejected": true, "reason": ev.Text}))
		case agent.EventToolOutputChunk:
			const maxPreview = 64 * 1024
			preview := output[ev.CallID] + ev.Chunk
			if len(preview) > maxPreview {
				preview = preview[len(preview)-maxPreview:]
			}
			output[ev.CallID] = preview
			s.sendUpdate(sessionID, map[string]any{
				"sessionUpdate": "tool_call_update", "toolCallId": ev.CallID,
				"content": toolTextContent(preview), "status": "in_progress",
			})
		case agent.EventToolCallExecuted:
			delete(openCalls, ev.ToolCall.ID)
			delete(output, ev.ToolCall.ID)
			raw := map[string]any{"content": ev.ToolResult.Content, "isError": ev.ToolResult.IsError}
			if ev.ToolResult.Metadata != nil {
				raw["metadata"] = ev.ToolResult.Metadata
			}
			s.sendUpdate(sessionID, toolCallResult(ev.ToolCall.ID, ev.ToolResult.Content, ev.ToolResult.IsError, raw))
		case agent.EventDone:
			done = true
		case agent.EventError:
			runErr = ev.Error
		}
	}

	state.mu.Lock()
	cancelled := state.cancelled || ctx.Err() != nil
	state.cancel = nil
	state.mu.Unlock()
	// active stays set until every trailing update and the prompt response have
	// been written, so a session/load landing mid-teardown is rejected as busy
	// instead of interleaving its replay with this prompt's final updates.
	defer func() {
		state.mu.Lock()
		state.active = false
		state.mu.Unlock()
	}()

	if cancelled || runErr != nil {
		for id := range openCalls {
			reason := "tool call cancelled"
			raw := map[string]any{"cancelled": true}
			if !cancelled {
				reason = "tool call failed"
				raw = map[string]any{"error": reason}
			}
			s.sendUpdate(sessionID, toolCallResult(id, reason, true, raw))
		}
	}
	if cancelled {
		// Plan entries have no cancelled state in ACP v1. Replace the complete
		// plan with an empty list so clients do not retain an in-progress task.
		s.sendUpdate(sessionID, map[string]any{"sessionUpdate": "plan", "entries": []any{}})
		s.sendResult(requestID, map[string]string{"stopReason": "cancelled"})
		return
	}
	if runErr != nil {
		s.sendUpdate(sessionID, map[string]any{"sessionUpdate": "plan", "entries": []any{}})
		s.sendError(requestID, codeInternalError, runErr.Error(), nil)
		return
	}
	if !done {
		s.sendError(requestID, codeInternalError, "agent event stream ended without a terminal event", nil)
		return
	}
	s.sendUpdate(sessionID, map[string]any{
		"sessionUpdate": "plan",
		"entries":       []map[string]string{{"content": planContent, "priority": "medium", "status": "completed"}},
	})
	s.sendResult(requestID, map[string]string{"stopReason": "end_turn"})
}

func toolCallStart(call provider.ToolCall) map[string]any {
	rawInput := map[string]any{}
	if err := json.Unmarshal([]byte(call.Arguments), &rawInput); err != nil {
		rawInput = map[string]any{"unparsed": call.Arguments}
	}
	return map[string]any{
		"sessionUpdate": "tool_call", "toolCallId": call.ID,
		"title": call.Name, "kind": toolKind(call.Name), "status": "pending", "rawInput": rawInput,
	}
}

func toolCallResult(id, content string, failed bool, raw map[string]any) map[string]any {
	status := "completed"
	if failed {
		status = "failed"
	}
	return map[string]any{
		"sessionUpdate": "tool_call_update", "toolCallId": id, "status": status,
		"content": toolTextContent(content), "rawOutput": raw,
	}
}

func toolTextContent(text string) []map[string]any {
	return []map[string]any{{"type": "content", "content": map[string]string{"type": "text", "text": text}}}
}

func toolKind(name string) string {
	switch name {
	case "read_file", "list_directory", "list_symbols", "find_definition", "find_references", "get_diagnostics":
		return "read"
	case "search_codebase":
		return "search"
	case "write_file", "patch_file":
		return "edit"
	case "execute_command":
		return "execute"
	default:
		return "other"
	}
}

func (s *Server) handleCancel(msg rpcMessage) {
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if err := decodeParams(msg.Params, &params); err != nil || params.SessionID == "" {
		s.replyError(msg, codeInvalidParams, "invalid session/cancel parameters", nil)
		return
	}
	state := s.session(params.SessionID)
	if state == nil {
		s.replyError(msg, codeInvalidParams, "unknown sessionId", nil)
		return
	}
	state.mu.Lock()
	if state.active {
		state.cancelled = true
		if state.cancel != nil {
			state.cancel()
		}
	}
	state.mu.Unlock()
	// session/cancel is an ACP notification. A non-standard request still
	// receives an empty result so a caller cannot hang indefinitely.
	if len(msg.ID) > 0 {
		s.sendResult(msg.ID, map[string]any{})
	}
}

func (a *permissionApprover) Approve(ctx context.Context, req agent.ApprovalRequest) agent.ApprovalDecision {
	if a == nil || a.server == nil || a.sessionID == "" {
		return agent.ApprovalDecision{Approved: false, Reason: "ACP permission channel unavailable"}
	}
	id := fmt.Sprintf("packetcode-permission-%d", a.server.nextPermissionID.Add(1))
	rawInput := map[string]any{}
	if err := json.Unmarshal(req.Params, &rawInput); err != nil {
		rawInput = map[string]any{"unparsed": string(req.Params)}
	}
	params := map[string]any{
		"sessionId": a.sessionID,
		"toolCall": map[string]any{
			"toolCallId": req.ToolCall.ID, "title": req.ToolCall.Name,
			"kind": toolKind(req.ToolCall.Name), "status": "pending", "rawInput": rawInput,
		},
		"options": []map[string]string{
			{"optionId": "allow_once", "name": "Allow once", "kind": "allow_once"},
			{"optionId": "reject_once", "name": "Reject", "kind": "reject_once"},
		},
	}
	response, err := a.server.callClient(ctx, id, "session/request_permission", params)
	if err != nil {
		return agent.ApprovalDecision{Approved: false, Reason: err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return agent.ApprovalDecision{Approved: false, Reason: "ACP permission request cancelled"}
	}
	var result struct {
		Outcome struct {
			Outcome  string `json:"outcome"`
			OptionID string `json:"optionId"`
		} `json:"outcome"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return agent.ApprovalDecision{Approved: false, Reason: "invalid ACP permission response"}
	}
	if result.Outcome.Outcome != "selected" {
		return agent.ApprovalDecision{Approved: false, Reason: "ACP permission request cancelled"}
	}
	switch result.Outcome.OptionID {
	case "allow_once":
		return agent.ApprovalDecision{Approved: true, EditedParams: req.Params}
	case "reject_once":
		return agent.ApprovalDecision{Approved: false, Reason: "user rejected the proposed action"}
	default:
		return agent.ApprovalDecision{Approved: false, Reason: "unsupported ACP permission option"}
	}
}

func (s *Server) callClient(ctx context.Context, id, method string, params any) (json.RawMessage, error) {
	idRaw, _ := json.Marshal(id)
	key := string(idRaw)
	ch := make(chan rpcResponse, 1)
	s.stateMu.Lock()
	s.pending[key] = ch
	s.stateMu.Unlock()
	defer func() {
		s.stateMu.Lock()
		delete(s.pending, key)
		s.stateMu.Unlock()
	}()
	s.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	select {
	case response := <-ch:
		// If cancellation and a client response become ready together,
		// cancellation wins. A late allow must never authorize a tool after
		// session/cancel has already been observed.
		if err := ctx.Err(); err != nil {
			return nil, context.Canceled
		}
		if response.err != nil {
			return nil, fmt.Errorf("ACP client rejected permission request: %s", response.err.Message)
		}
		return response.result, nil
	case <-ctx.Done():
		return nil, context.Canceled
	}
}

func (s *Server) handleResponse(msg rpcMessage) {
	key := string(msg.ID)
	s.stateMu.Lock()
	ch := s.pending[key]
	s.stateMu.Unlock()
	if ch == nil {
		fmt.Fprintf(s.log, "packetcode acp: ignoring response for unknown request id %s\n", key)
		return
	}
	select {
	case ch <- rpcResponse{result: msg.Result, err: msg.Error}:
	default:
	}
}

func (s *Server) session(id string) *sessionState {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.sessions[id]
}

func (s *Server) sendUpdate(sessionID string, update any) {
	s.write(map[string]any{
		"jsonrpc": "2.0", "method": "session/update",
		"params": map[string]any{"sessionId": sessionID, "update": update},
	})
}

func (s *Server) sendResult(id json.RawMessage, result any) {
	if len(id) == 0 {
		return
	}
	s.write(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{"2.0", id, result})
}

func (s *Server) replyError(msg rpcMessage, code int, message string, data any) {
	if len(msg.ID) == 0 {
		fmt.Fprintf(s.log, "packetcode acp: %s: %s\n", msg.Method, message)
		return
	}
	s.sendError(msg.ID, code, message, data)
}

func (s *Server) sendError(id json.RawMessage, code int, message string, data any) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	errValue := &rpcError{Code: code, Message: message}
	if data != nil {
		errValue.Data, _ = json.Marshal(data)
	}
	s.write(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   *rpcError       `json:"error"`
	}{"2.0", id, errValue})
}

func (s *Server) write(value any) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := json.NewEncoder(s.out).Encode(value); err != nil {
		fmt.Fprintf(s.log, "packetcode acp: write protocol response: %v\n", err)
	}
}

func (s *Server) shutdown() {
	s.stateMu.Lock()
	sessions := make([]*sessionState, 0, len(s.sessions))
	for _, state := range s.sessions {
		sessions = append(sessions, state)
	}
	s.stateMu.Unlock()
	for _, state := range sessions {
		state.mu.Lock()
		if state.cancel != nil {
			state.cancelled = true
			state.cancel()
		}
		state.mu.Unlock()
	}
	s.wg.Wait()
	for _, state := range sessions {
		if state.runtime.Close != nil {
			if err := state.runtime.Close(); err != nil {
				fmt.Fprintf(s.log, "packetcode acp: close session %s: %v\n", state.runtime.ID, err)
			}
		}
	}
}

func decodeParams(raw json.RawMessage, dst any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return errors.New("params are required")
	}
	return json.Unmarshal(raw, dst)
}

func validID(id json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(id))
	if trimmed == "" || trimmed == "null" {
		return false
	}
	if strings.HasPrefix(trimmed, "\"") {
		var value string
		return json.Unmarshal(id, &value) == nil
	}
	var number json.Number
	return json.Unmarshal(id, &number) == nil && !strings.ContainsAny(number.String(), ".eE")
}

func idOrNull(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return nil
	}
	return id
}
