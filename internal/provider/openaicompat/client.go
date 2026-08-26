// Package openaicompat implements the request, response, and SSE streaming
// logic shared by every OpenAI-compatible chat-completions endpoint.
//
// Three of packetcode's built-in providers (OpenAI itself, MiniMax, OpenRouter)
// speak this protocol; each one wraps a Client with provider-specific base
// URL, headers, model list, and pricing. The wrapper implements the public
// provider.Provider interface; this package never imports lipgloss or
// otherwise concerns itself with branding or UI.
package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/packetcode/packetcode/internal/provider"
)

// HeaderFunc lets a wrapper inject extra headers (e.g. OpenRouter's
// HTTP-Referer and X-Title) on every outbound request.
type HeaderFunc func(req *http.Request)

// ExtraChatFieldsFunc lets a wrapper add explicitly provider-scoped top-level
// request fields. The generic client never reflects arbitrary ChatRequest
// metadata onto the wire, which prevents Sugar-only governor data leaking to
// other OpenAI-compatible vendors.
type ExtraChatFieldsFunc func(req provider.ChatRequest) (map[string]any, error)

// Client speaks the OpenAI chat-completions protocol against a configurable
// base URL. It is safe for concurrent use.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	// ExtraHeaders is invoked just before each request is sent. Wrappers
	// use it to add provider-specific headers without mutating Client state.
	ExtraHeaders HeaderFunc
	// ExtraChatFields is nil for ordinary OpenAI-compatible providers. Sugar
	// uses it to serialize its validated private cache envelope.
	ExtraChatFields ExtraChatFieldsFunc
	// InterleavedThinking enables <think>-block handling for backends whose
	// models stream their reasoning chain inline in `content` (MiniMax M2.x/M3).
	// When set, reasoning is split out of the visible transcript and reported as
	// EventReasoningDelta, and SendReasoning governs echoing it back.
	//
	// Off by default: OpenAI and OpenRouter do not use this convention, and a
	// literal "<think>" in ordinary prose must stay visible for them.
	InterleavedThinking bool
	// SendReasoning re-wraps Message.Reasoning in <think> tags on outbound
	// assistant messages, which is how the OpenAI-native format preserves an
	// interleaved-thinking chain across tool calls. Kept separate from
	// InterleavedThinking so a session that switches providers never ships
	// one backend's reasoning to another that would reject it.
	SendReasoning bool
}

// NewClient returns a Client with a sensible default HTTP client. A nil
// HTTPClient is replaced lazily on first use.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: 0, // streaming — no overall timeout, rely on context
		},
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}

// modelsResponse is the OpenAI /v1/models payload.
type modelsResponse struct {
	Data []struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	} `json:"data"`
}

// ListModels calls GET <base>/models. The response is intentionally
// model-agnostic: each wrapper layers its own filtering, pricing, and
// context-window metadata on top of the bare ID list.
func (c *Client) ListModels(ctx context.Context) ([]provider.Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	c.applyHeaders(req, false)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := provider.ReadErrorBody(resp.Body)
		return nil, fmt.Errorf("list models: status %d: %s", resp.StatusCode, extractAPIErrorMessage(body))
	}

	var parsed modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}
	out := make([]provider.Model, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		out = append(out, provider.Model{
			ID:          m.ID,
			DisplayName: m.ID,
		})
	}
	return out, nil
}

// ValidateKey performs a GET /models under a 10-second timeout to confirm the
// key authenticates. Any 2xx is success.
func (c *Client) ValidateKey(ctx context.Context, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("api key is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return err
	}
	c.applyHeadersWithKey(req, false, apiKey)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("validate key: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body := provider.ReadErrorBody(resp.Body)
		return fmt.Errorf("validate key: status %d: %s", resp.StatusCode, extractAPIErrorMessage(body))
	}
	return nil
}

// chatRequestBody is the wire format for POST /chat/completions.
type chatRequestBody struct {
	Model         string          `json:"model"`
	Messages      []wireMessage   `json:"messages"`
	Tools         []wireTool      `json:"tools,omitempty"`
	Stream        bool            `json:"stream"`
	StreamOptions *wireStreamOpts `json:"stream_options,omitempty"`
}

type wireStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

func (m wireMessage) MarshalJSON() ([]byte, error) {
	type wireMessageAlias wireMessage
	if m.Role == string(provider.RoleTool) {
		return json.Marshal(struct {
			Role       string         `json:"role"`
			Content    string         `json:"content"`
			ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
			ToolCallID string         `json:"tool_call_id,omitempty"`
			Name       string         `json:"name,omitempty"`
		}{
			Role:       m.Role,
			Content:    m.Content,
			ToolCalls:  m.ToolCalls,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		})
	}
	return json.Marshal(wireMessageAlias(m))
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}

type wireFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireTool struct {
	Type     string         `json:"type"`
	Function wireToolSchema `json:"function"`
}

type wireToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

func chatBody(req provider.ChatRequest) chatRequestBody {
	return chatRequestBody{
		Model:         req.Model,
		Messages:      toWireMessages(req.Messages, false),
		Tools:         toWireTools(req.Tools),
		Stream:        true,
		StreamOptions: &wireStreamOpts{IncludeUsage: true},
	}
}

// MarshalChatRequest serializes the standard OpenAI-compatible chat body and
// deliberately excludes provider-private metadata such as SugarCache. The
// disabled-by-default Sugar shadow client uses this to mirror the exact
// model/messages/tools request shape only when a caller explicitly opts in.
func MarshalChatRequest(req provider.ChatRequest) ([]byte, error) {
	return marshalChatRequest(chatBody(req), nil)
}

// toWireMessages converts unified messages to the wire format. When
// sendReasoning is set, an assistant turn's stored reasoning is re-wrapped in
// <think> tags and prepended to its content, reconstructing the exact shape the
// model emitted so the reasoning chain stays intact across tool calls.
func toWireMessages(msgs []provider.Message, sendReasoning bool) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		content := m.Content
		if sendReasoning && m.Role == provider.RoleAssistant && m.Reasoning != "" {
			content = openTag + m.Reasoning + closeTag + content
		}
		wm := wireMessage{
			Role:       string(m.Role),
			Content:    content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		if len(m.ToolCalls) > 0 {
			wm.ToolCalls = make([]wireToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				wm.ToolCalls[i] = wireToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: wireFunctionCall{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				}
			}
		}
		out = append(out, wm)
	}
	return out
}

func toWireTools(tools []provider.ToolDefinition) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	tools = provider.CanonicalToolDefinitions(tools)
	out := make([]wireTool, len(tools))
	for i, t := range tools {
		out[i] = wireTool{
			Type: "function",
			Function: wireToolSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}
	return out
}

// ChatCompletion opens a streaming chat completion. The returned channel is
// closed when the stream terminates. Errors before the first byte arrives
// are returned synchronously; errors mid-stream surface as EventError.
func (c *Client) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	body := chatBody(req)
	if c.SendReasoning {
		body.Messages = toWireMessages(req.Messages, true)
	}
	var extraFields map[string]any
	var err error
	if c.ExtraChatFields != nil {
		extraFields, err = c.ExtraChatFields(req)
		if err != nil {
			return nil, fmt.Errorf("chat request fields: %w", err)
		}
	}
	buf, err := marshalChatRequest(body, extraFields)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	// Build the stall guard BEFORE issuing the request and bind the streaming
	// request/body to the derived context (sctx) so a stall closes the
	// connection and unblocks the parse loop. sctx derives from ctx, so Ctrl+C
	// still propagates. This is the only per-call deadline: the http.Client
	// Timeout is deliberately left at 0 so healthy long streams are never cut.
	guard, sctx := provider.NewStallGuard(ctx, provider.ConfiguredStallTimeout())

	newReq := func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(sctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(buf))
		if err != nil {
			return nil, err
		}
		c.applyHeaders(httpReq, true)
		httpReq.Header.Set("Accept", "text/event-stream")
		return httpReq, nil
	}
	resp, err := provider.DoWithRetry(sctx, c.httpClient(), provider.ConfiguredRetry(), newReq)
	if err != nil {
		guard.Stop()
		return nil, fmt.Errorf("request: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		errBody := provider.ReadErrorBody(resp.Body)
		_ = resp.Body.Close()
		guard.Stop()
		return nil, fmt.Errorf("status %d: %s",
			resp.StatusCode, extractAPIErrorMessage(errBody))
	}

	ch := make(chan provider.StreamEvent, 8)
	var filter *thinkFilter
	if c.InterleavedThinking {
		filter = &thinkFilter{}
	}
	go parseSSE(ctx, sctx, guard, resp.Body, ch, filter)
	return ch, nil
}

func marshalChatRequest(body chatRequestBody, extra map[string]any) ([]byte, error) {
	if len(extra) == 0 {
		return json.Marshal(body)
	}
	base, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(base, &object); err != nil {
		return nil, err
	}
	for key, value := range extra {
		if _, exists := object[key]; exists {
			return nil, fmt.Errorf("extra chat field %q conflicts with the base request", key)
		}
		object[key] = value
	}
	return json.Marshal(object)
}

// extractAPIErrorMessage pulls the human-readable message out of a JSON
// error body if it has the canonical {error:{message:...}} shape (OpenAI,
// MiniMax, OpenRouter all do). Falls back to the raw trimmed body so
// non-JSON responses still render something useful.
func extractAPIErrorMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	var wrapper struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Error.Message != "" {
		return wrapper.Error.Message
	}
	return trimmed
}

func (c *Client) applyHeaders(req *http.Request, json bool) {
	c.applyHeadersWithKey(req, json, c.APIKey)
}

func (c *Client) applyHeadersWithKey(req *http.Request, json bool, apiKey string) {
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if json {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.ExtraHeaders != nil {
		c.ExtraHeaders(req)
	}
}

// chatStreamChunk is one SSE frame from /chat/completions.
type chatStreamChunk struct {
	Choices []struct {
		Index        int             `json:"index"`
		Delta        chatStreamDelta `json:"delta"`
		FinishReason *string         `json:"finish_reason"`
	} `json:"choices"`
	Usage *chatUsage `json:"usage,omitempty"`
}

type chatStreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	// ReasoningContent is the out-of-band reasoning field used by some
	// OpenAI-compatible backends instead of inline <think> blocks. It is
	// surfaced for display only: we did not receive it as part of `content`,
	// so we do not synthesise a <think> wrapper to echo back.
	ReasoningContent string               `json:"reasoning_content,omitempty"`
	ToolCalls        []chatStreamToolCall `json:"tool_calls,omitempty"`
}

type chatStreamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// parseSSE reads OpenAI-style SSE frames and translates them into
// provider.StreamEvent values on ch. The function closes ch and the body
// before returning.
//
// ctx is checked once per scanner iteration so Ctrl+C from the App layer
// unblocks the parser even when the server's TCP keep-alive hides the
// body close from bufio.Scanner. On cancel we emit EventError with the
// ctx.Err() cause (context.Canceled / DeadlineExceeded) so the agent
// path surfaces the friendlier "turn cancelled" rendering.
// filter is non-nil only for interleaved-thinking backends; it splits inline
// <think> blocks out of the content stream.
func parseSSE(ctx, sctx context.Context, guard *provider.StallGuard, body io.ReadCloser, ch chan<- provider.StreamEvent, filter *thinkFilter) {
	defer close(ch)
	defer body.Close()
	defer guard.Stop()

	// emitContent routes one content delta. Without a filter this preserves the
	// original behaviour exactly, including suppressing text that shares a frame
	// with tool calls. With a filter, text is never suppressed: a frame that
	// carries both a closing </think> and a tool call is normal for M3, and
	// dropping it would lose the tail of the reasoning chain.
	emitContent := func(text string, hasToolCalls bool) {
		if text == "" {
			return
		}
		if filter == nil {
			if !hasToolCalls {
				ch <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: text}
			}
			return
		}
		visible, reasoning := filter.Write(text)
		if reasoning != "" {
			ch <- provider.StreamEvent{Type: provider.EventReasoningDelta, TextDelta: reasoning}
		}
		if visible != "" {
			ch <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: visible}
		}
	}

	// flushFilter drains any text held back as a possible partial tag.
	flushFilter := func() {
		if filter == nil {
			return
		}
		visible, reasoning := filter.Flush()
		if reasoning != "" {
			ch <- provider.StreamEvent{Type: provider.EventReasoningDelta, TextDelta: reasoning}
		}
		if visible != "" {
			ch <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: visible}
		}
	}

	// activeCalls tracks which indices we've already emitted Start for.
	activeCalls := map[int]bool{}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		if err := provider.StreamHaltError(ctx, sctx); err != nil {
			ch <- provider.StreamEvent{Type: provider.EventError, Error: err}
			return
		}
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		guard.Tick()
		if data == "[DONE]" {
			flushFilter()
			// End any tool calls still open (defensive — most providers
			// emit finish_reason before [DONE]).
			for idx := range activeCalls {
				ch <- provider.StreamEvent{
					Type:     provider.EventToolCallEnd,
					ToolCall: &provider.ToolCallDelta{Index: idx},
				}
			}
			ch <- provider.StreamEvent{Type: provider.EventDone}
			return
		}

		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("parse SSE chunk: %w", err)}
			return
		}

		for _, choice := range chunk.Choices {
			hasToolCalls := len(choice.Delta.ToolCalls) > 0
			if choice.Delta.ReasoningContent != "" {
				ch <- provider.StreamEvent{
					Type:      provider.EventReasoningDelta,
					TextDelta: choice.Delta.ReasoningContent,
				}
			}
			emitContent(choice.Delta.Content, hasToolCalls)
			for _, tc := range choice.Delta.ToolCalls {
				if !activeCalls[tc.Index] {
					ch <- provider.StreamEvent{
						Type: provider.EventToolCallStart,
						ToolCall: &provider.ToolCallDelta{
							Index: tc.Index,
							ID:    tc.ID,
							Name:  tc.Function.Name,
						},
					}
					activeCalls[tc.Index] = true
				}
				if tc.Function.Arguments != "" || tc.Function.Name != "" || tc.ID != "" {
					ch <- provider.StreamEvent{
						Type: provider.EventToolCallDelta,
						ToolCall: &provider.ToolCallDelta{
							Index:          tc.Index,
							ID:             tc.ID,
							Name:           tc.Function.Name,
							ArgumentsDelta: tc.Function.Arguments,
						},
					}
				}
			}
			if choice.FinishReason != nil {
				reason := *choice.FinishReason
				if reason == "length" || reason == "content_filter" {
					ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("tool call stream stopped with finish_reason %q", reason)}
					return
				}
				if reason != "tool_calls" && len(activeCalls) > 0 {
					ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("tool call stream stopped with finish_reason %q", reason)}
					return
				}
				for idx := range activeCalls {
					ch <- provider.StreamEvent{
						Type:     provider.EventToolCallEnd,
						ToolCall: &provider.ToolCallDelta{Index: idx},
					}
					delete(activeCalls, idx)
				}
			}
		}

		if chunk.Usage != nil {
			flushFilter()
			ch <- provider.StreamEvent{
				Type: provider.EventDone,
				Usage: &provider.Usage{
					InputTokens:  chunk.Usage.PromptTokens,
					OutputTokens: chunk.Usage.CompletionTokens,
				},
			}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() == nil && sctx.Err() != nil {
			ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("provider stream stalled (no data received)")}
			return
		}
		ch <- provider.StreamEvent{Type: provider.EventError, Error: err}
		return
	}
	if len(activeCalls) > 0 {
		ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("tool call stream ended before completion")}
		return
	}
	flushFilter()
	ch <- provider.StreamEvent{Type: provider.EventDone}
}
