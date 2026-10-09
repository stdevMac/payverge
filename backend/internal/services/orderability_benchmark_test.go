package services

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var orderabilityProjectionDBSequence atomic.Uint64

func setupOrderabilityProjectionDB(tb testing.TB, itemCount int) (*database.Business, []database.MenuCategory, *atomic.Int64) {
	tb.Helper()
	return setupOrderabilityProjectionDBWithInventory(tb, itemCount, true, nil)
}

// setupOrderabilityProjectionDBWithInventory seeds the projection fixture and,
// when tables is non-nil, records the FROM table of every query run after
// seeding so access-shape tests can assert which tables a projection touched.
func setupOrderabilityProjectionDBWithInventory(tb testing.TB, itemCount int, inventoryEnabled bool, tables *[]string) (*database.Business, []database.MenuCategory, *atomic.Int64) {
	tb.Helper()
	sequence := orderabilityProjectionDBSequence.Add(1)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:orderability-%s-%d-%d?mode=memory&cache=shared", tb.Name(), itemCount, sequence)), &gorm.Config{})
	require.NoError(tb, err)
	require.NoError(tb, db.AutoMigrate(&database.Business{}, &database.Menu{}, &database.InventorySettings{}, &database.InventoryItem{}, &database.InventoryRecipe{}))
	database.SetTestDB(db)
	business := &database.Business{BusinessId: fmt.Sprintf("orderability-%d-%d", itemCount, sequence), Name: "Projection", OwnerAddress: "0xowner", KitchenEnabled: true, OrdersEnabled: true}
	require.NoError(tb, db.Create(business).Error)
	require.NoError(tb, db.Create(&database.InventorySettings{BusinessID: business.ID, InventoryEnabled: inventoryEnabled, AvailabilitySyncMode: database.InventoryAvailabilityModeHardBlock}).Error)

	items := make([]database.InventoryItem, itemCount)
	categories := []database.MenuCategory{{ID: "mains", Name: "Mains", Items: make([]database.MenuItem, itemCount)}}
	for i := 0; i < itemCount; i++ {
		items[i] = database.InventoryItem{BusinessID: business.ID, Name: fmt.Sprintf("Ingredient %d", i), SKU: fmt.Sprintf("SKU-%d", i), Unit: "unit", CurrentQuantity: 100, IsActive: true}
		categories[0].Items[i] = database.MenuItem{ID: fmt.Sprintf("item-%d", i), Name: fmt.Sprintf("Item %d", i), Price: 10, IsAvailable: true}
	}
	require.NoError(tb, db.CreateInBatches(&items, 100).Error)
	recipes := make([]database.InventoryRecipe, itemCount)
	for i := range recipes {
		recipes[i] = database.InventoryRecipe{BusinessID: business.ID, MenuItemID: categories[0].Items[i].ID, InventoryItemID: items[i].ID, QuantityRequired: 1}
	}
	require.NoError(tb, db.CreateInBatches(&recipes, 100).Error)
	raw, err := json.Marshal(categories)
	require.NoError(tb, err)
	require.NoError(tb, db.Create(&database.Menu{BusinessID: business.ID, Categories: string(raw), IsActive: true}).Error)

	count := &atomic.Int64{}
	require.NoError(tb, db.Callback().Query().Before("gorm:query").Register(fmt.Sprintf("count_orderability_%d_%d", itemCount, sequence), func(q *gorm.DB) {
		count.Add(1)
		if tables != nil {
			*tables = append(*tables, q.Statement.Table)
		}
	}))
	return business, categories, count
}

func TestProjectOrderabilityQueryCountIsConstant(t *testing.T) {
	_, _, one := setupOrderabilityProjectionDB(t, 1)
	ProjectOrderability(&database.Business{ID: 1, KitchenEnabled: true, OrdersEnabled: true}, []database.MenuCategory{}, OrderabilityContextGuest, true)
	oneQueries := one.Load()

	business, categories, many := setupOrderabilityProjectionDB(t, 500)
	projection := ProjectOrderability(business, categories, OrderabilityContextGuest, true)
	require.Len(t, projection, 500)
	require.Equal(t, oneQueries, many.Load(), "database query count must not grow with menu item count")
}

func BenchmarkOrderabilityProjection(b *testing.B) {
	business, categories, _ := setupOrderabilityProjectionDB(b, 500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		projection := ProjectOrderability(business, categories, OrderabilityContextGuest, true)
		if len(projection) != 500 {
			b.Fatalf("projection size = %d", len(projection))
		}
	}
}

// Inventory disabled is the common case for guest menu/table polls. The
// projection must then read only the settings row: no inventory_items,
// inventory_recipes or menu reload behind a summary nobody consults.
func TestProjectOrderabilityInventoryDisabledSkipsInventoryReads(t *testing.T) {
	var tables []string
	business, categories, count := setupOrderabilityProjectionDBWithInventory(t, 50, false, &tables)
	projection := ProjectOrderability(business, categories, OrderabilityContextGuest, true)
	require.Len(t, projection, 50)
	for id, decision := range projection {
		require.Truef(t, decision.Orderable, "item %s must stay orderable with inventory disabled", id)
		require.Equal(t, OrderabilityAvailable, decision.State)
	}
	for _, table := range tables {
		require.NotContains(t, []string{"inventory_items", "inventory_recipes", "menus"}, table, "inventory disabled must not read %s", table)
	}
	require.Equal(t, int64(1), count.Load(), "inventory disabled must run exactly one settings read, got tables %v", tables)
}

func BenchmarkOrderabilityProjectionInventoryDisabled(b *testing.B) {
	business, categories, _ := setupOrderabilityProjectionDBWithInventory(b, 500, false, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		projection := ProjectOrderability(business, categories, OrderabilityContextGuest, true)
		if len(projection) != 500 {
			b.Fatalf("projection size = %d", len(projection))
		}
	}
}
