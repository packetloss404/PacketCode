package provider

import "context"

// StreamSink is the send side of a provider's event channel, bound to the
// turn's context.
//
// Every provider parses its own wire format and then pushes StreamEvents down
// an 8-buffered channel. Those sends used to be bare — `ch <- ev` with no
// select on ctx.Done() — in five parsers, sixty-nine places. Nothing has
// deadlocked, because the consumer in internal/agent drains until the channel
// closes; but that made "the consumer always drains" a contract held by
// convention across five files rather than by construction. A consumer that
// returns early, in any future refactor, strands the parser goroutine on a
// full buffer forever, holding the response body and the stall guard with it.
//
// The sink exists so the guarantee is structural. A parser cannot send without
// going through it, and the sink cannot send without observing cancellation.
//
// Not safe for concurrent use: each parser owns one and runs in one goroutine,
// which is what lets Send stay this small.
type StreamSink struct {
	ctx context.Context
	ch  chan<- StreamEvent
}

// NewStreamSink binds a channel to a context for the life of one stream.
func NewStreamSink(ctx context.Context, ch chan<- StreamEvent) *StreamSink {
	return &StreamSink{ctx: ctx, ch: ch}
}

// Send delivers one event, reporting false when the turn was cancelled before
// the consumer took it.
//
// A false return means stop parsing: the caller's remaining events have nobody
// to receive them, and the response body is about to be closed underneath it.
// Callers treat it as the end of the stream, not as an error to report — the
// cancellation is already known to whoever cancelled.
//
// Checking that return is hygiene, not the safety mechanism. Once the context
// is done Send refuses instantly and forever, so a parser that ignored the
// result would spin through its remaining frames and exit at its own
// cancellation check rather than hanging. What actually prevents the strand is
// this function never blocking indefinitely — which is why the guarantee lives
// here and not in sixty-nine call sites.
func (s *StreamSink) Send(ev StreamEvent) bool {
	if s == nil {
		return false
	}
	// The non-blocking attempt first. The buffer is almost always free, and
	// this keeps the common path off the select's context read.
	select {
	case s.ch <- ev:
		return true
	default:
	}
	select {
	case s.ch <- ev:
		return true
	case <-s.ctx.Done():
		return false
	}
}

// Text sends a text delta. Empty deltas are dropped rather than sent: a
// provider that emits them would otherwise wake the UI for nothing.
func (s *StreamSink) Text(delta string) bool {
	if delta == "" {
		return true
	}
	return s.Send(StreamEvent{Type: EventTextDelta, TextDelta: delta})
}

// Reasoning sends a reasoning delta, with the same empty-delta rule as Text.
func (s *StreamSink) Reasoning(delta string) bool {
	if delta == "" {
		return true
	}
	return s.Send(StreamEvent{Type: EventReasoningDelta, TextDelta: delta})
}

// Fail sends an error event. A nil error is not an error and is dropped, so a
// caller can forward a maybe-error without guarding every site.
func (s *StreamSink) Fail(err error) bool {
	if err == nil {
		return true
	}
	return s.Send(StreamEvent{Type: EventError, Error: err})
}

// Done sends the terminal event.
func (s *StreamSink) Done() bool {
	return s.Send(StreamEvent{Type: EventDone})
}
