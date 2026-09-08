package responses

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/provider"
)

func TestStreamRequiresResponseCompleted(t *testing.T) {
	tool := "data: " + `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call_1","name":"write_file"}}` + "\n\n" +
		"data: " + `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","call_id":"call_1","name":"write_file","arguments":"{\"path\":\"x\",\"content\":\"partial\"}"}}` + "\n\n"
	for _, tc := range []struct{ name, body, wantError string }{
		{"empty", "", "missing response.completed"},
		{"partial text", "data: " + `{"type":"response.output_text.delta","delta":"partial"}` + "\n\n", "missing response.completed"},
		{"closed tool without response completion", tool, "missing response.completed"},
		{"completed tool", tool + "data: " + `{"type":"response.completed"}` + "\n\n", ""},
		{"failed after tool", tool + "data: " + `{"type":"response.failed","response":{"error":{"message":"provider failure"}}}` + "\n\n", "provider failure"},
		{"incomplete after tool", tool + "data: " + `{"type":"response.incomplete"}` + "\n\n", "response incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotErr error
			var done bool
			for _, ev := range collectBackend(t, tc.body, BackendOpenAIAPI) {
				if ev.Type == provider.EventError {
					gotErr = ev.Error
				}
				if ev.Type == provider.EventDone {
					done = true
				}
			}
			if tc.wantError != "" {
				require.ErrorContains(t, gotErr, tc.wantError)
				require.False(t, done)
			} else {
				require.NoError(t, gotErr)
				require.True(t, done)
			}
		})
	}
}
