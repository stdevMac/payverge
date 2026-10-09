package fiscal

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"
)

func TestEnqueueIssueJobInTx_FiscalRuntimeControlDisabledSkips(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := database.Bill{BusinessID: 91, BillNumber: "B-outbox-disabled",
		TotalAmount: 1000, PaidAmount: 1000, Status: database.BillStatusPaid}
	require.NoError(t, db.Create(&bill).Error)
	outboxTestSettings(t, db, 91, database.FiscalModeAutomaticNonBlocking)
	require.NoError(t, db.Model(&runtimecontrol.Control{}).
		Where("key = ?", runtimecontrol.ControlFiscal).
		Update("enabled", false).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return EnqueueIssueJobInTx(tx, &bill, nil, nil, "system")
	}))
	require.Zero(t, countFiscalJobs(t, db))
}

func outboxTestSettings(t *testing.T, db *gorm.DB, businessID uint, mode database.FiscalMode) database.BusinessFiscalSettings {
	t.Helper()
	settings := database.BusinessFiscalSettings{
		BusinessID:  businessID,
		Country:     "AR",
		Provider:    "arca",
		Environment: "sandbox",
		Mode:        mode,
	}
	require.NoError(t, db.Create(&settings).Error)
	return settings
}

// TestEnqueueIssueJobInTx_CreatesJobOnCallerTransaction: the job row must be
// visible inside the caller's tx and committed with it — the outbox contract.
func TestEnqueueIssueJobInTx_CreatesJobOnCallerTransaction(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := database.Bill{BusinessID: 1, BillNumber: "B-outbox-1",
		TotalAmount: 1000, PaidAmount: 1000, Status: database.BillStatusPaid}
	require.NoError(t, db.Create(&bill).Error)
	outboxTestSettings(t, db, 1, database.FiscalModeAutomaticNonBlocking)

	paymentID := uint(77)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return EnqueueIssueJobInTx(tx, &bill, &paymentID, nil, "system")
	}))

	var job database.FiscalJob
	require.NoError(t, db.Where("bill_id = ? AND action = ?", bill.ID, ActionIssueReceipt).First(&job).Error)
	require.Equal(t, database.FiscalStatusPending, job.Status)
	require.NotNil(t, job.PaymentID)
	require.Equal(t, paymentID, *job.PaymentID)
}

// TestEnqueueIssueJobInTx_RollbackDiscardsJob: a rolled-back settlement must
// not leave a queued factura behind.
func TestEnqueueIssueJobInTx_RollbackDiscardsJob(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := database.Bill{BusinessID: 1, BillNumber: "B-outbox-2",
		TotalAmount: 1000, PaidAmount: 1000, Status: database.BillStatusPaid}
	require.NoError(t, db.Create(&bill).Error)
	outboxTestSettings(t, db, 1, database.FiscalModeAutomaticNonBlocking)

	rollback := gorm.ErrInvalidTransaction
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := EnqueueIssueJobInTx(tx, &bill, nil, nil, "system"); err != nil {
			return err
		}
		return rollback // force rollback AFTER a successful enqueue
	})
	require.ErrorIs(t, err, rollback)

	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Where("bill_id = ?", bill.ID).Count(&count).Error)
	require.Zero(t, count, "rolled-back settlement must not leave a fiscal job")
}

// TestEnqueueIssueJobInTx_SkipsOffManualAndUnpaid: gating must mirror
// HandleBillPaid exactly (service.go:151-170).
func TestEnqueueIssueJobInTx_SkipsOffManualAndUnpaid(t *testing.T) {
	db := newFiscalTestDB(t)

	// manual mode → skip
	billManual := database.Bill{BusinessID: 2, BillNumber: "B-outbox-3",
		TotalAmount: 1000, PaidAmount: 1000, Status: database.BillStatusPaid}
	require.NoError(t, db.Create(&billManual).Error)
	outboxTestSettings(t, db, 2, database.FiscalModeManual)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return EnqueueIssueJobInTx(tx, &billManual, nil, nil, "system")
	}))

	// unpaid bill → skip
	billOpen := database.Bill{BusinessID: 3, BillNumber: "B-outbox-4",
		TotalAmount: 1000, PaidAmount: 500, Status: database.BillStatusPartial}
	require.NoError(t, db.Create(&billOpen).Error)
	outboxTestSettings(t, db, 3, database.FiscalModeAutomaticNonBlocking)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return EnqueueIssueJobInTx(tx, &billOpen, nil, nil, "system")
	}))

	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).Count(&count).Error)
	require.Zero(t, count)
}

// TestEnqueueIssueJobInTx_IdempotentWithPostCommitPath: the in-tx enqueue and
// the legacy post-commit HandleBillPaid must collapse to ONE job via the
// shared idempotency key.
func TestEnqueueIssueJobInTx_IdempotentWithPostCommitPath(t *testing.T) {
	db := newFiscalTestDB(t)
	bill := database.Bill{BusinessID: 4, BillNumber: "B-outbox-5",
		TotalAmount: 1000, PaidAmount: 1000, Status: database.BillStatusPaid}
	require.NoError(t, db.Create(&bill).Error)
	outboxTestSettings(t, db, 4, database.FiscalModeAutomaticNonBlocking)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return EnqueueIssueJobInTx(tx, &bill, nil, nil, "system")
	}))
	// Legacy belt-and-braces post-commit path fires too.
	svc := NewService(db, NewProviderRegistry())
	require.NoError(t, svc.HandleBillPaid(t.Context(), BillPaidInput{BillID: bill.ID, Actor: "system"}))

	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).
		Where("bill_id = ? AND action = ?", bill.ID, ActionIssueReceipt).Count(&count).Error)
	require.Equal(t, int64(1), count, "in-tx + post-commit enqueue must dedupe to one job")
}

// TestEnqueueIssueJobInTx_BillIssueUniqueViolationIsGracefulNoOp: the
// belt-and-braces partial unique index (idx_fiscal_jobs_bill_issue_unique)
// must NEVER abort a payment settlement. If a job for the
// bill already exists under a DIVERGENT idempotency key (so the
// idempotency-key DO NOTHING does not absorb the conflict), the bill-scoped
// index fires — and the enqueue must treat it as already-enqueued (nil), not
// propagate an error that would roll back the settlement transaction.
func TestEnqueueIssueJobInTx_BillIssueUniqueViolationIsGracefulNoOp(t *testing.T) {
	db := newFiscalTestDB(t)
	// Mirror the production partial index (created by the genesis schema /
	// the fiscal ensure-helper on fresh DBs). SQLite supports partial indexes.
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_fiscal_jobs_bill_issue_unique
		ON fiscal_jobs (bill_id, action)
		WHERE action = 'issue_receipt'`).Error)

	bill := database.Bill{BusinessID: 9, BillNumber: "B-outbox-6",
		TotalAmount: 1000, PaidAmount: 1000, Status: database.BillStatusPaid}
	require.NoError(t, db.Create(&bill).Error)
	settings := outboxTestSettings(t, db, 9, database.FiscalModeAutomaticNonBlocking)

	// Pre-existing issue job under a divergent (legacy-format) key.
	require.NoError(t, db.Create(&database.FiscalJob{
		BusinessID: 9, SettingsID: settings.ID, BillID: bill.ID,
		Action: ActionIssueReceipt, IdempotencyKey: "legacy-divergent-key",
		Status: database.FiscalStatusPending, MaxAttempts: 5, CreatedBy: "system",
	}).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return EnqueueIssueJobInTx(tx, &bill, nil, nil, "system")
	}), "bill-scoped unique violation must be a graceful no-op, not a settlement abort")

	var count int64
	require.NoError(t, db.Model(&database.FiscalJob{}).
		Where("bill_id = ? AND action = ?", bill.ID, ActionIssueReceipt).Count(&count).Error)
	require.Equal(t, int64(1), count, "the pre-existing job remains the only one")
}
