package demo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestEnsureForAdminReusesPreexistingGlobalPlugins reproduces the production
// demo-ensure failure: the plugins table is GLOBAL and pre-populated by the
// plugin system at startup with its own display_name/description/price. The
// demo's ensurePluginConfigs must reuse those rows by name. A FirstOrCreate that
// passes the full demo spec struct as query conditions can never match the
// pre-existing row (different columns), so it falls through to an INSERT and
// violates the unique idx_plugins_name (SQLSTATE 23505), aborting the whole
// ensure transaction. Matching on name only (Attrs for create values) fixes it.
func TestEnsureForAdminReusesPreexistingGlobalPlugins(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-plugins@example.com")

	for _, name := range []string{"usdc_payment", "telegram", "stripe", "mercadopago"} {
		require.NoError(t, db.Create(&database.Plugin{
			Name:        name,
			DisplayName: name + " (global real)",
			Description: "Production-registered copy with different fields",
			Category:    database.PluginCategoryPayment,
			Version:     "9.9.9",
			IsActive:    true,
		}).Error)
	}

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err, "demo ensure must succeed against pre-existing global plugins")

	for _, name := range []string{"usdc_payment", "telegram", "stripe", "mercadopago"} {
		var count int64
		require.NoError(t, db.Model(&database.Plugin{}).Where("name = ?", name).Count(&count).Error)
		require.Equal(t, int64(1), count, "demo must reuse the existing global %q plugin, not insert a duplicate", name)
	}
}
