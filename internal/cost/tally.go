// Package cost owns the per-session and global token-usage tally that the
// status line and /cost slash command surface.
//
// On-disk format (~/.packetcode/cost-tally.json):
//
//   {
//     "sessions": {
//       "<session-uuid>": {
//         "input": 84000, "output": 12000,
//         "provider": "openai", "model": "gpt-4.1"
//       }
//     },
//     "start_time": 1736942400
//   }
//
// Per-session counts are recorded as high-water marks (only ever increase
// within a session) — the same pattern the existing Claude Code status
// line bash script uses.
package cost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Tally is the on-disk root document.
type Tally struct {
	Sessions  map[string]SessionCost `json:"sessions"`
	StartTime int64                  `json:"start_time"`
}

// SessionCost holds the per-session token totals plus the provider/model
// they came from. We keep provider+model on the cost record so totals can
// be re-priced later without re-reading every session.json.
type SessionCost struct {
	Input    int    `json:"input"`
	Output   int    `json:"output"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// Empty returns a fresh tally rooted at the current time.
func Empty() *Tally {
	return &Tally{
		Sessions:  map[string]SessionCost{},
		StartTime: time.Now().Unix(),
	}
}

// Load reads a tally from path. Missing file returns Empty() with no error
// — first-run is normal.
func Load(path string) (*Tally, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Empty(), nil
		}
		return nil, fmt.Errorf("load tally: %w", err)
	}
	t := Empty()
	if err := json.Unmarshal(data, t); err != nil {
		return nil, fmt.Errorf("decode tally: %w", err)
	}
	if t.Sessions == nil {
		t.Sessions = map[string]SessionCost{}
	}
	return t, nil
}

// Save writes the tally atomically.
func (t *Tally) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tally.*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// PricingFunc returns USD per 1M tokens for (provider, model). The cost
// package doesn't import the provider package directly to avoid coupling,
// so callers (the agent or app shell) supply this lookup.
type PricingFunc func(providerSlug, modelID string) (inputPer1M, outputPer1M float64)

// Tracker is a thread-safe wrapper around a Tally tied to a single
// session. It implements the high-water-mark logic and is the type the
// agent loop and status line both consume.
type Tracker struct {
	path      string
	pricing   PricingFunc
	mu        sync.Mutex
	tally     *Tally
}

func NewTracker(path string, pricing PricingFunc) (*Tracker, error) {
	t, err := Load(path)
	if err != nil {
		return nil, err
	}
	return &Tracker{path: path, pricing: pricing, tally: t}, nil
}

// RecordUsage applies the high-water-mark rule for the given session:
// input and output counts are *replaced* by the max of the existing value
// and the new value. This matches the Claude Code status line behaviour
// where a stream completion's running totals are the source of truth.
func (t *Tracker) RecordUsage(sessionID, providerSlug, modelID string, input, output int) error {
	t.mu.Lock()
	cur := t.tally.Sessions[sessionID]
	if input > cur.Input {
		cur.Input = input
	}
	if output > cur.Output {
		cur.Output = output
	}
	cur.Provider = providerSlug
	cur.Model = modelID
	t.tally.Sessions[sessionID] = cur
	t.mu.Unlock()
	return t.tally.Save(t.path)
}

// SessionCost returns the cumulative USD cost for the named session.
func (t *Tracker) SessionCost(sessionID string) float64 {
	t.mu.Lock()
	sc := t.tally.Sessions[sessionID]
	t.mu.Unlock()
	return t.priced(sc)
}

// TotalCost sums every session's cost using the current pricing function.
// Pricing changes propagate immediately — historical token counts stay,
// but their dollar value is computed at read time.
func (t *Tracker) TotalCost() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	var total float64
	for _, sc := range t.tally.Sessions {
		total += t.priced(sc)
	}
	return total
}

// SessionTokens returns the (input, output) token counts for a session.
func (t *Tracker) SessionTokens(sessionID string) (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	sc := t.tally.Sessions[sessionID]
	return sc.Input, sc.Output
}

// Reset clears the tally and resets start_time.
func (t *Tracker) Reset() error {
	t.mu.Lock()
	t.tally = Empty()
	t.mu.Unlock()
	return t.tally.Save(t.path)
}

// StartTime returns the unix timestamp the current tally window began at.
func (t *Tracker) StartTime() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tally.StartTime
}

// Breakdown returns a snapshot of every session's cost record.
func (t *Tracker) Breakdown() []Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Entry, 0, len(t.tally.Sessions))
	for id, sc := range t.tally.Sessions {
		out = append(out, Entry{
			SessionID: id,
			Input:     sc.Input,
			Output:    sc.Output,
			Provider:  sc.Provider,
			Model:     sc.Model,
			USD:       t.priced(sc),
		})
	}
	return out
}

// Entry is a per-session row in the breakdown.
type Entry struct {
	SessionID string
	Input     int
	Output    int
	Provider  string
	Model     string
	USD       float64
}

func (t *Tracker) priced(sc SessionCost) float64 {
	if t.pricing == nil {
		return 0
	}
	in, out := t.pricing(sc.Provider, sc.Model)
	return float64(sc.Input)*in/1_000_000 + float64(sc.Output)*out/1_000_000
}
