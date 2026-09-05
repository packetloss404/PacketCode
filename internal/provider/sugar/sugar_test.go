package sugar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/provider"
)

func TestListModelsUsesLiveSugarCatalogAndPrioritizesConduit(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sgr_test", r.Header.Get("Authorization"))
		assert.Equal(t, "/api/v1/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[{"id":"sugar/glm","object":"model","owned_by":"sugar"},{"id":"sugar/conduit","object":"model","owned_by":"sugar"},{"id":"sugar/future","object":"model","owned_by":"sugar"}]}`)
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "sgr_test")
	models, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 3)
	assert.Equal(t, "sugar/conduit", models[0].ID)
	assert.Equal(t, "Conduit (automatic)", models[0].DisplayName)
	assert.Equal(t, "sugar/glm", models[1].ID)
	assert.Equal(t, "GLM", models[1].DisplayName)
	assert.Equal(t, "sugar/future", models[2].ID)
	assert.True(t, models[2].SupportsTools)
}

func TestChatCompletionStreamsThroughSugar(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":1,\"total_tokens\":5}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "sgr_test")
	events, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "sugar/conduit",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
		Stream:   true,
	})
	require.NoError(t, err)
	var text string
	var done bool
	for event := range events {
		if event.Type == provider.EventTextDelta {
			text += event.TextDelta
		}
		if event.Type == provider.EventDone {
			done = true
		}
		if event.Type == provider.EventError {
			require.NoError(t, event.Error)
		}
	}
	assert.Equal(t, "hello", text)
	assert.True(t, done)
}

func TestChatCompletionSerializesValidatedSugarCacheOnly(t *testing.T) {
	t.Parallel()
	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		requestBody <- body
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "sgr_test")
	fingerprint := provider.CachePrefixFingerprint("stable", nil)
	events, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "sugar/conduit",
		Messages: []provider.Message{{Role: provider.RoleSystem, Content: "stable"}, {Role: provider.RoleUser, Content: "hi"}},
		SugarCache: &provider.SugarCacheMetadata{
			ConversationID:       "8f6147a8-80ba-4730-8bba-c886df706fa5",
			PrefixFingerprint:    fingerprint,
			StablePrefixMessages: 1,
			CompactionGeneration: 2,
			Mode:                 provider.SugarCacheAuto,
			Retention:            provider.SugarCacheProviderDefault,
			Privacy:              provider.SugarPrivacyStandard,
		},
	})
	require.NoError(t, err)
	for range events {
	}

	body := <-requestBody
	cache, ok := body["sugar_cache"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "8f6147a8-80ba-4730-8bba-c886df706fa5", cache["conversation_id"])
	assert.Equal(t, fingerprint, cache["prefix_fingerprint"])
	assert.Equal(t, float64(1), cache["stable_prefix_messages"])
	assert.Equal(t, float64(2), cache["compaction_generation"])
	assert.Equal(t, "auto", cache["mode"])
	assert.Equal(t, "provider_default", cache["retention"])
	assert.Equal(t, "standard", cache["privacy"])
}

func TestChatCompletionRejectsInvalidSugarCacheBeforeNetwork(t *testing.T) {
	t.Parallel()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewWithBaseURL(server.URL, "sgr_test")
	_, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "sugar/conduit",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
		SugarCache: &provider.SugarCacheMetadata{
			ConversationID:    "contains spaces",
			PrefixFingerprint: provider.CachePrefixFingerprint("", nil),
		},
	})
	require.Error(t, err)
	assert.Zero(t, requests)
}

func TestNormalizeBaseURL(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "https://sugar.example/api/v1", NormalizeBaseURL("https://sugar.example"))
	assert.Equal(t, "https://sugar.example/api/v1", NormalizeBaseURL("https://sugar.example/api/v1/"))
	assert.Equal(t, "https://sugar.example/api/v1", NormalizeBaseURL("https://sugar.example/v1"))
}
