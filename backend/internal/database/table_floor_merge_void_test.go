package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A line moved by a merge keeps its id, so voiding it afterwards still finds
// and trims its kitchen line. Re-keying moved lines left the dish on the
// ticket while the bill line disappeared.
func TestMergeTableChecks_VoidMovedLineTrimsItsKitchenTicket(t *testing.T) {
	gdb := setupTableFloorTestDB(t)
	require.NoError(t, gdb.AutoMigrate(&InventorySettings{}, &InventoryItem{}, &InventoryRecipe{}, &InventoryMovement{}, &Menu{}))
	biz, source, target := seedFloorBizAndTables(t)

	srcBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: source.ID, Actor: "host"})
	require.NoError(t, err)
	tgtBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: target.ID, Actor: "host"})
	require.NoError(t, err)

	const teaID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	const soupID = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	ticket := &Order{
		BillID:      srcBill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "MERGE-VOID-1",
		Status:      OrderStatusInKitchen,
		CreatedBy:   "guest",
		Items: `[{"id":"` + teaID + `","menu_item_id":"tea","menu_item_name":"Iced Tea","price":5,"quantity":1,"subtotal":5},` +
			`{"id":"` + soupID + `","menu_item_id":"soup","menu_item_name":"Soup","price":7,"quantity":1,"subtotal":7}]`,
	}
	require.NoError(t, db.Create(ticket).Error)

	orderID := ticket.ID
	srcLines := []BillItem{
		{ID: teaID, BillID: srcBill.ID, MenuItemID: "tea", Name: "Iced Tea", Price: 5, Quantity: 1, Subtotal: 5, ItemType: "menu_item", OrderID: &orderID, CreatedAt: time.Now()},
		{ID: soupID, BillID: srcBill.ID, MenuItemID: "soup", Name: "Soup", Price: 7, Quantity: 1, Subtotal: 7, ItemType: "menu_item", OrderID: &orderID, CreatedAt: time.Now()},
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		srcBill.Subtotal = 1200
		return updateBillTx(tx, srcBill, srcLines)
	}))
	require.NoError(t, db.Exec(
		`INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, item_type, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		"aaaaaaaa-0000-4000-8000-000000000001", tgtBill.ID, "Wine", 12.0, 1, 12.0, "menu_item", time.Now(),
	).Error)
	require.NoError(t, db.Model(tgtBill).Updates(map[string]interface{}{
		"subtotal": int64(1200), "total_amount": int64(1320),
		"items": `[{"id":"aaaaaaaa-0000-4000-8000-000000000001","name":"Wine","price":12,"quantity":1,"subtotal":12,"item_type":"menu_item"}]`,
	}).Error)

	mergedTarget, _, err := MergeTableChecks(MergeTablesInput{
		BusinessID:    biz.ID,
		SourceTableID: source.ID,
		TargetTableID: target.ID,
		Actor:         "host",
	})
	require.NoError(t, err)

	items, err := billItemsForBillSnapshotConn(db, mergedTarget.ID, mergedTarget.Items)
	require.NoError(t, err)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	require.Contains(t, ids, teaID, "moved line keeps its id")
	require.Contains(t, ids, soupID)

	var rows int64
	require.NoError(t, db.Model(&BillItem{}).Where("bill_id = ? AND id = ?", mergedTarget.ID, teaID).Count(&rows).Error)
	require.EqualValues(t, 1, rows, "relational row follows the snapshot id")

	_, _, err = AdjustBillItem(mergedTarget.ID, teaID, "staff@example.com", nil, true, "guest changed mind")
	require.NoError(t, err)

	remaining := orderItemLines(t, ticket.ID)
	require.Len(t, remaining, 1, "the voided dish leaves the kitchen ticket")
	assert.Equal(t, soupID, remaining[0].ID)
	var reloaded Order
	require.NoError(t, db.First(&reloaded, ticket.ID).Error)
	assert.Equal(t, mergedTarget.ID, reloaded.BillID)
	assert.Equal(t, OrderStatusInKitchen, reloaded.Status)
}
