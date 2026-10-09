package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// TestQueryLowStockItemsClassifiesNegativeAsOutOfStock is the INV-001
// regression. The classification CASE in lowStockQuery must treat a
// non-positive (zero OR negative) current_quantity as "out_of_stock". In
// "warn" availability mode an approved order can drive current_quantity below
// zero (DeductApprovedOrderInventoryTx only hard-blocks negatives in HardBlock
// mode), so a `= 0` test would mis-label a truly out-of-stock item as
// "low_stock".
//
// The query uses Postgres-only `NOW() - INTERVAL '24 hours'`, so this runs
// against a throwaway Postgres container (the repo's container-test
// convention) rather than the SQLite default.
func TestQueryLowStockItemsClassifiesNegativeAsOutOfStock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}

	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	require.NoError(t, pg.DB.AutoMigrate(
		&database.Business{},
		&database.InventoryItem{},
		&database.InventoryAlertLog{},
		&database.InventorySettings{},
	))

	// inventory_items.business_id carries an FK to businesses; seed the parent.
	biz := database.Business{Name: "Test Resto"}
	require.NoError(t, pg.DB.Create(&biz).Error)
	businessID := biz.ID

	// Enable inventory and warnings so the classification test can observe results.
	require.NoError(t, pg.DB.Create(&database.InventorySettings{
		BusinessID: businessID, InventoryEnabled: true, LowStockWarningsEnabled: true,
	}).Error)

	// Seed items spanning the classification boundary. All have a positive
	// reorder_threshold and current_quantity <= threshold so they qualify for
	// an alert; only the quantity sign should decide the alert_type.
	items := []database.InventoryItem{
		{Name: "negative-stock", BusinessID: businessID, Unit: "kg", CurrentQuantity: -5, ReorderThreshold: 10, IsActive: true},
		{Name: "zero-stock", BusinessID: businessID, Unit: "kg", CurrentQuantity: 0, ReorderThreshold: 10, IsActive: true},
		{Name: "low-stock", BusinessID: businessID, Unit: "kg", CurrentQuantity: 3, ReorderThreshold: 10, IsActive: true},
		{Name: "at-threshold", BusinessID: businessID, Unit: "kg", CurrentQuantity: 10, ReorderThreshold: 10, IsActive: true},
		// Healthy: above threshold, must NOT appear in results at all.
		{Name: "healthy-stock", BusinessID: businessID, Unit: "kg", CurrentQuantity: 50, ReorderThreshold: 10, IsActive: true},
	}
	for i := range items {
		require.NoError(t, pg.DB.Create(&items[i]).Error)
	}

	scheduler := NewInventoryAlertScheduler(pg.DB, nil)

	rows, err := scheduler.queryLowStockItems()
	require.NoError(t, err)

	gotAlertType := make(map[string]string, len(rows))
	for _, r := range rows {
		gotAlertType[r.ItemName] = r.AlertType
	}

	// Negative stock is the bug: it MUST classify as out_of_stock, not low_stock.
	require.Equal(t, "out_of_stock", gotAlertType["negative-stock"],
		"negative current_quantity must classify as out_of_stock (INV-001)")
	require.Equal(t, "out_of_stock", gotAlertType["zero-stock"],
		"zero current_quantity must classify as out_of_stock")
	require.Equal(t, "low_stock", gotAlertType["low-stock"],
		"positive-but-below-threshold quantity must classify as low_stock")
	require.Equal(t, "low_stock", gotAlertType["at-threshold"],
		"at-threshold quantity must classify as low_stock")

	// Healthy stock (above threshold) must be excluded entirely.
	_, healthyPresent := gotAlertType["healthy-stock"]
	require.False(t, healthyPresent, "above-threshold item must not be alerted")
	require.Len(t, rows, 4, "exactly the four at/below-threshold items should be returned")
}

// TestQueryLowStockItemsDedupMatchesNegativeOutOfStock proves the NOT EXISTS
// dedup subquery's CASE stays consistent with the SELECT projection: an
// existing recent out_of_stock alert log for a negative-stock item must
// suppress re-alerting that item. If the two CASE expressions ever diverge
// (e.g. one says `= 0`, the other `<= 0`), the dedup join key for a negative
// item would not match and this would fail.
func TestQueryLowStockItemsDedupMatchesNegativeOutOfStock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}

	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	require.NoError(t, pg.DB.AutoMigrate(
		&database.Business{},
		&database.InventoryItem{},
		&database.InventoryAlertLog{},
		&database.InventorySettings{},
	))

	// inventory_items.business_id carries an FK to businesses; seed the parent.
	biz := database.Business{Name: "Test Resto"}
	require.NoError(t, pg.DB.Create(&biz).Error)
	businessID := biz.ID

	// Enable inventory + warnings so the item can be considered; dedup will then suppress it.
	require.NoError(t, pg.DB.Create(&database.InventorySettings{
		BusinessID: businessID, InventoryEnabled: true, LowStockWarningsEnabled: true,
	}).Error)

	neg := database.InventoryItem{Name: "negative-stock", BusinessID: businessID, Unit: "kg", CurrentQuantity: -5, ReorderThreshold: 10, IsActive: true}
	require.NoError(t, pg.DB.Create(&neg).Error)

	// A recent out_of_stock alert already exists for this negative item.
	require.NoError(t, pg.DB.Create(&database.InventoryAlertLog{
		BusinessID:      businessID,
		InventoryItemID: neg.ID,
		AlertType:       "out_of_stock",
		AlertedAt:       time.Now().UTC().Add(-1 * time.Hour),
	}).Error)

	scheduler := NewInventoryAlertScheduler(pg.DB, nil)
	rows, err := scheduler.queryLowStockItems()
	require.NoError(t, err)

	require.Empty(t, rows,
		"a recent out_of_stock alert must dedup the negative-stock item; "+
			"the SELECT and NOT EXISTS CASE expressions must classify it identically")
}

// TestQueryLowStockItemsRespectsInventoryEnabled asserts that a business whose
// InventorySettings.InventoryEnabled=false receives NO alerts even when
// inventory items are below their reorder threshold.
func TestQueryLowStockItemsRespectsInventoryEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}

	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	require.NoError(t, pg.DB.AutoMigrate(
		&database.Business{},
		&database.InventoryItem{},
		&database.InventoryAlertLog{},
		&database.InventorySettings{},
	))

	// Business with inventory disabled.
	biz := database.Business{Name: "Disabled Inventory Biz"}
	require.NoError(t, pg.DB.Create(&biz).Error)

	// Low-stock item that would normally trigger an alert.
	item := database.InventoryItem{
		Name: "low-but-disabled", BusinessID: biz.ID,
		Unit: "kg", CurrentQuantity: 2, ReorderThreshold: 10, IsActive: true,
	}
	require.NoError(t, pg.DB.Create(&item).Error)

	// Explicitly disable inventory for this business.
	// Use raw SQL to bypass GORM's zero-value omission: InventoryEnabled=false
	// is a zero-value bool that GORM would skip, letting Postgres apply
	// DEFAULT false (which happens to be correct here) — but be explicit to
	// guard against any future schema default change.
	require.NoError(t, pg.DB.Exec(
		`INSERT INTO inventory_settings (business_id, inventory_enabled, low_stock_warnings_enabled) VALUES (?, ?, ?)`,
		biz.ID, false, true,
	).Error)

	scheduler := NewInventoryAlertScheduler(pg.DB, nil)
	rows, err := scheduler.queryLowStockItems()
	require.NoError(t, err)

	// Must be empty: inventory_enabled=false should suppress all alerts.
	require.Empty(t, rows,
		"a business with inventory_enabled=false must not receive low-stock alerts")
}

// TestQueryLowStockItemsRespectsLowStockWarningsEnabled asserts that a business
// with InventoryEnabled=true but LowStockWarningsEnabled=false receives no
// alerts even when items are below their reorder threshold.
func TestQueryLowStockItemsRespectsLowStockWarningsEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}

	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	require.NoError(t, pg.DB.AutoMigrate(
		&database.Business{},
		&database.InventoryItem{},
		&database.InventoryAlertLog{},
		&database.InventorySettings{},
	))

	// Business with warnings disabled.
	biz := database.Business{Name: "Warnings Disabled Biz"}
	require.NoError(t, pg.DB.Create(&biz).Error)

	item := database.InventoryItem{
		Name: "low-but-no-warnings", BusinessID: biz.ID,
		Unit: "kg", CurrentQuantity: 1, ReorderThreshold: 5, IsActive: true,
	}
	require.NoError(t, pg.DB.Create(&item).Error)

	// Inventory enabled, but warnings specifically disabled.
	// Use raw SQL to bypass GORM's zero-value omission for boolean columns that
	// carry DEFAULT true in the schema; GORM would otherwise skip the explicit
	// false and let Postgres apply the default.
	require.NoError(t, pg.DB.Exec(
		`INSERT INTO inventory_settings (business_id, inventory_enabled, low_stock_warnings_enabled) VALUES (?, ?, ?)`,
		biz.ID, true, false,
	).Error)

	scheduler := NewInventoryAlertScheduler(pg.DB, nil)
	rows, err := scheduler.queryLowStockItems()
	require.NoError(t, err)

	require.Empty(t, rows,
		"a business with low_stock_warnings_enabled=false must not receive low-stock alerts")
}

// TestQueryLowStockItemsAllowsWhenBothSettingsEnabled confirms that a business
// with InventoryEnabled=true AND LowStockWarningsEnabled=true still receives
// alerts for below-threshold items (the happy path is not broken by the fix).
func TestQueryLowStockItemsAllowsWhenBothSettingsEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}

	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	require.NoError(t, pg.DB.AutoMigrate(
		&database.Business{},
		&database.InventoryItem{},
		&database.InventoryAlertLog{},
		&database.InventorySettings{},
	))

	biz := database.Business{Name: "Fully Enabled Biz"}
	require.NoError(t, pg.DB.Create(&biz).Error)

	item := database.InventoryItem{
		Name: "genuinely-low", BusinessID: biz.ID,
		Unit: "kg", CurrentQuantity: 3, ReorderThreshold: 10, IsActive: true,
	}
	require.NoError(t, pg.DB.Create(&item).Error)

	settings := database.InventorySettings{
		BusinessID:              biz.ID,
		InventoryEnabled:        true,
		LowStockWarningsEnabled: true,
	}
	require.NoError(t, pg.DB.Create(&settings).Error)

	scheduler := NewInventoryAlertScheduler(pg.DB, nil)
	rows, err := scheduler.queryLowStockItems()
	require.NoError(t, err)

	require.Len(t, rows, 1, "a fully-enabled business must receive an alert for its low-stock item")
	require.Equal(t, item.ID, rows[0].ItemID)
	require.Equal(t, "low_stock", rows[0].AlertType)
}

// TestQueryLowStockItemsNoSettingsRowSuppressesAlert asserts that a business
// with NO row in inventory_settings (settings never saved) does NOT receive
// alerts, because inventory_enabled defaults to false and a missing settings row
// is equivalent to inventory disabled.
func TestQueryLowStockItemsNoSettingsRowSuppressesAlert(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}

	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	require.NoError(t, pg.DB.AutoMigrate(
		&database.Business{},
		&database.InventoryItem{},
		&database.InventoryAlertLog{},
		&database.InventorySettings{},
	))

	// Business with no inventory_settings row at all.
	biz := database.Business{Name: "No Settings Biz"}
	require.NoError(t, pg.DB.Create(&biz).Error)

	item := database.InventoryItem{
		Name: "low-no-settings", BusinessID: biz.ID,
		Unit: "kg", CurrentQuantity: 1, ReorderThreshold: 5, IsActive: true,
	}
	require.NoError(t, pg.DB.Create(&item).Error)

	// Intentionally no InventorySettings row for this business.

	scheduler := NewInventoryAlertScheduler(pg.DB, nil)
	rows, err := scheduler.queryLowStockItems()
	require.NoError(t, err)

	require.Empty(t, rows,
		"a business with no inventory_settings row must not receive low-stock alerts "+
			"(inventory_enabled defaults to false)")
}
