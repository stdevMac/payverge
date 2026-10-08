package fiscal

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newDeliveryTaskTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Unique DSN per test so bill_number/settings uniqueness never collides
	// across parallel package tests, while MaxOpenConns=1 keeps one schema.
	dsn := fmt.Sprintf("file:delivery_task_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.BusinessFiscalSettings{},
		&database.FiscalReceipt{},
		&database.FiscalJob{},
		&database.FiscalAuditEvent{},
		&database.FiscalDeliveryTask{},
		&runtimecontrol.Control{},
	))
	require.NoError(t, db.Create(&runtimecontrol.Control{
		Key: runtimecontrol.ControlFiscal, Enabled: true, Owner: "test",
		Reason: "delivery tests enabled", ExpiresAt: time.Now().Add(24 * time.Hour),
		UpdatedBy: "test", UpdatedAt: time.Now(),
	}).Error)
	return db
}

func seedAuthorizedReceipt(t *testing.T, db *gorm.DB, businessID uint) database.FiscalReceipt {
	t.Helper()
	// Business row required so LoadJobContext succeeds for delivery worker tests.
	_ = db.Where("id = ?", businessID).FirstOrCreate(&database.Business{
		ID: businessID, BusinessId: fmt.Sprintf("biz-%d", businessID),
		OwnerAddress: "owner", Name: "Test Biz",
		SettlementAddr: "settle", TippingAddr: "tip",
	}).Error
	settings := database.BusinessFiscalSettings{
		BusinessID: businessID, Country: "AR", Provider: "arca",
		Mode: database.FiscalModeAutomaticNonBlocking, Environment: "sandbox",
		SetupStatus: "valid", TaxID: "20123456789", TaxCondition: "monotributo",
	}
	require.NoError(t, db.Create(&settings).Error)
	// Unique bill_number avoids UNIQUE constraint on empty default across seeds.
	billNum := fmt.Sprintf("B-%d-%d", businessID, time.Now().UnixNano())
	bill := database.Bill{BusinessID: businessID, Status: "paid", BillNumber: billNum}
	require.NoError(t, db.Create(&bill).Error)
	now := time.Now().UTC()
	num, cae, qr := "43", "71000000000001", "https://www.afip.gob.ar/fe/qr/?p=abc"
	receipt := database.FiscalReceipt{
		BusinessID: businessID, SettingsID: settings.ID, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
		ReceiptType: "factura_c", ReceiptNumber: &num, AuthCode: &cae, QRPayload: &qr,
		Status:           database.FiscalStatusAuthorized,
		TotalAmountCents: 121000, Currency: "ARS",
		IssuedAt: &now,
	}
	require.NoError(t, db.Create(&receipt).Error)
	return receipt
}

func TestEnqueueDeliveryTasks_IdempotentOnReceiptChannel(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 1)
	now := time.Now().UTC()
	channels := []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelArtifact},
		{Channel: database.FiscalDeliveryChannelEmail, Locale: "es-AR"},
		{Channel: database.FiscalDeliveryChannelPrint},
	}
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 1, channels, "receipt:1", now))
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 1, channels, "receipt:1", now))

	var count int64
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ?", receipt.ID).Count(&count).Error)
	require.Equal(t, int64(3), count)

	var email database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ? AND channel = ?", receipt.ID, database.FiscalDeliveryChannelEmail).
		First(&email).Error)
	require.Equal(t, database.FiscalDeliveryStatusPending, email.Status)
	require.Equal(t, database.DefaultFiscalDeliveryMaxAttempts, email.MaxAttempts)
	require.NotNil(t, email.NextAttemptAt)
	require.Equal(t, "receipt:1:email", email.IdempotencyKey)
	require.NotNil(t, email.Locale)
	require.Equal(t, "es-AR", *email.Locale)
}

func TestEnqueueDeliveryTasks_UniqueReceiptChannel(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	receipt := seedAuthorizedReceipt(t, db, 2)
	now := time.Now().UTC()
	task := database.FiscalDeliveryTask{
		BusinessID: 2, ReceiptID: receipt.ID, Channel: database.FiscalDeliveryChannelArtifact,
		Status: database.FiscalDeliveryStatusPending, MaxAttempts: 8,
		IdempotencyKey: "a", NextAttemptAt: &now,
	}
	require.NoError(t, db.Create(&task).Error)
	dup := task
	dup.ID = 0
	dup.IdempotencyKey = "b"
	err := db.Create(&dup).Error
	require.Error(t, err, "duplicate (receipt_id, channel) must fail")
}

func TestClaimDueDeliveryTasks_LeaseOwnerAndExpiry(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 3)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 3, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelArtifact},
	}, "r3", now))

	claimed, err := repo.ClaimDueDeliveryTasks("worker-a", now, 2*time.Minute, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, database.FiscalDeliveryStatusLeased, claimed[0].Status)
	require.Equal(t, "worker-a", claimed[0].LeaseOwner)
	require.NotNil(t, claimed[0].LeaseExpiresAt)
	require.True(t, claimed[0].LeaseExpiresAt.After(now))

	// Second worker gets nothing while lease is live.
	claimed2, err := repo.ClaimDueDeliveryTasks("worker-b", now.Add(10*time.Second), 2*time.Minute, 10)
	require.NoError(t, err)
	require.Empty(t, claimed2)

	// After lease expiry, reclaim succeeds.
	expired := now.Add(5 * time.Minute)
	claimed3, err := repo.ClaimDueDeliveryTasks("worker-b", expired, 2*time.Minute, 10)
	require.NoError(t, err)
	require.Len(t, claimed3, 1)
	require.Equal(t, "worker-b", claimed3[0].LeaseOwner)
}

func TestMarkDeliverySucceeded_ClearsLease(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 4)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 4, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelEmail},
	}, "r4", now))
	claimed, err := repo.ClaimDueDeliveryTasks("w1", now, time.Minute, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	require.NoError(t, repo.MarkDeliverySucceeded(claimed[0].ID, "w1", "msg-abc", now.Add(time.Second)))
	var task database.FiscalDeliveryTask
	require.NoError(t, db.First(&task, claimed[0].ID).Error)
	require.Equal(t, database.FiscalDeliveryStatusSucceeded, task.Status)
	require.NotNil(t, task.SucceededAt)
	require.Empty(t, task.LeaseOwner)
	require.NotNil(t, task.ProviderMessageID)
	require.Equal(t, "msg-abc", *task.ProviderMessageID)
}

func TestMarkDeliveryFailed_RetryThenDead(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 5)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 5, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelPrint},
	}, "r5", now))
	// Lower max_attempts for the test.
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ?", receipt.ID).
		Update("max_attempts", 2).Error)

	// Attempt 1 → pending retry
	claimed, err := repo.ClaimDueDeliveryTasks("w1", now, time.Minute, 1)
	require.NoError(t, err)
	next := now.Add(30 * time.Second)
	require.NoError(t, repo.MarkDeliveryFailed(claimed[0].ID, "w1", "transient boom", next, false, now))
	var task database.FiscalDeliveryTask
	require.NoError(t, db.First(&task, claimed[0].ID).Error)
	require.Equal(t, database.FiscalDeliveryStatusPending, task.Status)
	require.Equal(t, 1, task.Attempts)
	require.Equal(t, "transient boom", task.LastError)
	require.NotNil(t, task.NextAttemptAt)

	// Attempt 2 exhausts → dead
	claimed2, err := repo.ClaimDueDeliveryTasks("w1", next, time.Minute, 1)
	require.NoError(t, err)
	require.Len(t, claimed2, 1)
	require.NoError(t, repo.MarkDeliveryFailed(claimed2[0].ID, "w1", "still failing", next.Add(time.Minute), false, next))
	require.NoError(t, db.First(&task, claimed[0].ID).Error)
	require.Equal(t, database.FiscalDeliveryStatusDead, task.Status)
	require.Equal(t, 2, task.Attempts)
	require.NotNil(t, task.DeadAt)
}

func TestMarkDeliveryFailed_PermanentWithoutExhausting(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 6)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 6, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelArtifact},
	}, "r6", now))
	claimed, err := repo.ClaimDueDeliveryTasks("w1", now, time.Minute, 1)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDeliveryFailed(claimed[0].ID, "w1", "validation permanent", now, true, now))
	var task database.FiscalDeliveryTask
	require.NoError(t, db.First(&task, claimed[0].ID).Error)
	require.Equal(t, database.FiscalDeliveryStatusDead, task.Status)
	require.Equal(t, 1, task.Attempts)
}

func TestLastErrorBounded(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 7)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 7, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelEmail},
	}, "r7", now))
	claimed, err := repo.ClaimDueDeliveryTasks("w1", now, time.Minute, 1)
	require.NoError(t, err)
	long := strings.Repeat("x", database.FiscalDeliveryLastErrorMaxLen+500)
	require.NoError(t, repo.MarkDeliveryFailed(claimed[0].ID, "w1", long, now.Add(time.Minute), false, now))
	var task database.FiscalDeliveryTask
	require.NoError(t, db.First(&task, claimed[0].ID).Error)
	require.LessOrEqual(t, len([]rune(task.LastError)), database.FiscalDeliveryLastErrorMaxLen)
}

func TestRequeueDeliveryTask_DeadToPending(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 8)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 8, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelEmail},
	}, "r8", now))
	claimed, err := repo.ClaimDueDeliveryTasks("w1", now, time.Minute, 1)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDeliveryDead(claimed[0].ID, "w1", "gave up", now))

	requeued, err := repo.RequeueDeliveryTask(claimed[0].ID, 8, "owner", now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, database.FiscalDeliveryStatusPending, requeued.Status)
	require.Equal(t, 0, requeued.Attempts)
	require.Contains(t, requeued.IdempotencyKey, "|retry:")
	require.Empty(t, requeued.LastError)
}

func TestRequeueDeliveryTask_CrossTenantDenied(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 9)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 9, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelPrint},
	}, "r9", now))
	var task database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ?", receipt.ID).First(&task).Error)
	// Wrong business
	_, err := repo.RequeueDeliveryTask(task.ID, 999, "attacker", now)
	require.Error(t, err)
}

func TestRequeueDeliveryTask_SucceededIdempotent(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 10)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 10, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelArtifact},
	}, "r10", now))
	claimed, err := repo.ClaimDueDeliveryTasks("w1", now, time.Minute, 1)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDeliverySucceeded(claimed[0].ID, "w1", "", now))

	requeued, err := repo.RequeueDeliveryTask(claimed[0].ID, 10, "owner", now)
	require.NoError(t, err)
	require.Equal(t, database.FiscalDeliveryStatusSucceeded, requeued.Status)
}

func TestConcurrentClaim_ExactlyOneLease(t *testing.T) {
	// SQLite single-conn cannot prove SKIP LOCKED; still prove claim CAS under
	// sequential-ish contention with shared memory DB + multiple claims.
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	receipt := seedAuthorizedReceipt(t, db, 11)
	now := time.Now().UTC()
	require.NoError(t, repo.EnqueueDeliveryTasks(db, receipt.ID, 11, []DeliveryChannelSpec{
		{Channel: database.FiscalDeliveryChannelArtifact},
	}, "r11", now))

	var wg sync.WaitGroup
	var mu sync.Mutex
	var winners []string
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Each claim uses its own session through the pinned single conn;
			// only one should get the row.
			claimed, err := repo.ClaimDueDeliveryTasks(fmt.Sprintf("w-%d", id), now, time.Minute, 1)
			if err != nil {
				return
			}
			if len(claimed) == 1 {
				mu.Lock()
				winners = append(winners, claimed[0].LeaseOwner)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	require.Len(t, winners, 1, "exactly one worker must win the lease; got %v", winners)
}

func TestBackfillDeliveryTasks_Idempotent(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)
	_ = seedAuthorizedReceipt(t, db, 12) // delivered_at nil → needs backfill
	now := time.Now().UTC()
	n1, err := repo.BackfillDeliveryTasksForAuthorizedReceipts(now, 50)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n1, 3)
	n2, err := repo.BackfillDeliveryTasksForAuthorizedReceipts(now, 50)
	require.NoError(t, err)
	require.Equal(t, 0, n2)
}

// Demo-provider receipts are simulated end-to-end (seeded or sandbox-issued) —
// the backfill sweep must not enqueue real artifact/email/print work for them.
// Before this guard, every seeded demo receipt (pdf_path NULL) got three tasks,
// artifact/email died against real transports, and needs_attention flagged 100%
// of the demo business's invoices.
func TestBackfillDeliveryTasks_SkipsDemoProvider(t *testing.T) {
	db := newDeliveryTaskTestDB(t)
	repo := NewRepository(db)

	receipt := seedAuthorizedReceipt(t, db, 13)
	require.NoError(t, db.Model(&database.FiscalReceipt{}).
		Where("id = ?", receipt.ID).
		Update("provider", "demo").Error)

	n, err := repo.BackfillDeliveryTasksForAuthorizedReceipts(time.Now().UTC(), 50)
	require.NoError(t, err)
	require.Equal(t, 0, n, "backfill must skip provider=demo receipts")

	count, err := repo.countTasksForReceipt(receipt.ID)
	require.NoError(t, err)
	require.Zero(t, count)
}
