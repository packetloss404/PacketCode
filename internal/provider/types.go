// Package provider defines the unified abstraction layer that the built-in
// LLM backends (OpenAI, Anthropic, Gemini, MiniMax, OpenRouter, Ollama)
// implement.
//
// The interface is intentionally narrow: identity, key validation, model
// listing, streaming chat completion, and pricing/context metadata. Anything
// provider-specific is hidden inside each implementation.
package provider

import (
	"encoding/json"
)

// Role is the message author identity. The provider implementations are
// responsible for translating these to/from their wire format (e.g. Gemini
// uses "model" instead of "assistant").
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Model describes a model exposed by a provider, with metadata used by the
// model selector UI and the cost tracker.
type Model struct {
	ID            string
	DisplayName   string
	ContextWindow int
	SupportsTools bool
	InputPer1M    float64 // USD per 1M input tokens; 0 for free/local
	OutputPer1M   float64
}

// ReasoningEffort describes one provider-advertised reasoning level.
type ReasoningEffort struct {
	ID          string
	Description string
}

// Message is the unified chat message format. Tool calls and tool responses
// share this struct: assistant messages may carry ToolCalls; tool messages
// carry ToolCallID + Name + textual Content.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content,omitempty"`
	// ModelContent is an immutable, bounded projection used only when this
	// message is sent back to a model. Content remains the authoritative full
	// result for local persistence and UI rendering. Older session files omit
	// this additive field and are upgraded by the session package.
	ModelContent string     `json:"model_content,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID   string     `json:"tool_call_id,omitempty"`
	Name         string     `json:"name,omitempty"`
	// Reasoning holds an assistant turn's thinking chain, stripped out of
	// Content so it never renders as ordinary text. Interleaved-thinking
	// models require it to be fed back on the next request or multi-turn tool
	// use degrades; providers that only expose reasoning *summaries* (Codex)
	// record it for display and never echo it. Each provider decides whether
	// and how to send it back — see openaicompat.Client.SendReasoning.
	Reasoning string `json:"reasoning,omitempty"`
}

// ToolDefinition is sent to the LLM to declare an available tool. Parameters
// is a raw JSON Schema document — the tool registry produces these.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall is a complete (assembled) tool invocation returned by the LLM.
// Arguments is a JSON string per the OpenAI/Anthropic convention — providers
// that use a different shape (Gemini's structured args) marshal to this.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatRequest is what the agent loop sends to a provider. Stream is always
// true for the MVP; the field exists for future non-streaming use cases
// (validation pings, summarization).
type ChatRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolDefinition
	Stream   bool
	// SugarCache carries member/session cache-affinity and governor hints to
	// Sugar. Generic and direct provider adapters deliberately ignore it.
	SugarCache *SugarCacheMetadata `json:"-"`
}

// SugarCacheMode controls Sugar's provider-aware prompt-cache behavior.
type SugarCacheMode string

const (
	SugarCacheAuto SugarCacheMode = "auto"
	SugarCacheOff  SugarCacheMode = "off"
)

// SugarCacheRetention requests a provider-supported cache lifetime. Sugar is
// responsible for rejecting or adapting combinations a selected provider
// cannot honor.
type SugarCacheRetention string

const (
	SugarCacheProviderDefault SugarCacheRetention = "provider_default"
	SugarCache5Minutes        SugarCacheRetention = "5m"
	SugarCache1Hour           SugarCacheRetention = "1h"
	SugarCache30Minutes       SugarCacheRetention = "30m"
)

// SugarPrivacyMode is fail-closed routing policy, not a claim that every
// provider supports zero-data-retention requests.
type SugarPrivacyMode string

const (
	SugarPrivacyStandard    SugarPrivacyMode = "standard"
	SugarPrivacyZDRRequired SugarPrivacyMode = "zdr_required"
)

// SugarCacheMetadata is private Packetcode-to-Sugar request metadata. It must
// never be blindly serialized by OpenAI-compatible adapters; Sugar's adapter
// validates and maps it to the `sugar_cache` wire object explicitly.
type SugarCacheMetadata struct {
	ConversationID       string
	PrefixFingerprint    string
	StablePrefixMessages int
	CompactionGeneration int
	Mode                 SugarCacheMode
	Retention            SugarCacheRetention
	Privacy              SugarPrivacyMode
}

// EventType discriminates StreamEvent payloads. Providers emit a sequence
// like: TextDelta* (ToolCallStart ToolCallDelta* ToolCallEnd)* Done.
type EventType int

const (
	EventTextDelta EventType = iota
	EventToolCallStart
	EventToolCallDelta
	EventToolCallEnd
	EventDone
	EventError
	// EventReasoningDelta carries a chunk of the model's reasoning summary
	// (e.g. Codex/Responses reasoning). Text is in TextDelta. Providers that
	// don't surface reasoning never emit it.
	EventReasoningDelta
)

// String renders an EventType for logs and tests.
func (e EventType) String() string {
	switch e {
	case EventTextDelta:
		return "TextDelta"
	case EventToolCallStart:
		return "ToolCallStart"
	case EventToolCallDelta:
		return "ToolCallDelta"
	case EventToolCallEnd:
		return "ToolCallEnd"
	case EventDone:
		return "Done"
	case EventError:
		return "Error"
	case EventReasoningDelta:
		return "ReasoningDelta"
	default:
		return "Unknown"
	}
}

// ToolCallDelta represents an in-flight tool call being streamed token by
// token. Index lets us correlate deltas when the model emits parallel calls.
type ToolCallDelta struct {
	Index          int
	ID             string
	Name           string
	ArgumentsDelta string
}

// Usage is the per-completion token accounting. InputTokens is the total
// prompt occupancy, including cached input. Cache fields are optional subsets
// used for provider-specific billing and diagnostics; consumers must not add
// them to InputTokens again.
type Usage struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

// StreamEvent is the provider-agnostic envelope yielded by ChatCompletion.
// Exactly one of TextDelta / ToolCall / Usage / Error is meaningful, keyed
// off Type.
type StreamEvent struct {
	Type      EventType
	TextDelta string
	ToolCall  *ToolCallDelta
	Usage     *Usage
	Error     error
}
