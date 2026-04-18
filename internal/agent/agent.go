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

	"github.com/packetcode/packetcode/internal/cost"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
	"github.com/packetcode/packetcode/internal/tools"
)

// maxToolIterations caps the back-and-forth between LLM and tools per
// user message. Without a cap a misbehaving model could loop forever
// (e.g. retrying read_file on a path that keeps not existing). 25 is high
// enough for legitimate multi-step tasks and low enough to fail fast.
const maxToolIterations = 25

// EventType discriminates AgentEvent payloads.
type EventType int

const (
	EventTextDelta EventType = iota
	EventToolCallProposed   // LLM emitted a complete tool call (pre-approval)
	EventToolCallApproved   // user approved (or trust mode auto-approved)
	EventToolCallRejected   // user rejected
	EventToolCallExecuted   // tool finished, result available
	EventUsageUpdate        // usage tokens recorded
	EventDone               // turn complete (no more tool calls)
	EventError              // unrecoverable error; channel about to close
)

// AgentEvent is the unified message the agent emits to the TUI.
type AgentEvent struct {
	Type       EventType
	Text       string             // EventTextDelta
	ToolCall   provider.ToolCall  // EventToolCall*
	ToolResult tools.ToolResult   // EventToolCallExecuted
	Usage      provider.Usage     // EventUsageUpdate
	Error      error              // EventError
}

// Agent owns the long-lived dependencies required to run a turn.
// Run() is safe to call repeatedly but not concurrently — the conversation
// is intrinsically serial.
type Agent struct {
	registry     *provider.Registry
	toolRegistry *tools.Registry
	session      *session.Manager
	costTracker  *cost.Tracker
	approver     Approver
	systemPrompt string
}

// Config bundles the agent's required dependencies.
type Config struct {
	Registry     *provider.Registry
	Tools        *tools.Registry
	Session      *session.Manager
	CostTracker  *cost.Tracker
	Approver     Approver
	SystemPrompt string
}

// New constructs an Agent. Approver defaults to AutoReject if omitted —
// the safer default; production code must supply a real one.
func New(cfg Config) *Agent {
	if cfg.Approver == nil {
		cfg.Approver = AutoReject("no approver configured")
	}
	return &Agent{
		registry:     cfg.Registry,
		toolRegistry: cfg.Tools,
		session:      cfg.Session,
		costTracker:  cfg.CostTracker,
		approver:     cfg.Approver,
		systemPrompt: cfg.SystemPrompt,
	}
}

// SetApprover swaps the approver at runtime — used by /trust to flip
// between user-prompted and auto-approve modes mid-conversation.
func (a *Agent) SetApprover(approver Approver) {
	a.approver = approver
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

	if err := a.session.AddMessage(provider.Message{
		Role:    provider.RoleUser,
		Content: userMessage,
	}); err != nil {
		events <- AgentEvent{Type: EventError, Error: fmt.Errorf("save user message: %w", err)}
		return
	}

	for iter := 0; iter < maxToolIterations; iter++ {
		more, err := a.oneTurn(ctx, events)
		if err != nil {
			events <- AgentEvent{Type: EventError, Error: err}
			return
		}
		if !more {
			events <- AgentEvent{Type: EventDone}
			return
		}
	}
	events <- AgentEvent{Type: EventError, Error: fmt.Errorf("exceeded %d tool iterations", maxToolIterations)}
}

// oneTurn streams one assistant response and processes any tool calls.
// Returns (true, nil) if more turns are needed (i.e. tool calls were
// executed and the LLM should respond to their results), (false, nil) if
// the LLM emitted no tool calls (turn complete), or (_, err) on failure.
func (a *Agent) oneTurn(ctx context.Context, events chan<- AgentEvent) (bool, error) {
	prov, modelID := a.registry.Active()
	if prov == nil {
		return false, errors.New("no active provider")
	}

	req := provider.ChatRequest{
		Model:    modelID,
		Messages: a.buildMessages(),
		Tools:    a.toolRegistry.Definitions(),
		Stream:   true,
	}

	stream, err := prov.ChatCompletion(ctx, req)
	if err != nil {
		return false, fmt.Errorf("chat completion: %w", err)
	}

	asm := newCallAssembler()
	var fullText string
	var lastUsage *provider.Usage

	for ev := range stream {
		switch ev.Type {
		case provider.EventTextDelta:
			fullText += ev.TextDelta
			events <- AgentEvent{Type: EventTextDelta, Text: ev.TextDelta}

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
			return false, ev.Error
		}
	}

	calls := asm.finalize()

	// Persist the assistant message (text + completed tool calls).
	assistantMsg := provider.Message{
		Role:      provider.RoleAssistant,
		Content:   fullText,
		ToolCalls: calls,
	}
	if err := a.session.AddMessage(assistantMsg); err != nil {
		return false, fmt.Errorf("save assistant message: %w", err)
	}

	if lastUsage != nil {
		inRate, outRate := prov.Pricing(modelID)
		_ = a.session.UpdateUsage(*lastUsage, inRate, outRate)
		if a.costTracker != nil {
			cur := a.session.Current()
			if cur != nil {
				_ = a.costTracker.RecordUsage(cur.ID, prov.Slug(), modelID,
					cur.TokenUsage.TotalInput, cur.TokenUsage.TotalOutput)
			}
		}
		events <- AgentEvent{Type: EventUsageUpdate, Usage: *lastUsage}
	}

	if len(calls) == 0 {
		return false, nil
	}

	for _, call := range calls {
		if err := a.handleToolCall(ctx, call, events); err != nil {
			return false, err
		}
	}
	return true, nil
}

// handleToolCall runs the approval flow and either executes the tool or
// records a rejection message. Either way a tool-role message is appended
// to the session so the LLM has full visibility into what happened.
func (a *Agent) handleToolCall(ctx context.Context, call provider.ToolCall, events chan<- AgentEvent) error {
	tool, ok := a.toolRegistry.Get(call.Name)
	if !ok {
		// Unknown tool — feed the error back to the LLM and continue.
		events <- AgentEvent{Type: EventToolCallExecuted, ToolCall: call, ToolResult: tools.ToolResult{
			Content: fmt.Sprintf("unknown tool: %s", call.Name),
			IsError: true,
		}}
		return a.session.AddMessage(provider.Message{
			Role:       provider.RoleTool,
			ToolCallID: call.ID,
			Name:       call.Name,
			Content:    fmt.Sprintf("unknown tool: %s", call.Name),
		})
	}

	events <- AgentEvent{Type: EventToolCallProposed, ToolCall: call}

	params := json.RawMessage(call.Arguments)
	if tool.RequiresApproval() {
		decision := a.approver.Approve(ctx, ApprovalRequest{
			Tool:     tool,
			ToolCall: call,
			Params:   params,
		})
		if !decision.Approved {
			events <- AgentEvent{Type: EventToolCallRejected, ToolCall: call}
			rejection := decision.Reason
			if rejection == "" {
				rejection = "user rejected the proposed action"
			}
			return a.session.AddMessage(provider.Message{
				Role:       provider.RoleTool,
				ToolCallID: call.ID,
				Name:       call.Name,
				Content:    rejection,
			})
		}
		events <- AgentEvent{Type: EventToolCallApproved, ToolCall: call}
		if len(decision.EditedParams) > 0 {
			params = decision.EditedParams
		}
	}

	res, err := tool.Execute(ctx, params)
	if err != nil {
		// Distinguish "tool returned an error result" (res.IsError) from
		// "tool itself failed to run" (err != nil). The latter still
		// becomes a tool-role message so the LLM can adapt.
		res = tools.ToolResult{Content: fmt.Sprintf("tool execution failed: %s", err), IsError: true}
	}
	events <- AgentEvent{Type: EventToolCallExecuted, ToolCall: call, ToolResult: res}
	return a.session.AddMessage(provider.Message{
		Role:       provider.RoleTool,
		ToolCallID: call.ID,
		Name:       call.Name,
		Content:    res.Content,
	})
}

// buildMessages assembles the message array sent to the provider:
// optional system prompt + the session's accumulated messages.
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
		msgs = append(msgs, cur.Messages...)
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
