package app

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Test 20: /spawn parse variants — happy path, flags, and malformed.
// ---------------------------------------------------------------------------

func TestParseSlashCommand_Spawn(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		cmd, args, ok := ParseSlashCommand("/spawn hello")
		if !ok || cmd != "spawn" {
			t.Fatalf("ParseSlashCommand = %q %v %v", cmd, args, ok)
		}
		if !reflect.DeepEqual(args, []string{"hello"}) {
			t.Fatalf("args = %v", args)
		}
		prov, model, write, prompt, err := ParseSpawnFlags(args)
		if err != nil {
			t.Fatalf("ParseSpawnFlags: %v", err)
		}
		if prov != "" || model != "" || write {
			t.Fatalf("expected no flags, got prov=%q model=%q write=%v", prov, model, write)
		}
		if prompt != "hello" {
			t.Fatalf("prompt = %q", prompt)
		}
	})

	t.Run("flags and multiword prompt", func(t *testing.T) {
		args := []string{"--provider", "gemini", "--model", "g-flash", "hello", "there"}
		prov, model, write, prompt, err := ParseSpawnFlags(args)
		if err != nil {
			t.Fatalf("ParseSpawnFlags: %v", err)
		}
		if prov != "gemini" || model != "g-flash" {
			t.Fatalf("prov=%q model=%q", prov, model)
		}
		if write {
			t.Fatalf("--write not present but allowWrite=true")
		}
		if prompt != "hello there" {
			t.Fatalf("prompt = %q", prompt)
		}
	})

	t.Run("--write", func(t *testing.T) {
		_, _, write, prompt, err := ParseSpawnFlags([]string{"--write", "hello"})
		if err != nil {
			t.Fatalf("ParseSpawnFlags: %v", err)
		}
		if !write {
			t.Fatalf("allowWrite = false")
		}
		if prompt != "hello" {
			t.Fatalf("prompt = %q", prompt)
		}
	})

	t.Run("flag without value", func(t *testing.T) {
		_, _, _, _, err := ParseSpawnFlags([]string{"--provider"})
		if err == nil {
			t.Fatalf("expected error for bare --provider")
		}
	})

	t.Run("empty prompt", func(t *testing.T) {
		_, _, _, _, err := ParseSpawnFlags([]string{"--write"})
		if err == nil {
			t.Fatalf("expected error for empty prompt")
		}
	})

	t.Run("flags order swapped", func(t *testing.T) {
		prov, model, write, prompt, err := ParseSpawnFlags(
			[]string{"--model", "m1", "--write", "--provider", "p1", "do", "stuff"})
		if err != nil {
			t.Fatalf("ParseSpawnFlags: %v", err)
		}
		if prov != "p1" || model != "m1" || !write || prompt != "do stuff" {
			t.Fatalf("unexpected: prov=%q model=%q write=%v prompt=%q", prov, model, write, prompt)
		}
	})

	t.Run("prompt preserves flag-like tokens once started", func(t *testing.T) {
		_, _, _, prompt, err := ParseSpawnFlags([]string{"hello", "--write"})
		if err != nil {
			t.Fatalf("ParseSpawnFlags: %v", err)
		}
		// After the first non-flag token the rest is prompt verbatim.
		if prompt != "hello --write" {
			t.Fatalf("prompt = %q", prompt)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 21: /jobs parse variants.
// ---------------------------------------------------------------------------

func TestParseSlashCommand_Jobs(t *testing.T) {
	t.Run("bare", func(t *testing.T) {
		cmd, args, ok := ParseSlashCommand("/jobs")
		if !ok || cmd != "jobs" || len(args) != 0 {
			t.Fatalf("parse = %q %v %v", cmd, args, ok)
		}
	})
	t.Run("with id", func(t *testing.T) {
		cmd, args, ok := ParseSlashCommand("/jobs 7f3a")
		if !ok || cmd != "jobs" {
			t.Fatalf("parse = %q %v %v", cmd, args, ok)
		}
		if !reflect.DeepEqual(args, []string{"7f3a"}) {
			t.Fatalf("args = %v", args)
		}
	})
	t.Run("whitespace tolerated", func(t *testing.T) {
		cmd, args, ok := ParseSlashCommand("   /jobs    abc  ")
		if !ok || cmd != "jobs" || !reflect.DeepEqual(args, []string{"abc"}) {
			t.Fatalf("parse = %q %v %v", cmd, args, ok)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 22: /cancel parse variants.
// ---------------------------------------------------------------------------

func TestParseSlashCommand_Cancel(t *testing.T) {
	t.Run("by id", func(t *testing.T) {
		cmd, args, ok := ParseSlashCommand("/cancel 7f3a")
		if !ok || cmd != "cancel" || !reflect.DeepEqual(args, []string{"7f3a"}) {
			t.Fatalf("parse = %q %v %v", cmd, args, ok)
		}
	})
	t.Run("all", func(t *testing.T) {
		cmd, args, ok := ParseSlashCommand("/cancel all")
		if !ok || cmd != "cancel" || !reflect.DeepEqual(args, []string{"all"}) {
			t.Fatalf("parse = %q %v %v", cmd, args, ok)
		}
	})
	t.Run("missing arg still parses; handler should reject", func(t *testing.T) {
		cmd, args, ok := ParseSlashCommand("/cancel")
		if !ok || cmd != "cancel" || len(args) != 0 {
			t.Fatalf("parse = %q %v %v", cmd, args, ok)
		}
	})
}

// ---------------------------------------------------------------------------
// Bonus: non-slash / unknown command.
// ---------------------------------------------------------------------------

func TestParseSlashCommand_NotACommand(t *testing.T) {
	for _, in := range []string{"hello", " ", "", "not/slash", "/unknown foo"} {
		if _, _, ok := ParseSlashCommand(in); ok {
			t.Fatalf("input %q: expected ok=false", in)
		}
	}
}
