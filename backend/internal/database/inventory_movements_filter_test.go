package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupMovementsFilterDB spins up an isolated in-memory DB with the inventory
// tables and returns it. Each test gets its own DSN so parallel runs don't share
// rows through the shared-cache SQLite backend.
func setupMovementsFilterDB(t testing.TB) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db = gormDB
	require.NoError(t, db.AutoMigrate(
		&Business{},
		&InventoryItem{},
		&InventoryMovement{},
	))
	return gormDB
}

// seedMovements creates `count` movements for one item spaced one minute apart,
// oldest first, so ordering assertions are deterministic.
func seedMovements(t testing.TB, businessID, itemID uint, movementType string, count int) {
	t.Helper()
	base := time.Now().Add(-time.Duration(count) * time.Minute)
	for i := 0; i < count; i++ {
		require.NoError(t, db.Create(&InventoryMovement{
			BusinessID:      businessID,
			InventoryItemID: itemID,
			MovementType:    movementType,
			QuantityDelta:   1,
			QuantityBefore:  float64(i),
			QuantityAfter:   float64(i + 1),
			CreatedAt:       base.Add(time.Duration(i) * time.Minute),
		}).Error)
	}
}

// TestListInventoryMovementsFiltered_ItemAndTypeAndPaging is the RED-first
// regression for the movement-ledger fix: the query must support an optional
// item filter, an optional movement-type filter, offset paging, and return the
// total matching count so the UI can page truthfully instead of filtering 25
// global rows in the browser.
func TestListInventoryMovementsFiltered_ItemAndTypeAndPaging(t *testing.T) {
	setupMovementsFilterDB(t)

	business := &Business{Name: "Ledger Kitchen", OwnerAddress: "0xledger"}
	require.NoError(t, db.Create(business).Error)

	itemA := &InventoryItem{BusinessID: business.ID, Name: "Tomato", Unit: "kg", CostPerUnit: 1.25, IsActive: true}
	itemB := &InventoryItem{BusinessID: business.ID, Name: "Onion", Unit: "kg", CostPerUnit: 0.5, IsActive: true}
	require.NoError(t, db.Create(itemA).Error)
	require.NoError(t, db.Create(itemB).Error)

	// itemA: 30 purchase + 10 waste. itemB: 5 restock.
	seedMovements(t, business.ID, itemA.ID, InventoryMovementTypePurchase, 30)
	seedMovements(t, business.ID, itemA.ID, InventoryMovementTypeWaste, 10)
	seedMovements(t, business.ID, itemB.ID, InventoryMovementTypeRestock, 5)

	// No filter: total across the business is 45; first page bounded by limit.
	movements, total, err := ListInventoryMovementsFiltered(business.ID, InventoryMovementFilter{Limit: 20})
	require.NoError(t, err)
	assert.EqualValues(t, 45, total, "unfiltered total must count every business movement, not the returned page")
	assert.Len(t, movements, 20, "limit must bound the returned page")
	// Newest first.
	for i := 1; i < len(movements); i++ {
		assert.False(t, movements[i].CreatedAt.After(movements[i-1].CreatedAt),
			"movements must be ordered created_at DESC")
	}

	// Item filter: only itemA's 40 rows are counted/returned.
	movements, total, err = ListInventoryMovementsFiltered(business.ID, InventoryMovementFilter{ItemID: itemA.ID, Limit: 100})
	require.NoError(t, err)
	assert.EqualValues(t, 40, total, "item filter must scope the total")
	assert.Len(t, movements, 40)
	for _, m := range movements {
		assert.Equal(t, itemA.ID, m.InventoryItemID, "item filter must exclude other items")
	}

	// Item + type filter.
	movements, total, err = ListInventoryMovementsFiltered(business.ID, InventoryMovementFilter{
		ItemID:       itemA.ID,
		MovementType: InventoryMovementTypeWaste,
		Limit:        100,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 10, total, "item+type filter must scope the total")
	assert.Len(t, movements, 10)
	for _, m := range movements {
		assert.Equal(t, InventoryMovementTypeWaste, m.MovementType)
	}

	// Offset paging: page 2 of itemA at page size 15 returns rows 16-30.
	page1, total, err := ListInventoryMovementsFiltered(business.ID, InventoryMovementFilter{ItemID: itemA.ID, Limit: 15, Offset: 0})
	require.NoError(t, err)
	assert.EqualValues(t, 40, total)
	require.Len(t, page1, 15)
	page2, _, err := ListInventoryMovementsFiltered(business.ID, InventoryMovementFilter{ItemID: itemA.ID, Limit: 15, Offset: 15})
	require.NoError(t, err)
	require.Len(t, page2, 15)
	// Pages must not overlap.
	firstPageIDs := map[uint]bool{}
	for _, m := range page1 {
		firstPageIDs[m.ID] = true
	}
	for _, m := range page2 {
		assert.False(t, firstPageIDs[m.ID], "offset paging must not repeat rows across pages")
	}
}

// TestListInventoryMovementsFiltered_ProjectionAndBounds asserts the projection
// survives (FE renders name + cost_per_unit) and that the limit is clamped so a
// caller can't request the whole table.
func TestListInventoryMovementsFiltered_ProjectionAndBounds(t *testing.T) {
	setupMovementsFilterDB(t)

	business := &Business{Name: "Bounds Kitchen", OwnerAddress: "0xbounds"}
	require.NoError(t, db.Create(business).Error)
	item := &InventoryItem{BusinessID: business.ID, Name: "Basil", Unit: "bunch", CostPerUnit: 2.0, IsActive: true}
	require.NoError(t, db.Create(item).Error)
	seedMovements(t, business.ID, item.ID, InventoryMovementTypePurchase, 3)

	// Limit above the hard cap is clamped, not honored verbatim.
	movements, _, err := ListInventoryMovementsFiltered(business.ID, InventoryMovementFilter{Limit: 100000})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(movements), 200, "limit must be clamped to the hard cap")

	// Zero/negative limit falls back to a sane default.
	movements, _, err = ListInventoryMovementsFiltered(business.ID, InventoryMovementFilter{Limit: 0})
	require.NoError(t, err)
	require.Len(t, movements, 3)
	assert.Equal(t, item.Name, movements[0].InventoryItem.Name, "projection must still hydrate item name")
	assert.InDelta(t, item.CostPerUnit, movements[0].InventoryItem.CostPerUnit, 0.001, "projection must still hydrate cost_per_unit")
}

// setupMovementsBenchDB seeds a business with `items` items and `perItem`
// movements each (so the ledger has items*perItem rows), for the movement-list
// benchmarks. Ordered oldest-first per item.
func setupMovementsBenchDB(b *testing.B, items, perItem int) (*gorm.DB, uint, uint) {
	b.Helper()
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:mvbench-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		b.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		b.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = sqlDB.Close() })

	db = gormDB
	if err := db.AutoMigrate(&Business{}, &InventoryItem{}, &InventoryMovement{}); err != nil {
		b.Fatalf("auto-migrate: %v", err)
	}

	business := &Business{Name: "Movement Bench", OwnerAddress: fmt.Sprintf("0xmv%d", time.Now().UnixNano())}
	if err := db.Create(business).Error; err != nil {
		b.Fatalf("create business: %v", err)
	}

	itemIDs := make([]uint, items)
	for i := 0; i < items; i++ {
		it := &InventoryItem{BusinessID: business.ID, Name: fmt.Sprintf("Item %d", i), Unit: "kg", CostPerUnit: 1.0, IsActive: true}
		if err := db.Create(it).Error; err != nil {
			b.Fatalf("create item: %v", err)
		}
		itemIDs[i] = it.ID
	}

	base := time.Now().Add(-time.Duration(items*perItem) * time.Minute)
	batch := make([]InventoryMovement, 0, 500)
	seq := 0
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := db.CreateInBatches(&batch, 500).Error; err != nil {
			b.Fatalf("create movements: %v", err)
		}
		batch = batch[:0]
	}
	for _, id := range itemIDs {
		for j := 0; j < perItem; j++ {
			batch = append(batch, InventoryMovement{
				BusinessID:      business.ID,
				InventoryItemID: id,
				MovementType:    InventoryMovementTypePurchase,
				QuantityDelta:   1,
				CreatedAt:       base.Add(time.Duration(seq) * time.Minute),
			})
			seq++
			if len(batch) == 500 {
				flush()
			}
		}
	}
	flush()

	return gormDB, business.ID, itemIDs[0]
}

// BenchmarkListInventoryMovementsFiltered_Business measures the newest-page read
// over the whole business ledger (the Activity tab's default view).
func BenchmarkListInventoryMovementsFiltered_Business(b *testing.B) {
	_, businessID, _ := setupMovementsBenchDB(b, 200, 100) // 20k rows
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := ListInventoryMovementsFiltered(businessID, InventoryMovementFilter{Limit: 25}); err != nil {
			b.Fatalf("list: %v", err)
		}
	}
}

// BenchmarkListInventoryMovementsFiltered_Item measures the per-item history
// read (the item drawer's "Recent movements" load-more) over a large ledger —
// the query shape the optional composite index would serve.
func BenchmarkListInventoryMovementsFiltered_Item(b *testing.B) {
	_, businessID, itemID := setupMovementsBenchDB(b, 200, 100) // 20k rows, 100 per item
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := ListInventoryMovementsFiltered(businessID, InventoryMovementFilter{ItemID: itemID, Limit: 25}); err != nil {
			b.Fatalf("list: %v", err)
		}
	}
}
