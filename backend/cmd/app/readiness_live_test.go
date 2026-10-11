package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/health"
)

type fakeOutboxCounter struct {
	n       int64
	err     error
	onCount func(ctx context.Context)
}

func (f fakeOutboxCounter) CountBacklog(ctx context.Context, _ time.Time) (int64, error) {
	if f.onCount != nil {
		f.onCount(ctx)
	}
	return f.n, f.err
}

func TestEmailOutboxReadiness(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	t.Run("small backlog", func(t *testing.T) {
		var sawDeadline bool
		got := emailOutboxReadiness(context.Background(), fakeOutboxCounter{
			n: 5,
			onCount: func(ctx context.Context) {
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Fatal("CountBacklog context has no deadline")
				}
				d := time.Until(deadline)
				if d < 500*time.Millisecond || d > 1500*time.Millisecond {
					t.Fatalf("timeout derived from ctx: got %s, want about 1s", d)
				}
				sawDeadline = true
			},
		}, now)
		if !sawDeadline {
			t.Fatal("CountBacklog was not called")
		}
		assertEmailOutbox(t, got, "")
	})

	t.Run("high backlog", func(t *testing.T) {
		got := emailOutboxReadiness(context.Background(), fakeOutboxCounter{n: 5000}, now)
		assertEmailOutbox(t, got, "email_outbox.backlog_high")
	})

	t.Run("count error", func(t *testing.T) {
		got := emailOutboxReadiness(context.Background(), fakeOutboxCounter{err: errors.New("db down")}, now)
		assertEmailOutbox(t, got, "email_outbox.unavailable")
	})

	t.Run("nil counter", func(t *testing.T) {
		got := emailOutboxReadiness(context.Background(), nil, now)
		assertEmailOutbox(t, got, "email_outbox.unavailable")
	})
}

func assertEmailOutbox(t *testing.T, got health.ComponentResult, wantCode string) {
	t.Helper()
	if got.Component != "email_outbox" || got.Status != "ok" || got.Source != "live" || got.Code != wantCode {
		t.Fatalf("got %+v, want component email_outbox status ok source live code %q", got, wantCode)
	}
}

func TestCachedLiveReadinessRefreshesAfterTTL(t *testing.T) {
	clock := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	calls := 0
	cached := newCachedLiveReadiness(readinessLiveCacheTTL, func() time.Time { return clock }, func() []health.ComponentResult {
		calls++
		return []health.ComponentResult{{
			Component: "email_outbox",
			Status:    "ok",
			Source:    "live",
			Code:      "call",
		}}
	})

	first := cached()
	second := cached()
	if calls != 1 {
		t.Fatalf("compute calls within TTL: got %d, want 1", calls)
	}
	if len(first) != 1 || len(second) != 1 || first[0].Component != "email_outbox" || second[0].Source != "live" {
		t.Fatalf("cached results: first=%+v second=%+v", first, second)
	}

	clock = clock.Add(readinessLiveCacheTTL + time.Millisecond)
	third := cached()
	if calls != 2 {
		t.Fatalf("compute calls after TTL: got %d, want 2", calls)
	}
	if len(third) != 1 || third[0].Source != "live" {
		t.Fatalf("refreshed result: %+v", third)
	}
}

func TestReadinessComponentsFromPreflightSourceConfig(t *testing.T) {
	entries := readinessComponentsFromPreflight(config.Report{})
	if len(entries) == 0 {
		t.Fatal("empty preflight report returned no components")
	}
	for _, entry := range entries {
		if entry.Source != "config" {
			t.Fatalf("component %s source: got %q, want config", entry.Component, entry.Source)
		}
		if entry.Status != "ok" {
			t.Fatalf("component %s status: got %q, want ok for an empty report", entry.Component, entry.Status)
		}
		if entry.Code != "" {
			t.Fatalf("component %s code: got %q, want empty for an empty report", entry.Component, entry.Code)
		}
	}
}
