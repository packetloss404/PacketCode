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

func TestSummaryFor_OmitsForUnsupportedModel(t *testing.T) {
	p := New(writeAuth(t, "tok"))
	// Inject a catalog with a spark-like model that rejects the summary param.
	p.catalog = []cachedModel{
		{Slug: "gpt-5.5", SummarySupported: true},
		{Slug: "gpt-5.3-codex-spark", SummarySupported: false},
	}
	if got := p.summaryFor("gpt-5.5"); got != "auto" {
		t.Fatalf("gpt-5.5 summary = %q, want auto", got)
	}
	if got := p.summaryFor("gpt-5.3-codex-spark"); got != "" {
		t.Fatalf("spark summary = %q, want empty (omit to avoid 400)", got)
	}
	// Unknown model defaults to auto.
	if got := p.summaryFor("something-new"); got != "auto" {
		t.Fatalf("unknown summary = %q, want auto", got)
	}
}

func TestReasoningEffortUsesAdvertisedOverrideAndReset(t *testing.T) {
	p := New(writeAuth(t, "tok"))
	if got, want := p.ReasoningEffort("gpt-5.6-sol"), "low"; got != want {
		t.Fatalf("default effort = %q, want %q", got, want)
	}
	if got := len(p.ReasoningEfforts("gpt-5.6-sol")); got != 6 {
		t.Fatalf("advertised efforts = %d, want 6", got)
	}
	if err := p.SetReasoningEffort("gpt-5.6-sol", "ultra"); err != nil {
		t.Fatalf("set ultra: %v", err)
	}
	if got, want := p.ReasoningEffort("gpt-5.6-sol"), "ultra"; got != want {
		t.Fatalf("overridden effort = %q, want %q", got, want)
	}
	if err := p.SetReasoningEffort("gpt-5.6-sol", "impossible"); err == nil {
		t.Fatal("unsupported effort should fail")
	}
	if err := p.SetReasoningEffort("gpt-5.6-sol", "default"); err != nil {
		t.Fatalf("reset effort: %v", err)
	}
	if got, want := p.ReasoningEffort("gpt-5.6-sol"), "low"; got != want {
		t.Fatalf("reset effort = %q, want %q", got, want)
	}
}
