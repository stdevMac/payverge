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
	"gorm.io/gorm/logger"
)

// newWasteVarianceTestDB opens an isolated in-memory SQLite DB, migrates the
// tables used by waste-variance readers, registers it as the package DB, and
// returns the wrapper.
func newWasteVarianceTestDB(t *testing.T) (*DB, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &InventoryItem{}, &InventoryRecipe{}, &InventoryMovement{}))
	SetTestDB(gormDB)
	return GetDBWrapper(), gormDB
}

// TestAggregateInventoryMovements_SumsAndExcludes verifies that the reader:
//   - sums quantity_delta per (inventory_item_id, movement_type) bucket
//   - returns correct totals for each bucket inside the time window
//   - excludes movements whose created_at falls before the window start
func TestAggregateInventoryMovements_SumsAndExcludes(t *testing.T) {
	db, gormDB := newWasteVarianceTestDB(t)

	biz := Business{Name: "Waste Test Bistro"}
	require.NoError(t, gormDB.Create(&biz).Error)

	item := InventoryItem{BusinessID: biz.ID, Name: "Flour", Unit: "kg", CostPerUnit: 1.5}
	require.NoError(t, gormDB.Create(&item).Error)

	now := time.Now().UTC()
	windowStart := now.Add(-1 * time.Hour)
	windowEnd := now.Add(1 * time.Hour)

	// Two order_consumption rows inside the window: -2 and -3 => total -5
	require.NoError(t, gormDB.Create(&InventoryMovement{
		BusinessID:      biz.ID,
		InventoryItemID: item.ID,
		MovementType:    InventoryMovementTypeOrderConsumption,
		QuantityDelta:   -2,
		QuantityBefore:  10,
		QuantityAfter:   8,
		CreatedAt:       now.Add(-30 * time.Minute),
	}).Error)
	require.NoError(t, gormDB.Create(&InventoryMovement{
		BusinessID:      biz.ID,
		InventoryItemID: item.ID,
		MovementType:    InventoryMovementTypeOrderConsumption,
		QuantityDelta:   -3,
		QuantityBefore:  8,
		QuantityAfter:   5,
		CreatedAt:       now.Add(-15 * time.Minute),
	}).Error)

	// One waste row inside the window: -1
	require.NoError(t, gormDB.Create(&InventoryMovement{
		BusinessID:      biz.ID,
		InventoryItemID: item.ID,
		MovementType:    InventoryMovementTypeWaste,
		QuantityDelta:   -1,
		QuantityBefore:  5,
		QuantityAfter:   4,
		CreatedAt:       now.Add(-20 * time.Minute),
	}).Error)

	// One purchase row inside the window: +10
	require.NoError(t, gormDB.Create(&InventoryMovement{
		BusinessID:      biz.ID,
		InventoryItemID: item.ID,
		MovementType:    InventoryMovementTypePurchase,
		QuantityDelta:   10,
		QuantityBefore:  4,
		QuantityAfter:   14,
		CreatedAt:       now.Add(-10 * time.Minute),
	}).Error)

	// One order_consumption row BEFORE the window (must be excluded): -99
	require.NoError(t, gormDB.Create(&InventoryMovement{
		BusinessID:      biz.ID,
		InventoryItemID: item.ID,
		MovementType:    InventoryMovementTypeOrderConsumption,
		QuantityDelta:   -99,
		QuantityBefore:  100,
		QuantityAfter:   1,
		CreatedAt:       now.Add(-2 * time.Hour), // before windowStart
	}).Error)

	rows, err := db.AggregateInventoryMovements(biz.ID, windowStart, windowEnd)
	require.NoError(t, err)
	require.Len(t, rows, 3, "expect 3 buckets: order_consumption, waste, purchase")

	byType := make(map[string]float64, len(rows))
	for _, r := range rows {
		byType[r.MovementType] = r.TotalDelta
	}

	assert.InDelta(t, -5.0, byType[InventoryMovementTypeOrderConsumption], 0.001,
		"order_consumption total should be -2 + -3 = -5 (excludes -99 before window)")
	assert.InDelta(t, -1.0, byType[InventoryMovementTypeWaste], 0.001,
		"waste total should be -1")
	assert.InDelta(t, 10.0, byType[InventoryMovementTypePurchase], 0.001,
		"purchase total should be +10")

	// Verify the excluded -99 movement is not counted
	assert.NotContains(t, []float64{byType[InventoryMovementTypeOrderConsumption]}, -104.0,
		"the -99 movement before the window must not appear in the totals")
}

// TestGetInventoryItemDimsForBusiness_IncludesInactive_Isolated verifies that:
//   - id/name/unit/cost_per_unit are populated
//   - INACTIVE items are INCLUDED (so deactivated items that still accrued waste
//     in the window get valued — otherwise tracked loss is understated)
//   - cross-business isolation holds (only queried business's items)
func TestGetInventoryItemDimsForBusiness_IncludesInactive_Isolated(t *testing.T) {
	db, gormDB := newWasteVarianceTestDB(t)

	bizA := Business{Name: "Alpha Kitchen", BusinessId: "biz-dim-a"}
	bizB := Business{Name: "Beta Kitchen", BusinessId: "biz-dim-b"}
	require.NoError(t, gormDB.Create(&bizA).Error)
	require.NoError(t, gormDB.Create(&bizB).Error)

	// bizA: one active, one inactive.
	// IsActive=false is the zero value for bool; GORM skips it on Create when
	// the column has `default:true`, so the DB would silently keep the default.
	// Create both items as active, then explicitly deactivate the second one
	// using UpdateColumn (which bypasses GORM's zero-value-omit logic).
	activeItem := InventoryItem{
		BusinessID:  bizA.ID,
		Name:        "Olive Oil",
		Unit:        "L",
		CostPerUnit: 5.50,
		IsActive:    true,
	}
	inactiveItem := InventoryItem{
		BusinessID:  bizA.ID,
		Name:        "Old Spice",
		Unit:        "g",
		CostPerUnit: 2.00,
		IsActive:    true, // create active, then flip below
	}
	require.NoError(t, gormDB.Create(&activeItem).Error)
	require.NoError(t, gormDB.Create(&inactiveItem).Error)
	// Explicitly set is_active=false — UpdateColumn bypasses zero-value omission.
	require.NoError(t, gormDB.Model(&inactiveItem).UpdateColumn("is_active", false).Error)

	// bizB: one active (must not appear in bizA query)
	otherItem := InventoryItem{
		BusinessID:  bizB.ID,
		Name:        "Sugar",
		Unit:        "kg",
		CostPerUnit: 1.20,
		IsActive:    true,
	}
	require.NoError(t, gormDB.Create(&otherItem).Error)

	dims, err := db.GetInventoryItemDimsForBusiness(bizA.ID)
	require.NoError(t, err)
	require.Len(t, dims, 2, "both the active AND inactive items of bizA must be returned")

	byID := map[uint]InventoryItemDim{}
	for _, d := range dims {
		byID[d.ID] = d
	}
	// Active item populated.
	require.Contains(t, byID, activeItem.ID)
	assert.Equal(t, "Olive Oil", byID[activeItem.ID].Name)
	assert.Equal(t, "L", byID[activeItem.ID].Unit)
	assert.InDelta(t, 5.50, byID[activeItem.ID].CostPerUnit, 0.001)

	// Inactive item INCLUDED with its cost (so its tracked loss can be valued).
	require.Contains(t, byID, inactiveItem.ID, "deactivated item must still be valued")
	assert.InDelta(t, 2.00, byID[inactiveItem.ID].CostPerUnit, 0.001)

	// Cross-business isolation.
	for _, dim := range dims {
		assert.NotEqual(t, otherItem.ID, dim.ID, "bizB item must not appear in bizA query")
	}
}

// TestAggregateInventoryMovements_SingleQuery asserts that AggregateInventoryMovements
// issues exactly ONE SQL query that touches inventory_movements — no N+1.
func TestAggregateInventoryMovements_SingleQuery(t *testing.T) {
	db, gormDB := newWasteVarianceTestDB(t)

	biz := Business{Name: "Throughput Bistro"}
	require.NoError(t, gormDB.Create(&biz).Error)

	now := time.Now().UTC()
	windowStart := now.Add(-1 * time.Hour)
	windowEnd := now.Add(1 * time.Hour)

	// Seed 3 items with 2 movements each across 3 types — 9 rows total.
	types := []string{
		InventoryMovementTypeOrderConsumption,
		InventoryMovementTypeWaste,
		InventoryMovementTypePurchase,
	}
	for i := 0; i < 3; i++ {
		item := InventoryItem{
			BusinessID:  biz.ID,
			Name:        fmt.Sprintf("Ingredient%d", i),
			Unit:        "kg",
			CostPerUnit: float64(i+1) * 1.5,
		}
		require.NoError(t, gormDB.Create(&item).Error)
		mt := types[i%len(types)]
		for j := 0; j < 2; j++ {
			require.NoError(t, gormDB.Create(&InventoryMovement{
				BusinessID:      biz.ID,
				InventoryItemID: item.ID,
				MovementType:    mt,
				QuantityDelta:   float64(-(j + 1)),
				QuantityBefore:  float64(10 - j),
				QuantityAfter:   float64(9 - j),
				CreatedAt:       now.Add(time.Duration(-10*(i+j+1)) * time.Minute),
			}).Error)
		}
	}

	// NOTE: Scan in GORM v1.30.0 calls Rows() internally, which uses the Row
	// callback chain (gorm:row / RowQuery), NOT the Query chain (gorm:query).
	// Registering After("gorm:row") is the correct hook for Scan-based queries.
	cbName := "count_movement_queries_" + t.Name()
	var movementQueries int
	require.NoError(t, gormDB.Callback().Row().After("gorm:row").
		Register(cbName, func(tx *gorm.DB) {
			if strings.Contains(strings.ToLower(tx.Statement.SQL.String()), "inventory_movements") {
				movementQueries++
			}
		}))
	t.Cleanup(func() { _ = gormDB.Callback().Row().Remove(cbName) })

	rows, err := db.AggregateInventoryMovements(biz.ID, windowStart, windowEnd)
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	require.Equal(t, 1, movementQueries,
		"AggregateInventoryMovements must issue exactly ONE query touching inventory_movements, not one per item/type")
}
