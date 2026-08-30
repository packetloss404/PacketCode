package responses

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/packetcode/packetcode/internal/provider"
)

// collectBackend drains parseSSE for one stream body at a given backend.
func collectBackend(t *testing.T, body string, backend Backend) []provider.StreamEvent {
	t.Helper()
	ch := make(chan provider.StreamEvent, 16)
	guard, sctx := provider.NewStallGuard(context.Background(), 0)
	parseSSE(context.Background(), sctx, guard, io.NopCloser(strings.NewReader(body)), ch, backend)
	var out []provider.StreamEvent
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

// The public API nests the message under "error". Reading only the top-level
// "message" turned a message that told the user exactly what to do -- verify
// the organization, with the URL -- into "stream error".
func TestParseSSE_SurfacesNestedErrorMessage(t *testing.T) {
	const want = "Your organization must be verified to use the model `o3-pro`."
	body := "event: error\n" +
		`data: {"type":"error","error":{"type":"invalid_request_error","code":"model_not_found","message":"` + want + `"}}` + "\n\n"

	var got string
	for _, ev := range collectBackend(t, body, BackendOpenAIAPI) {
		if ev.Type == provider.EventError && ev.Error != nil {
			got = ev.Error.Error()
		}
	}
	if got != want {
		t.Fatalf("Error = %q,\n   want %q", got, want)
	}
}

// The top-level form still works; the nested one is an addition, not a swap.
func TestParseSSE_SurfacesTopLevelErrorMessage(t *testing.T) {
	body := "event: error\n" + `data: {"type":"error","message":"top level boom"}` + "\n\n"
	var got string
	for _, ev := range collectBackend(t, body, BackendChatGPT) {
		if ev.Type == provider.EventError && ev.Error != nil {
			got = ev.Error.Error()
		}
	}
	if got != "top level boom" {
		t.Fatalf("Error = %q", got)
	}
}

// An error with no message at all still has to name something, and it must
// name the service the user configured rather than always saying "codex".
func TestParseSSE_MessagelessErrorNamesTheBackend(t *testing.T) {
	body := "event: error\n" + `data: {"type":"error"}` + "\n\n"
	for _, tc := range []struct {
		backend Backend
		want    string
	}{
		{BackendOpenAIAPI, "openai stream error"},
		{BackendChatGPT, "codex stream error"},
	} {
		var got string
		for _, ev := range collectBackend(t, body, tc.backend) {
			if ev.Type == provider.EventError && ev.Error != nil {
				got = ev.Error.Error()
			}
		}
		if got != tc.want {
			t.Errorf("backend %v: Error = %q, want %q", tc.backend, got, tc.want)
		}
	}
}
