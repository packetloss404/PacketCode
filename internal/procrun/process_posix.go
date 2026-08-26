//go:build !windows

package procrun

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// trackedGroups maps a tracked command to its process-group id. The pgid is
// captured at track time rather than read back later because cmd.Process is
// unusable once Wait has returned, which is exactly when releaseTree runs.
var trackedGroups sync.Map // map[*exec.Cmd]int

// groupSweepTimeout bounds the wait for a signalled group to drain. SIGKILL is
// delivered asynchronously, so an immediate emptiness check would report
// survivors that are merely mid-teardown.
const groupSweepTimeout = 250 * time.Millisecond

// posixEscapeReason is the honest limit of process-group containment. POSIX
// has no portable equivalent of a job object: a descendant that calls setsid()
// leaves the group, stops receiving group signals, and cannot be enumerated
// without platform-specific machinery (cgroups on Linux, nothing on macOS).
const posixEscapeReason = "SIGKILL reached the process group, but a descendant that called setsid() is no longer a member and POSIX cannot enumerate one portably"

func configurePlatform(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func configureTrackedPlatform(cmd *exec.Cmd) { configurePlatform(cmd) }

func killTree(cmd *exec.Cmd) (KillOutcome, error) {
	if cmd == nil || cmd.Process == nil {
		return KillOutcome{Method: KillMethodNone, Confirmed: true}, nil
	}
	// Setpgid made the child its own group leader, so its pid is the pgid.
	return signalGroup(cmd.Process.Pid, false)
}

// signalGroup SIGKILLs a process group. reaped says whether the group leader
// has already been waited on: until it has, it lingers as an unreaped entry
// and would be miscounted as a survivor, so emptiness is only provable
// afterwards.
func signalGroup(pgid int, reaped bool) (KillOutcome, error) {
	out := KillOutcome{Method: KillMethodProcessGroup}
	err := syscall.Kill(-pgid, syscall.SIGKILL)
	switch {
	case errors.Is(err, syscall.ESRCH):
		// The group is already empty, so nothing was left to escape it.
		out.Confirmed = true
		return out, nil
	case err != nil:
		out.Reason = err.Error()
		return out, err
	}
	if !reaped {
		out.Reason = posixEscapeReason
		return out, nil
	}
	if waitForEmptyGroup(pgid) {
		out.Confirmed = true
		return out, nil
	}
	out.Reason = "process group still had members " + groupSweepTimeout.String() + " after SIGKILL"
	return out, nil
}

// waitForEmptyGroup polls until the group reports ESRCH or the deadline
// passes. Signal 0 performs the existence check without delivering anything.
func waitForEmptyGroup(pgid int) bool {
	deadline := time.Now().Add(groupSweepTimeout)
	for {
		if errors.Is(syscall.Kill(-pgid, syscall.Signal(0)), syscall.ESRCH) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func trackTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return errors.New("process has not started")
	}
	trackedGroups.Store(cmd, cmd.Process.Pid)
	return nil
}

// releaseTree is the POSIX counterpart to closing a kill-on-close job object.
// It runs after Wait, so anything still in the group outlived the root that
// created it and must not be left running. Leaving this a no-op is what let a
// server's children survive the server.
func releaseTree(cmd *exec.Cmd) (KillOutcome, error) {
	value, ok := trackedGroups.LoadAndDelete(cmd)
	if !ok {
		return KillOutcome{Method: KillMethodNone, Confirmed: true}, nil
	}
	return signalGroup(value.(int), true)
}
