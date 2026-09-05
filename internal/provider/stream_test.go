package provider

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/testwait"
)

// waitCancelled blocks until ctx is done or the deadline elapses, returning
// whether ctx was cancelled.
func waitCancelled(ctx context.Context, within time.Duration) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(within):
		return false
	}
}

// guardWindow scales a stall-guard timeout for the machine the test runs on.
//
// testwait.Factor rather than testwait.Timeout: this is a real-time window the
// guard itself measures, so Timeout's five-second floor would stretch a
// millisecond-scale test into a ten-second one. What these tests need from the
// scale is the ratio, not the floor -- scheduling jitter is roughly constant in
// absolute terms, so a wider window makes a starved ticker proportionally
// rarer.
func guardWindow(baseline time.Duration) time.Duration {
	return time.Duration(float64(baseline) * testwait.Factor())
}

// tickRecorder records the widest gap between consecutive Ticks, so a test can
// check its own premise rather than assume it.
//
// The tests below assert a negative: that the guard does *not* fire while Ticks
// keep arriving. That holds only while the ticking goroutine is actually
// scheduled inside the guard's window. When a loaded machine starves it past
// that window the guard fires -- and firing is then exactly correct. Without
// this the test reported the guard as broken for doing its job, which is the
// one failure a test must never produce.
type tickRecorder struct {
	mu    sync.Mutex
	last  time.Time
	worst time.Duration
}

func newTickRecorder() *tickRecorder { return &tickRecorder{last: time.Now()} }

// tick calls g.Tick outside the lock, so concurrent tickers stay concurrent and
// the race detector still sees Tick called from several goroutines at once.
func (r *tickRecorder) tick(g *StallGuard) {
	g.Tick()
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if gap := now.Sub(r.last); gap > r.worst {
		r.worst = gap
	}
	r.last = now
}

func (r *tickRecorder) widestGap() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.worst
}

// explainCancellation fails the test, unless the ticks it was relying on were
// themselves late enough to justify the guard firing.
func explainCancellation(t *testing.T, rec *tickRecorder, window time.Duration, err error) {
	t.Helper()
	if gap := rec.widestGap(); gap >= window {
		t.Skipf("machine starved the ticker: the widest gap between Ticks was %s, "+
			"past the %s guard window, so cancelling was the correct behaviour",
			gap.Round(time.Millisecond), window)
	}
	t.Fatalf("ctx was cancelled although every Tick landed inside the %s window "+
		"(widest gap %s): %v", window, rec.widestGap().Round(time.Millisecond), err)
}

func TestStallGuard_AbsentTickCancels(t *testing.T) {
	const timeout = 25 * time.Millisecond
	g, ctx := NewStallGuard(context.Background(), timeout)
	defer g.Stop()

	if ctx.Err() != nil {
		t.Fatalf("ctx should not be cancelled immediately: %v", ctx.Err())
	}

	// No Tick: ctx must cancel shortly after the timeout. Scaled because this
	// is a positive wait -- it returns the instant the guard fires, so slack
	// only ever costs a machine too busy to have fired yet.
	if !waitCancelled(ctx, testwait.Timeout(timeout+200*time.Millisecond)) {
		t.Fatalf("expected ctx to be cancelled after stall timeout with no Tick")
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", ctx.Err())
	}
}

func TestStallGuard_TickKeepsAlive(t *testing.T) {
	timeout := guardWindow(30 * time.Millisecond)
	interval := timeout / 3
	const ticks = 8

	g, ctx := NewStallGuard(context.Background(), timeout)
	defer g.Stop()

	// Tick several times at sub-timeout intervals so the guard never fires.
	rec := newTickRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < ticks; i++ {
			time.Sleep(interval)
			rec.tick(g)
		}
	}()

	// Across a span well beyond a single timeout, ctx must stay alive because
	// of regular Ticks.
	cancelled := waitCancelled(ctx, ticks*interval+interval/2)
	<-done
	if cancelled {
		explainCancellation(t, rec, timeout, ctx.Err())
	}

	// After Ticks stop, the guard should eventually fire. A positive wait, so a
	// generous budget costs a fast machine nothing: it returns the moment the
	// context is done.
	if !waitCancelled(ctx, testwait.Timeout(timeout+200*time.Millisecond)) {
		t.Fatalf("expected ctx to cancel after Ticks ceased")
	}
}

func TestStallGuard_StopPreventsCancellation(t *testing.T) {
	const timeout = 25 * time.Millisecond
	g, ctx := NewStallGuard(context.Background(), timeout)

	g.Stop()

	// After Stop, ctx is cancelled by Stop's teardown (context.Canceled), but
	// it must NOT be a delayed stall-driven cancellation — it is immediate.
	if ctx.Err() == nil {
		t.Fatalf("expected ctx cancelled by Stop teardown")
	}

	// Stop is idempotent: calling again must not panic.
	g.Stop()
	g.Stop()

	// Tick after Stop is a no-op and must not panic.
	g.Tick()
}

func TestStallGuard_StopBeforeFireNoLeak(t *testing.T) {
	const timeout = 20 * time.Millisecond
	before := runtime.NumGoroutine()

	for i := 0; i < 50; i++ {
		g, ctx := NewStallGuard(context.Background(), timeout)
		g.Tick()
		g.Stop()
		_ = ctx
	}

	// Allow any finished timers/goroutines to be reaped.
	time.Sleep(2 * timeout)
	runtime.GC()

	after := runtime.NumGoroutine()
	if after > before+5 {
		t.Fatalf("possible goroutine leak: before=%d after=%d", before, after)
	}
}

func TestStallGuard_DisabledZeroTimeoutReturnsParent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		timeout time.Duration
	}{
		{"zero", 0},
		{"negative", -10 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent, cancelParent := context.WithCancel(context.Background())
			defer cancelParent()

			g, ctx := NewStallGuard(parent, tc.timeout)
			defer g.Stop()

			// Must be the parent context unchanged.
			if ctx != parent {
				t.Fatalf("disabled guard must return the parent ctx unchanged")
			}

			// Tick/Stop are no-ops and must not panic or cancel.
			g.Tick()
			g.Tick()

			// The guard must never cancel on its own, even past any plausible
			// timeout window.
			if waitCancelled(ctx, 60*time.Millisecond) {
				t.Fatalf("disabled guard cancelled ctx; err=%v", ctx.Err())
			}

			// Stop must not panic and must be idempotent on a disabled guard.
			g.Stop()
			g.Stop()
		})
	}
}

func TestStallGuard_ConcurrentTicks(t *testing.T) {
	timeout := guardWindow(40 * time.Millisecond)
	g, ctx := NewStallGuard(context.Background(), timeout)
	defer g.Stop()

	rec := newTickRecorder()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					rec.tick(g)
					time.Sleep(timeout / 10)
				}
			}
		}()
	}

	// With many concurrent Ticks the ctx must stay alive (and the race
	// detector must find no data races on Tick).
	cancelled := waitCancelled(ctx, 3*timeout)
	close(stop)
	wg.Wait()
	if cancelled {
		explainCancellation(t, rec, timeout, ctx.Err())
	}
}

func TestConfiguredStallTimeout_DefaultAndRoundTrip(t *testing.T) {
	// Save and restore process-wide state so the test is self-contained.
	orig := func() time.Duration {
		configuredStallTimeoutMu.RLock()
		defer configuredStallTimeoutMu.RUnlock()
		return configuredStallTimeout
	}()
	t.Cleanup(func() { SetConfiguredStallTimeout(orig) })

	// Unset (zero) -> default.
	SetConfiguredStallTimeout(0)
	if got := ConfiguredStallTimeout(); got != DefaultStallTimeout {
		t.Fatalf("unset: got %v, want default %v", got, DefaultStallTimeout)
	}
	if DefaultStallTimeout != 60*time.Second {
		t.Fatalf("DefaultStallTimeout = %v, want 60s", DefaultStallTimeout)
	}

	// Positive value round-trips.
	SetConfiguredStallTimeout(5 * time.Second)
	if got := ConfiguredStallTimeout(); got != 5*time.Second {
		t.Fatalf("round-trip: got %v, want 5s", got)
	}

	// Negative value is stored verbatim (the "disable at call site" sentinel).
	SetConfiguredStallTimeout(-1)
	if got := ConfiguredStallTimeout(); got != -1 {
		t.Fatalf("negative: got %v, want -1", got)
	}

	// Reset back to default behavior.
	SetConfiguredStallTimeout(0)
	if got := ConfiguredStallTimeout(); got != DefaultStallTimeout {
		t.Fatalf("after reset: got %v, want default %v", got, DefaultStallTimeout)
	}
}
