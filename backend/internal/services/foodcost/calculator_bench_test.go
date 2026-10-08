package foodcost

import (
	"fmt"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// BenchmarkAnalyze captures the read-path baseline for a representative menu
// (100 items × 3 ingredients). Run with -benchmem; record ns/op, B/op,
// allocs/op in the perf notes. Off the order hot path, but the access shape
// (single batched item load) is asserted in the database package test.
func BenchmarkAnalyze(b *testing.B) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", b.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(b, err)
	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(b, gormDB.AutoMigrate(&database.Business{}, &database.InventoryItem{}, &database.InventoryRecipe{}))
	prev := database.GetDB()
	database.SetTestDB(gormDB)
	b.Cleanup(func() { database.SetTestDB(prev); sqlDB.Close() })
	db := database.GetDBWrapper()

	biz := database.Business{Name: "Bench Bistro"}
	require.NoError(b, gormDB.Create(&biz).Error)

	rows := make([]analytics.ItemStats, 0, 100)
	for i := 0; i < 100; i++ {
		menu := fmt.Sprintf("item-%d", i)
		for j := 0; j < 3; j++ {
			item := database.InventoryItem{BusinessID: biz.ID, Name: fmt.Sprintf("%s-ing%d", menu, j), Unit: "kg", CostPerUnit: 1.25}
			require.NoError(b, gormDB.Create(&item).Error)
			require.NoError(b, gormDB.Create(&database.InventoryRecipe{BusinessID: biz.ID, MenuItemID: menu, MenuItemName: menu, InventoryItemID: item.ID, QuantityRequired: 0.5}).Error)
		}
		rows = append(rows, analytics.ItemStats{ItemID: menu, ItemName: menu, TotalSold: 10, RecognizedQuantity: 10, Revenue: 100, AveragePrice: 10.0})
	}
	items := &stubItemStats{byPeriod: map[string][]analytics.ItemStats{"week": rows}}
	calc := NewCalculator(db, items)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := calc.Analyze(biz.ID, "week", nil); err != nil {
			b.Fatal(err)
		}
	}
}
