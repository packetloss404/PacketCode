package tools

import (
	"sort"
	"sync"

	"github.com/packetcode/packetcode/internal/provider"
)

// Registry holds the active set of tools. It is safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

// Register adds (or replaces) a tool by name.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get returns the named tool.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// All returns every registered tool in name-sorted order. Sorting keeps
// the LLM-facing definition list deterministic, which is friendly to
// prompt caching once we add it.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Tool, len(names))
	for i, n := range names {
		out[i] = r.tools[n]
	}
	return out
}

// Definitions translates the registry into the format LLM providers expect
// when announcing available tools at the start of a chat completion.
func (r *Registry) Definitions() []provider.ToolDefinition {
	tools := r.All()
	out := make([]provider.ToolDefinition, len(tools))
	for i, t := range tools {
		out[i] = provider.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		}
	}
	return out
}
