package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_Identity(t *testing.T) {
	p := New("k")
	assert.Equal(t, "openrouter", p.Slug())
	assert.Equal(t, "OpenRouter", p.Name())
}

func TestProvider_RequiredHeadersOnEveryRequest(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		assert.Equal(t, referer, r.Header.Get("HTTP-Referer"))
		assert.Equal(t, title, r.Header.Get("X-Title"))
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "k")
	require.NoError(t, p.ValidateKey(context.Background(), "k"))
	_, err := p.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, hits)
}

func TestProvider_ListModels_DynamicPricingAndToolDetection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"data":[
				{
					"id":"anthropic/claude-sonnet-4",
					"name":"Claude Sonnet 4",
					"context_length":200000,
					"pricing":{"prompt":"0.000003","completion":"0.000015"},
					"supported_parameters":["tools","temperature"]
				},
				{
					"id":"meta-llama/llama-3.3-70b",
					"name":"Llama 3.3 70B",
					"context_length":131072,
					"pricing":{"prompt":"0.00000023","completion":"0.0000004"},
					"supported_parameters":["temperature"]
				}
			]
		}`))
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "k")
	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)

	byID := map[string]int{}
	for i, m := range models {
		byID[m.ID] = i
	}

	claude := models[byID["anthropic/claude-sonnet-4"]]
	assert.Equal(t, 200_000, claude.ContextWindow)
	assert.Equal(t, 3.00, claude.InputPer1M)
	assert.Equal(t, 15.00, claude.OutputPer1M)
	assert.True(t, claude.SupportsTools)

	llama := models[byID["meta-llama/llama-3.3-70b"]]
	assert.InDelta(t, 0.23, llama.InputPer1M, 0.001)
	assert.InDelta(t, 0.40, llama.OutputPer1M, 0.001)
	assert.False(t, llama.SupportsTools)

	// Cached metadata is queryable via the provider methods.
	in, out := p.Pricing("anthropic/claude-sonnet-4")
	assert.Equal(t, 3.00, in)
	assert.Equal(t, 15.00, out)
	assert.Equal(t, 200_000, p.ContextWindow("anthropic/claude-sonnet-4"))
	assert.True(t, p.SupportsTools("anthropic/claude-sonnet-4"))
}

func TestProvider_MetadataBeforeListModelsUsesSafeFallback(t *testing.T) {
	p := New("")
	in, out := p.Pricing("anything")
	assert.Equal(t, 3.0, in)
	assert.Equal(t, 15.0, out)
	assert.Equal(t, 128_000, p.ContextWindow("anything"))
	assert.True(t, p.SupportsTools("anything"))
}

func TestPer1MFromPerToken(t *testing.T) {
	assert.Equal(t, 0.0, per1MFromPerToken(""))
	assert.Equal(t, 0.0, per1MFromPerToken("not-a-number"))
	assert.InDelta(t, 3.0, per1MFromPerToken("0.000003"), 1e-9)
}
