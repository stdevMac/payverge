package fiscal

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// T9: ResendReceipt requeues delivery tasks (does not inline-send) and writes
// an audit event. The delivery worker then executes the new attempt.
func TestResendReceipt_ForcesDeliveryEvenWhenDelivered(t *testing.T) {
	disp := &stubDispatcher{}
	db := newFiscalTestDBWithCustomer(t)
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)

	seedBillWithCustomer(t, db, 1, 1, 100, "guest@example.com", 12100)
	settingsID := seedReadySettings(t, db, 1)
	delivered := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	rid := seedSweepReceipt(t, db, 1, 1, settingsID, database.FiscalStatusAuthorized, &delivered, &delivered)

	require.NoError(t, svc.ResendReceipt(context.Background(), 1, rid, "manager"))
	// Operator resend only requeues tasks — no inline email.
	require.Equal(t, 0, disp.emailCalls, "resend must not inline-deliver")

	var pending int64
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ? AND status = ?", rid, database.FiscalDeliveryStatusPending).
		Count(&pending).Error)
	require.GreaterOrEqual(t, pending, int64(1), "resend must requeue at least one delivery channel")

	// Worker executes the requeued attempt.
	dw := NewDeliveryWorker(db, svc, disp)
	dw.WorkerID = "resend-worker"
	for i := 0; i < 5; i++ {
		_ = db.Model(&database.FiscalDeliveryTask{}).
			Where("status = ?", database.FiscalDeliveryStatusPending).
			Update("next_attempt_at", time.Now().UTC().Add(-time.Minute)).Error
		n, err := dw.ProcessDue(context.Background())
		require.NoError(t, err)
		if n == 0 {
			break
		}
	}
	require.Equal(t, 1, disp.emailCalls, "worker must deliver after resend requeue")

	var events []database.FiscalAuditEvent
	require.NoError(t, db.Where("event_type = ?", "fiscal_receipt_resent").Find(&events).Error)
	require.Len(t, events, 1)
	require.NotNil(t, events[0].ReceiptID)
	require.Equal(t, rid, *events[0].ReceiptID)
	require.Equal(t, "manager", events[0].Actor)
}

// A non-authorized receipt has no deliverable PDF → ErrReceiptNotDeliverable.
func TestResendReceipt_NonAuthorizedReturnsNotDeliverable(t *testing.T) {
	disp := &stubDispatcher{}
	db := newFiscalTestDBWithCustomer(t)
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)

	seedBillWithCustomer(t, db, 1, 1, 100, "guest@example.com", 12100)
	settingsID := seedReadySettings(t, db, 1)
	old := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	rid := seedSweepReceipt(t, db, 1, 1, settingsID, database.FiscalStatusFailedRetryable, &old, nil)

	err := svc.ResendReceipt(context.Background(), 1, rid, "manager")
	require.ErrorIs(t, err, ErrReceiptNotDeliverable)
	require.Equal(t, 0, disp.emailCalls)
}

func TestResendReceipt_MissingReceiptReturnsError(t *testing.T) {
	db := newFiscalTestDBWithCustomer(t)
	svc := NewService(db, NewProviderRegistry()).WithDelivery(&stubDispatcher{})
	err := svc.ResendReceipt(context.Background(), 1, 999, "manager")
	require.Error(t, err)
}
