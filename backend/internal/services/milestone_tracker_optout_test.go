package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/stretchr/testify/require"
)

// seedPendingFirstOrderEvent inserts a pending first-order milestone event,
// exactly as recordFirstOrderMilestoneTx would at payment time.
func seedPendingFirstOrderEvent(t *testing.T, businessID uint) database.BusinessMilestoneEvent {
	t.Helper()
	event := database.BusinessMilestoneEvent{
		BusinessID:    businessID,
		MilestoneType: database.BusinessMilestoneTypeFirstOrder,
		Status:        database.BusinessMilestoneStatusPending,
	}
	require.NoError(t, database.GetDB().Create(&event).Error)
	return event
}

// TestProcessPendingMilestones_SkipsOptedOutOwner locks the opt-out parity the
// P1-5 single-owner change must preserve: the retired lifecycle milestone
// checks consulted marketingEmailOptedOut; the event system — now the only
// sender — must do the same. Skipped events are marked sent so they never
// retry (one-shot semantics preserved).
func TestProcessPendingMilestones_SkipsOptedOutOwner(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.User{}, &database.BusinessMilestoneEvent{}))

	provider := installCaptureEmailServer(t)

	optedOut := &database.User{Email: "optout@example.com"}
	optedOut.NotificationPreferences = structs.NotificationPreferences{EmailEnabled: false}
	require.NoError(t, db.Create(optedOut).Error)
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", optedOut.ID).
		Update("email_enabled", false).Error)

	biz := database.Business{
		ID: 1, BusinessId: "optout-biz", Name: "OptOut", OwnerName: "Owner",
		Email: "optout-biz@example.com", DefaultLanguage: "en", UserID: &optedOut.ID,
	}
	require.NoError(t, db.Create(&biz).Error)
	event := seedPendingFirstOrderEvent(t, biz.ID)

	tracker := &MilestoneTracker{}
	require.NoError(t, tracker.ProcessPendingMilestones(biz.ID))

	require.Empty(t, provider.sent, "opted-out owner must not receive a milestone email")

	var after database.BusinessMilestoneEvent
	require.NoError(t, db.First(&after, event.ID).Error)
	require.Equal(t, database.BusinessMilestoneStatusSent, after.Status,
		"skipped event must be marked sent so it never retries")
}

// TestProcessPendingMilestones_SkipsDemoBusiness — demo-exclusion parity
// ("demo exclusion everywhere", review Verified-clean appendix).
func TestProcessPendingMilestones_SkipsDemoBusiness(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.User{}, &database.BusinessMilestoneEvent{}))

	provider := installCaptureEmailServer(t)

	biz := database.Business{
		ID: 1, BusinessId: "demo-biz", Name: "Demo", OwnerName: "Owner",
		Email: "demo@example.com", DefaultLanguage: "en", IsDemo: true,
	}
	require.NoError(t, db.Create(&biz).Error)
	event := seedPendingFirstOrderEvent(t, biz.ID)

	tracker := &MilestoneTracker{}
	require.NoError(t, tracker.ProcessPendingMilestones(biz.ID))

	require.Empty(t, provider.sent, "demo business must not receive a milestone email")
	var after database.BusinessMilestoneEvent
	require.NoError(t, db.First(&after, event.ID).Error)
	require.Equal(t, database.BusinessMilestoneStatusSent, after.Status)
}

// TestProcessPendingMilestones_SendsForOptedInOwner is the control: a real,
// opted-in business still gets exactly one celebration email.
func TestProcessPendingMilestones_SendsForOptedInOwner(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.User{}, &database.BusinessMilestoneEvent{}))

	provider := installCaptureEmailServer(t)

	biz := database.Business{
		ID: 1, BusinessId: "optin-biz", Name: "OptIn", OwnerName: "Owner",
		Email: "optin-biz@example.com", DefaultLanguage: "en",
	}
	require.NoError(t, db.Create(&biz).Error)
	seedPendingFirstOrderEvent(t, biz.ID)

	tracker := &MilestoneTracker{}
	require.NoError(t, tracker.ProcessPendingMilestones(biz.ID))

	require.Len(t, provider.sent, 1, "opted-in business gets exactly one celebration email")
}
