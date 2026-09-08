package mcp

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/packetcode/packetcode/internal/testwait"
)

// Hold the previous process open so overlap is exercised independently of
// scheduler speed or how quickly the replacement completes its handshake.
type heldMCPClose struct {
	io.WriteCloser
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	hold    atomic.Bool
}

func (w *heldMCPClose) Close() error {
	if w.hold.Load() {
		w.once.Do(func() { close(w.entered) })
		<-w.release
	}
	return w.WriteCloser.Close()
}

func lifecycleManager(t *testing.T) (*Manager, *heldMCPClose) {
	t.Helper()
	requireStub(t)
	m := NewManager(Config{Servers: []ServerConfig{{Name: "lifecycle", Command: stubBinaryPath, Enabled: true, TimeoutSec: 5}}, LogDir: t.TempDir()})
	t.Cleanup(func() { require.NoError(t, m.Shutdown(testwait.Timeout(time.Second))) })
	// Install the wrapper before the client starts its reader and reaper. A
	// live client's pipe fields are immutable and cannot be swapped safely.
	cmd, stdin, stdout, logFile, err := spawnServerProcess(m.cfg.Servers[0], m.cfg.LogDir)
	require.NoError(t, err)
	gate := &heldMCPClose{WriteCloser: stdin, entered: make(chan struct{}), release: make(chan struct{})}
	c, err := newClientFromIO(context.Background(), "lifecycle", gate, stdout, logFile, cmd, testwait.Timeout(time.Second), m.cfg.ClientInfo)
	require.NoError(t, err)
	// Handshake failures must be able to close the pipe before the test has
	// an opportunity to release the barrier.
	gate.hold.Store(true)
	m.clients[c.Name()] = c
	m.reports = []StartupReport{startupReportFor(m.cfg.Servers[0], "running", c, nil)}
	return m, gate
}

func waitMCPSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(testwait.Timeout(time.Second)):
		t.Fatal(what)
	}
}

func TestManager_ConcurrentRestartRejected(t *testing.T) {
	m, gate := lifecycleManager(t)
	var release sync.Once
	defer release.Do(func() { close(gate.release) })
	type outcome struct {
		client *Client
		err    error
	}
	done := make(chan outcome, 1)
	go func() { _, c, _, err := m.Restart(context.Background(), "lifecycle"); done <- outcome{c, err} }()
	waitMCPSignal(t, gate.entered, "restart did not close previous process")
	_, c, previous, err := m.Restart(context.Background(), "lifecycle")
	require.ErrorContains(t, err, "already restarting")
	require.Nil(t, c)
	require.Nil(t, previous)
	release.Do(func() { close(gate.release) })
	select {
	case result := <-done:
		require.NoError(t, result.err)
		require.NoError(t, m.Shutdown(testwait.Timeout(time.Second)))
		require.False(t, result.client.IsAlive(), "replacement must be owned by Shutdown")
	case <-time.After(testwait.Timeout(time.Second)):
		t.Fatal("restart did not finish")
	}
}

func TestManager_ShutdownDuringRestart(t *testing.T) {
	m, gate := lifecycleManager(t)
	var release sync.Once
	defer release.Do(func() { close(gate.release) })
	restarted := make(chan error, 1)
	go func() { _, _, _, err := m.Restart(context.Background(), "lifecycle"); restarted <- err }()
	waitMCPSignal(t, gate.entered, "restart did not close previous process")
	stopped := make(chan error, 1)
	go func() { stopped <- m.Shutdown(testwait.Timeout(time.Second)) }()
	testwait.For(t, time.Second, "shutdown closed admission", func() bool { m.mu.RLock(); defer m.mu.RUnlock(); return m.closed })
	release.Do(func() { close(gate.release) })
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(testwait.Timeout(2 * time.Second)):
		t.Fatal("Shutdown did not finish")
	}
	select {
	case err := <-restarted:
		require.Error(t, err)
	case <-time.After(testwait.Timeout(time.Second)):
		t.Fatal("restart did not finish after Shutdown")
	}
	require.Empty(t, m.Clients())
	_, _, _, err := m.Restart(context.Background(), "lifecycle")
	require.ErrorContains(t, err, "shut down")
	reports := m.Start(context.Background())
	require.Equal(t, "failed", reports[0].Status)
	require.Empty(t, m.Clients())
}
