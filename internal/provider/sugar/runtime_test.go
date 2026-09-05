package sugar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/provider"
)

func TestRuntimeClientIsInertUntilExplicitlyEnabled(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	client := NewRuntimeClient(server.URL, "sgr_test", server.Client(), false)
	run, err := client.StartRun(context.Background(), validRuntimeRunStart())
	require.NoError(t, err)
	assert.Nil(t, run)
	event, err := client.EmitEvent(context.Background(), validRuntimeEvent())
	require.NoError(t, err)
	assert.Nil(t, event)
	decision, err := client.Continue(context.Background(), "run-1", "continue-1")
	require.NoError(t, err)
	assert.Nil(t, decision)
	assert.Zero(t, requests.Load())
}

func TestRuntimeClientFallsBackAndDisablesWhenStartEndpointIsUnavailable(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewRuntimeClient(server.URL, "sgr_test", server.Client(), true)
	run, err := client.StartRun(context.Background(), validRuntimeRunStart())
	require.NoError(t, err)
	assert.Nil(t, run)
	assert.False(t, client.Enabled())
	_, err = client.StartRun(context.Background(), validRuntimeRunStart())
	require.NoError(t, err)
	assert.Equal(t, int32(1), requests.Load())
}

func TestRuntimeClientMatchesSugarV8ShadowContract(t *testing.T) {
	var sawStart, sawEvent, sawContinue bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Bearer sgr_test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/conduit/runs":
			sawStart = true
			assert.Equal(t, "run-start-1", r.Header.Get("Idempotency-Key"))
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "sugar/conduit", body["model"])
			assert.Contains(t, body, "messages", "the v8 start route deliberately reuses the bounded chat body")
			assert.NotContains(t, body, "sugar_cache")
			assert.NotContains(t, body, "idempotency_key")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"run":{"id":"run-1","object":"conduit.shadow_run","mode":"shadow","requestedModel":"sugar/conduit","status":"shadow_observing"},"replayed":false,"executesUpstream":false}`)
		case "/api/v1/conduit/runs/run-1/events":
			sawEvent = true
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, float64(1), body["seq"])
			assert.Equal(t, "event-1", body["idempotencyKey"])
			assert.Equal(t, "validation", body["type"])
			assert.NotContains(t, body, "RunID")
			assert.NotContains(t, body, "run_id")
			assert.NotContains(t, body, "messages")
			assert.NotContains(t, body, "prompt")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"object":"conduit.shadow_event","event":{"seq":1,"type":"validation","scoreDelta":0,"duplicate":false},"run":{"id":"run-1"},"executesUpstream":false}`)
		case "/api/v1/conduit/runs/run-1/continue":
			sawContinue = true
			assert.Equal(t, "continue-1", r.Header.Get("Idempotency-Key"))
			fmt.Fprint(w, `{"object":"conduit.shadow_decision","decision":{"action":"observe","wouldEscalate":false,"tier":"economy_fast","predictedModel":"model","reasonCodes":["awaiting_runtime_evidence"],"stopReason":null},"run":{"id":"run-1"},"replayed":false,"executesUpstream":false}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewRuntimeClient(server.URL, "sgr_test", server.Client(), true)
	runResponse, err := client.StartRun(context.Background(), validRuntimeRunStart())
	require.NoError(t, err)
	require.NotNil(t, runResponse)
	assert.Equal(t, "run-1", runResponse.Run.ID)
	event := validRuntimeEvent()
	event.RunID = runResponse.Run.ID
	eventResponse, err := client.EmitEvent(context.Background(), event)
	require.NoError(t, err)
	require.NotNil(t, eventResponse)
	assert.False(t, eventResponse.Event.Duplicate)
	decision, err := client.Continue(context.Background(), runResponse.Run.ID, "continue-1")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, "observe", decision.Decision.Action)
	assert.True(t, sawStart)
	assert.True(t, sawEvent)
	assert.True(t, sawContinue)
}

func TestRuntimeEventRejectsRawFailureText(t *testing.T) {
	event := validRuntimeEvent()
	event.FailureFingerprint = "test output: secret token"
	err := validateRuntimeEvent(event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sha256")
}

func TestRuntimeEventRejectsContradictorySuccessfulValidation(t *testing.T) {
	event := validRuntimeEvent()
	nonzero := 1
	event.ExitCode = &nonzero
	err := validateRuntimeEvent(event)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "successful")
}

func TestRuntimeClientNestedRouteFallbackDistinguishesMissingEndpointFromMissingRun(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if requests.Load() == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":{"type":"run_not_found","message":"missing"}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewRuntimeClient(server.URL, "sgr_test", server.Client(), true)
	_, err := client.EmitEvent(context.Background(), validRuntimeEvent())
	require.Error(t, err)
	assert.True(t, client.Enabled(), "a missing run is not a missing endpoint")
	response, err := client.EmitEvent(context.Background(), validRuntimeEvent())
	require.NoError(t, err)
	assert.Nil(t, response)
	assert.False(t, client.Enabled(), "a generic route 404 disables optional hooks")
}

func validRuntimeRunStart() RuntimeRunStart {
	return RuntimeRunStart{
		IdempotencyKey: "run-start-1",
		Request: provider.ChatRequest{
			Model:    DefaultModel,
			Messages: []provider.Message{{Role: provider.RoleUser, Content: "fix the test"}},
			Stream:   true,
			SugarCache: &provider.SugarCacheMetadata{
				ConversationID:    "conversation-1",
				PrefixFingerprint: provider.CachePrefixFingerprint("", nil),
			},
		},
	}
}

func validRuntimeEvent() RuntimeEvent {
	success := true
	exitCode := 0
	return RuntimeEvent{
		RunID:          "run-1",
		Seq:            1,
		IdempotencyKey: "event-1",
		Type:           RuntimeValidation,
		ToolCategory:   RuntimeToolTest,
		Success:        &success,
		ExitCode:       &exitCode,
	}
}
