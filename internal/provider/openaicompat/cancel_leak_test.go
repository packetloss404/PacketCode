package openaicompat

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/provider"
)

// parserRunning reports whether any goroutine is inside parseSSE.
//
// The stack is read rather than runtime.NumGoroutine counted: the httptest
// server's accept loop and the transport's idle connections outlive the
// request, so a count answers a different question than the one being asked.
func parserRunning() bool {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return bytes.Contains(buf[:n], []byte("openaicompat.parseSSE"))
}

// A consumer that stops draining must not strand the parser goroutine.
//
// The parser already re-checks cancellation at the top of each scanner
// iteration, so a consumer that cancels *and then drains* was always fine —
// the drain releases the blocked send and the next iteration bails. The gap
// the sink closes is the one where nobody drains again: a bare `ch <- ev` on a
// full 8-buffered channel never returns, the loop never reaches its next
// iteration, and the goroutine sits there forever holding the response body
// and the stall guard.
//
// So this test deliberately never receives after cancelling. It asserts on the
// goroutine, not on the channel, because observing a channel close requires
// receiving from it — which is the very thing that hides the bug.
func TestParseSSE_CancelledConsumerDoesNotStrandTheParser(t *testing.T) {
	const frames = 500 // far more than the channel buffers, so a send must block

	var body strings.Builder
	for i := 0; i < frames; i++ {
		fmt.Fprintf(&body, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"chunk %d\"}}]}\n\n", i)
	}
	body.WriteString("data: [DONE]\n\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body.String())
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := NewClient(srv.URL, "k").ChatCompletion(ctx, provider.ChatRequest{
		Model:    "m",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	// One event, then stop receiving so the parser fills the buffer and blocks
	// on a send — the state the bug lives in.
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("no first event")
	}
	deadline := time.Now().Add(3 * time.Second)
	for !parserRunning() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !parserRunning() {
		t.Skip("parser finished before it could be caught mid-send; nothing to assert")
	}

	cancel()

	// From here nothing is ever received from ch. The parser must still exit.
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !parserRunning() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	t.Fatalf("parseSSE is still running 10s after cancellation with no consumer: "+
		"it is stranded on a send\n%s", buf[:n])
}
