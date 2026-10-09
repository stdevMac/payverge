package services

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestGettingStartedClaimIsIdempotent asserts a business inside the 1-day
// window is claimed once; a second sweep (restart) does not re-claim.
func TestGettingStartedClaimIsIdempotent(t *testing.T) {
	setupTestDB(t)
	biz := seedTestBusiness(t)
	ls := NewLifecycleScheduler(database.GetDBWrapper())

	if !ls.claimCampaign(context.Background(), biz.ID, "getting_started_email_sent_at") {
		t.Fatal("first claim should win")
	}
	if ls.claimCampaign(context.Background(), biz.ID, "getting_started_email_sent_at") {
		t.Fatal("second claim (restart) should lose")
	}
}

// TestSetupNudgeClaimIsIdempotent mirrors the above for the one-shot
// setup-nudge campaign.
func TestSetupNudgeClaimIsIdempotent(t *testing.T) {
	setupTestDB(t)
	biz := seedTestBusiness(t)

	if !claimOneShotCampaign(context.Background(), biz.ID, "setup_nudge_email_sent_at") {
		t.Fatal("first claim should win")
	}
	if claimOneShotCampaign(context.Background(), biz.ID, "setup_nudge_email_sent_at") {
		t.Fatal("second claim (restart) should lose")
	}
}

// TestClaimsArePerColumn asserts that claiming one campaign column does not
// block a different campaign column for the same business.
func TestClaimsArePerColumn(t *testing.T) {
	setupTestDB(t)
	biz := seedTestBusiness(t)
	ls := NewLifecycleScheduler(database.GetDBWrapper())

	if !ls.claimCampaign(context.Background(), biz.ID, "getting_started_email_sent_at") {
		t.Fatal("getting_started claim should win")
	}
	if !ls.claimCampaign(context.Background(), biz.ID, "setup_nudge_email_sent_at") {
		t.Fatal("setup_nudge claim for the same business should be independent and win")
	}
}
