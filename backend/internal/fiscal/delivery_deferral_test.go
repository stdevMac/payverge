package fiscal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// Review F3: a fiscal receipt email refused by the tenant outbound email
// budget (a rolling daily quota) must wait for the budget, not burn the
// 8-attempt backoff schedule and dead-letter within about half an hour.
func TestDeliveryWorker_BudgetRefusalDefersWithoutBurningAttempts(t *testing.T) {
	db := newFiscalTestDBWithCustomer(t)
	budgetErr := errors.New("tenant email budget exceeded: business_daily_standard")
	disp := &stubDispatcher{emailErr: DeferDelivery(budgetErr, time.Hour)}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	w := NewDeliveryWorker(db, svc, disp)
	w.WorkerID = "dw-defer"
	clock := time.Now().UTC()
	w.Now = func() time.Time { return clock }

	seedBillWithCustomer(t, db, 40, 40, 400, "guest@example.com", 12100)
	settingsID := seedReadySettings(t, db, 40)
	num, cae, qr := "45", "71000000000003", "https://www.afip.gob.ar/fe/qr/?p=def"
	receipt := database.FiscalReceipt{
		BusinessID: 40, SettingsID: settingsID, BillID: 40,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_c", ReceiptNumber: &num, AuthCode: &cae, QRPayload: &qr,
		Status: database.FiscalStatusAuthorized, TotalAmountCents: 12100, Currency: "ARS",
		IssuedAt: &clock,
	}
	require.NoError(t, db.Create(&receipt).Error)
	require.NoError(t, NewRepository(db).EnqueueDeliveryTasks(db, receipt.ID, 40, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelEmail},
	}, "dw-defer", clock))

	var task database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", receipt.ID).First(&task).Error)
	maxAttempts := task.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = database.DefaultFiscalDeliveryMaxAttempts
	}

	// More refusals than the retry schedule allows attempts.
	for i := 0; i < maxAttempts+2; i++ {
		n, err := w.ProcessDue(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, n, "round %d must claim the deferred task", i)

		require.NoError(t, db.First(&task, task.ID).Error)
		require.Equal(t, database.FiscalDeliveryStatusPending, task.Status, "a budget refusal never dead-letters inside the horizon")
		require.Zero(t, task.Attempts, "a budget refusal does not consume an attempt")
		require.Contains(t, task.LastError, "deferred")
		require.NotNil(t, task.NextAttemptAt)
		wait := task.NextAttemptAt.Sub(clock)
		require.GreaterOrEqual(t, wait, 54*time.Minute)
		require.LessOrEqual(t, wait, 66*time.Minute)
		require.Empty(t, task.LeaseOwner)

		clock = task.NextAttemptAt.Add(time.Second)
	}

	// Budget available again: the receipt goes out.
	disp.emailErr = nil
	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.NoError(t, db.First(&task, task.ID).Error)
	require.Equal(t, database.FiscalDeliveryStatusSucceeded, task.Status)
	require.Equal(t, maxAttempts+3, disp.emailCalls)
}

func TestDeliveryWorker_DeferralHorizonDeadLetters(t *testing.T) {
	db := newFiscalTestDBWithCustomer(t)
	disp := &stubDispatcher{emailErr: DeferDelivery(errors.New("quota"), time.Hour)}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	w := NewDeliveryWorker(db, svc, disp)
	w.WorkerID = "dw-horizon"
	clock := time.Now().UTC()
	w.Now = func() time.Time { return clock }

	seedBillWithCustomer(t, db, 41, 41, 410, "guest@example.com", 12100)
	settingsID := seedReadySettings(t, db, 41)
	num, cae, qr := "46", "71000000000004", "https://www.afip.gob.ar/fe/qr/?p=ghi"
	receipt := database.FiscalReceipt{
		BusinessID: 41, SettingsID: settingsID, BillID: 41,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_c", ReceiptNumber: &num, AuthCode: &cae, QRPayload: &qr,
		Status: database.FiscalStatusAuthorized, TotalAmountCents: 12100, Currency: "ARS",
		IssuedAt: &clock,
	}
	require.NoError(t, db.Create(&receipt).Error)
	require.NoError(t, NewRepository(db).EnqueueDeliveryTasks(db, receipt.ID, 41, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelEmail},
	}, "dw-horizon", clock))

	_, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	var task database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", receipt.ID).First(&task).Error)
	require.Equal(t, database.FiscalDeliveryStatusPending, task.Status)

	clock = task.CreatedAt.Add(MaxDeliveryDeferral + time.Minute)
	_, err = w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.NoError(t, db.First(&task, task.ID).Error)
	require.Equal(t, database.FiscalDeliveryStatusDead, task.Status, "a task deferred past the horizon surfaces to the operator")
	require.Contains(t, task.LastError, "deferred past")
	require.Zero(t, task.Attempts)
}

func TestDeferDelivery(t *testing.T) {
	require.NoError(t, DeferDelivery(nil, time.Hour))

	base := errors.New("budget")
	err := DeferDelivery(base, 0)
	var deferred *DeliveryDeferredError
	require.True(t, errors.As(err, &deferred))
	require.Equal(t, DefaultDeliveryDeferral, deferred.After)
	require.True(t, errors.Is(err, base), "the cause stays matchable")
	require.False(t, isPermanentDeliveryError(err))
}
