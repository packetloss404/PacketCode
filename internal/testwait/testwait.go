// Package testwait gives asynchronous tests a deadline that can tell a slow
// machine apart from a broken one.
//
// The tests that needed this were not wrong about what they were waiting for.
// They were wrong about how long that is allowed to take: a job spawning a
// PowerShell hook had a two-second budget, which is ample on an idle laptop and
// not ample when the same laptop is also compiling. They failed in batches
// under load and passed in isolation, which is the one thing a test must never
// do — a result that depends on what else is running is not evidence about the
// code.
//
// The fix is not "sleep longer". Polling already returns the instant the
// condition holds, so a generous deadline costs a fast machine nothing; it is
// only spent when something is actually wrong. What the call sites lost by
// being tight was the ability to distinguish "never" from "not yet", and that
// is the distinction the whole test depends on.
//
// So a deadline here is a baseline, scaled by Factor. The baseline still says
// what the author expected; the scale says how much slack the machine gets.
package testwait

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// ScaleEnv overrides the multiplier, for CI runners slower than a developer
// machine.
const ScaleEnv = "PACKETCODE_TEST_TIMEOUT_SCALE"

// defaultScale multiplies every baseline.
//
// Ten, because the observed failures were tests missing a 2s budget while the
// machine was busy, not tests missing it by 10%. A regression takes this much
// longer to report, which is the trade: a test that fails in two seconds and
// lies is worth less than one that takes twenty and does not.
const defaultScale = 10

// minTimeout floors the scaled result. A call site with a very short baseline
// is usually expressing "this should be immediate", not "fail after 50ms".
//
// A var rather than a const so this package's own tests can exercise the
// give-up path without taking the floor to do it.
var minTimeout = 5 * time.Second

// PollInterval is short on purpose: the deadline is generous, so responsiveness
// has to come from how often the condition is checked, not from how soon the
// wait gives up.
const PollInterval = 5 * time.Millisecond

// Factor is the multiplier applied to every baseline.
//
// Read on each call rather than cached. This is test-only code where an
// os.Getenv costs nothing, and caching it behind a sync.Once made the setting
// impossible to exercise from a test — a knob whose behaviour cannot be
// asserted is how a suite ends up with a scale nobody has ever verified.
//
// A value that is absent, unparseable or not positive falls back to the
// default rather than being honoured: a typo in CI config must not silently
// shorten every deadline in the suite, which is the failure this package
// exists to prevent.
func Factor() float64 {
	if raw := os.Getenv(ScaleEnv); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
			return v
		}
	}
	return defaultScale
}

// Timeout scales a baseline into the budget a wait actually gets.
func Timeout(baseline time.Duration) time.Duration {
	d := time.Duration(float64(baseline) * Factor())
	if d < minTimeout {
		d = minTimeout
	}
	return d
}

// TB is the subset of testing.TB used here, so this package does not import
// testing into a non-test build.
type TB interface {
	Helper()
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
}

// For polls pred until it holds or the scaled deadline passes.
//
// When the condition holds later than its baseline, that is reported with
// Logf rather than a failure: the machine was slow, the test still proved what
// it set out to prove, and the slowness is worth seeing without being told the
// code is broken. That log line is the thing these tests could not previously
// say.
func For(t TB, baseline time.Duration, msg string, pred func() bool) {
	t.Helper()
	forMsg(t, baseline, func() string { return msg }, pred)
}

// forMsg is For with the message built at failure time, so a caller can report
// the value it actually observed rather than only what it wanted.
func forMsg(t TB, baseline time.Duration, msg func() string, pred func() bool) {
	t.Helper()
	budget := Timeout(baseline)
	start := time.Now()
	deadline := start.Add(budget)
	for {
		if pred() {
			if elapsed := time.Since(start); elapsed > baseline {
				t.Logf("slow wait: %s took %s, over its %s baseline (budget %s) — "+
					"machine load, not a failure", msg(), elapsed.Round(time.Millisecond), baseline, budget)
			}
			return
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(PollInterval)
	}
	t.Fatalf("timed out after %s (baseline %s x%g): %s", budget, baseline, Factor(), msg())
}

// ForEq is For for an integer that must reach want.
//
// The failure names the value it actually reached: "got 1, want 0" localises a
// failure that "condition was false" does not, and the whole point of a wait
// that no longer fails spuriously is that the failures left are worth reading.
func ForEq(t TB, baseline time.Duration, msg string, fn func() int, want int) {
	t.Helper()
	last := want - 1 // a value fn has not returned, in case it is never called
	forMsg(t, baseline,
		func() string { return fmt.Sprintf("%s: got %d, want %d", msg, last, want) },
		func() bool {
			last = fn()
			return last == want
		})
}
