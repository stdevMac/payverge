// Package circuitbreaker is a minimal per-upstream half-open breaker so a
// hard-down PSP/LLM upstream fast-fails instead of re-paying the full timeout
// on every concurrent request (EXT-5).
package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// ErrOpen is returned when the breaker is open and the call is short-circuited.
var ErrOpen = errors.New("circuitbreaker: upstream temporarily unavailable")

// Config holds tunables for a Breaker.
type Config struct {
	Threshold int           // consecutive failures before opening (default 5)
	OpenFor   time.Duration // how long to stay open before a half-open probe (default 30s)
}

// Breaker is a concurrency-safe half-open circuit breaker.
type Breaker struct {
	mu        sync.Mutex
	threshold int
	openFor   time.Duration
	failures  int
	openedAt  time.Time
	halfOpen  bool
}

// New constructs a Breaker with the given Config. Zero values for Threshold and
// OpenFor are replaced with safe defaults (5 failures, 30 s).
func New(cfg Config) *Breaker {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 5
	}
	if cfg.OpenFor <= 0 {
		cfg.OpenFor = 30 * time.Second
	}
	return &Breaker{threshold: cfg.Threshold, openFor: cfg.OpenFor}
}

// Do runs fn unless the breaker is open. A nil error from fn closes the
// breaker; a non-nil error counts toward the threshold. When the breaker is
// open, Do returns ErrOpen without calling fn.
func (b *Breaker) Do(fn func() error) error {
	b.mu.Lock()
	if !b.openedAt.IsZero() {
		if time.Since(b.openedAt) < b.openFor {
			b.mu.Unlock()
			return ErrOpen
		}
		// Window elapsed: enter half-open and let probes through. Concurrent
		// callers in this window may each probe (bounded to one burst per
		// OpenFor); the first non-nil result re-opens, a nil result closes.
		b.halfOpen = true
		b.openedAt = time.Time{}
	}
	// Capture whether THIS call is a half-open probe before releasing the lock.
	// fn() runs unlocked, so by the time we re-lock to record the result the
	// shared b.halfOpen may have been cleared by a concurrent probe that
	// finished first. Deciding re-open off this local flag (not b.halfOpen)
	// guarantees a failed probe still re-opens even if a sibling success raced
	// ahead and closed the breaker — the documented "first non-nil result
	// re-opens" contract held per-probe rather than lost to interleaving.
	probing := b.halfOpen
	b.mu.Unlock()

	err := fn()

	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		b.failures = 0
		// Only a half-open probe closes the breaker; a stray closed-state
		// success just resets the counter and must not clear a half-open state
		// that a concurrent probe failure is about to act on.
		if probing {
			b.halfOpen = false
		}
		return nil
	}
	b.failures++
	if probing || b.failures >= b.threshold {
		b.openedAt = time.Now()
		b.halfOpen = false
	}
	return err
}
