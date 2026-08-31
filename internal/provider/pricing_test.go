package provider

import (
	"math"
	"testing"
)

func closeTo(t *testing.T, got, want float64, what string) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s = %.10f, want %.10f", what, got, want)
	}
}

// The bug, stated as the number a user saw.
//
// A six-task benchmark ran 345,986 input tokens of which 322,651 were served
// from cache, and packetcode displayed $2.03. Charging every cached token at
// the full rate is where that came from; at a tenth, the same usage costs about
// a sixth of it. The point of this test is that the ratio stays roughly six --
// not that a particular dollar figure is right, which depends on rates nobody
// here controls.
func TestEstimateCost_CachedInputIsNotBilledAsFresh(t *testing.T) {
	const (
		input     = 345986
		output    = 10748
		cacheRead = 322651
		inRate    = 5.0
		outRate   = 15.0
	)

	naive := float64(input)*inRate/1_000_000 + float64(output)*outRate/1_000_000
	fixed := EstimateCost(input, output, cacheRead, 0, inRate, outRate,
		CacheReadMultiplier, CacheWriteMultiplier)

	if fixed >= naive {
		t.Fatalf("cached input was not discounted: fixed %.4f >= naive %.4f", fixed, naive)
	}
	// The input component alone should fall by roughly 6x at 93% cache hits.
	naiveIn := float64(input) * inRate / 1_000_000
	fixedIn := fixed - float64(output)*outRate/1_000_000
	ratio := naiveIn / fixedIn
	if ratio < 5 || ratio > 7 {
		t.Fatalf("input cost fell by %.2fx, expected about 6x", ratio)
	}
}

// A session with no cache hits must cost exactly what it always did. The fix
// has to be invisible to anyone it does not apply to.
func TestEstimateCost_NoCacheMatchesThePlainFormula(t *testing.T) {
	got := EstimateCost(1000, 500, 0, 0, 2.0, 8.0, CacheReadMultiplier, CacheWriteMultiplier)
	want := 1000*2.0/1_000_000 + 500*8.0/1_000_000
	closeTo(t, got, want, "cost with no cache")
}

// Cache counts are subsets of input, never addends. Treating them as additional
// tokens would bill every cached prompt twice -- the opposite error, and just
// as wrong.
func TestEstimateCost_CacheCountsAreSubsetsOfInput(t *testing.T) {
	// 1000 input of which 800 cached, at a tenth, is 200 + 80 = 280 effective.
	got := EstimateCost(1000, 0, 800, 0, 1.0, 0, 0.10, 1.00)
	closeTo(t, got, 280.0/1_000_000, "cost with 80% cache")
}

// Anthropic charges a premium for writing the cache, which is why the
// multipliers are separable at all.
func TestEstimateCost_WriteMultiplierIsAppliedSeparately(t *testing.T) {
	// 1000 input: 600 read at 0.10, 200 written at 1.25, 200 fresh at 1.0.
	// -> 60 + 250 + 200 = 510 effective tokens.
	got := EstimateCost(1000, 0, 600, 200, 1.0, 0, 0.10, 1.25)
	closeTo(t, got, 510.0/1_000_000, "cost with a write premium")
}

// A provider reporting more cached tokens than input must not produce a
// negative bill. A wrong number that looks impossible is easier to catch than
// a wrong number that looks fine.
func TestEstimateCost_NeverGoesNegative(t *testing.T) {
	for _, tc := range []struct{ input, read, create int }{
		{100, 500, 0},
		{100, 0, 500},
		{100, 400, 400},
		{100, -5, -5},
	} {
		got := EstimateCost(tc.input, 0, tc.read, tc.create, 10.0, 0, 0.10, 1.25)
		if got < 0 {
			t.Fatalf("input=%d read=%d create=%d gave a negative cost %.10f",
				tc.input, tc.read, tc.create, got)
		}
	}
}

type cacheRatedProvider struct {
	Provider
	read, write float64
}

func (f cacheRatedProvider) CacheMultipliers(string) (float64, float64) { return f.read, f.write }

type uncachedProvider struct{ Provider }

func TestCacheMultipliersFor_DefaultsAndOverrides(t *testing.T) {
	read, write := CacheMultipliersFor(uncachedProvider{}, "m")
	if read != CacheReadMultiplier || write != CacheWriteMultiplier {
		t.Fatalf("a provider that says nothing got %v/%v, want the defaults", read, write)
	}

	read, write = CacheMultipliersFor(cacheRatedProvider{read: 0.25, write: 1.5}, "m")
	if read != 0.25 || write != 1.5 {
		t.Fatalf("a provider's own rates were ignored: %v/%v", read, write)
	}

	// A negative multiplier would subtract from the bill, so it is refused
	// rather than honoured.
	read, write = CacheMultipliersFor(cacheRatedProvider{read: -1, write: -1}, "m")
	if read != CacheReadMultiplier || write != CacheWriteMultiplier {
		t.Fatalf("negative multipliers were honoured: %v/%v", read, write)
	}
}
