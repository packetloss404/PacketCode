package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandFileMentions(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main(){}"), 0o644)
	os.MkdirAll(filepath.Join(root, "internal"), 0o755)
	os.WriteFile(filepath.Join(root, "internal", "util.go"), []byte("package internal"), 0o644)

	prompt := "explain @main.go and @internal/util.go please"
	expanded, attached := expandFileMentions(prompt, root)

	if len(attached) != 2 || attached[0] != "internal/util.go" || attached[1] != "main.go" {
		t.Fatalf("attached wrong: %v", attached)
	}
	if !strings.Contains(expanded, "package main") || !strings.Contains(expanded, "package internal") {
		t.Fatalf("file contents not inlined:\n%s", expanded)
	}
	if !strings.HasSuffix(expanded, prompt) {
		t.Fatalf("original prompt not preserved at end")
	}
	if !strings.Contains(expanded, `<file path="main.go">`) {
		t.Fatalf("file tag missing:\n%s", expanded)
	}
}

func TestExpandFileMentions_TrailingPunctuationAndDedup(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("AAA"), 0o644)
	// Same file twice + trailing period.
	_, attached := expandFileMentions("look at @a.go. and again @a.go", root)
	if len(attached) != 1 || attached[0] != "a.go" {
		t.Fatalf("dedup/punctuation failed: %v", attached)
	}
}

func TestExpandFileMentions_IgnoresMissingAndEscapes(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "real.go"), []byte("X"), 0o644)
	prompt := "email me @someone and read @nope.go and @../../etc/passwd but also @real.go"
	expanded, attached := expandFileMentions(prompt, root)
	if len(attached) != 1 || attached[0] != "real.go" {
		t.Fatalf("should attach only real.go, got %v", attached)
	}
	// Non-file mentions stay as literal text.
	if !strings.Contains(expanded, "@someone") || !strings.Contains(expanded, "@nope.go") {
		t.Fatalf("literal mentions should be preserved")
	}
}

func TestExpandFileMentions_NoMentions(t *testing.T) {
	got, attached := expandFileMentions("just a normal prompt", t.TempDir())
	if got != "just a normal prompt" || attached != nil {
		t.Fatalf("no-mention case altered prompt: %q %v", got, attached)
	}
}

func TestExpandFileMentions_SkipsBinary(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "bin.dat"), []byte{1, 2, 0, 3, 4}, 0o644)
	_, attached := expandFileMentions("check @bin.dat", root)
	if len(attached) != 0 {
		t.Fatalf("binary file should not be attached: %v", attached)
	}
}
