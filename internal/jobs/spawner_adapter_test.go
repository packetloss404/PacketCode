package jobs

import (
	"testing"
	"time"
)

// A cyclic ParentJobID chain must not hang the manager. Spawn never mints a
// cycle, but these records are read back from disk, which the persistence
// layer already treats as potentially hostile -- and the walk runs under the
// manager's read lock, so spinning here freezes every caller, not one.
func TestIsDescendantLocked_TerminatesOnCyclicParentChain(t *testing.T) {
	mgr, _ := newTestManager(t, &scriptedProvider{turns: scriptedHello()})
	mgr.mu.Lock()
	mgr.jobs["aaa"] = &Job{ID: "aaa", ParentJobID: "bbb"}
	mgr.jobs["bbb"] = &Job{ID: "bbb", ParentJobID: "aaa"}
	mgr.mu.Unlock()

	a := &spawnerAdapter{m: mgr}
	done := make(chan bool, 1)
	go func() {
		mgr.mu.RLock()
		defer mgr.mu.RUnlock()
		done <- a.isDescendantLocked("aaa", "not-in-the-cycle")
	}()

	select {
	case got := <-done:
		if got {
			t.Fatal("a job outside the cycle must not be reported as an ancestor")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("isDescendantLocked spun on a cyclic parent chain")
	}
}
