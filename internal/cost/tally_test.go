package cost

import (
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
	require.NoError(t, tr.RecordUsage("s1", "openai", "gpt-4.1", 800, 400)) // smaller — should be ignored
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
