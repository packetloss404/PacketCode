package cost

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedPricing returns $2 in / $8 out per 1M for everything. Tests can
// validate dollar math against round numbers.
func fixedPricing(string, string) (float64, float64) { return 2.00, 8.00 }

func TestTally_LoadMissingReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tally.json")
	tally, err := Load(path)
	require.NoError(t, err)
	assert.NotNil(t, tally.Sessions)
	assert.Empty(t, tally.Sessions)
	assert.Greater(t, tally.StartTime, int64(0))
}

func TestTally_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tally.json")
	t1, _ := Load(path)
	t1.Sessions["abc"] = SessionCost{Input: 100, Output: 200, Provider: "openai", Model: "gpt-4.1"}
	require.NoError(t, t1.Save(path))

	t2, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 100, t2.Sessions["abc"].Input)
	assert.Equal(t, "openai", t2.Sessions["abc"].Provider)
}

func TestTracker_HighWaterMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tally.json")
	tr, err := NewTracker(path, fixedPricing)
	require.NoError(t, err)

	require.NoError(t, tr.RecordUsage("s1", "openai", "gpt-4.1", 1000, 500))
	require.NoError(t, tr.RecordUsage("s1", "openai", "gpt-4.1", 800, 400))  // smaller — should be ignored
	require.NoError(t, tr.RecordUsage("s1", "openai", "gpt-4.1", 1200, 600)) // larger — should win

	in, out := tr.SessionTokens("s1")
	assert.Equal(t, 1200, in)
	assert.Equal(t, 600, out)
}

func TestTracker_SessionCost(t *testing.T) {
	tr, _ := NewTracker(filepath.Join(t.TempDir(), "tally.json"), fixedPricing)
	require.NoError(t, tr.RecordUsage("s1", "openai", "gpt-4.1", 1_000_000, 500_000))

	// 1M * $2/M + 0.5M * $8/M = $6.00
	assert.InDelta(t, 6.00, tr.SessionCost("s1"), 1e-9)
}

func TestTracker_TotalCostAggregates(t *testing.T) {
	tr, _ := NewTracker(filepath.Join(t.TempDir(), "tally.json"), fixedPricing)
	require.NoError(t, tr.RecordUsage("s1", "openai", "gpt-4.1", 1_000_000, 0))
	require.NoError(t, tr.RecordUsage("s2", "openai", "gpt-4.1", 0, 1_000_000))
	// $2 + $8 = $10
	assert.InDelta(t, 10.00, tr.TotalCost(), 1e-9)
}

func TestTracker_Reset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tally.json")
	tr, _ := NewTracker(path, fixedPricing)
	require.NoError(t, tr.RecordUsage("s1", "openai", "gpt-4.1", 100, 50))
	require.NoError(t, tr.Reset())
	in, out := tr.SessionTokens("s1")
	assert.Zero(t, in)
	assert.Zero(t, out)
}

func TestTracker_Breakdown(t *testing.T) {
	tr, _ := NewTracker(filepath.Join(t.TempDir(), "tally.json"), fixedPricing)
	require.NoError(t, tr.RecordUsage("s1", "openai", "gpt-4.1", 1000, 500))
	require.NoError(t, tr.RecordUsage("s2", "gemini", "gemini-2.5-pro", 2000, 1000))

	rows := tr.Breakdown()
	require.Len(t, rows, 2)
}

func TestTracker_SessionCostsForIDs(t *testing.T) {
	tr, _ := NewTracker(filepath.Join(t.TempDir(), "tally.json"), fixedPricing)
	require.NoError(t, tr.RecordUsage("s1", "openai", "gpt-4.1", 1_000_000, 0)) // $2
	require.NoError(t, tr.RecordUsage("s2", "openai", "gpt-4.1", 0, 1_000_000)) // $8
	require.NoError(t, tr.RecordUsage("s3", "openai", "gpt-4.1", 500_000, 0))   // $1

	// Two known ids → sum.
	got := tr.SessionCostsForIDs([]string{"s1", "s2"})
	assert.InDelta(t, 10.00, got, 1e-9)

	// Including an unknown id contributes 0, no error.
	got = tr.SessionCostsForIDs([]string{"s1", "missing", "s3"})
	assert.InDelta(t, 3.00, got, 1e-9)

	// Empty input → 0.
	assert.InDelta(t, 0.0, tr.SessionCostsForIDs(nil), 1e-9)
}

// TestTracker_CacheCountsAreNotPriced pins the pricing invariant: cached
// tokens are already inside Input, so priced() must derive cost from Input
// alone. Charging for the subsets again would overstate every cached
// session.
func TestTracker_CacheCountsAreNotPriced(t *testing.T) {
	tr, _ := NewTracker(filepath.Join(t.TempDir(), "tally.json"), fixedPricing)
	require.NoError(t, tr.RecordUsageWithCache("s1", "anthropic", "claude", 1_000_000, 500_000, 200_000, 700_000))

	// Same $6.00 as the cache-free case: 1M * $2/M + 0.5M * $8/M.
	assert.InDelta(t, 6.00, tr.SessionCost("s1"), 1e-9)

	e := tr.Breakdown()
	require.Len(t, e, 1)
	assert.Equal(t, 200_000, e[0].CacheCreation)
	assert.Equal(t, 700_000, e[0].CacheRead)
	assert.LessOrEqual(t, e[0].CacheCreation+e[0].CacheRead, e[0].Input)
}

// TestTracker_CacheHighWaterMark confirms the cache counters follow the same
// only-ever-increase rule as input and output, so an out-of-order or replayed
// usage report cannot walk them backwards.
func TestTracker_CacheHighWaterMark(t *testing.T) {
	tr, _ := NewTracker(filepath.Join(t.TempDir(), "tally.json"), fixedPricing)
	require.NoError(t, tr.RecordUsageWithCache("s1", "anthropic", "claude", 1000, 500, 100, 800))
	require.NoError(t, tr.RecordUsageWithCache("s1", "anthropic", "claude", 800, 400, 50, 600))
	require.NoError(t, tr.RecordUsageWithCache("s1", "anthropic", "claude", 1200, 600, 150, 900))

	e := tr.Breakdown()
	require.Len(t, e, 1)
	assert.Equal(t, 1200, e[0].Input)
	assert.Equal(t, 150, e[0].CacheCreation)
	assert.Equal(t, 900, e[0].CacheRead)
}

// TestTracker_RecordUsagePreservesCacheCounts covers the narrow-signature
// caller: it has no cache figures to offer, and passing implicit zeros would
// be indistinguishable from a provider reporting no cache hits. It must
// leave whatever cache counts are already recorded alone.
func TestTracker_RecordUsagePreservesCacheCounts(t *testing.T) {
	tr, _ := NewTracker(filepath.Join(t.TempDir(), "tally.json"), fixedPricing)
	require.NoError(t, tr.RecordUsageWithCache("s1", "anthropic", "claude", 1000, 500, 100, 800))
	require.NoError(t, tr.RecordUsage("s1", "anthropic", "claude", 2000, 900))

	e := tr.Breakdown()
	require.Len(t, e, 1)
	assert.Equal(t, 2000, e[0].Input)
	assert.Equal(t, 100, e[0].CacheCreation)
	assert.Equal(t, 800, e[0].CacheRead)
}

// TestTally_LoadWithoutCacheFields confirms tallies written before cache
// plumbing still decode: the absent fields become zero and the existing
// counts survive untouched.
func TestTally_LoadWithoutCacheFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tally.json")
	legacy := `{"sessions":{"s1":{"input":1000,"output":500,"provider":"openai","model":"gpt-4.1"}},"start_time":1736942400}`
	require.NoError(t, os.WriteFile(path, []byte(legacy), 0o600))

	tr, err := NewTracker(path, fixedPricing)
	require.NoError(t, err)
	in, out := tr.SessionTokens("s1")
	assert.Equal(t, 1000, in)
	assert.Equal(t, 500, out)

	e := tr.Breakdown()
	require.Len(t, e, 1)
	assert.Zero(t, e[0].CacheCreation)
	assert.Zero(t, e[0].CacheRead)

	// Re-saving a cache-free record must not introduce the new keys, so an
	// older build reading the file back sees exactly what it wrote.
	require.NoError(t, tr.RecordUsage("s1", "openai", "gpt-4.1", 1100, 550))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "cache_creation")
	assert.NotContains(t, string(raw), "cache_read")
}
