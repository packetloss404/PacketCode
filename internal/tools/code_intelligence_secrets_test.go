package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeIntelRefusesSecretIdentifiers(t *testing.T) {
	root := t.TempDir()
	const secret = "syntheticsecretvalue"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("AUDIT_TOKEN="+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []Tool{NewFindDefinitionTool(root), NewFindReferencesTool(root)} {
		t.Run(tool.Name(), func(t *testing.T) {
			result, err := tool.Execute(context.Background(), json.RawMessage(`{"path":".env","line":1,"column":13}`))
			if err != nil || !result.IsError || !strings.Contains(result.Content, "dotenv secret files") {
				t.Fatalf("expected secret-file refusal, result=%+v err=%v", result, err)
			}
			if strings.Contains(result.Content, secret) {
				t.Fatal("secret appeared in inferred-identifier output")
			}
		})
	}
}

func TestCodeIntelSkipsSecretSourceFiles(t *testing.T) {
	root := t.TempDir()
	// A dotenv suffix can also be a source extension. All collectors must
	// skip it before reading; diagnostics can leak source text in errors too.
	if err := os.WriteFile(filepath.Join(root, ".env.go"), []byte("package p\nconst SecretMarker = \"syntheticsecretvalue\"\nfunc Broken(\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		tool Tool
		raw  string
	}{
		{NewListSymbolsTool(root), `{}`},
		{NewFindDefinitionTool(root), `{"symbol":"SecretMarker"}`},
		{NewFindReferencesTool(root), `{"symbol":"SecretMarker"}`},
		{NewGetDiagnosticsTool(root), `{}`},
	} {
		t.Run(tc.tool.Name(), func(t *testing.T) {
			result, err := tc.tool.Execute(context.Background(), json.RawMessage(tc.raw))
			if err != nil || result.IsError {
				t.Fatalf("scan failed: result=%+v err=%v", result, err)
			}
			if strings.Contains(result.Content, ".env.go") || strings.Contains(result.Content, "syntheticsecretvalue") {
				t.Fatalf("secret source was scanned: %s", result.Content)
			}
		})
	}
	for _, name := range []string{".env", ".env.go", "nested/.env.ts"} {
		if isCodeIntelSource(name) {
			t.Errorf("secret path %q accepted as source", name)
		}
	}
}

func TestCodeIntelSecretAliasesAndRootConfinement(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=syntheticsecretvalue\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "safe.go"), []byte("package safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveCodeIntelPath(root, "safe.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveCodeIntelPath(root, "../outside.go"); err == nil {
		t.Fatal("outside path accepted")
	}
	alias := filepath.Join(root, "alias.go")
	if err := os.Symlink(filepath.Join(root, ".env"), alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := resolveCodeIntelPath(root, "alias.go"); err == nil || !strings.Contains(err.Error(), "dotenv secret files") {
		t.Fatalf("secret alias accepted: %v", err)
	}
	// A secret-looking lexical name must remain refused even if its target
	// is ordinary source content.
	if err := os.Symlink(filepath.Join(root, "safe.go"), filepath.Join(root, ".env.ts")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveCodeIntelPath(root, ".env.ts"); err == nil {
		t.Fatal("secret lexical alias accepted")
	}
}
