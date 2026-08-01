package computers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// registryVersion is bumped when the on-disk shape changes incompatibly.
// Readers refuse a newer version rather than silently misinterpreting it.
const registryVersion = 1

// FileName is the registry file inside the computers directory.
const FileName = "registry.json"

type registryFile struct {
	Version   int        `json:"version"`
	Computers []Computer `json:"computers"`
}

// Registry is the durable set of Packet Computers, backed by
// <home>/computers/registry.json.
//
// It is data-only: loading, listing, and saving records. It never contacts a
// machine, so every method here is local filesystem work.
type Registry struct {
	mu   sync.RWMutex
	dir  string
	byID map[string]Computer
}

// Load reads the registry from dir, creating an empty one if the file does
// not exist. A malformed or future-versioned file is an error rather than a
// silent reset — losing a machine list quietly is worse than failing loudly.
func Load(dir string) (*Registry, error) {
	r := &Registry{dir: dir, byID: map[string]Computer{}}
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("computers: empty registry dir")
	}
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, fmt.Errorf("computers: read registry: %w", err)
	}
	var f registryFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("computers: parse registry: %w", err)
	}
	if f.Version > registryVersion {
		return nil, fmt.Errorf(
			"computers: registry version %d is newer than this build supports (%d)",
			f.Version, registryVersion,
		)
	}
	now := time.Now().UTC()
	for _, c := range f.Computers {
		norm, nerr := c.normalize(now)
		if nerr != nil {
			// Skip an unusable row rather than failing the whole registry;
			// one bad record must not hide the others.
			continue
		}
		// Preserve stored timestamps: normalize stamps UpdatedAt as a write
		// helper, which is wrong on a pure read.
		norm.UpdatedAt = c.UpdatedAt
		if !c.CreatedAt.IsZero() {
			norm.CreatedAt = c.CreatedAt
		}
		r.byID[norm.ID] = norm
	}
	return r, nil
}

// Dir returns the directory backing this registry.
func (r *Registry) Dir() string { return r.dir }

// List returns all computers sorted by name.
func (r *Registry) List() []Computer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Computer, 0, len(r.byID))
	for _, c := range r.byID {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Get returns the computer with the given name (case-insensitive).
func (r *Registry) Get(name string) (Computer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	want := strings.ToLower(strings.TrimSpace(name))
	for _, c := range r.byID {
		if strings.ToLower(c.Name) == want {
			return c, true
		}
	}
	return Computer{}, false
}

// Upsert validates and stores c, then persists the registry. Names are
// unique case-insensitively so `/computers <name>` is unambiguous.
func (r *Registry) Upsert(c Computer) (Computer, error) {
	now := time.Now().UTC()
	norm, err := c.normalize(now)
	if err != nil {
		return Computer{}, err
	}

	r.mu.Lock()
	for id, existing := range r.byID {
		if id != norm.ID && strings.EqualFold(existing.Name, norm.Name) {
			r.mu.Unlock()
			return Computer{}, &ErrInvalid{
				Reason: fmt.Sprintf("name %q is already used by computer %s", norm.Name, id),
			}
		}
	}
	if prev, ok := r.byID[norm.ID]; ok && !prev.CreatedAt.IsZero() {
		norm.CreatedAt = prev.CreatedAt
	}
	r.byID[norm.ID] = norm
	r.mu.Unlock()

	if err := r.save(); err != nil {
		return Computer{}, err
	}
	return norm, nil
}

// Remove deletes a computer by name and persists the result.
func (r *Registry) Remove(name string) (bool, error) {
	r.mu.Lock()
	want := strings.ToLower(strings.TrimSpace(name))
	var found string
	for id, c := range r.byID {
		if strings.ToLower(c.Name) == want {
			found = id
			break
		}
	}
	if found == "" {
		r.mu.Unlock()
		return false, nil
	}
	delete(r.byID, found)
	r.mu.Unlock()

	if err := r.save(); err != nil {
		return false, err
	}
	return true, nil
}

// save writes the registry with atomic temp-file-then-rename semantics,
// matching how sessions and jobs persist.
func (r *Registry) save() error {
	r.mu.RLock()
	f := registryFile{Version: registryVersion, Computers: make([]Computer, 0, len(r.byID))}
	for _, c := range r.byID {
		f.Computers = append(f.Computers, c)
	}
	dir := r.dir
	r.mu.RUnlock()

	sort.Slice(f.Computers, func(i, j int) bool {
		return strings.ToLower(f.Computers[i].Name) < strings.ToLower(f.Computers[j].Name)
	})

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("computers: ensure dir: %w", err)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("computers: marshal registry: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".registry.*.json.tmp")
	if err != nil {
		return fmt.Errorf("computers: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("computers: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("computers: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, FileName)); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("computers: rename: %w", err)
	}
	return nil
}
