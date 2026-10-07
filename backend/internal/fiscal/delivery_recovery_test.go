package fiscal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// Task 5: simulate each dispatcher failure then recovery and prove eventual
// success WITHOUT re-authorizing or duplicating fiscal documents.
func TestDeliveryRecovery_AllChannelsEventualSuccess(t *testing.T) {
	db := newFiscalTestDBWithCustomer(t)
	disp := &stubDispatcher{
		uploadErr: errors.New("s3 503"),
		emailErr:  errors.New("smtp 421"),
		printErr:  errors.New("printer offline"),
	}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	w := NewDeliveryWorker(db, svc, disp)
	w.WorkerID = "recovery-worker"

	seedBillWithCustomer(t, db, 30, 30, 300, "guest@example.com", 12100)
	settingsID := seedReadySettings(t, db, 30)
	now := time.Now().UTC()
	num, cae, qr := "99", "71000000000099", "https://www.afip.gob.ar/fe/qr/?p=rec"
	receipt := database.FiscalReceipt{
		BusinessID: 30, SettingsID: settingsID, BillID: 30,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_c", ReceiptNumber: &num, AuthCode: &cae, QRPayload: &qr,
		Status: database.FiscalStatusAuthorized, TotalAmountCents: 12100, Currency: "ARS",
		IssuedAt: &now,
	}
	require.NoError(t, db.Create(&receipt).Error)
	// Single authorized receipt — never re-create.
	var receiptCount int64
	require.NoError(t, db.Model(&database.FiscalReceipt{}).
		Where("bill_id = ? AND status = ?", 30, database.FiscalStatusAuthorized).
		Count(&receiptCount).Error)
	require.Equal(t, int64(1), receiptCount)

	require.NoError(t, NewRepository(db).EnqueueDeliveryTasks(db, receipt.ID, 30, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelArtifact},
		{Channel: database.FiscalDeliveryChannelEmail},
		{Channel: database.FiscalDeliveryChannelPrint},
	}, "recovery", now))

	// Round 1: all channels fail → pending retry (not dead, max_attempts=8).
	forceDue := func() {
		_ = db.Model(&database.FiscalDeliveryTask{}).
			Where("receipt_id = ?", receipt.ID).
			Update("next_attempt_at", time.Now().UTC().Add(-time.Minute)).Error
	}
	forceDue()
	_, err := w.ProcessDue(context.Background())
	require.NoError(t, err)

	// Clear failures — recovery.
	disp.uploadErr = nil
	disp.emailErr = nil
	disp.printErr = nil

	for i := 0; i < 6; i++ {
		forceDue()
		_, err := w.ProcessDue(context.Background())
		require.NoError(t, err)
	}

	var tasks []database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", receipt.ID).Find(&tasks).Error)
	require.Len(t, tasks, 3)
	for _, task := range tasks {
		require.Equal(t, database.FiscalDeliveryStatusSucceeded, task.Status,
			"channel %s must eventually succeed", task.Channel)
	}

	// Still exactly one authorized receipt (no re-authorization / duplicate).
	require.NoError(t, db.Model(&database.FiscalReceipt{}).
		Where("bill_id = ? AND status = ?", 30, database.FiscalStatusAuthorized).
		Count(&receiptCount).Error)
	require.Equal(t, int64(1), receiptCount)

	var reloaded database.FiscalReceipt
	require.NoError(t, db.First(&reloaded, receipt.ID).Error)
	require.NotNil(t, reloaded.DeliveredAt)
}

// Legacy coarse sweep remains gated: when delivery tasks exist, sweep must not
// double-deliver. Retirement happens only when CountLegacyUndelivered == 0 and
// operators confirm (documented in fiscal runbook).
func TestSweepSkipsReceiptsWithDeliveryTasks(t *testing.T) {
	db := newFiscalTestDBWithCustomer(t)
	disp := &stubDispatcher{}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)

	seedBillWithCustomer(t, db, 31, 31, 310, "guest@example.com", 12100)
	settingsID := seedReadySettings(t, db, 31)
	old := time.Now().UTC().Add(-time.Hour)
	rid := seedSweepReceipt(t, db, 31, 31, settingsID, database.FiscalStatusAuthorized, &old, nil)
	require.NoError(t, NewRepository(db).EnqueueDeliveryTasks(db, rid, 31, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelEmail},
	}, "sweep-skip", old))

	n, err := svc.SweepUndeliveredReceipts(context.Background(), time.Now().UTC(), 25)
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.Equal(t, 0, disp.emailCalls, "sweep must not inline-deliver when tasks exist")
}
