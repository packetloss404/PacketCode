package mcp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// maxParallelStartup bounds the number of concurrent server spawns
// during Manager.Start. Spec: 8.
const maxParallelStartup = 8

// shutdownExtraTimeout is added to the per-client Close timeout when
// computing the overall Manager.Shutdown deadline. Spec: 1s.
const shutdownExtraTimeout = time.Second

// Manager owns a fleet of MCP Clients and surfaces them to the rest of
// the app via a name-keyed map. It is safe for concurrent use after
// Start has returned.
type Manager struct {
	cfg        Config
	mu         sync.RWMutex
	clients    map[string]*Client
	reports    []StartupReport
	closed     bool
	restarting map[string]bool
	operations sync.WaitGroup
	stopCtx    context.Context
	stop       context.CancelFunc
}

// NewManager constructs a Manager. Start() must be called before
// Clients() / Client() will return anything useful.
func NewManager(cfg Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		cfg:        cfg,
		clients:    map[string]*Client{},
		restarting: map[string]bool{},
		stopCtx:    ctx,
		stop:       cancel,
	}
}

// Start spawns every configured server in parallel (max 8 concurrent),
// runs the MCP handshake on each, and returns a StartupReport per
// server in input order. Successful clients are stored on the manager.
//
// Start is intended to be called once during app startup; calling it
// again is allowed but will overwrite the cached clients/reports.
func (m *Manager) Start(ctx context.Context) []StartupReport {
	servers := m.cfg.Servers
	reports := make([]StartupReport, len(servers))
	clients := make([]*Client, len(servers))
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		for i, sc := range servers {
			reports[i] = startupReportFor(sc, "failed", nil, fmt.Errorf("mcp: manager is shut down"))
		}
		return reports
	}
	m.operations.Add(1)
	m.mu.Unlock()
	defer m.operations.Done()
	ctx, cancel := context.WithCancel(ctx)
	stopCancel := context.AfterFunc(m.stopCtx, cancel)
	defer stopCancel()
	defer cancel()

	sem := make(chan struct{}, maxParallelStartup)
	var wg sync.WaitGroup
	for i, sc := range servers {
		i, sc := i, sc
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if !sc.Enabled {
				reports[i] = StartupReport{
					Name:       sc.Name,
					Status:     "disabled",
					Command:    renderServerCommand(sc),
					TimeoutSec: effectiveTimeoutSec(sc),
					Auth:       authSummary(sc),
				}
				return
			}
			cli, err := NewClient(ctx, sc, m.cfg.LogDir, m.cfg.ClientInfo)
			if err != nil {
				reports[i] = StartupReport{
					Name:       sc.Name,
					Status:     "failed",
					Command:    renderServerCommand(sc),
					Err:        err.Error(),
					TimeoutSec: effectiveTimeoutSec(sc),
					Auth:       authSummary(sc),
				}
				return
			}
			clients[i] = cli
			reports[i] = StartupReport{
				Name:       sc.Name,
				Status:     "running",
				ToolCount:  len(cli.Tools()),
				PID:        cli.PID(),
				Command:    renderServerCommand(sc),
				TimeoutSec: effectiveTimeoutSec(sc),
				Auth:       authSummary(sc),
			}
		}()
	}
	wg.Wait()

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		for i, c := range clients {
			if c != nil {
				_ = c.Close(2 * time.Second)
			}
			reports[i] = startupReportFor(servers[i], "failed", nil, fmt.Errorf("mcp: manager shut down during startup"))
		}
		return reports
	}
	oldClients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		if c != nil {
			oldClients = append(oldClients, c)
		}
	}
	m.reports = reports
	m.clients = map[string]*Client{}
	for _, c := range clients {
		if c != nil {
			m.clients[c.Name()] = c
		}
	}
	m.mu.Unlock()
	for _, c := range oldClients {
		_ = c.Close(2 * time.Second)
	}
	return reports
}

// Clients returns the live client list in alphabetic-by-name order.
// Dead clients are filtered out.
func (m *Manager) Clients() []*Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.clients))
	for n, c := range m.clients {
		if c != nil && c.IsAlive() {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	out := make([]*Client, len(names))
	for i, n := range names {
		out[i] = m.clients[n]
	}
	return out
}

// Client returns the named client (alive or not) along with ok=false if
// the name was never registered.
func (m *Manager) Client(name string) (*Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clients[name]
	return c, ok
}

// Restart replaces one configured server process without disturbing the rest
// of the MCP fleet. The manager keeps its startup configuration immutable:
// configuration file changes still take effect on the next PacketCode start,
// while a crashed or unhealthy process can be reconnected immediately.
//
// The previous client is returned so the caller can remove tool adapters that
// still point at it before registering tools from the replacement.
func (m *Manager) Restart(ctx context.Context, name string) (StartupReport, *Client, *Client, error) {
	var server *ServerConfig
	for i := range m.cfg.Servers {
		if m.cfg.Servers[i].Name == name {
			copy := m.cfg.Servers[i]
			server = &copy
			break
		}
	}
	if server == nil {
		return StartupReport{}, nil, nil, fmt.Errorf("mcp: no configured server named %s", name)
	}
	if !server.Enabled {
		return StartupReport{}, nil, nil, fmt.Errorf("mcp: server %s is disabled", name)
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return StartupReport{}, nil, nil, fmt.Errorf("mcp: manager is shut down")
	}
	if m.restarting[name] {
		m.mu.Unlock()
		return StartupReport{}, nil, nil, fmt.Errorf("mcp: server %s is already restarting", name)
	}
	m.restarting[name] = true
	m.operations.Add(1)
	previous := m.clients[name]
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.restarting, name)
		m.mu.Unlock()
		m.operations.Done()
	}()
	ctx, cancel := context.WithCancel(ctx)
	stopCancel := context.AfterFunc(m.stopCtx, cancel)
	defer stopCancel()
	defer cancel()

	if previous != nil {
		// A close error here means the old process would not exit on its
		// own and was killed, or exited non-zero -- the unhealthy server a
		// restart exists to replace. It is gone either way, so the error is
		// not a reason to leave the user with no server at all.
		_ = previous.Close(2 * time.Second)
	}

	client, err := NewClient(ctx, *server, m.cfg.LogDir, m.cfg.ClientInfo)
	if err != nil {
		report := startupReportFor(*server, "failed", nil, err)
		m.replaceReport(report)
		return report, nil, previous, fmt.Errorf("mcp restart %s: %w", name, err)
	}
	report := startupReportFor(*server, "running", client, nil)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = client.Close(2 * time.Second)
		return StartupReport{}, nil, previous, fmt.Errorf("mcp: manager shut down during restart")
	}
	m.clients[name] = client
	m.mu.Unlock()
	m.replaceReport(report)
	return report, client, previous, nil
}

func startupReportFor(sc ServerConfig, status string, client *Client, err error) StartupReport {
	report := StartupReport{
		Name:       sc.Name,
		Status:     status,
		Command:    renderServerCommand(sc),
		TimeoutSec: effectiveTimeoutSec(sc),
		Auth:       authSummary(sc),
	}
	if client != nil {
		report.ToolCount = len(client.Tools())
		report.PID = client.PID()
	}
	if err != nil {
		report.Err = err.Error()
	}
	return report
}

func (m *Manager) replaceReport(report StartupReport) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.reports {
		if m.reports[i].Name == report.Name {
			m.reports[i] = report
			return
		}
	}
	m.reports = append(m.reports, report)
}

// Reports returns the cached StartupReport slice from Start, adjusted
// for clients that have exited since startup. Returns a defensive copy.
func (m *Manager) Reports() []StartupReport {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]StartupReport, len(m.reports))
	for i, r := range m.reports {
		if r.Status == "running" {
			c := m.clients[r.Name]
			if c == nil || !c.IsAlive() {
				r.Status = "exited"
				r.PID = -1
				// Waited, not sampled: this is the line a human reads to find
				// out why a server died, and asking before the child is reaped
				// reported "exited: EOF" for one that exited 7.
				if c != nil {
					if reason := c.DeathReasonWithin(DeathReasonWait); reason != nil {
						r.Err = reason.Error()
					}
				}
			}
		}
		out[i] = r
	}
	return out
}

func renderServerCommand(sc ServerConfig) string {
	parts := make([]string, 0, 1+len(sc.Args))
	if sc.Command != "" {
		parts = append(parts, sc.Command)
	}
	parts = append(parts, sc.Args...)
	return strings.Join(parts, " ")
}

func effectiveTimeoutSec(sc ServerConfig) int {
	if sc.TimeoutSec > 0 {
		return sc.TimeoutSec
	}
	return defaultInitTimeoutSec
}

func authSummary(sc ServerConfig) string {
	if len(sc.Env) == 0 {
		return "none"
	}
	names := make([]string, 0, len(sc.Env))
	for k := range sc.Env {
		if looksSecretKey(k) {
			names = append(names, k)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	sort.Strings(names)
	if len(names) > 3 {
		return fmt.Sprintf("env:%s,+%d", strings.Join(names[:3], ","), len(names)-3)
	}
	return "env:" + strings.Join(names, ",")
}

func looksSecretKey(k string) bool {
	k = strings.ToLower(k)
	for _, token := range []string{"api_key", "apikey", "token", "secret", "password", "bearer"} {
		if strings.Contains(k, token) {
			return true
		}
	}
	return false
}

// Shutdown closes every alive client in parallel. Returns a composite
// error listing per-client failures, or nil if every client closed
// cleanly.
func (m *Manager) Shutdown(timeout time.Duration) error {
	m.mu.Lock()
	m.closed = true
	m.stop()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		if c != nil {
			clients = append(clients, c)
		}
	}
	m.mu.Unlock()

	type result struct {
		name string
		err  error
	}
	resCh := make(chan result, len(clients)+1)
	go func() {
		m.operations.Wait()
		resCh <- result{name: "restarts"}
	}()
	for _, c := range clients {
		c := c
		go func() {
			err := c.Close(timeout)
			resCh <- result{name: c.Name(), err: err}
		}()
	}

	deadline := time.After(timeout + shutdownExtraTimeout)
	var errs []string
	collected := 0
	for collected < len(clients)+1 {
		select {
		case r := <-resCh:
			collected++
			if r.err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", r.name, r.err))
			}
		case <-deadline:
			errs = append(errs, fmt.Sprintf("MCP shutdown did not finish within %s", timeout+shutdownExtraTimeout))
			collected = len(clients) + 1 // bail
		}
	}
	if len(errs) > 0 {
		return errors.New("mcp.Shutdown: " + strings.Join(errs, "; "))
	}
	return nil
}
