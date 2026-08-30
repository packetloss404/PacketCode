package mcp

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exitingChild starts a real process that exits with the given status and
// returns a Client wired to it, with neither goroutine started yet.
//
// A real child matters here. The earlier version of these tests constructed a
// Client and called markDead itself, which pinned the contract and exercised
// none of the code that has to honour it -- removing the fix from readerLoop
// left them green. Anything asserting who records the death reason has to let
// the real goroutines record it.
func exitingChild(t *testing.T, status int) *Client {
	t.Helper()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", fmt.Sprintf("exit %d", status))
	} else {
		cmd = exec.Command("sh", "-c", fmt.Sprintf("exit %d", status))
	}
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	return &Client{name: "x", cmd: cmd, stdin: stdin, stdout: stdout, reaped: make(chan struct{})}
}

// Liveness and cause are two different facts. `dead` flips the instant the
// reader sees stdout close, because that is what unblocks stuck callers; the
// cause is not known until the child is reaped. Recording EOF as the cause in
// between is worse than recording nothing, because "exited: EOF" reads like an
// explanation and displaces the real one.
//
// Only the reader runs here, so the window this bug lives in is held open
// rather than raced through.
func TestDeathReason_ReaderDoesNotRecordEOFAsTheCause(t *testing.T) {
	c := exitingChild(t, 7)
	defer func() { _ = c.cmd.Wait() }()

	go c.readerLoop()
	require.Eventually(t, func() bool { return !c.IsAlive() }, 5*time.Second, 5*time.Millisecond,
		"the reader must mark the client dead as soon as stdout closes")

	reason := c.DeathReason()
	require.NotNil(t, reason, "a dead client always has some reason")
	assert.Equal(t, ErrServerExited.Error(), reason.Error(),
		"an unreaped client must report only what is known")
	assert.False(t, errors.Is(reason, io.EOF),
		"EOF is the first symptom, not the cause; reporting it displaces the real one")
	assert.True(t, errors.Is(reason, ErrServerExited))
}

// And once the reaper runs, the authoritative status replaces it -- through
// the real reaperLoop, which is also what closes `reaped`.
func TestDeathReason_ReaperUpgradesToTheExitStatus(t *testing.T) {
	c := exitingChild(t, 7)

	go c.readerLoop()
	require.Eventually(t, func() bool { return !c.IsAlive() }, 5*time.Second, 5*time.Millisecond)

	go c.reaperLoop()

	start := time.Now()
	reason := c.DeathReasonWithin(DeathReasonWait)
	require.NotNil(t, reason)
	assert.Contains(t, reason.Error(), "exit status 7",
		"reported %q instead of the child's exit status", reason)
	assert.False(t, errors.Is(reason, io.EOF), "reported EOF as the cause: %v", reason)
	assert.Less(t, time.Since(start), DeathReasonWait,
		"returned via the timeout, so the reaper never signalled")
}

// DeathReasonWithin waits for the reap rather than sampling, which is the
// whole reason it exists: the diagnostic that reported "exited: EOF" for a
// server that exited 7 was reading in exactly this window.
func TestDeathReasonWithin_WaitsForTheReaper(t *testing.T) {
	c := &Client{name: "x", cmd: &exec.Cmd{}, reaped: make(chan struct{})}
	c.markDead(pendingExit())

	go func() {
		time.Sleep(30 * time.Millisecond)
		c.deadErr.Store(eofExit(errors.New("exit status 7")))
		close(c.reaped)
	}()

	start := time.Now()
	got := c.DeathReasonWithin(time.Second)
	require.NotNil(t, got)
	assert.Contains(t, got.Error(), "exit status 7",
		"the wait returned before the authoritative reason was stored")
	assert.Less(t, time.Since(start), time.Second, "returned via the timeout rather than the reap")
}

// The bound has to hold, or a child that closes its output and lingers would
// hang every diagnostic that asks about it.
func TestDeathReasonWithin_ReturnsWhatIsKnownAtTheBound(t *testing.T) {
	c := &Client{name: "x", cmd: &exec.Cmd{}, reaped: make(chan struct{})}
	c.markDead(pendingExit())

	start := time.Now()
	got := c.DeathReasonWithin(40 * time.Millisecond)
	elapsed := time.Since(start)

	require.NotNil(t, got, "a dead client always has some reason")
	assert.Equal(t, ErrServerExited.Error(), got.Error())
	assert.GreaterOrEqual(t, elapsed, 30*time.Millisecond, "did not actually wait")
	assert.Less(t, elapsed, 2*time.Second, "waited past its own bound")
}

// A live client has no reason, and asking must not block on a reap that is
// not coming.
func TestDeathReasonWithin_AliveReturnsNilImmediately(t *testing.T) {
	c := &Client{name: "x", cmd: &exec.Cmd{}, reaped: make(chan struct{})}
	start := time.Now()
	assert.Nil(t, c.DeathReasonWithin(time.Second))
	assert.Less(t, time.Since(start), 200*time.Millisecond)
}

// An attached client has no child to reap, so whatever the reader recorded is
// already the whole story — and `reaped` must be pre-closed or every ask burns
// the full timeout waiting for a goroutine that was never started.
func TestDeathReasonWithin_AttachedClientDoesNotWait(t *testing.T) {
	stub := makeBasicStub(t, "stub", []ServerTool{{Name: "t"}}, nil)
	cli, err := NewClientWithStub("stub", stub, stubInfo, 5)
	require.NoError(t, err)
	defer stub.Stop()

	stub.CloseStdout()
	require.Eventually(t, func() bool { return !cli.IsAlive() }, time.Second, 5*time.Millisecond)

	start := time.Now()
	reason := cli.DeathReasonWithin(2 * time.Second)
	require.NotNil(t, reason)
	assert.Less(t, time.Since(start), 500*time.Millisecond,
		"an attached client waited for a reaper that never runs")
	// Nothing else was ever going to be learned here, so EOF is the honest
	// answer rather than a placeholder.
	assert.True(t, errors.Is(reason, io.EOF), "got %v", reason)
}
