//go:build !windows

package procrun

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The POSIX counterpart to TestTrackedTreeKillsDescendantAfterRootExits.
// releaseTree used to be a no-op here, so a server that exited normally left
// its children running — the exact gap the Windows job object closed. The
// three roles re-exec this binary: a root that spawns a sleeper and exits
// immediately, and the sleeper itself.
func TestTrackedTreeKillsDescendantAfterRootExits(t *testing.T) {
	const roleKey = "PACKETCODE_PROCRUN_TEST_ROLE"
	switch os.Getenv(roleKey) {
	case "sleeper":
		time.Sleep(30 * time.Second)
		return
	case "parent":
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

	pidFile := filepath.Join(t.TempDir(), "descendant.pid")
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestTrackedTreeKillsDescendantAfterRootExits$")
	cmd.Env = append(os.Environ(), roleKey+"=parent", "PACKETCODE_PROCRUN_PID_FILE="+pidFile)
	ConfigureTrackedTreeCancel(cmd)
	require.NoError(t, cmd.Start())
	require.NoError(t, TrackTree(cmd))
	require.NoError(t, cmd.Wait())

	raw, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(raw))
	require.NoError(t, err)
	require.True(t, posixProcessAlive(pid), "the descendant should outlive its parent until release sweeps it")

	outcome, err := ReleaseTreeOutcome(cmd)
	require.NoError(t, err)
	assert.Equal(t, KillMethodProcessGroup, outcome.Method)

	require.Eventually(t, func() bool { return !posixProcessAlive(pid) }, 3*time.Second, 25*time.Millisecond,
		"descendant process %d survived the process-group sweep", pid)
}

// A group with nothing in it must report Confirmed rather than an error: the
// caller asked for the tree to be gone and it is gone.
func TestKillTreeOnExitedProcessIsConfirmed(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^$")
	ConfigureTreeCancel(cmd)
	require.NoError(t, cmd.Start())
	require.NoError(t, cmd.Wait())

	outcome, err := KillTreeOutcome(cmd)
	require.NoError(t, err)
	assert.True(t, outcome.Confirmed, "an already-empty group is proof, not a failure")
	assert.Empty(t, outcome.Survivors)
}

// A live tree torn down before the root is reaped cannot be confirmed, and the
// reason must say why rather than leaving the caller to guess.
func TestKillTreeBeforeReapIsUnconfirmedWithReason(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestTrackedTreeKillsDescendantAfterRootExits$")
	cmd.Env = append(os.Environ(), "PACKETCODE_PROCRUN_TEST_ROLE=sleeper")
	ConfigureTreeCancel(cmd)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Wait() })

	outcome, err := KillTreeOutcome(cmd)
	require.NoError(t, err)
	assert.Equal(t, KillMethodProcessGroup, outcome.Method)
	assert.True(t, outcome.Unconfirmed(), "the group leader is not yet reaped, so emptiness is unprovable")
	assert.Contains(t, outcome.Reason, "setsid")
}

func TestKillTreeNilProcessIsConfirmedNoop(t *testing.T) {
	outcome, err := KillTreeOutcome(nil)
	require.NoError(t, err)
	assert.Equal(t, KillMethodNone, outcome.Method)
	assert.True(t, outcome.Confirmed)

	outcome, err = KillTreeOutcome(exec.Command(os.Args[0], "-test.run=^$"))
	require.NoError(t, err)
	assert.Equal(t, KillMethodNone, outcome.Method, "a command that never started has no tree")
}

func posixProcessAlive(pid int) bool {
	return !errors.Is(syscall.Kill(pid, syscall.Signal(0)), syscall.ESRCH)
}
