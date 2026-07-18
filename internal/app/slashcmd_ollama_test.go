package app

import (
	"strings"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/provider"
	"github.com/packetcode/packetcode/internal/provider/ollama"
)

func TestFormatOllamaModels(t *testing.T) {
	got := formatOllamaModels([]provider.Model{
		{ID: "qwen3-coder:30b", ContextWindow: 262144, SupportsTools: true},
		{ID: "codellama:13b", ContextWindow: 0, SupportsTools: false},
	})
	if !strings.Contains(got, "qwen3-coder:30b — ctx 262K, tools") {
		t.Fatalf("missing tool model line:\n%s", got)
	}
	if !strings.Contains(got, "codellama:13b — ctx ?, no tools") {
		t.Fatalf("missing no-tool/unknown-ctx line:\n%s", got)
	}
	if got := formatOllamaModels(nil); !strings.Contains(got, "no models installed") {
		t.Fatalf("empty case wrong: %s", got)
	}
}

func TestFormatOllamaPS_GPUPlacementAndTokS(t *testing.T) {
	loaded := []ollama.LoadedModel{
		{Name: "qwen3-coder:30b", Size: 20 << 30, SizeVRAM: 20 << 30},
		{Name: "big:70b", Size: 40 << 30, SizeVRAM: 24 << 30},
		{Name: "cpu:7b", Size: 5 << 30, SizeVRAM: 0},
	}
	stats := ollama.GenStats{OutputTokens: 500, EvalDuration: 5 * time.Second, PromptTokens: 100, PromptEvalDuration: 1 * time.Second, LoadDuration: time.Second}
	got := formatOllamaPS(loaded, stats)
	if !strings.Contains(got, "qwen3-coder:30b — 20.0 GB, GPU") {
		t.Fatalf("full-GPU line wrong:\n%s", got)
	}
	if !strings.Contains(got, "partial GPU") {
		t.Fatalf("partial line wrong:\n%s", got)
	}
	if !strings.Contains(got, "CPU only") {
		t.Fatalf("cpu line wrong:\n%s", got)
	}
	if !strings.Contains(got, "100 tok/s") {
		t.Fatalf("tok/s missing:\n%s", got)
	}
	if got := formatOllamaPS(nil, ollama.GenStats{}); !strings.Contains(got, "no models currently loaded") {
		t.Fatalf("empty ps case: %s", got)
	}
}

func TestFormatOllamaStatus_RecommendationsAndMemory(t *testing.T) {
	got := formatOllamaStatus(
		[]provider.Model{{ID: "a"}, {ID: "b"}},
		[]ollama.LoadedModel{{Name: "a", Size: 5 << 30, SizeVRAM: 5 << 30}},
		48<<30,
		ollama.Recommend(48<<30),
		ollama.GenStats{},
	)
	if !strings.Contains(got, "48.0 GB unified memory") {
		t.Fatalf("memory missing:\n%s", got)
	}
	if !strings.Contains(got, "Installed: 2 model(s)") || !strings.Contains(got, "loaded: a") {
		t.Fatalf("counts wrong:\n%s", got)
	}
	if !strings.Contains(got, "Recommended coding models") || !strings.Contains(got, "/ollama pull") {
		t.Fatalf("recommendations missing:\n%s", got)
	}
}

func TestHumanBytes(t *testing.T) {
	if humanBytes(20<<30) != "20.0 GB" {
		t.Fatalf("GB: %s", humanBytes(20<<30))
	}
	if humanBytes(512<<20) != "512 MB" {
		t.Fatalf("MB: %s", humanBytes(512<<20))
	}
}
