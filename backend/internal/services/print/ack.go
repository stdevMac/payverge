package print

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

// recordKitchenAck stamps orders.kitchen_acked_at the FIRST time a kitchen or
// bar ticket for the order prints successfully (X-4) and observes the
// payverge_order_to_kitchen_seconds histogram. First print wins — the
// conditional UPDATE makes reprints and duplicate jobs no-ops, which also
// keeps the idx_orders_kitchen_unacked partial-index predicate meaningful.
// Best-effort: failures log and never affect the print transition.
func recordKitchenAck(ctx context.Context, db *gorm.DB, job database.PrintJob) {
	if job.Kind != database.PrintJobKindKitchen && job.Kind != database.PrintJobKindBar {
		return
	}
	orderID := job.SourceID
	if job.OrderID != nil && *job.OrderID != 0 {
		orderID = *job.OrderID
	}
	if orderID == 0 {
		return
	}

	var order database.Order
	if err := db.WithContext(ctx).Select("id", "created_at", "kitchen_acked_at").
		First(&order, orderID).Error; err != nil {
		log.Printf("[print-ack] order lookup failed order_id=%d: %v", orderID, err)
		return
	}
	if order.KitchenAckedAt != nil {
		return
	}

	now := time.Now()
	res := db.WithContext(ctx).Model(&database.Order{}).
		Where("id = ? AND kitchen_acked_at IS NULL", orderID).
		Update("kitchen_acked_at", &now)
	if res.Error != nil {
		log.Printf("[print-ack] ack write failed order_id=%d: %v", orderID, res.Error)
		return
	}
	if res.RowsAffected > 0 {
		metrics.OrderToKitchenSeconds.Observe(now.Sub(order.CreatedAt).Seconds())
	}
}
