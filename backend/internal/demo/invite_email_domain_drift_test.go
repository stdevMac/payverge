package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// Live-review regression: the demo pending-invitation FirstOrCreate keyed
// idempotency on (business_id, email), but the unique constraint lives on the
// deterministic `token` column. The demo email domain is config-driven, so when
// it drifts between deploys the same (adminUserID, businessID) yields a new
// email while the token stays fixed. The (business_id, email) lookup then misses
// the existing token-holding row and the INSERT collides on
// idx_staff_invitations_token, aborting the entire demo transaction. The ensure
// must key on the token so a domain change re-uses the existing row cleanly.
func TestEnsureForAdmin_InvitationSurvivesEmailDomainDrift(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-drift@example.com")

	// First run pins the invitation with domain A.
	svcA := NewService(db, Options{
		Now:          fixedNow,
		SeedVersion:  "test-seed-v1",
		BaselineDays: 3,
		EmailDomain:  "demo-a.example.com",
	})
	_, err := svcA.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var beforeCount int64
	require.NoError(t, db.Model(&database.StaffInvitation{}).Count(&beforeCount).Error)
	require.Positive(t, beforeCount, "first ensure should create at least one pending invitation")

	// Second run with the SAME seed version (ensure path, no wipe) but a DIFFERENT
	// email domain AND a LATER clock — this is the real production scenario: a
	// service restart re-runs EnsureAllAdmins at a later wall-clock. Both vectors
	// (drifted demoEmail and advanced now()-derived timestamps) must not leak into
	// any FirstOrCreate WHERE and collide on a unique column.
	laterNow := func() time.Time { return fixedNow().Add(90 * time.Minute) }
	svcB := NewService(db, Options{
		Now:          laterNow,
		SeedVersion:  "test-seed-v1",
		BaselineDays: 3,
		EmailDomain:  "demo-b.example.com",
	})
	_, err = svcB.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err, "email-domain drift must not collide on idx_staff_invitations_token")

	// No duplicate rows were created: the token-keyed ensure re-used the row.
	var afterCount int64
	require.NoError(t, db.Model(&database.StaffInvitation{}).Count(&afterCount).Error)
	require.Equal(t, beforeCount, afterCount,
		"drift re-run must re-use the token-keyed invitation, not insert a duplicate")
}
