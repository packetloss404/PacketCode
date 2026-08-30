package testwait

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeTB records what a wait would have told the test framework.
type fakeTB struct {
	fatals []string
	logs   []string
}

func (f *fakeTB) Helper() {}
func (f *fakeTB) Fatalf(format string, args ...any) {
	f.fatals = append(f.fatals, fmt.Sprintf(format, args...))
	panic(sentinel{})
}
func (f *fakeTB) Logf(format string, args ...any) {
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}

type sentinel struct{}

// run invokes fn, absorbing the panic a Fatalf raises so the assertions can
// inspect what was reported.
func run(f *fakeTB, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(sentinel); !ok {
				panic(r)
			}
		}
	}()
	fn()
}

func TestFor_ReturnsImmediatelyWhenAlreadyTrue(t *testing.T) {
	f := &fakeTB{}
	start := time.Now()
	run(f, func() { For(f, time.Second, "already true", func() bool { return true }) })
	if len(f.fatals) != 0 {
		t.Fatalf("unexpected failure: %v", f.fatals)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("a satisfied condition waited %s; polling must return at once", elapsed)
	}
	if len(f.logs) != 0 {
		t.Fatalf("a prompt success should not report slowness: %v", f.logs)
	}
}

// The point of the package: a condition that takes longer than its baseline
// still passes, and says so, instead of failing because the machine was busy.
func TestFor_SlowConditionPassesAndIsReported(t *testing.T) {
	f := &fakeTB{}
	start := time.Now()
	done := start.Add(120 * time.Millisecond)
	run(f, func() {
		For(f, 40*time.Millisecond, "slow but fine", func() bool { return time.Now().After(done) })
	})
	if len(f.fatals) != 0 {
		t.Fatalf("a slow-but-satisfied condition failed: %v", f.fatals)
	}
	if len(f.logs) != 1 || !strings.Contains(f.logs[0], "slow wait") {
		t.Fatalf("slowness was not reported: %v", f.logs)
	}
	if !strings.Contains(f.logs[0], "not a failure") {
		t.Fatalf("the log does not say the test still passed: %q", f.logs[0])
	}
}

// A condition that never holds must still fail — the deadline is generous, not
// absent.
func TestFor_NeverTrueStillFails(t *testing.T) {
	// Lower the floor so the give-up path can be exercised in milliseconds
	// rather than seconds; the floor itself is asserted separately.
	old := minTimeout
	minTimeout = 20 * time.Millisecond
	defer func() { minTimeout = old }()
	f := &fakeTB{}
	run(f, func() { For(f, time.Millisecond, "never", func() bool { return false }) })
	if len(f.fatals) != 1 {
		t.Fatalf("a condition that never holds did not fail: %v", f.fatals)
	}
	if !strings.Contains(f.fatals[0], "never") {
		t.Fatalf("the failure does not name what was awaited: %q", f.fatals[0])
	}
	if !strings.Contains(f.fatals[0], "baseline") {
		t.Fatalf("the failure does not distinguish budget from baseline: %q", f.fatals[0])
	}
}

func TestTimeout_ScalesAndFloors(t *testing.T) {
	t.Setenv(ScaleEnv, "2")
	// Below the floor even after scaling.
	if got := Timeout(time.Millisecond); got != minTimeout {
		t.Fatalf("Timeout(1ms) = %s, want the %s floor", got, minTimeout)
	}
	// Above the floor: the scale applies.
	if got := Timeout(10 * time.Second); got != 20*time.Second {
		t.Fatalf("Timeout(10s) at scale 2 = %s, want 20s", got)
	}
}

func TestFactor_DefaultsWhenUnsetOrInvalid(t *testing.T) {
	for _, raw := range []string{"", "not-a-number", "0", "-3"} {
		t.Setenv(ScaleEnv, raw)
		if got := Factor(); got != defaultScale {
			t.Fatalf("%q gave factor %g, want the default %g — a bad value must not "+
				"silently shorten every deadline in the suite", raw, got, float64(defaultScale))
		}
	}
}

// A ForEq failure has to name what it actually saw. "condition was false"
// sends the reader back to the test to find out what the number was.
func TestForEq_FailureReportsTheObservedValue(t *testing.T) {
	old := minTimeout
	minTimeout = 20 * time.Millisecond
	defer func() { minTimeout = old }()

	f := &fakeTB{}
	run(f, func() {
		ForEq(f, time.Millisecond, "queued jobs", func() int { return 3 }, 0)
	})
	if len(f.fatals) != 1 {
		t.Fatalf("expected one failure, got %v", f.fatals)
	}
	if !strings.Contains(f.fatals[0], "got 3, want 0") {
		t.Fatalf("failure does not report the observed value: %q", f.fatals[0])
	}
}

func TestForEq_PassesWhenTheValueArrives(t *testing.T) {
	f := &fakeTB{}
	n := 0
	run(f, func() {
		ForEq(f, time.Second, "counter", func() int { n++; return n }, 3)
	})
	if len(f.fatals) != 0 {
		t.Fatalf("unexpected failure: %v", f.fatals)
	}
}
