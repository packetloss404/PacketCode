package computers

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// os/exec quotes the /C argument for a C runtime, and cmd.exe is not one: it
// stripped the outer quotes and passed the escaped inner ones through, so
// `echo "hello world"` printed \"hello world\" and the PowerShell invocation
// the tool description recommends ran nothing. The command line now reaches
// cmd.exe verbatim.
func TestLocalBackend_Execute_WindowsPreservesQuotes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd.exe quoting is Windows-specific")
	}
	b, err := NewLocalBackend(t.TempDir())
	require.NoError(t, err)

	var out bytes.Buffer
	res, err := b.Execute(context.Background(), `echo "hello world"`, "", &out)
	require.NoError(t, err)
	assert.Equal(t, 0, res.ExitCode)
	got := strings.TrimSpace(out.String())
	assert.Equal(t, `"hello world"`, got)
	assert.NotContains(t, got, `\"`)

	out.Reset()
	res, err = b.Execute(context.Background(), `powershell -NoProfile -Command "Write-Output 'x y'"`, "", &out)
	require.NoError(t, err)
	assert.Equal(t, 0, res.ExitCode)
	assert.Equal(t, "x y", strings.TrimSpace(out.String()))
}

// A shell that exits 0 while a child it started still holds the output pipe
// is how a dev server gets launched. That used to be reported as exit -1 with
// a "WaitDelay expired" backend error: the model was told a successful
// command had failed.
func TestLocalBackend_Execute_LingeringChildIsNotAFailure(t *testing.T) {
	// Not t.TempDir(): this test deliberately leaves a child process alive,
	// and on Windows that child holds the working directory open, so the
	// automatic cleanup fails the test for a reason that has nothing to do
	// with what it asserts. Removed best-effort once the child has exited.
	dir, err := os.MkdirTemp("", "packetcode-lingering")
	require.NoError(t, err)
	t.Cleanup(func() {
		deadline := time.Now().Add(10 * time.Second)
		for {
			if err := os.RemoveAll(dir); err == nil || time.Now().After(deadline) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	b, err := NewLocalBackend(dir)
	require.NoError(t, err)

	command := "sleep 2 & echo started"
	if runtime.GOOS == "windows" {
		command = `start /B ping -n 3 127.0.0.1 >NUL & echo started`
	}
	var out bytes.Buffer
	res, err := b.Execute(context.Background(), command, "", &out)
	require.NoError(t, err)
	assert.Equal(t, 0, res.ExitCode)
	assert.Contains(t, out.String(), "started")
}
