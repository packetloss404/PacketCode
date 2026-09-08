package acp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/agent"
)

type promptHandoffWriter func([]byte) (int, error)

func (w promptHandoffWriter) Write(p []byte) (int, error) { return w(p) }

type promptHandoffRunner struct {
	first   bool
	events  []agent.AgentEvent
	started chan struct{}
}

func (r *promptHandoffRunner) Run(ctx context.Context, _ string) <-chan agent.AgentEvent {
	if !r.first {
		r.first = true
		out := make(chan agent.AgentEvent, len(r.events))
		for _, ev := range r.events {
			out <- ev
		}
		close(out)
		return out
	}
	out := make(chan agent.AgentEvent, 1)
	close(r.started)
	go func() {
		defer close(out)
		<-ctx.Done()
		out <- agent.AgentEvent{Type: agent.EventError, Error: ctx.Err()}
	}()
	return out
}

// Deliver the next prompt while the previous response is being written. This
// fixes the scheduling order instead of relying on a client winning a race.
func TestServerPromptCleanupPreservesNextPrompt(t *testing.T) {
	for _, tc := range []struct {
		name      string
		events    []agent.AgentEvent
		cancelled bool
	}{
		{name: "completed", events: []agent.AgentEvent{{Type: agent.EventDone}}},
		{name: "failed", events: []agent.AgentEvent{{Type: agent.EventError, Error: errors.New("failed")}}},
		{name: "cancelled", cancelled: true},
		{name: "missing terminal event"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &promptHandoffRunner{events: tc.events, started: make(chan struct{})}
			state := &sessionState{active: true, cancelled: tc.cancelled, runtime: &Runtime{Runner: runner}}
			ctx, cancel := context.WithCancel(context.Background())
			s := NewServer(nil, nil, nil, nil, "test")
			s.ctx = ctx
			s.sessions["session"] = state
			delivered := false
			s.out = promptHandoffWriter(func(p []byte) (int, error) {
				var msg struct {
					ID json.RawMessage `json:"id"`
				}
				if err := json.Unmarshal(p, &msg); err != nil {
					return 0, err
				}
				if string(msg.ID) == "1" {
					delivered = true
					s.handlePrompt(rpcMessage{ID: json.RawMessage("2"), Params: json.RawMessage(
						`{"sessionId":"session","prompt":[{"type":"text","text":"next"}]}`)})
				}
				return len(p), nil
			})
			t.Cleanup(func() {
				cancel()
				s.wg.Wait()
			})
			s.runPrompt(ctx, json.RawMessage("1"), "session", "first", state)
			require.True(t, delivered, "the first prompt never sent its terminal response")
			select {
			case <-runner.started:
			case <-time.After(3 * time.Second):
				t.Fatal("next prompt was not accepted")
			}
			state.mu.Lock()
			active := state.active
			state.mu.Unlock()
			assert.True(t, active, "previous prompt cleanup cleared the next prompt's active flag")
			s.handleCancel(rpcMessage{Params: json.RawMessage(`{"sessionId":"session"}`)})
			stopped := make(chan struct{})
			go func() { s.wg.Wait(); close(stopped) }()
			select {
			case <-stopped:
			case <-time.After(3 * time.Second):
				t.Fatal("next prompt ignored cancellation")
			}
		})
	}
}
