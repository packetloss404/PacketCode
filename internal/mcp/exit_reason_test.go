package mcp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
)

// The reader loop ends for several reasons and only some of them are the
// server's fault. Getting this wrong is not cosmetic: markDead is
// first-writer-wins, so a reason recorded here cannot be replaced by the
// reaper's clean exit status, and Close surfaces it as a shutdown failure.
func TestExitReasonFromRead(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		haveChild bool
		wantClean bool
	}{
		{
			name:      "EOF with a child to reap is provisional",
			err:       io.EOF,
			haveChild: true,
			wantClean: true,
		},
		{
			// os/exec closes the pipes it created once Wait sees the child
			// exit, so a read still in flight fails with "file already
			// closed" rather than reporting EOF. The scanner lost a race with
			// reaperLoop; the server is fine.
			name:      "a pipe closed by cmd.Wait is provisional",
			err:       &os.PathError{Op: "read", Path: "|0", Err: os.ErrClosed},
			haveChild: true,
			wantClean: true,
		},
		{
			name:      "a wrapped ErrClosed is still provisional",
			err:       fmt.Errorf("scanner: %w", os.ErrClosed),
			haveChild: true,
			wantClean: true,
		},
		{
			// With no child to reap, nothing is coming to replace the reason,
			// so it records EOF concretely instead of staying provisional. It
			// is still a clean shutdown: closeExitErr filters a wrapped EOF.
			name:      "EOF with no child records the exit and is still clean",
			err:       io.EOF,
			haveChild: false,
			wantClean: true,
		},
		{
			// A real stream failure is what happened, and the reaper's exit
			// status does not describe it.
			name:      "a genuine read failure is kept",
			err:       errors.New("connection reset by peer"),
			haveChild: true,
			wantClean: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reason := exitReasonFromRead(tc.err, tc.haveChild)
			if !errors.Is(reason, ErrServerExited) {
				t.Fatalf("reason %v does not wrap ErrServerExited", reason)
			}

			// closeExitErr is what decides whether Close reports a failure,
			// so assert through it rather than on the shape of the value.
			got := closeExitErr(reason)
			if tc.wantClean && got != nil {
				t.Errorf("Close would report %v, want a clean shutdown", got)
			}
			if !tc.wantClean && got == nil {
				t.Errorf("Close would report success, want the reason surfaced")
			}
		})
	}
}

// The pipe-closed case is the one that made TestManager_Shutdown_AllClients
// fail intermittently, so pin it end to end: the reason it produces must be
// the provisional one, which is what lets the reaper's exit status win.
func TestExitReasonFromRead_ClosedPipeStaysReplaceable(t *testing.T) {
	reason := exitReasonFromRead(&os.PathError{Op: "read", Path: "|0", Err: os.ErrClosed}, true)

	var exit *serverExitError
	if !errors.As(reason, &exit) {
		t.Fatalf("reason %v is not a serverExitError", reason)
	}
	if !exit.pending {
		t.Fatal("a pipe closed by cmd.Wait must be provisional, or the reaper cannot replace it")
	}
	if exit.underlying != nil {
		t.Errorf("underlying = %v, want nil: the closed pipe is not the cause of death", exit.underlying)
	}
}
