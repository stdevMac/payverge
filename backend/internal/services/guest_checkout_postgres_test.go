package services

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGuestCheckoutConcurrentReplayPostgres exercises the production locking
// and partial-unique-index semantics that SQLite cannot model faithfully.
func TestGuestCheckoutConcurrentReplayPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short")
	}
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	previous := database.GetDB()
	database.SetTestDB(pg.DB)
	t.Cleanup(func() { database.SetTestDB(previous) })
	require.NoError(t, pg.DB.AutoMigrate(
		&database.Business{}, &database.Table{}, &database.Bill{},
		&database.Order{}, &database.Menu{}, &database.Offer{}, &database.Bundle{},
		&database.InventorySettings{}, &database.InventoryItem{}, &database.InventoryRecipe{},
		&database.DeliveryOrder{}, &database.BillHistoryEvent{}, &database.BusinessOperatingHours{},
	))
	require.NoError(t, pg.DB.AutoMigrate(&database.BillItem{}))
	require.NoError(t, pg.DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_guest_request_identity
		ON orders (business_id, created_by, client_request_id)
		WHERE created_by = 'guest' AND client_request_id IS NOT NULL`).Error)

	business := database.Business{
		BusinessId: "checkout-pg", Name: "Checkout PG", OwnerAddress: "0xowner",
		KitchenEnabled: true, OrdersEnabled: true, DefaultCurrency: "USD", Timezone: "UTC",
	}
	require.NoError(t, pg.DB.Create(&business).Error)
	table := database.Table{BusinessID: business.ID, Name: "1", TableCode: "TABLE-PG", Capacity: 4, IsActive: true}
	require.NoError(t, pg.DB.Create(&table).Error)
	categories := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{{ID: "burger", Name: "Burger", Price: 9.99, IsAvailable: true}},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, pg.DB.Create(&database.Menu{BusinessID: business.ID, Categories: string(raw), IsActive: true}).Error)
	require.NoError(t, pg.DB.Create(&database.InventorySettings{BusinessID: business.ID, InventoryEnabled: false}).Error)

	service := NewGuestCheckoutService(pg.DB)
	const racers = 8
	start := make(chan struct{})
	results := make([]GuestCheckoutResult, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index], errs[index] = service.Checkout(ctx, checkoutInput(table.TableCode, "same-request"))
		}(i)
	}
	close(start)
	wg.Wait()

	var firstExecutions int
	var billID, orderID uint
	for i := range errs {
		require.NoError(t, errs[i])
		if !results[i].Replay {
			firstExecutions++
		}
		if billID == 0 {
			billID, orderID = results[i].Bill.ID, results[i].Order.ID
		}
		assert.Equal(t, billID, results[i].Bill.ID)
		assert.Equal(t, orderID, results[i].Order.ID)
	}
	assert.Equal(t, 1, firstExecutions)

	var bills, orders, billItems int64
	require.NoError(t, pg.DB.Model(&database.Bill{}).Where("business_id = ?", business.ID).Count(&bills).Error)
	require.NoError(t, pg.DB.Model(&database.Order{}).Where("business_id = ?", business.ID).Count(&orders).Error)
	require.NoError(t, pg.DB.Model(&database.BillItem{}).Where("bill_id = ?", billID).Count(&billItems).Error)
	assert.EqualValues(t, 1, bills)
	assert.EqualValues(t, 1, orders)
	assert.EqualValues(t, 1, billItems)

	// A fresh request without a cached bill ID must attach to the table's
	// existing active bill instead of creating a second active aggregate.
	attached, err := service.Checkout(ctx, checkoutInput(table.TableCode, "second-request"))
	require.NoError(t, err)
	assert.Equal(t, billID, attached.Bill.ID)
	require.NoError(t, pg.DB.Model(&database.Bill{}).Where("business_id = ?", business.ID).Count(&bills).Error)
	assert.EqualValues(t, 1, bills)

	// Force the production Postgres order insert to fail and prove the bill
	// mutation/items are rolled back with it.
	require.NoError(t, pg.DB.Exec(`ALTER TABLE orders ADD CONSTRAINT reject_forced_checkout
		CHECK (client_request_id IS NULL OR client_request_id <> 'force-fail')`).Error)
	failed := checkoutInput(table.TableCode, "force-fail")
	failed.BillID = &billID
	_, err = service.Checkout(ctx, failed)
	require.Error(t, err)
	require.NoError(t, pg.DB.Model(&database.Order{}).Where("business_id = ?", business.ID).Count(&orders).Error)
	require.NoError(t, pg.DB.Model(&database.BillItem{}).Where("bill_id = ?", billID).Count(&billItems).Error)
	assert.EqualValues(t, 2, orders)
	assert.EqualValues(t, 2, billItems)
}

// TestGuestCheckoutConcurrentCrossTableRequestPostgres forces the unique-index
// recovery branch: each checkout locks a different table, both miss the replay
// lookup, and one loses the business/request-identity insert race. The loser
// must receive a stable conflict, never the winner's bill capability.
func TestGuestCheckoutConcurrentCrossTableRequestPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short")
	}
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	previous := database.GetDB()
	database.SetTestDB(pg.DB)
	t.Cleanup(func() { database.SetTestDB(previous) })
	require.NoError(t, pg.DB.AutoMigrate(
		&database.Business{}, &database.Table{}, &database.Bill{},
		&database.Order{}, &database.Menu{}, &database.Offer{}, &database.Bundle{},
		&database.InventorySettings{}, &database.InventoryItem{}, &database.InventoryRecipe{},
		&database.DeliveryOrder{}, &database.BillHistoryEvent{}, &database.BusinessOperatingHours{},
	))
	require.NoError(t, pg.DB.AutoMigrate(&database.BillItem{}))
	require.NoError(t, pg.DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_guest_request_identity
		ON orders (business_id, created_by, client_request_id)
		WHERE created_by = 'guest' AND client_request_id IS NOT NULL`).Error)

	business := database.Business{
		BusinessId: "checkout-cross-table-pg", Name: "Checkout cross table PG", OwnerAddress: "0xowner",
		KitchenEnabled: true, OrdersEnabled: true, DefaultCurrency: "USD", Timezone: "UTC",
	}
	require.NoError(t, pg.DB.Create(&business).Error)
	tables := []database.Table{
		{BusinessID: business.ID, Name: "1", TableCode: "TABLE-PG-A", Capacity: 4, IsActive: true},
		{BusinessID: business.ID, Name: "2", TableCode: "TABLE-PG-B", Capacity: 4, IsActive: true},
	}
	for i := range tables {
		require.NoError(t, pg.DB.Create(&tables[i]).Error)
	}
	categories := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{{ID: "burger", Name: "Burger", Price: 9.99, IsAvailable: true}},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, pg.DB.Create(&database.Menu{BusinessID: business.ID, Categories: string(raw), IsActive: true}).Error)
	require.NoError(t, pg.DB.Create(&database.InventorySettings{BusinessID: business.ID, InventoryEnabled: false}).Error)

	service := NewGuestCheckoutService(pg.DB)
	start := make(chan struct{})
	results := make([]GuestCheckoutResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range tables {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index], errs[index] = service.Checkout(ctx, checkoutInput(tables[index].TableCode, "same-cross-table-request"))
		}(i)
	}
	close(start)
	wg.Wait()

	var successes, conflicts int
	for i := range errs {
		if errs[i] == nil {
			successes++
			assert.NotZero(t, results[i].Bill.ID)
			assert.Equal(t, tables[i].ID, results[i].Bill.TableID)
			continue
		}
		var conflict *GuestCheckoutReplayConflictError
		require.ErrorAs(t, errs[i], &conflict)
		conflicts++
		assert.Zero(t, results[i].Bill.ID)
		assert.Zero(t, results[i].Order.ID)
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
}
