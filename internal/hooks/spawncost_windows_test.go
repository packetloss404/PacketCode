//go:build windows

// Windows only, because the problem is. On POSIX a hook spawn is `sh -c`,
// which costs single-digit milliseconds cold or warm and has never been the
// reason a test failed.

package hooks

import (
	"bytes"
	"context"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/procrun"
)

// TestHookSpawnCost breaks a Windows hook launch into tiers so a slow one can
// be attributed rather than argued about. Each tier adds exactly one thing to
// the one above it, so the difference between two rows is the cost of that
// thing:
//
//   - a bare CreateProcess says whether the machine is contended at all;
//   - PowerShell with no stdin adds the interpreter's own start-up;
//   - stdin attached, then the full hook script, add the plumbing this package
//     is responsible for;
//   - the tree-cancel wiring and the Runner add what internal/hooks costs;
//   - pwsh is measured beside powershell because "use the other one" is the
//     first thing anyone suggests, and the numbers say it is slower.
//
// The reference reading, taken on four GitHub windows-latest runners in
// September 2026: bare CreateProcess 15-38ms, the job's first PowerShell
// 4.33-4.87s, every later PowerShell 0.16-0.19s, stdin and the hook script
// under 10ms each on top, internal/hooks under 20ms on top, pwsh 265-285ms
// against powershell's 165-185ms. A run that departs sharply from those is
// worth reading before any budget in this file is changed.
//
// Off by default: it takes about fifteen seconds and asserts nothing. It is
// here so the next person to suspect the hook path can measure it rather than
// guess, which is how the budgets in this package were set.
//
//	PACKETCODE_HOOK_TIMING=1 go test -run TestHookSpawnCost -v ./internal/hooks/
//
// That variable also stands TestMain's warm-up down, so the first PowerShell
// row below is a genuine cold start rather than a second look at the warm one.
func TestHookSpawnCost(t *testing.T) {
	if !timingRequested() {
		t.Skipf("set %s=1 to measure hook spawn cost", TimingEnv)
	}
	const runs = 7

	psArgs := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command"}
	hookScript := "$data = [Console]::In.ReadToEnd(); if ($data -match 'hello') { 'injected-context' } else { exit 1 }"
	payload := []byte(`{"event":"UserPromptSubmit","prompt":"hello"}`)

	ps := func(script string) []string { return append(append([]string{}, psArgs...), script) }

	// raw runs exe directly. treeCancel adds the wiring shellCommand applies,
	// so the pair isolates what that wiring costs.
	raw := func(exe string, args []string, stdin []byte) func() error {
		return func() error { return runOnce(exec.Command(exe, args...), stdin) }
	}
	treeCancel := func(exe string, args []string, stdin []byte) func() error {
		return func() error {
			cmd := exec.CommandContext(context.Background(), exe, args...)
			procrun.ConfigureTreeCancel(cmd)
			return runOnce(cmd, stdin)
		}
	}
	viaRunner := func() error {
		r := New(config.HooksConfig{
			UserPromptSubmit: []config.HookConfig{{Command: hookScript, TimeoutSec: 60}},
		}, t.TempDir())
		_, err := r.RunUserPromptSubmit(context.Background(), PromptPayload{Prompt: "hello"})
		return err
	}

	t.Log("=== idle ===")
	report(t, "bare CreateProcess (cmd.exe /c exit)", runs, raw("cmd.exe", []string{"/c", "exit"}, nil))
	report(t, "powershell, no stdin", runs, raw("powershell", ps("exit 0"), nil))
	report(t, "powershell, stdin attached", runs, raw("powershell", ps("exit 0"), payload))
	report(t, "powershell, full hook script", runs, raw("powershell", ps(hookScript), payload))
	report(t, "powershell, + tree-cancel wiring", runs, treeCancel("powershell", ps(hookScript), payload))
	report(t, "hooks.Runner, end to end", runs, viaRunner)
	if _, err := exec.LookPath("pwsh"); err == nil {
		report(t, "pwsh, full hook script", runs, raw("pwsh", ps(hookScript), payload))
	} else {
		t.Log("pwsh not on PATH; skipping the interpreter comparison")
	}

	// The suite does not run on an idle machine, so measure a loaded one too.
	// A gap that only opens here is contention, not interpreter cost.
	t.Log("=== under CPU load ===")
	stop := burnCPU(4)
	report(t, "powershell, full hook script", runs, raw("powershell", ps(hookScript), payload))
	report(t, "hooks.Runner, end to end", runs, viaRunner)
	stop()

	t.Log("=== concurrent ===")
	start := time.Now()
	var wg sync.WaitGroup
	each := make([]time.Duration, 8)
	for i := range each {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := time.Now()
			_ = raw("powershell", ps(hookScript), payload)()
			each[i] = time.Since(s)
		}(i)
	}
	wg.Wait()
	t.Logf("%-38s wall=%-9s each=%s", "8 concurrent powershell spawns",
		time.Since(start).Round(time.Millisecond), durations(each))
}

// runOnce runs cmd with stdin attached and its output discarded, so the timing
// reflects the spawn rather than the caller's buffering.
func runOnce(cmd *exec.Cmd, stdin []byte) error {
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	return cmd.Run()
}

// report times fn n times and logs the spread.
//
// It prints the first result separately from the minimum because on Windows
// those are the two different questions: the first spawn on a machine pays the
// interpreter's start-up and the rest do not, and a summary that hides that
// distinction is what let a five-second budget look generous.
func report(t *testing.T, name string, n int, fn func() error) {
	t.Helper()
	ds := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		err := fn()
		ds = append(ds, time.Since(start))
		if err != nil {
			t.Logf("%-38s run %d returned %v", name, i, err)
		}
	}
	sorted := append([]time.Duration(nil), ds...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	t.Logf("%-38s first=%-9s min=%-9s med=%-9s max=%-9s all=%s",
		name,
		ds[0].Round(time.Millisecond),
		sorted[0].Round(time.Millisecond),
		sorted[len(sorted)/2].Round(time.Millisecond),
		sorted[len(sorted)-1].Round(time.Millisecond),
		durations(ds))
}

// burnCPU keeps n goroutines busy and returns a function that stops them and
// waits for them to finish, so the load cannot leak into a later measurement.
func burnCPU(n int) func() {
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x := 0
			for {
				select {
				case <-stop:
					return
				default:
					for j := 0; j < 1e6; j++ {
						x += j
					}
					_ = x
				}
			}
		}()
	}
	return func() {
		close(stop)
		wg.Wait()
	}
}

func durations(ds []time.Duration) string {
	parts := make([]string, len(ds))
	for i, d := range ds {
		parts[i] = d.Round(time.Millisecond).String()
	}
	return "[" + strings.Join(parts, " ") + "]"
}
