package anthropic

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/provider"
)

func TestStreamRequiresMessageStop(t *testing.T) {
	tool := "data: " + `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"write_file","input":{}}}` + "\n\n" +
		"data: " + `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"x\",\"content\":\"partial\"}"}}` + "\n\n"
	for _, tc := range []struct {
		name, body string
		wantError  bool
	}{
		{"empty", "", true},
		{"partial text", "data: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial"}}` + "\n\n", true},
		{"open tool with valid JSON", tool, true},
		{"closed tool without message stop", tool + "data: " + `{"type":"content_block_stop","index":0}` + "\n\n", true},
		{"completed tool", tool + "data: " + `{"type":"content_block_stop","index":0}` + "\n\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"tool_use"}}` + "\n\ndata: " + `{"type":"message_stop"}` + "\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan provider.StreamEvent, 16)
			guard, ctx := provider.NewStallGuard(context.Background(), 0)
			parseSSE(ctx, ctx, guard, io.NopCloser(strings.NewReader(tc.body)), ch)
			var gotErr error
			var done bool
			for ev := range ch {
				if ev.Type == provider.EventError {
					gotErr = ev.Error
				}
				if ev.Type == provider.EventDone {
					done = true
				}
			}
			if tc.wantError {
				require.ErrorContains(t, gotErr, "missing message_stop")
				require.False(t, done)
			} else {
				require.NoError(t, gotErr)
				require.True(t, done)
			}
		})
	}
}
