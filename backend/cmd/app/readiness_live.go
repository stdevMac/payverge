package main

import (
	"context"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/health"
)

// outboxBacklogCounter is the slice of the email outbox store the readiness
// probe needs. *emails.GormOutboxStore satisfies it.
type outboxBacklogCounter interface {
	CountBacklog(ctx context.Context, now time.Time) (int64, error)
}

const emailOutboxBacklogWarn = 1000

const readinessLiveCacheTTL = 15 * time.Second

// emailOutboxReadiness reports the due email outbox backlog. A large backlog
// or a failed count is surfaced as a code but stays "ok": a mail backlog must
// not pull the API out of rotation (the database component already covers DB loss).
func emailOutboxReadiness(ctx context.Context, counter outboxBacklogCounter, now time.Time) health.ComponentResult {
	result := health.ComponentResult{
		Component: "email_outbox",
		Status:    "ok",
		Source:    "live",
	}
	if counter == nil {
		result.Code = "email_outbox.unavailable"
		return result
	}

	countCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	count, err := counter.CountBacklog(countCtx, now)
	if err != nil {
		result.Code = "email_outbox.unavailable"
		return result
	}
	if count >= emailOutboxBacklogWarn {
		result.Code = "email_outbox.backlog_high"
	}
	return result
}

// newCachedLiveReadiness returns a probe function that runs compute at most
// once per ttl. The unauthenticated public readiness route calls it on every
// request; the cache keeps that from issuing a COUNT each time.
func newCachedLiveReadiness(ttl time.Duration, now func() time.Time, compute func() []health.ComponentResult) func() []health.ComponentResult {
	var (
		mu       sync.Mutex
		cached   []health.ComponentResult
		cachedAt time.Time
		has      bool
	)
	return func() []health.ComponentResult {
		mu.Lock()
		defer mu.Unlock()
		t := now()
		if has && t.Sub(cachedAt) < ttl {
			return cached
		}
		cached = compute()
		cachedAt = t
		has = true
		return cached
	}
}
