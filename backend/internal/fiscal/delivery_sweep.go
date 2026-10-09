package fiscal

import (
	"context"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// SweepUndeliveredReceipts drains the LEGACY undelivered backlog (authorized
// receipts with delivered_at IS NULL and NO fiscal_delivery_tasks rows). Wave 4
// receipts enqueue durable per-channel tasks at authorization and are owned by
// DeliveryWorker; this sweep skips any receipt that already has task rows.
//
// TODO(wave4): retire this sweep when CountLegacyUndeliveredAuthorizedReceipts
// is zero in production (metric + operator confirmation). Until then keep the
// 000095 delivery locks and this drain path.
//
// Bounded to `limit` receipts per call. Best-effort per receipt. A nil
// dispatcher is a no-op.
func (s *Service) SweepUndeliveredReceipts(ctx context.Context, olderThan time.Time, limit int) (int, error) {
	if s.dispatcher == nil {
		return 0, nil
	}
	receipts, err := s.repo.ListUndeliveredAuthorizedReceipts(olderThan, limit)
	if err != nil {
		return 0, err
	}
	delivered := 0
	staleBefore := time.Now().Add(-5 * time.Minute)
	for i := range receipts {
		receipt := receipts[i]
		// Wave 4: receipts that already have durable delivery tasks are owned by
		// the delivery worker. Only drain the legacy backlog (no task rows).
		var taskCount int64
		if err := s.db.Model(&database.FiscalDeliveryTask{}).
			Where("receipt_id = ?", receipt.ID).Count(&taskCount).Error; err == nil && taskCount > 0 {
			continue
		}
		won, err := s.repo.ClaimReceiptDelivery(receipt.ID, "fiscal-sweep", staleBefore)
		if err != nil {
			logger.Logger.Warnf("Fiscal sweep: claim receipt %d failed: %v", receipt.ID, err)
			continue
		}
		if !won {
			continue
		}
		// A receipt carries the bill/settings/business ids the delivery context
		// needs; reuse the job-context loader (narrow projections) with a synthetic
		// job built from the receipt.
		jobCtx, err := s.repo.LoadJobContext(database.FiscalJob{
			BillID:     receipt.BillID,
			SettingsID: receipt.SettingsID,
			BusinessID: receipt.BusinessID,
		})
		if err != nil {
			logger.Logger.Warnf("Fiscal sweep: load context for undelivered receipt %d failed: %v", receipt.ID, err)
			continue
		}
		s.DeliverReceipt(ctx, &receipt, jobCtx)
		// DeliverReceipt stamps DeliveredAt on the passed receipt on success.
		if receipt.DeliveredAt != nil {
			delivered++
		}
	}
	return delivered, nil
}
