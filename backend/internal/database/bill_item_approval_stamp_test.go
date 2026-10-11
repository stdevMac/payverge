package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #708: approving an order appended its lines to the bills.items JSON snapshot
// without a CreatedAt, so every operator/guest bill payload rendered those
// lines at 0001-01-01T00:00:00Z while lines written by the bill-create/update
// paths carried a real time. The Date Night bundle parent and its children came
// in through approval; the tea line did not — hence the mixed timestamps on the
// live check. Children must also keep their catalog unit price with a $0
// subtotal (money lives on the parent line).
func TestApproveOrder_BundleLinesCarryCatalogPriceAndRealTimestamps(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 10, 0)

	teaStamp := time.Date(2026, 8, 19, 22, 41, 2, 0, time.UTC)
	tea := BillItem{
		ID:        "11111111-1111-4111-8111-111111111111",
		Name:      "Iced Tea",
		Price:     5,
		Quantity:  1,
		Subtotal:  5,
		ItemType:  "menu_item",
		CreatedAt: teaStamp,
	}
	bill := helperBill(t, biz, []BillItem{tea}, 5)

	bundleID := uint(6)
	occurrence := "occ-date-night-1"
	orderItems := []OrderItem{
		{
			ID: "bundle-6-1", ItemType: "bundle", MenuItemID: "6", MenuItemName: "Date Night",
			Price: 68, Quantity: 1, Subtotal: 68,
			BundleID: &bundleID, BundleOccurrenceID: occurrence,
		},
		{
			ID: "bundle-item-6-steak", ItemType: "bundle_item", MenuItemID: "steak", MenuItemName: "Steak",
			Price: 42, Quantity: 1, Subtotal: 0,
			BundleID: &bundleID, ParentBundleID: &bundleID, BundleOccurrenceID: occurrence,
		},
		{
			ID: "bundle-item-6-spritz", ItemType: "bundle_item", MenuItemID: "spritz", MenuItemName: "Demo Spritz",
			Price: 14, Quantity: 1, Subtotal: 0,
			BundleID: &bundleID, ParentBundleID: &bundleID, BundleOccurrenceID: occurrence,
		},
		{
			ID: "bundle-item-6-tart", ItemType: "bundle_item", MenuItemID: "tart", MenuItemName: "Chocolate Tart",
			Price: 12, Quantity: 1, Subtotal: 0,
			BundleID: &bundleID, ParentBundleID: &bundleID, BundleOccurrenceID: occurrence,
		},
	}
	order := helperOrder(t, biz, bill, orderItems)

	before := time.Now().Add(-time.Second)
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "manager", ""))

	var saved Bill
	require.NoError(t, db.First(&saved, bill.ID).Error)

	var snapshot []BillItem
	require.NoError(t, json.Unmarshal([]byte(saved.Items), &snapshot))
	require.Len(t, snapshot, 5)

	byName := make(map[string]BillItem, len(snapshot))
	for _, item := range snapshot {
		byName[item.Name] = item
	}

	// The pre-existing line keeps its own recorded time.
	require.Contains(t, byName, "Iced Tea")
	assert.True(t, byName["Iced Tea"].CreatedAt.Equal(teaStamp))

	for _, name := range []string{"Date Night", "Steak", "Demo Spritz", "Chocolate Tart"} {
		require.Contains(t, byName, name)
		line := byName[name]
		assert.False(t, line.CreatedAt.IsZero(), "%s snapshot line has a year-1 timestamp", name)
		assert.True(t, line.CreatedAt.After(before), "%s snapshot line must be stamped at approval time", name)
	}

	// Bundle children keep the catalog unit price for kitchen/costing display
	// while contributing no money — a $0 child must stay distinguishable from
	// an unset price.
	for name, wantPrice := range map[string]float64{"Steak": 42, "Demo Spritz": 14, "Chocolate Tart": 12} {
		line := byName[name]
		assert.Equal(t, "bundle_item", line.ItemType, "%s must stay a bundle child", name)
		assert.InDelta(t, wantPrice, line.Price, 0.001, "%s child lost its catalog unit price", name)
		assert.InDelta(t, 0.0, line.Subtotal, 0.001, "%s child must not carry money", name)
	}
	assert.InDelta(t, 68.0, byName["Date Night"].Price, 0.001)
	assert.InDelta(t, 68.0, byName["Date Night"].Subtotal, 0.001)

	// Only the parent bundle line bills: $5 tea + $68 bundle.
	assert.Equal(t, int64(7300), saved.Subtotal)

	// The relational rows are the read source of truth — they must agree with
	// the snapshot instant, not drift a second or a bill-creation-time apart.
	var relational []BillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&relational).Error)
	require.Len(t, relational, 5)
	relByName := make(map[string]BillItem, len(relational))
	for _, item := range relational {
		relByName[item.Name] = item
	}
	for _, name := range []string{"Date Night", "Steak", "Demo Spritz", "Chocolate Tart"} {
		require.Contains(t, relByName, name)
		assert.WithinDuration(t, byName[name].CreatedAt.UTC(), relByName[name].CreatedAt.UTC(), time.Millisecond,
			"%s relational row and snapshot must share one instant (rel=%s snap=%s)",
			name, relByName[name].CreatedAt, byName[name].CreatedAt)
		assert.InDelta(t, byName[name].Price, relByName[name].Price, 0.001,
			"%s relational row lost the catalog unit price", name)
	}
}
