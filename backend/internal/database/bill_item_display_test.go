package database

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedDateNightCatalog(t *testing.T, businessID uint) {
	t.Helper()
	categories, err := json.Marshal([]MenuCategory{
		{
			ID: "mains", Name: "Mains",
			Items: []MenuItem{
				{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
				{ID: "demo-dessert", Name: "Chocolate Tart", Price: 12, IsAvailable: true},
			},
		},
		{
			ID: "drinks", Name: "Drinks",
			Items: []MenuItem{
				{ID: "demo-cocktail", Name: "Demo Spritz", Price: 14, IsAvailable: true},
				{ID: "iced-tea", Name: "Iced Tea", Price: 5, IsAvailable: true},
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&Menu{
		BusinessID: businessID,
		Categories: string(categories),
		IsActive:   true,
	}).Error)
}

func TestCreateBill_ZeroPriceBundleChildrenFilledFromCatalog(t *testing.T) {
	setupTestDB(t)

	business := &Business{Name: "Core Date Night persist"}
	require.NoError(t, db.Create(business).Error)
	seedDateNightCatalog(t, business.ID)

	bundleID := uint(6)
	items := []BillItem{
		{
			MenuItemID: "bundle-6",
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
			Price:          0,
			Quantity:       1,
			ItemType:       "bundle_item",
			BundleID:       &bundleID,
			ParentBundleID: &bundleID,
			Subtotal:       0,
		},
		{
			MenuItemID:     "demo-cocktail",
			Name:           "Demo Spritz",
			Price:          0,
			Quantity:       2,
			ItemType:       "bundle_item",
			BundleID:       &bundleID,
			ParentBundleID: &bundleID,
			Subtotal:       0,
		},
		{
			MenuItemID:     "demo-dessert",
			Name:           "Chocolate Tart",
			Price:          0,
			Quantity:       1,
			ItemType:       "bundle_item",
			BundleID:       &bundleID,
			ParentBundleID: &bundleID,
			Subtotal:       0,
		},
	}
	bill := &Bill{
		BusinessID:     business.ID,
		Status:         BillStatusOpen,
		Subtotal:       6800,
		TotalAmount:    6800,
		SettlementAddr: "0x1",
		TippingAddr:    "0x2",
	}
	require.NoError(t, CreateBill(bill, items))

	var saved Bill
	require.NoError(t, db.First(&saved, bill.ID).Error)
	var snapshot []BillItem
	require.NoError(t, json.Unmarshal([]byte(saved.Items), &snapshot))
	require.Len(t, snapshot, 4)

	prices := map[string]float64{}
	for _, item := range snapshot {
		prices[item.Name] = item.Price
		assert.False(t, item.CreatedAt.IsZero(), "%s persisted year-1 created_at", item.Name)
		assert.GreaterOrEqual(t, item.CreatedAt.Year(), 2020)
		if item.ItemType == "bundle_item" {
			assert.Equal(t, 0.0, item.Subtotal, "%s child must not bill", item.Name)
		}
	}
	assert.Equal(t, 68.0, prices["Date Night for Two"])
	assert.Equal(t, 42.0, prices["Steak Plate"])
	assert.Equal(t, 14.0, prices["Demo Spritz"])
	assert.Equal(t, 12.0, prices["Chocolate Tart"])

	var rows []BillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&rows).Error)
	require.Len(t, rows, 4)
	for _, row := range rows {
		if row.ItemType != "bundle_item" {
			continue
		}
		assert.Greater(t, row.Price, 0.0, "relational %s catalog price", row.Name)
		assert.Equal(t, 0.0, row.Subtotal)
		assert.False(t, row.CreatedAt.IsZero())
	}
}

func TestGetBillByID_LeftoverDateNightSnapshotHydratesCatalogPriceAndTimestamp(t *testing.T) {
	setupTestDB(t)

	business := &Business{Name: "Core leftover 1140"}
	require.NoError(t, db.Create(business).Error)
	table := &Table{BusinessID: business.ID, TableCode: "4UKUUJRQX5"}
	require.NoError(t, db.Create(table).Error)
	seedDateNightCatalog(t, business.ID)

	opened := time.Date(2026, 8, 19, 22, 41, 2, 0, time.UTC)
	teaStamp := opened
	bundleID := uint(6)
	leftover := []BillItem{
		{
			ID: "11111111-1111-4111-8111-111111111111", Name: "Iced Tea", MenuItemID: "iced-tea",
			Price: 5, Quantity: 1, Subtotal: 5, ItemType: "menu_item", CreatedAt: teaStamp,
		},
		{
			ID: "22222222-2222-4222-8222-222222222222", Name: "Date Night for Two", MenuItemID: "bundle-6",
			Price: 68, Quantity: 1, Subtotal: 68, ItemType: "bundle", BundleID: &bundleID,
		},
		{
			ID: "33333333-3333-4333-8333-333333333333", Name: "Steak Plate", MenuItemID: "demo-steak",
			Price: 0, Quantity: 1, Subtotal: 0, ItemType: "bundle_item",
			BundleID: &bundleID, ParentBundleID: &bundleID,
		},
		{
			ID: "44444444-4444-4444-8444-444444444444", Name: "Demo Spritz", MenuItemID: "demo-cocktail",
			Price: 0, Quantity: 2, Subtotal: 0, ItemType: "bundle_item",
			BundleID: &bundleID, ParentBundleID: &bundleID,
		},
		{
			ID: "55555555-5555-4555-8555-555555555555", Name: "Chocolate Tart", MenuItemID: "demo-dessert",
			Price: 0, Quantity: 1, Subtotal: 0, ItemType: "bundle_item",
			BundleID: &bundleID, ParentBundleID: &bundleID,
		},
	}
	raw, err := json.Marshal(leftover)
	require.NoError(t, err)

	bill := &Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  "B85-8f259208-3de",
		Status:      BillStatusAbandoned,
		Items:       string(raw),
		Subtotal:    7300,
		TotalAmount: 8240,
		CreatedAt:   opened,
		UpdatedAt:   opened,
	}
	require.NoError(t, db.Create(bill).Error)
	for i := range leftover {
		row := leftover[i]
		row.BillID = bill.ID
		require.NoError(t, db.Create(&row).Error)
	}
	// GORM stamps CreatedAt on insert; leftover Date Night lines on prod were
	// stored as year-1 / NULL. Force that shape so the read path is what QA sees.
	require.NoError(t, db.Exec(
		"UPDATE bill_items SET created_at = NULL WHERE bill_id = ? AND name != ?",
		bill.ID, "Iced Tea",
	).Error)

	loaded, items, err := GetBillByID(bill.ID)
	require.NoError(t, err)
	require.Len(t, items, 5)

	byName := map[string]BillItem{}
	for _, item := range items {
		byName[item.Name] = item
		assert.False(t, item.CreatedAt.IsZero(), "%s read still year-1", item.Name)
		assert.GreaterOrEqual(t, item.CreatedAt.Year(), 2020)
	}
	assert.Equal(t, 5.0, byName["Iced Tea"].Price)
	assert.True(t, byName["Iced Tea"].CreatedAt.Equal(teaStamp))
	assert.Equal(t, 68.0, byName["Date Night for Two"].Price)
	assert.Equal(t, 68.0, byName["Date Night for Two"].Subtotal)
	assert.Equal(t, 42.0, byName["Steak Plate"].Price)
	assert.Equal(t, 14.0, byName["Demo Spritz"].Price)
	assert.Equal(t, 12.0, byName["Chocolate Tart"].Price)
	for _, name := range []string{"Steak Plate", "Demo Spritz", "Chocolate Tart"} {
		assert.Equal(t, 0.0, byName[name].Subtotal, "%s must stay informational", name)
	}

	// Wire snapshot must match the hydrated read — leftover bills.items is
	// what Bill.MarshalJSON and operator grouping still parse.
	var snapshot []BillItem
	require.NoError(t, json.Unmarshal([]byte(loaded.Items), &snapshot))
	snapPrices := map[string]float64{}
	for _, item := range snapshot {
		snapPrices[item.Name] = item.Price
		assert.False(t, item.CreatedAt.IsZero(), "%s snapshot still year-1", item.Name)
	}
	assert.Equal(t, 42.0, snapPrices["Steak Plate"])
	assert.Equal(t, 14.0, snapPrices["Demo Spritz"])
	assert.Equal(t, 12.0, snapPrices["Chocolate Tart"])

	// Parent + children must not inflate the leftover check.
	assert.Equal(t, int64(7300), loaded.Subtotal)
	assert.Equal(t, int64(8240), loaded.TotalAmount)
}

func TestHydrateBillItemsForRead_DoesNotInventMoneyOnChildren(t *testing.T) {
	setupTestDB(t)
	business := &Business{Name: "no double count"}
	require.NoError(t, db.Create(business).Error)
	seedDateNightCatalog(t, business.ID)

	bundleID := uint(6)
	items := []BillItem{
		{Name: "Date Night for Two", Price: 68, Quantity: 1, ItemType: "bundle", BundleID: &bundleID, Subtotal: 68},
		{Name: "Steak Plate", MenuItemID: "demo-steak", Price: 0, Quantity: 1, ItemType: "bundle_item", Subtotal: 0},
	}
	require.NoError(t, hydrateBillItemsForRead(db, items, business.ID, time.Date(2026, 8, 19, 22, 41, 2, 0, time.UTC)))
	assert.Equal(t, 42.0, items[1].Price)
	assert.Equal(t, 0.0, items[1].Subtotal)

	bill := &Bill{}
	applyBillTotals(bill, items, business)
	// Only the parent $68 is billable.
	assert.Equal(t, int64(6800), bill.Subtotal)
}

func leftoverDateNightBillItems(opened time.Time) (teaStamp time.Time, bundleID uint, leftover []BillItem) {
	teaStamp = opened
	bundleID = 6
	leftover = []BillItem{
		{
			ID: "11111111-1111-4111-8111-111111111111", Name: "Iced Tea", MenuItemID: "iced-tea",
			Price: 5, Quantity: 1, Subtotal: 5, ItemType: "menu_item", CreatedAt: teaStamp,
		},
		{
			ID: "22222222-2222-4222-8222-222222222222", Name: "Date Night for Two", MenuItemID: "bundle-6",
			Price: 68, Quantity: 1, Subtotal: 68, ItemType: "bundle", BundleID: &bundleID,
		},
		{
			ID: "33333333-3333-4333-8333-333333333333", Name: "Steak Plate", MenuItemID: "demo-steak",
			Price: 0, Quantity: 1, Subtotal: 0, ItemType: "bundle_item",
			BundleID: &bundleID, ParentBundleID: &bundleID,
		},
		{
			ID: "44444444-4444-4444-8444-444444444444", Name: "Demo Spritz", MenuItemID: "demo-cocktail",
			Price: 0, Quantity: 2, Subtotal: 0, ItemType: "bundle_item",
			BundleID: &bundleID, ParentBundleID: &bundleID,
		},
		{
			ID: "55555555-5555-4555-8555-555555555555", Name: "Chocolate Tart", MenuItemID: "demo-dessert",
			Price: 0, Quantity: 1, Subtotal: 0, ItemType: "bundle_item",
			BundleID: &bundleID, ParentBundleID: &bundleID,
		},
	}
	return teaStamp, bundleID, leftover
}

func TestGetBillListRows_LeftoverDateNightSnapshotHydratesCatalogPriceAndTimestamp(t *testing.T) {
	setupTestDB(t)

	business := &Business{Name: "Core leftover list 1140"}
	require.NoError(t, db.Create(business).Error)
	table := &Table{BusinessID: business.ID, TableCode: "4UKUUJRQX5"}
	require.NoError(t, db.Create(table).Error)
	seedDateNightCatalog(t, business.ID)

	opened := time.Date(2026, 8, 19, 22, 41, 2, 0, time.UTC)
	teaStamp, _, leftover := leftoverDateNightBillItems(opened)
	raw, err := json.Marshal(leftover)
	require.NoError(t, err)

	bill := &Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  "B85-8f259208-3de",
		Status:      BillStatusAbandoned,
		Items:       string(raw),
		Subtotal:    7300,
		TotalAmount: 8240,
		CreatedAt:   opened,
		UpdatedAt:   opened,
	}
	require.NoError(t, db.Create(bill).Error)
	for i := range leftover {
		row := leftover[i]
		row.BillID = bill.ID
		require.NoError(t, db.Create(&row).Error)
	}

	result, err := GetBillListRowsByBusinessIDFilteredPaginated(
		business.ID, BillListFilters{}, PaginationParams{Page: 1, PageSize: 20},
	)
	require.NoError(t, err)
	require.Len(t, result.Data, 1)

	var snapshot []BillItem
	require.NoError(t, json.Unmarshal([]byte(result.Data[0].Items), &snapshot))
	require.Len(t, snapshot, 5)

	byName := map[string]BillItem{}
	for _, item := range snapshot {
		byName[item.Name] = item
		assert.False(t, item.CreatedAt.IsZero(), "%s list snapshot still year-1", item.Name)
		assert.GreaterOrEqual(t, item.CreatedAt.Year(), 2020)
	}
	assert.True(t, byName["Iced Tea"].CreatedAt.Equal(teaStamp))
	assert.Equal(t, 42.0, byName["Steak Plate"].Price)
	assert.Equal(t, 14.0, byName["Demo Spritz"].Price)
	assert.Equal(t, 12.0, byName["Chocolate Tart"].Price)
	for _, name := range []string{"Steak Plate", "Demo Spritz", "Chocolate Tart"} {
		assert.Equal(t, 0.0, byName[name].Subtotal, "%s must stay informational", name)
	}
	assert.Equal(t, int64(7300), result.Data[0].Subtotal)
	assert.Equal(t, int64(8240), result.Data[0].TotalAmount)
}

func leftoverDateNightKitchenJSON() string {
	// Live leftover ticket 1128 stored $0 children and year-1 created_at.
	return `[{"id":"bundle-6-1","item_type":"bundle","menu_item_id":"bundle-6","menu_item_name":"Date Night for Two","quantity":1,"price":68,"subtotal":68,"created_at":"0001-01-01T00:00:00Z"},` +
		`{"id":"bundle-item-steak","item_type":"bundle_item","menu_item_id":"demo-steak","menu_item_name":"Steak Plate","quantity":1,"price":0,"subtotal":0,"created_at":"0001-01-01T00:00:00Z"},` +
		`{"id":"bundle-item-spritz","item_type":"bundle_item","menu_item_id":"demo-cocktail","menu_item_name":"Demo Spritz","quantity":2,"price":0,"subtotal":0,"created_at":"0001-01-01T00:00:00Z"},` +
		`{"id":"bundle-item-tart","item_type":"bundle_item","menu_item_id":"demo-dessert","menu_item_name":"Chocolate Tart","quantity":1,"price":0,"subtotal":0,"created_at":"0001-01-01T00:00:00Z"}]`
}

func TestGetOrdersByBusinessIDPaginated_LeftoverKitchenSnapshotHydrates(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)
	seedDateNightCatalog(t, business.ID)
	opened := time.Date(2026, 8, 19, 22, 42, 55, 0, time.UTC)
	bill := helperBill(t, business, nil, 68)
	require.NoError(t, db.Model(bill).Updates(map[string]any{"created_at": opened, "updated_at": opened}).Error)

	order := &Order{
		BillID:      bill.ID,
		BusinessID:  business.ID,
		OrderNumber: "G85-48196765",
		Status:      OrderStatusInKitchen,
		CreatedBy:   "guest",
		Items:       leftoverDateNightKitchenJSON(),
		CreatedAt:   opened,
		UpdatedAt:   opened,
	}
	require.NoError(t, db.Create(order).Error)

	result, err := GetOrdersByBusinessIDPaginated(
		business.ID, "", PaginationParams{Page: 1, PageSize: 20},
	)
	require.NoError(t, err)
	require.Len(t, result.Data, 1)
	assertKitchenDateNightSnapshotHydrated(t, result.Data[0].Items, opened)

	loaded, items, err := GetOrderByID(order.ID)
	require.NoError(t, err)
	assertKitchenDateNightSnapshotHydrated(t, loaded.Items, opened)
	require.Len(t, items, 4)
	assert.Equal(t, 42.0, items[1].Price)
	assert.Equal(t, 14.0, items[2].Price)
	assert.Equal(t, 12.0, items[3].Price)
	assert.False(t, items[1].CreatedAt.IsZero())
}

func assertKitchenDateNightSnapshotHydrated(t *testing.T, raw string, opened time.Time) {
	t.Helper()
	require.NotContains(t, raw, "0001-01-01", "kitchen snapshot still has year-1 timestamps")
	var items []OrderItem
	require.NoError(t, json.Unmarshal([]byte(raw), &items))
	require.Len(t, items, 4)
	byName := map[string]OrderItem{}
	for _, item := range items {
		byName[item.MenuItemName] = item
		assert.False(t, item.CreatedAt.IsZero(), "%s kitchen snapshot still year-1", item.MenuItemName)
		assert.GreaterOrEqual(t, item.CreatedAt.Year(), 2020)
		assert.True(t, item.CreatedAt.Equal(opened) || item.CreatedAt.After(opened.Add(-time.Second)))
	}
	assert.Equal(t, 68.0, byName["Date Night for Two"].Price)
	assert.Equal(t, 42.0, byName["Steak Plate"].Price)
	assert.Equal(t, 14.0, byName["Demo Spritz"].Price)
	assert.Equal(t, 12.0, byName["Chocolate Tart"].Price)
	for _, name := range []string{"Steak Plate", "Demo Spritz", "Chocolate Tart"} {
		assert.Equal(t, 0.0, byName[name].Subtotal, "%s must stay informational", name)
	}
}

func leftoverDateNightZeroChildren() []BillItem {
	bundleID := uint(6)
	return []BillItem{
		{
			Name: "Date Night for Two", MenuItemID: "bundle-6",
			Price: 68, Quantity: 1, Subtotal: 68, ItemType: "bundle", BundleID: &bundleID,
		},
		{
			Name: "Steak Plate", MenuItemID: "demo-steak",
			Price: 0, Quantity: 1, Subtotal: 0, ItemType: "bundle_item",
			BundleID: &bundleID, ParentBundleID: &bundleID,
		},
		{
			Name: "Demo Spritz", MenuItemID: "demo-cocktail",
			Price: 0, Quantity: 2, Subtotal: 0, ItemType: "bundle_item",
			BundleID: &bundleID, ParentBundleID: &bundleID,
		},
		{
			Name: "Chocolate Tart", MenuItemID: "demo-dessert",
			Price: 0, Quantity: 1, Subtotal: 0, ItemType: "bundle_item",
			BundleID: &bundleID, ParentBundleID: &bundleID,
		},
	}
}

func seedLeftoverDateNightBill(t *testing.T, businessID uint, withCatalog bool) *Bill {
	t.Helper()
	if withCatalog {
		seedDateNightCatalog(t, businessID)
	}
	opened := time.Date(2026, 8, 19, 22, 41, 2, 0, time.UTC)
	_, _, leftover := leftoverDateNightBillItems(opened)
	raw, err := json.Marshal(leftover)
	require.NoError(t, err)
	bill := &Bill{
		BusinessID:  businessID,
		BillNumber:  "B85-leftover-1140",
		Status:      BillStatusAbandoned,
		Items:       string(raw),
		Subtotal:    7300,
		TotalAmount: 8240,
		CreatedAt:   opened,
		UpdatedAt:   opened,
	}
	require.NoError(t, db.Create(bill).Error)
	for i := range leftover {
		row := leftover[i]
		row.BillID = bill.ID
		require.NoError(t, db.Create(&row).Error)
	}
	require.NoError(t, db.Exec(
		"UPDATE bill_items SET created_at = NULL WHERE bill_id = ? AND name != ?",
		bill.ID, "Iced Tea",
	).Error)
	return bill
}

func seedLeftoverKitchenOrder(t *testing.T, businessID uint, withCatalog bool) *Order {
	t.Helper()
	if withCatalog {
		seedDateNightCatalog(t, businessID)
	}
	opened := time.Date(2026, 8, 19, 22, 42, 55, 0, time.UTC)
	bill := &Bill{
		BusinessID:  businessID,
		BillNumber:  "B85-kds-1128",
		Status:      BillStatusOpen,
		Items:       leftoverDateNightKitchenJSON(),
		Subtotal:    6800,
		TotalAmount: 6800,
		CreatedAt:   opened,
		UpdatedAt:   opened,
	}
	require.NoError(t, db.Create(bill).Error)
	order := &Order{
		BillID:      bill.ID,
		BusinessID:  businessID,
		OrderNumber: "G85-48196765",
		Status:      OrderStatusInKitchen,
		CreatedBy:   "guest",
		Items:       leftoverDateNightKitchenJSON(),
		CreatedAt:   opened,
		UpdatedAt:   opened,
	}
	require.NoError(t, db.Create(order).Error)
	return order
}

func assertDateNightLeftoverPrices(t *testing.T, raw string) {
	t.Helper()
	var items []BillItem
	require.NoError(t, json.Unmarshal([]byte(raw), &items))
	byName := map[string]float64{}
	for _, item := range items {
		byName[item.Name] = item.Price
		assert.False(t, item.CreatedAt.IsZero(), "%s still year-1", item.Name)
		if item.ItemType == "bundle_item" {
			assert.Equal(t, 0.0, item.Subtotal, "%s must stay informational", item.Name)
		}
	}
	assert.Equal(t, 42.0, byName["Steak Plate"])
	assert.Equal(t, 14.0, byName["Demo Spritz"])
	assert.Equal(t, 12.0, byName["Chocolate Tart"])
}

func TestCreateBill_ZeroPriceBundleChildWithoutCatalogFails(t *testing.T) {
	setupTestDB(t)
	business := &Business{Name: "catalog miss persist"}
	require.NoError(t, db.Create(business).Error)

	bill := &Bill{
		BusinessID:     business.ID,
		Status:         BillStatusOpen,
		Subtotal:       6800,
		TotalAmount:    6800,
		SettlementAddr: "0x1",
		TippingAddr:    "0x2",
	}
	err := CreateBill(bill, leftoverDateNightZeroChildren())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMenuCatalogUnavailable)

	var count int64
	require.NoError(t, db.Model(&Bill{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count, "catalog miss must not persist a $0 leftover bill")
}

func TestCreateBill_ZeroPriceBundleChildCatalogMissFails(t *testing.T) {
	setupTestDB(t)
	business := &Business{Name: "partial catalog persist"}
	require.NoError(t, db.Create(business).Error)
	categories, err := json.Marshal([]MenuCategory{{
		ID: "drinks", Name: "Drinks",
		Items: []MenuItem{{ID: "iced-tea", Name: "Iced Tea", Price: 5, IsAvailable: true}},
	}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&Menu{
		BusinessID: business.ID,
		Categories: string(categories),
		IsActive:   true,
	}).Error)

	bill := &Bill{
		BusinessID:     business.ID,
		Status:         BillStatusOpen,
		Subtotal:       6800,
		TotalAmount:    6800,
		SettlementAddr: "0x1",
		TippingAddr:    "0x2",
	}
	err = CreateBill(bill, leftoverDateNightZeroChildren())
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBundleChildCatalogMiss)
}

func TestGetBillByID_LeftoverCatalogMissDoesNotKeepZero(t *testing.T) {
	setupTestDB(t)
	business := &Business{Name: "leftover miss detail"}
	require.NoError(t, db.Create(business).Error)
	bill := seedLeftoverDateNightBill(t, business.ID, false)

	loaded, items, err := GetBillByID(bill.ID)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrMenuCatalogUnavailable) || errors.Is(err, ErrBundleChildCatalogMiss), err)
	assert.Nil(t, loaded)
	assert.Nil(t, items)
}

func TestGetBillListRows_LeftoverCatalogMissDoesNotKeepZero(t *testing.T) {
	setupTestDB(t)
	business := &Business{Name: "leftover miss list"}
	require.NoError(t, db.Create(business).Error)
	_ = seedLeftoverDateNightBill(t, business.ID, false)

	_, err := GetBillListRowsByBusinessIDFilteredPaginated(
		business.ID, BillListFilters{}, PaginationParams{Page: 1, PageSize: 20},
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrMenuCatalogUnavailable) || errors.Is(err, ErrBundleChildCatalogMiss), err)
}

func TestGetOrdersByBusinessIDPaginated_CatalogMissDoesNotKeepZero(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)
	order := seedLeftoverKitchenOrder(t, business.ID, false)

	_, err := GetOrdersByBusinessIDPaginated(
		business.ID, "", PaginationParams{Page: 1, PageSize: 20},
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrMenuCatalogUnavailable) || errors.Is(err, ErrBundleChildCatalogMiss), err)

	_, items, err := GetOrderByID(order.ID)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrMenuCatalogUnavailable) || errors.Is(err, ErrBundleChildCatalogMiss), err)
	assert.Nil(t, items)
}

func TestHydrateBillSnapshotForWire_LeftoverDateNight(t *testing.T) {
	setupTestDB(t)
	business := &Business{Name: "sse bill leftover"}
	require.NoError(t, db.Create(business).Error)
	seedDateNightCatalog(t, business.ID)
	opened := time.Date(2026, 8, 19, 22, 41, 2, 0, time.UTC)
	_, _, leftover := leftoverDateNightBillItems(opened)
	raw, err := json.Marshal(leftover)
	require.NoError(t, err)
	bill := &Bill{BusinessID: business.ID, Items: string(raw), CreatedAt: opened}

	require.NoError(t, HydrateBillSnapshotForWire(bill))
	assertDateNightLeftoverPrices(t, bill.Items)
}

func TestHydrateBillSnapshotForWire_CatalogMissDoesNotKeepZero(t *testing.T) {
	setupTestDB(t)
	opened := time.Date(2026, 8, 19, 22, 41, 2, 0, time.UTC)
	_, _, leftover := leftoverDateNightBillItems(opened)
	raw, err := json.Marshal(leftover)
	require.NoError(t, err)
	bill := &Bill{BusinessID: 99, Items: string(raw), CreatedAt: opened}

	err = HydrateBillSnapshotForWire(bill)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMenuCatalogUnavailable)
	require.Contains(t, bill.Items, `"price":0`)
}

func TestHydrateOrderSnapshotForWire_LeftoverKitchenTicket(t *testing.T) {
	setupTestDB(t)
	business := &Business{Name: "sse kitchen leftover"}
	require.NoError(t, db.Create(business).Error)
	seedDateNightCatalog(t, business.ID)
	opened := time.Date(2026, 8, 19, 22, 42, 55, 0, time.UTC)
	order := &Order{
		BusinessID: business.ID,
		Items:      leftoverDateNightKitchenJSON(),
		CreatedAt:  opened,
	}
	require.NoError(t, HydrateOrderSnapshotForWire(order))
	assertKitchenDateNightSnapshotHydrated(t, order.Items, opened)
}

func TestHydrateOrderSnapshotForWire_CatalogMissDoesNotKeepZero(t *testing.T) {
	setupTestDB(t)
	order := &Order{
		BusinessID: 99,
		Items:      leftoverDateNightKitchenJSON(),
		CreatedAt:  time.Date(2026, 8, 19, 22, 42, 55, 0, time.UTC),
	}
	err := HydrateOrderSnapshotForWire(order)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMenuCatalogUnavailable)
	require.Contains(t, order.Items, `"price":0`)
}
