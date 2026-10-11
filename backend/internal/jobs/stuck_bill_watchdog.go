package jobs

import (
	"log"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"gorm.io/gorm"
)

// StaleBillThreshold mirrors the constant used by Today's Briefings.
// Defined here so the watchdog and the briefing source agree without an
// import cycle into internal/server.
const StaleBillThreshold = 120 * time.Minute

// StuckBillWatchdog scans for open bills older than StaleBillThreshold
// and publishes an SSE `bill.stuck` event per affected business. The
// frontend's briefings panel refetches when this event fires, so the
// "N open bills are over 2 hours old" alert appears in realtime
// instead of waiting on the briefing endpoint's 60s poll cadence.
//
// We deliberately do NOT mutate the bill's status enum here — flipping
// status=open -> status=stale would change every query that filters
// status=open (kitchen, payments, fiscal, reports). The briefing
// endpoint already detects stale bills by age; the watchdog's only job
// is realtime visibility.
type StuckBillWatchdog struct {
	db        *gorm.DB
	interval  time.Duration
	threshold time.Duration

	stopCh chan struct{}
	done   chan struct{}

	// lastCounts tracks the per-business stuck-bill count from the previous
	// sweep so repeat sweeps over the same stuck set do not republish
	// bill.stuck (the frontend toasts on each event — without dedup operators
	// get the same alert every 5 minutes). Only a count INCREASE (a new bill
	// crossed the threshold) republishes. A per-process seen-set is
	// intentional: after a restart the first sweep re-alerts once, which is
	// the desired "fresh process, fresh visibility" behavior.
	mu         sync.Mutex
	lastCounts map[uint]int64
}

// StuckBillWatchdogConfig knobs.
type StuckBillWatchdogConfig struct {
	Interval  time.Duration // default 5 minutes
	Threshold time.Duration // default StaleBillThreshold (2 hours)
}

// NewStuckBillWatchdog builds a watchdog backed by db.
func NewStuckBillWatchdog(db *gorm.DB, cfg StuckBillWatchdogConfig) *StuckBillWatchdog {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Minute
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = StaleBillThreshold
	}
	return &StuckBillWatchdog{
		db:        db,
		interval:  cfg.Interval,
		threshold: cfg.Threshold,
	}
}

// Start launches the watchdog goroutine. Idempotent.
func (w *StuckBillWatchdog) Start() {
	if w.stopCh != nil {
		return
	}
	stopCh := make(chan struct{})
	done := make(chan struct{})
	w.stopCh = stopCh
	w.done = done
	go w.run(stopCh, done)
}

// Stop signals shutdown and waits for the goroutine.
func (w *StuckBillWatchdog) Stop() {
	if w.stopCh == nil {
		return
	}
	close(w.stopCh)
	<-w.done
	w.stopCh = nil
	w.done = nil
}

func (w *StuckBillWatchdog) run(stopCh, done chan struct{}) {
	defer close(done)

	// Initial sweep so a fresh process starts surfacing stuck bills
	// immediately instead of waiting one tick. SafeTick recovers a panicking
	// sweep so it never unwinds this goroutine and crashes the whole process.
	logger.SafeTick("stuck-bill-watchdog", w.RunOnce)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			logger.SafeTick("stuck-bill-watchdog", w.RunOnce)
		}
	}
}

// RunOnce executes a single sweep. Exported for tests.
//
// Aggregates stuck-bill counts per business in a single query, then
// publishes one event per business that has at least one stuck bill.
// `updated_at` (not created_at) is the staleness signal — staff edits,
// alt-payment attempts, etc. all reset the clock so a long-running
// dinner party doesn't trigger spurious alerts.
func (w *StuckBillWatchdog) RunOnce() {
	cutoff := time.Now().Add(-w.threshold)

	type row struct {
		BusinessID uint
		Count      int64
	}
	var rows []row
	if err := w.db.Table("bills").
		Select("business_id, COUNT(*) as count").
		Where("status = ? AND updated_at < ?", database.BillStatusOpen, cutoff).
		Group("business_id").
		Scan(&rows).Error; err != nil {
		log.Printf("[stuck-bill-watchdog] query failed: %v", err)
		return
	}

	w.mu.Lock()
	prev := w.lastCounts
	next := make(map[uint]int64, len(rows))
	var toPublish []row
	for _, r := range rows {
		next[r.BusinessID] = r.Count
		// Publish only when a NEW bill crossed the threshold since the last
		// sweep (count increased, or the business newly has stuck bills). A
		// business whose count dropped or cleared is removed from the map by
		// the replacement below, so a later re-stick alerts again.
		if r.Count > prev[r.BusinessID] {
			toPublish = append(toPublish, r)
		}
	}
	w.lastCounts = next
	w.mu.Unlock()

	if len(toPublish) == 0 {
		return
	}

	hub := events.GetHub()
	for _, r := range toPublish {
		hub.PublishJSON(r.BusinessID, "bill.stuck", map[string]interface{}{
			"count":             r.Count,
			"threshold_minutes": int(w.threshold / time.Minute),
		})
	}
}
