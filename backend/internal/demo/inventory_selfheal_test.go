package demo

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestEnsureInventoryIdempotentUniqueSKU proves the demo seed lands exactly one
// inventory item per SKU no matter how many times it runs. Before the
// (business_id, sku) unique index the FirstOrCreate read-then-write race let a
// concurrent/failed reseed spawn duplicate SKUs that inflated the summary and
// fired false out-of-stock alerts. Runs under ENFORCED foreign keys to mirror
// production (the memory note: sqlite FKs were OFF and masked a real bug).
func TestEnsureInventoryIdempotentUniqueSKU(t *testing.T) {
	db := newFKEnforcedDemoDB(t)
	// Mirror prod: the partial unique index is created by SQL
	// migration, not GORM auto-migrate (which can't express the partial WHERE).
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_items_biz_sku ON inventory_items (business_id, sku) WHERE sku <> ''`).Error)
	admin := seedAdmin(t, db, "inv-idempotent@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 5})
	require.NoError(t, mustEnsure(svc, admin.ID))
	// Second full ensure must not duplicate any inventory row.
	require.NoError(t, mustEnsure(svc, admin.ID))

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Order("id ASC").Find(&businesses).Error)
	require.Len(t, businesses, 2)

	for _, business := range businesses {
		var total int64
		require.NoError(t, db.Model(&database.InventoryItem{}).
			Where("business_id = ?", business.ID).Count(&total).Error)
		require.Equalf(t, int64(5), total, "business %d must have exactly 5 inventory rows", business.ID)

		var distinct int64
		require.NoError(t, db.Model(&database.InventoryItem{}).
			Where("business_id = ?", business.ID).Distinct("sku").Count(&distinct).Error)
		require.Equalf(t, int64(5), distinct, "business %d must have 5 distinct SKUs", business.ID)
	}
}

// TestInventoryUpsertIsConcurrencySafe proves the PARTIAL unique index is the
// real dup backstop: many goroutines racing to insert the same (business_id,
// sku) can never create more than one row. Losers get a unique violation
// (tolerated); the DB — not app-level idempotency — guarantees the invariant.
func TestInventoryUpsertIsConcurrencySafe(t *testing.T) {
	db := newFKEnforcedDemoDB(t)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_items_biz_sku ON inventory_items (business_id, sku) WHERE sku <> ''`).Error)
	admin := seedAdmin(t, db, "inv-concurrent@example.com")
	require.NoError(t, db.Create(&database.Business{
		BusinessId: "concurrent-biz", OwnerAddress: "0xc", SettlementAddr: "0xc", TippingAddr: "0xc",
		Name: "Concurrent Biz", IsDemo: true, DemoOwnerUserID: &admin.ID,
	}).Error)
	var business database.Business
	require.NoError(t, db.Where("business_id = ?", "concurrent-biz").First(&business).Error)

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			create := database.InventoryItem{
				BusinessID: business.ID, Name: "Mixed Greens", SKU: "DEMO-GREENS",
				Unit: "kg", CurrentQuantity: 48, IsActive: true,
			}
			// Errors (unique violation / sqlite SQLITE_BUSY) are tolerated; the
			// invariant is that no duplicate row is ever created.
			_ = db.Create(&create).Error
		}()
	}
	wg.Wait()

	var count int64
	require.NoError(t, db.Model(&database.InventoryItem{}).
		Where("business_id = ? AND sku = ?", business.ID, "DEMO-GREENS").Count(&count).Error)
	require.Equal(t, int64(1), count, "concurrent upserts must yield exactly one row")
}

// TestAppendDueDaysSelfHealsFailedInstance is the showroom-freeze regression: a
// transient error flips an instance to `failed`, and before the fix the hourly
// AppendDueDays filtered strictly on `ready`, so nothing ever re-readied it —
// the demo froze until a backend restart. The cron must now recover it.
func TestAppendDueDaysSelfHealsFailedInstance(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "selfheal@example.com")

	now := fixedNow()
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})
	first, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	// Simulate a transient failure that froze the showroom.
	require.NoError(t, db.Model(&database.DemoInstance{}).
		Where("id = ?", first.ID).
		Updates(map[string]interface{}{"status": database.DemoInstanceStatusFailed, "last_error": "transient boom"}).Error)

	before := countAdminBills(t, db, admin.ID)
	now = now.AddDate(0, 0, 3)

	// The hourly tick must pick up the failed instance, re-ready it, and advance.
	require.NoError(t, svc.AppendDueDays(context.Background()))

	var recovered database.DemoInstance
	require.NoError(t, db.First(&recovered, first.ID).Error)
	require.Equal(t, database.DemoInstanceStatusReady, recovered.Status, "failed instance must be re-readied")
	require.Empty(t, recovered.LastError)
	require.Greater(t, countAdminBills(t, db, admin.ID), before, "recovered instance must append the due days it missed")
}

// TestEnsureInventoryHealsStaleBeefRecipeOntoSteak is #727: a leftover
// FirstOrCreate pairing left the beef SKU on demo-bowl while the stored name
// said the steak cut. Re-ensure must drop that drift so 86 targets the cuts
// off the media res (#945 widened that from the bife alone to every one).
func TestEnsureInventoryHealsStaleBeefRecipeOntoSteak(t *testing.T) {
	db := newFKEnforcedDemoDB(t)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_inventory_items_biz_sku ON inventory_items (business_id, sku) WHERE sku <> ''`).Error)
	admin := seedAdmin(t, db, "inv-heal-recipe@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 5})
	require.NoError(t, mustEnsure(svc, admin.ID))

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&businesses).Error)
	require.Len(t, businesses, 2)

	for _, business := range businesses {
		var beef database.InventoryItem
		require.NoError(t, db.Where("business_id = ? AND name = ?", business.ID, "Bife de chorizo (media res)").First(&beef).Error)
		require.NoError(t, db.Model(&database.InventoryRecipe{}).
			Where("business_id = ? AND inventory_item_id = ?", business.ID, beef.ID).
			Updates(map[string]interface{}{
				"menu_item_id":   "demo-bowl",
				"menu_item_name": "Bife de chorizo",
			}).Error)
	}

	require.NoError(t, mustEnsure(svc, admin.ID))

	for _, business := range businesses {
		var beef database.InventoryItem
		require.NoError(t, db.Where("business_id = ? AND name = ?", business.ID, "Bife de chorizo (media res)").First(&beef).Error)

		recipes := beefRecipesByMenuItem(t, db, business.ID, beef.ID)
		require.Lenf(t, recipes, len(grillCutsFromTheMediaRes),
			"business %d must keep exactly the media-res cuts", business.ID)
		require.NotContains(t, recipes, "demo-bowl", "drifted pairing must be dropped")
		require.Equal(t, "Bife de chorizo", recipes["demo-bife"].MenuItemName)
	}
}
