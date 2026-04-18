package openaicompat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/provider"
)

// TestParseSSE_CancelStopsStream verifies that cancelling the context
// passed to ChatCompletion tears down the in-flight SSE reader promptly
// (well under 1s) and surfaces an EventError whose cause satisfies
// errors.Is(context.Canceled). This is the Round 5 invariant: first
// Ctrl+C at the App layer must actually stop the provider stream, not
// just silence the spinner.
func TestParseSSE_CancelStopsStream(t *testing.T) {
	// Slow-trickle server: one SSE "frame" every 200ms, flushed
	// individually. ChatCompletion returns as soon as headers are sent.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter does not implement Flusher")
		}
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		// Emit up to ~50 frames so the test's cancel definitely lands
		// mid-stream; the server side terminates when the client drops.
		for i := 0; i < 50; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(200 * time.Millisecond):
			}
			if _, err := fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"chunk %d \"}}]}\n\n", i); err != nil {
				return
			}
			flusher.Flush()
		}
	}))
	defer server.Close()

	c := NewClient(server.URL, "sk-test")
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := c.ChatCompletion(ctx, provider.ChatRequest{
		Model:    "gpt-4.1",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "stream please"}},
	})
	require.NoError(t, err)

	// Cancel after 100ms, ensuring the first chunk has (likely) arrived
	// but the stream is nowhere near exhausted.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// Drain with a 1s ceiling — if the channel doesn't close, fail.
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

	// Locate the cancellation EventError and confirm the cause chain.
	var sawCancelErr bool
	for _, ev := range events {
		if ev.Type == provider.EventError && ev.Error != nil && errors.Is(ev.Error, context.Canceled) {
			sawCancelErr = true
			break
		}
	}
	assert.True(t, sawCancelErr, "expected EventError wrapping context.Canceled; got events: %+v", events)
}
