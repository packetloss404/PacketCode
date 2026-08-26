package procrun

import (
	"os/exec"
	"sync"
	"time"
)

// KillMethod names the mechanism that actually performed a teardown. Which
// tier fired is itself evidence: a job object contains a tree by construction,
// while a fallback that enumerates processes can only report what it managed
// to reach.
type KillMethod string

const (
	// KillMethodNone means there was no live process to tear down.
	KillMethodNone KillMethod = "none"
	// KillMethodJobObject is the Windows job object, which contains the whole
	// tree by construction.
	KillMethodJobObject KillMethod = "job-object"
	// KillMethodProcessGroup is a POSIX signal to the negative pgid.
	KillMethodProcessGroup KillMethod = "process-group"
	// KillMethodTreeWalk enumerated descendants and terminated them
	// individually. It reaches only what the snapshot could see.
	KillMethodTreeWalk KillMethod = "tree-walk"
	// KillMethodTaskkill is the Windows last resort, which reports nothing
	// useful about what it did or did not reach.
	KillMethodTaskkill KillMethod = "taskkill"
)

// KillOutcome is the evidence a teardown leaves behind.
//
// It exists because "we asked the process to die" and "the process is dead"
// are different claims, and the code that reports to a user must not conflate
// them. Confirmed is deliberately conservative: false never means something
// definitely survived, only that nothing proved otherwise. Callers must report
// an unconfirmed teardown as unconfirmed rather than rounding it up.
type KillOutcome struct {
	// Method is the mechanism that ran.
	Method KillMethod
	// Confirmed reports that no process from the tree can still be running.
	// Only a mechanism that contains the tree, or a verified-empty group, may
	// set this.
	Confirmed bool
	// Survivors are pids observed still alive after the attempt. Best effort:
	// an empty slice with Confirmed false means "not enumerable", not "none".
	Survivors []int
	// Reason explains an unconfirmed outcome in terms a user can act on.
	Reason string
}

// Unconfirmed reports whether the caller must describe this teardown as
// possibly incomplete.
func (o KillOutcome) Unconfirmed() bool { return !o.Confirmed }

func ConfigureTreeCancel(cmd *exec.Cmd) {
	_ = ConfigureTreeCancelRecorder(cmd)
}

// ConfigureTreeCancelRecorder is ConfigureTreeCancel that captures the
// evidence from the teardown os/exec performs on cancellation.
//
// The teardown happens inside a callback os/exec owns, so without this the
// outcome is produced and immediately discarded — which is why callers could
// only ever say cancellation was *requested*. The returned accessor reports
// the outcome and whether a teardown ran at all, and is safe to call once
// Run or Wait has returned.
func ConfigureTreeCancelRecorder(cmd *exec.Cmd) func() (KillOutcome, bool) {
	configurePlatform(cmd)
	var (
		mu      sync.Mutex
		outcome KillOutcome
		ran     bool
	)
	cmd.Cancel = func() error {
		got, err := killTree(cmd)
		mu.Lock()
		outcome, ran = got, true
		mu.Unlock()
		return err
	}
	cmd.WaitDelay = 250 * time.Millisecond
	return func() (KillOutcome, bool) {
		mu.Lock()
		defer mu.Unlock()
		return outcome, ran
	}
}

// ConfigureTrackedTreeCancel is the strict subprocess variant. On Windows it
// creates the process suspended so TrackTree can atomically assign a Job Object
// before any child code can run; TrackTree then resumes it.
func ConfigureTrackedTreeCancel(cmd *exec.Cmd) {
	configureTrackedPlatform(cmd)
	cmd.Cancel = func() error {
		return KillTree(cmd)
	}
	cmd.WaitDelay = 250 * time.Millisecond
}

// TrackTree attaches platform-specific lifetime tracking after Start. On
// Windows this places the process in a kill-on-close Job Object so descendants
// cannot survive a normally exiting parent; on POSIX it records the process
// group so ReleaseTree can sweep it for the same reason.
func TrackTree(cmd *exec.Cmd) error {
	return trackTree(cmd)
}

// KillTree tears down cmd's process tree. It keeps an error-only signature
// because exec.Cmd.Cancel requires one; callers that need to report what
// happened should use KillTreeOutcome.
func KillTree(cmd *exec.Cmd) error {
	_, err := killTree(cmd)
	return err
}

// KillTreeOutcome tears down cmd's process tree and reports the evidence.
func KillTreeOutcome(cmd *exec.Cmd) (KillOutcome, error) {
	return killTree(cmd)
}

// ReleaseTree releases platform tracking after Wait. It is not merely cleanup:
// on both platforms it is the point at which descendants left behind by a
// normally exiting root are torn down.
func ReleaseTree(cmd *exec.Cmd) error {
	_, err := releaseTree(cmd)
	return err
}

// ReleaseTreeOutcome releases platform tracking and reports the evidence.
func ReleaseTreeOutcome(cmd *exec.Cmd) (KillOutcome, error) {
	return releaseTree(cmd)
}
