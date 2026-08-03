package openaicompat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/packetcode/packetcode/internal/provider"
)

// A tag is not guaranteed to arrive in one SSE frame. The filter must hold back
// any suffix that could still grow into "<think>" or "</think>" and resolve it
// once the next chunk lands, rather than leaking "<thi" into the transcript.
func TestThinkFilterSplitsTagsAcrossChunks(t *testing.T) {
	f := &thinkFilter{}

	var visible, reasoning strings.Builder
	for _, chunk := range []string{"before <th", "ink>plan", "ning</thi", "nk> after"} {
		v, r := f.Write(chunk)
		visible.WriteString(v)
		reasoning.WriteString(r)
	}
	v, r := f.Flush()
	visible.WriteString(v)
	reasoning.WriteString(r)

	if got, want := visible.String(), "before  after"; got != want {
		t.Errorf("visible = %q, want %q", got, want)
	}
	if got, want := reasoning.String(), "planning"; got != want {
		t.Errorf("reasoning = %q, want %q", got, want)
	}
}

// A stream that ends mid-reasoning must not silently swallow the tail.
func TestThinkFilterFlushesUnterminatedBlock(t *testing.T) {
	f := &thinkFilter{}
	if v, r := f.Write("<think>cut off"); v != "" || r != "cut off" {
		t.Fatalf("Write = (%q, %q), want (\"\", \"cut off\")", v, r)
	}
	// A dangling partial tag is still buffered; Flush must release it.
	if v, r := f.Write(" more</thi"); v != "" || r != " more" {
		t.Fatalf("Write = (%q, %q), want (\"\", \" more\")", v, r)
	}
	v, r := f.Flush()
	if v != "" || r != "</thi" {
		t.Fatalf("Flush = (%q, %q), want (\"\", \"</thi\")", v, r)
	}
}

// MiniMax M3 reasons between tool calls, and the frame that closes a <think>
// block routinely carries the tool call too. The reasoning must reach the
// reasoning stream, the visible remainder must survive, and the tool call must
// still be assembled.
func TestChatCompletionSplitsReasoningFromToolCallFrame(t *testing.T) {
	frames := []string{
		`{"choices":[{"delta":{"content":"<think>I should list the files"}}]}`,
		`{"choices":[{"delta":{"content":" first</think>On it.","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"list_directory","arguments":"{}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}
	srv := newSSEServer(t, frames)
	defer srv.Close()

	client := &Client{BaseURL: srv.URL, APIKey: "test", InterleavedThinking: true}
	ev, err := client.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "MiniMax-M3",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	var text, reasoning, toolName string
	for e := range ev {
		switch e.Type {
		case provider.EventTextDelta:
			text += e.TextDelta
		case provider.EventReasoningDelta:
			reasoning += e.TextDelta
		case provider.EventToolCallStart:
			toolName = e.ToolCall.Name
		case provider.EventError:
			t.Fatalf("unexpected stream error: %v", e.Error)
		}
	}

	if want := "I should list the files first"; reasoning != want {
		t.Errorf("reasoning = %q, want %q", reasoning, want)
	}
	if want := "On it."; text != want {
		t.Errorf("text = %q, want %q — content sharing a frame with a tool call must not be dropped", text, want)
	}
	if want := "list_directory"; toolName != want {
		t.Errorf("tool = %q, want %q", toolName, want)
	}
}

// Providers that do not use the <think> convention must be untouched: a literal
// "<think>" in ordinary prose stays in the visible transcript.
func TestChatCompletionLeavesThinkTagsAloneByDefault(t *testing.T) {
	frames := []string{
		`{"choices":[{"delta":{"content":"Use <think>tags</think> like this"}}]}`,
	}
	srv := newSSEServer(t, frames)
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
	for e := range ev {
		if e.Type == provider.EventTextDelta {
			text += e.TextDelta
		}
		if e.Type == provider.EventReasoningDelta {
			t.Fatal("a non-interleaved provider must not emit reasoning deltas")
		}
	}
	if want := "Use <think>tags</think> like this"; text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
}

// Out-of-band reasoning_content is surfaced for display on backends that use it
// instead of inline <think> blocks.
func TestChatCompletionSurfacesReasoningContentField(t *testing.T) {
	frames := []string{
		`{"choices":[{"delta":{"reasoning_content":"weighing options"}}]}`,
		`{"choices":[{"delta":{"content":"Done."}}]}`,
	}
	srv := newSSEServer(t, frames)
	defer srv.Close()

	client := &Client{BaseURL: srv.URL, APIKey: "test"}
	ev, err := client.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "MiniMax-M3",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	var text, reasoning string
	for e := range ev {
		switch e.Type {
		case provider.EventTextDelta:
			text += e.TextDelta
		case provider.EventReasoningDelta:
			reasoning += e.TextDelta
		}
	}
	if want := "weighing options"; reasoning != want {
		t.Errorf("reasoning = %q, want %q", reasoning, want)
	}
	if want := "Done."; text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
}

// The reasoning chain has to go back to the model in the shape it arrived in.
// MiniMax's tool-use guide requires the complete response — thinking included —
// to be replayed, with the <think> tags preserved exactly.
func TestToWireMessagesEchoesReasoningWhenEnabled(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleUser, Content: "list the files"},
		{
			Role:      provider.RoleAssistant,
			Reasoning: "I should call list_directory",
			ToolCalls: []provider.ToolCall{{ID: "call_1", Name: "list_directory", Arguments: "{}"}},
		},
		{Role: provider.RoleTool, ToolCallID: "call_1", Content: "a.go"},
	}

	got := toWireMessages(msgs, true)
	want := "<think>I should call list_directory</think>"
	if got[1].Content != want {
		t.Errorf("assistant content = %q, want %q", got[1].Content, want)
	}
	if len(got[1].ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(got[1].ToolCalls))
	}

	// Without the flag the reasoning must never leave the session: a provider
	// switch would otherwise ship MiniMax's chain to a backend that rejects it.
	off := toWireMessages(msgs, false)
	if off[1].Content != "" {
		t.Errorf("assistant content = %q, want empty when SendReasoning is off", off[1].Content)
	}
}

// Reasoning is prepended to visible text rather than replacing it, so a turn
// that both thinks and speaks replays in the original order.
func TestToWireMessagesPrependsReasoningToContent(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleAssistant, Reasoning: "short thought", Content: "Here you go."},
	}
	got := toWireMessages(msgs, true)
	if want := "<think>short thought</think>Here you go."; got[0].Content != want {
		t.Errorf("content = %q, want %q", got[0].Content, want)
	}
}

// newSSEServer serves the given JSON frames as an SSE stream, then [DONE].
func newSSEServer(t *testing.T, frames []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		for _, f := range frames {
			_, _ = w.Write([]byte("data: " + f + "\n\n"))
			if fl != nil {
				fl.Flush()
			}
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
}
