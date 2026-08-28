package openaicompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/provider"
)

func TestChatCompletionStreamsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
		if fl != nil {
			fl.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client := &Client{BaseURL: srv.URL, APIKey: "test"}
	ev, err := client.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "gpt-4",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	var text string
	var sawDone bool
	for e := range ev {
		switch e.Type {
		case provider.EventTextDelta:
			text += e.TextDelta
		case provider.EventDone:
			sawDone = true
		case provider.EventError:
			t.Fatalf("unexpected stream error: %v", e.Error)
		}
	}
	if text != "Hello" {
		t.Fatalf("text = %q, want %q", text, "Hello")
	}
	if !sawDone {
		t.Fatal("expected an EventDone")
	}
}

func TestChatCompletionDoesNotLeakSugarCacheWithoutSugarAdapter(t *testing.T) {
	requestBody := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		requestBody <- body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test")
	events, err := client.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "direct-model",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
		SugarCache: &provider.SugarCacheMetadata{
			ConversationID:    "session-1",
			PrefixFingerprint: provider.CachePrefixFingerprint("", nil),
			Mode:              provider.SugarCacheAuto,
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	for range events {
	}
	body := <-requestBody
	if _, leaked := body["sugar_cache"]; leaked {
		t.Fatal("Sugar-only cache metadata leaked through the generic OpenAI-compatible client")
	}
}

func TestMarshalChatRequestCanonicalizesExactToolWirePrefix(t *testing.T) {
	left := provider.ChatRequest{Model: "model", Tools: []provider.ToolDefinition{
		{Name: "zeta", Parameters: json.RawMessage(`{"type":"object","properties":{"b":{"type":"string"},"a":{"type":"integer"}}}`)},
		{Name: "alpha", Parameters: json.RawMessage(`{"required":["path"],"type":"object"}`)},
	}}
	right := provider.ChatRequest{Model: "model", Tools: []provider.ToolDefinition{
		{Name: "alpha", Parameters: json.RawMessage(`{ "type": "object", "required": ["path"] }`)},
		{Name: "zeta", Parameters: json.RawMessage(`{"properties":{"a":{"type":"integer"},"b":{"type":"string"}},"type":"object"}`)},
	}}
	leftJSON, err := MarshalChatRequest(left)
	if err != nil {
		t.Fatal(err)
	}
	rightJSON, err := MarshalChatRequest(right)
	if err != nil {
		t.Fatal(err)
	}
	if string(leftJSON) != string(rightJSON) {
		t.Fatalf("canonical wire differs:\n%s\n%s", leftJSON, rightJSON)
	}
}

// TestChatCompletionStallTimeoutAborts drives a real stalled stream end-to-end:
// the server opens the SSE response (so ChatCompletion returns successfully and
// the parse loop is reading the body) and then goes silent forever. The stall
// guard must cancel the request within the configured window, close the
// connection, unblock the parse loop's read, and surface an EventError.
//
// This is the integration test the per-call stall timeout round requires: it
// exercises the actual adapter parse loop against a genuine stalled HTTP body,
// not the StallGuard in isolation.
func TestChatCompletionStallTimeoutAborts(t *testing.T) {
	prev := provider.ConfiguredStallTimeout()
	provider.SetConfiguredStallTimeout(150 * time.Millisecond)
	defer provider.SetConfiguredStallTimeout(prev)

	// release lets the handler return once the client side is done, so the
	// test server can shut down cleanly.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush() // open the stream, then send nothing (silent stall).
		}
		select {
		case <-r.Context().Done(): // client cancelled (the stall guard fired).
		case <-release:
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	defer close(release)

	client := &Client{BaseURL: srv.URL, APIKey: "test"}
	ev, err := client.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "gpt-4",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		select {
		case e, ok := <-ev:
			if !ok {
				t.Fatal("stream closed without a stall EventError")
			}
			if e.Type == provider.EventError {
				if e.Error == nil {
					t.Fatal("EventError with nil error")
				}
				// Parent context was never cancelled, so this must be the
				// stall message rather than a propagated cancellation.
				if got := e.Error.Error(); got != "provider stream stalled (no data received)" {
					t.Fatalf("unexpected error %q, want the stall message", got)
				}
				return
			}
		case <-deadline:
			t.Fatal("stall timeout did not fire within the window")
		}
	}
}

// TestChatCompletionParentCancelPropagates verifies that cancelling the parent
// context (e.g. Ctrl+C) still aborts the stream and surfaces the parent's
// cancellation cause rather than the stall message.
func TestChatCompletionParentCancelPropagates(t *testing.T) {
	prev := provider.ConfiguredStallTimeout()
	provider.SetConfiguredStallTimeout(10 * time.Second) // long: stall must not fire first.
	defer provider.SetConfiguredStallTimeout(prev)

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-release:
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	client := &Client{BaseURL: srv.URL, APIKey: "test"}
	ev, err := client.ChatCompletion(ctx, provider.ChatRequest{
		Model:    "gpt-4",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		cancel()
		t.Fatalf("ChatCompletion: %v", err)
	}

	time.AfterFunc(100*time.Millisecond, cancel)

	deadline := time.After(3 * time.Second)
	for {
		select {
		case e, ok := <-ev:
			if !ok {
				t.Fatal("stream closed without a cancellation EventError")
			}
			if e.Type == provider.EventError {
				if e.Error != context.Canceled {
					t.Fatalf("error = %v, want context.Canceled", e.Error)
				}
				return
			}
		case <-deadline:
			cancel()
			t.Fatal("parent cancellation did not abort the stream")
		}
	}
}

// usageFromStream runs one SSE frame carrying a usage object through the
// client and returns the Usage attached to EventDone.
func usageFromStream(t *testing.T, usageJSON string) *provider.Usage {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":" + usageJSON + "}\n\n"))
	}))
	defer srv.Close()

	client := &Client{BaseURL: srv.URL, APIKey: "test"}
	ev, err := client.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "gpt-4",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	var got *provider.Usage
	for e := range ev {
		if e.Type == provider.EventError {
			t.Fatalf("unexpected stream error: %v", e.Error)
		}
		if e.Type == provider.EventDone && e.Usage != nil {
			got = e.Usage
		}
	}
	if got == nil {
		t.Fatal("no usage reported on EventDone")
	}
	return got
}

// TestUsageCachedTokensAreASubsetOfPrompt pins the contract that makes every
// downstream cost figure correct: OpenAI-compatible prompt_tokens already
// counts cached input, so cached_tokens is surfaced as a subset and must
// never be added to InputTokens. This one struct feeds eight providers, so a
// regression here double-counts cached prompts across all of them.
func TestUsageCachedTokensAreASubsetOfPrompt(t *testing.T) {
	got := usageFromStream(t, `{"prompt_tokens":1000,"completion_tokens":50,"prompt_tokens_details":{"cached_tokens":800}}`)
	if got.InputTokens != 1000 {
		t.Fatalf("InputTokens = %d, want 1000 (cached tokens must not be added)", got.InputTokens)
	}
	if got.OutputTokens != 50 {
		t.Fatalf("OutputTokens = %d, want 50", got.OutputTokens)
	}
	if got.CacheReadInputTokens != 800 {
		t.Fatalf("CacheReadInputTokens = %d, want 800", got.CacheReadInputTokens)
	}
	if got.CacheReadInputTokens > got.InputTokens {
		t.Fatal("cached tokens must be a subset of the prompt total")
	}
	// No provider on this path reports a cache-write count; leaving it zero
	// keeps it from being mistaken for one.
	if got.CacheCreationInputTokens != 0 {
		t.Fatalf("CacheCreationInputTokens = %d, want 0", got.CacheCreationInputTokens)
	}
}

// TestUsageWithoutCachedTokensDetail covers the backends that omit
// prompt_tokens_details entirely: the cache fields stay zero and the input
// total is untouched.
func TestUsageWithoutCachedTokensDetail(t *testing.T) {
	got := usageFromStream(t, `{"prompt_tokens":1000,"completion_tokens":50}`)
	if got.InputTokens != 1000 || got.OutputTokens != 50 {
		t.Fatalf("usage = %+v, want input 1000 / output 50", *got)
	}
	if got.CacheReadInputTokens != 0 || got.CacheCreationInputTokens != 0 {
		t.Fatalf("cache fields = %d/%d, want 0/0", got.CacheCreationInputTokens, got.CacheReadInputTokens)
	}
}
