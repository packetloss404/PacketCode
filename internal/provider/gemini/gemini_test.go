package gemini

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
	p := New("k")
	assert.Equal(t, "gemini", p.Slug())
	assert.Equal(t, "Google Gemini", p.Name())
}

func TestProvider_PricingAndContext(t *testing.T) {
	p := New("")
	in, out := p.Pricing("gemini-2.5-pro")
	assert.Equal(t, 1.25, in)
	assert.Equal(t, 10.00, out)
	assert.Equal(t, 2_000_000, p.ContextWindow("gemini-2.5-pro"))
	assert.True(t, p.SupportsTools("gemini-2.5-flash"))
	assert.False(t, p.SupportsTools("gemini-1.5-pro"))
}

func TestProvider_ListModels_FiltersAndStripsPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "key=k")
		_, _ = w.Write([]byte(`{
			"models":[
				{"name":"models/gemini-2.5-pro","displayName":"Gemini 2.5 Pro","inputTokenLimit":2000000,"supportedGenerationMethods":["generateContent","streamGenerateContent"]},
				{"name":"models/gemini-2.5-flash","displayName":"Gemini 2.5 Flash","inputTokenLimit":1000000,"supportedGenerationMethods":["generateContent"]},
				{"name":"models/embedding-001","displayName":"Embeddings","supportedGenerationMethods":["embedContent"]}
			]
		}`))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "k")
	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	ids := make([]string, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	assert.ElementsMatch(t, []string{"gemini-2.5-pro", "gemini-2.5-flash"}, ids)
}

func TestProvider_ValidateKey(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "")
	require.NoError(t, p.ValidateKey(context.Background(), "k"))
	assert.Equal(t, 1, hits)

	require.Error(t, p.ValidateKey(context.Background(), ""))
}

func TestToWireRequest_RoleMapping(t *testing.T) {
	wr, err := toWireRequest(provider.ChatRequest{
		Model: "gemini-2.5-pro",
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "you are helpful"},
			{Role: provider.RoleUser, Content: "read main.go"},
			{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{
				{ID: "call_0", Name: "read_file", Arguments: `{"path":"main.go"}`},
			}},
			{Role: provider.RoleTool, Name: "read_file", ToolCallID: "call_0", Content: "package main\n"},
		},
		Tools: []provider.ToolDefinition{
			{Name: "read_file", Description: "read a file", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	})
	require.NoError(t, err)

	require.NotNil(t, wr.SystemInstruction)
	assert.Equal(t, "you are helpful", wr.SystemInstruction.Parts[0].Text)

	require.Len(t, wr.Contents, 3)
	assert.Equal(t, "user", wr.Contents[0].Role)
	assert.Equal(t, "read main.go", wr.Contents[0].Parts[0].Text)

	assert.Equal(t, "model", wr.Contents[1].Role)
	require.NotNil(t, wr.Contents[1].Parts[0].FunctionCall)
	assert.Equal(t, "read_file", wr.Contents[1].Parts[0].FunctionCall.Name)

	assert.Equal(t, "user", wr.Contents[2].Role)
	require.NotNil(t, wr.Contents[2].Parts[0].FunctionResponse)
	assert.Equal(t, "read_file", wr.Contents[2].Parts[0].FunctionResponse.Name)
	// Plain-text tool result was wrapped in {"output":...}
	assert.Contains(t, string(wr.Contents[2].Parts[0].FunctionResponse.Response), `"output"`)

	require.Len(t, wr.Tools, 1)
	assert.Equal(t, "read_file", wr.Tools[0].FunctionDeclarations[0].Name)
}

func TestToWireRequest_ToolResponseAlreadyJSON(t *testing.T) {
	wr, err := toWireRequest(provider.ChatRequest{
		Model: "gemini-2.5-pro",
		Messages: []provider.Message{
			{Role: provider.RoleTool, Name: "list_dir", Content: `{"files":["a","b"]}`},
		},
	})
	require.NoError(t, err)
	require.Len(t, wr.Contents, 1)
	resp := wr.Contents[0].Parts[0].FunctionResponse
	require.NotNil(t, resp)
	assert.JSONEq(t, `{"files":["a","b"]}`, string(resp.Response))
}

func TestProvider_ChatCompletion_StreamsTextAndUsage(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]}}]}`,
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":" world"}]}}]}`,
		`data: {"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":2}}`,
		``,
	}, "\n\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models/gemini-2.5-pro:streamGenerateContent", r.URL.Path)
		assert.Contains(t, r.URL.RawQuery, "alt=sse")
		assert.Contains(t, r.URL.RawQuery, "key=k")

		body, _ := io.ReadAll(r.Body)
		var parsed wireRequest
		require.NoError(t, json.Unmarshal(body, &parsed))
		require.Len(t, parsed.Contents, 1)
		assert.Equal(t, "user", parsed.Contents[0].Role)

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "k")
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "gemini-2.5-pro",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
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
		case provider.EventError:
			t.Fatalf("error: %v", ev.Error)
		}
	}
	assert.Equal(t, "Hello world", got.String())
	assert.True(t, done)
	require.NotNil(t, usage)
	assert.Equal(t, 7, usage.InputTokens)
	assert.Equal(t, 2, usage.OutputTokens)
}

func TestProvider_ChatCompletion_StreamsToolCall(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{"path":"main.go"}}}]}}]}`,
		`data: {"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":4}}`,
		``,
	}, "\n\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "k")
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "gemini-2.5-pro",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "read main.go"}},
	})
	require.NoError(t, err)

	var starts, ends int
	var args, name string
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

func TestProvider_ChatCompletion_SuppressesTextOnToolCallParts(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"<|python_tag|>{\"path\":\"main.go\"}"},{"functionCall":{"name":"read_file","args":{"path":"main.go"}}}]}}]}`,
		`data: {"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}]}`,
		``,
	}, "\n\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "k")
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "gemini-2.5-pro",
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

func TestProvider_ChatCompletion_MalformedFunctionCallFinishErrors(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"MALFORMED_FUNCTION_CALL"}]}`,
		``,
	}, "\n\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "k")
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "gemini-2.5-pro",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "read main.go"}},
	})
	require.NoError(t, err)

	var gotErr error
	for ev := range ch {
		if ev.Type == provider.EventError {
			gotErr = ev.Error
		}
	}
	require.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), "MALFORMED_FUNCTION_CALL")
}

func TestProvider_ChatCompletion_FunctionCallArgsMustBeObject(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"read_file","args":["main.go"]}}]}}]}`,
		``,
	}, "\n\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "k")
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "gemini-2.5-pro",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "read main.go"}},
	})
	require.NoError(t, err)

	var gotErr error
	for ev := range ch {
		if ev.Type == provider.EventError {
			gotErr = ev.Error
		}
	}
	require.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), "args must be a JSON object")
}

// TestGemini_ChatCompletion_CancellationStopsStream verifies the
// per-iteration ctx.Err() guard in parseGeminiSSE: cancelling the ctx
// passed to ChatCompletion closes the stream channel within 1s and
// surfaces an EventError whose cause is (or wraps) context.Canceled.
func TestGemini_ChatCompletion_CancellationStopsStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
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
				"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"chunk %d \"}]}}]}\n\n",
				i); err != nil {
				return
			}
			flusher.Flush()
		}
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "k")
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := p.ChatCompletion(ctx, provider.ChatRequest{
		Model:    "gemini-2.5-pro",
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

func TestProvider_ChatCompletion_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad model"}}`))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "k")
	_, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model: "gemini-bogus",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}
