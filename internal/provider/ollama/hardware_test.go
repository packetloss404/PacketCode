package ollama

import (
	"testing"
	"time"
)

func TestRecommend_FitByMemory(t *testing.T) {
	// 24 GB Mac: budget ~16 GB. gpt-oss:20b (14+4=18 > 16) does NOT fit;
	// nothing in the curated list fits comfortably at 24 GB — all flagged.
	recs := Recommend(24 * gib)
	if len(recs) != len(codingModels) {
		t.Fatalf("want %d recs, got %d", len(codingModels), len(recs))
	}
	for _, r := range recs {
		if r.Fits {
			t.Fatalf("nothing should comfortably fit 24GB budget, but %s did", r.Name)
		}
	}

	// 48 GB Mac: budget ~36 GB → the whole curated trio/quartet fits.
	recs48 := Recommend(48 * gib)
	fits := 0
	for _, r := range recs48 {
		if r.Fits {
			fits++
		}
	}
	if fits != len(codingModels) {
		t.Fatalf("expected all models to fit 48GB, got %d/%d", fits, len(codingModels))
	}

	// 64 GB: fitting models sort first (all fit here, so order is preserved best-first).
	if Recommend(64 * gib)[0].Name != "gpt-oss:20b" {
		t.Fatalf("best-first order not preserved")
	}
}

func TestRecommend_MixedFitSortsFittingFirst(t *testing.T) {
	// 36 GB: budget = 24 GB. gpt-oss(14) and devstral(14) fit (+4=18<=24);
	// qwen3-coder(19+4=23<=24) fits; granite(20+4=24<=24) fits — all fit at 36.
	// Use 30 GB to force a split: budget = 20 GB. Only 14GB models fit (18<=20).
	recs := Recommend(30 * gib)
	// First entries must be the fitting ones.
	if !recs[0].Fits {
		t.Fatalf("fitting model should sort first, got %+v", recs[0])
	}
	// The last entry must not fit (the 19–20 GB models exceed the 20 GB budget).
	if recs[len(recs)-1].Fits {
		t.Fatalf("largest model should not fit 30GB budget")
	}
}

func TestRecommend_UnknownMemory(t *testing.T) {
	for _, r := range Recommend(0) {
		if r.Fits || r.Reason != "memory unknown" {
			t.Fatalf("unknown memory should not claim fit: %+v", r)
		}
	}
}

func TestFitsInMemory(t *testing.T) {
	if fits, ok := FitsInMemory(14*gib, 48*gib); !ok || !fits {
		t.Fatalf("14GB model should fit 48GB Mac")
	}
	if fits, ok := FitsInMemory(40*gib, 24*gib); !ok || fits {
		t.Fatalf("40GB model should NOT fit 24GB Mac")
	}
	if _, ok := FitsInMemory(14*gib, 0); ok {
		t.Fatalf("unknown memory should report ok=false")
	}
}

func TestGenStats_TokensPerSec(t *testing.T) {
	s := GenStats{
		PromptTokens:       1000,
		OutputTokens:       500,
		PromptEvalDuration: 1 * time.Second,
		EvalDuration:       5 * time.Second,
		LoadDuration:       2 * time.Second,
	}
	if got := s.OutputTokensPerSec(); got != 100 {
		t.Fatalf("output tok/s = %v, want 100", got)
	}
	if got := s.PromptTokensPerSec(); got != 1000 {
		t.Fatalf("prompt tok/s = %v, want 1000", got)
	}
	if got := s.TimeToFirstToken(); got != 3*time.Second {
		t.Fatalf("TTFT = %v, want 3s", got)
	}
	// Zero-duration guards.
	if (GenStats{OutputTokens: 5}).OutputTokensPerSec() != 0 {
		t.Fatalf("zero duration must yield 0 tok/s")
	}
}
