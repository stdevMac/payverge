package aicontract

import (
	"fmt"
	"sync"
)

// EffectRecorder counts named side effects without storing content.
type EffectRecorder struct {
	mu     sync.Mutex
	counts map[string]int
	events []string
}

// NewEffectRecorder constructs an empty recorder.
func NewEffectRecorder() *EffectRecorder {
	return &EffectRecorder{counts: map[string]int{}}
}

// Record increments a privacy-safe effect counter.
func (r *EffectRecorder) Record(name string) {
	if r == nil || name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[name]++
	r.events = append(r.events, name)
}

// Snapshot returns a copy of all counters.
func (r *EffectRecorder) Snapshot() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int, len(r.counts))
	for k, v := range r.counts {
		out[k] = v
	}
	return out
}

// Match returns an error when expected effect counts differ.
func (r *EffectRecorder) Match(expected map[string]int) error {
	got := r.Snapshot()
	for k, want := range expected {
		if got[k] != want {
			return fmt.Errorf("effect %q: want %d got %d", k, want, got[k])
		}
	}
	return nil
}
