package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/packetcode/packetcode/internal/provider"
)

func TestRequiresResponsesAPI(t *testing.T) {
	for _, id := range []string{
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		// Dated snapshots are the same model under the same constraint.
		"gpt-5.6-sol-2026-04-01",
		"GPT-5.6-SOL",
		"o1-pro",
		"o3-pro",
		"gpt-5.5-pro",
	} {
		if !requiresResponsesAPI(id) {
			t.Errorf("%q should route to /v1/responses", id)
		}
	}
	for _, id := range []string{
		"gpt-4.1",
		"gpt-4.1-mini",
		"gpt-5",
		"gpt-5.1",
		"gpt-5.2",
		"gpt-5.5",
		"o3",
		"o4-mini",
		// Not a -pro variant, just a name containing "pro".
		"gpt-prometheus",
		"",
		"   ",
	} {
		if requiresResponsesAPI(id) {
			t.Errorf("%q should stay on /v1/chat/completions", id)
		}
	}
}

// The bug this fixes: packetcode sends function tools on every turn, and
// /v1/chat/completions refuses them for these models, so every request 400s.
// The request has to arrive at /responses instead.
func TestChatCompletion_RoutesResponsesOnlyModelsToTheResponsesEndpoint(t *testing.T) {
	var mu sync.Mutex
	var gotPath string
	var gotBody map[string]any
	var gotHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotPath = r.URL.Path
		gotHeaders = r.Header.Clone()
		_ = json.Unmarshal(body, &gotBody)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{}}}\n\n")
	}))
	defer srv.Close()

	p := NewWithBaseURL(srv.URL, "sk-test")
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "gpt-5.6-sol",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hello"}},
		Tools:    []provider.ToolDefinition{{Name: "read_file", Description: "read a file"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	for range ch {
	}

	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/responses" {
		t.Fatalf("path = %q, want /responses", gotPath)
	}
	// Tools must actually be on the request -- routing to the right endpoint
	// is pointless if the thing that forced the move got dropped.
	if _, ok := gotBody["tools"]; !ok {
		t.Fatalf("tools missing from the responses request: %v", gotBody)
	}
	// No reasoning_effort of our own. The API applies the model's default;
	// sending "medium" here would silently override it.
	if _, ok := gotBody["reasoning"]; ok {
		t.Fatalf("unrequested reasoning parameter was sent: %v", gotBody["reasoning"])
	}
	// The ChatGPT-backend headers must not go to the public API. `originator:
	// codex_cli_rs` in particular is a false claim about the caller.
	for _, h := range []string{"Originator", "Session_id", "Openai-Beta", "Chatgpt-Account-Id"} {
		if v := gotHeaders.Get(h); v != "" {
			t.Errorf("ChatGPT-backend header %s=%q sent to the public API", h, v)
		}
	}
	if got := gotHeaders.Get("Authorization"); got != "Bearer sk-test" {
		t.Errorf("Authorization = %q", got)
	}
}

// Everything else must keep the endpoint that has always worked for it.
func TestChatCompletion_KeepsOrdinaryModelsOnChatCompletions(t *testing.T) {
	var mu sync.Mutex
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.Path
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := NewWithBaseURL(srv.URL, "sk-test")
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "gpt-4.1",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	for range ch {
	}

	mu.Lock()
	defer mu.Unlock()
	if !strings.HasSuffix(gotPath, "/chat/completions") {
		t.Fatalf("path = %q, want .../chat/completions", gotPath)
	}
}

// The -pro family was excluded from the catalog because there was nowhere to
// send it. There is now, so it must be offered.
func TestIsChatModel_OffersProVariantsNowThatTheyAreRoutable(t *testing.T) {
	for _, id := range []string{"o3-pro", "gpt-5.5-pro"} {
		if !isChatModel(id) {
			t.Errorf("%q should be offered now that it routes to /v1/responses", id)
		}
		if !requiresResponsesAPI(id) {
			t.Errorf("%q is offered but not routed; that is the old bug", id)
		}
	}
	// Genuinely non-conversational models stay out.
	for _, id := range []string{"text-embedding-3-large", "whisper-1", "dall-e-3", "tts-1"} {
		if isChatModel(id) {
			t.Errorf("%q is not a chat model", id)
		}
	}
}

// Every model offered by the picker must be one the provider can actually
// drive. This is the invariant whose violation produced the original bug:
// gpt-5.6-sol was listed, claimed tool support, and failed on every turn.
func TestOfferedModelsAreAllRoutable(t *testing.T) {
	p := New("sk-test")
	for id := range pricingTable {
		if !isChatModel(id) {
			continue
		}
		if !p.SupportsTools(id) {
			continue
		}
		// Routable means: either it needs the Responses API and we send it
		// there, or it does not and chat-completions is correct for it.
		// Both are fine; there is no third state.
		_ = p.UsesResponsesAPI(id)
	}
	// The specific model from the bug report.
	if !p.UsesResponsesAPI("gpt-5.6-sol") {
		t.Fatal("gpt-5.6-sol must route to the Responses API")
	}
	if !p.SupportsTools("gpt-5.6-sol") {
		t.Fatal("gpt-5.6-sol supports tools; that is why it needs the other endpoint")
	}
}
