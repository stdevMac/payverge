package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginSyncTestDB(t *testing.T) {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Plugin{}))
	SetTestDB(gormDB)
}

// A code-backed plugin that reports IsActive()==false (e.g. an env-disabled
// provider) must be able to deactivate its catalog row. GORM's Updates(struct)
// skips zero values, so is_active=false never propagated with the old
// UpdatePlugin; UpdatePluginForSync writes it explicitly.
func TestUpdatePluginForSyncPropagatesInactive(t *testing.T) {
	setupPluginSyncTestDB(t)

	created, err := CreatePlugin(Plugin{
		Name:        "acmepay",
		DisplayName: "AcmePay",
		Category:    PluginCategoryPayment,
		Version:     "1.0.0",
		IsActive:    true,
	})
	require.NoError(t, err)
	require.True(t, created.IsActive)

	// Sync a definition whose IsActive() is false over the active row.
	updated, err := UpdatePluginForSync(created.ID, Plugin{
		Name:        "acmepay",
		DisplayName: "AcmePay",
		Category:    PluginCategoryPayment,
		Version:     "1.0.0",
		IsActive:    false,
	})
	require.NoError(t, err)
	require.False(t, updated.IsActive, "is_active=false must propagate through sync")

	reloaded, err := GetPluginByName("acmepay")
	require.NoError(t, err)
	require.False(t, reloaded.IsActive, "catalog row must be inactive after sync")
}

// The reverse direction (re-activation) must also work, and the non-zero display
// columns must update normally.
func TestUpdatePluginForSyncReactivatesAndUpdatesColumns(t *testing.T) {
	setupPluginSyncTestDB(t)

	created, err := CreatePlugin(Plugin{
		Name:        "stripe",
		DisplayName: "Old Name",
		Category:    PluginCategoryPayment,
		Version:     "0.9.0",
		IsActive:    false,
	})
	require.NoError(t, err)

	updated, err := UpdatePluginForSync(created.ID, Plugin{
		Name:        "stripe",
		DisplayName: "Stripe",
		Category:    PluginCategoryPayment,
		Version:     "1.0.0",
		IsActive:    true,
	})
	require.NoError(t, err)
	require.True(t, updated.IsActive)
	require.Equal(t, "Stripe", updated.DisplayName)
	require.Equal(t, "1.0.0", updated.Version)
}
