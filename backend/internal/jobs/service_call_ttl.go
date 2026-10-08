package jobs

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/logger"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
)

// ServiceCallTTLJanitor resolves open waiter calls past the seating SLA so
// Tables Live View, the sidebar badge, and guest QR chrome drop them even
// if nobody polls. Empty-table and newer-bill check-please expire; unpaid
// same-seating check-please and claimed calls stay. Read paths also hide
// lapsed calls; this persists the resolve and publishes alert.resolved.
type ServiceCallTTLJanitor struct {
	svc      *operational_alerts.Service
	interval time.Duration

	stopCh chan struct{}
	done   chan struct{}
}

// NewServiceCallTTLJanitor builds a janitor. interval defaults to 2 minutes.
func NewServiceCallTTLJanitor(db *gorm.DB, interval time.Duration) *ServiceCallTTLJanitor {
	if interval <= 0 {
		interval = 2 * time.Minute
	}
	return &ServiceCallTTLJanitor{
		svc:      operational_alerts.NewService(db),
		interval: interval,
	}
}

// Start launches the janitor goroutine. Idempotent.
func (j *ServiceCallTTLJanitor) Start() {
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
func (j *ServiceCallTTLJanitor) Stop() {
	if j.stopCh == nil {
		return
	}
	close(j.stopCh)
	<-j.done
	j.stopCh = nil
	j.done = nil
}

func (j *ServiceCallTTLJanitor) run(stopCh, done chan struct{}) {
	defer close(done)
	logger.SafeTick("service-call-ttl", j.RunOnce)
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			logger.SafeTick("service-call-ttl", j.RunOnce)
		}
	}
}

// RunOnce expires one bounded batch of stale service calls. Exported for tests.
func (j *ServiceCallTTLJanitor) RunOnce() {
	if j == nil || j.svc == nil {
		return
	}
	n, err := j.svc.ExpireStaleServiceCalls(context.Background(), time.Now())
	if err != nil {
		log.Printf("[service-call-ttl] sweep failed: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[service-call-ttl] expired %d stale service calls", n)
	}
}
