package codexauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeAuthFile(t *testing.T, dir string, obj map[string]any) string {
	t.Helper()
	path := filepath.Join(dir, "auth.json")
	raw, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatalf("marshal auth file: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	return path
}

func fixedNow() func() time.Time {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return func() time.Time { return ts }
}

func TestTokenReadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := writeAuthFile(t, dir, map[string]any{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": "",
		"tokens": map[string]any{
			"id_token":      "idtok",
			"access_token":  "acc-1",
			"refresh_token": "ref-1",
			"account_id":    "acct-123",
		},
		"last_refresh": "2026-01-01T00:00:00Z",
	})

	s := New(path)
	if err := s.Available(); err != nil {
		t.Fatalf("Available: %v", err)
	}
	tok, err := s.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "acc-1" || tok.AccountID != "acct-123" {
		t.Fatalf("unexpected tokens: %+v", tok)
	}
}

func TestTokenReflectsExternalRewrite(t *testing.T) {
	dir := t.TempDir()
	path := writeAuthFile(t, dir, map[string]any{
		"tokens": map[string]any{"access_token": "acc-1", "refresh_token": "ref-1", "account_id": "a"},
	})
	s := New(path)
	if _, err := s.Token(context.Background()); err != nil {
		t.Fatalf("first Token: %v", err)
	}
	// Simulate the Codex CLI refreshing the file underneath us.
	writeAuthFile(t, dir, map[string]any{
		"tokens": map[string]any{"access_token": "acc-2", "refresh_token": "ref-2", "account_id": "a"},
	})
	tok, err := s.Token(context.Background())
	if err != nil {
		t.Fatalf("second Token: %v", err)
	}
	if tok.AccessToken != "acc-2" {
		t.Fatalf("expected reloaded acc-2, got %q", tok.AccessToken)
	}
}

func TestAvailableRejectsAPIKeyMode(t *testing.T) {
	dir := t.TempDir()
	path := writeAuthFile(t, dir, map[string]any{
		"OPENAI_API_KEY": "sk-abc",
		"tokens":         map[string]any{},
	})
	if err := New(path).Available(); err == nil {
		t.Fatal("expected error for API-key-only auth.json")
	}
}

func TestAvailableMissingFile(t *testing.T) {
	if err := New(filepath.Join(t.TempDir(), "nope.json")).Available(); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRefreshPersistsAndPreservesFields(t *testing.T) {
	dir := t.TempDir()
	path := writeAuthFile(t, dir, map[string]any{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": "",
		"tokens": map[string]any{
			"id_token":      "old-id",
			"access_token":  "old-acc",
			"refresh_token": "old-ref",
			"account_id":    "acct-xyz",
		},
		"last_refresh": "2020-01-01T00:00:00Z",
		"custom_field": "keep-me",
	})

	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id_token":"new-id","access_token":"new-acc","refresh_token":"new-ref","expires_in":3600}`))
	}))
	defer srv.Close()

	s := New(path)
	s.tokenURL = srv.URL
	s.now = fixedNow()

	tok, err := s.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if tok.AccessToken != "new-acc" || tok.RefreshToken != "new-ref" || tok.IDToken != "new-id" {
		t.Fatalf("unexpected refreshed tokens: %+v", tok)
	}
	// account_id is not part of the OAuth response; must be carried forward.
	if tok.AccountID != "acct-xyz" {
		t.Fatalf("account_id not preserved: %q", tok.AccountID)
	}
	// Request body must carry the refresh grant.
	if gotBody["grant_type"] != "refresh_token" || gotBody["refresh_token"] != "old-ref" || gotBody["client_id"] != oauthClientID {
		t.Fatalf("unexpected refresh request body: %+v", gotBody)
	}

	// File on disk must reflect new tokens and preserve custom_field.
	raw, _ := os.ReadFile(path)
	var onDisk map[string]any
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if onDisk["custom_field"] != "keep-me" {
		t.Fatalf("custom_field not preserved: %v", onDisk["custom_field"])
	}
	if onDisk["last_refresh"] != "2026-01-02T03:04:05Z" {
		t.Fatalf("last_refresh not updated: %v", onDisk["last_refresh"])
	}
	persisted := onDisk["tokens"].(map[string]any)
	if persisted["access_token"] != "new-acc" || persisted["account_id"] != "acct-xyz" {
		t.Fatalf("persisted tokens wrong: %+v", persisted)
	}
}

func TestRefreshKeepsOldRefreshTokenWhenOmitted(t *testing.T) {
	dir := t.TempDir()
	path := writeAuthFile(t, dir, map[string]any{
		"tokens": map[string]any{"access_token": "old-acc", "refresh_token": "old-ref", "account_id": "a"},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"new-acc","expires_in":3600}`))
	}))
	defer srv.Close()

	s := New(path)
	s.tokenURL = srv.URL
	s.now = fixedNow()

	tok, err := s.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if tok.RefreshToken != "old-ref" {
		t.Fatalf("expected old refresh token retained, got %q", tok.RefreshToken)
	}
}

func TestRefreshServerError(t *testing.T) {
	dir := t.TempDir()
	path := writeAuthFile(t, dir, map[string]any{
		"tokens": map[string]any{"access_token": "a", "refresh_token": "r", "account_id": "x"},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	s := New(path)
	s.tokenURL = srv.URL
	if _, err := s.Refresh(context.Background()); err == nil {
		t.Fatal("expected error on 401 refresh")
	}
}
