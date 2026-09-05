//go:build windows

package hooks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/packetcode/packetcode/internal/config"
	"github.com/packetcode/packetcode/internal/procrun"
)

// TestHookTimingDiagnostics is temporary instrumentation. It is not an
// assertion about anything; it measures where the Windows hook budget goes.
func TestHookTimingDiagnostics(t *testing.T) {
	if os.Getenv("PACKETCODE_HOOK_TIMING") != "1" {
		t.Skip("set PACKETCODE_HOOK_TIMING=1 to run the timing instrumentation")
	}
	const runs = 7

	report := func(name string, fn func() error) []time.Duration {
		var ds []time.Duration
		for i := 0; i < runs; i++ {
			start := time.Now()
			err := fn()
			d := time.Since(start)
			ds = append(ds, d)
			if err != nil {
				t.Logf("%-46s run %d: ERROR %v", name, i, err)
			}
		}
		sorted := append([]time.Duration(nil), ds...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		t.Logf("%-46s first=%-9s min=%-9s med=%-9s max=%-9s all=%v",
			name, ds[0].Round(time.Millisecond), sorted[0].Round(time.Millisecond),
			sorted[len(sorted)/2].Round(time.Millisecond), sorted[len(sorted)-1].Round(time.Millisecond),
			roundAll(ds))
		return ds
	}

	psArgs := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command"}
	stdinScript := "$data = [Console]::In.ReadToEnd(); if ($data -match 'hello') { 'injected-context' } else { exit 1 }"
	payload := []byte(`{"event":"UserPromptSubmit","prompt":"hello"}`)

	raw := func(exe string, args []string, stdin []byte) func() error {
		return func() error {
			cmd := exec.Command(exe, args...)
			if stdin != nil {
				cmd.Stdin = bytes.NewReader(stdin)
			}
			cmd.Stdout = &bytes.Buffer{}
			cmd.Stderr = &bytes.Buffer{}
			return cmd.Run()
		}
	}
	rawTree := func(exe string, args []string, stdin []byte) func() error {
		return func() error {
			cmd := exec.CommandContext(context.Background(), exe, args...)
			procrun.ConfigureTreeCancel(cmd)
			if stdin != nil {
				cmd.Stdin = bytes.NewReader(stdin)
			}
			cmd.Stdout = &bytes.Buffer{}
			cmd.Stderr = &bytes.Buffer{}
			return cmd.Run()
		}
	}

	t.Log("=== sequential, idle ===")
	report("cmd.exe /c exit (bare CreateProcess)", raw("cmd.exe", []string{"/c", "exit"}, nil))
	report("cmd.exe /c findstr (stdin)", raw("cmd.exe", []string{"/c", "findstr", "hello"}, payload))
	report("powershell -Command exit 0 (no stdin)", raw("powershell", append(append([]string{}, psArgs...), "exit 0"), nil))
	report("powershell -Command exit 0 (stdin attached)", raw("powershell", append(append([]string{}, psArgs...), "exit 0"), payload))
	report("powershell full hook script (stdin read)", raw("powershell", append(append([]string{}, psArgs...), stdinScript), payload))
	report("powershell full + ConfigureTreeCancel", rawTree("powershell", append(append([]string{}, psArgs...), stdinScript), payload))
	if _, err := exec.LookPath("pwsh"); err == nil {
		report("pwsh full hook script (stdin read)", raw("pwsh", append(append([]string{}, psArgs...), stdinScript), payload))
	} else {
		t.Log("pwsh not on PATH")
	}

	runner := func(command string, timeoutSec int) func() error {
		return func() error {
			r := New(config.HooksConfig{
				UserPromptSubmit: []config.HookConfig{{Command: command, TimeoutSec: timeoutSec}},
			}, t.TempDir())
			_, err := r.RunUserPromptSubmit(context.Background(), PromptPayload{Prompt: "hello"})
			return err
		}
	}
	report("hooks.Runner end-to-end (60s budget)", runner(stdinScript, 60))

	// Now the same thing while the machine is busy, which is the condition the
	// suite actually runs under.
	t.Log("=== sequential, under CPU load ===")
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
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
	report("powershell full hook script (loaded)", raw("powershell", append(append([]string{}, psArgs...), stdinScript), payload))
	report("hooks.Runner end-to-end (loaded)", runner(stdinScript, 60))
	close(stop)
	wg.Wait()

	// Concurrent spawns, which is what -parallel does to a suite.
	t.Log("=== 8 concurrent powershell spawns ===")
	start := time.Now()
	var cwg sync.WaitGroup
	durs := make([]time.Duration, 8)
	for i := 0; i < 8; i++ {
		cwg.Add(1)
		go func(i int) {
			defer cwg.Done()
			s := time.Now()
			_ = raw("powershell", append(append([]string{}, psArgs...), stdinScript), payload)()
			durs[i] = time.Since(s)
		}(i)
	}
	cwg.Wait()
	t.Logf("8 concurrent: wall=%s each=%v", time.Since(start).Round(time.Millisecond), roundAll(durs))
}

func roundAll(ds []time.Duration) string {
	out := "["
	for i, d := range ds {
		if i > 0 {
			out += " "
		}
		out += fmt.Sprint(d.Round(time.Millisecond))
	}
	return out + "]"
}
