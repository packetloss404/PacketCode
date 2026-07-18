package responses

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/packetcode/packetcode/internal/provider"
)

// fakeAuth is a test double for the Auth interface.
type fakeAuth struct {
	access    string
	account   string
	refreshes int32
}

func (f *fakeAuth) Token(context.Context) (string, string, error) {
	return f.access, f.account, nil
}

func (f *fakeAuth) Refresh(context.Context) (string, string, error) {
	atomic.AddInt32(&f.refreshes, 1)
	f.access = "refreshed-token"
	return f.access, f.account, nil
}

func TestBuildRequestTranslation(t *testing.T) {
	c := NewClient("http://x", &fakeAuth{})
	body := c.buildRequest(provider.ChatRequest{
		Model: "gpt-5-codex",
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "be helpful"},
			{Role: provider.RoleUser, Content: "hi"},
			{Role: provider.RoleAssistant, Content: "thinking", ToolCalls: []provider.ToolCall{
				{ID: "call_1", Name: "read_file", Arguments: `{"path":"a"}`},
			}},
			{Role: provider.RoleTool, ToolCallID: "call_1", Name: "read_file", Content: "file body"},
		},
		Tools: []provider.ToolDefinition{
			{Name: "read_file", Description: "read a file", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	})

	if body.Instructions != "be helpful" {
		t.Fatalf("instructions = %q", body.Instructions)
	}
	if body.Model != "gpt-5-codex" || !body.Stream || body.Store {
		t.Fatalf("unexpected scalar fields: %+v", body)
	}
	if body.ToolChoice != "auto" || len(body.Tools) != 1 || body.Tools[0].Type != "function" {
		t.Fatalf("tools not translated: %+v", body.Tools)
	}
	// Expect: user message, assistant message, function_call, function_call_output.
	if len(body.Input) != 4 {
		t.Fatalf("want 4 input items, got %d: %+v", len(body.Input), body.Input)
	}
	if body.Input[0].Type != "message" || body.Input[0].Role != "user" || body.Input[0].Content[0].Type != "input_text" {
		t.Fatalf("user item wrong: %+v", body.Input[0])
	}
	if body.Input[1].Type != "message" || body.Input[1].Role != "assistant" || body.Input[1].Content[0].Type != "output_text" {
		t.Fatalf("assistant item wrong: %+v", body.Input[1])
	}
	if body.Input[2].Type != "function_call" || body.Input[2].CallID != "call_1" || body.Input[2].Name != "read_file" {
		t.Fatalf("function_call item wrong: %+v", body.Input[2])
	}
	if body.Input[3].Type != "function_call_output" || body.Input[3].CallID != "call_1" || body.Input[3].Output != "file body" {
		t.Fatalf("function_call_output item wrong: %+v", body.Input[3])
	}
	if body.Reasoning == nil || body.Reasoning.Effort != "medium" {
		t.Fatalf("reasoning param wrong: %+v", body.Reasoning)
	}
}

// collect drains a stream channel into a slice.
func collect(ch <-chan provider.StreamEvent) []provider.StreamEvent {
	var out []provider.StreamEvent
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

const sseStream = `data: {"type":"response.created","response":{"status":"in_progress"}}

data: {"type":"response.output_text.delta","delta":"Hello "}

data: {"type":"response.output_text.delta","delta":"world"}

data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call_1","name":"read_file"}}

data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"path\":"}

data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"\"x\"}"}

data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"x\"}"}}

data: {"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":5,"input_tokens_details":{"cached_tokens":2}}}}

`

func sseServer(t *testing.T, auth *fakeAuth, failFirstWith401 bool) *httptest.Server {
	t.Helper()
	var hits int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if failFirstWith401 && n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"token expired"}}`))
			return
		}
		// Auth header must reflect the (possibly refreshed) token.
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Errorf("missing bearer auth header: %q", got)
		}
		if got := r.Header.Get("chatgpt-account-id"); got != auth.account {
			t.Errorf("account-id header = %q, want %q", got, auth.account)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sseStream)
	}))
}

func TestChatCompletionStream(t *testing.T) {
	auth := &fakeAuth{access: "tok", account: "acct-1"}
	srv := sseServer(t, auth, false)
	defer srv.Close()

	c := NewClient(srv.URL, auth)
	ch, err := c.ChatCompletion(context.Background(), provider.ChatRequest{Model: "gpt-5-codex"})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	events := collect(ch)

	var text strings.Builder
	var args strings.Builder
	var sawStart, sawEnd, sawDone bool
	var usage *provider.Usage
	for _, ev := range events {
		switch ev.Type {
		case provider.EventTextDelta:
			text.WriteString(ev.TextDelta)
		case provider.EventToolCallStart:
			sawStart = true
			if ev.ToolCall.ID != "call_1" || ev.ToolCall.Name != "read_file" {
				t.Fatalf("bad tool start: %+v", ev.ToolCall)
			}
		case provider.EventToolCallDelta:
			args.WriteString(ev.ToolCall.ArgumentsDelta)
		case provider.EventToolCallEnd:
			sawEnd = true
		case provider.EventDone:
			sawDone = true
			usage = ev.Usage
		case provider.EventError:
			t.Fatalf("unexpected error event: %v", ev.Error)
		}
	}

	if text.String() != "Hello world" {
		t.Fatalf("text = %q", text.String())
	}
	if !sawStart || !sawEnd || !sawDone {
		t.Fatalf("missing lifecycle events: start=%v end=%v done=%v", sawStart, sawEnd, sawDone)
	}
	if args.String() != `{"path":"x"}` {
		t.Fatalf("assembled args = %q", args.String())
	}
	if usage == nil || usage.InputTokens != 10 || usage.OutputTokens != 5 || usage.CacheReadInputTokens != 2 {
		t.Fatalf("usage wrong: %+v", usage)
	}
}

func TestChatCompletionRefreshesOn401(t *testing.T) {
	auth := &fakeAuth{access: "stale", account: "acct-1"}
	srv := sseServer(t, auth, true)
	defer srv.Close()

	c := NewClient(srv.URL, auth)
	ch, err := c.ChatCompletion(context.Background(), provider.ChatRequest{Model: "gpt-5-codex"})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	events := collect(ch)
	if atomic.LoadInt32(&auth.refreshes) != 1 {
		t.Fatalf("expected exactly one refresh, got %d", auth.refreshes)
	}
	if len(events) == 0 || events[len(events)-1].Type != provider.EventDone {
		t.Fatalf("expected successful stream after refresh, got %+v", events)
	}
}

// When arguments arrive only in the terminal done frame (no deltas), the parser
// must still deliver the complete arguments to the agent.
func TestToolArgsFromDoneFrameOnly(t *testing.T) {
	stream := `data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"c1","name":"run"}}

data: {"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"c1","name":"run","arguments":"{\"cmd\":\"ls\"}"}}

data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1}}}

`
	auth := &fakeAuth{access: "t", account: "a"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, stream)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, auth)
	ch, err := c.ChatCompletion(context.Background(), provider.ChatRequest{Model: "gpt-5-codex"})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	var args strings.Builder
	for ev := range ch {
		if ev.Type == provider.EventToolCallDelta {
			args.WriteString(ev.ToolCall.ArgumentsDelta)
		}
	}
	if args.String() != `{"cmd":"ls"}` {
		t.Fatalf("args from done frame = %q", args.String())
	}
}

func TestChatCompletionAPIError(t *testing.T) {
	auth := &fakeAuth{access: "t", account: "a"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"model not available"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, auth)
	_, err := c.ChatCompletion(context.Background(), provider.ChatRequest{Model: "bad"})
	if err == nil || !strings.Contains(err.Error(), "model not available") {
		t.Fatalf("expected API error surfaced, got %v", err)
	}
}
