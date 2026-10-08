package server

import (
	"sync"
	"time"
)

// spaceScanMaxPairFailures is how many pair-code attempts a pending scan
// session accepts before its pair code is expired. Six digits with five
// guesses per code keeps a guessing attacker's odds at 1 in 200,000.
const spaceScanMaxPairFailures = 5

// spaceScanPairAttemptTTL bounds how long an attempt entry is remembered. It
// outlives any scan session, so pruning never reopens a live session's budget.
const spaceScanPairAttemptTTL = 24 * time.Hour

type spaceScanPairAttempts struct {
	count int
	first time.Time
}

// spaceScanPairAttemptGuard counts pair-code attempts per pending session in
// process memory. Only sessions still waiting_for_phone are counted (callers
// gate on status), and reaching the cap expires the pending pair code in the
// database (see database.ExpireSpaceScanPairCode), so the bound survives a
// restart for the session that hit it. It never tears down session state.
type spaceScanPairAttemptGuard struct {
	mu     sync.Mutex
	limit  int
	counts map[uint]spaceScanPairAttempts
}

var spaceScanPairFailures = newSpaceScanPairAttemptGuard(spaceScanMaxPairFailures)

func newSpaceScanPairAttemptGuard(limit int) *spaceScanPairAttemptGuard {
	return &spaceScanPairAttemptGuard{limit: limit, counts: map[uint]spaceScanPairAttempts{}}
}

// reserve atomically claims one attempt for the session before the code is
// compared. It returns the attempt number and whether the attempt may proceed;
// once the cap is reached every further caller is refused, so concurrent
// requests cannot exceed it (check and increment happen under one lock).
func (g *spaceScanPairAttemptGuard) reserve(sessionID uint) (int, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if len(g.counts) > 10000 {
		for id, a := range g.counts {
			if now.Sub(a.first) >= spaceScanPairAttemptTTL {
				delete(g.counts, id)
			}
		}
	}
	a, ok := g.counts[sessionID]
	if !ok {
		a = spaceScanPairAttempts{first: now}
	}
	if a.count >= g.limit {
		return a.count, false
	}
	a.count++
	g.counts[sessionID] = a
	return a.count, true
}

// forget clears the session after a successful pairing.
func (g *spaceScanPairAttemptGuard) forget(sessionID uint) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.counts, sessionID)
}
