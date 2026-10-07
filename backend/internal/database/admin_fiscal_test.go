package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAdminFiscalDB(t testing.TB) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:admin_fiscal_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	SetTestDB(db)
	require.NoError(t, db.AutoMigrate(
		&Business{},
		&BusinessFiscalSettings{},
		&FiscalJob{},
		&FiscalReceipt{},
	))
	return db
}

func seedAdminFiscalBusiness(t testing.TB, db *gorm.DB, name string, kind BusinessKind) Business {
	t.Helper()
	now := time.Now().UTC()
	b := Business{
		BusinessId: fmt.Sprintf("biz-%s-%d", kind, time.Now().UnixNano()),
		Name:       name,
		OwnerName:  "Owner",
		Kind:       kind,
		IsDemo:     kind == BusinessKindDemo,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	require.NoError(t, db.Create(&b).Error)
	return b
}

// TestListAdminFiscalJobs_IncludesBusinessNameAndExcludesNonReal locks Task 15:
// the production fiscal queue projects the business display name and, by default,
// excludes kind != real so demo AFIP jobs never pad the admin queue.
func TestListAdminFiscalJobs_IncludesBusinessNameAndExcludesNonReal(t *testing.T) {
	db := setupAdminFiscalDB(t)

	realBiz := seedAdminFiscalBusiness(t, db, "Real Bistro", BusinessKindReal)
	demoBiz := seedAdminFiscalBusiness(t, db, "Demo Cafe", BusinessKindDemo)
	testBiz := seedAdminFiscalBusiness(t, db, "CI Fixture", BusinessKindTest)

	mkJob := func(biz Business, key string, status FiscalStatus) {
		t.Helper()
		require.NoError(t, db.Create(&FiscalJob{
			BusinessID:     biz.ID,
			SettingsID:     1,
			BillID:         biz.ID * 10,
			Action:         "issue_receipt",
			IdempotencyKey: key,
			Status:         status,
			MaxAttempts:    5,
		}).Error)
	}
	mkJob(realBiz, "k-real", FiscalStatusFailedRetryable)
	mkJob(demoBiz, "k-demo", FiscalStatusPending)
	mkJob(testBiz, "k-test", FiscalStatusFailedPermanent)

	// Default (kind=real): only the real job, with the business name joined in.
	rows, total, err := ListAdminFiscalJobsWithKind(25, 0, "", string(BusinessKindReal))
	require.NoError(t, err)
	assert.EqualValues(t, 1, total, "default kind=real must exclude demo/test fiscal jobs")
	require.Len(t, rows, 1)
	assert.Equal(t, realBiz.ID, rows[0].BusinessID)
	assert.Equal(t, "Real Bistro", rows[0].BusinessName,
		"queue projection must join businesses.name — not raw #id")
	assert.Equal(t, FiscalStatusFailedRetryable, rows[0].Status)

	// Explicit kind=all surfaces every job (debug / support).
	all, allTotal, err := ListAdminFiscalJobsWithKind(25, 0, "", AdminBusinessKindAll)
	require.NoError(t, err)
	assert.EqualValues(t, 3, allTotal)
	require.Len(t, all, 3)
}

// TestAdminRequeueFiscalJob_FailedRetryableIsRequeueable locks the requeue
// control for failed jobs (Task 15 Actions column).
func TestAdminRequeueFiscalJob_FailedRetryableIsRequeueable(t *testing.T) {
	db := setupAdminFiscalDB(t)
	biz := seedAdminFiscalBusiness(t, db, "Retry Cafe", BusinessKindReal)
	job := FiscalJob{
		BusinessID:     biz.ID,
		SettingsID:     1,
		BillID:         99,
		Action:         "issue_receipt",
		IdempotencyKey: "k-requeue",
		Status:         FiscalStatusFailedRetryable,
		Attempts:       4,
		MaxAttempts:    5,
	}
	require.NoError(t, db.Create(&job).Error)

	out, err := AdminRequeueFiscalJob(job.ID, false)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, FiscalStatusPending, out.Status)
	assert.Equal(t, 0, out.Attempts)
}

// TestGetAdminFiscalSummary_JobCountsScopeToReal ensures summary counters for
// due/failed jobs ignore kind!=real (demo data must not report 371 AFIP jobs).
func TestGetAdminFiscalSummary_JobCountsScopeToReal(t *testing.T) {
	db := setupAdminFiscalDB(t)
	realBiz := seedAdminFiscalBusiness(t, db, "Real One", BusinessKindReal)
	demoBiz := seedAdminFiscalBusiness(t, db, "Demo One", BusinessKindDemo)

	require.NoError(t, db.Create(&BusinessFiscalSettings{
		BusinessID: realBiz.ID, Mode: FiscalModeManual, Country: "AR", Provider: "arca",
	}).Error)
	require.NoError(t, db.Create(&BusinessFiscalSettings{
		BusinessID: demoBiz.ID, Mode: FiscalModeManual, Country: "AR", Provider: "arca",
	}).Error)

	require.NoError(t, db.Create(&FiscalJob{
		BusinessID: realBiz.ID, SettingsID: 1, BillID: 1, Action: "issue_receipt",
		IdempotencyKey: "sum-real", Status: FiscalStatusFailedRetryable, MaxAttempts: 5,
	}).Error)
	// Flood of demo failures that must not appear in the production summary.
	for i := 0; i < 5; i++ {
		require.NoError(t, db.Create(&FiscalJob{
			BusinessID: demoBiz.ID, SettingsID: 2, BillID: uint(100 + i), Action: "issue_receipt",
			IdempotencyKey: fmt.Sprintf("sum-demo-%d", i), Status: FiscalStatusFailedRetryable, MaxAttempts: 5,
		}).Error)
	}

	summary, err := GetAdminFiscalSummary()
	require.NoError(t, err)
	assert.EqualValues(t, 1, summary.BusinessesWithFiscal)
	assert.EqualValues(t, 1, summary.FailedRetryableJobs,
		"failed_retryable_jobs must count only kind=real")
	assert.EqualValues(t, 1, summary.JobsByStatus[string(FiscalStatusFailedRetryable)])
}
