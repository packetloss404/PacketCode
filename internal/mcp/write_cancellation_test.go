package mcp

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/testwait"
)

type signalledMCPWriter struct {
	io.WriteCloser
	started  chan struct{}
	finished chan struct{}
}

func (w *signalledMCPWriter) Write(p []byte) (int, error) {
	close(w.started)
	defer close(w.finished)
	return w.WriteCloser.Write(p)
}

func TestClient_BlockedWriteHonorsCancellation(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	writer := &signalledMCPWriter{WriteCloser: w, started: make(chan struct{}), finished: make(chan struct{})}
	c := &Client{stdin: writer}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.callTimed(ctx, "tools/call", map[string]any{}); done <- err }()
	select {
	case <-writer.started:
	case <-time.After(testwait.Timeout(time.Second)):
		t.Fatal("write never started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("call error = %v", err)
		}
		if c.IsAlive() {
			t.Fatal("partially written transport must be closed")
		}
	case <-time.After(testwait.Timeout(time.Second)):
		_ = r.Close()
		<-done
		t.Fatal("MCP write stayed blocked after cancellation")
	}
	pending := 0
	c.pending.Range(func(_, _ any) bool { pending++; return true })
	if pending != 0 {
		t.Fatalf("pending calls leaked: %d", pending)
	}
	select {
	case <-writer.finished:
	case <-time.After(testwait.Timeout(time.Second)):
		t.Fatal("cancelled transport left its write goroutine blocked")
	}
}
