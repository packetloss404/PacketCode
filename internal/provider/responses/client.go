// Package responses implements the wire protocol for OpenAI's Responses API
// as exposed by the ChatGPT backend used by Codex subscriptions
// (https://chatgpt.com/backend-api/codex/responses).
//
// It is the Responses-API sibling of internal/provider/openaicompat: it
// translates packetcode's provider.ChatRequest into a Responses request,
// streams the server-sent events back, and re-emits them as the same
// provider.StreamEvent sequence the rest of the app already understands. The
// codex provider wraps a Client with subscription auth and model metadata;
// this package never concerns itself with credentials storage or branding.
//
// The two protocols differ in shape: chat-completions carries a flat messages
// array and function tools nested under a "function" key, while the Responses
// API carries a top-level "instructions" string, a typed "input" item array
// (messages, function_call, function_call_output), and flat function tools.
package responses

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/packetcode/packetcode/internal/provider"
)

// userAgent identifies packetcode to the ChatGPT backend. The backend accepts
// the Codex CLI's originator; we present packetcode's own agent string.
const userAgent = "packetcode (+https://github.com/packetcode/packetcode)"

// Auth supplies the subscription credentials for each request. Token returns
// the current access token and ChatGPT account id; Refresh forces a token
// refresh and returns the new pair. The Client calls Refresh at most once per
// request, when the backend rejects the access token with 401.
type Auth interface {
	Token(ctx context.Context) (accessToken, accountID string, err error)
	Refresh(ctx context.Context) (accessToken, accountID string, err error)
}

// Client speaks the Responses API against a configurable base URL. It is safe
// for concurrent use.
type Client struct {
	BaseURL    string
	Auth       Auth
	HTTPClient *http.Client

	// Reasoning effort sent for reasoning-capable models ("low", "medium",
	// "high", ...). Empty means omit the reasoning field entirely.
	ReasoningEffort string

	// EffortFor, when set, overrides ReasoningEffort per model. It lets the
	// codex provider send each model's preferred default effort. A non-empty
	// return wins over ReasoningEffort; an empty return falls back to it.
	EffortFor func(model string) string

	// SummaryFor, when set, returns the reasoning.summary value for a model
	// ("auto", "detailed", …) or "" to omit it. Omitting matters for models
	// that reject the parameter (a 400). Defaults to "auto" when unset.
	SummaryFor func(model string) string
}

// NewClient returns a Client with a streaming-friendly HTTP client (no overall
// timeout; cancellation is driven by context and the stall guard).
func NewClient(baseURL string, auth Auth) *Client {
	return &Client{
		BaseURL:         strings.TrimRight(baseURL, "/"),
		Auth:            auth,
		HTTPClient:      &http.Client{Timeout: 0},
		ReasoningEffort: "medium",
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}

// ---- request wire types ------------------------------------------------

type requestBody struct {
	Model             string          `json:"model"`
	Instructions      string          `json:"instructions,omitempty"`
	Input             []inputItem     `json:"input"`
	Tools             []functionTool  `json:"tools,omitempty"`
	ToolChoice        string          `json:"tool_choice,omitempty"`
	ParallelToolCalls bool            `json:"parallel_tool_calls"`
	Store             bool            `json:"store"`
	Stream            bool            `json:"stream"`
	Reasoning         *reasoningParam `json:"reasoning,omitempty"`
}

type reasoningParam struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// inputItem is one element of the Responses "input" array. The set of fields
// present depends on Type: "message" uses Role+Content; "function_call" uses
// Name+Arguments+CallID; "function_call_output" uses CallID+Output.
type inputItem struct {
	Type      string        `json:"type"`
	Role      string        `json:"role,omitempty"`
	Content   []contentPart `json:"content,omitempty"`
	Name      string        `json:"name,omitempty"`
	Arguments string        `json:"arguments,omitempty"`
	CallID    string        `json:"call_id,omitempty"`
	Output    string        `json:"output,omitempty"`
}

type contentPart struct {
	Type string `json:"type"` // input_text | output_text
	Text string `json:"text"`
}

type functionTool struct {
	Type        string          `json:"type"` // always "function"
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// buildRequest translates a provider.ChatRequest into a Responses request
// body. System messages are concatenated into Instructions; the remaining
// messages become typed input items.
func (c *Client) buildRequest(req provider.ChatRequest) requestBody {
	var instructions []string
	input := make([]inputItem, 0, len(req.Messages))

	for _, m := range req.Messages {
		switch m.Role {
		case provider.RoleSystem:
			if strings.TrimSpace(m.Content) != "" {
				instructions = append(instructions, m.Content)
			}
		case provider.RoleUser:
			input = append(input, inputItem{
				Type: "message",
				Role: "user",
				Content: []contentPart{{
					Type: "input_text",
					Text: m.Content,
				}},
			})
		case provider.RoleAssistant:
			// Assistant turns may carry visible text and/or tool calls. Emit
			// the text (if any) as a message item, then one function_call item
			// per tool call so the backend can correlate later outputs.
			if strings.TrimSpace(m.Content) != "" {
				input = append(input, inputItem{
					Type: "message",
					Role: "assistant",
					Content: []contentPart{{
						Type: "output_text",
						Text: m.Content,
					}},
				})
			}
			for _, tc := range m.ToolCalls {
				input = append(input, inputItem{
					Type:      "function_call",
					Name:      tc.Name,
					Arguments: tc.Arguments,
					CallID:    tc.ID,
				})
			}
		case provider.RoleTool:
			input = append(input, inputItem{
				Type:   "function_call_output",
				CallID: m.ToolCallID,
				Output: m.Content,
			})
		}
	}

	body := requestBody{
		Model:             req.Model,
		Instructions:      strings.Join(instructions, "\n\n"),
		Input:             input,
		Tools:             toFunctionTools(req.Tools),
		ParallelToolCalls: true,
		Store:             false,
		Stream:            true,
	}
	if len(body.Tools) > 0 {
		body.ToolChoice = "auto"
	}
	if effort := c.effortForModel(req.Model); effort != "" {
		// Request a reasoning summary the UI can show as live "thinking".
		// summaryForModel returns "" for models that reject the parameter, so
		// we don't 400 them; models that support it stream summary deltas.
		body.Reasoning = &reasoningParam{Effort: effort, Summary: c.summaryForModel(req.Model)}
	}
	return body
}

// effortForModel resolves the reasoning effort for a model, preferring the
// per-model EffortFor hook and falling back to the static ReasoningEffort.
func (c *Client) effortForModel(model string) string {
	if c.EffortFor != nil {
		if e := c.EffortFor(model); e != "" {
			return e
		}
	}
	return c.ReasoningEffort
}

// summaryForModel resolves the reasoning.summary value for a model. The
// per-model SummaryFor hook decides (returning "" to omit the parameter for
// models that reject it); without a hook we default to "auto".
func (c *Client) summaryForModel(model string) string {
	if c.SummaryFor != nil {
		return c.SummaryFor(model)
	}
	return "auto"
}

func toFunctionTools(tools []provider.ToolDefinition) []functionTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]functionTool, len(tools))
	for i, t := range tools {
		out[i] = functionTool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		}
	}
	return out
}

// ChatCompletion opens a streaming Responses request and translates the event
// stream into provider.StreamEvent values. Errors before the first byte are
// returned synchronously; mid-stream errors surface as EventError.
func (c *Client) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	buf, err := json.Marshal(c.buildRequest(req))
	if err != nil {
		return nil, fmt.Errorf("marshal responses request: %w", err)
	}

	guard, sctx := provider.NewStallGuard(ctx, provider.ConfiguredStallTimeout())

	resp, err := c.send(sctx, buf)
	if err != nil {
		guard.Stop()
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		errBody := provider.ReadErrorBody(resp.Body)
		_ = resp.Body.Close()
		guard.Stop()
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, extractErrorMessage(errBody))
	}

	ch := make(chan provider.StreamEvent, 8)
	go parseSSE(ctx, sctx, guard, resp.Body, ch)
	return ch, nil
}

// send issues the request, reactively refreshing the access token once if the
// backend answers 401. Transient failures (429/5xx) are retried by DoWithRetry
// within each auth attempt.
func (c *Client) send(ctx context.Context, body []byte) (*http.Response, error) {
	for authAttempt := 0; authAttempt < 2; authAttempt++ {
		access, account, err := c.Auth.Token(ctx)
		if err != nil {
			return nil, fmt.Errorf("codex auth: %w", err)
		}

		sessionID := newSessionID()
		newReq := func() (*http.Request, error) {
			r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/responses", bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			applyHeaders(r, access, account, sessionID)
			return r, nil
		}

		resp, err := provider.DoWithRetry(ctx, c.httpClient(), provider.ConfiguredRetry(), newReq)
		if err != nil {
			return nil, fmt.Errorf("request: %w", err)
		}
		if resp.StatusCode == http.StatusUnauthorized && authAttempt == 0 {
			_ = resp.Body.Close()
			if _, _, rerr := c.Auth.Refresh(ctx); rerr != nil {
				return nil, fmt.Errorf("codex token refresh: %w", rerr)
			}
			continue
		}
		return resp, nil
	}
	// Unreachable: the loop either returns a response or an error each pass.
	return nil, fmt.Errorf("codex request failed after token refresh")
}

func applyHeaders(req *http.Request, accessToken, accountID, sessionID string) {
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "codex_cli_rs")
	req.Header.Set("User-Agent", userAgent)
	if sessionID != "" {
		req.Header.Set("session_id", sessionID)
	}
	if accountID != "" {
		req.Header.Set("chatgpt-account-id", accountID)
	}
}

// newSessionID returns a random RFC-4122-ish v4 UUID string. The ChatGPT
// backend expects a session identifier on Codex Responses requests.
func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	s := hex.EncodeToString(b[:])
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}

// extractErrorMessage pulls a human-readable message out of a Responses error
// body, which may be {error:{message}} or {detail:...}. Falls back to the raw
// trimmed body.
func extractErrorMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	var wrapper struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil {
		if wrapper.Error.Message != "" {
			return wrapper.Error.Message
		}
		if wrapper.Detail != "" {
			return wrapper.Detail
		}
	}
	return trimmed
}

// ---- SSE event wire types ---------------------------------------------

// sseEvent is the common envelope; only the fields relevant to a given
// event.type are populated by the server.
type sseEvent struct {
	Type        string       `json:"type"`
	OutputIndex int          `json:"output_index"`
	Delta       string       `json:"delta"`
	Item        *sseItem     `json:"item"`
	Response    *sseResponse `json:"response"`
	Message     string       `json:"message"` // top-level "error" events
	Code        string       `json:"code"`
}

type sseItem struct {
	Type      string `json:"type"` // function_call | message | reasoning
	ID        string `json:"id"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type sseResponse struct {
	Status string    `json:"status"`
	Usage  *sseUsage `json:"usage"`
	Error  *sseError `json:"error"`
}

type sseError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

type sseUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

// callState tracks a streaming function call so we can guarantee the agent
// receives the complete argument string even when the server delivers it only
// in the terminal output_item.done frame rather than as incremental deltas.
type callState struct {
	started  bool
	gotDelta bool
}

// parseSSE reads Responses-API server-sent events and translates them into the
// provider.StreamEvent sequence. It closes ch and body before returning.
func parseSSE(ctx, sctx context.Context, guard *provider.StallGuard, body interface{ Read([]byte) (int, error) }, ch chan<- provider.StreamEvent) {
	defer close(ch)
	if closer, ok := body.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	defer guard.Stop()

	calls := map[int]*callState{}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		if err := provider.StreamHaltError(ctx, sctx); err != nil {
			ch <- provider.StreamEvent{Type: provider.EventError, Error: err}
			return
		}
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		guard.Tick()
		if data == "" || data == "[DONE]" {
			continue
		}

		var ev sseEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("parse SSE event: %w", err)}
			return
		}

		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" {
				ch <- provider.StreamEvent{Type: provider.EventTextDelta, TextDelta: ev.Delta}
			}

		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			if ev.Delta != "" {
				ch <- provider.StreamEvent{Type: provider.EventReasoningDelta, TextDelta: ev.Delta}
			}

		case "response.output_item.added":
			if ev.Item != nil && ev.Item.Type == "function_call" {
				calls[ev.OutputIndex] = &callState{started: true}
				ch <- provider.StreamEvent{
					Type: provider.EventToolCallStart,
					ToolCall: &provider.ToolCallDelta{
						Index: ev.OutputIndex,
						ID:    ev.Item.CallID,
						Name:  ev.Item.Name,
					},
				}
			}

		case "response.function_call_arguments.delta":
			if ev.Delta != "" {
				if st := calls[ev.OutputIndex]; st != nil {
					st.gotDelta = true
				}
				ch <- provider.StreamEvent{
					Type: provider.EventToolCallDelta,
					ToolCall: &provider.ToolCallDelta{
						Index:          ev.OutputIndex,
						ArgumentsDelta: ev.Delta,
					},
				}
			}

		case "response.output_item.done":
			if ev.Item != nil && ev.Item.Type == "function_call" {
				st := calls[ev.OutputIndex]
				// If the server never streamed argument deltas, deliver the
				// complete arguments now so the agent can assemble the call.
				if (st == nil || !st.gotDelta) && ev.Item.Arguments != "" {
					if st == nil || !st.started {
						ch <- provider.StreamEvent{
							Type: provider.EventToolCallStart,
							ToolCall: &provider.ToolCallDelta{
								Index: ev.OutputIndex,
								ID:    ev.Item.CallID,
								Name:  ev.Item.Name,
							},
						}
					}
					ch <- provider.StreamEvent{
						Type: provider.EventToolCallDelta,
						ToolCall: &provider.ToolCallDelta{
							Index:          ev.OutputIndex,
							ArgumentsDelta: ev.Item.Arguments,
						},
					}
				}
				ch <- provider.StreamEvent{
					Type:     provider.EventToolCallEnd,
					ToolCall: &provider.ToolCallDelta{Index: ev.OutputIndex},
				}
				delete(calls, ev.OutputIndex)
			}

		case "response.completed":
			// Close any calls that somehow never emitted a done frame.
			for idx := range calls {
				ch <- provider.StreamEvent{
					Type:     provider.EventToolCallEnd,
					ToolCall: &provider.ToolCallDelta{Index: idx},
				}
			}
			done := provider.StreamEvent{Type: provider.EventDone}
			if ev.Response != nil && ev.Response.Usage != nil {
				done.Usage = &provider.Usage{
					InputTokens:          ev.Response.Usage.InputTokens,
					OutputTokens:         ev.Response.Usage.OutputTokens,
					CacheReadInputTokens: ev.Response.Usage.InputTokensDetails.CachedTokens,
				}
			}
			ch <- done
			return

		case "response.failed", "response.incomplete":
			msg := "response " + strings.TrimPrefix(ev.Type, "response.")
			if ev.Response != nil && ev.Response.Error != nil && ev.Response.Error.Message != "" {
				msg = ev.Response.Error.Message
			}
			ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("%s", msg)}
			return

		case "error":
			msg := ev.Message
			if msg == "" {
				msg = "codex stream error"
			}
			ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("%s", msg)}
			return
		}
		// All other event types (response.created, reasoning_summary_part.*,
		// content_part.*, output_text.done, etc.) are intentionally ignored.
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() == nil && sctx.Err() != nil {
			ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("provider stream stalled (no data received)")}
			return
		}
		ch <- provider.StreamEvent{Type: provider.EventError, Error: err}
		return
	}
	// Stream ended without an explicit completed/failed event.
	if len(calls) > 0 {
		ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("codex stream ended before completion")}
		return
	}
	ch <- provider.StreamEvent{Type: provider.EventDone}
}
