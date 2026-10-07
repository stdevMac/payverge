package print

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// OrphanSweepWorker scans for orders that are older than a grace window but
// have no associated kitchen-ticket print_job row, and re-enqueues them.
//
// Runs piggy-backed on the same ticker as the retry worker but with a
// longer interval (default 60s) since the typical case is "no orphans".
type OrphanSweepWorker struct {
	db       *gorm.DB
	service  *Service
	interval time.Duration
	grace    time.Duration

	// onSweep is called after each RunOnce invocation (initial and ticker).
	// Intended for tests only; nil in production.
	onSweep func()

	stopCh chan struct{}
	done   chan struct{}
}

// NewOrphanSweepWorker wires a sweep against the given DB + print Service.
// interval defaults to 60s; grace defaults to 30s.
func NewOrphanSweepWorker(db *gorm.DB, service *Service, interval, grace time.Duration) *OrphanSweepWorker {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if grace <= 0 {
		grace = 30 * time.Second
	}
	return &OrphanSweepWorker{
		db:       db,
		service:  service,
		interval: interval,
		grace:    grace,
	}
}

// Start launches the goroutine.
func (w *OrphanSweepWorker) Start(ctx context.Context) {
	if w.stopCh != nil {
		return
	}
	w.stopCh = make(chan struct{})
	w.done = make(chan struct{})
	go w.run(ctx)
}

// Stop signals shutdown and waits for the goroutine.
func (w *OrphanSweepWorker) Stop() {
	if w.stopCh == nil {
		return
	}
	close(w.stopCh)
	<-w.done
	w.stopCh = nil
}

// RunOnce performs a single sweep. Returns the number of orphans recovered.
// onSweep (test hook) is called after every invocation, even on error.
func (w *OrphanSweepWorker) RunOnce(ctx context.Context) (int, error) {
	if w.onSweep != nil {
		defer w.onSweep()
	}
	cutoff := time.Now().Add(-w.grace)

	var orders []database.Order
	// X-2: only orders the kitchen should cook (approved, in_kitchen) and only
	// for businesses with an enabled kitchen/bar printer. The sweep is a retry
	// net behind the approve-time enqueue, not the primary ticket source.
	printableStatuses := []database.OrderStatus{
		database.OrderStatusApproved,
		database.OrderStatusInKitchen,
	}
	printerRoles := []string{"kitchen", "bar"}
	kitchenKinds := []database.PrintJobKind{
		database.PrintJobKindKitchen,
		database.PrintJobKindBar,
	}
	err := w.db.WithContext(ctx).
		Where("created_at <= ? AND created_at > ?", cutoff, time.Now().Add(-24*time.Hour)).
		Where("kitchen_acked_at IS NULL").
		Where("status IN ?", printableStatuses).
		Where("EXISTS (SELECT 1 FROM printers p WHERE p.business_id = orders.business_id AND p.enabled = ? AND p.role IN (?))", true, printerRoles).
		Where("NOT EXISTS (SELECT 1 FROM print_jobs pj WHERE pj.order_id = orders.id AND pj.kind IN (?))", kitchenKinds).
		Limit(50).
		Find(&orders).Error
	if err != nil {
		// Some drivers (older SQLite) reject the NOT EXISTS subquery against
		// orders.id when the outer alias differs. Fall back to a plain query;
		// hasKitchenJob() below covers the per-row filter so correctness is
		// preserved — only the prefetch is less efficient.
		err = w.db.WithContext(ctx).
			Where("created_at <= ? AND created_at > ?", cutoff, time.Now().Add(-24*time.Hour)).
			Where("kitchen_acked_at IS NULL").
			Where("status IN ?", printableStatuses).
			Where("EXISTS (SELECT 1 FROM printers p WHERE p.business_id = orders.business_id AND p.enabled = ? AND p.role IN (?))", true, printerRoles).
			Limit(50).
			Find(&orders).Error
		if err != nil {
			return 0, err
		}
	}

	recovered := 0
	for _, order := range orders {
		if w.hasKitchenJob(ctx, order.ID) {
			continue
		}
		params := EnqueueParams{
			BusinessID: order.BusinessID,
			Kind:       database.PrintJobKindKitchen,
			SourceType: "order",
			SourceID:   order.ID,
			OrderID:    &order.ID,
			CreatedBy:  "orphan_sweep",
		}
		if _, err := w.service.Enqueue(ctx, params); err != nil {
			log.Printf("[print-orphan-sweep] re-enqueue failed order_id=%d: %v", order.ID, err)
			continue
		}
		log.Printf("[print-orphan-sweep] recovered order_id=%d business_id=%d", order.ID, order.BusinessID)
		recovered++
	}
	return recovered, nil
}

func (w *OrphanSweepWorker) hasKitchenJob(ctx context.Context, orderID uint) bool {
	var count int64
	err := w.db.WithContext(ctx).
		Model(&database.PrintJob{}).
		Where("order_id = ? AND kind IN ?", orderID,
			[]database.PrintJobKind{database.PrintJobKindKitchen, database.PrintJobKindBar}).
		Count(&count).Error
	if err != nil {
		return false
	}
	return count > 0
}

func (w *OrphanSweepWorker) run(ctx context.Context) {
	defer close(w.done)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Run once immediately on start so a restart doesn't leave orphaned kitchen
	// tickets unrecovered for a full interval (matches RetryWorker.run).
	logger.SafeTick("print-orphan-sweep", func() {
		if _, err := w.RunOnce(ctx); err != nil {
			log.Printf("[print-orphan-sweep] initial sweep error: %v", err)
		}
	})

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			logger.SafeTick("print-orphan-sweep", func() {
				if _, err := w.RunOnce(ctx); err != nil {
					log.Printf("[print-orphan-sweep] sweep error: %v", err)
				}
			})
		}
	}
}
