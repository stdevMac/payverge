package database

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Helper to setup test DB and assign to global 'db' variable
func setupTestDB(t *testing.T) {
	var err error
	// Use in-memory SQLite
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Assign to the package-level 'db' variable used by business.go
	db = gormDB
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("Failed to get sql DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	// Migrate models
	err = db.AutoMigrate(
		&Business{},
		&Table{},
		&Bill{},
		&Menu{},
		// &BillItem{}, // Skip auto-migration for BillItem due to postgres specific tags
		&Payment{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	// Manually create bill_items table for SQLite
	db.Exec("DROP TABLE IF EXISTS bill_items")
	if err := db.Exec(`
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
	`).Error; err != nil {
		t.Fatalf("Failed to create bill_items table: %v", err)
	}
}

func TestDualSync_CreateBill(t *testing.T) {
	setupTestDB(t)

	// Setup Business and Table
	business := &Business{Name: "Test Resto"}
	if err := db.Create(business).Error; err != nil {
		t.Fatalf("Failed to create business: %v", err)
	}
	table := &Table{BusinessID: business.ID, TableCode: "T1"}
	if err := db.Create(table).Error; err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Create Bill Items
	items := []BillItem{
		{
			MenuItemID: "item1",
			Name:       "Burger",
			Price:      10.0,
			Quantity:   2,
			Subtotal:   20.0,
		},
	}

	bill := &Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		TotalAmount: 20.0,
		Status:      BillStatusOpen,
	}

	// Call CreateBill
	err := CreateBill(bill, items)
	assert.NoError(t, err)

	t.Logf("Created Bill ID: %d", bill.ID)

	// 1. Verify Bill is created and has JSON items
	var savedBill Bill
	err = db.First(&savedBill, bill.ID).Error
	assert.NoError(t, err)
	assert.NotEmpty(t, savedBill.Items)

	// Verify JSON content
	var jsonItems []BillItem
	err = json.Unmarshal([]byte(savedBill.Items), &jsonItems)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(jsonItems))
	assert.Equal(t, "Burger", jsonItems[0].Name)
	// assert.Equal(t, bill.ID, jsonItems[0].BillID) // JSON items won't have the new BillID yet as they are marshaled before insert

	// 2. Verify BillItem relation matches
	var savedItems []BillItem
	err = db.Where("bill_id = ?", bill.ID).Find(&savedItems).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, len(savedItems))
	assert.Equal(t, "Burger", savedItems[0].Name)
	assert.Equal(t, 2, savedItems[0].Quantity)
}

func TestCreateBill_BundleChildSnapshotHasCatalogPriceAndTimestamp(t *testing.T) {
	setupTestDB(t)

	business := &Business{Name: "Core Date Night"}
	require.NoError(t, db.Create(business).Error)

	bundleID := uint(7)
	items := []BillItem{
		{
			MenuItemID: "bundle-7",
			Name:       "Date Night for Two",
			Price:      68,
			Quantity:   1,
			ItemType:   "bundle",
			BundleID:   &bundleID,
			Subtotal:   68,
		},
		{
			MenuItemID:     "demo-steak",
			Name:           "Steak Plate",
			Price:          42,
			Quantity:       1,
			ItemType:       "bundle_item",
			BundleID:       &bundleID,
			ParentBundleID: &bundleID,
			Subtotal:       0,
		},
	}
	bill := &Bill{BusinessID: business.ID, Status: BillStatusOpen, SettlementAddr: "0x1", TippingAddr: "0x2"}
	require.NoError(t, CreateBill(bill, items))

	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var snapshot []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &snapshot))
	require.Len(t, snapshot, 2)

	var child *BillItem
	for i := range snapshot {
		if snapshot[i].ItemType == "bundle_item" {
			child = &snapshot[i]
			break
		}
	}
	require.NotNil(t, child)
	assert.Equal(t, 42.0, child.Price)
	assert.Equal(t, 0.0, child.Subtotal)
	assert.False(t, child.CreatedAt.IsZero(), "JSON snapshot must not serialize year-1 timestamps")
	assert.GreaterOrEqual(t, child.CreatedAt.Year(), 2020)

	var rows []BillItem
	require.NoError(t, db.Where("bill_id = ? AND item_type = ?", bill.ID, "bundle_item").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, 42.0, rows[0].Price)
	assert.False(t, rows[0].CreatedAt.IsZero())
}

func TestDualSync_UpdateBill(t *testing.T) {
	setupTestDB(t)

	// Setup
	business := &Business{Name: "Test Resto"}
	db.Create(business)
	bill := &Bill{BusinessID: business.ID, Status: BillStatusOpen}
	db.Create(bill)

	// Update with new items
	items := []BillItem{
		{MenuItemID: "item1", Name: "Burger", Price: 10.0, Quantity: 1, Subtotal: 10.0},
		{MenuItemID: "item2", Name: "Fries", Price: 5.0, Quantity: 1, Subtotal: 5.0},
	}

	bill.TotalAmount = 15.0

	// Call UpdateBill
	err := UpdateBill(bill, items)
	assert.NoError(t, err)

	// 1. Verify JSON updated
	var savedBill Bill
	db.First(&savedBill, bill.ID)
	var jsonItems []BillItem
	assert.NoError(t, json.Unmarshal([]byte(savedBill.Items), &jsonItems))
	assert.Equal(t, 2, len(jsonItems))

	// 2. Verify Relational items updated
	var savedItems []BillItem
	db.Where("bill_id = ?", bill.ID).Order("name").Find(&savedItems)
	assert.Equal(t, 2, len(savedItems))
	assert.Equal(t, "Burger", savedItems[0].Name)
	assert.Equal(t, "Fries", savedItems[1].Name)

	// Verify clearing old items: Update again with fewer items
	items = []BillItem{
		{MenuItemID: "item1", Name: "Burger", Price: 10.0, Quantity: 3, Subtotal: 30.0},
	}
	bill.TotalAmount = 30.0
	err = UpdateBill(bill, items)
	assert.NoError(t, err)

	// Verify only 1 item remains in relation
	db.Where("bill_id = ?", bill.ID).Find(&savedItems)
	assert.Equal(t, 1, len(savedItems))
	assert.Equal(t, "Burger", savedItems[0].Name)
	assert.Equal(t, 3, savedItems[0].Quantity)
}

func TestGetBillByIDPrefersRelationalItemsWhenSnapshotIsStale(t *testing.T) {
	setupTestDB(t)

	business := &Business{Name: "Test Resto"}
	assert.NoError(t, db.Create(business).Error)
	table := &Table{BusinessID: business.ID, TableCode: "T-rel"}
	assert.NoError(t, db.Create(table).Error)

	staleItemsJSON, err := json.Marshal([]BillItem{
		{ID: "11111111-1111-1111-1111-111111111111", MenuItemID: "old", Name: "Stale Burger", Price: 9, Quantity: 1, Subtotal: 9},
	})
	assert.NoError(t, err)

	bill := &Bill{
		BusinessID: business.ID,
		TableID:    table.ID,
		BillNumber: "B-stale-snapshot",
		Status:     BillStatusOpen,
		Items:      string(staleItemsJSON),
	}
	assert.NoError(t, db.Create(bill).Error)

	relationalItem := BillItem{
		ID:         "22222222-2222-2222-2222-222222222222",
		BillID:     bill.ID,
		MenuItemID: "fresh",
		Name:       "Fresh Fries",
		Price:      5,
		Quantity:   2,
		Subtotal:   10,
	}
	assert.NoError(t, db.Create(&relationalItem).Error)

	_, items, err := GetBillByID(bill.ID)
	assert.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, "Fresh Fries", items[0].Name)
	assert.Equal(t, bill.ID, items[0].BillID)
}
