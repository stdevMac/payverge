package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
)

func TestGuestFeedbackSchedulerSendGuestFeedbackEmail_SkipsWhenEmailServerUnavailable(t *testing.T) {
	setupTestDB(t)

	originalEmailServer := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	defer func() { emails.EmailServerInstance = originalEmailServer }()

	scheduler := NewGuestFeedbackScheduler(database.GetDBWrapper())
	bill := &database.Bill{
		Business: database.Business{
			Name:            "Feedback Bistro",
			BusinessId:      "feedback-bistro",
			DefaultLanguage: "es",
		},
	}

	require.NoError(t, scheduler.sendGuestFeedbackEmail(bill, "guest@example.com"))
}

// TestGuestFeedbackSchedulerCatchUpAfterDowntime is the downtime regression: the
// candidate window used to be closed_at in [30h-ago, 24h-ago] against a 6h tick,
// so any scheduler gap longer than ~6h silently dropped that stretch of bills'
// feedback emails forever. The window must be wide enough (24h..48h ago) that a
// long gap self-heals; the feedback_email_sent_at claim column dedups, so a
// wider window can never double-send.
func TestGuestFeedbackSchedulerCatchUpAfterDowntime(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	biz := createTestHospitalityBusiness(t, db, "feedback-catchup")

	originalEmailServer := emails.EmailServerInstance
	emails.EmailServerInstance = nil // send = no-op success; the claim must stick
	defer func() { emails.EmailServerInstance = originalEmailServer }()

	cust := &database.Customer{Email: "catchup-guest@example.com", Name: "Catchup Guest"}
	require.NoError(t, db.Create(cust).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID: cust.ID, BusinessID: biz.ID, OptInEmail: true,
	}).Error)

	mkBill := func(hoursAgo int) *database.Bill {
		closedAt := time.Now().Add(-time.Duration(hoursAgo) * time.Hour)
		b := &database.Bill{
			BusinessID: biz.ID, Status: database.BillStatusPaid, ClosedAt: &closedAt,
			CRMCustomerID: &cust.ID, SettlementAddr: "0xsettlement", TippingAddr: "0xtipping",
			BillNumber: fmt.Sprintf("FB-CATCHUP-%d", hoursAgo),
		}
		require.NoError(t, db.Create(b).Error)
		return b
	}

	// Closed 36h ago: the old 24-30h window elapsed during a >6h scheduler gap —
	// this bill MUST still get its feedback email on the next tick.
	missed := mkBill(36)
	// Closed 50h ago: beyond the 48h upper bound — too stale, never emailed.
	stale := mkBill(50)
	// Closed 12h ago: not yet due (the 24h courtesy delay still applies).
	early := mkBill(12)

	gfs := NewGuestFeedbackScheduler(database.GetDBWrapper())
	gfs.checkFeedbackEmails()

	var b database.Bill
	require.NoError(t, db.First(&b, missed.ID).Error)
	require.NotNil(t, b.FeedbackEmailSentAt, "a bill whose window elapsed during downtime must still be emailed")
	b = database.Bill{}
	require.NoError(t, db.First(&b, stale.ID).Error)
	require.Nil(t, b.FeedbackEmailSentAt, "bills older than the 48h bound must not be emailed")
	b = database.Bill{}
	require.NoError(t, db.First(&b, early.ID).Error)
	require.Nil(t, b.FeedbackEmailSentAt, "bills closed <24h ago are not yet due")

	// Second tick: the claim column dedups — no double-send/state change.
	gfs.checkFeedbackEmails()
	b = database.Bill{}
	require.NoError(t, db.First(&b, missed.ID).Error)
	require.NotNil(t, b.FeedbackEmailSentAt)
}

// Demo seed data uses special-use addresses (demo+...@payverge.local) that the
// mailer drops. The scheduler must neither count nor log those as sent, and
// must settle the bill so it is not retried every tick.
func TestGuestFeedbackSchedulerSkipsSpecialUseRecipients(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	biz := createTestHospitalityBusiness(t, db, "feedback-reserved")

	originalEmailServer := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	defer func() { emails.EmailServerInstance = originalEmailServer }()

	cust := &database.Customer{Email: "demo+guest1@payverge.local", Name: "Demo Guest"}
	require.NoError(t, db.Create(cust).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID: cust.ID, BusinessID: biz.ID, OptInEmail: true,
	}).Error)
	closedAt := time.Now().Add(-30 * time.Hour)
	bill := &database.Bill{
		BusinessID: biz.ID, Status: database.BillStatusPaid, ClosedAt: &closedAt,
		CRMCustomerID: &cust.ID, SettlementAddr: "0xsettlement", TippingAddr: "0xtipping",
		BillNumber: "FB-RESERVED-1",
	}
	require.NoError(t, db.Create(bill).Error)

	sent, skipped := NewGuestFeedbackScheduler(database.GetDBWrapper()).checkFeedbackEmails()
	require.Equal(t, 0, sent)
	require.Equal(t, 1, skipped)
	var b database.Bill
	require.NoError(t, db.First(&b, bill.ID).Error)
	require.NotNil(t, b.FeedbackEmailSentAt, "a skipped bill leaves the candidate set")
}
