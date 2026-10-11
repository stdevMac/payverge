package services

import (
	"fmt"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DefaultBillAbandonThreshold is how long an open/partial bill may sit before
// the lifecycle sweeper marks it abandoned. Distinct from the 2h *surfacing*
// threshold used by Today's Briefings / stuck-bill watchdog — abandon is a
// larger, separate decision.
const DefaultBillAbandonThreshold = 24 * time.Hour

const (
	defaultBillLifecycleInterval  = 15 * time.Minute
	defaultBillLifecycleBatchSize = 100
)

// BillLifecycleConfig knobs for the abandon sweeper.
type BillLifecycleConfig struct {
	// Interval between sweeps (default 15m).
	Interval time.Duration
	// DefaultThreshold applied when ThresholdForBusiness is nil or returns 0.
	DefaultThreshold time.Duration
	// BatchSize caps bills abandoned per RunOnce (default 100).
	BatchSize int
	// ThresholdForBusiness optional per-business override. Return 0 to use DefaultThreshold.
	ThresholdForBusiness func(businessID uint) time.Duration
	// Notify is invoked once per bill, immediately AFTER a successful abandon
	// transition. When nil, a bill.abandoned SSE is published. It never fires
	// for a bill the sweeper refused to abandon — announcing "abandoned" every
	// tick for a check that stays open was the #798 spam.
	Notify func(businessID uint, bill database.Bill, age time.Duration)
}

// BillLifecycleSweeper marks long-open bills abandoned so they leave the live set.
type BillLifecycleSweeper struct {
	db                   *gorm.DB
	interval             time.Duration
	defaultThreshold     time.Duration
	batchSize            int
	thresholdForBusiness func(businessID uint) time.Duration
	notify               func(businessID uint, bill database.Bill, age time.Duration)

	stopCh  chan struct{}
	done    chan struct{}
	mu      sync.Mutex
	running bool
}

// NewBillLifecycleSweeper builds a sweeper. db may be nil only for tests that never Start/RunOnce.
func NewBillLifecycleSweeper(db *gorm.DB, cfg BillLifecycleConfig) *BillLifecycleSweeper {
	if cfg.Interval <= 0 {
		cfg.Interval = defaultBillLifecycleInterval
	}
	if cfg.DefaultThreshold <= 0 {
		cfg.DefaultThreshold = DefaultBillAbandonThreshold
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultBillLifecycleBatchSize
	}
	notify := cfg.Notify
	if notify == nil {
		notify = defaultBillAbandonedNotify
	}
	return &BillLifecycleSweeper{
		db:                   db,
		interval:             cfg.Interval,
		defaultThreshold:     cfg.DefaultThreshold,
		batchSize:            cfg.BatchSize,
		thresholdForBusiness: cfg.ThresholdForBusiness,
		notify:               notify,
	}
}

func defaultBillAbandonedNotify(businessID uint, bill database.Bill, age time.Duration) {
	hub := events.GetHub()
	if hub == nil {
		return
	}
	hub.PublishJSON(businessID, "bill.abandoned", map[string]interface{}{
		"bill_id":     bill.ID,
		"bill_number": bill.BillNumber,
		"table_id":    bill.TableID,
		"age_minutes": int(age / time.Minute),
	})
}

// Start launches the background loop. Idempotent.
func (s *BillLifecycleSweeper) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.done = make(chan struct{})
	go s.loop(s.stopCh, s.done)
}

// Stop signals shutdown and waits for the loop.
func (s *BillLifecycleSweeper) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	stopCh, done := s.stopCh, s.done
	s.mu.Unlock()
	close(stopCh)
	<-done
	s.mu.Lock()
	s.running = false
	s.stopCh = nil
	s.done = nil
	s.mu.Unlock()
}

func (s *BillLifecycleSweeper) loop(stopCh, done chan struct{}) {
	defer close(done)
	logger.SafeTick("bill-lifecycle-sweeper", func() {
		if _, err := s.RunOnce(time.Now()); err != nil {
			logger.Logger.Warnf("bill-lifecycle initial sweep: %v", err)
		}
	})
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			logger.SafeTick("bill-lifecycle-sweeper", func() {
				if _, err := s.RunOnce(time.Now()); err != nil {
					logger.Logger.Warnf("bill-lifecycle sweep: %v", err)
				}
			})
		}
	}
}

// RunOnce abandons up to BatchSize eligible open bills. Returns the number abandoned.
//
// Eligibility:
//   - status IN (open, partial)
//   - abandoned_at IS NULL
//   - created_at older than the (per-business or default) threshold
//   - no payment / alternative_payment still pending (in flight)
func (s *BillLifecycleSweeper) RunOnce(now time.Time) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("bill lifecycle: nil db")
	}
	// Scan cutoff: default threshold, or a 1h floor when per-business overrides
	// exist so tighter overrides still match; each candidate is re-filtered.
	scanThreshold := s.defaultThreshold
	if s.thresholdForBusiness != nil && scanThreshold > time.Hour {
		scanThreshold = time.Hour
	}
	scanCutoff := now.Add(-scanThreshold)

	// Unpaid remaining is a hard refusal inside abandonBill (leftover money is
	// settle, not write-off — #704/762), so bills with money outstanding can
	// never be swept. Excluding them in SQL keeps the sweeper from locking and
	// re-inspecting the same permanent leftovers on every tick (#798).
	var candidates []database.Bill
	err := s.db.
		Where("status IN ? AND abandoned_at IS NULL AND created_at < ? AND total_amount - paid_amount <= 0",
			[]string{string(database.BillStatusOpen), string(database.BillStatusPartial)},
			scanCutoff).
		Order("created_at ASC, id ASC").
		Limit(s.batchSize * 4). // over-fetch; payment-in-flight + threshold filter reduce set
		Find(&candidates).Error
	if err != nil {
		return 0, fmt.Errorf("bill lifecycle load: %w", err)
	}

	abandoned := 0
	for i := range candidates {
		if abandoned >= s.batchSize {
			break
		}
		bill := candidates[i]
		threshold := s.thresholdFor(bill.BusinessID)
		if !bill.CreatedAt.Before(now.Add(-threshold)) {
			continue
		}
		if s.hasPaymentInFlight(bill.ID) {
			continue
		}
		age := now.Sub(bill.CreatedAt)
		if err := s.abandonBill(bill, now); err != nil {
			logger.Logger.Warnf("bill lifecycle abandon bill %d: %v", bill.ID, err)
			continue
		}
		// Notify only after the transition actually committed. abandonBill's
		// guard (status open/partial AND abandoned_at IS NULL) makes success
		// unique per bill, so the operator hears "abandoned" exactly once —
		// never re-emitted on the next tick, never for a refused candidate
		// that stays open (#798).
		if s.notify != nil {
			s.notify(bill.BusinessID, bill, age)
		}
		abandoned++
	}
	return abandoned, nil
}

func (s *BillLifecycleSweeper) thresholdFor(businessID uint) time.Duration {
	if s.thresholdForBusiness != nil {
		if d := s.thresholdForBusiness(businessID); d > 0 {
			return d
		}
	}
	return s.defaultThreshold
}

func (s *BillLifecycleSweeper) hasPaymentInFlight(billID uint) bool {
	var payCount int64
	if err := s.db.Model(&database.Payment{}).
		Where("bill_id = ? AND status IN ?", billID, []database.PaymentStatus{
			database.PaymentStatusPending,
			database.PaymentStatusRefundPending,
		}).
		Count(&payCount).Error; err != nil {
		// Fail closed: treat query errors as in-flight so we never abandon mid-pay.
		logger.Logger.Warnf("bill lifecycle payment-in-flight check bill %d: %v", billID, err)
		return true
	}
	if payCount > 0 {
		return true
	}
	var altCount int64
	if err := s.db.Model(&database.AlternativePayment{}).
		Where("bill_id = ? AND status = ?", billID, database.AltPaymentStatusPending).
		Count(&altCount).Error; err != nil {
		logger.Logger.Warnf("bill lifecycle alt-payment-in-flight check bill %d: %v", billID, err)
		return true
	}
	return altCount > 0
}

func (s *BillLifecycleSweeper) abandonBill(bill database.Bill, now time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var locked database.Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status IN ? AND abandoned_at IS NULL",
				bill.ID,
				[]string{string(database.BillStatusOpen), string(database.BillStatusPartial)},
			).
			First(&locked).Error; err != nil {
			return err
		}
		// Re-check in-flight inside the transaction.
		var payCount, altCount int64
		if err := tx.Model(&database.Payment{}).
			Where("bill_id = ? AND status IN ?", locked.ID, []database.PaymentStatus{
				database.PaymentStatusPending,
				database.PaymentStatusRefundPending,
			}).Count(&payCount).Error; err != nil {
			return err
		}
		if payCount > 0 {
			return fmt.Errorf("payment in flight")
		}
		if err := tx.Model(&database.AlternativePayment{}).
			Where("bill_id = ? AND status = ?", locked.ID, database.AltPaymentStatusPending).
			Count(&altCount).Error; err != nil {
			return err
		}
		if altCount > 0 {
			return fmt.Errorf("alt payment in flight")
		}

		// Do not flip a check abandoned while expo still owes it food, or
		// while a guest send is still waiting for approval. That is how
		// in_kitchen tickets sat on Available tables (#704).
		var liveKitchen, pendingOrders int64
		if err := tx.Model(&database.Order{}).
			Where("bill_id = ? AND status IN ?", locked.ID, database.KitchenLiveOrderStatuses()).
			Count(&liveKitchen).Error; err != nil {
			return err
		}
		if liveKitchen > 0 {
			return fmt.Errorf("live kitchen tickets")
		}
		if err := tx.Model(&database.Order{}).
			Where("bill_id = ? AND status = ?", locked.ID, database.OrderStatusPending).
			Count(&pendingOrders).Error; err != nil {
			return err
		}
		if pendingOrders > 0 {
			return fmt.Errorf("pending guest orders")
		}
		// After order_delivered the kitchen count is 0, but leftover money is
		// still settle — not a sweeper write-off (#704 / 762). Delivery walk-out
		// is the only unpaid-abandon door (AbandonUnpaidOpenBill).
		remaining := locked.TotalAmount - locked.PaidAmount
		if remaining < 0 {
			remaining = 0
		}
		if remaining > 0 {
			return fmt.Errorf("unpaid remaining")
		}

		// Abandoned is a terminal state: stamp closed_at (append-once) alongside
		// abandoned_at, matching AbandonUnpaidOpenBill. Leaving closed_at NULL
		// left swept leftovers half-terminal (#798 bills 1134/1139/1141).
		// settled_at is deliberately NOT written — the sweeper settles no money.
		if err := tx.Model(&database.Bill{}).Where("id = ?", locked.ID).
			Updates(map[string]interface{}{
				"status":       database.BillStatusAbandoned,
				"abandoned_at": now,
				"closed_at":    gorm.Expr("COALESCE(closed_at, ?)", now),
				"updated_at":   now,
			}).Error; err != nil {
			return err
		}

		event := database.BillHistoryEvent{
			BillID:     locked.ID,
			BusinessID: locked.BusinessID,
			EventType:  database.BillHistoryEventBillAbandoned,
			Actor:      "system",
			Reason:     "stale open bill abandoned by lifecycle sweeper",
			Details: map[string]interface{}{
				"abandoned_at": now.UTC().Format(time.RFC3339),
				"age_minutes":  int(now.Sub(locked.CreatedAt) / time.Minute),
			},
			CreatedAt: now,
		}
		if err := tx.Create(&event).Error; err != nil {
			// History is best-effort for audit; do not roll back the abandon.
			logger.Logger.Warnf("bill lifecycle history for bill %d: %v", locked.ID, err)
		}
		return nil
	})
}
