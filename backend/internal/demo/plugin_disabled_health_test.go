package demo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Issue #795: the demo seeder stamped last_status="ok" + last_success_at on
// EVERY seeded business plugin, including the ones it seeds disabled. That is
// exactly the production symptom (all rails is_enabled=false, last_status=ok):
// health is webhook evidence, not decoration, and a rail that never processed
// a webhook — and is off — must not carry an ok stamp.
func TestDemoEnsureDoesNotStampOkOnDisabledPlugins(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-795@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var disabled []database.BusinessPlugin
	require.NoError(t, db.Where("is_enabled = ?", false).Find(&disabled).Error)
	require.NotEmpty(t, disabled, "demo seeds disabled plugin subscriptions")
	for _, bp := range disabled {
		require.NotEqual(t, "ok", bp.LastStatus,
			"disabled demo rail (plugin %d) must not be stamped last_status=ok (#795)", bp.PluginID)
		require.Nil(t, bp.LastSuccessAt,
			"disabled demo rail (plugin %d) must not carry a fake last_success_at (#795)", bp.PluginID)
	}
}

// The ensure pass self-heals rows an earlier (buggy) seed already stamped:
// a reseed clears the fake ok health from disabled rows.
func TestDemoEnsureSelfHealsFakeOkOnDisabledPlugins(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-795-heal@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	// Poison every disabled row the way the pre-#795 seeder did.
	now := fixedNow().UTC()
	require.NoError(t, db.Model(&database.BusinessPlugin{}).
		Where("is_enabled = ?", false).
		Updates(map[string]interface{}{"last_status": "ok", "last_success_at": now}).Error)

	// A reseed (new seed version forces the ensure pass to rewrite rows) must
	// clear the fake health.
	svc2 := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-2", BaselineDays: 30})
	_, err = svc2.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var stillFake int64
	require.NoError(t, db.Model(&database.BusinessPlugin{}).
		Where("is_enabled = ? AND last_status = ?", false, "ok").
		Count(&stillFake).Error)
	require.Zero(t, stillFake,
		"reseed must self-heal disabled rails stamped ok by the old seeder (#795)")
}
