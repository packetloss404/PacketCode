// Package gemini implements provider.Provider for Google's Gemini API.
//
// Gemini's wire protocol differs from OpenAI's in three notable ways:
//  1. Roles are "user" / "model" (not "assistant").
//  2. System messages live under a separate top-level systemInstruction
//     field, not in the contents array.
//  3. Content is structured as parts ([{text}, {functionCall}, ...])
//     rather than a flat string.
//
// This package handles all of that translation in one place so the rest
// of packetcode can keep speaking the unified provider.Message format.
package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/provider"
)

const (
	defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	slug           = "gemini"
	displayName    = "Google Gemini"
)

var brandColor = lipgloss.Color("#4285F4")

type Provider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func New(apiKey string) *Provider {
	return NewWithBaseURL(defaultBaseURL, apiKey)
}

func NewWithBaseURL(baseURL, apiKey string) *Provider {
	return &Provider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

func (p *Provider) Name() string               { return displayName }
func (p *Provider) Slug() string               { return slug }
func (p *Provider) BrandColor() lipgloss.Color { return brandColor }

func (p *Provider) ValidateKey(ctx context.Context, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("api key is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return err
	}
	setAPIKeyHeader(req, apiKey)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("validate key: %w", redactAPIKey(err, apiKey))
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body := provider.ReadErrorBody(resp.Body)
		return fmt.Errorf("validate key: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// redactAPIKey strips the API key out of an error's message while preserving
// the error chain for errors.Is/As.
//
// Gemini used to be authenticated with a ?key=<secret> query parameter, and
// net/http wraps every transport failure in *url.Error, whose Error() prints
// the full request URL — query string included. The key now travels in the
// x-goog-api-key header instead, so no request URL carries it; this scrubber
// stays as defence in depth for any path that still formats a URL built from
// the key. url.Error's own redaction only covers userinfo passwords.
func redactAPIKey(err error, apiKey string) error {
	if err == nil || apiKey == "" {
		return err
	}
	message := err.Error()
	// Both forms: the raw key and the percent-encoded spelling that actually
	// appears in the URL. They are identical for typical keys, but a key with
	// a reserved character must not slip through in its escaped form.
	for _, form := range []string{apiKey, url.QueryEscape(apiKey)} {
		message = strings.ReplaceAll(message, form, "[REDACTED]")
	}
	if message == err.Error() {
		return err
	}
	return &redactedError{message: message, cause: err}
}

// redactedError reports a sanitized message but still unwraps to the original
// error, so callers keep errors.Is(err, context.DeadlineExceeded) and friends.
type redactedError struct {
	message string
	cause   error
}

func (e *redactedError) Error() string { return e.message }
func (e *redactedError) Unwrap() error { return e.cause }

// apiKeyHeader is the header Gemini accepts in place of the ?key= query
// parameter. The key used to travel in the URL, which put it in every place a
// URL is copied without anyone deciding to copy a secret: *url.Error text (see
// redactAPIKey, kept as a second line of defence), proxy and gateway access
// logs, and any request URL a wrapper chooses to print. A header is sent over
// the same TLS connection and lands in none of those.
const apiKeyHeader = "x-goog-api-key"

func setAPIKeyHeader(req *http.Request, apiKey string) {
	if apiKey != "" {
		req.Header.Set(apiKeyHeader, apiKey)
	}
}

// modelsResponse is the Gemini /models response payload.
type modelsResponse struct {
	Models []struct {
		Name                       string   `json:"name"` // "models/gemini-2.5-pro"
		DisplayName                string   `json:"displayName"`
		InputTokenLimit            int      `json:"inputTokenLimit"`
		SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	} `json:"models"`
}

func (p *Provider) ListModels(ctx context.Context) ([]provider.Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	setAPIKeyHeader(req, p.apiKey)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", redactAPIKey(err, p.apiKey))
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body := provider.ReadErrorBody(resp.Body)
		return nil, fmt.Errorf("list models: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}

	out := make([]provider.Model, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		if !supportsGenerate(m.SupportedGenerationMethods) {
			continue
		}
		id := stripModelsPrefix(m.Name)
		in, outRate := p.Pricing(id)
		ctxWindow := m.InputTokenLimit
		if ctxWindow == 0 {
			ctxWindow = p.ContextWindow(id)
		}
		out = append(out, provider.Model{
			ID:            id,
			DisplayName:   m.DisplayName,
			ContextWindow: ctxWindow,
			SupportsTools: p.SupportsTools(id),
			InputPer1M:    in,
			OutputPer1M:   outRate,
		})
	}
	return out, nil
}

func supportsGenerate(methods []string) bool {
	for _, m := range methods {
		if m == "generateContent" || m == "streamGenerateContent" {
			return true
		}
	}
	return false
}

func stripModelsPrefix(name string) string {
	return strings.TrimPrefix(name, "models/")
}

// extractGeminiErrorMessage unwraps the {error:{message:...}} envelope
// Google returns on non-2xx responses so the UI shows a one-line reason
// instead of the raw JSON blob. Falls back to the trimmed raw bytes on
// parse failure.
func extractGeminiErrorMessage(body []byte) string {
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

func (p *Provider) Pricing(modelID string) (float64, float64) {
	if entry, ok := pricingTable[modelID]; ok {
		return entry.Input, entry.Output
	}
	return 1.25, 10.00 // conservative — bias toward overcounting cost
}

func (p *Provider) ContextWindow(modelID string) int {
	if entry, ok := pricingTable[modelID]; ok {
		return entry.ContextWindow
	}
	return 1_000_000
}

func (p *Provider) SupportsTools(modelID string) bool {
	// All Gemini 2.x models support function calling. 1.x is out of scope.
	return strings.HasPrefix(modelID, "gemini-2.")
}

// ────────────────────────────────────────────────────────────────────────────
// Wire types and message translation
// ────────────────────────────────────────────────────────────────────────────

type wirePart struct {
	Text             string                `json:"text,omitempty"`
	FunctionCall     *wireFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *wireFunctionResponse `json:"functionResponse,omitempty"`
}

type wireFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type wireFunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type wireContent struct {
	Role  string     `json:"role,omitempty"`
	Parts []wirePart `json:"parts"`
}

type wireTool struct {
	FunctionDeclarations []wireFunctionDeclaration `json:"functionDeclarations"`
}

type wireFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type wireRequest struct {
	Contents          []wireContent `json:"contents"`
	Tools             []wireTool    `json:"tools,omitempty"`
	SystemInstruction *wireContent  `json:"systemInstruction,omitempty"`
}

// toWireRequest translates packetcode's unified messages into Gemini's
// shape. System messages are pulled out into systemInstruction; tool
// responses become user-role messages with a functionResponse part.
func toWireRequest(req provider.ChatRequest) (wireRequest, error) {
	wr := wireRequest{}

	for _, m := range req.Messages {
		switch m.Role {
		case provider.RoleSystem:
			// Concatenate multiple system messages with newlines into a
			// single systemInstruction. Gemini accepts only one.
			text := m.Content
			if wr.SystemInstruction == nil {
				wr.SystemInstruction = &wireContent{Parts: []wirePart{{Text: text}}}
			} else {
				wr.SystemInstruction.Parts[0].Text += "\n\n" + text
			}
		case provider.RoleUser:
			wr.Contents = append(wr.Contents, wireContent{
				Role:  "user",
				Parts: []wirePart{{Text: m.Content}},
			})
		case provider.RoleAssistant:
			parts := []wirePart{}
			if m.Content != "" {
				parts = append(parts, wirePart{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				args, err := normalizeToolArgs(tc.Name, json.RawMessage(tc.Arguments))
				if err != nil {
					return wr, err
				}
				parts = append(parts, wirePart{
					FunctionCall: &wireFunctionCall{
						Name: tc.Name,
						Args: args,
					},
				})
			}
			wr.Contents = append(wr.Contents, wireContent{
				Role:  "model",
				Parts: parts,
			})
		case provider.RoleTool:
			// Gemini expects tool results as a user-role message with a
			// functionResponse part. The "response" field must be a JSON
			// object; if Content is plain text, wrap it as {"output": ...}.
			respObj, err := wrapToolResponse(m.Content)
			if err != nil {
				return wr, err
			}
			wr.Contents = append(wr.Contents, wireContent{
				Role: "user",
				Parts: []wirePart{{
					FunctionResponse: &wireFunctionResponse{
						Name:     m.Name,
						Response: respObj,
					},
				}},
			})
		}
	}

	for _, t := range req.Tools {
		wr.Tools = append(wr.Tools, wireTool{
			FunctionDeclarations: []wireFunctionDeclaration{{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			}},
		})
	}

	return wr, nil
}

// wrapToolResponse coerces a tool result into the JSON object Gemini wants.
// If the content is already a JSON object we pass it through; otherwise we
// wrap a string in {"output": "..."} so Gemini sees structured data.
func wrapToolResponse(content string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		// Validate it parses as an object.
		var probe map[string]any
		if err := json.Unmarshal([]byte(trimmed), &probe); err == nil {
			return json.RawMessage(trimmed), nil
		}
	}
	return json.Marshal(map[string]string{"output": content})
}

// ────────────────────────────────────────────────────────────────────────────
// Streaming
// ────────────────────────────────────────────────────────────────────────────

type streamChunk struct {
	Candidates []struct {
		Content      wireContent `json:"content"`
		FinishReason string      `json:"finishReason,omitempty"`
		Index        int         `json:"index"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
		// CachedContentTokenCount is the part of promptTokenCount served from
		// cached content. Gemini already counts it in promptTokenCount, so it
		// is reported as a subset and never added to the input total.
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata,omitempty"`
	// PromptFeedback is set, with no candidates, when the prompt itself was
	// blocked. Without reading it the turn ended as an empty success.
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback,omitempty"`
}

// isBlockedFinish reports the finish reasons under which the candidate is not
// the model's answer: a filter fired, or the output cap cut it off. Each of
// these used to fall through to EventDone, so a safety block rendered as a
// finished turn with nothing in it and a truncated answer as a complete one.
func isBlockedFinish(reason string) bool {
	switch reason {
	case "SAFETY", "RECITATION", "PROHIBITED_CONTENT", "BLOCKLIST", "SPII",
		"IMAGE_SAFETY", "LANGUAGE", "MAX_TOKENS":
		return true
	default:
		return false
	}
}

func (p *Provider) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	body, err := toWireRequest(req)
	if err != nil {
		return nil, err
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse",
		p.baseURL, url.PathEscape(req.Model))

	// Build the stall guard BEFORE issuing the request and bind the streaming
	// request/body to the derived context (sctx) so a stall closes the
	// connection and unblocks the parse loop. sctx derives from ctx, so Ctrl+C
	// still propagates.
	guard, sctx := provider.NewStallGuard(ctx, provider.ConfiguredStallTimeout())

	newReq := func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(sctx, http.MethodPost, endpoint, bytes.NewReader(buf))
		if err != nil {
			return nil, err
		}
		setAPIKeyHeader(httpReq, p.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		return httpReq, nil
	}
	resp, err := provider.DoWithRetry(sctx, p.httpClient, provider.ConfiguredRetry(), newReq)
	if err != nil {
		guard.Stop()
		return nil, fmt.Errorf("request: %w", redactAPIKey(err, p.apiKey))
	}
	if resp.StatusCode/100 != 2 {
		errBody := provider.ReadErrorBody(resp.Body)
		_ = resp.Body.Close()
		guard.Stop()
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, extractGeminiErrorMessage(errBody))
	}

	ch := make(chan provider.StreamEvent, 8)
	go parseGeminiSSE(ctx, sctx, guard, resp.Body, ch)
	return ch, nil
}

// parseGeminiSSE reads Gemini's SSE stream (one JSON candidate per event)
// and translates it into provider.StreamEvent values.
//
// Gemini does not stream tool calls token-by-token the way OpenAI does:
// each functionCall part arrives as a complete unit. We still emit the
// Start/Delta/End triple so downstream code (the agent loop) can treat
// every provider uniformly.
//
// ctx is checked once per scanner iteration so Ctrl+C from the App layer
// unblocks the parser promptly even when server keep-alive hides the
// body-close from Scanner. On cancel we emit EventError with ctx.Err()
// so the agent path surfaces the friendlier "turn cancelled" rendering.
func parseGeminiSSE(ctx, sctx context.Context, guard *provider.StallGuard, body io.ReadCloser, ch chan<- provider.StreamEvent) {
	// Every event goes through the sink, which observes cancellation.
	// Bare sends here could block forever on a full buffer if the consumer
	// ever stopped draining, stranding this goroutine with the response
	// body and the stall guard still held.
	sink := provider.NewStreamSink(ctx, ch)

	defer close(ch)
	defer body.Close()
	defer guard.Stop()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var lastUsage *provider.Usage
	toolCallIdx := 0

	for scanner.Scan() {
		if err := provider.StreamHaltError(ctx, sctx); err != nil {
			if !sink.Send(provider.StreamEvent{Type: provider.EventError, Error: err}) {
				return
			}
			return
		}
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		guard.Tick()

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			if !sink.Send(provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("parse gemini chunk: %w", err)}) {
				return
			}
			return
		}

		if chunk.PromptFeedback != nil && chunk.PromptFeedback.BlockReason != "" {
			if !sink.Send(provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("gemini blocked the prompt: %s", chunk.PromptFeedback.BlockReason)}) {
				return
			}
			return
		}
		for _, cand := range chunk.Candidates {
			if isBlockedFinish(cand.FinishReason) {
				if !sink.Send(provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("gemini stopped with finishReason %s", cand.FinishReason)}) {
					return
				}
				return
			}
			if isMalformedFunctionCallFinish(cand.FinishReason) {
				if !sink.Send(provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("gemini returned %s", cand.FinishReason)}) {
					return
				}
				return
			}
			hasFunctionCall := false
			for _, part := range cand.Content.Parts {
				if part.FunctionCall != nil {
					hasFunctionCall = true
					break
				}
			}
			for _, part := range cand.Content.Parts {
				if part.Text != "" && !hasFunctionCall {
					if !sink.Send(provider.StreamEvent{
						Type:      provider.EventTextDelta,
						TextDelta: part.Text,
					}) {
						return
					}
				}
				if part.FunctionCall != nil {
					args, err := normalizeToolArgs(part.FunctionCall.Name, part.FunctionCall.Args)
					if err != nil {
						if !sink.Send(provider.StreamEvent{Type: provider.EventError, Error: err}) {
							return
						}
						return
					}
					id := fmt.Sprintf("call_%d", toolCallIdx)
					if !sink.Send(provider.StreamEvent{
						Type: provider.EventToolCallStart,
						ToolCall: &provider.ToolCallDelta{
							Index: toolCallIdx,
							ID:    id,
							Name:  part.FunctionCall.Name,
						},
					}) {
						return
					}
					if !sink.Send(provider.StreamEvent{
						Type: provider.EventToolCallDelta,
						ToolCall: &provider.ToolCallDelta{
							Index:          toolCallIdx,
							ID:             id,
							Name:           part.FunctionCall.Name,
							ArgumentsDelta: string(args),
						},
					}) {
						return
					}
					if !sink.Send(provider.StreamEvent{
						Type:     provider.EventToolCallEnd,
						ToolCall: &provider.ToolCallDelta{Index: toolCallIdx},
					}) {
						return
					}
					toolCallIdx++
				}
			}
		}

		if chunk.UsageMetadata != nil {
			lastUsage = &provider.Usage{
				InputTokens:          chunk.UsageMetadata.PromptTokenCount,
				OutputTokens:         chunk.UsageMetadata.CandidatesTokenCount,
				CacheReadInputTokens: chunk.UsageMetadata.CachedContentTokenCount,
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() == nil && sctx.Err() != nil {
			if !sink.Send(provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("provider stream stalled (no data received)")}) {
				return
			}
			return
		}
		if !sink.Send(provider.StreamEvent{Type: provider.EventError, Error: err}) {
			return
		}
		return
	}
	if !sink.Send(provider.StreamEvent{Type: provider.EventDone, Usage: lastUsage}) {
		return
	}
}

func normalizeToolArgs(name string, args json.RawMessage) (json.RawMessage, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("gemini function call missing name")
	}
	trimmed := strings.TrimSpace(string(args))
	if trimmed == "" {
		return json.RawMessage("{}"), nil
	}
	if !strings.HasPrefix(trimmed, "{") {
		return nil, fmt.Errorf("gemini function call %q args must be a JSON object", name)
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		return nil, fmt.Errorf("gemini function call %q args: %w", name, err)
	}
	return json.RawMessage(trimmed), nil
}

func isMalformedFunctionCallFinish(reason string) bool {
	switch reason {
	case "MALFORMED_FUNCTION_CALL", "UNEXPECTED_TOOL_CALL":
		return true
	default:
		return false
	}
}
