// Package agent is the orchestrator that drives a conversation forward:
// user message → LLM stream → tool calls → approval → tool execution →
// LLM stream → … until the LLM returns no more tool calls.
//
// The agent emits a typed channel of AgentEvent values that the TUI (or
// any other consumer) renders. It deliberately knows nothing about the
// terminal — Approver and the event channel are the only seams.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/hooks"
	"github.com/packetcode/packetcode/internal/permissions"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/sugar"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
)

// maxToolIterations caps the back-and-forth between LLM and tools per
// user message. Without a cap a misbehaving model could loop forever
// (e.g. retrying read_file on a path that keeps not existing). 25 is high
// enough for legitimate multi-step tasks and low enough to fail fast.
//
// It stays the backstop for churn that varies just enough to look like
// progress; the degenerate repeat it names is caught far sooner, and with a
// reason the user can act on, by loopDetector (loopdetect.go).
const maxToolIterations = 25

// EventType discriminates AgentEvent payloads.
type EventType int

const (
	EventTextDelta        EventType = iota
	EventReasoningDelta             // streaming chunk of the model's reasoning summary
	EventToolCallProposed           // LLM emitted a complete tool call (pre-approval)
	EventToolCallApproved           // user approved (or trust mode auto-approved)
	EventToolCallRejected           // user rejected
	EventToolOutputChunk            // incremental, live stdout/stderr from a running tool
	EventToolCallExecuted           // tool finished, result available
	EventUsageUpdate                // usage tokens recorded
	EventDone                       // turn complete (no more tool calls)
	EventError                      // unrecoverable error; channel about to close
)

// AgentEvent is the unified message the agent emits to the TUI.
//
// EventToolOutputChunk uses CallID + Chunk to carry one incremental piece of a
// running tool's combined stdout/stderr, tagged with the tool call it belongs
// to. Zero or more chunks are emitted on this same channel between the tool's
// EventToolCallProposed/Approved and its EventToolCallExecuted, all sharing the
// same CallID (== provider.ToolCall.ID). These chunks are for live display
// ONLY: they are not persisted and are not the model-facing result. The bounded
// final output still arrives exactly once as EventToolCallExecuted's
// ToolResult. The UI buffers chunks per CallID and, on EventToolCallExecuted,
// drops the live preview in favor of the authoritative result.
type AgentEvent struct {
	Type       EventType
	Text       string            // EventTextDelta
	ToolCall   provider.ToolCall // EventToolCall*
	ToolResult tools.ToolResult  // EventToolCallExecuted
	Usage      provider.Usage    // EventUsageUpdate
	Error      error             // EventError
	CallID     string            // EventToolOutputChunk: provider.ToolCall.ID of the running tool
	Chunk      string            // EventToolOutputChunk: raw output bytes (may span partial/multiple lines)
}

// Agent owns the long-lived dependencies required to run a turn.
// Run() is safe to call repeatedly but not concurrently — the conversation
// is intrinsically serial.
type Agent struct {
	registry      *provider.Registry
	toolRegistry  *tools.Registry
	session       *session.Manager
	costTracker   *cost.Tracker
	approverMu    sync.RWMutex
	approver      Approver
	policyMu      sync.RWMutex
	policy        *permissions.Policy
	systemPrompt  string
	hooks         *hooks.Runner
	tokenBudget   int
	sugarCache    SugarCacheConfig
	conduitShadow ConduitShadowConfig
	loopDetection LoopDetectionConfig
	toolOutput    ToolOutputStore
}

// ToolOutputStore bounds what a single tool result contributes to model
// context. *toolout.Store implements it; the agent keeps the dependency as an
// interface so the loop is testable without touching disk.
type ToolOutputStore interface {
	// Capture returns the model-facing text for a tool result and whether it
	// was replaced by a bounded excerpt. Implementations must return content
	// unchanged (and false) when it is already within the limit.
	Capture(toolName, content string) (string, bool)
}

// Config bundles the agent's required dependencies.
type Config struct {
	Registry      *provider.Registry
	Tools         *tools.Registry
	Session       *session.Manager
	CostTracker   *cost.Tracker
	Approver      Approver
	Policy        *permissions.Policy
	SystemPrompt  string
	Hooks         *hooks.Runner
	TokenBudget   int // input+output tokens; zero disables the boundary check
	SugarCache    SugarCacheConfig
	ConduitShadow ConduitShadowConfig
	LoopDetection LoopDetectionConfig // zero value enables detection with the defaults
	// ToolOutput bounds oversized tool results at the one chokepoint every
	// native, MCP, and skill result passes through. Nil leaves results
	// untouched, which is the pre-existing behavior.
	ToolOutput ToolOutputStore
}

// New constructs an Agent. Approver defaults to AutoReject if omitted —
// the safer default; production code must supply a real one.
func New(cfg Config) *Agent {
	if cfg.Approver == nil {
		cfg.Approver = AutoReject("no approver configured")
	}
	if cfg.Policy == nil {
		cfg.Policy = permissions.DefaultPolicy()
	}
	if cfg.SugarCache.Enabled && cfg.SugarCache.Mode == "" {
		cfg.SugarCache.Mode = provider.SugarCacheAuto
	}
	if cfg.SugarCache.Enabled && cfg.SugarCache.Retention == "" {
		cfg.SugarCache.Retention = provider.SugarCacheProviderDefault
	}
	if cfg.SugarCache.Enabled && cfg.SugarCache.Privacy == "" {
		cfg.SugarCache.Privacy = provider.SugarPrivacyStandard
	}
	return &Agent{
		registry:      cfg.Registry,
		toolRegistry:  cfg.Tools,
		session:       cfg.Session,
		costTracker:   cfg.CostTracker,
		approver:      cfg.Approver,
		policy:        cfg.Policy,
		systemPrompt:  cfg.SystemPrompt,
		hooks:         cfg.Hooks,
		tokenBudget:   cfg.TokenBudget,
		sugarCache:    cfg.SugarCache,
		conduitShadow: cfg.ConduitShadow,
		loopDetection: cfg.LoopDetection,
		toolOutput:    cfg.ToolOutput,
	}
}

// SetApprover swaps the approver at runtime — used by /trust to flip
// between user-prompted and auto-approve modes mid-conversation. The swap
// therefore races the running turn's read in handleToolCall, so it is
// guarded the same way SetPolicy is.
func (a *Agent) SetApprover(approver Approver) {
	if approver == nil {
		approver = AutoReject("no approver configured")
	}
	a.approverMu.Lock()
	a.approver = approver
	a.approverMu.Unlock()
}

func (a *Agent) currentApprover() Approver {
	a.approverMu.RLock()
	approver := a.approver
	a.approverMu.RUnlock()
	if approver == nil {
		return AutoReject("no approver configured")
	}
	return approver
}

func (a *Agent) SetPolicy(policy *permissions.Policy) {
	if policy == nil {
		policy = permissions.DefaultPolicy()
	}
	a.policyMu.Lock()
	a.policy = policy
	a.policyMu.Unlock()
}

func (a *Agent) currentPolicy() *permissions.Policy {
	a.policyMu.RLock()
	policy := a.policy
	a.policyMu.RUnlock()
	if policy == nil {
		return permissions.DefaultPolicy()
	}
	return policy
}

// Run processes a single user message through the full agentic loop.
// The returned channel is closed once the turn completes (or errors).
// Cancelling ctx interrupts streaming and any in-flight approval.
func (a *Agent) Run(ctx context.Context, userMessage string) <-chan AgentEvent {
	events := make(chan AgentEvent, 16)
	go a.run(ctx, userMessage, events)
	return events
}

func (a *Agent) run(ctx context.Context, userMessage string, events chan<- AgentEvent) {
	defer close(events)

	submittedMessage := userMessage
	if a.hooks != nil {
		cur := a.session.Current()
		sessionID := ""
		if cur != nil {
			sessionID = cur.ID
		}
		out, err := a.hooks.RunUserPromptSubmit(ctx, hooks.PromptPayload{
			SessionID: sessionID,
			Prompt:    userMessage,
		})
		if err != nil {
			events <- AgentEvent{Type: EventError, Error: fmt.Errorf("user prompt hook: %w", err)}
			return
		}
		if out != "" {
			submittedMessage += "\n\n[UserPromptSubmit hook output]\n" + out
		}
	}

	if err := a.session.AddMessage(provider.Message{
		Role:    provider.RoleUser,
		Content: submittedMessage,
	}); err != nil {
		events <- AgentEvent{Type: EventError, Error: fmt.Errorf("save user message: %w", err)}
		return
	}
	var shadow *conduitShadowState
	if a.conduitShadow.Enabled {
		shadow = newConduitShadowState(a.conduitShadow, a.session, userMessage)
	}

	// The window belongs to this run: it must see one user message's turns and
	// nothing from the message before it.
	loop := newLoopDetector(a.loopDetection)

	for iter := 0; iter < maxToolIterations; iter++ {
		more, err := a.oneTurn(ctx, events, shadow, loop)
		if err != nil {
			events <- AgentEvent{Type: EventError, Error: err}
			return
		}
		if !more {
			events <- AgentEvent{Type: EventDone}
			return
		}
		if a.tokenBudget > 0 {
			cur := a.session.Current()
			if cur != nil {
				used := cur.TokenUsage.TotalInput + cur.TokenUsage.TotalOutput
				if used >= a.tokenBudget {
					events <- AgentEvent{Type: EventError, Error: fmt.Errorf("token budget exhausted at turn boundary: used %d tokens (budget %d)", used, a.tokenBudget)}
					return
				}
			}
		}
	}
	events <- AgentEvent{Type: EventError, Error: fmt.Errorf("exceeded %d tool iterations", maxToolIterations)}
}

// oneTurn streams one assistant response and processes any tool calls.
// Returns (true, nil) if more turns are needed (i.e. tool calls were
// executed and the LLM should respond to their results), (false, nil) if
// the LLM emitted no tool calls (turn complete), or (_, err) on failure.
func (a *Agent) oneTurn(ctx context.Context, events chan<- AgentEvent, shadow *conduitShadowState, loop *loopDetector) (bool, error) {
	prov, modelID := a.registry.Active()
	if prov == nil {
		return false, errors.New("no active provider")
	}

	req := provider.ChatRequest{
		Model:    modelID,
		Messages: a.buildMessages(),
		Stream:   true,
	}
	if prov.SupportsTools(modelID) {
		req.Tools = provider.CanonicalToolDefinitions(a.toolRegistry.Definitions())
	} else if a.toolRegistry != nil && len(a.toolRegistry.Definitions()) > 0 {
		req.Messages = append([]provider.Message{{
			Role:    provider.RoleSystem,
			Content: unsupportedToolsMessage(prov.Name(), modelID),
		}}, req.Messages...)
	}
	if cur := a.session.Current(); cur != nil && a.sugarCache.Enabled && prov.Slug() == sugar.Slug {
		stablePrefixMessages := 0
		if a.systemPrompt != "" {
			stablePrefixMessages = 1
		}
		req.SugarCache = &provider.SugarCacheMetadata{
			ConversationID:       cur.ID,
			PrefixFingerprint:    provider.CachePrefixFingerprint(a.systemPrompt, req.Tools),
			StablePrefixMessages: stablePrefixMessages,
			CompactionGeneration: cur.Cache.CompactionGeneration,
			Mode:                 a.sugarCache.Mode,
			Retention:            a.sugarCache.Retention,
			Privacy:              a.sugarCache.Privacy,
		}
	}
	shadow.start(ctx, prov, req)

	stream, err := prov.ChatCompletion(ctx, req)
	if err != nil {
		shadow.providerFailure(ctx, err)
		return false, fmt.Errorf("chat completion: %w", err)
	}

	asm := newCallAssembler()
	var fullText string
	var fullReasoning string
	var lastUsage *provider.Usage

	for ev := range stream {
		switch ev.Type {
		case provider.EventTextDelta:
			fullText += ev.TextDelta
			events <- AgentEvent{Type: EventTextDelta, Text: ev.TextDelta}

		case provider.EventReasoningDelta:
			// Reasoning is never part of fullText — it must not render as
			// ordinary assistant output. It is recorded separately so
			// interleaved-thinking models can be handed their own chain back on
			// the next request; providers that expose only reasoning summaries
			// store it for display and never echo it.
			fullReasoning += ev.TextDelta
			events <- AgentEvent{Type: EventReasoningDelta, Text: ev.TextDelta}

		case provider.EventToolCallStart:
			asm.start(ev.ToolCall)

		case provider.EventToolCallDelta:
			asm.append(ev.ToolCall)

		case provider.EventToolCallEnd:
			asm.end(ev.ToolCall.Index)

		case provider.EventDone:
			if ev.Usage != nil {
				lastUsage = ev.Usage
			}

		case provider.EventError:
			shadow.providerFailure(ctx, ev.Error)
			return false, ev.Error
		}
	}

	calls := asm.finalize()
	for _, call := range calls {
		if err := validateToolCall(call); err != nil {
			return false, err
		}
	}

	if fullText != "" || fullReasoning != "" || len(calls) > 0 {
		// Persist the assistant message (text + reasoning + completed tool
		// calls). Reasoning is stored even on tool-calling turns, where the
		// visible content is deliberately dropped: for interleaved-thinking
		// models the chain is exactly what the next request has to replay.
		content := fullText
		if len(calls) > 0 {
			content = ""
		}
		assistantMsg := provider.Message{
			Role:      provider.RoleAssistant,
			Content:   content,
			Reasoning: fullReasoning,
			ToolCalls: calls,
		}
		if err := a.session.AddMessage(assistantMsg); err != nil {
			return false, fmt.Errorf("save assistant message: %w", err)
		}
	}

	if lastUsage != nil {
		inRate, outRate := prov.Pricing(modelID)
		// Cache pricing is asked for here, where the provider is in hand. The
		// session manager prices cached input at a fraction of fresh input and
		// defaults are right for most providers; this is what lets Anthropic
		// state its cache-write premium instead of being averaged into them.
		readMul, writeMul := provider.CacheMultipliersFor(prov, modelID)
		a.session.SetCacheMultipliers(readMul, writeMul)
		_ = a.session.UpdateUsage(*lastUsage, inRate, outRate)
		if a.costTracker != nil {
			cur := a.session.Current()
			if cur != nil {
				// Cache counts come from the session's running totals, which
				// UpdateUsage has already accumulated from this turn's usage
				// report. Passing them explicitly is what closes the last cut
				// in the chain: the tally used to receive input/output only.
				_ = a.costTracker.RecordUsageWithCache(cur.ID, prov.Slug(), modelID,
					cur.TokenUsage.TotalInput, cur.TokenUsage.TotalOutput,
					cur.TokenUsage.TotalCacheCreation, cur.TokenUsage.TotalCacheRead)
			}
		}
		events <- AgentEvent{Type: EventUsageUpdate, Usage: *lastUsage}
	}

	if len(calls) == 0 {
		return false, nil
	}

	observed := make([]toolObservation, 0, len(calls))
	for _, call := range calls {
		obs, err := a.handleToolCall(ctx, call, events, shadow)
		if err != nil {
			return false, err
		}
		observed = append(observed, obs)
	}
	// Judged only once every call in the turn has settled, so parallel calls
	// are weighed as the single unit of work the model actually requested.
	if err := loop.observe(observed); err != nil {
		return false, err
	}
	return true, nil
}

func unsupportedToolsMessage(providerName, modelID string) string {
	return fmt.Sprintf("Native tool calling is unavailable for %s model %q. Do not claim to use tools or request file/command actions; explain the limitation and ask the user to switch to a tool-capable model if tool access is required.", providerName, modelID)
}

// handleToolCall runs the approval flow and either executes the tool or
// records a rejection message. Either way a tool-role message is appended
// to the session so the LLM has full visibility into what happened.
//
// The returned toolObservation is what the loop detector signs: the call as it
// actually ran and the content the model was actually shown. Rejections and
// unknown tools are observed too — a model that keeps re-proposing a call the
// policy keeps refusing is looping just as surely as one re-reading a missing
// file.
func (a *Agent) handleToolCall(ctx context.Context, call provider.ToolCall, events chan<- AgentEvent, shadow *conduitShadowState) (toolObservation, error) {
	events <- AgentEvent{Type: EventToolCallProposed, ToolCall: call}

	tool, ok := a.toolRegistry.Get(call.Name)
	if !ok {
		// Unknown tool — feed the error back to the LLM and continue.
		unknown := fmt.Sprintf("unknown tool: %s", call.Name)
		events <- AgentEvent{Type: EventToolCallExecuted, ToolCall: call, ToolResult: tools.ToolResult{
			Content: unknown,
			IsError: true,
		}}
		shadow.toolResult(ctx, call, tools.ToolResult{Content: "unknown tool", IsError: true})
		return toolObservation{name: call.Name, arguments: call.Arguments, content: unknown},
			a.session.AddMessage(provider.Message{
				Role:       provider.RoleTool,
				ToolCallID: call.ID,
				Name:       call.Name,
				Content:    unknown,
			})
	}

	// The agent consults the permission policy itself rather than delegating
	// it to the Approver. An Approver-side hook for this existed from the same
	// commit that added policies and was never called from here; the two would
	// have applied the same policy to the same request, so this is the one
	// that runs and the other has been removed. Approve is consulted only for
	// the Ask outcome below.
	params := json.RawMessage(call.Arguments)
	policyResult := a.currentPolicy().Decide(permissions.Request{
		ToolName:         call.Name,
		RequiresApproval: tool.RequiresApproval(),
		Params:           params,
	})
	if policyResult.Decision == permissions.DecisionDeny {
		rejection := "permission denied: " + policyResult.Reason
		events <- AgentEvent{Type: EventToolCallRejected, ToolCall: call, Text: rejection}
		shadow.blocked(ctx, call, rejection)
		return a.rejected(call, params, rejection)
	}
	if a.hooks != nil {
		preOut, preErr := a.hooks.RunPreToolUse(ctx, hooks.ToolPayload{
			SessionID:  a.currentSessionID(),
			ToolName:   call.Name,
			ToolCallID: call.ID,
			Arguments:  params,
		})
		if preErr != nil {
			rejection := "pre-tool hook blocked " + call.Name + ": " + preErr.Error()
			if preOut != "" {
				rejection += "\n" + preOut
			}
			events <- AgentEvent{Type: EventToolCallRejected, ToolCall: call, Text: rejection}
			shadow.blocked(ctx, call, rejection)
			return a.rejected(call, params, rejection)
		}
	}
	if policyResult.Decision == permissions.DecisionAsk {
		decision := a.currentApprover().Approve(ctx, ApprovalRequest{
			Tool:     tool,
			ToolCall: call,
			Params:   params,
		})
		if !decision.Approved {
			rejection := decision.Reason
			if rejection == "" {
				rejection = "user rejected the proposed action"
			}
			events <- AgentEvent{Type: EventToolCallRejected, ToolCall: call, Text: rejection}
			shadow.blocked(ctx, call, rejection)
			return a.rejected(call, params, rejection)
		}
		if err := ctx.Err(); err != nil {
			return toolObservation{}, err
		}
		events <- AgentEvent{Type: EventToolCallApproved, ToolCall: call}
		if len(decision.EditedParams) > 0 {
			params = decision.EditedParams
			editedPolicyResult := a.currentPolicy().Decide(permissions.Request{
				ToolName:         call.Name,
				RequiresApproval: tool.RequiresApproval(),
				Params:           params,
			})
			if editedPolicyResult.Decision == permissions.DecisionDeny {
				rejection := "permission denied after approval edit: " + editedPolicyResult.Reason
				events <- AgentEvent{Type: EventToolCallRejected, ToolCall: call, Text: rejection}
				shadow.blocked(ctx, call, rejection)
				return a.rejected(call, params, rejection)
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return toolObservation{}, err
	}
	executedCall := call
	executedCall.Arguments = string(params)
	res, err := a.executeTool(ctx, tool, call.ID, params, events)
	if err != nil {
		if isContextCancellation(err) {
			return toolObservation{}, err
		}
		// Distinguish "tool returned an error result" (res.IsError) from
		// "tool itself failed to run" (err != nil). The latter still
		// becomes a tool-role message so the LLM can adapt.
		res = tools.ToolResult{Content: fmt.Sprintf("tool execution failed: %s", err), IsError: true}
	}
	if a.hooks != nil {
		hookOut, hookErr := a.hooks.RunPostToolUse(ctx, hooks.ToolPayload{
			SessionID:  a.currentSessionID(),
			ToolName:   call.Name,
			ToolCallID: call.ID,
			Arguments:  params,
			Result: &hooks.ToolResult{
				Content:  res.Content,
				IsError:  res.IsError,
				Metadata: res.Metadata,
			},
		})
		if hookOut != "" || hookErr != nil {
			res.Content = appendHookOutput(res.Content, hookOut, hookErr)
		}
	}
	events <- AgentEvent{Type: EventToolCallExecuted, ToolCall: executedCall, ToolResult: res}
	shadow.toolResult(ctx, executedCall, res)
	// This is the one place every tool result — native, MCP, or skill —
	// becomes a message, so it is the only place a cap holds for all of them.
	// Per-tool caps drift and MCP has none; capping here also means the UI and
	// the session file keep the full Content while only the model-facing
	// projection is bounded.
	modelContent := ""
	if excerpt, capped := a.captureToolOutput(call.Name, res.Content); capped {
		modelContent = excerpt
	}
	return toolObservation{name: call.Name, arguments: executedCall.Arguments, content: res.Content},
		a.session.AddMessage(provider.Message{
			Role:         provider.RoleTool,
			ToolCallID:   call.ID,
			Name:         call.Name,
			Content:      res.Content,
			ModelContent: modelContent,
		})
}

// captureToolOutput spills oversized output and returns the bounded excerpt the
// model should see. With no store configured the result passes through
// untouched and the session layer's projection remains the only bound.
func (a *Agent) captureToolOutput(toolName, content string) (string, bool) {
	if a.toolOutput == nil {
		return content, false
	}
	return a.toolOutput.Capture(toolName, content)
}

// rejected records a refusal as the tool-role message the LLM sees and reports
// that same text as the turn's observation, so a re-proposed and re-refused
// call still registers as the non-progress it is.
func (a *Agent) rejected(call provider.ToolCall, params json.RawMessage, reason string) (toolObservation, error) {
	return toolObservation{name: call.Name, arguments: string(params), content: reason},
		a.session.AddMessage(provider.Message{
			Role:       provider.RoleTool,
			ToolCallID: call.ID,
			Name:       call.Name,
			Content:    reason,
		})
}

// executeTool runs a tool, preferring its streaming path so live output can be
// surfaced to the UI as EventToolOutputChunk events. Tools that do not
// implement tools.StreamingTool fall through to the plain Execute path with
// identical behavior. The returned ToolResult is the bounded, model-facing
// result in both cases.
func (a *Agent) executeTool(ctx context.Context, tool tools.Tool, callID string, params json.RawMessage, events chan<- AgentEvent) (tools.ToolResult, error) {
	if st, ok := tool.(tools.StreamingTool); ok {
		sink := &chunkSink{events: events, callID: callID}
		return st.ExecuteStreaming(ctx, params, sink)
	}
	return tool.Execute(ctx, params)
}

// chunkSink adapts a running tool's incremental output to the agent's event
// channel. It implements tools.OutputSink. Each chunk is published as an
// EventToolOutputChunk tagged with the originating tool call ID.
//
// The send is best-effort and non-blocking: if the event channel is momentarily
// full the chunk is dropped rather than stalling the tool's output-draining
// goroutine (which would risk back-pressuring, and ultimately blocking, the
// child process). Dropping a live-display chunk is safe — the complete, bounded
// output is still delivered once via EventToolCallExecuted.
type chunkSink struct {
	events chan<- AgentEvent
	callID string
}

func (s *chunkSink) WriteChunk(chunk string) {
	if chunk == "" {
		return
	}
	select {
	case s.events <- AgentEvent{
		Type:   EventToolOutputChunk,
		CallID: s.callID,
		Chunk:  chunk,
	}:
	default:
		// Channel full; drop this live chunk (final result still delivered).
	}
}

func isContextCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (a *Agent) currentSessionID() string {
	if a == nil || a.session == nil || a.session.Current() == nil {
		return ""
	}
	return a.session.Current().ID
}

func appendHookOutput(content, out string, err error) string {
	var b strings.Builder
	b.WriteString(content)
	if out != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("[PostToolUse hook output]\n")
		b.WriteString(out)
	}
	if err != nil {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("[PostToolUse hook error]\n")
		b.WriteString(err.Error())
	}
	return b.String()
}

func validateToolCall(call provider.ToolCall) error {
	if call.Name == "" {
		return fmt.Errorf("tool call missing name")
	}
	if !json.Valid([]byte(call.Arguments)) {
		return fmt.Errorf("tool call %q arguments are invalid JSON", call.Name)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(call.Arguments), &obj); err != nil {
		return fmt.Errorf("tool call %q arguments must be a JSON object", call.Name)
	}
	if obj == nil {
		return fmt.Errorf("tool call %q arguments must be a JSON object", call.Name)
	}
	return nil
}

// buildMessages assembles the message array sent to the provider. Persisted
// history remains complete; oversized tool results use the immutable bounded
// projection recorded when the result first entered the session.
func (a *Agent) buildMessages() []provider.Message {
	cur := a.session.Current()
	var msgs []provider.Message
	if a.systemPrompt != "" {
		msgs = append(msgs, provider.Message{
			Role:    provider.RoleSystem,
			Content: a.systemPrompt,
		})
	}
	if cur != nil {
		transcript := normalizeToolTranscript(session.ModelMessages(cur.Messages))
		msgs = append(msgs, transcript...)
	}
	return msgs
}

// ────────────────────────────────────────────────────────────────────────────
// Tool call assembler
// ────────────────────────────────────────────────────────────────────────────

// callAssembler reassembles streaming tool-call deltas into complete
// provider.ToolCall values, indexed by the provider's `Index` field.
//
// Some providers stream tool calls token by token (OpenAI); some emit
// them whole (Gemini, Ollama). The assembler handles both — the only
// invariant is that each call gets a Start, zero-or-more Deltas, and an
// End at some point in the stream.
type callAssembler struct {
	calls map[int]*provider.ToolCall
}

func newCallAssembler() *callAssembler {
	return &callAssembler{calls: map[int]*provider.ToolCall{}}
}

func (a *callAssembler) start(d *provider.ToolCallDelta) {
	if d == nil {
		return
	}
	if _, ok := a.calls[d.Index]; ok {
		return
	}
	a.calls[d.Index] = &provider.ToolCall{
		ID:   d.ID,
		Name: d.Name,
	}
}

func (a *callAssembler) append(d *provider.ToolCallDelta) {
	if d == nil {
		return
	}
	c, ok := a.calls[d.Index]
	if !ok {
		// Some providers skip the explicit Start event and emit the first
		// chunk as a Delta. Treat it as an implicit Start.
		c = &provider.ToolCall{ID: d.ID, Name: d.Name}
		a.calls[d.Index] = c
	}
	if d.ID != "" && c.ID == "" {
		c.ID = d.ID
	}
	if d.Name != "" && c.Name == "" {
		c.Name = d.Name
	}
	c.Arguments += d.ArgumentsDelta
}

func (a *callAssembler) end(_ int) {
	// Nothing to do — finalisation happens in finalize().
}

// finalize returns the assembled calls in Index order.
func (a *callAssembler) finalize() []provider.ToolCall {
	if len(a.calls) == 0 {
		return nil
	}
	indices := make([]int, 0, len(a.calls))
	for i := range a.calls {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	out := make([]provider.ToolCall, len(indices))
	for i, idx := range indices {
		c := a.calls[idx]
		if c.Arguments == "" {
			c.Arguments = "{}"
		}
		out[i] = *c
	}
	return out
}
