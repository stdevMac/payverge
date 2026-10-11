package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(value int) *int {
	return &value
}

func TestAdjustBillItem_UpdateQuantity(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 5)
	bill := helperBill(t, biz, []BillItem{
		{
			ID:         "line-1",
			MenuItemID: "burger",
			Name:       "Burger",
			Price:      10,
			Quantity:   1,
			Subtotal:   10,
			ItemType:   "menu_item",
		},
	}, 10)

	updatedBill, updatedItems, err := AdjustBillItem(
		bill.ID,
		"line-1",
		"manager@example.com",
		intPtr(3),
		false,
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, updatedBill)
	require.Len(t, updatedItems, 1)

	assert.Equal(t, 3, updatedItems[0].Quantity)
	assert.InDelta(t, 30.0, updatedItems[0].Subtotal, 0.01)
	assert.InDelta(t, 3000.0, updatedBill.Subtotal, 0.01)
	assert.InDelta(t, 300.0, updatedBill.TaxAmount, 0.01)
	assert.InDelta(t, 150.0, updatedBill.ServiceFeeAmount, 0.01)
	assert.InDelta(t, 3450.0, updatedBill.TotalAmount, 0.01)

	var history []BillHistoryEvent
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Order("id ASC").Find(&history).Error)
	require.Len(t, history, 1)
	assert.Equal(t, BillHistoryEventItemQtyUpdated, history[0].EventType)
	assert.Equal(t, "manager@example.com", history[0].Actor)
	assert.Equal(t, "Burger", history[0].ItemName)
	assert.Equal(t, float64(1), history[0].Details["quantity_before"])
	assert.Equal(t, float64(3), history[0].Details["quantity_after"])
}

func TestAdjustBillItem_RoundTripPreservesMinorUnitTotalWithDiscounts(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 8.875, 7.25)
	items := []BillItem{
		{
			ID:         "00000000-0000-4000-8000-000000000001",
			MenuItemID: "special",
			Name:       "Special",
			Price:      8.99,
			Quantity:   1,
			Subtotal:   8.99,
			ItemType:   "menu_item",
		},
		{
			ID:       "00000000-0000-4000-8000-000000000002",
			Name:     "Promotion",
			Price:    -0.17,
			Quantity: 1,
			Subtotal: -0.17,
			ItemType: "discount",
		},
	}
	bill := helperBill(t, biz, items, 8.82)
	bill.TaxAmount = 78
	bill.ServiceFeeAmount = 64
	bill.LoyaltyDiscountCents = 123
	bill.TotalAmount = 901
	require.NoError(t, db.Save(bill).Error)

	beforeCents := bill.TotalAmount
	_, _, err := AdjustBillItem(bill.ID, "00000000-0000-4000-8000-000000000001", "manager", intPtr(2), false, "")
	require.NoError(t, err)
	_, _, err = AdjustBillItem(bill.ID, "00000000-0000-4000-8000-000000000001", "manager", intPtr(1), false, "")
	require.NoError(t, err)

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Equal(t, beforeCents, reloaded.TotalAmount)
	require.Equal(t, int64(882), reloaded.Subtotal)
	require.Equal(t, int64(78), reloaded.TaxAmount)
	require.Equal(t, int64(64), reloaded.ServiceFeeAmount)
}

func TestGetBillByID_PreservesBundleOccurrencesAcrossRelationalRebuild(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bundleID := uint(10)
	parentA := "00000000-0000-4000-8000-000000000001"
	childA := "00000000-0000-4000-8000-000000000002"
	parentB := "00000000-0000-4000-8000-000000000003"
	childB := "00000000-0000-4000-8000-000000000004"
	bill := helperBill(t, biz, []BillItem{
		{ID: parentA, Name: "Combo", Price: 15, Quantity: 1, Subtotal: 15, ItemType: "bundle", BundleID: &bundleID, BundleOccurrenceID: parentA},
		{ID: childA, Name: "Burger", Quantity: 1, Price: 8, ItemType: "bundle_item", ParentBundleID: &bundleID, BundleOccurrenceID: parentA},
		{ID: parentB, Name: "Combo", Price: 15, Quantity: 1, Subtotal: 15, ItemType: "bundle", BundleID: &bundleID, BundleOccurrenceID: parentB},
		{ID: childB, Name: "Cola", Quantity: 1, Price: 3, ItemType: "bundle_item", ParentBundleID: &bundleID, BundleOccurrenceID: parentB},
	}, 30)

	loadedItems, err := billItemsForBillSnapshot(bill.ID, bill.Items)
	require.NoError(t, err)
	require.Len(t, loadedItems, 4)
	require.NoError(t, UpdateBill(bill, loadedItems))

	updatedItems, err := billItemsForBillSnapshot(bill.ID, bill.Items)
	require.NoError(t, err)
	require.Len(t, updatedItems, 4)

	occurrences := make(map[string]string)
	for _, item := range updatedItems {
		occurrences[item.ID] = item.BundleOccurrenceID
	}
	require.Equal(t, parentA, occurrences[parentA])
	require.Equal(t, parentA, occurrences[childA])
	require.Equal(t, parentB, occurrences[parentB])
	require.Equal(t, parentB, occurrences[childB])
}

func TestAdjustBillItem_VoidRequiresReason(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, []BillItem{
		{
			ID:         "line-2",
			MenuItemID: "fries",
			Name:       "Fries",
			Price:      5,
			Quantity:   1,
			Subtotal:   5,
			ItemType:   "menu_item",
		},
	}, 5)

	_, _, err := AdjustBillItem(bill.ID, "line-2", "staff", nil, true, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "void reason is required")
}

func TestAdjustBillItem_VoidRemovesItemAndRecordsHistory(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 0)
	bill := helperBill(t, biz, []BillItem{
		{
			ID:         "line-1",
			MenuItemID: "burger",
			Name:       "Burger",
			Price:      10,
			Quantity:   1,
			Subtotal:   10,
			ItemType:   "menu_item",
		},
		{
			ID:         "line-2",
			MenuItemID: "fries",
			Name:       "Fries",
			Price:      5,
			Quantity:   2,
			Subtotal:   10,
			ItemType:   "menu_item",
		},
	}, 20)

	updatedBill, updatedItems, err := AdjustBillItem(
		bill.ID,
		"line-2",
		"staff@example.com",
		nil,
		true,
		"Guest changed their mind",
	)
	require.NoError(t, err)
	require.NotNil(t, updatedBill)
	require.Len(t, updatedItems, 1)

	assert.Equal(t, "Burger", updatedItems[0].Name)
	assert.InDelta(t, 1000.0, updatedBill.Subtotal, 0.01)
	assert.InDelta(t, 100.0, updatedBill.TaxAmount, 0.01)
	assert.InDelta(t, 1100.0, updatedBill.TotalAmount, 0.01)

	var history []BillHistoryEvent
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Order("id ASC").Find(&history).Error)
	require.Len(t, history, 1)
	assert.Equal(t, BillHistoryEventItemVoided, history[0].EventType)
	assert.Equal(t, "staff@example.com", history[0].Actor)
	assert.Equal(t, "Guest changed their mind", history[0].Reason)
	assert.Equal(t, "Fries", history[0].ItemName)
}
