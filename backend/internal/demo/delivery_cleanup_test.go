package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// Seeded demo receipts historically accumulated real delivery tasks via the
// fiscal backfill sweep; artifact/email dead-lettered against real transports
// and flagged every demo invoice as needs_attention. The hourly demo
// maintenance pass must purge delivery tasks attached to provider=demo
// receipts (and only those) so existing environments self-heal.
func TestCleanupSimulatedDeliveryTasks(t *testing.T) {
	db := newDemoServiceTestDB(t)
	svc := NewService(db, Options{Now: fixedNow})

	demoReceipt := database.FiscalReceipt{
		BusinessID: 1, SettingsID: 1, BillID: 1,
		Country: "US", Provider: "demo", Action: "issue_receipt", ReceiptType: "receipt",
		Status: database.FiscalStatusAuthorized, TotalAmountCents: 1000, Currency: "USD",
	}
	require.NoError(t, db.Create(&demoReceipt).Error)
	realReceipt := database.FiscalReceipt{
		BusinessID: 2, SettingsID: 2, BillID: 2,
		Country: "AR", Provider: "arca", Action: "issue_receipt", ReceiptType: "factura_c",
		Status: database.FiscalStatusAuthorized, TotalAmountCents: 2000, Currency: "ARS",
	}
	require.NoError(t, db.Create(&realReceipt).Error)

	now := time.Now().UTC()
	mkTask := func(receiptID uint, channel, status, key string) {
		require.NoError(t, db.Create(&database.FiscalDeliveryTask{
			ReceiptID: receiptID, BusinessID: 1, Channel: channel,
			Status: status, IdempotencyKey: key, MaxAttempts: 5,
			NextAttemptAt: &now,
		}).Error)
	}
	mkTask(demoReceipt.ID, database.FiscalDeliveryChannelArtifact, database.FiscalDeliveryStatusDead, "d1")
	mkTask(demoReceipt.ID, database.FiscalDeliveryChannelEmail, database.FiscalDeliveryStatusDead, "d2")
	mkTask(demoReceipt.ID, database.FiscalDeliveryChannelPrint, database.FiscalDeliveryStatusSucceeded, "d3")
	mkTask(realReceipt.ID, database.FiscalDeliveryChannelEmail, database.FiscalDeliveryStatusDead, "r1")

	removed, err := svc.cleanupSimulatedDeliveryTasks(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 3, removed)

	var demoCount, realCount int64
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ?", demoReceipt.ID).Count(&demoCount).Error)
	require.Zero(t, demoCount, "all demo-receipt delivery tasks must be purged")
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ?", realReceipt.ID).Count(&realCount).Error)
	require.EqualValues(t, 1, realCount, "real-provider tasks must be untouched")

	// Idempotent: second pass removes nothing.
	removed, err = svc.cleanupSimulatedDeliveryTasks(context.Background())
	require.NoError(t, err)
	require.Zero(t, removed)
}
