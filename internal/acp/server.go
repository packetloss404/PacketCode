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
	CWD string
	// MCPServers is the client-supplied stdio MCP server list. It is only
	// meaningful together with MCPServersSet.
	MCPServers []MCPServer
	// MCPServersSet reports whether the request carried an "mcpServers" field
	// at all, which the factory needs to tell two very different intents apart:
	//
	//   absent      (false)          -> the client has no opinion; the factory
	//                                  should run the agent's own configured
	//                                  MCP servers, matching the TUI.
	//   []          (true, len 0)    -> the client explicitly wants no MCP
	//                                  servers at all. ACP's contract: the
	//                                  client owns the session's MCP fleet.
	//   [a, b, ...] (true, len > 0)  -> exactly these, nothing else.
	//
	// The wire decode uses *[]wireMCPServer precisely so absent and [] stay
	// distinguishable; collapsing them would make it impossible for a desktop
	// client to opt into the agent's configuration without hard-coding it.
	MCPServersSet bool
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

// ErrPermissionModeDenied marks a session/new permissionMode override that is
// more permissive than the server allows. Clients must not be able to
// escalate past the profile the operator started the server with.
var ErrPermissionModeDenied = errors.New("permission mode not allowed")

// Runner is PacketCode's terminal-independent agent event source.
type Runner interface {
	Run(context.Context, string) <-chan agent.AgentEvent
}

// Runtime owns one independently persisted PacketCode session.
type Runtime struct {
	ID     string
	Runner Runner
	Close  func() error
	// MCPServers is the live per-session MCP fleet, as resolved and started by
	// the factory. Served by _packetcode/mcp/list when the client passes this
	// session's ID. Nil when the factory does not report it.
	MCPServers []MCPServerStatus
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

// SessionRenamer updates a persisted session's display name for the
// _packetcode/sessions/rename extension. Optional: when unset the method
// answers method-not-found, matching older servers. Implementations receive
// the raw client-supplied name and may normalize it before persisting.
type SessionRenamer interface {
	RenameSession(id, name string) error
}

// SessionUsage is the wire projection served by the
// _packetcode/sessions/usage extension method and attached to successful
// session/prompt results under "_packetcode".usage. Fields are additive;
// clients must tolerate new ones.
type SessionUsage struct {
	// ContextTokens is the live context-window occupancy after the most
	// recent turn (prompt + completion), not a cumulative total.
	ContextTokens int `json:"contextTokens"`
	// TotalInput and TotalOutput are cumulative across the whole session.
	TotalInput  int     `json:"totalInput"`
	TotalOutput int     `json:"totalOutput"`
	CostUSD     float64 `json:"costUsd"`
}

// UsageReader supplies per-session token/cost usage for the
// _packetcode/sessions/usage extension and for prompt-result enrichment.
// Optional: when unset the method answers method-not-found and prompt
// results stay bare, matching older servers.
type UsageReader interface {
	ReadUsage(sessionID string) (SessionUsage, error)
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

// MCPServerStatus is one MCP server as reported by the _packetcode/mcp/list
// extension. Fields are additive; clients must tolerate new ones.
type MCPServerStatus struct {
	Name string `json:"name"`
	// Status is "running", "failed", "disabled", or "configured" (known from
	// configuration but not started in the queried scope).
	Status    string `json:"status"`
	ToolCount int    `json:"toolCount"`
	// Source is "agent" for servers taken from the agent's own configuration
	// and "client" for servers the ACP client supplied in session/new.
	Source  string `json:"source,omitempty"`
	Command string `json:"command,omitempty"`
	Error   string `json:"error,omitempty"`
}

// MCPLister supplies the agent's configured MCP servers for the
// _packetcode/mcp/list extension. Optional: when unset the method answers
// method-not-found, matching older servers. Per-session live status comes
// from Runtime.MCPServers instead; this reports the static configuration a
// session created without a client-supplied list would inherit.
type MCPLister interface {
	ListMCPServers() ([]MCPServerStatus, error)
}

// CommandInfo is the wire projection served by the _packetcode/commands/list
// extension method. Fields are additive; clients must tolerate new ones.
type CommandInfo struct {
	// Name is the bare command word without the leading slash.
	Name string `json:"name"`
	// Description is one line of help text for a completion menu.
	Description string `json:"description"`
	// Source is "builtin", "user", or "project" — where the command came
	// from, so clients can group or badge the menu.
	Source string `json:"source"`
	// ArgumentHint is a short usage tail such as "[arguments]", shown after
	// the name in a menu. Empty for commands that take no arguments.
	ArgumentHint string `json:"argumentHint,omitempty"`
	// Body is the prompt text the command expands to. Server-side only —
	// json:"-" keeps it off the wire. The server uses it to expand a leading
	// "/name" in session/prompt text so a client that inserts the completion
	// gets the command's real prompt instead of the literal token.
	Body string `json:"-"`
}

// CommandCatalog supplies invocable slash commands for the
// _packetcode/commands/list extension. Optional: when unset the method
// answers method-not-found, matching older servers. cwd scopes project-local
// command discovery; implementations may return only user-scoped commands
// when it is empty.
type CommandCatalog interface {
	ListCommands(cwd string) ([]CommandInfo, error)
}

// ProjectFileIndex answers @-mention file searches for the
// _packetcode/project/files extension. Optional: when unset the method
// answers method-not-found, matching older servers. Results are
// project-relative, slash-separated paths ordered best-match first, and
// implementations must honour limit.
type ProjectFileIndex interface {
	SearchFiles(cwd, query string, limit int) ([]string, error)
}

const (
	// defaultProjectFilesLimit caps a _packetcode/project/files response when
	// the client sends no limit; it matches a comfortable menu length.
	defaultProjectFilesLimit = 20
	// maxProjectFilesLimit is the hard ceiling, so a client cannot ask the
	// server to serialize a whole monorepo into one reply.
	maxProjectFilesLimit = 200
)

// Server is a single ACP stdio connection. It is safe for prompt workers and
// permission responses to use concurrently.
type Server struct {
	in      io.Reader
	out     io.Writer
	log     io.Writer
	factory SessionFactory
	version string
	lister  SessionLister
	renamer SessionRenamer
	catalog ModelCatalog
	usage   UsageReader
	mcp     MCPLister
	// commands gates _packetcode/commands/list and slash expansion in
	// session/prompt; files gates _packetcode/project/files.
	commands CommandCatalog
	files    ProjectFileIndex
	// permissionModes, when non-nil, replaces the advertised PermissionModes.
	permissionModes []string
	// defaultPermissionMode is the mode a session/new without an override
	// actually runs under. Advertised so clients can label their control
	// honestly instead of guessing "ask".
	defaultPermissionMode string

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

	// workers tracks this session's prompt goroutines. session/close has to
	// know when the turn it just cancelled has finished writing before it may
	// release the runtime; the server-wide wg cannot answer that per session.
	workers sync.WaitGroup

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

// SetSessionRenamer enables the _packetcode/sessions/rename extension. Must
// be called before Serve; a nil renamer leaves the method unregistered.
func (s *Server) SetSessionRenamer(r SessionRenamer) {
	s.renamer = r
}

// SetModelCatalog enables the _packetcode/models/list extension. Must be
// called before Serve; a nil catalog leaves the method unregistered.
func (s *Server) SetModelCatalog(c ModelCatalog) {
	s.catalog = c
}

// SetMCPLister enables the _packetcode/mcp/list extension. Must be called
// before Serve; a nil lister leaves the method unregistered.
func (s *Server) SetMCPLister(l MCPLister) {
	s.mcp = l
}

// SetUsageReader enables the _packetcode/sessions/usage extension and
// prompt-result usage enrichment. Must be called before Serve; a nil reader
// leaves the method unregistered.
func (s *Server) SetUsageReader(r UsageReader) {
	s.usage = r
}

// SetCommandCatalog enables the _packetcode/commands/list extension and, with
// it, server-side expansion of a leading "/name" in session/prompt text. Must
// be called before Serve; a nil catalog leaves the method unregistered and
// prompt text untouched.
func (s *Server) SetCommandCatalog(c CommandCatalog) {
	s.commands = c
}

// SetProjectFileIndex enables the _packetcode/project/files extension. Must be
// called before Serve; a nil index leaves the method unregistered.
func (s *Server) SetProjectFileIndex(f ProjectFileIndex) {
	s.files = f
}

// SetPermissionModes overrides the permission modes advertised in initialize.
// Operators cap this to their startup profile so clients cannot be offered an
// escalation the factory would reject. Must be called before Serve; nil keeps
// the full PermissionModes vocabulary.
func (s *Server) SetPermissionModes(modes []string) {
	s.permissionModes = modes
}

// SetDefaultPermissionMode advertises the mode sessions run under when
// session/new carries no override. Must be called before Serve; empty leaves
// the key absent, which clients read as "unknown".
func (s *Server) SetDefaultPermissionMode(mode string) {
	s.defaultPermissionMode = mode
}

// Serve processes ACP messages until stdin closes or ctx is cancelled.
//
// Reading runs in its own goroutine so that cancelling ctx really does stop
// the server: a blocking Scan on stdin cannot be interrupted, so a loop that
// scanned inline would sit there until the client closed the pipe and would
// never reach shutdown — leaking every session's runtime, including its MCP
// child processes. Dispatch still happens on this goroutine, so requests are
// handled strictly in order, exactly as before.
//
// Note what this still cannot cover: if the process is killed outright
// (SIGKILL, Windows TerminateProcess, or a parent that drops the child
// handle) no Go code runs at all and the MCP children are reparented rather
// than shut down. Clients that own the process lifetime must release sessions
// explicitly — that is what session/close is for.
func (s *Server) Serve(ctx context.Context) error {
	s.ctx = ctx
	lines := make(chan string, 16)
	scanErr := make(chan error, 1)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(s.in)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
		scanErr <- scanner.Err()
	}()

	for {
		select {
		case line, ok := <-lines:
			if !ok {
				s.shutdown()
				select {
				case err := <-scanErr:
					if err != nil {
						return fmt.Errorf("read ACP transport: %w", err)
					}
				default:
					// Reader exited on ctx.Done rather than end-of-input.
				}
				return nil
			}
			s.dispatch(line)
		case <-ctx.Done():
			s.shutdown()
			return nil
		}
	}
}

// dispatch validates one framed JSON-RPC line and routes it. Called only from
// Serve's loop, which is what keeps request handling serial.
func (s *Server) dispatch(line string) {
	if !json.Valid([]byte(line)) {
		s.sendError(nil, codeParseError, "Parse error", nil)
		return
	}
	var msg rpcMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		s.sendError(nil, codeInvalidRequest, "Invalid Request", nil)
		return
	}
	if msg.JSONRPC != "2.0" || (msg.Method == "" && len(msg.ID) == 0) {
		s.sendError(idOrNull(msg.ID), codeInvalidRequest, "Invalid Request", nil)
		return
	}
	if len(msg.ID) > 0 && !validID(msg.ID) {
		s.sendError(nil, codeInvalidRequest, "Invalid Request", nil)
		return
	}
	if msg.Method == "" {
		s.handleResponse(msg)
		return
	}
	s.handleRequest(msg)
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
	case "session/close":
		s.handleCloseSession(msg)
	case "_packetcode/sessions/list":
		s.handleSessionsList(msg)
	case "_packetcode/sessions/rename":
		s.handleSessionsRename(msg)
	case "_packetcode/sessions/usage":
		s.handleSessionsUsage(msg)
	case "_packetcode/models/list":
		s.handleModelsList(msg)
	case "_packetcode/mcp/list":
		s.handleMCPList(msg)
	case "_packetcode/commands/list":
		s.handleCommandsList(msg)
	case "_packetcode/project/files":
		s.handleProjectFiles(msg)
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
			"mcpCapabilities": map[string]bool{"http": false, "sse": false},
			// session/close is a SPEC method, not a vendor extension, so it is
			// advertised the spec's way: SessionCapabilities.close is an
			// object, where "{}" means supported and an absent/null field
			// means not. Unlike the _packetcode/* extensions there is no
			// injected dependency to gate on — closing a session is intrinsic
			// to the server — so it is unconditionally advertised. Engines
			// predating it simply answer -32601 from the default case, which
			// is exactly what a client checking this flag avoids provoking.
			"sessionCapabilities": map[string]any{"close": map[string]any{}},
			// Vendor extension surface; underscore-prefixed so spec-only
			// clients skip it. sessionsList gates _packetcode/sessions/list;
			// sessionsRename gates _packetcode/sessions/rename; modelsList
			// gates _packetcode/models/list; mcpList gates the CONFIGURED
			// half of _packetcode/mcp/list (the sessionId-less query). The
			// per-session half answers off the session's own Runtime and is
			// always available, including for client-supplied fleets on an
			// agent with no MCP configuration of its own.
			//
			// mcpDefaults is a wire-behaviour promise, not a feature toggle:
			// this agent accepts session/new and session/load WITHOUT an
			// "mcpServers" field and reads the omission as "use the agent's own
			// configured MCP servers". Older agents reject the omission with
			// invalid-params, so clients must send [] unless they see this flag.
			"_packetcode": map[string]any{
				"sessionsList":          s.lister != nil,
				"sessionsRename":        s.renamer != nil,
				"sessionsUsage":         s.usage != nil,
				"modelsList":            s.catalog != nil,
				"mcpList":               s.mcp != nil,
				"mcpDefaults":           true,
				"commandsList":          s.commands != nil,
				"projectFiles":          s.files != nil,
				"permissionModes":       s.advertisedPermissionModes(),
				"defaultPermissionMode": s.defaultPermissionMode,
			},
		},
		"agentInfo":   map[string]string{"name": "packetcode", "title": "PacketCode", "version": s.version},
		"authMethods": []any{},
	})
}

func (s *Server) advertisedPermissionModes() []string {
	if s.permissionModes != nil {
		return s.permissionModes
	}
	return PermissionModes
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

func (s *Server) handleSessionsRename(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring _packetcode/sessions/rename notification; a request ID is required")
		return
	}
	if s.renamer == nil {
		s.replyError(msg, codeMethodNotFound, "Method not found", nil)
		return
	}
	var params struct {
		SessionID string `json:"sessionId"`
		Name      string `json:"name"`
	}
	if err := decodeParams(msg.Params, &params); err != nil {
		s.replyError(msg, codeInvalidParams, "invalid _packetcode/sessions/rename parameters", nil)
		return
	}
	if strings.TrimSpace(params.SessionID) == "" {
		s.replyError(msg, codeInvalidParams, "sessionId is required", nil)
		return
	}
	if strings.TrimSpace(params.Name) == "" {
		s.replyError(msg, codeInvalidParams, "name is required", nil)
		return
	}
	if err := s.renamer.RenameSession(params.SessionID, params.Name); err != nil {
		s.replyError(msg, codeInternalError, fmt.Sprintf("rename session: %v", err), nil)
		return
	}
	s.sendResult(msg.ID, map[string]any{})
}

func (s *Server) handleSessionsUsage(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring _packetcode/sessions/usage notification; a request ID is required")
		return
	}
	if s.usage == nil {
		s.replyError(msg, codeMethodNotFound, "Method not found", nil)
		return
	}
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if err := decodeParams(msg.Params, &params); err != nil || strings.TrimSpace(params.SessionID) == "" {
		s.replyError(msg, codeInvalidParams, "sessionId is required", nil)
		return
	}
	usage, err := s.usage.ReadUsage(params.SessionID)
	if err != nil {
		s.replyError(msg, codeInternalError, fmt.Sprintf("read session usage: %v", err), nil)
		return
	}
	s.sendResult(msg.ID, usage)
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

// handleMCPList answers _packetcode/mcp/list. With a sessionId it reports that
// live session's MCP fleet (what actually started, with tool counts); without
// one it reports the agent's configured servers, which is what a session
// created without a client-supplied mcpServers list would inherit.
//
// The two halves have different prerequisites, so the availability check sits
// in the no-sessionId branch rather than up here: a per-session answer is read
// straight off the session's Runtime and never touches s.mcp. Rejecting the
// live query with method-not-found just because no MCPLister was configured
// would hide a fleet the session demonstrably has — client-supplied servers
// being the obvious case, since those exist with no agent configuration at all.
func (s *Server) handleMCPList(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring _packetcode/mcp/list notification; a request ID is required")
		return
	}
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if err := decodeParams(msg.Params, &params); err != nil {
		s.replyError(msg, codeInvalidParams, "invalid _packetcode/mcp/list parameters", nil)
		return
	}
	if id := strings.TrimSpace(params.SessionID); id != "" {
		s.stateMu.Lock()
		state := s.sessions[id]
		s.stateMu.Unlock()
		if state == nil {
			s.replyError(msg, codeInvalidParams, fmt.Sprintf("unknown session %q", id), nil)
			return
		}
		servers := state.runtime.MCPServers
		if servers == nil {
			servers = []MCPServerStatus{}
		}
		s.sendResult(msg.ID, map[string]any{"servers": servers})
		return
	}
	if s.mcp == nil {
		s.replyError(msg, codeMethodNotFound, "Method not found", nil)
		return
	}
	servers, err := s.mcp.ListMCPServers()
	if err != nil {
		s.replyError(msg, codeInternalError, fmt.Sprintf("list MCP servers: %v", err), nil)
		return
	}
	if servers == nil {
		servers = []MCPServerStatus{}
	}
	s.sendResult(msg.ID, map[string]any{"servers": servers})
}

func (s *Server) handleCommandsList(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring _packetcode/commands/list notification; a request ID is required")
		return
	}
	if s.commands == nil {
		s.replyError(msg, codeMethodNotFound, "Method not found", nil)
		return
	}
	var params struct {
		CWD string `json:"cwd"`
	}
	if err := decodeParams(msg.Params, &params); err != nil {
		s.replyError(msg, codeInvalidParams, "invalid _packetcode/commands/list parameters", nil)
		return
	}
	// An absent cwd is legal: it simply scopes the answer to user-level
	// commands, since project commands live under <cwd>/.packetcode/commands.
	commands, err := s.commands.ListCommands(strings.TrimSpace(params.CWD))
	if err != nil {
		s.replyError(msg, codeInternalError, fmt.Sprintf("list commands: %v", err), nil)
		return
	}
	if commands == nil {
		commands = []CommandInfo{}
	}
	s.sendResult(msg.ID, map[string]any{"commands": commands})
}

func (s *Server) handleProjectFiles(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring _packetcode/project/files notification; a request ID is required")
		return
	}
	if s.files == nil {
		s.replyError(msg, codeMethodNotFound, "Method not found", nil)
		return
	}
	var params struct {
		CWD   string `json:"cwd"`
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := decodeParams(msg.Params, &params); err != nil {
		s.replyError(msg, codeInvalidParams, "invalid _packetcode/project/files parameters", nil)
		return
	}
	// Unlike commands, a file search has no meaning without a root to search.
	cwd := strings.TrimSpace(params.CWD)
	if cwd == "" {
		s.replyError(msg, codeInvalidParams, "cwd is required", nil)
		return
	}
	limit := params.Limit
	if limit <= 0 {
		limit = defaultProjectFilesLimit
	}
	if limit > maxProjectFilesLimit {
		limit = maxProjectFilesLimit
	}
	files, err := s.files.SearchFiles(cwd, strings.TrimSpace(params.Query), limit)
	if err != nil {
		s.replyError(msg, codeInternalError, fmt.Sprintf("search project files: %v", err), nil)
		return
	}
	if files == nil {
		files = []string{}
	}
	s.sendResult(msg.ID, map[string]any{"files": files})
}

// expandSlashCommand replaces a whole-prompt "/name [args]" invocation with the
// matching catalog command's body, substituting $ARGUMENTS. Without this a
// client that inserts a completion would send the literal "/name" token to the
// model, since the ACP surface has no other slash processing. Prompts that are
// not a single line, do not start with "/", or name no known command pass
// through untouched — so shell paths and prose beginning with a slash are safe.
func (s *Server) expandSlashCommand(prompt, cwd string) string {
	if s.commands == nil {
		return prompt
	}
	name, args, ok := splitSlashInvocation(prompt)
	if !ok {
		return prompt
	}
	commands, err := s.commands.ListCommands(cwd)
	if err != nil {
		fmt.Fprintf(s.log, "packetcode acp: slash expansion skipped: %v\n", err)
		return prompt
	}
	for _, cmd := range commands {
		if !strings.EqualFold(cmd.Name, name) || cmd.Body == "" {
			continue
		}
		if strings.Contains(cmd.Body, "$ARGUMENTS") {
			return strings.ReplaceAll(cmd.Body, "$ARGUMENTS", args)
		}
		// No placeholder: the arguments must still reach the model. Dropping
		// them silently deletes half of what the user asked for while the
		// client still shows them their whole sentence.
		if args == "" {
			return cmd.Body
		}
		return cmd.Body + "\n\n" + args
	}
	return prompt
}

// splitSlashInvocation parses "/name rest" out of a single-line prompt. It
// mirrors internal/app's slash grammar: the verb is [A-Za-z0-9_-]+ and the
// remainder is passed to $ARGUMENTS verbatim (trimmed).
func splitSlashInvocation(prompt string) (name, args string, ok bool) {
	trimmed := strings.TrimSpace(prompt)
	if !strings.HasPrefix(trimmed, "/") || strings.ContainsAny(trimmed, "\n\r") {
		return "", "", false
	}
	verb, rest, _ := strings.Cut(strings.TrimPrefix(trimmed, "/"), " ")
	if verb == "" {
		return "", "", false
	}
	for _, r := range verb {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return "", "", false
		}
	}
	return verb, strings.TrimSpace(rest), true
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
	if len(params.AdditionalDirectories) > 0 {
		s.replyError(msg, codeInvalidParams, "additionalDirectories are not supported", nil)
		return
	}
	// ACP marks mcpServers required, but this agent is deliberately lenient on
	// input: an omitted field means "no opinion", which the factory answers
	// with the agent's own configured servers. An explicit [] still means none.
	// Being lenient here costs nothing (every spec-conformant client sends the
	// field) and is the only way a client can defer to the agent's config.
	mcpServers, err := parseOptionalMCPServers(params.MCPServers)
	if err != nil {
		s.replyError(msg, codeInvalidParams, err.Error(), nil)
		return
	}
	if s.factory == nil {
		s.replyError(msg, codeInternalError, "session factory is not configured", nil)
		return
	}
	sessionConfig := SessionConfig{CWD: cwd, MCPServers: mcpServers, MCPServersSet: params.MCPServers != nil}
	if params.Packetcode != nil {
		sessionConfig.Provider = strings.TrimSpace(params.Packetcode.Provider)
		sessionConfig.Model = strings.TrimSpace(params.Packetcode.Model)
		sessionConfig.PermissionMode = strings.TrimSpace(params.Packetcode.PermissionMode)
	}
	approver := &permissionApprover{server: s}
	runtime, err := s.factory.NewSession(s.ctx, sessionConfig, approver)
	if err != nil {
		if errors.Is(err, ErrUnknownProvider) || errors.Is(err, ErrUnknownPermissionMode) ||
			errors.Is(err, ErrPermissionModeDenied) {
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
	// Same absent-vs-empty contract as session/new: omitting mcpServers defers
	// to the agent's configured servers, [] means run with none.
	mcpServers, err := parseOptionalMCPServers(params.MCPServers)
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
	runtime, err := s.factory.NewSession(s.ctx, SessionConfig{
		CWD: cwd, MCPServers: mcpServers, MCPServersSet: params.MCPServers != nil,
		SessionID: params.SessionID,
	}, approver)
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

// parseOptionalMCPServers validates a possibly-absent wire mcpServers field.
// An absent field yields a nil slice; the caller records the distinction in
// SessionConfig.MCPServersSet.
func parseOptionalMCPServers(in *[]wireMCPServer) ([]MCPServer, error) {
	if in == nil {
		return nil, nil
	}
	return parseMCPServers(*in)
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
	// A client that offers command completion sends the raw "/name" token;
	// turn it back into the command's prompt before the turn starts.
	prompt = s.expandSlashCommand(prompt, state.cwd)
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
	// Registered on the session too, so session/close can tell when this
	// worker has stopped writing and the runtime is safe to release.
	state.workers.Add(1)
	go func() {
		defer s.wg.Done()
		defer state.workers.Done()
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
	// Successful turns carry the session's usage in the vendor-extension
	// namespace so clients can refresh cost/context gauges without a second
	// round-trip. A failed read degrades to a bare spec result.
	result := map[string]any{"stopReason": "end_turn"}
	if s.usage != nil {
		if usage, err := s.usage.ReadUsage(sessionID); err == nil {
			result["_packetcode"] = map[string]any{"usage": usage}
		} else {
			fmt.Fprintf(s.log, "packetcode acp: read usage for session %s: %v\n", sessionID, err)
		}
	}
	s.sendResult(requestID, result)
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

// handleCloseSession answers the spec's session/close: it drops the session
// from this connection and frees everything behind it — the provider and tool
// registries, the backup manager, the in-memory transcript, and (via
// Runtime.Close) the session's MCP child processes. Without it the only way to
// release a session is to kill the whole agent, so a client that browses fifty
// history entries pins fifty runtimes for the process's lifetime.
//
// Three decisions worth stating, because the spec leaves them open:
//
//   - A session with a running prompt is CANCELLED, not rejected. The spec is
//     explicit ("treat it as if session/cancel was called"), and it is also
//     the honest behaviour: rejecting with -32000 would make close fail
//     exactly when a client is trying to reclaim a session that is stuck. The
//     turn still gets its normal cancelled ending — trailing tool-call
//     updates and a {"stopReason":"cancelled"} response — so nothing is
//     silently orphaned.
//   - Nothing is awaited on this goroutine. Serve dispatches serially, and the
//     worker being cancelled may be parked inside callClient waiting for a
//     session/request_permission reply that can only arrive through this very
//     loop; blocking here would deadlock exactly that case. Cancelling the
//     turn's context is what unblocks callClient (it selects on ctx.Done),
//     which is how outstanding permission requests are answered, and the
//     runtime is released on a tracked goroutine once the worker drains.
//   - An unknown session is idempotent SUCCESS, not -32602. Close is a
//     release, and a client racing two closes — or closing a session the agent
//     already dropped — has got what it asked for. The spec permits either
//     ("agents might reply with an error"); an error here would only push
//     clients into swallowing it. A missing or blank sessionId is still
//     -32602, since that is a malformed call rather than a lost race.
func (s *Server) handleCloseSession(msg rpcMessage) {
	if len(msg.ID) == 0 {
		fmt.Fprintln(s.log, "packetcode acp: ignoring session/close notification; a request ID is required")
		return
	}
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if err := decodeParams(msg.Params, &params); err != nil {
		s.replyError(msg, codeInvalidParams, "invalid session/close parameters", nil)
		return
	}
	if strings.TrimSpace(params.SessionID) == "" {
		s.replyError(msg, codeInvalidParams, "sessionId is required", nil)
		return
	}
	// Unregister first: from here on session/prompt answers "unknown
	// sessionId", session/load builds a fresh runtime rather than superseding
	// this one, and shutdown will not double-close what release is about to.
	s.stateMu.Lock()
	state := s.sessions[params.SessionID]
	delete(s.sessions, params.SessionID)
	s.stateMu.Unlock()
	if state != nil {
		s.releaseSession(state)
	}
	s.sendResult(msg.ID, map[string]any{})
}

// releaseSession cancels a session's in-flight turn exactly as session/cancel
// would and frees its runtime, waiting for the prompt worker to finish first
// so the close cannot race the updates it is still writing. Never blocks the
// caller: see handleCloseSession for why that matters.
func (s *Server) releaseSession(state *sessionState) {
	state.mu.Lock()
	active := state.active
	if active {
		state.cancelled = true
		if state.cancel != nil {
			state.cancel()
		}
	}
	state.mu.Unlock()
	if !active {
		s.closeRuntime(state)
		return
	}
	// Tracked on s.wg so shutdown still drains it: a close immediately
	// followed by the transport closing must not leave the runtime unreleased.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		state.workers.Wait()
		s.closeRuntime(state)
	}()
}

func (s *Server) closeRuntime(state *sessionState) {
	if state.runtime == nil || state.runtime.Close == nil {
		return
	}
	if err := state.runtime.Close(); err != nil {
		fmt.Fprintf(s.log, "packetcode acp: close session %s: %v\n", state.runtime.ID, err)
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
	// Drains prompt workers AND any release goroutine an earlier
	// session/close left in flight, so every runtime is closed exactly once.
	s.wg.Wait()
	for _, state := range sessions {
		s.closeRuntime(state)
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
