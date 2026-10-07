package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLifecycleSchedulerEmailHelpers_SkipWhenEmailServerUnavailable(t *testing.T) {
	originalEmailServer := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	t.Cleanup(func() {
		emails.EmailServerInstance = originalEmailServer
	})

	scheduler := &LifecycleScheduler{}
	business := &database.Business{
		ID:              42,
		BusinessId:      "lifecycle-email-biz",
		Name:            "Lifecycle Biz",
		OwnerName:       "Owner",
		Email:           "owner@example.com",
		DefaultLanguage: "en",
	}

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "getting_started",
			call: func() error { return scheduler.sendGettingStartedEmail(business) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.call())
		})
	}
}

func TestMilestoneTrackerEmailHelpers_SkipWhenEmailServerUnavailable(t *testing.T) {
	originalEmailServer := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	t.Cleanup(func() {
		emails.EmailServerInstance = originalEmailServer
	})

	tracker := &MilestoneTracker{}
	business := &database.Business{
		ID:              99,
		BusinessId:      "milestone-email-biz",
		Name:            "Milestone Biz",
		OwnerName:       "Owner",
		Email:           "owner@example.com",
		DefaultLanguage: "en",
	}

	require.NoError(t, tracker.sendFirstOrderEmail(business))
	require.NoError(t, tracker.sendRevenueMilestoneEmail(business, "$10,000"))
}

// migrateSetupStatusTables migrates the tables ComputeSetupStatus queries so a
// scheduler test that reaches the setup-summary computation runs cleanly.
func migrateSetupStatusTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(
		&database.Table{},
		&database.Menu{},
		&database.Staff{},
		&database.Plugin{},
		&database.BusinessPlugin{},
	))
}

// testUserAuth mirrors the auth package's UserAuth columns the scheduler
// queries by raw table name (services cannot import internal/auth — cycle).
type testUserAuth struct {
	ID            uint `gorm:"primaryKey"`
	UserID        uint `gorm:"index"`
	Provider      string
	EmailVerified bool `gorm:"default:false"`
}

func (testUserAuth) TableName() string { return "user_auths" }

// TestOwnerEmailUnverified covers the day-3 nudge's verify-reminder signal:
// true only when the business has an owner user whose email-auth record is
// unverified; nil owner, verified owner, and lookup-miss all report false.
func TestOwnerEmailUnverified(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION", "required")
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&testUserAuth{}))

	unverifiedOwner := uint(701)
	verifiedOwner := uint(702)
	require.NoError(t, db.Create(&testUserAuth{UserID: unverifiedOwner, Provider: "email", EmailVerified: false}).Error)
	require.NoError(t, db.Create(&testUserAuth{UserID: verifiedOwner, Provider: "email", EmailVerified: true}).Error)

	bizUnverified := database.Business{ID: 71, BusinessId: "nudge-verify-71", Name: "B", OwnerName: "O", Email: "o71@example.com", UserID: &unverifiedOwner}
	bizVerified := database.Business{ID: 72, BusinessId: "nudge-verify-72", Name: "B", OwnerName: "O", Email: "o72@example.com", UserID: &verifiedOwner}
	bizNoOwner := database.Business{ID: 73, BusinessId: "nudge-verify-73", Name: "B", OwnerName: "O", Email: "o73@example.com"}
	require.NoError(t, db.Create(&bizUnverified).Error)
	require.NoError(t, db.Create(&bizVerified).Error)
	require.NoError(t, db.Create(&bizNoOwner).Error)

	require.True(t, ownerEmailUnverified(&bizUnverified), "unverified email-auth owner must trigger the reminder")
	require.False(t, ownerEmailUnverified(&bizVerified), "verified owner must not trigger the reminder")
	require.False(t, ownerEmailUnverified(&bizNoOwner), "web3-only business (no user) must not trigger the reminder")

	// EMAIL_VERIFICATION=off leaves the flag false, but there is nothing to
	// verify, so the nudge must not ask for it.
	t.Setenv("EMAIL_VERIFICATION", "off")
	require.False(t, ownerEmailUnverified(&bizUnverified), "verification off: no verify reminder")
}

// seedNudgeBusiness inserts a business created `createdDaysAgo` ago (with an
// email) so the day-3 setup-nudge window ([6d,3d] ago after P1-8) can target it.
func seedNudgeBusiness(t *testing.T, db *gorm.DB, id uint, createdDaysAgo float64) database.Business {
	t.Helper()
	biz := database.Business{
		ID: id, BusinessId: fmt.Sprintf("nudge-%d", id), Name: "Nudge Biz",
		OwnerName: "Owner", Email: fmt.Sprintf("owner%d@example.com", id), DefaultLanguage: "en",
	}
	require.NoError(t, db.Create(&biz).Error)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", id).
		UpdateColumn("created_at", time.Now().Add(-time.Duration(createdDaysAgo*24)*time.Hour)).Error)
	return biz
}

// TestCheckSetupNudgeEmails covers the day-3 conditional nudge: a business in the
// [6d,3d]-ago window (P1-8) whose required setup is incomplete and whose claim is
// NULL is sent+stamped; a business that has finished required setup is NOT sent;
// an already-stamped one is skipped.
func TestCheckSetupNudgeEmails(t *testing.T) {
	db := setupTestDB(t)
	migrateSetupStatusTables(t, db)

	orig := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	t.Cleanup(func() { emails.EmailServerInstance = orig })

	// Business 1: created 3.5d ago, empty (menu missing) → SEND + stamp.
	seedNudgeBusiness(t, db, 1, 3.5)

	// Business 2: created 3.5d ago but fully set up (required_done) → NOT sent.
	seedNudgeBusiness(t, db, 2, 3.5)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", 2).
		Updates(map[string]interface{}{"default_currency": "USD"}).Error)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", 2).
		UpdateColumn("city", "Buenos Aires").Error)
	require.NoError(t, db.Create(&database.Table{BusinessID: 2, TableCode: "N2-1", Name: "1", IsActive: true}).Error)
	require.NoError(t, db.Create(&database.Menu{BusinessID: 2, IsActive: true,
		Categories: `[{"name":"Mains","items":[{"name":"Burger"}]}]`}).Error)

	// Business 3: in window, incomplete, but already stamped → SKIP (no re-stamp).
	seedNudgeBusiness(t, db, 3, 3.5)
	stampedAt := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", 3).
		UpdateColumn("setup_nudge_email_sent_at", stampedAt).Error)

	// Business 4: created 10d ago → outside window → SKIP.
	seedNudgeBusiness(t, db, 4, 10)

	ls := &LifecycleScheduler{}
	ls.checkSetupNudgeEmails()

	var b1, b2, b3, b4 database.Business
	require.NoError(t, db.First(&b1, 1).Error)
	require.NoError(t, db.First(&b2, 2).Error)
	require.NoError(t, db.First(&b3, 3).Error)
	require.NoError(t, db.First(&b4, 4).Error)

	require.NotNil(t, b1.SetupNudgeEmailSentAt, "incomplete in-window business must be nudged")
	require.Nil(t, b2.SetupNudgeEmailSentAt, "fully-set-up business must not be nudged")
	require.NotNil(t, b3.SetupNudgeEmailSentAt)
	require.True(t, b3.SetupNudgeEmailSentAt.Equal(stampedAt), "already-stamped business must not be re-stamped")
	require.Nil(t, b4.SetupNudgeEmailSentAt, "out-of-window business must be skipped")
}

// TestComputeSetupStatus_FirstMissingStep verifies the shared helper the
// scheduler uses: an empty business reports menu as the first missing required
// step; a fully-set-up business reports no missing step.
func TestComputeSetupStatus_FirstMissingStep(t *testing.T) {
	db := setupTestDB(t)
	migrateSetupStatusTables(t, db)

	// Empty business: name only, nothing else.
	require.NoError(t, db.Create(&database.Business{
		ID: 1, BusinessId: "empty-1", Name: "Empty", OwnerName: "Owner", Email: "e@x.com",
	}).Error)

	empty, err := ComputeSetupStatus(1)
	require.NoError(t, err)
	require.False(t, empty.RequiredDone)
	require.Equal(t, "menu", empty.FirstMissingRequiredStep())

	// Fully-set-up business: profile + a table + a menu with an item + a plugin.
	require.NoError(t, db.Create(&database.Business{
		ID: 2, BusinessId: "ready-2", Name: "Ready", OwnerName: "Owner", Email: "r@x.com",
		Address: database.BusinessAddress{City: "Buenos Aires"}, DefaultCurrency: "USD",
	}).Error)
	require.NoError(t, db.Create(&database.Table{BusinessID: 2, TableCode: "T2-1", Name: "1", IsActive: true}).Error)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: 2, IsActive: true,
		Categories: `[{"name":"Mains","items":[{"name":"Burger"}]}]`,
	}).Error)
	require.NoError(t, db.Create(&database.Plugin{ID: 1, Name: "usdc_payment", DisplayName: "USDC", IsActive: true}).Error)
	require.NoError(t, db.Create(&database.BusinessPlugin{BusinessID: 2, PluginID: 1, IsEnabled: true}).Error)

	ready, err := ComputeSetupStatus(2)
	require.NoError(t, err)
	require.True(t, ready.RequiredDone)
	require.Equal(t, "", ready.FirstMissingRequiredStep())

	// Crypto-plugin gating: with only the USDC plugin enabled and NO settlement
	// (payout) address, the payment step must NOT count as done — guests can't
	// actually pay. Adding the payout wallet flips it.
	require.False(t, ready.PaymentDone, "crypto plugin without a payout wallet must not complete the payment step")

	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", 2).
		UpdateColumn("settlement_addr", "0x1111111111111111111111111111111111111111").Error)

	withWallet, err := ComputeSetupStatus(2)
	require.NoError(t, err)
	require.True(t, withWallet.PaymentDone, "crypto plugin plus payout wallet completes the payment step")
}

// seedMilestoneBusiness inserts a plain business (with an email) for the
// milestone campaigns to target.
func seedMilestoneBusiness(t *testing.T, db *gorm.DB, id uint) database.Business {
	t.Helper()
	biz := database.Business{
		ID: id, BusinessId: fmt.Sprintf("milestone-%d", id), Name: "Milestone Biz",
		OwnerName: "Owner", Email: fmt.Sprintf("owner%d@example.com", id), DefaultLanguage: "en",
	}
	require.NoError(t, db.Create(&biz).Error)
	return biz
}

// seedPaidBill inserts a paid bill of the given cents amount for a business.
func seedPaidBill(t *testing.T, db *gorm.DB, businessID uint, cents int64) {
	t.Helper()
	require.NoError(t, db.Create(&database.Bill{
		BusinessID: businessID, Status: database.BillStatusPaid, TotalAmount: cents,
		BillNumber: fmt.Sprintf("PB-%d-%d", businessID, cents),
	}).Error)
}

// TestCheckLifecycleEmails_MilestonesAreEventSystemOwned locks the P1-5 dedup
// fix: milestone celebrations are owned exclusively by the transactional event
// system (milestone_tracker.go). The daily lifecycle tick must no longer stamp
// or send first-order / revenue milestones — previously BOTH systems fired,
// duplicating every celebration email (~1 minute apart and next daily tick).
// The mig-000118 claim columns stay in the schema as historical dedup; this
// asserts they remain NULL after a full tick.
func TestCheckLifecycleEmails_MilestonesAreEventSystemOwned(t *testing.T) {
	db := setupTestDB(t)
	migrateSetupStatusTables(t, db)
	require.NoError(t, db.AutoMigrate(&database.Bill{}, &database.User{}))

	orig := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	t.Cleanup(func() { emails.EmailServerInstance = orig })

	// Exactly the business the OLD daily checks would have targeted on this
	// tick: fresh paid bill over $10k, both claim columns NULL.
	seedMilestoneBusiness(t, db, 1)
	seedPaidBill(t, db, 1, 1_200_000)

	ls := &LifecycleScheduler{}
	ls.checkLifecycleEmails()

	var b database.Business
	require.NoError(t, db.First(&b, 1).Error)
	require.Nil(t, b.FirstOrderMilestoneSentAt,
		"lifecycle tick must not stamp first-order milestones (event system owns them)")
	require.Nil(t, b.RevenueMilestoneSentAt,
		"lifecycle tick must not stamp revenue milestones (event system owns them)")
}
