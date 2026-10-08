package database

import (
	"encoding/json"
	"fmt"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupInventoryDeductionBenchDB(b *testing.B) (*gorm.DB, *Business, []Order) {
	b.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file:inventory-deduction-bench?mode=memory&cache=shared"), &gorm.Config{})
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
	if err := db.AutoMigrate(
		&Business{},
		&InventorySettings{},
		&InventoryItem{},
		&InventoryRecipe{},
		&InventoryMovement{},
		&Menu{},
		&Bill{},
		&Order{},
	); err != nil {
		b.Fatalf("auto-migrate: %v", err)
	}

	business := &Business{Name: "Inventory Bench"}
	if err := db.Create(business).Error; err != nil {
		b.Fatalf("create business: %v", err)
	}
	if err := db.Create(&InventorySettings{
		BusinessID:                business.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		LowStockWarningsEnabled:   false,
		AvailabilitySyncMode:      InventoryAvailabilityModeWarn,
	}).Error; err != nil {
		b.Fatalf("create settings: %v", err)
	}

	menuCategories := []MenuCategory{{
		ID:   "bench-cat",
		Name: "Bench",
		Items: []MenuItem{{
			ID:          "bench-burger",
			Name:        "Bench Burger",
			Price:       12,
			IsAvailable: true,
		}},
	}}
	rawMenu, err := json.Marshal(menuCategories)
	if err != nil {
		b.Fatalf("marshal menu: %v", err)
	}
	if err := db.Create(&Menu{BusinessID: business.ID, Categories: string(rawMenu), IsActive: true}).Error; err != nil {
		b.Fatalf("create menu: %v", err)
	}

	stock := float64(b.N*2 + 100)
	bun := InventoryItem{BusinessID: business.ID, Name: "Bun", Unit: "pcs", CurrentQuantity: stock, IsActive: true}
	patty := InventoryItem{BusinessID: business.ID, Name: "Patty", Unit: "pcs", CurrentQuantity: stock, IsActive: true}
	if err := db.Create(&bun).Error; err != nil {
		b.Fatalf("create bun: %v", err)
	}
	if err := db.Create(&patty).Error; err != nil {
		b.Fatalf("create patty: %v", err)
	}
	if err := db.Create([]InventoryRecipe{
		{BusinessID: business.ID, MenuItemID: "bench-burger", MenuItemName: "Bench Burger", InventoryItemID: bun.ID, QuantityRequired: 1},
		{BusinessID: business.ID, MenuItemID: "bench-burger", MenuItemName: "Bench Burger", InventoryItemID: patty.ID, QuantityRequired: 1},
	}).Error; err != nil {
		b.Fatalf("create recipes: %v", err)
	}

	bills := make([]Bill, b.N)
	for i := range bills {
		bills[i] = Bill{
			BusinessID:  business.ID,
			BillNumber:  fmt.Sprintf("INV-BENCH-BILL-%06d", i),
			Status:      BillStatusOpen,
			Items:       "[]",
			TotalAmount: 1200,
		}
	}
	if err := db.CreateInBatches(&bills, 500).Error; err != nil {
		b.Fatalf("create bills: %v", err)
	}

	orders := make([]Order, b.N)
	for i := range orders {
		orders[i] = Order{
			BusinessID:  business.ID,
			BillID:      bills[i].ID,
			OrderNumber: fmt.Sprintf("INV-BENCH-ORDER-%06d", i),
			Status:      OrderStatusApproved,
			CreatedBy:   "guest",
		}
	}
	if err := db.CreateInBatches(&orders, 500).Error; err != nil {
		b.Fatalf("create orders: %v", err)
	}

	return db, business, orders
}

func BenchmarkDeductApprovedOrderInventoryTx(b *testing.B) {
	gormDB, _, orders := setupInventoryDeductionBenchDB(b)
	items := []OrderItem{{
		ID:           "bench-order-line",
		ItemType:     "menu_item",
		MenuItemID:   "bench-burger",
		MenuItemName: "Bench Burger",
		Quantity:     1,
		Price:        12,
		Subtotal:     12,
	}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		order := orders[i]
		if err := gormDB.Transaction(func(tx *gorm.DB) error {
			return DeductApprovedOrderInventoryTx(tx, &order, items, "bench")
		}); err != nil {
			b.Fatalf("deduct inventory: %v", err)
		}
	}
}
