package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/git"
	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/session"
)

type runWireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type runWireRequest struct {
	Model    string           `json:"model"`
	Messages []runWireMessage `json:"messages"`
}

// TestExecuteRunWithRuntime_EndToEndAndResume is deliberately below the
// command's executeRunCommand seam. It exercises config loading, the shared
// runtime builder, the real agent stream, session persistence, runtime close,
// and resume against a credential-free OpenAI-compatible SSE endpoint.
func TestExecuteRunWithRuntime_EndToEndAndResume(t *testing.T) {
	var (
		requestsMu sync.Mutex
		requests   []runWireRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request runWireRequest
		decodeErr := json.NewDecoder(r.Body).Decode(&request)
		requestsMu.Lock()
		requestNumber := len(requests) + 1
		if decodeErr == nil {
			requests = append(requests, request)
		}
		requestsMu.Unlock()
		if decodeErr != nil {
			http.Error(w, decodeErr.Error(), http.StatusBadRequest)
			return
		}

		answer := "first answer"
		input, output, cached := 11, 3, 5
		if requestNumber == 2 {
			answer = "second answer"
			input, output, cached = 20, 4, 10
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%q}}]}\n\n", answer)
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprintf(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":%d,\"completion_tokens\":%d,\"total_tokens\":%d,\"prompt_tokens_details\":{\"cached_tokens\":%d}}}\n\n", input, output, input+output, cached)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	packetHome := t.TempDir()
	t.Setenv(config.HomeEnv, packetHome)
	keyless, supportsTools := false, false
	cfg := config.Default()
	cfg.Default.Provider = "headless-test"
	cfg.Default.Model = "test-model"
	cfg.Behavior.ProviderMaxRetries = 1
	cfg.Providers["headless-test"] = config.ProviderConfig{
		Type:           "openai_compatible",
		BaseURL:        server.URL + "/v1",
		APIKeyRequired: &keyless,
		DefaultModel:   "test-model",
		Models: []config.ProviderModelConfig{{
			ID:            "test-model",
			SupportsTools: &supportsTools,
		}},
	}
	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatal(err)
	}

	first, err := executeRunWithRuntime(t.Context(), runCommandOptions{
		PermissionMode: "read-only",
		Prompt:         "first prompt",
	}, io.Discard)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Output != "first answer" || first.SessionID == "" {
		t.Fatalf("first result = %+v", first)
	}
	if first.Provider != "headless-test" || first.Model != "test-model" {
		t.Fatalf("first selected %s/%s", first.Provider, first.Model)
	}
	if first.Usage != (runUsage{InputTokens: 11, OutputTokens: 3, CacheReadInputTokens: 5}) {
		t.Fatalf("first usage = %+v", first.Usage)
	}

	sessionsDir, err := config.SessionsDir()
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := session.NewManager(sessionsDir).Load(first.SessionID)
	if err != nil {
		t.Fatalf("load first session: %v", err)
	}
	assertPersistedRunMessages(t, persisted.Messages, []provider.Message{
		{Role: provider.RoleUser, Content: "first prompt"},
		{Role: provider.RoleAssistant, Content: "first answer"},
	})

	second, err := executeRunWithRuntime(t.Context(), runCommandOptions{
		PermissionMode: "read-only",
		ResumeID:       first.SessionID,
		Prompt:         "second prompt",
	}, io.Discard)
	if err != nil {
		t.Fatalf("resumed run: %v", err)
	}
	if second.SessionID != first.SessionID || second.Output != "second answer" {
		t.Fatalf("resumed result = %+v", second)
	}
	if second.Provider != "headless-test" || second.Model != "test-model" {
		t.Fatalf("resumed selected %s/%s", second.Provider, second.Model)
	}
	if second.Usage != (runUsage{InputTokens: 20, OutputTokens: 4, CacheReadInputTokens: 10}) {
		t.Fatalf("resumed per-run usage = %+v", second.Usage)
	}

	persisted, err = session.NewManager(sessionsDir).Load(first.SessionID)
	if err != nil {
		t.Fatalf("load resumed session: %v", err)
	}
	assertPersistedRunMessages(t, persisted.Messages, []provider.Message{
		{Role: provider.RoleUser, Content: "first prompt"},
		{Role: provider.RoleAssistant, Content: "first answer"},
		{Role: provider.RoleUser, Content: "second prompt"},
		{Role: provider.RoleAssistant, Content: "second answer"},
	})
	if got := persisted.TokenUsage; got.TotalInput != 31 || got.TotalOutput != 7 || got.TotalCacheRead != 15 {
		t.Fatalf("persisted cumulative usage = %+v", got)
	}

	requestsMu.Lock()
	defer requestsMu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(requests))
	}
	if requests[0].Model != "test-model" || requests[1].Model != "test-model" {
		t.Fatalf("provider models = %q, %q", requests[0].Model, requests[1].Model)
	}
	if !wireMessagesContainInOrder(requests[1].Messages, []runWireMessage{
		{Role: "user", Content: "first prompt"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second prompt"},
	}) {
		t.Fatalf("resumed provider request did not contain persisted history: %+v", requests[1].Messages)
	}
}

// Even when the model handles a rejected tool call and produces a final text
// response, headless ask mode must remain a failed run. This exercises the
// actual native write_file registration and policy/approver path, rather than
// simulating errRunApprovalUnavailable at the command seam.
func TestExecuteRunWithRuntime_ApprovalRequiredHasNoSideEffect(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(git.RepoRoot(cwd), ".packetcode-run-approval-"+uuid.NewString())
	t.Cleanup(func() { _ = os.Remove(marker) })
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marker unexpectedly exists before run: %v", err)
	}

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := requestCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if call == 1 {
			arguments, marshalErr := json.Marshal(map[string]string{
				"path":    marker,
				"content": "this must never be written",
			})
			if marshalErr != nil {
				http.Error(w, marshalErr.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_write\",\"type\":\"function\",\"function\":{\"name\":\"write_file\",\"arguments\":\"\"}}]}}]}\n\n")
			fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":%q}}]}}]}\n\n", string(arguments))
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":1,\"total_tokens\":8}}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"adapted after rejection\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	packetHome := t.TempDir()
	t.Setenv(config.HomeEnv, packetHome)
	keyless, supportsTools := false, true
	cfg := config.Default()
	cfg.Default.Provider = "approval-test"
	cfg.Default.Model = "test-model"
	cfg.Permissions.Profile = "ask"
	cfg.Behavior.ProviderMaxRetries = 1
	cfg.Providers["approval-test"] = config.ProviderConfig{
		Type:           "openai_compatible",
		BaseURL:        server.URL + "/v1",
		APIKeyRequired: &keyless,
		DefaultModel:   "test-model",
		Models: []config.ProviderModelConfig{{
			ID:            "test-model",
			SupportsTools: &supportsTools,
		}},
	}
	configPath, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTo(configPath); err != nil {
		t.Fatal(err)
	}

	result, err := executeRunWithRuntime(t.Context(), runCommandOptions{
		Prompt: "write the marker file",
	}, io.Discard)
	if !errors.Is(err, errRunApprovalUnavailable) {
		t.Fatalf("run error = %v, want approval unavailable", err)
	}
	if result.Output != "adapted after rejection" {
		t.Fatalf("adapted output = %q", result.Output)
	}
	if result.Usage != (runUsage{InputTokens: 17, OutputTokens: 3}) {
		t.Fatalf("per-run usage = %+v", result.Usage)
	}
	if got := requestCount.Load(); got != 2 {
		t.Fatalf("provider requests = %d, want rejection plus adapted response", got)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("approval-gated write had a side effect: %v", statErr)
	}
}

func assertPersistedRunMessages(t *testing.T, got, want []provider.Message) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("persisted messages = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content {
			t.Fatalf("persisted message %d = (%s, %q), want (%s, %q)",
				i, got[i].Role, got[i].Content, want[i].Role, want[i].Content)
		}
	}
}

func wireMessagesContainInOrder(got, want []runWireMessage) bool {
	next := 0
	for _, message := range got {
		if next < len(want) && message.Role == want[next].Role && message.Content == want[next].Content {
			next++
		}
	}
	return next == len(want)
}
