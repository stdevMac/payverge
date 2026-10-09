package fiscal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// TestDeliveryWorker_EmailNotResentAfterCrashWindow proves the P5 Gap-5 fix:
// once SendReceiptEmail succeeds, the task records a dispatch marker
// (provider_message_id) so a crash / lease-expiry re-claim is a no-op send.
// Without the marker, the email channel is at-least-once and a re-queued task
// re-sends the customer's fiscal receipt.
func TestDeliveryWorker_EmailNotResentAfterCrashWindow(t *testing.T) {
	db := newFiscalTestDBWithCustomer(t)
	disp := &stubDispatcher{}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	w := NewDeliveryWorker(db, svc, disp)
	w.WorkerID = "dw-crash"
	w.Now = func() time.Time { return time.Now().UTC() }

	seedBillWithCustomer(t, db, 30, 30, 300, "guest@example.com", 12100)
	settingsID := seedReadySettings(t, db, 30)
	now := time.Now().UTC()
	num, cae, qr := "44", "71000000000002", "https://www.afip.gob.ar/fe/qr/?p=xyz"
	receipt := database.FiscalReceipt{
		BusinessID: 30, SettingsID: settingsID, BillID: 30,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_c", ReceiptNumber: &num, AuthCode: &cae, QRPayload: &qr,
		Status: database.FiscalStatusAuthorized, TotalAmountCents: 12100, Currency: "ARS",
		IssuedAt: &now,
	}
	require.NoError(t, db.Create(&receipt).Error)
	require.NoError(t, NewRepository(db).EnqueueDeliveryTasks(db, receipt.ID, 30, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelEmail},
	}, "dw-crash", now))

	// First round: email is sent and the task succeeds.
	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 1, disp.emailCalls, "email sent exactly once on first delivery")

	var task database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", receipt.ID).First(&task).Error)
	require.Equal(t, database.FiscalDeliveryStatusSucceeded, task.Status)
	// A dispatch marker must be persisted so a re-claim is a no-op send.
	require.NotNil(t, task.ProviderMessageID, "dispatch marker must be recorded after a successful send")
	require.Equal(t, task.IdempotencyKey, strings.TrimSpace(*task.ProviderMessageID),
		"dispatch marker must record the task idempotency key")

	// Simulate the crash window: the send succeeded and the marker persisted, but
	// the task was NOT finalized (process died / lease expired) and is re-queued as
	// due. The marker must prevent a duplicate receipt email.
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
		Where("id = ?", task.ID).
		Updates(map[string]interface{}{
			"status":           database.FiscalDeliveryStatusPending,
			"succeeded_at":     nil,
			"lease_owner":      "",
			"lease_expires_at": nil,
			"next_attempt_at":  now.Add(-time.Minute),
		}).Error)

	n, err = w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 1, disp.emailCalls, "re-claim after the crash window must NOT resend the receipt email")

	require.NoError(t, db.First(&task, task.ID).Error)
	require.Equal(t, database.FiscalDeliveryStatusSucceeded, task.Status)
}

func TestDeliveryWorker_TransientRetryThenSuccess(t *testing.T) {
	db := newFiscalTestDBWithCustomer(t)
	disp := &stubDispatcher{emailErr: errors.New("smtp 421 try later")}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	w := NewDeliveryWorker(db, svc, disp)
	w.WorkerID = "dw-1"
	w.Now = func() time.Time { return time.Now().UTC() }

	// Full bill+customer so email channel actually invokes SendReceiptEmail.
	seedBillWithCustomer(t, db, 20, 20, 200, "guest@example.com", 12100)
	settingsID := seedReadySettings(t, db, 20)
	now := time.Now().UTC()
	num, cae, qr := "43", "71000000000001", "https://www.afip.gob.ar/fe/qr/?p=abc"
	receipt := database.FiscalReceipt{
		BusinessID: 20, SettingsID: settingsID, BillID: 20,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_c", ReceiptNumber: &num, AuthCode: &cae, QRPayload: &qr,
		Status: database.FiscalStatusAuthorized, TotalAmountCents: 12100, Currency: "ARS",
		IssuedAt: &now,
	}
	require.NoError(t, db.Create(&receipt).Error)
	require.NoError(t, NewRepository(db).EnqueueDeliveryTasks(db, receipt.ID, 20, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelEmail},
	}, "dw-retry", now))

	// First attempt fails → pending retry
	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	var task database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", receipt.ID).First(&task).Error)
	require.Equal(t, database.FiscalDeliveryStatusPending, task.Status)
	require.Equal(t, 1, task.Attempts)

	// Clear error and force next_attempt_at to past
	disp.emailErr = nil
	past := now.Add(-time.Minute)
	require.NoError(t, db.Model(&task).Update("next_attempt_at", past).Error)
	n, err = w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.NoError(t, db.First(&task, task.ID).Error)
	require.Equal(t, database.FiscalDeliveryStatusSucceeded, task.Status)
}

func TestDeliveryWorker_PermanentValidationDead(t *testing.T) {
	db := newFiscalTestDB(t)
	// No dispatcher → permanent-ish failure... actually "dispatcher not configured"
	// is retryable. Use permanent via missing authorized status after enqueue.
	disp := &stubDispatcher{}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	w := NewDeliveryWorker(db, svc, disp)
	w.WorkerID = "dw-perm"

	// Create a pending (not authorized) receipt
	settings := database.BusinessFiscalSettings{
		BusinessID: 21, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox",
		SetupStatus: "valid", TaxID: "20123456789", TaxCondition: "monotributo",
	}
	require.NoError(t, db.Create(&settings).Error)
	receipt := database.FiscalReceipt{
		BusinessID: 21, SettingsID: settings.ID, BillID: 1,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "C", Status: database.FiscalStatusPending,
		TotalAmountCents: 100, Currency: "ARS",
	}
	require.NoError(t, db.Create(&receipt).Error)
	now := time.Now().UTC()
	require.NoError(t, NewRepository(db).EnqueueDeliveryTasks(db, receipt.ID, 21, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelArtifact},
	}, "dw-perm", now))

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	var task database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", receipt.ID).First(&task).Error)
	require.Equal(t, database.FiscalDeliveryStatusDead, task.Status)
	require.Equal(t, 0, task.Attempts, "permanent dead should not burn attempts the same way — MarkDeliveryDead does not increment")
}

func TestDeliveryWorker_MaxAttemptsDeadLetter(t *testing.T) {
	db := newFiscalTestDB(t)
	disp := &stubDispatcher{uploadErr: errors.New("s3 503")}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	w := NewDeliveryWorker(db, svc, disp)
	w.WorkerID = "dw-max"

	receipt := seedAuthorizedReceipt(t, db, 22)
	now := time.Now().UTC()
	require.NoError(t, NewRepository(db).EnqueueDeliveryTasks(db, receipt.ID, 22, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelArtifact},
	}, "dw-max", now))
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ?", receipt.ID).
		Update("max_attempts", 2).Error)

	for i := 0; i < 2; i++ {
		// Ensure due
		require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
			Where("receipt_id = ?", receipt.ID).
			Update("next_attempt_at", now.Add(-time.Minute)).Error)
		_, err := w.ProcessDue(context.Background())
		require.NoError(t, err)
	}
	var task database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", receipt.ID).First(&task).Error)
	require.Equal(t, database.FiscalDeliveryStatusDead, task.Status)
	require.Equal(t, 2, task.Attempts)
}

func TestDeliveryWorker_LeaseExpiryReclaim(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 23)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 23, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelPrint},
	}, "dw-lease", now))

	// Simulate a crashed worker: lease expired
	expired := now.Add(-time.Minute)
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ?", receipt.ID).
		Updates(map[string]interface{}{
			"status":           database.FiscalDeliveryStatusLeased,
			"lease_owner":      "crashed-worker",
			"lease_expires_at": expired,
		}).Error)

	disp := &stubDispatcher{}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	w := NewDeliveryWorker(db, svc, disp)
	w.WorkerID = "dw-reclaim"
	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	var task database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", receipt.ID).First(&task).Error)
	require.Equal(t, database.FiscalDeliveryStatusSucceeded, task.Status)
}

func TestDeliveryWorker_ShutdownCancellation(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	disp := &stubDispatcher{}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	w := NewDeliveryWorker(db, svc, disp)
	w.WorkerID = "dw-stop"

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		StartDeliveryWorker(ctx, w, 20*time.Millisecond)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartDeliveryWorker did not return after cancel")
	}
}

func TestDeliveryWorker_DuplicateWorkerOneLease(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 24)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 24, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelPrint},
	}, "dw-dup", now))

	disp := &stubDispatcher{}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	var successes int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			w := NewDeliveryWorker(db, svc, disp)
			w.WorkerID = "dup-" + string(rune('a'+id))
			n, _ := w.ProcessDue(context.Background())
			atomic.AddInt32(&successes, int32(n))
		}(i)
	}
	wg.Wait()
	require.Equal(t, int32(1), successes)

	var task database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", receipt.ID).First(&task).Error)
	require.Equal(t, database.FiscalDeliveryStatusSucceeded, task.Status)
}

func TestDeliveryWorker_AlreadySucceededIdempotent(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 25)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 25, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelPrint},
	}, "dw-ok", now))
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ?", receipt.ID).
		Updates(map[string]interface{}{
			"status":       database.FiscalDeliveryStatusSucceeded,
			"succeeded_at": now,
		}).Error)

	disp := &stubDispatcher{}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	w := NewDeliveryWorker(db, svc, disp)
	w.WorkerID = "dw-idem"
	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, n, "succeeded tasks must not be reclaimed")
	require.Equal(t, 0, disp.printCalls)
}

// Delivery for demo-provider receipts is simulated: the worker must mark the
// task succeeded without touching real transports (S3 upload, Resend email,
// print queue). Real transports failing for fake receipts is what dead-lettered
// every seeded demo invoice into needs_attention.
func TestDeliveryWorker_DemoProviderShortCircuits(t *testing.T) {
	db := newFiscalTestDBWithCustomer(t)
	disp := &stubDispatcher{
		uploadErr: errors.New("must not upload for demo receipts"),
		emailErr:  errors.New("must not email for demo receipts"),
	}
	svc := NewService(db, NewProviderRegistry()).WithDelivery(disp)
	w := NewDeliveryWorker(db, svc, disp)
	w.WorkerID = "dw-demo"
	w.Now = func() time.Time { return time.Now().UTC() }

	seedBillWithCustomer(t, db, 40, 40, 400, "guest@example.com", 9543)
	settingsID := seedReadySettings(t, db, 40)
	now := time.Now().UTC()
	num, auth, qr := "DEMO-1", "AUTH-1", "https://payverge.local/fiscal/1"
	receipt := database.FiscalReceipt{
		BusinessID: 40, SettingsID: settingsID, BillID: 40,
		Country: "US", Provider: "demo", Action: "invoice",
		ReceiptType: "receipt", ReceiptNumber: &num, AuthCode: &auth, QRPayload: &qr,
		Status: database.FiscalStatusAuthorized, TotalAmountCents: 9543, Currency: "USD",
		IssuedAt: &now,
	}
	require.NoError(t, db.Create(&receipt).Error)
	require.NoError(t, NewRepository(db).EnqueueDeliveryTasks(db, receipt.ID, 40, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelArtifact},
		{Channel: database.FiscalDeliveryChannelEmail},
	}, "dw-demo", now))

	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, 0, disp.emailCalls, "demo receipts must not send real email")
	require.Equal(t, 0, disp.uploads, "demo receipts must not upload real artifacts")

	var tasks []database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", receipt.ID).Find(&tasks).Error)
	require.Len(t, tasks, 2)
	for _, task := range tasks {
		require.Equal(t, database.FiscalDeliveryStatusSucceeded, task.Status,
			"demo delivery tasks must succeed as simulated (channel %s)", task.Channel)
	}
}
