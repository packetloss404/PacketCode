package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		_, _ = w.Write([]byte(`{
			"models":[
				{"name":"qwen2.5-coder:14b","model":"qwen2.5-coder:14b","size":9000000000},
				{"name":"deepseek-coder:6.7b","model":"deepseek-coder:6.7b","size":4000000000}
			]
		}`))
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
	assert.False(t, byID["deepseek-coder:6.7b"].SupportsTools)
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
