//go:build windows

package procrun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestTrackedTreeKillsDescendantAfterRootExits(t *testing.T) {
	const roleKey = "PACKETCODE_PROCRUN_TEST_ROLE"
	role := os.Getenv(roleKey)
	if role == "sleeper" {
		time.Sleep(30 * time.Second)
		return
	}
	if role == "parent" {
		child := exec.Command(os.Args[0], "-test.run=^TestTrackedTreeKillsDescendantAfterRootExits$")
		child.Env = append(os.Environ(), roleKey+"=sleeper")
		if err := child.Start(); err != nil {
			panic(err)
		}
		if err := os.WriteFile(os.Getenv("PACKETCODE_PROCRUN_PID_FILE"), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			panic(err)
		}
		return
	}

	pidFile := t.TempDir() + `\descendant.pid`
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestTrackedTreeKillsDescendantAfterRootExits$")
	cmd.Env = append(os.Environ(), roleKey+"=parent", "PACKETCODE_PROCRUN_PID_FILE="+pidFile)
	ConfigureTrackedTreeCancel(cmd)
	require.NoError(t, cmd.Start())
	require.NoError(t, TrackTree(cmd))
	require.NoError(t, cmd.Wait())
	require.NoError(t, ReleaseTree(cmd))

	raw, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid64, err := strconv.ParseUint(string(raw), 10, 32)
	require.NoError(t, err)
	pid := uint32(pid64)
	require.Eventually(t, func() bool { return !windowsProcessAlive(pid) }, 3*time.Second, 25*time.Millisecond,
		fmt.Sprintf("descendant process %d survived Job Object release", pid))
}

func windowsProcessAlive(pid uint32) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	result, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && result == uint32(windows.WAIT_TIMEOUT)
}
