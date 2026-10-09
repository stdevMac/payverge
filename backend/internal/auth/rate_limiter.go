package auth

import (
	"net/netip"
	"strings"
	"sync"
	"time"
)

// RateLimiter is a simple sliding-window in-memory rate limiter.
// Counter state lives in process memory; restart resets counters.
// For email-resend gating where the worst case on restart is one
// duplicate email per key, this is acceptable.
type RateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	limit   int
	buckets map[string][]time.Time
}

func NewRateLimiter(window time.Duration, limit int) *RateLimiter {
	rl := &RateLimiter{
		window:  window,
		limit:   limit,
		buckets: make(map[string][]time.Time),
	}
	go rl.sweepLoop()
	return rl
}

// normalizeKey lowercases/trims and buckets IPv6 addresses to their /64: a
// single host rotates SLAAC addresses freely inside its /64, so per-address
// buckets were free limit resets; one /64 is the standard end-user
// allocation. Non-IP keys (emails) and IPv4 pass through unchanged.
func normalizeKey(key string) string {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return ""
	}
	if addr, err := netip.ParseAddr(normalized); err == nil && addr.Is6() && !addr.Is4In6() {
		if prefix, perr := addr.Prefix(64); perr == nil {
			return prefix.String()
		}
	}
	return normalized
}

// Allow records an event for the key and returns true if the caller is
// within the rate limit. Keys are normalized (lowercased, trimmed, IPv6→/64).
func (rl *RateLimiter) Allow(key string) bool {
	normalized := normalizeKey(key)
	if normalized == "" {
		return false
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rl.window)

	events := rl.buckets[normalized]
	filtered := events[:0]
	for _, t := range events {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) >= rl.limit {
		rl.buckets[normalized] = filtered
		return false
	}
	rl.buckets[normalized] = append(filtered, now)
	return true
}

// Peek reports whether the key has budget WITHOUT consuming any. Pair with
// Record for count-only-successes flows: the register cap must not let
// duplicate-email 409s (or validation 400s) exhaust a shared IP's budget.
func (rl *RateLimiter) Peek(key string) bool {
	normalized := normalizeKey(key)
	if normalized == "" {
		return false
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-rl.window)
	count := 0
	for _, t := range rl.buckets[normalized] {
		if t.After(cutoff) {
			count++
		}
	}
	return count < rl.limit
}

// Record consumes one unit of budget for the key without checking the limit.
func (rl *RateLimiter) Record(key string) {
	normalized := normalizeKey(key)
	if normalized == "" {
		return
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.buckets[normalized] = append(rl.buckets[normalized], time.Now())
}

func (rl *RateLimiter) sweepLoop() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.window)
		for key, events := range rl.buckets {
			filtered := events[:0]
			for _, t := range events {
				if t.After(cutoff) {
					filtered = append(filtered, t)
				}
			}
			if len(filtered) == 0 {
				delete(rl.buckets, key)
			} else {
				rl.buckets[key] = filtered
			}
		}
		rl.mu.Unlock()
	}
}
