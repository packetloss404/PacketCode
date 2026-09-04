package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMedian(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []int64
		want int64
	}{
		{"empty", nil, 0},
		{"odd", []int64{9, 1, 5}, 5},
		{"even", []int64{8, 2, 4, 6}, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := median(tc.in); got != tc.want {
				t.Fatalf("median(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestSummarizeRequiresMatchedPairs(t *testing.T) {
	samples := []sample{
		{Pair: 1, Path: "run", OK: true, WallMS: 100, ReportedMS: 90, Usage: usage{Input: 10}, ProviderCalls: 2, ToolCalls: 1},
		{Pair: 1, Path: "acp", OK: true, WallMS: 120, InitializeMS: 5, SessionNewMS: 10, PromptMS: 105, Usage: usage{Input: 10}, ProviderCalls: 2, ToolCalls: 1},
		{Pair: 2, Path: "run", OK: true, WallMS: 200, Usage: usage{Input: 20}, ProviderCalls: 2, ToolCalls: 1},
		{Pair: 2, Path: "acp", OK: true, WallMS: 260, Usage: usage{Input: 21}, ProviderCalls: 3, ToolCalls: 1},
	}
	got := summarize(samples, 2)
	if got.ComparablePairs != 1 || got.MedianPairedDelta != 20 {
		t.Fatalf("unexpected pair summary: %+v", got)
	}
	if got.UsageMatchedByPair || got.CallsMatchedByPair {
		t.Fatalf("mismatches were not reported: %+v", got)
	}
}

func TestRemoveIsolatedHomeIsBoundedToOwnedTempDirectory(t *testing.T) {
	dir, err := os.MkdirTemp("", "packetcode-benchrun-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "owned"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeIsolatedHome(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("isolated home still exists: %v", err)
	}
	if err := removeIsolatedHome(filepath.Join("not-temp", "packetcode-benchrun-fake")); err == nil {
		t.Fatal("unexpected path was accepted")
	}
}
