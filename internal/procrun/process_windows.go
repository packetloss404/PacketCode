//go:build windows

package procrun

import (
	"os/exec"
	"strconv"
	"syscall"
)

func configurePlatform(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func KillTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	if err := exec.Command("taskkill", "/T", "/F", "/PID", pid).Run(); err != nil {
		// Fall back to killing the direct child if taskkill is unavailable or
		// cannot enumerate descendants in the current environment.
		return cmd.Process.Kill()
	}
	return nil
}
