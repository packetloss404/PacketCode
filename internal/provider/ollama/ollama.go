// Package ollama implements provider.Provider against an Ollama instance.
//
// Two characteristics make Ollama the odd one out:
//  1. There's no API key. ValidateKey is a no-op; "validation" really
//     means "is the daemon reachable on this host?"
//  2. The streaming format is NDJSON (one JSON object per line), not SSE.
//
// Tool calling support is per-model. SupportsTools lets callers decide
// whether to send native tool definitions or run without tools for that
// model.
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/packetcode/packetcode/internal/provider"
)

const (
	defaultBaseURL = "http://localhost:11434"
	slug           = "ollama"
	displayName    = "Ollama"
)

// brandColor: deliberately neutral white-ish to match the design tokens
// (Ollama doesn't really have a brand color in the same way the cloud
// providers do).
var brandColor = lipgloss.Color("#E1E1E8")

// modelsKnownToSupportTools is the conservative allow-list of locally-
// hostable model families that ship native tool calling. Anything not on
// this list still loads, but SupportsTools returns false so the agent loop
// omits native tool definitions.
var modelsKnownToSupportTools = []string{
	"qwen2.5", "qwen2.5-coder", "qwen3",
	"llama3.1", "llama3.2", "llama3.3",
	"mistral-nemo", "mistral-small",
	"firefunction",
	"command-r", "command-r-plus",
}

// Options holds optional user tuning from [providers.ollama]. The zero value
// means "use packetcode's smart defaults", so a stock local install needs none
// of it.
type Options struct {
	NumCtx      int      // fixed context window; 0 = auto-size per request
	KeepAlive   string   // e.g. "30m", "-1" (pin), "0" (unload now); "" = default
	Temperature *float64 // nil = leave to the model
}

type Provider struct {
	baseURL    string
	httpClient *http.Client
	opts       Options

	// metaCache memoizes per-model /api/show results (context length + tool
	// capability). Ollama's /api/tags list carries neither, so we enrich each
	// model with a /api/show call and cache it. Guarded by mu.
	mu        sync.Mutex
	metaCache map[string]modelMeta
	lastGen   GenStats // timings from the most recent completion; guarded by mu
}

// GenStats captures the timing of one completion, from which the UI can derive
// tokens/sec and time-to-first-token — the key feel signals for local models.
type GenStats struct {
	PromptTokens       int
	OutputTokens       int
	PromptEvalDuration time.Duration // time spent ingesting the prompt (prefill)
	EvalDuration       time.Duration // time spent generating the reply
	LoadDuration       time.Duration // time to load the model (0 if already resident)
	TotalDuration      time.Duration
}

// OutputTokensPerSec is decode throughput.
func (s GenStats) OutputTokensPerSec() float64 { return tokPerSec(s.OutputTokens, s.EvalDuration) }

// PromptTokensPerSec is prefill throughput.
func (s GenStats) PromptTokensPerSec() float64 {
	return tokPerSec(s.PromptTokens, s.PromptEvalDuration)
}

// TimeToFirstToken approximates latency before generation starts: model load
// plus prompt prefill.
func (s GenStats) TimeToFirstToken() time.Duration { return s.LoadDuration + s.PromptEvalDuration }

func tokPerSec(tokens int, d time.Duration) float64 {
	if d <= 0 || tokens <= 0 {
		return 0
	}
	return float64(tokens) / d.Seconds()
}

func (p *Provider) setLastGen(s GenStats) {
	p.mu.Lock()
	p.lastGen = s
	p.mu.Unlock()
}

// LastGenStats returns the timing of the most recent completion (zero value if
// none yet).
func (p *Provider) LastGenStats() GenStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastGen
}

// modelMeta is the distilled /api/show metadata packetcode needs.
type modelMeta struct {
	contextLength int  // the model's trained max context (0 if unknown)
	supportsTools bool // capabilities array contains "tools"
}

func New(host string) *Provider {
	return NewWithOptions(host, Options{})
}

// NewWithOptions constructs a provider with user tuning from config. An empty
// host falls back to the local default.
func NewWithOptions(host string, opts Options) *Provider {
	if host == "" {
		host = defaultBaseURL
	}
	return &Provider{
		baseURL:    normalizeHost(host),
		httpClient: &http.Client{},
		opts:       opts,
		metaCache:  map[string]modelMeta{},
	}
}

func (p *Provider) Name() string               { return displayName }
func (p *Provider) Slug() string               { return slug }
func (p *Provider) BrandColor() lipgloss.Color { return brandColor }

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.TrimRight(host, "/"))
	if host == "" {
		return defaultBaseURL
	}
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}
	withoutScheme := strings.TrimPrefix(strings.TrimPrefix(host, "http://"), "https://")
	if !strings.Contains(withoutScheme, ":") {
		host += ":11434"
	}
	return host
}

// ValidateKey ignores the apiKey argument and instead probes the daemon
// reachability. Returns nil iff GET /api/tags succeeds.
func (p *Provider) ValidateKey(ctx context.Context, _ string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama not reachable at %s — is it running? start it with `ollama serve` (%w)", p.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ollama not reachable at %s: status %d", p.baseURL, resp.StatusCode)
	}
	return nil
}

// tagsResponse is the GET /api/tags payload — pulled (local) models.
type tagsResponse struct {
	Models []struct {
		Name       string `json:"name"`
		Model      string `json:"model"`
		Size       int64  `json:"size"`
		ModifiedAt string `json:"modified_at"`
	} `json:"models"`
}

func (p *Provider) ListModels(ctx context.Context) ([]provider.Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list ollama models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body := provider.ReadErrorBody(resp.Body)
		return nil, fmt.Errorf("list ollama models: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode ollama models: %w", err)
	}
	out := make([]provider.Model, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		// Enrich each model with /api/show (context length + tool capability).
		// Best-effort: if it fails, fall back to the static name allow-list and
		// an unknown (0) context window.
		meta, ok := p.fetchMeta(ctx, m.Name)
		supportsTools := meta.supportsTools
		if !ok {
			supportsTools = detectToolSupport(m.Name)
		}
		out = append(out, provider.Model{
			ID:            m.Name,
			DisplayName:   m.Name,
			ContextWindow: meta.contextLength,
			SupportsTools: supportsTools,
		})
	}
	return out, nil
}

// showResponse is the subset of POST /api/show packetcode reads.
type showResponse struct {
	Capabilities []string                   `json:"capabilities"`
	ModelInfo    map[string]json.RawMessage `json:"model_info"`
}

// fetchMeta returns cached /api/show metadata for a model, fetching it on a
// miss. The second return is false when the daemon is unreachable or the model
// is unknown, so callers can fall back. Results (including a successful fetch
// that reported no tool support) are cached; failures are not.
func (p *Provider) fetchMeta(ctx context.Context, model string) (modelMeta, bool) {
	p.mu.Lock()
	if m, ok := p.metaCache[model]; ok {
		p.mu.Unlock()
		return m, true
	}
	p.mu.Unlock()

	body, err := json.Marshal(map[string]string{"model": model})
	if err != nil {
		return modelMeta{}, false
	}
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, p.baseURL+"/api/show", bytes.NewReader(body))
	if err != nil {
		return modelMeta{}, false
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return modelMeta{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return modelMeta{}, false
	}
	var sr showResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return modelMeta{}, false
	}

	meta := modelMeta{
		contextLength: contextLengthFromModelInfo(sr.ModelInfo),
		supportsTools: containsString(sr.Capabilities, "tools"),
	}
	p.mu.Lock()
	p.metaCache[model] = meta
	p.mu.Unlock()
	return meta, true
}

// contextLengthFromModelInfo pulls the model's trained context length out of
// the model_info map. The key is "<architecture>.context_length" where the
// architecture is itself under "general.architecture" (e.g. "qwen3"). Falls
// back to any key ending in ".context_length".
func contextLengthFromModelInfo(info map[string]json.RawMessage) int {
	if info == nil {
		return 0
	}
	var arch string
	if raw, ok := info["general.architecture"]; ok {
		_ = json.Unmarshal(raw, &arch)
	}
	if arch != "" {
		if raw, ok := info[arch+".context_length"]; ok {
			var n int
			if json.Unmarshal(raw, &n) == nil {
				return n
			}
		}
	}
	for k, raw := range info {
		if strings.HasSuffix(k, ".context_length") {
			var n int
			if json.Unmarshal(raw, &n) == nil {
				return n
			}
		}
	}
	return 0
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// cachedContextLength returns a model's context length from the cache without a
// network call. 0 means unknown (not yet fetched or unavailable).
func (p *Provider) cachedContextLength(model string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.metaCache[model].contextLength
}

// detectToolSupport matches the model name against the curated allow-list.
// We strip any tag suffix (":14b", ":latest") before comparing so e.g.
// "qwen2.5-coder:14b" still matches "qwen2.5-coder".
func detectToolSupport(modelName string) bool {
	base := modelName
	if i := strings.IndexByte(modelName, ':'); i != -1 {
		base = modelName[:i]
	}
	for _, supported := range modelsKnownToSupportTools {
		if base == supported {
			return true
		}
	}
	return false
}

// ────────────────────────────────────────────────────────────────────────────
// Streaming
// ────────────────────────────────────────────────────────────────────────────

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	Stream    bool          `json:"stream"`
	Tools     []chatTool    `json:"tools,omitempty"`
	Options   *chatOptions  `json:"options,omitempty"`
	KeepAlive string        `json:"keep_alive,omitempty"`
}

// defaultKeepAlive keeps a loaded model resident between turns. Ollama unloads
// after 5 minutes of idle by default, which adds a multi-second reload to the
// next request; 30 minutes comfortably spans an interactive coding session
// without pinning the model in memory forever.
const defaultKeepAlive = "30m"

// chatOptions carries Ollama runtime options. Only num_ctx is set today; more
// (temperature, keep_alive plumbing) arrive with the options-config work.
type chatOptions struct {
	// NumCtx is the context window, in tokens. Ollama's default (~4K) silently
	// truncates the prompt, so packetcode always sets this explicitly.
	NumCtx int `json:"num_ctx,omitempty"`
	// Temperature is set only when the user configured one.
	Temperature *float64 `json:"temperature,omitempty"`
}

// Context-window sizing. Ollama silently truncates any prompt that exceeds the
// model's loaded context window, so packetcode sets num_ctx explicitly, sized
// to the prompt plus room for a reply. The value is snapped up to a bucket so
// it changes only when the conversation crosses a size boundary — each change
// forces Ollama to reallocate the KV cache (a reload), so churning it per
// request would be slow.
const (
	ollamaReplyHeadroom = 8192   // tokens reserved for the model's reply
	ollamaMaxNumCtx     = 131072 // upper bound (refined to the model's max later)
)

// numCtxBuckets are the context sizes packetcode snaps to. 16384 is the floor:
// with 8192 tokens reserved for the reply, a smaller window leaves too little
// room for an agentic prompt (system prompt + tool schemas + a file or two).
var numCtxBuckets = []int{16384, 32768, 65536, 131072}

// estimateTokens approximates the prompt size. No local tokenizer is available,
// so it uses the standard ~4-chars-per-token heuristic — deliberately rough;
// it only needs to pick the right bucket, and the headroom absorbs error.
func estimateTokens(msgs []chatMessage, tools []chatTool) int {
	chars := 0
	for _, m := range msgs {
		chars += len(m.Role) + len(m.Content) + len(m.ToolName)
		for _, tc := range m.ToolCalls {
			chars += len(tc.Function.Name) + len(tc.Function.Arguments)
		}
	}
	for _, t := range tools {
		chars += len(t.Function.Name) + len(t.Function.Description) + len(t.Function.Parameters)
	}
	return chars / 4
}

// numCtxFor picks the smallest bucket that holds the estimated prompt plus
// reply headroom, capped at ollamaMaxNumCtx and, when known (>0), at the
// model's own trained context length modelMax — asking for more than the model
// supports wastes memory and Ollama clamps it anyway.
func numCtxFor(msgs []chatMessage, tools []chatTool, modelMax int) int {
	limit := ollamaMaxNumCtx
	if modelMax > 0 && modelMax < limit {
		limit = modelMax
	}
	need := estimateTokens(msgs, tools) + ollamaReplyHeadroom
	for _, bucket := range numCtxBuckets {
		if bucket >= limit {
			return limit
		}
		if need <= bucket {
			return bucket
		}
	}
	return limit
}

type chatMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
	// ToolName identifies the function a "tool"-role result belongs to. Ollama's
	// native /api/chat schema names this field tool_name (an earlier version of
	// this provider sent the non-standard "name", which Ollama ignored).
	ToolName string `json:"tool_name,omitempty"`
}

type chatToolCall struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type chatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type chatChunk struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Message   struct {
		Role      string         `json:"role"`
		Content   string         `json:"content"`
		ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
	} `json:"message"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"` // nanoseconds
	EvalDuration       int64  `json:"eval_duration,omitempty"`        // nanoseconds
	LoadDuration       int64  `json:"load_duration,omitempty"`        // nanoseconds
	TotalDuration      int64  `json:"total_duration,omitempty"`       // nanoseconds
}

func toOllamaMessages(msgs []provider.Message) []chatMessage {
	out := make([]chatMessage, 0, len(msgs))
	for _, m := range msgs {
		om := chatMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
		// Ollama correlates a tool result to its call via tool_name on a
		// "tool"-role message.
		if m.Role == provider.RoleTool {
			om.ToolName = m.Name
		}
		if len(m.ToolCalls) > 0 {
			om.ToolCalls = make([]chatToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				args := json.RawMessage(tc.Arguments)
				if len(args) == 0 {
					args = json.RawMessage("{}")
				}
				om.ToolCalls[i].Function.Name = tc.Name
				om.ToolCalls[i].Function.Arguments = args
			}
		}
		out = append(out, om)
	}
	return out
}

func toOllamaTools(tools []provider.ToolDefinition) []chatTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]chatTool, len(tools))
	for i, t := range tools {
		out[i].Type = "function"
		out[i].Function.Name = t.Name
		out[i].Function.Description = t.Description
		out[i].Function.Parameters = t.Parameters
	}
	return out
}

func (p *Provider) ChatCompletion(ctx context.Context, req provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	msgs := toOllamaMessages(req.Messages)
	tools := toOllamaTools(req.Tools)
	// Cap num_ctx to the model's real context length when we know it. The cache
	// is warmed by ListModels at startup and on model switch, so this needs no
	// extra round-trip here; a cold miss (0) just means "uncapped auto-size".
	modelMax := p.cachedContextLength(req.Model)

	// num_ctx: an explicit config value wins (still capped to the model max);
	// otherwise auto-size to the prompt.
	numCtx := numCtxFor(msgs, tools, modelMax)
	if p.opts.NumCtx > 0 {
		numCtx = p.opts.NumCtx
		if modelMax > 0 && numCtx > modelMax {
			numCtx = modelMax
		}
	}
	keepAlive := defaultKeepAlive
	if p.opts.KeepAlive != "" {
		keepAlive = p.opts.KeepAlive
	}

	body := chatRequest{
		Model:     req.Model,
		Messages:  msgs,
		Stream:    true,
		Tools:     tools,
		Options:   &chatOptions{NumCtx: numCtx, Temperature: p.opts.Temperature},
		KeepAlive: keepAlive,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	// Build the stall guard BEFORE issuing the request and bind the streaming
	// request/body to the derived context (sctx) so a stall closes the
	// connection and unblocks the parse loop. sctx derives from ctx, so Ctrl+C
	// still propagates.
	guard, sctx := provider.NewStallGuard(ctx, provider.ConfiguredStallTimeout())

	newReq := func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(sctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(buf))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		return httpReq, nil
	}
	resp, err := provider.DoWithRetry(sctx, p.httpClient, provider.ConfiguredRetry(), newReq)
	if err != nil {
		guard.Stop()
		return nil, fmt.Errorf("ollama chat: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		errBody := provider.ReadErrorBody(resp.Body)
		_ = resp.Body.Close()
		guard.Stop()
		return nil, fmt.Errorf("ollama chat: status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	ch := make(chan provider.StreamEvent, 8)
	go p.parseOllamaStream(ctx, sctx, guard, resp.Body, ch)
	return ch, nil
}

// parseOllamaStream reads NDJSON (one JSON object per line) and translates
// chunks into provider.StreamEvent values.
//
// Ollama emits tool calls as a single complete object on the message that
// also carries done=true (or earlier, for some models). We buffer them and
// emit Start/Delta/End as one unit per call to keep the stream protocol
// uniform with the other providers.
//
// ctx is checked once per NDJSON line so Ctrl+C from the App layer
// unblocks the parser promptly even when local daemon keep-alive hides
// the body-close from Scanner. On cancel we emit EventError with
// ctx.Err() so the agent surfaces the friendlier "turn cancelled" line.
func (p *Provider) parseOllamaStream(ctx, sctx context.Context, guard *provider.StallGuard, body io.ReadCloser, ch chan<- provider.StreamEvent) {
	defer close(ch)
	defer body.Close()
	defer guard.Stop()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	toolIdx := 0
	var lastUsage *provider.Usage

	for scanner.Scan() {
		if err := provider.StreamHaltError(ctx, sctx); err != nil {
			ch <- provider.StreamEvent{Type: provider.EventError, Error: err}
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		guard.Tick()
		var chunk chatChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("parse ollama chunk: %w", err)}
			return
		}

		if chunk.Message.Content != "" && len(chunk.Message.ToolCalls) == 0 {
			ch <- provider.StreamEvent{
				Type:      provider.EventTextDelta,
				TextDelta: chunk.Message.Content,
			}
		}

		for _, tc := range chunk.Message.ToolCalls {
			id := fmt.Sprintf("call_%d", toolIdx)
			ch <- provider.StreamEvent{
				Type: provider.EventToolCallStart,
				ToolCall: &provider.ToolCallDelta{
					Index: toolIdx,
					ID:    id,
					Name:  tc.Function.Name,
				},
			}
			ch <- provider.StreamEvent{
				Type: provider.EventToolCallDelta,
				ToolCall: &provider.ToolCallDelta{
					Index:          toolIdx,
					ID:             id,
					Name:           tc.Function.Name,
					ArgumentsDelta: string(tc.Function.Arguments),
				},
			}
			ch <- provider.StreamEvent{
				Type:     provider.EventToolCallEnd,
				ToolCall: &provider.ToolCallDelta{Index: toolIdx},
			}
			toolIdx++
		}

		if chunk.Done {
			lastUsage = &provider.Usage{
				InputTokens:  chunk.PromptEvalCount,
				OutputTokens: chunk.EvalCount,
			}
			// Record generation timings so the UI can show tokens/sec and
			// time-to-first-token for the local model.
			p.setLastGen(GenStats{
				PromptTokens:       chunk.PromptEvalCount,
				OutputTokens:       chunk.EvalCount,
				PromptEvalDuration: time.Duration(chunk.PromptEvalDuration),
				EvalDuration:       time.Duration(chunk.EvalDuration),
				LoadDuration:       time.Duration(chunk.LoadDuration),
				TotalDuration:      time.Duration(chunk.TotalDuration),
			})
			// done_reason "length" means the model hit the token/context limit
			// and the output was truncated — surface it as an error like the
			// OpenAI-compatible path rather than presenting a partial answer as
			// complete. "stop" (and empty) is a normal completion.
			if chunk.DoneReason == "length" {
				ch <- provider.StreamEvent{Type: provider.EventError, Error: fmt.Errorf("ollama response truncated (hit token limit; raise num_ctx or shorten the prompt)")}
				return
			}
			ch <- provider.StreamEvent{Type: provider.EventDone, Usage: lastUsage}
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
	ch <- provider.StreamEvent{Type: provider.EventDone, Usage: lastUsage}
}

// Pricing — local inference is free.
func (p *Provider) Pricing(modelID string) (float64, float64) { return 0, 0 }

// ContextWindow returns the model's trained context length, read from
// /api/show and cached by ListModels/ChatCompletion. Returns 0 (unknown) if the
// model hasn't been queried yet, so the UI can hide the bar or show a dash.
func (p *Provider) ContextWindow(modelID string) int {
	return p.cachedContextLength(modelID)
}

// SupportsTools reports whether the model advertises the "tools" capability via
// /api/show (cached). Before the model has been queried it falls back to the
// static name allow-list so an early call still gives a reasonable answer.
func (p *Provider) SupportsTools(modelID string) bool {
	p.mu.Lock()
	meta, ok := p.metaCache[modelID]
	p.mu.Unlock()
	if ok {
		return meta.supportsTools
	}
	return detectToolSupport(modelID)
}
