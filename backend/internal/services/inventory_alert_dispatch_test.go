package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupInventoryDispatchTestDB(t *testing.T) (*gorm.DB, *database.Business) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.InventorySettings{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
	))
	database.SetTestDB(gormDB)
	ResetTelegramNotificationEligibilityCache()

	business := &database.Business{
		BusinessId:     "inv-dispatch-test",
		OwnerAddress:   "owner",
		Name:           "Inv Dispatch",
		SettlementAddr: "settlement",
		TippingAddr:    "tipping",
	}
	require.NoError(t, gormDB.Create(business).Error)
	require.NoError(t, gormDB.Create(&database.InventorySettings{
		BusinessID:              business.ID,
		InventoryEnabled:        true,
		LowStockWarningsEnabled: true,
		AvailabilitySyncMode:    database.InventoryAvailabilityModeWarn,
	}).Error)
	require.NoError(t, gormDB.Create(&database.InventoryItem{
		BusinessID: business.ID, Name: "Tomatoes", Unit: "kg",
		CurrentQuantity: 1, ReorderThreshold: 5, IsActive: true,
	}).Error)
	return gormDB, business
}

// TestInventoryLowStockEventID_SharedCanonicalKey pins the single dedup key that
// BOTH the on-demand path and the 15-min scheduler must emit, so the outbox
// unique key (business, plugin, type, event_id) collapses cross-path duplicates
// (INV-L1). Two enqueues for the same item/status/day produce one delivery row.
func TestInventoryLowStockEventID_SharedCanonicalKey(t *testing.T) {
	gormDB, business := setupInventoryDispatchTestDB(t)
	at := time.Date(2026, 5, 11, 9, 30, 0, 0, time.UTC)

	require.Equal(t, "inventory:7:low_stock:2026-05-11", inventoryLowStockEventID(7, "low_stock", at))

	items := []database.InventoryItemHealth{
		{ID: 7, Name: "Tomatoes", Unit: "kg", CurrentQuantity: 1, ReorderThreshold: 5, Status: "low_stock"},
	}
	n1, err := EnqueueTelegramInventoryLowStockAlerts(business.ID, items, at)
	require.NoError(t, err)
	require.Equal(t, 1, n1)

	// A second enqueue for the same (item, status, day) — as would happen when
	// the scheduler re-checks an item already alerted on-demand — must dedupe.
	n2, err := EnqueueTelegramInventoryLowStockAlerts(business.ID, items, at)
	require.NoError(t, err)
	require.Equal(t, 0, n2)

	var count int64
	require.NoError(t, gormDB.Model(&database.PluginNotificationDelivery{}).
		Where("business_id = ?", business.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)

	var row database.PluginNotificationDelivery
	require.NoError(t, gormDB.Where("business_id = ?", business.ID).First(&row).Error)
	require.Equal(t, inventoryLowStockEventID(7, "low_stock", at), row.EventID)
}

// TestMaybeEnqueueTelegramInventoryLowStockAlerts_RespectsGate proves the
// manual-adjustment / order path runs the full low-stock summary + enqueue only
// when the Telegram gate is satisfied (INV-L2). With the gate closed, no outbox
// row is written even though a genuinely low item exists.
func TestMaybeEnqueueTelegramInventoryLowStockAlerts_RespectsGate(t *testing.T) {
	gormDB, business := setupInventoryDispatchTestDB(t)
	at := time.Date(2026, 5, 11, 9, 30, 0, 0, time.UTC)

	// Gate CLOSED (no Telegram connection): must not enqueue.
	storeTelegramNotificationEligibility(business.ID, false, nil)
	n, err := MaybeEnqueueTelegramInventoryLowStockAlerts(business.ID, at)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	var count int64
	require.NoError(t, gormDB.Model(&database.PluginNotificationDelivery{}).
		Where("business_id = ?", business.ID).Count(&count).Error)
	require.Equal(t, int64(0), count, "gate closed must not enqueue")

	// Gate OPEN (connected + low_inventory enabled): enqueues the low item.
	storeTelegramNotificationEligibility(business.ID, true, map[string]interface{}{
		"notifications": map[string]interface{}{"low_inventory": true},
	})
	n, err = MaybeEnqueueTelegramInventoryLowStockAlerts(business.ID, at)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	require.NoError(t, gormDB.Model(&database.PluginNotificationDelivery{}).
		Where("business_id = ?", business.ID).Count(&count).Error)
	require.Equal(t, int64(1), count, "gate open must enqueue the low item")
}
