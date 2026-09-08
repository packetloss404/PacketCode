package mcp

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/testwait"
)

// stubBinaryPath is set by TestMain after compiling internal/mcp/cmd/stub.
var stubBinaryPath string

// TestMain compiles the stub MCP server binary used by the manager
// tests. If the Go toolchain is not on PATH the manager tests skip
// themselves rather than fail.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mcp-stub-")
	if err == nil {
		bin := filepath.Join(dir, "stub")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		// We rely on the test process's working directory still being
		// inside the module — Go test sets cwd to the package dir,
		// which is internal/mcp/, so the source path is relative.
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/stub")
		// Suppress any GOFLAGS that might break the build.
		cmd.Env = os.Environ()
		if err := cmd.Run(); err == nil {
			stubBinaryPath = bin
		}
	}
	code := m.Run()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

func TestManager_Start_RejectsUnsafeServerName(t *testing.T) {
	requireStub(t)
	mgr := NewManager(Config{
		Servers: []ServerConfig{{
			Name:       "../evil",
			Command:    stubBinaryPath,
			Enabled:    true,
			TimeoutSec: 2,
		}},
		LogDir:     t.TempDir(),
		ClientInfo: ClientInfo{Name: "packetcode-test", Version: "0.0.0"},
	})

	reports := mgr.Start(context.Background())
	require.Len(t, reports, 1)
	assert.Equal(t, "failed", reports[0].Status)
	assert.Contains(t, reports[0].Err, "invalid MCP server name")
}

func TestManager_Start_RejectsProtocolMismatch(t *testing.T) {
	requireStub(t)
	mgr := NewManager(Config{
		Servers: []ServerConfig{{
			Name:       "old",
			Command:    stubBinaryPath,
			Env:        map[string]string{"PACKETCODE_STUB_PROTOCOL_VERSION": "1900-01-01"},
			Enabled:    true,
			TimeoutSec: 2,
		}},
		LogDir:     t.TempDir(),
		ClientInfo: ClientInfo{Name: "packetcode-test", Version: "0.0.0"},
	})

	reports := mgr.Start(context.Background())
	require.Len(t, reports, 1)
	assert.Equal(t, "failed", reports[0].Status)
	assert.Contains(t, reports[0].Err, "unsupported protocol version")
}

func TestClient_DeathReason_PreservesNonZeroExit(t *testing.T) {
	requireStub(t)
	mgr := NewManager(Config{
		Servers: []ServerConfig{{
			Name:       "crashy",
			Command:    stubBinaryPath,
			Env:        map[string]string{"PACKETCODE_STUB_EXIT_AFTER_TOOLS": "7"},
			Enabled:    true,
			TimeoutSec: 2,
		}},
		LogDir:     t.TempDir(),
		ClientInfo: ClientInfo{Name: "packetcode-test", Version: "0.0.0"},
	})
	defer func() { _ = mgr.Shutdown(2 * time.Second) }()
	reports := mgr.Start(context.Background())
	require.Len(t, reports, 1)
	require.Equal(t, "running", reports[0].Status, reports[0].Err)
	cli, ok := mgr.Client("crashy")
	require.True(t, ok)

	// A one-second hand-rolled budget: ample when the machine is idle, and the
	// reason this test failed in batches under load. See internal/testwait.
	testwait.For(t, time.Second, "server to exit", func() bool { return !cli.IsAlive() })
	// Waited, not sampled. This used to call DeathReason directly, which reads
	// in the window between the reader seeing stdout close and the reaper
	// collecting the status -- so it passed only because the poll above
	// usually let the reaper land first. Whether a test passes must not depend
	// on which of two goroutines won.
	reason := cli.DeathReasonWithin(DeathReasonWait)
	require.Error(t, reason)
	assert.True(t, strings.Contains(reason.Error(), "exit status 7") || strings.Contains(reason.Error(), "ExitStatus 7"), "death reason = %v", reason)
	// And never the symptom in place of the cause.
	assert.False(t, errors.Is(reason, io.EOF), "reported EOF as the cause: %v", reason)
}

func requireStub(t *testing.T) {
	t.Helper()
	if stubBinaryPath == "" {
		t.Skip("stub MCP binary not built (go toolchain unavailable?)")
	}
}

// TestManager_Start_MixedStatuses asserts the report slice mirrors the
// input order with one running, one disabled, and one failed entry.
func TestManager_Start_MixedStatuses(t *testing.T) {
	requireStub(t)
	logDir := t.TempDir()
	mgr := NewManager(Config{
		Servers: []ServerConfig{
			{
				Name:       "ok",
				Command:    stubBinaryPath,
				Enabled:    true,
				TimeoutSec: 5,
			},
			{
				Name:    "off",
				Command: stubBinaryPath,
				Enabled: false,
			},
			{
				Name:       "broken",
				Command:    filepath.Join(t.TempDir(), "does-not-exist"),
				Enabled:    true,
				TimeoutSec: 2,
			},
		},
		LogDir:     logDir,
		ClientInfo: ClientInfo{Name: "packetcode-test", Version: "0.0.0"},
	})
	defer func() { _ = mgr.Shutdown(2 * time.Second) }()

	reports := mgr.Start(context.Background())
	require.Len(t, reports, 3)
	assert.Equal(t, "ok", reports[0].Name)
	assert.Equal(t, "running", reports[0].Status)
	assert.Contains(t, reports[0].Command, filepath.Base(stubBinaryPath))
	assert.Equal(t, "off", reports[1].Name)
	assert.Equal(t, "disabled", reports[1].Status)
	assert.Equal(t, "broken", reports[2].Name)
	assert.Equal(t, "failed", reports[2].Status)
	assert.NotEmpty(t, reports[2].Err)

	clients := mgr.Clients()
	require.Len(t, clients, 1)
	assert.Equal(t, "ok", clients[0].Name())

	// Reports() returns a defensive copy.
	cached := mgr.Reports()
	require.Len(t, cached, 3)
	cached[0].Name = "mutated"
	assert.Equal(t, "ok", mgr.Reports()[0].Name)
}

func TestManager_StartAgainClosesPreviousClients(t *testing.T) {
	requireStub(t)
	logDir := t.TempDir()
	mgr := NewManager(Config{
		Servers: []ServerConfig{{
			Name:       "ok",
			Command:    stubBinaryPath,
			Enabled:    true,
			TimeoutSec: 5,
		}},
		LogDir:     logDir,
		ClientInfo: ClientInfo{Name: "packetcode-test", Version: "0.0.0"},
	})
	defer func() { _ = mgr.Shutdown(2 * time.Second) }()

	reports := mgr.Start(context.Background())
	require.Equal(t, "running", reports[0].Status, reports[0].Err)
	first, ok := mgr.Client("ok")
	require.True(t, ok)
	require.True(t, first.IsAlive())

	reports = mgr.Start(context.Background())
	require.Equal(t, "running", reports[0].Status, reports[0].Err)
	assert.False(t, first.IsAlive(), "previous client should be closed when Start runs again")
	second, ok := mgr.Client("ok")
	require.True(t, ok)
	assert.NotSame(t, first, second)
}

func TestManager_Restart_ReplacesOnlyNamedClient(t *testing.T) {
	requireStub(t)
	mgr := NewManager(Config{
		Servers: []ServerConfig{
			{Name: "one", Command: stubBinaryPath, Enabled: true, TimeoutSec: 5},
			{Name: "two", Command: stubBinaryPath, Enabled: true, TimeoutSec: 5},
		},
		LogDir:     t.TempDir(),
		ClientInfo: ClientInfo{Name: "packetcode-test", Version: "0.0.0"},
	})
	defer func() { _ = mgr.Shutdown(2 * time.Second) }()
	reports := mgr.Start(context.Background())
	require.Equal(t, "running", reports[0].Status, reports[0].Err)
	require.Equal(t, "running", reports[1].Status, reports[1].Err)
	one, _ := mgr.Client("one")
	two, _ := mgr.Client("two")

	report, replacement, previous, err := mgr.Restart(context.Background(), "one")
	require.NoError(t, err)
	require.Equal(t, "running", report.Status)
	require.Same(t, one, previous)
	require.NotSame(t, one, replacement)
	assert.False(t, one.IsAlive())
	assert.True(t, replacement.IsAlive())
	stillTwo, _ := mgr.Client("two")
	assert.Same(t, two, stillTwo)
	assert.True(t, two.IsAlive())
}

func TestManager_Restart_RejectsUnknownAndDisabledServers(t *testing.T) {
	mgr := NewManager(Config{
		Servers: []ServerConfig{{
			Name: "off", Command: "unused", Enabled: false,
		}},
	})

	_, _, _, err := mgr.Restart(context.Background(), "missing")
	require.ErrorContains(t, err, "no configured server")
	_, _, _, err = mgr.Restart(context.Background(), "off")
	require.ErrorContains(t, err, "disabled")
}

// Every stub must enter initialize before any may finish it. Serial startup
// cannot reach the barrier, regardless of the speed of the test machine.
func TestManager_Start_ParallelSpawn(t *testing.T) {
	requireStub(t)
	logDir := t.TempDir()
	barrierDir := t.TempDir()
	release := filepath.Join(barrierDir, "release")
	var readyFiles []string

	servers := []ServerConfig{}
	for i := 0; i < 4; i++ {
		name := "p" + string(rune('a'+i))
		ready := filepath.Join(barrierDir, name+".ready")
		readyFiles = append(readyFiles, ready)
		servers = append(servers, ServerConfig{
			Name:    name,
			Command: stubBinaryPath,
			Env: map[string]string{
				"PACKETCODE_STUB_READY_FILE":   ready,
				"PACKETCODE_STUB_RELEASE_FILE": release,
			},
			Enabled: true, TimeoutSec: testwait.Seconds(5 * time.Second),
		})
	}
	mgr := NewManager(Config{
		Servers:    servers,
		LogDir:     logDir,
		ClientInfo: ClientInfo{Name: "packetcode-test", Version: "0.0.0"},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		require.NoError(t, mgr.Shutdown(testwait.Timeout(2*time.Second)))
	}()
	done := make(chan []StartupReport, 1)
	go func() { done <- mgr.Start(ctx) }()
	testwait.For(t, 2*time.Second, "all MCP handshakes reached startup barrier", func() bool {
		for _, ready := range readyFiles {
			if _, err := os.Stat(ready); err != nil {
				return false
			}
		}
		return true
	})
	select {
	case <-done:
		t.Fatal("startup returned before blocked handshakes were released")
	default:
	}
	require.NoError(t, os.WriteFile(release, []byte("release"), 0o600))
	var reports []StartupReport
	select {
	case reports = <-done:
	case <-time.After(testwait.Timeout(2 * time.Second)):
		t.Fatal("startup did not finish after releasing all handshakes")
	}

	require.Len(t, reports, 4)
	for _, r := range reports {
		assert.Equal(t, "running", r.Status, "expected all running, got %+v", r)
	}
}

// TestManager_Shutdown_AllClients confirms Shutdown closes every alive
// client and that subsequent Clients() returns no entries.
func TestManager_Shutdown_AllClients(t *testing.T) {
	requireStub(t)
	logDir := t.TempDir()
	mgr := NewManager(Config{
		Servers: []ServerConfig{
			{Name: "a", Command: stubBinaryPath, Enabled: true, TimeoutSec: 5},
			{Name: "b", Command: stubBinaryPath, Enabled: true, TimeoutSec: 5},
		},
		LogDir:     logDir,
		ClientInfo: ClientInfo{Name: "packetcode-test", Version: "0.0.0"},
	})
	reports := mgr.Start(context.Background())
	for _, r := range reports {
		require.Equal(t, "running", r.Status, "%s failed: %s", r.Name, r.Err)
	}
	require.Len(t, mgr.Clients(), 2)

	require.NoError(t, mgr.Shutdown(2*time.Second))

	// After Shutdown the underlying clients should be marked dead.
	for _, name := range []string{"a", "b"} {
		c, ok := mgr.Client(name)
		require.True(t, ok, "client %s missing after Shutdown", name)
		assert.False(t, c.IsAlive(), "client %s should be marked dead", name)
	}
	assert.Empty(t, mgr.Clients())
}
