package jobs

// InjectResultForTests appends a synthetic Result onto the manager's
// internal drain queue. Intended only for unit tests in sibling
// packages (e.g. internal/app) that want to verify App.startTurn's
// DrainResults injection path without booting a real provider.
//
// Callers should treat this as a black-box seam: the shape of the
// queue is a private implementation detail, but the contract "what
// DrainResults will yield on the next call" is stable.
func InjectResultForTests(m *Manager, r Result) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.results = append(m.results, r)
	m.mu.Unlock()
}
