package ollama

import (
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// This file adds local-hardware awareness so packetcode can recommend models
// that actually fit — critical on Apple Silicon, where models that don't fit
// the GPU budget silently spill to the CPU and run several times slower.

const gib = int64(1) << 30

// SystemMemoryBytes returns total physical RAM. ok is false when it can't be
// determined (unsupported OS or probe failure). On Apple Silicon this is the
// unified memory shared between CPU and GPU.
func SystemMemoryBytes() (int64, bool) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0, false
		}
		n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err != nil || n <= 0 {
			return 0, false
		}
		return n, true
	case "linux":
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0, false
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					kb, err := strconv.ParseInt(fields[1], 10, 64)
					if err == nil && kb > 0 {
						return kb * 1024, true
					}
				}
			}
		}
		return 0, false
	default:
		return 0, false
	}
}

// gpuBudgetBytes estimates memory usable for model weights + KV cache. On Apple
// Silicon macOS caps GPU-usable unified memory at roughly two-thirds of RAM at
// or below 36 GB and about three-quarters above it (Metal's
// recommendedMaxWorkingSetSize). We reserve nothing extra here; the fit check
// applies its own headroom.
func gpuBudgetBytes(ramBytes int64) int64 {
	if ramBytes <= 0 {
		return 0
	}
	if ramBytes > 36*gib {
		return ramBytes * 3 / 4
	}
	return ramBytes * 2 / 3
}

// CodingModel is a curated local model suited to agentic coding (tool calling +
// solid code quality). ApproxResidentBytes is the Q4_K_M-class weight footprint;
// real usage adds KV cache that grows with context.
type CodingModel struct {
	Name                string // the `ollama pull` name
	Label               string
	ApproxResidentBytes int64
	Tools               bool
	Note                string
}

// codingModels is ordered best-first for an agentic coding agent, per the
// mid-2026 research: MoE models with ~3B active params give near-30B quality at
// interactive speed. All support native tool calling.
var codingModels = []CodingModel{
	{"gpt-oss:20b", "gpt-oss 20B (MoE)", 14 * gib, true, "excellent tool-calling; fits 24 GB (MXFP4)"},
	{"qwen3-coder:30b", "Qwen3-Coder 30B-A3B", 19 * gib, true, "top agentic coder; needs ~24 GB+"},
	{"devstral:24b", "Devstral Small 2 24B", 14 * gib, true, "agent-first, dense"},
	{"granite4:32b", "Granite 4.0 Small 32B-A9B", 20 * gib, true, "strong function-calling scores"},
}

// ModelRec is a recommendation annotated for the current machine.
type ModelRec struct {
	CodingModel
	Fits   bool   // fits the GPU budget with headroom (won't spill to CPU)
	Reason string // human-readable fit note
}

// fitHeadroomBytes is reserved on top of the weights for KV cache and OS use
// when deciding whether a model comfortably fits the GPU budget.
const fitHeadroomBytes = 4 * gib

// Recommend ranks the curated coding models for a machine with ramBytes of
// (unified) memory, best-first, annotating whether each comfortably fits the
// GPU budget. When ramBytes is 0 (unknown), everything is returned unannotated.
func Recommend(ramBytes int64) []ModelRec {
	budget := gpuBudgetBytes(ramBytes)
	out := make([]ModelRec, 0, len(codingModels))
	for _, m := range codingModels {
		rec := ModelRec{CodingModel: m}
		switch {
		case budget == 0:
			rec.Reason = "memory unknown"
		case m.ApproxResidentBytes+fitHeadroomBytes <= budget:
			rec.Fits = true
			rec.Reason = "fits comfortably"
		default:
			rec.Reason = "may spill to CPU (slow) — needs more unified memory"
		}
		out = append(out, rec)
	}
	// Stable sort: fitting models first, otherwise keep curated (best-first) order.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Fits && !out[j].Fits
	})
	return out
}

// FitsInMemory reports whether a model of approxResidentBytes comfortably fits
// the GPU budget for a machine with ramBytes. Used to warn before loading a
// model that would offload to CPU. ok is false when memory is unknown.
func FitsInMemory(approxResidentBytes, ramBytes int64) (fits bool, ok bool) {
	budget := gpuBudgetBytes(ramBytes)
	if budget == 0 {
		return false, false
	}
	return approxResidentBytes+fitHeadroomBytes <= budget, true
}
