package demo

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"

	"github.com/stretchr/testify/require"
)

// DEMO_MODE hands the showroom to a non-admin owner: the seeded venues belong
// to that account (user_id), while demo_owner_user_id keeps the admin that
// owns the seed. Re-ensuring (the nightly reset re-runs boot) keeps it so.
func TestEnsureAssignsVenuesToShowroomOwner(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-showroom@example.com")
	ownerID, err := demomode.EnsureShowroomOwner(context.Background(), db)
	require.NoError(t, err)
	require.NotEqual(t, admin.ID, ownerID)

	for i := 0; i < 2; i++ {
		svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-v1", BaselineDays: 3, ShowroomOwnerUserID: ownerID})
		_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
		require.NoError(t, err)

		var seeded []database.Business
		require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&seeded).Error)
		require.NotEmpty(t, seeded)
		for _, b := range seeded {
			require.NotNil(t, b.UserID)
			require.Equal(t, ownerID, *b.UserID, "run %d: %s must belong to the showroom owner", i, b.BusinessId)
		}
	}

	venues, err := demomode.ShowroomVenues(context.Background(), db, ownerID)
	require.NoError(t, err)
	require.NotEmpty(t, venues)
	require.Contains(t, venues[0].BusinessId, "-primary", "the primary venue comes first")

	for _, role := range []string{"kitchen", "waiter"} {
		staffRole, ok := demomode.StaffRoleFor(role)
		require.True(t, ok)
		staff, err := demomode.ShowroomStaff(context.Background(), db, ownerID, staffRole)
		require.NoError(t, err, role)
		require.Equal(t, venues[0].ID, staff.BusinessID)
	}
}

func TestEnsureWithoutShowroomOwnerKeepsAdminOwnership(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-plain@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-v1", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)
	var seeded []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&seeded).Error)
	require.NotEmpty(t, seeded)
	for _, b := range seeded {
		require.NotNil(t, b.UserID)
		require.Equal(t, admin.ID, *b.UserID)
	}
}
