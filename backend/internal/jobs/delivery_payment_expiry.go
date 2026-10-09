package jobs

import (
	"log"
	"time"

	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// DeliveryPaymentExpiry cancels prepay delivery orders whose 15-minute
// payment window lapsed without payment (spec §3.1). Runs every minute;
// the sweep itself is idempotent and bounded (LIMIT 200).
type DeliveryPaymentExpiry struct {
	svc      *services.DeliveryService
	interval time.Duration
	stopCh   chan struct{}
	done     chan struct{}
}

func NewDeliveryPaymentExpiry(svc *services.DeliveryService, interval time.Duration) *DeliveryPaymentExpiry {
	if interval <= 0 {
		interval = time.Minute
	}
	return &DeliveryPaymentExpiry{svc: svc, interval: interval}
}

// Start launches the expiry goroutine. Idempotent.
func (j *DeliveryPaymentExpiry) Start() {
	if j.stopCh != nil {
		return
	}
	stopCh := make(chan struct{})
	done := make(chan struct{})
	j.stopCh = stopCh
	j.done = done
	go j.run(stopCh, done)
}

// Stop signals shutdown and waits for the goroutine.
func (j *DeliveryPaymentExpiry) Stop() {
	if j.stopCh == nil {
		return
	}
	close(j.stopCh)
	<-j.done
	j.stopCh = nil
	j.done = nil
}

func (j *DeliveryPaymentExpiry) run(stopCh, done chan struct{}) {
	defer close(done)
	// SafeTick recovers a panicking sweep so it never unwinds this goroutine
	// and crashes the whole process (all tenants).
	logger.SafeTick("delivery-payment-expiry", j.RunOnce)
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			logger.SafeTick("delivery-payment-expiry", j.RunOnce)
		}
	}
}

// RunOnce executes a single sweep. Exported for tests.
// Also reconciles paid-but-still-confirmed deliveries so a failed post-pay
// HandleDeliveryBillPaid hook cannot leave kitchen work stranded forever
// (payment_expires_at may still be in the future for those rows).
func (j *DeliveryPaymentExpiry) RunOnce() {
	if _, err := j.svc.ReconcilePaidDeliveries(100); err != nil {
		log.Printf("[delivery-payment-expiry] paid-delivery reconcile failed: %v", err)
	}
	if err := j.svc.ExpireUnpaidDeliveries(); err != nil {
		log.Printf("[delivery-payment-expiry] sweep failed: %v", err)
	}
}
