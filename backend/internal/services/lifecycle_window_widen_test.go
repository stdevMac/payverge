package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// silenceEmailServer nils the global email server so send helpers no-op
// success (a "send" == a claim stamp), restoring it on cleanup.
func silenceEmailServer(t *testing.T) {
	t.Helper()
	orig := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	t.Cleanup(func() { emails.EmailServerInstance = orig })
}

// seedAgedBusiness inserts a business created createdDaysAgo ago.
func seedAgedBusiness(t *testing.T, db *gorm.DB, id uint, createdDaysAgo float64) {
	t.Helper()
	require.NoError(t, db.Create(&database.Business{
		ID: id, BusinessId: fmt.Sprintf("aged-%d", id), Name: "Aged Biz",
		OwnerName: "Owner", Email: fmt.Sprintf("owner%d@example.com", id), DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", id).
		UpdateColumn("created_at", time.Now().Add(-time.Duration(createdDaysAgo*24)*time.Hour)).Error)
}

// P1-8 row 1: a business missed on tick N (created 1.5d ago at that tick) is
// 2.5d old on tick N+1 — outside the old [2d,1d] band, inside the widened
// [4d,1d] band — and must still be caught. The one-shot claim owns dedup.
func TestGettingStartedWindow_CatchesMissedTickOnNextTick(t *testing.T) {
	db := setupTestDB(t)
	silenceEmailServer(t)

	seedAgedBusiness(t, db, 1, 2.5) // tick N+1 view of a missed tick-N business → SEND
	seedAgedBusiness(t, db, 2, 3.5) // deep in widened band, claim NULL → SEND
	seedAgedBusiness(t, db, 3, 2.5) // already stamped → SKIP
	stamped := time.Now().Add(-24 * time.Hour)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", 3).
		UpdateColumn("getting_started_email_sent_at", stamped).Error)
	seedAgedBusiness(t, db, 4, 4.5) // outside even the widened band → SKIP

	ls := NewLifecycleScheduler(database.GetDBWrapper())
	ls.checkGettingStartedEmails()

	var b1, b2, b3, b4 database.Business
	require.NoError(t, db.First(&b1, 1).Error)
	require.NoError(t, db.First(&b2, 2).Error)
	require.NoError(t, db.First(&b3, 3).Error)
	require.NoError(t, db.First(&b4, 4).Error)
	require.NotNil(t, b1.GettingStartedEmailSentAt, "missed-tick business must be caught on tick N+1")
	require.NotNil(t, b2.GettingStartedEmailSentAt)
	require.True(t, b3.GettingStartedEmailSentAt.Equal(stamped), "claim owns dedup — no re-stamp")
	require.Nil(t, b4.GettingStartedEmailSentAt, "outside widened band must be skipped")
}

// P1-8 row 4: setup nudge [6d,3d].
func TestSetupNudgeWindow_CatchesMissedTickOnNextTick(t *testing.T) {
	db := setupTestDB(t)
	migrateSetupStatusTables(t, db)
	silenceEmailServer(t)

	seedNudgeBusiness(t, db, 1, 4.5) // missed on tick N (3.5d), caught at 4.5d → SEND
	seedNudgeBusiness(t, db, 2, 6.5) // outside widened band → SKIP

	ls := &LifecycleScheduler{}
	ls.checkSetupNudgeEmails()

	var b1, b2 database.Business
	require.NoError(t, db.First(&b1, 1).Error)
	require.NoError(t, db.First(&b2, 2).Error)
	require.NotNil(t, b1.SetupNudgeEmailSentAt, "missed-tick business must be nudged on tick N+1")
	require.Nil(t, b2.SetupNudgeEmailSentAt)
}

// Perf gate: the day-1 check must project, never SELECT * businesses.
func TestGettingStartedCheck_NarrowProjection(t *testing.T) {
	db := setupTestDB(t)
	silenceEmailServer(t)
	seedAgedBusiness(t, db, 1, 1.5)
	seedAgedBusiness(t, db, 2, 7.5)

	rec := &checkerSQLRecorder{Interface: gormlogger.Discard}
	origLogger := db.Logger
	db.Logger = rec
	// Route global DB queries through the same handle so captured SQL includes
	// the lifecycle Select projections (check* use database.GetDB()).
	database.SetTestDB(db)
	t.Cleanup(func() { db.Logger = origLogger })

	ls := &LifecycleScheduler{db: database.GetDBWrapper()}
	ls.checkGettingStartedEmails()

	require.Zero(t, rec.businessSelectStarCount(),
		"lifecycle checks must not hydrate full business rows; captured: %v", rec.statements)
}
