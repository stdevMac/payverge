package print

import (
	"context"
	"testing"
	"time"
)

// newTestOrphanSweepWorker creates an OrphanSweepWorker backed by an
// in-memory SQLite DB, ready for use in tests.
func newTestOrphanSweepWorker(t *testing.T, interval time.Duration) *OrphanSweepWorker {
	t.Helper()
	db := setupTestDB(t)
	svc := NewService(db)
	return NewOrphanSweepWorker(db, svc, interval, 30*time.Second)
}

// TestOrphanSweepRunsImmediatelyOnStart asserts RunOnce fires before the first
// tick interval elapses after Start.
func TestOrphanSweepRunsImmediatelyOnStart(t *testing.T) {
	w := newTestOrphanSweepWorker(t, 10*time.Minute) // long interval — first tick never fires
	swept := make(chan struct{}, 1)
	w.onSweep = func() {
		select {
		case swept <- struct{}{}:
		default:
		}
	}
	w.Start(context.Background())
	defer w.Stop()
	select {
	case <-swept:
		// immediate sweep fired — pass
	case <-time.After(2 * time.Second):
		t.Fatal("orphan sweep did not run immediately on start")
	}
}

// TestOrphanSweepRunsOnTick asserts RunOnce also fires on each ticker tick so
// the worker self-heals on a regular cadence (not just at startup).
func TestOrphanSweepRunsOnTick(t *testing.T) {
	w := newTestOrphanSweepWorker(t, 20*time.Millisecond) // short interval
	count := 0
	done := make(chan struct{})
	w.onSweep = func() {
		count++
		if count >= 2 {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}
	w.Start(context.Background())
	defer w.Stop()
	select {
	case <-done:
		// at least 2 sweeps (1 immediate + at least 1 tick) — pass
	case <-time.After(3 * time.Second):
		t.Fatalf("expected at least 2 sweeps (immediate + 1 tick), got %d", count)
	}
}

// TestOrphanSweepNoOrphansOnEmptyDB asserts RunOnce returns 0 with no error
// when there are no qualifying orders in the DB.
func TestOrphanSweepNoOrphansOnEmptyDB(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	w := NewOrphanSweepWorker(db, svc, time.Minute, 30*time.Second)
	n, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce on empty DB: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 orphans on empty DB, got %d", n)
	}
}
