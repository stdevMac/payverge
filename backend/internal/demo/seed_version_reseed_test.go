package demo

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// Live-review regression: legacy demo instances stored an EMPTY seed_version,
// and the reseed guard exempted "" from the mismatch check. Bumping
// DefaultSeedVersion then stamped the new version WITHOUT wiping, permanently
// defusing every future forced-regeneration bump — the live showroom kept
// stale shifts, owner names, and duplicated loyalty tiers. An empty stored
// version must be treated as a mismatch and trigger a full wipe + reseed.
func TestEmptySeedVersionForcesReseed(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-reseed@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-v1", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var before []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&before).Error)
	require.NotEmpty(t, before)

	// Simulate the legacy pre-versioning instance.
	require.NoError(t, db.Model(&database.DemoInstance{}).
		Where("admin_user_id = ?", admin.ID).
		Update("seed_version", "").Error)

	// Plant a stale marker that only a wipe removes (ensure would keep it).
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", before[0].ID).
		Update("owner_name", "Stale Owner").Error)

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var after []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&after).Error)
	require.NotEmpty(t, after)
	for _, business := range after {
		require.Equal(t, expectedDemoOwner(business), business.OwnerName,
			"empty seed_version must force a wipe+reseed; stale business %d survived", business.ID)
	}

	var instance database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).First(&instance).Error)
	require.Equal(t, "test-seed-v1", instance.SeedVersion)
}
