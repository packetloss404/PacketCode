package codex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/packetcode/packetcode/internal/provider"
)

func writeAuth(t *testing.T, access string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	obj := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token":  access,
			"refresh_token": "ref",
			"account_id":    "acct-1",
		},
	}
	raw, _ := json.MarshalIndent(obj, "", "  ")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	return path
}

func TestIdentity(t *testing.T) {
	p := New("")
	if p.Slug() != "codex" || p.Name() == "" {
		t.Fatalf("identity wrong: slug=%q name=%q", p.Slug(), p.Name())
	}
}

func TestListModelsAndPricing(t *testing.T) {
	p := New(writeAuth(t, "tok"))
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 || models[0].ID != DefaultModel {
		t.Fatalf("expected default model first, got %+v", models)
	}
	in, out := p.Pricing("gpt-5-codex")
	if in != 0 || out != 0 {
		t.Fatalf("subscription pricing should be zero, got %v/%v", in, out)
	}
	if !p.SupportsTools("gpt-5-codex") {
		t.Fatal("codex models must support tools")
	}
}

func TestValidateKeyRequiresLogin(t *testing.T) {
	// Missing file → error.
	if err := New(filepath.Join(t.TempDir(), "missing.json")).ValidateKey(context.Background(), ""); err == nil {
		t.Fatal("expected error when auth.json is missing")
	}
	// Present login → ok.
	if err := New(writeAuth(t, "tok")).ValidateKey(context.Background(), ""); err != nil {
		t.Fatalf("expected valid login to pass, got %v", err)
	}
}

func TestChatCompletionEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `data: {"type":"response.output_text.delta","delta":"hi"}

data: {"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":1}}}

`)
	}))
	defer srv.Close()

	p := NewWithBaseURL(writeAuth(t, "tok"), srv.URL)
	ch, err := p.ChatCompletion(context.Background(), provider.ChatRequest{
		Model:    "gpt-5-codex",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hey"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	var text string
	var done bool
	for ev := range ch {
		switch ev.Type {
		case provider.EventTextDelta:
			text += ev.TextDelta
		case provider.EventDone:
			done = true
		case provider.EventError:
			t.Fatalf("stream error: %v", ev.Error)
		}
	}
	if text != "hi" || !done {
		t.Fatalf("unexpected stream result: text=%q done=%v", text, done)
	}
}
