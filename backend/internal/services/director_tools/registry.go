package director_tools

import (
	"fmt"
	"sort"
	"sync"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

// Registry holds the set of tools wired into the Director Console.
// It is built once at service init and read-only thereafter; Register
// panics on duplicate names so a misconfiguration fails fast on boot.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	if t == nil {
		panic("director_tools: nil tool")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[t.Name()]; exists {
		panic(fmt.Sprintf("director_tools: duplicate tool registration %q", t.Name()))
	}
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Declarations returns the tool set as neutral llm.Tool values, sorted by name
// for stable output.
func (r *Registry) Declarations() []llm.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]llm.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, llm.Tool{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Names returns all registered tool names — handy for debugging.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
