package database

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupOrdersDeliveryMetaDB is a TB-compatible variant of setupOrderTestDB for
// use in both *testing.T and *testing.B contexts.
func setupOrdersDeliveryMetaDB(tb testing.TB) {
	tb.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(tb, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(tb, err)
	sqlDB.SetMaxOpenConns(1)

	db = gormDB

	require.NoError(tb, db.AutoMigrate(
		&Business{},
		&InventorySettings{},
		&InventoryItem{},
		&InventoryRecipe{},
		&InventoryMovement{},
		&Menu{},
		&Table{},
		&Bill{},
		&BillHistoryEvent{},
		&Order{},
		&DeliveryOrder{},
		&DeliveryStatusHistory{},
	))

	db.Exec("DROP TABLE IF EXISTS bill_items")
	require.NoError(tb, db.Exec(`
		CREATE TABLE bill_items (
			id TEXT PRIMARY KEY,
			bill_id INTEGER NOT NULL,
			menu_item_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			price REAL NOT NULL,
			quantity INTEGER NOT NULL,
			options TEXT,
			item_type TEXT DEFAULT 'menu_item',
			bundle_id INTEGER,
			parent_bundle_id INTEGER,
			source_offer_id INTEGER,
			order_id INTEGER,
			subtotal REAL NOT NULL,
			created_at DATETIME
		)
	`).Error)
}

func TestOrdersList_AttachesDeliveryMeta(t *testing.T) {
	setupOrdersDeliveryMetaDB(t)

	business := Business{Name: "T", IsActive: true}
	if err := db.Create(&business).Error; err != nil {
		t.Fatal(err)
	}

	makeOrder := func(i int, withDelivery bool) Order {
		bill := Bill{BusinessID: business.ID, BillNumber: fmt.Sprintf("B-M-%d", i), Status: BillStatusOpen}
		if err := db.Omit("table_id").Create(&bill).Error; err != nil {
			t.Fatal(err)
		}
		order := Order{BillID: bill.ID, BusinessID: business.ID, OrderNumber: fmt.Sprintf("O-M-%d", i), Status: OrderStatusPending, Items: "[]"}
		if withDelivery {
			order.CreatedBy = "guest_delivery"
		}
		if err := db.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		if withDelivery {
			d := DeliveryOrder{
				BusinessID: business.ID, BillID: bill.ID, OrderID: &order.ID,
				DeliveryNumber: fmt.Sprintf("DEL-M%d", i), DeliveryType: DeliveryTypeInHouse,
				Status: DeliveryStatusPending, CustomerName: "G", CustomerPhone: "1",
				DeliveryAddress: DeliveryAddress{Street: "Calle 1", City: "BA"},
			}
			if err := db.Create(&d).Error; err != nil {
				t.Fatal(err)
			}
		}
		return order
	}

	makeOrder(1, true)
	makeOrder(2, true)
	dineIn := makeOrder(3, false)

	result, err := GetOrdersByBusinessIDPaginated(business.ID, "pending", PaginationParams{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}

	deliveryCount := 0
	for _, o := range result.Data {
		if o.ID == dineIn.ID {
			if o.Delivery != nil {
				t.Fatal("dine-in order must not carry delivery meta")
			}
			continue
		}
		if o.Delivery == nil {
			t.Fatalf("delivery order %d missing meta", o.ID)
		}
		if o.Delivery.Street != "Calle 1" || o.Delivery.DeliveryNumber == "" {
			t.Fatalf("bad meta: %+v", o.Delivery)
		}
		deliveryCount++
	}
	if deliveryCount != 2 {
		t.Fatalf("expected 2 delivery-linked orders, got %d", deliveryCount)
	}
}

// TestKitchenQueueExcludesCancelledDeliveries verifies that orders whose linked
// delivery is in a terminal dead state (cancelled/failed — e.g. swept by the
// unpaid-delivery expiry job) drop out of the approval/kitchen queue, while
// alive delivery orders and dine-in orders (no delivery_orders row) remain. This
// closes audit M7: a dead order sat 28h in "Requiere aprobación".
func TestKitchenQueueExcludesCancelledDeliveries(t *testing.T) {
	setupOrdersDeliveryMetaDB(t)

	business := Business{Name: "T", IsActive: true}
	if err := db.Create(&business).Error; err != nil {
		t.Fatal(err)
	}

	makeDeliveryOrder := func(i int, status DeliveryStatus) Order {
		bill := Bill{BusinessID: business.ID, BillNumber: fmt.Sprintf("B-K-%d", i), Status: BillStatusOpen}
		if err := db.Omit("table_id").Create(&bill).Error; err != nil {
			t.Fatal(err)
		}
		order := Order{BillID: bill.ID, BusinessID: business.ID, OrderNumber: fmt.Sprintf("O-K-%d", i), Status: OrderStatusPending, Items: "[]", CreatedBy: "guest_delivery"}
		if err := db.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		d := DeliveryOrder{
			BusinessID: business.ID, BillID: bill.ID, OrderID: &order.ID,
			DeliveryNumber: fmt.Sprintf("DEL-K%d", i), DeliveryType: DeliveryTypeInHouse,
			Status: status, CustomerName: "G", CustomerPhone: "1",
			DeliveryAddress: DeliveryAddress{Street: "Calle 1", City: "BA"},
		}
		if err := db.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
		return order
	}

	makeDineIn := func(i int) Order {
		bill := Bill{BusinessID: business.ID, BillNumber: fmt.Sprintf("B-KD-%d", i), Status: BillStatusOpen}
		if err := db.Omit("table_id").Create(&bill).Error; err != nil {
			t.Fatal(err)
		}
		order := Order{BillID: bill.ID, BusinessID: business.ID, OrderNumber: fmt.Sprintf("O-KD-%d", i), Status: OrderStatusPending, Items: "[]"}
		if err := db.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		return order
	}

	alive := makeDeliveryOrder(1, DeliveryStatusConfirmed)
	cancelled := makeDeliveryOrder(2, DeliveryStatusCancelled)
	failed := makeDeliveryOrder(3, DeliveryStatusFailed)
	dineIn := makeDineIn(4)

	result, err := GetOrdersByBusinessIDPaginated(business.ID, "pending", PaginationParams{Page: 1, PageSize: 50})
	require.NoError(t, err)

	ids := make(map[uint]struct{}, len(result.Data))
	for _, o := range result.Data {
		ids[o.ID] = struct{}{}
	}

	require.Contains(t, ids, alive.ID, "confirmed delivery order must stay in queue")
	require.Contains(t, ids, dineIn.ID, "dine-in order (no delivery row) must stay in queue")
	require.NotContains(t, ids, cancelled.ID, "cancelled delivery order must be excluded")
	require.NotContains(t, ids, failed.ID, "failed delivery order must be excluded")
	require.Equal(t, int64(2), result.Total, "total count must exclude dead delivery orders")
}

func BenchmarkGetOrdersByBusinessIDPaginated(b *testing.B) {
	setupOrdersDeliveryMetaDB(b)

	business := Business{Name: "BenchBiz", IsActive: true}
	if err := db.Create(&business).Error; err != nil {
		b.Fatal(err)
	}

	// Seed 200 orders, half delivery-linked; a slice of those are terminal
	// (cancelled/failed) so the NOT EXISTS exclusion has real work to do.
	for i := 0; i < 200; i++ {
		bill := Bill{BusinessID: business.ID, BillNumber: fmt.Sprintf("BENCH-B-%d", i), Status: BillStatusOpen}
		if err := db.Omit("table_id").Create(&bill).Error; err != nil {
			b.Fatal(err)
		}
		order := Order{
			BillID: bill.ID, BusinessID: business.ID,
			OrderNumber: fmt.Sprintf("BENCH-O-%d", i), Status: OrderStatusPending, Items: "[]",
		}
		if i%2 == 0 {
			order.CreatedBy = "guest_delivery"
		}
		if err := db.Create(&order).Error; err != nil {
			b.Fatal(err)
		}
		if i%2 == 0 {
			status := DeliveryStatusPending
			switch i % 6 {
			case 0:
				status = DeliveryStatusCancelled
			case 2:
				status = DeliveryStatusFailed
			}
			d := DeliveryOrder{
				BusinessID: business.ID, BillID: bill.ID, OrderID: &order.ID,
				DeliveryNumber: fmt.Sprintf("BENCH-DEL-%d", i), DeliveryType: DeliveryTypeInHouse,
				Status: status, CustomerName: "Guest", CustomerPhone: "5550000",
				DeliveryAddress: DeliveryAddress{Street: "Av. Libertador", City: "CABA"},
			}
			if err := db.Create(&d).Error; err != nil {
				b.Fatal(err)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetOrdersByBusinessIDPaginated(business.ID, "pending", PaginationParams{Page: 1, PageSize: 50})
		if err != nil {
			b.Fatal(err)
		}
	}
}
