package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/provider"
)

func TestProvider_Identity(t *testing.T) {
	p := New("sk-ant-test")
	assert.Equal(t, "anthropic", p.Slug())
	assert.Equal(t, "Anthropic", p.Name())
	assert.NotEmpty(t, string(p.BrandColor()))
}

func TestProvider_PricingContextAndTools(t *testing.T) {
	p := New("")
	in, out := p.Pricing(DefaultModel)
	assert.Equal(t, 5.00, in)
	assert.Equal(t, 25.00, out)
	assert.Equal(t, 1_000_000, p.ContextWindow(DefaultModel))
	assert.True(t, p.SupportsTools(DefaultModel))

	in, out = p.Pricing("claude-new")
	assert.Equal(t, 5.00, in)
	assert.Equal(t, 25.00, out)
	assert.Equal(t, 200_000, p.ContextWindow("claude-new"))
	assert.True(t, p.SupportsTools("claude-new"))
}

func TestProvider_ValidateKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/models", r.URL.Path)
		assert.Equal(t, "sk-ant-good", r.Header.Get("x-api-key"))
		assert.Equal(t, anthropicVersion, r.Header.Get("anthropic-version"))
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "")
	require.NoError(t, p.ValidateKey(context.Background(), "sk-ant-good"))
	require.Error(t, p.ValidateKey(context.Background(), ""))
}

func TestProvider_ListModels_MetadataAndDefaultFirst(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": [
				{"id":"claude-sonnet-4-6","display_name":"Claude Sonnet 4.6","max_input_tokens":1000000,"max_tokens":64000,"capabilities":{"tools":true}},
				{"id":"claude-opus-4-8","display_name":"Claude Opus 4.8","max_input_tokens":1000000,"max_tokens":128000,"capabilities":{"tools":true}}
			]
		}`))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "sk-ant-test")
	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.Equal(t, DefaultModel, models[0].ID)
	assert.Equal(t, "Claude Opus 4.8", models[0].DisplayName)
	assert.Equal(t, 1_000_000, models[0].ContextWindow)
	assert.True(t, models[0].SupportsTools)
	assert.Equal(t, 5.00, models[0].InputPer1M)
	assert.Equal(t, 25.00, models[0].OutputPer1M)
}

func TestToWireRequest_RoleAndToolMapping(t *testing.T) {
	wr, err := toWireRequest(provider.ChatRequest{
		Model: DefaultModel,
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "you are helpful"},
			{Role: provider.RoleUser, Content: "read main.go"},
			{Role: provider.RoleAssistant, Content: "I will look.", ToolCalls: []provider.ToolCall{
				{ID: "toolu_1", Name: "read_file", Arguments: `{"path":"main.go"}`},
			}},
			{Role: provider.RoleTool, ToolCallID: "toolu_1", Name: "read_file", Content: "package main\n"},
		},
		Tools: []provider.ToolDefinition{
			{Name: "read_file", Description: "read a file", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, DefaultModel, wr.Model)
	require.Len(t, wr.System, 1)
	assert.Equal(t, "you are helpful", wr.System[0].Text)
	require.NotNil(t, wr.System[0].CacheControl)
	assert.Equal(t, "ephemeral", wr.System[0].CacheControl.Type)
	assert.True(t, wr.Stream)
	assert.Equal(t, defaultMaxTokens, wr.MaxTokens)
	require.NotNil(t, wr.CacheControl)
	assert.Equal(t, "ephemeral", wr.CacheControl.Type)
	require.Len(t, wr.Messages, 3)
	assert.Equal(t, "user", wr.Messages[0].Role)
	assert.JSONEq(t, `[{"type":"text","text":"read main.go"}]`, string(wr.Messages[0].Content))
	assert.Equal(t, "assistant", wr.Messages[1].Role)
	assert.JSONEq(t, `[{"type":"text","text":"I will look."},{"type":"tool_use","id":"toolu_1","name":"read_file","input":{"path":"main.go"}}]`, string(wr.Messages[1].Content))
	assert.Equal(t, "user", wr.Messages[2].Role)
	assert.JSONEq(t, `[{"type":"tool_result","tool_use_id":"toolu_1","content":"package main\n"}]`, string(wr.Messages[2].Content))
	require.Len(t, wr.Tools, 1)
	assert.Equal(t, "read_file", wr.Tools[0].Name)
	assert.JSONEq(t, `{"type":"object"}`, string(wr.Tools[0].InputSchema))
	require.NotNil(t, wr.Tools[0].CacheControl)
	assert.Equal(t, "ephemeral", wr.Tools[0].CacheControl.Type)
}

func TestToProviderUsageIncludesCachedInputInTotal(t *testing.T) {
	got := toProviderUsage(&anthropicUsage{
		InputTokens:              25,
		OutputTokens:             4,
		CacheCreationInputTokens: 100,
		CacheReadInputTokens:     900,
	})
	require.NotNil(t, got)
	assert.Equal(t, 1025, got.InputTokens)
	assert.Equal(t, 100, got.CacheCreationInputTokens)
	assert.Equal(t, 900, got.CacheReadInputTokens)
}

func TestProvider_ChatCompletion_StreamsTextAndUsage(t *testing.T) {
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":25,"output_tokens":1,"cache_creation_input_tokens":100,"cache_read_input_tokens":900}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","usage":{"output_tokens":4}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	var captured wireRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &captured))
		assert.Equal(t, "/messages", r.URL.Path)
		assert.Equal(t, "sk-ant-test", r.Header.Get("x-api-key"))
		assert.Equal(t, anthropicVersion, r.Header.Get("anthropic-version"))
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "sk-ant-test")
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    DefaultModel,
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	require.NoError(t, err)

	var text strings.Builder
	var done bool
	var usage *provider.Usage
	for ev := range ch {
		switch ev.Type {
		case provider.EventTextDelta:
			text.WriteString(ev.TextDelta)
		case provider.EventDone:
			done = true
			usage = ev.Usage
		case provider.EventError:
			t.Fatalf("unexpected error: %v", ev.Error)
		}
	}
	assert.Equal(t, "Hello world", text.String())
	assert.True(t, done)
	require.NotNil(t, usage)
	assert.Equal(t, 1025, usage.InputTokens)
	assert.Equal(t, 4, usage.OutputTokens)
	assert.Equal(t, 100, usage.CacheCreationInputTokens)
	assert.Equal(t, 900, usage.CacheReadInputTokens)
	assert.Equal(t, DefaultModel, captured.Model)
}

func TestProvider_ChatCompletion_StreamsToolCall(t *testing.T) {
	stream := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"main.go\"}"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":1}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "sk-ant-test")
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    DefaultModel,
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "read main.go"}},
	})
	require.NoError(t, err)

	var starts, ends int
	var id, name string
	var args strings.Builder
	for ev := range ch {
		switch ev.Type {
		case provider.EventToolCallStart:
			starts++
			id = ev.ToolCall.ID
			name = ev.ToolCall.Name
		case provider.EventToolCallDelta:
			args.WriteString(ev.ToolCall.ArgumentsDelta)
		case provider.EventToolCallEnd:
			ends++
		case provider.EventError:
			t.Fatalf("unexpected error: %v", ev.Error)
		}
	}
	assert.Equal(t, 1, starts)
	assert.Equal(t, 1, ends)
	assert.Equal(t, "toolu_1", id)
	assert.Equal(t, "read_file", name)
	assert.JSONEq(t, `{"path":"main.go"}`, args.String())
}

func TestProvider_ChatCompletion_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad model"}}`))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "sk-ant-test")
	_, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "bogus",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
	assert.Contains(t, err.Error(), "bad model")
}

// Anthropic's wire input_tokens uniquely EXCLUDES cached input, so the parser
// sums it back in to satisfy provider.Usage's contract. A message_delta that
// carries usage used to overwrite that sum with the raw value, silently
// breaking the subset invariant every downstream consumer assumes — cost
// under-reported, and the statusline's three context fields no longer summing
// to the total. The existing streaming test used a delta carrying only
// output_tokens, which is not the shape the live API sends when caching.
func TestProvider_MessageDeltaKeepsCacheInsideInputTokens(t *testing.T) {
	const (
		freshInput = 25
		creation   = 100
		read       = 900
	)
	body := "event: message_start\n" +
		`data: {"type":"message_start","message":{"usage":{"input_tokens":25,"output_tokens":1,` +
		`"cache_creation_input_tokens":100,"cache_read_input_tokens":900}}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","usage":{"input_tokens":25,"output_tokens":4,` +
		`"cache_creation_input_tokens":100,"cache_read_input_tokens":900}}` + "\n\n" +
		"event: message_stop\ndata: " + `{"type":"message_stop"}` + "\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := New("test-key")
	p.baseURL = srv.URL
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "claude-test",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	var got *provider.Usage
	for ev := range ch {
		if ev.Type == provider.EventDone && ev.Usage != nil {
			got = ev.Usage
		}
	}
	if got == nil {
		t.Fatal("no usage reported")
	}
	if want := freshInput + creation + read; got.InputTokens != want {
		t.Fatalf("InputTokens = %d, want %d — cached input must be inside the total",
			got.InputTokens, want)
	}
	if got.CacheCreationInputTokens+got.CacheReadInputTokens > got.InputTokens {
		t.Fatalf("cache (%d+%d) exceeds InputTokens (%d): cached figures are a subset, never an addend",
			got.CacheCreationInputTokens, got.CacheReadInputTokens, got.InputTokens)
	}
	if got.OutputTokens != 4 {
		t.Fatalf("OutputTokens = %d, want 4 (the delta's value)", got.OutputTokens)
	}
}

// A reply cut off by max_tokens is not a finished reply. Before stop_reason
// was read, a tool call truncated mid-JSON surfaced as "arguments are invalid
// JSON" and truncated prose was persisted as complete.
func TestProvider_ChatCompletion_MaxTokensStopIsAnError(t *testing.T) {
	stream := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"write_file","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"big.go\",\"content\":\"package"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"max_tokens","stop_sequence":null},"usage":{"output_tokens":8192}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "sk-ant-test")
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    DefaultModel,
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "write a big file"}},
	})
	require.NoError(t, err)

	var gotErr error
	var done bool
	for ev := range ch {
		switch ev.Type {
		case provider.EventError:
			gotErr = ev.Error
		case provider.EventDone:
			done = true
		}
	}
	require.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), "max_tokens")
	assert.Contains(t, gotErr.Error(), "tool call")
	assert.False(t, done, "a truncated reply must not also be reported as done")
}

// end_turn is the normal case and must stay silent.
func TestStopReasonError_EndTurnIsNotAnError(t *testing.T) {
	assert.NoError(t, stopReasonError("end_turn", false))
	assert.NoError(t, stopReasonError("tool_use", true))
	assert.NoError(t, stopReasonError("", false))
	assert.Error(t, stopReasonError("refusal", false))
}
