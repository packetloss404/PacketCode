//go:build !windows

package procrun

import (
	"os/exec"
	"syscall"
)

func configurePlatform(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func configureTrackedPlatform(cmd *exec.Cmd) { configurePlatform(cmd) }

func KillTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// Negative PID targets the process group created in configurePlatform.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

func trackTree(_ *exec.Cmd) error { return nil }

func releaseTree(_ *exec.Cmd) error { return nil }
