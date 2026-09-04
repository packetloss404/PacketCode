package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/mcp"
)

func testRuntimeConfig(t *testing.T) (*config.Config, packetRuntimeConfig) {
	t.Helper()
	root := t.TempDir()
	state := t.TempDir()
	cfg := config.Default()
	cfg.Default.Provider = "ollama"
	cfg.Default.Model = "llama-test"
	cfg.Providers["ollama"] = config.ProviderConfig{DefaultModel: "llama-test"}
	cfg.Providers["codex"] = config.ProviderConfig{DefaultModel: "codex-test"}
	return cfg, packetRuntimeConfig{
		Config:        cfg,
		Root:          root,
		SessionsDir:   filepath.Join(state, "sessions"),
		BackupsDir:    filepath.Join(state, "backups"),
		MCPServers:    []mcp.ServerConfig{},
		MCPServersSet: true,
		SystemPrompt:  "test prompt",
		DisableHooks:  true,
	}
}

func TestBuildPacketRuntimeCreatesCompleteLocalSession(t *testing.T) {
	_, opts := testRuntimeConfig(t)
	rt, err := buildPacketRuntime(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	if rt.Provider != "ollama" || rt.Model != "llama-test" {
		t.Fatalf("selected %s/%s", rt.Provider, rt.Model)
	}
	if rt.SessionID == "" || rt.CurrentSession() == nil {
		t.Fatal("runtime did not expose its persisted session")
	}
	for _, name := range []string{"read_file", "write_file", "patch_file", "execute_command", "todo_write", "skill", "fetch"} {
		if _, ok := rt.Tools.Get(name); !ok {
			t.Errorf("native tool %q is missing", name)
		}
	}
	if rt.NewAgent(nil) == nil {
		t.Fatal("NewAgent returned nil")
	}
}

func TestBuildPacketRuntimeResumeAndExplicitOverrides(t *testing.T) {
	cfg, opts := testRuntimeConfig(t)
	first, err := buildPacketRuntime(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := first.SessionID
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	// Changed defaults must not rewrite the provider/model stored with a
	// resumed conversation.
	cfg.Default.Provider = "codex"
	cfg.Default.Model = "new-default"
	opts.ResumeID = sessionID
	resumed, err := buildPacketRuntime(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.Resumed || resumed.Provider != "ollama" || resumed.Model != "llama-test" {
		t.Fatalf("resume selected %s/%s (resumed=%v)", resumed.Provider, resumed.Model, resumed.Resumed)
	}
	if err := resumed.Close(); err != nil {
		t.Fatal(err)
	}

	// An explicit provider override invalidates the old provider's model and
	// falls back to the new provider's configured default.
	opts.ProviderOverride = "codex"
	overridden, err := buildPacketRuntime(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = overridden.Close() })
	if overridden.Provider != "codex" || overridden.Model != "codex-test" {
		t.Fatalf("override selected %s/%s", overridden.Provider, overridden.Model)
	}
}

func TestPacketRuntimeCloseIsReverseOrderAndIdempotent(t *testing.T) {
	_, opts := testRuntimeConfig(t)
	rt, err := buildPacketRuntime(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("cleanup failed")
	var order []int
	rt.AddCleanup(func() error { order = append(order, 1); return nil })
	rt.AddCleanup(func() error { order = append(order, 2); return wantErr })

	if err := rt.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("Close error = %v", err)
	}
	if !reflect.DeepEqual(order, []int{2, 1}) {
		t.Fatalf("cleanup order = %v", order)
	}
	if err := rt.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
	if !reflect.DeepEqual(order, []int{2, 1}) {
		t.Fatalf("cleanup ran twice: %v", order)
	}
}
