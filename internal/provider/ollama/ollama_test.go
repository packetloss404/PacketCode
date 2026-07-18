package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/provider"
)

func TestProvider_Identity(t *testing.T) {
	p := New("")
	assert.Equal(t, "ollama", p.Slug())
	assert.Equal(t, "Ollama", p.Name())
}

func TestProvider_NewDefaultsHost(t *testing.T) {
	p := New("")
	assert.Equal(t, "http://localhost:11434", p.baseURL)
}

func TestProvider_NewNormalizesHost(t *testing.T) {
	assert.Equal(t, "http://ollama.internal:11434", New("ollama.internal").baseURL)
	assert.Equal(t, "http://ollama.internal:11434", New("http://ollama.internal").baseURL)
	assert.Equal(t, "http://ollama.internal:11435", New("http://ollama.internal:11435/").baseURL)
}

func TestProvider_PricingIsZero(t *testing.T) {
	p := New("")
	in, out := p.Pricing("anything")
	assert.Equal(t, 0.0, in)
	assert.Equal(t, 0.0, out)
}

func TestDetectToolSupport(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"qwen2.5-coder:14b", true},
		{"qwen2.5-coder", true},
		{"llama3.3:70b-instruct-q4_K_M", true},
		{"deepseek-coder", false},
		{"codellama:13b", false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, detectToolSupport(tt.model))
		})
	}
}

func TestProvider_ValidateKey_OllamaUnreachable(t *testing.T) {
	// Use a port nothing is listening on.
	p := New("http://127.0.0.1:1")
	err := p.ValidateKey(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not reachable")
}

func TestProvider_ValidateKey_OllamaReachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/tags", r.URL.Path)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()

	p := New(server.URL)
	require.NoError(t, p.ValidateKey(context.Background(), ""))
}

func TestProvider_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{
				"models":[
					{"name":"qwen2.5-coder:14b","model":"qwen2.5-coder:14b","size":9000000000},
					{"name":"deepseek-coder:6.7b","model":"deepseek-coder:6.7b","size":4000000000}
				]
			}`))
		case "/api/show":
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if strings.HasPrefix(body.Model, "qwen2.5-coder") {
				_, _ = io.WriteString(w, `{"capabilities":["completion","tools"],"model_info":{"general.architecture":"qwen2","qwen2.context_length":131072}}`)
			} else {
				_, _ = io.WriteString(w, `{"capabilities":["completion"],"model_info":{"general.architecture":"llama","llama.context_length":16384}}`)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	p := New(server.URL)
	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)

	byID := map[string]provider.Model{}
	for _, m := range models {
		byID[m.ID] = m
	}
	assert.True(t, byID["qwen2.5-coder:14b"].SupportsTools)
	assert.Equal(t, 131072, byID["qwen2.5-coder:14b"].ContextWindow)
	assert.False(t, byID["deepseek-coder:6.7b"].SupportsTools)
	assert.Equal(t, 16384, byID["deepseek-coder:6.7b"].ContextWindow)
}

func TestProvider_ChatCompletion_NDJSONStream(t *testing.T) {
	stream := strings.Join([]string{
		`{"model":"qwen2.5-coder:14b","message":{"role":"assistant","content":"Hello"},"done":false}`,
		`{"model":"qwen2.5-coder:14b","message":{"role":"assistant","content":" world"},"done":false}`,
		`{"model":"qwen2.5-coder:14b","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":11,"eval_count":2}`,
		``,
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := New(server.URL)
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "qwen2.5-coder:14b",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "say hello"}},
	})
	require.NoError(t, err)

	var got strings.Builder
	var done bool
	var usage *provider.Usage
	for ev := range ch {
		switch ev.Type {
		case provider.EventTextDelta:
			got.WriteString(ev.TextDelta)
		case provider.EventDone:
			done = true
			usage = ev.Usage
		}
	}
	assert.Equal(t, "Hello world", got.String())
	assert.True(t, done)
	require.NotNil(t, usage)
	assert.Equal(t, 11, usage.InputTokens)
	assert.Equal(t, 2, usage.OutputTokens)
}

// TestOllama_ChatCompletion_CancellationStopsStream verifies the
// per-iteration ctx.Err() guard in parseOllamaStream: cancelling the
// ctx passed to ChatCompletion closes the NDJSON channel within 1s and
// surfaces an EventError wrapping context.Canceled.
func TestOllama_ChatCompletion_CancellationStopsStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not implement Flusher")
		}
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		for i := 0; i < 50; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(200 * time.Millisecond):
			}
			if _, err := fmt.Fprintf(w,
				"{\"model\":\"qwen2.5-coder:14b\",\"message\":{\"role\":\"assistant\",\"content\":\"chunk %d \"},\"done\":false}\n",
				i); err != nil {
				return
			}
			flusher.Flush()
		}
	}))
	defer server.Close()

	p := New(server.URL)
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := p.ChatCompletion(ctx, provider.ChatRequest{
		Model:    "qwen2.5-coder:14b",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "stream please"}},
	})
	require.NoError(t, err)

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelDrain()

	var events []provider.StreamEvent
	var channelClosed bool
loop:
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				channelClosed = true
				break loop
			}
			events = append(events, ev)
		case <-drainCtx.Done():
			break loop
		}
	}

	assert.True(t, channelClosed, "channel must close within 1s of cancel")
	var sawCancelErr bool
	for _, ev := range events {
		if ev.Type == provider.EventError && ev.Error != nil && errors.Is(ev.Error, context.Canceled) {
			sawCancelErr = true
			break
		}
	}
	assert.True(t, sawCancelErr, "expected EventError wrapping context.Canceled; got events: %+v", events)
}

func TestProvider_ChatCompletion_ToolCall(t *testing.T) {
	stream := strings.Join([]string{
		`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"main.go"}}}]},"done":true,"prompt_eval_count":15,"eval_count":8}`,
		``,
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := New(server.URL)
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "qwen2.5-coder:14b",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "read main.go"}},
	})
	require.NoError(t, err)

	var starts, ends int
	var name, args string
	for ev := range ch {
		switch ev.Type {
		case provider.EventToolCallStart:
			starts++
			name = ev.ToolCall.Name
		case provider.EventToolCallDelta:
			args += ev.ToolCall.ArgumentsDelta
		case provider.EventToolCallEnd:
			ends++
		}
	}
	assert.Equal(t, 1, starts)
	assert.Equal(t, 1, ends)
	assert.Equal(t, "read_file", name)
	assert.JSONEq(t, `{"path":"main.go"}`, args)
}

func TestProvider_ChatCompletion_SuppressesTextOnToolCallChunk(t *testing.T) {
	stream := strings.Join([]string{
		`{"message":{"role":"assistant","content":"<|python_tag|>{\"path\":\"main.go\"}","tool_calls":[{"function":{"name":"read_file","arguments":{"path":"main.go"}}}]},"done":true,"prompt_eval_count":15,"eval_count":8}`,
		``,
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := New(server.URL)
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "qwen2.5-coder:14b",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "read main.go"}},
	})
	require.NoError(t, err)

	var text, args strings.Builder
	for ev := range ch {
		switch ev.Type {
		case provider.EventTextDelta:
			text.WriteString(ev.TextDelta)
		case provider.EventToolCallDelta:
			args.WriteString(ev.ToolCall.ArgumentsDelta)
		case provider.EventError:
			t.Fatalf("unexpected error: %v", ev.Error)
		}
	}
	assert.Empty(t, text.String())
	assert.JSONEq(t, `{"path":"main.go"}`, args.String())
}

func TestNumCtxFor_BucketsAndFloor(t *testing.T) {
	// Tiny prompt → the 16384 floor (8192 reply headroom rules out anything smaller).
	if got := numCtxFor([]chatMessage{{Role: "user", Content: "hi"}}, nil, 0); got != 16384 {
		t.Fatalf("tiny prompt num_ctx = %d, want 16384", got)
	}
	// A prompt whose estimate+headroom crosses 8192 lands in the next bucket.
	big := chatMessage{Role: "user", Content: strings.Repeat("x", 4*10000)} // ~10000 tok
	if got := numCtxFor([]chatMessage{big}, nil, 0); got != 32768 {
		t.Fatalf("10k-token prompt num_ctx = %d, want 32768", got)
	}
	// Enormous prompt caps at the max bucket.
	huge := chatMessage{Role: "user", Content: strings.Repeat("x", 4*200000)}
	if got := numCtxFor([]chatMessage{huge}, nil, 0); got != ollamaMaxNumCtx {
		t.Fatalf("huge prompt num_ctx = %d, want %d", got, ollamaMaxNumCtx)
	}
	// A small model context caps below the floor.
	if got := numCtxFor([]chatMessage{huge}, nil, 8192); got != 8192 {
		t.Fatalf("small-model cap num_ctx = %d, want 8192", got)
	}
	// A model max between buckets caps exactly at the model max.
	if got := numCtxFor([]chatMessage{huge}, nil, 40000); got != 40000 {
		t.Fatalf("model-max cap num_ctx = %d, want 40000", got)
	}
}

func TestFetchMeta_ContextLengthAndTools(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		hits++
		_, _ = io.WriteString(w, `{
			"capabilities": ["completion", "tools", "vision"],
			"model_info": {"general.architecture": "qwen3", "qwen3.context_length": 262144}
		}`)
	}))
	defer srv.Close()

	p := New(srv.URL)
	meta, ok := p.fetchMeta(context.Background(), "qwen3-coder:30b")
	if !ok || meta.contextLength != 262144 || !meta.supportsTools {
		t.Fatalf("fetchMeta = %+v ok=%v, want ctx=262144 tools=true", meta, ok)
	}
	// Cached: a second call must not hit the server.
	if _, _ = p.fetchMeta(context.Background(), "qwen3-coder:30b"); hits != 1 {
		t.Fatalf("expected 1 /api/show hit (cached), got %d", hits)
	}
	if p.ContextWindow("qwen3-coder:30b") != 262144 {
		t.Fatalf("ContextWindow not served from cache")
	}
	if !p.SupportsTools("qwen3-coder:30b") {
		t.Fatalf("SupportsTools not served from cache")
	}
}

func TestFetchMeta_NoToolsCapability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"capabilities":["completion"],"model_info":{"general.architecture":"llama","llama.context_length":32768}}`)
	}))
	defer srv.Close()
	p := New(srv.URL)
	meta, ok := p.fetchMeta(context.Background(), "qwen2.5-coder:32b")
	if !ok || meta.supportsTools {
		t.Fatalf("expected tools=false from capabilities, got %+v", meta)
	}
	// Authoritative: even though the static allow-list would say true for a
	// qwen2.5-coder base name, the cached /api/show answer wins.
	if p.SupportsTools("qwen2.5-coder:32b") {
		t.Fatalf("cached capability (no tools) must override the static allow-list")
	}
}

func TestChatCompletion_SendsNumCtx(t *testing.T) {
	var gotBody chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"model":"m","done":true,"prompt_eval_count":1,"eval_count":1}`+"\n")
	}))
	defer srv.Close()

	p := New(srv.URL)
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "qwen3-coder",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	for range ch {
	}
	if gotBody.Options == nil || gotBody.Options.NumCtx < 8192 {
		t.Fatalf("expected options.num_ctx >= 8192, got %+v", gotBody.Options)
	}
}

func TestToOllamaMessages_ToolResultUsesToolName(t *testing.T) {
	msgs := toOllamaMessages([]provider.Message{
		{Role: provider.RoleUser, Content: "hi"},
		{Role: provider.RoleTool, Name: "read_file", Content: "file body", ToolCallID: "call_1"},
	})
	if msgs[1].Role != "tool" || msgs[1].ToolName != "read_file" || msgs[1].Content != "file body" {
		t.Fatalf("tool result mapped wrong: %+v", msgs[1])
	}
	// The user message must not carry a tool_name.
	if msgs[0].ToolName != "" {
		t.Fatalf("user message should have no tool_name: %+v", msgs[0])
	}
}

func TestChatCompletion_SendsKeepAlive(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"model":"m","done":true,"done_reason":"stop","prompt_eval_count":1,"eval_count":1}`+"\n")
	}))
	defer srv.Close()

	p := New(srv.URL)
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "qwen3-coder",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	for range ch {
	}
	if got.KeepAlive != "30m" {
		t.Fatalf("keep_alive = %q, want 30m", got.KeepAlive)
	}
}

func TestChatCompletion_TruncationIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"model":"m","message":{"role":"assistant","content":"partial"},"done":false}`+"\n")
		_, _ = io.WriteString(w, `{"model":"m","done":true,"done_reason":"length","prompt_eval_count":5,"eval_count":9}`+"\n")
	}))
	defer srv.Close()

	p := New(srv.URL)
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "m",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	var sawErr, sawDone bool
	for ev := range ch {
		switch ev.Type {
		case provider.EventError:
			sawErr = true
			if !strings.Contains(ev.Error.Error(), "truncated") {
				t.Fatalf("error not descriptive: %v", ev.Error)
			}
		case provider.EventDone:
			sawDone = true
		}
	}
	if !sawErr || sawDone {
		t.Fatalf("length truncation should be error, not done: err=%v done=%v", sawErr, sawDone)
	}
}

func TestChatCompletion_ConfigOptionsOverride(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"model":"m","done":true,"done_reason":"stop"}`+"\n")
	}))
	defer srv.Close()

	temp := 0.15
	p := NewWithOptions(srv.URL, Options{NumCtx: 65536, KeepAlive: "-1", Temperature: &temp})
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "qwen3-coder",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	for range ch {
	}
	if got.Options == nil || got.Options.NumCtx != 65536 {
		t.Fatalf("configured num_ctx not used: %+v", got.Options)
	}
	if got.KeepAlive != "-1" {
		t.Fatalf("configured keep_alive not used: %q", got.KeepAlive)
	}
	if got.Options.Temperature == nil || *got.Options.Temperature != 0.15 {
		t.Fatalf("configured temperature not used: %+v", got.Options.Temperature)
	}
}

func TestChatCompletion_DefaultsWhenNoOptions(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"model":"m","done":true,"done_reason":"stop"}`+"\n")
	}))
	defer srv.Close()

	p := New(srv.URL) // zero-config local default
	ch, _ := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "m",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	for range ch {
	}
	if got.KeepAlive != "30m" || got.Options.NumCtx < 16384 || got.Options.Temperature != nil {
		t.Fatalf("zero-config defaults wrong: keep=%q ctx=%d temp=%v", got.KeepAlive, got.Options.NumCtx, got.Options.Temperature)
	}
}
