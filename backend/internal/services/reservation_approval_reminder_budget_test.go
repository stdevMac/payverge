package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
)

// recordingTenantBudget admits every send and records what was claimed.
type recordingTenantBudget struct {
	mu     sync.Mutex
	claims []emails.TenantMailClaimRequest
}

func (b *recordingTenantBudget) Claim(_ context.Context, req emails.TenantMailClaimRequest) (emails.TenantMailClaim, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.claims = append(b.claims, req)
	return emails.TenantMailClaim{}, nil
}

func (b *recordingTenantBudget) Release(context.Context, emails.TenantMailClaim) error { return nil }

func (b *recordingTenantBudget) snapshot() []emails.TenantMailClaimRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]emails.TenantMailClaimRequest(nil), b.claims...)
}

// Review M1: the "still unactioned" reminder goes to the tenant-typed,
// unverified business.Email. It must be tenant mail, and when the pending
// booking came from the public form it belongs to the guest-booking lane, so
// a flood of anonymous requests cannot spend the operator's budget or the
// venue address's per-recipient allowance that staff and wallet notices share.
func TestApprovalReminderIsTenantMailInTheBookingLane(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	business := createTestHospitalityBusiness(t, db, "approval-remind-budget")
	const contact = "venue-contact@example.test"
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", business.ID).UpdateColumn("email", contact).Error)

	provider := installCaptureEmailServer(t)
	budget := &recordingTenantBudget{}
	emails.EmailServerInstance.SetTenantMailBudget(budget)

	now := time.Now().UTC()
	nextWeek := now.AddDate(0, 0, 7)
	guestMade := seedApprovalSweeperReservation(t, db, business.ID, "pending", nextWeek, now.Add(-13*time.Hour))
	staffMade := seedApprovalSweeperReservation(t, db, business.ID, "pending", nextWeek, now.Add(-13*time.Hour))
	require.NoError(t, db.Model(staffMade).UpdateColumn("created_by", "staff:4").Error)
	staffMade.CreatedBy = "staff:4"

	sweeper := NewReservationApprovalSweeper()
	sweeper.notifyOperatorPending(guestMade, now.Add(11*time.Hour))
	sweeper.notifyOperatorPending(staffMade, now.Add(11*time.Hour))

	require.Len(t, provider.sent, 2)
	claims := budget.snapshot()
	require.Len(t, claims, 2, "every reminder goes through the tenant budget")
	require.Equal(t, business.ID, claims[0].BusinessID)
	require.Equal(t, []string{contact}, claims[0].Recipients)
	require.Equal(t, emails.MailPurposeGuestBookingFollowUp, claims[0].Purpose,
		"a public-form booking's reminder stays in the guest-booking lane")
	require.Equal(t, business.ID, claims[1].BusinessID)
	require.Empty(t, claims[1].Purpose, "a staff-entered booking's reminder is operator mail")
}
