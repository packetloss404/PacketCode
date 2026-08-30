package provider

import (
	"context"
	"testing"
	"time"
)

func TestStreamSink_DeliversWhenDrained(t *testing.T) {
	ch := make(chan StreamEvent, 8)
	s := NewStreamSink(context.Background(), ch)
	if !s.Text("hello") {
		t.Fatal("Send reported failure on an empty buffer")
	}
	ev := <-ch
	if ev.Type != EventTextDelta || ev.TextDelta != "hello" {
		t.Fatalf("got %+v", ev)
	}
}

// The bug this exists to make impossible: a bare send on a full channel whose
// consumer has stopped draining blocks forever, stranding the parser goroutine
// with the response body and the stall guard still held.
func TestStreamSink_UnblocksOnCancellation(t *testing.T) {
	ch := make(chan StreamEvent) // unbuffered and nobody receiving
	ctx, cancel := context.WithCancel(context.Background())
	s := NewStreamSink(ctx, ch)

	done := make(chan bool, 1)
	go func() { done <- s.Text("stuck") }()

	select {
	case <-done:
		t.Fatal("Send returned while no consumer was receiving")
	case <-time.After(30 * time.Millisecond):
	}

	cancel()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("Send reported success after cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send did not unblock on cancellation; this is the deadlock")
	}
}

// An already-cancelled context must not let one more event through, or a
// cancelled turn can still push text into a conversation that has moved on.
func TestStreamSink_RefusesAfterCancellation(t *testing.T) {
	ch := make(chan StreamEvent) // unbuffered: no room to sneak one in
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if NewStreamSink(ctx, ch).Text("late") {
		t.Fatal("Send succeeded on a cancelled context")
	}
}

// Empty deltas are dropped rather than sent: forwarding them wakes the UI for
// nothing, and every provider emits them somewhere.
func TestStreamSink_DropsEmptyDeltasAndNilErrors(t *testing.T) {
	ch := make(chan StreamEvent, 4)
	s := NewStreamSink(context.Background(), ch)
	for _, ok := range []bool{s.Text(""), s.Reasoning(""), s.Fail(nil)} {
		if !ok {
			t.Fatal("dropping a no-op event must report success")
		}
	}
	if len(ch) != 0 {
		t.Fatalf("expected nothing sent, got %d events", len(ch))
	}
}

// A nil sink is a programming error, not a panic: report failure so a parser
// stops rather than crashing the process mid-turn.
func TestStreamSink_NilIsNotAPanic(t *testing.T) {
	var s *StreamSink
	if s.Send(StreamEvent{Type: EventDone}) {
		t.Fatal("a nil sink reported success")
	}
}
