package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedKitchenTicket(t *testing.T, billID, businessID uint, number string, status OrderStatus) *Order {
	t.Helper()
	order := &Order{
		BillID:      billID,
		BusinessID:  businessID,
		OrderNumber: number,
		Status:      status,
		CreatedBy:   "guest",
		Items:       `[{"id":"tea-1","menu_item_name":"Iced Tea","price":5,"quantity":1,"subtotal":5}]`,
	}
	require.NoError(t, db.Create(order).Error)
	return order
}

// #704 (bill 1132 / order 1123): merging two checks moved the money and the
// items onto the target and voided the source at $0, but left every kitchen
// ticket pointing at the now-terminal source bill. Expo kept cooking against a
// voided $0 check. The ticket must follow the money onto the target bill.
func TestMergeTableChecks_MovesLiveKitchenTicketsToTargetBill(t *testing.T) {
	setupTableFloorTestDB(t)
	biz, source, target := seedFloorBizAndTables(t)

	srcBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: source.ID, Actor: "host"})
	require.NoError(t, err)
	tgtBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: target.ID, Actor: "host"})
	require.NoError(t, err)

	require.NoError(t, db.Exec(
		`INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, item_type, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		"src-tea", srcBill.ID, "Iced Tea", 5.0, 1, 5.0, "menu_item", time.Now(),
	).Error)
	require.NoError(t, db.Model(srcBill).Updates(map[string]interface{}{
		"subtotal": int64(500), "tax_amount": int64(50), "total_amount": int64(550),
		"items": `[{"id":"src-tea","name":"Iced Tea","price":5,"quantity":1,"subtotal":5,"item_type":"menu_item"}]`,
	}).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, item_type, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		"tgt-wine", tgtBill.ID, "Wine", 12.0, 1, 12.0, "menu_item", time.Now(),
	).Error)
	require.NoError(t, db.Model(tgtBill).Updates(map[string]interface{}{
		"subtotal": int64(1200), "tax_amount": int64(120), "total_amount": int64(1320),
		"items": `[{"id":"tgt-wine","name":"Wine","price":12,"quantity":1,"subtotal":12,"item_type":"menu_item"}]`,
	}).Error)

	kitchenTicket := seedKitchenTicket(t, srcBill.ID, biz.ID, "G86-36604192", OrderStatusInKitchen)
	readyTicket := seedKitchenTicket(t, srcBill.ID, biz.ID, "G86-36604193", OrderStatusOrderReady)

	mergedTarget, mergedSource, err := MergeTableChecks(MergeTablesInput{
		BusinessID:    biz.ID,
		SourceTableID: source.ID,
		TargetTableID: target.ID,
		Actor:         "host",
	})
	require.NoError(t, err)
	require.NotNil(t, mergedTarget)
	require.NotNil(t, mergedSource)
	require.Equal(t, BillStatusVoided, mergedSource.Status)
	assert.Nil(t, mergedSource.SettledAt)
	require.Equal(t, int64(0), mergedSource.TotalAmount)

	for _, seeded := range []*Order{kitchenTicket, readyTicket} {
		var reloaded Order
		require.NoError(t, db.First(&reloaded, seeded.ID).Error)
		assert.Equal(t, mergedTarget.ID, reloaded.BillID,
			"live kitchen ticket %s must follow the money onto the merged check", seeded.OrderNumber)
		assert.Equal(t, seeded.Status, reloaded.Status, "merging must not silently cancel cooking food")
	}

	// No live kitchen work may reference the zeroed, voided source check.
	var orphaned int64
	require.NoError(t, db.Model(&Order{}).
		Where("bill_id = ? AND status IN ?", mergedSource.ID, KitchenLiveOrderStatuses()).
		Count(&orphaned).Error)
	assert.Equal(t, int64(0), orphaned, "voided $0 source check still owns live kitchen tickets")
}

func putMoneyOnOpenCheck(t *testing.T, bill *Bill, cents int64) {
	t.Helper()
	require.NoError(t, db.Model(bill).Updates(map[string]interface{}{
		"subtotal":     cents,
		"total_amount": cents,
		"items":        `[{"id":"tea","name":"Iced Tea","price":18.04,"quantity":1,"subtotal":18.04,"item_type":"menu_item"}]`,
	}).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, item_type, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		fmt.Sprintf("item-%d", bill.ID), bill.ID, "Iced Tea", 18.04, 1, 18.04, "menu_item", time.Now(),
	).Error)
}

// Four Liberar arms from live #704 (T9 / bill 761 remaining $18.04 returned
// settle_required and never kitchen_tickets_live / orders_pending_approval).
// Kitchen and the approval queue must win over the open-check settle door.
func TestClearTable_FourArms(t *testing.T) {
	type arm struct {
		name       string
		status     OrderStatus
		withMoney  bool
		wantErr    error
		wantVoided bool
		notErr     error
	}
	for _, tc := range []arm{
		{
			name:       "clean_empty_clear",
			wantVoided: true,
		},
		{
			name:    "pending_send_refuses_queue_not_kitchen",
			status:  OrderStatusPending,
			wantErr: ErrFloorPendingOrders,
			notErr:  ErrFloorLiveKitchenTickets,
		},
		{
			name:    "approved_ticket_refuses_kitchen",
			status:  OrderStatusApproved,
			wantErr: ErrFloorLiveKitchenTickets,
		},
		{
			name:    "in_kitchen_ticket_refuses_kitchen",
			status:  OrderStatusInKitchen,
			wantErr: ErrFloorLiveKitchenTickets,
		},
		{
			name:       "delivered_ticket_allows_clear",
			status:     OrderStatusOrderDelivered,
			wantVoided: true,
		},
		{
			name:      "unpaid_plus_in_kitchen_is_kitchen_not_settle",
			status:    OrderStatusInKitchen,
			withMoney: true,
			wantErr:   ErrFloorLiveKitchenTickets,
			notErr:    ErrFloorSettleRequired,
		},
		{
			name:      "unpaid_plus_pending_is_queue_not_kitchen_or_settle",
			status:    OrderStatusPending,
			withMoney: true,
			wantErr:   ErrFloorPendingOrders,
			notErr:    ErrFloorLiveKitchenTickets,
		},
		{
			name:      "unpaid_after_delivered_still_settle_required",
			status:    OrderStatusOrderDelivered,
			withMoney: true,
			wantErr:   ErrFloorSettleRequired,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTableFloorTestDB(t)
			biz, table, _ := seedFloorBizAndTables(t)

			bill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: table.ID, Actor: "host"})
			require.NoError(t, err)
			if tc.withMoney {
				putMoneyOnOpenCheck(t, bill, 1804)
			}
			if tc.status != "" {
				seedKitchenTicket(t, bill.ID, biz.ID, "G86-"+tc.name, tc.status)
			}

			cleared, err := ClearTable(ClearTableInput{BusinessID: biz.ID, TableID: table.ID, Actor: "host"})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				if tc.notErr != nil {
					require.NotErrorIs(t, err, tc.notErr)
				}
				var reloaded Bill
				require.NoError(t, db.First(&reloaded, bill.ID).Error)
				assert.Equal(t, BillStatusOpen, reloaded.Status)
				assert.Nil(t, reloaded.ClosedAt)
				return
			}
			require.NoError(t, err)
			require.True(t, tc.wantVoided)
			assert.Equal(t, BillStatusVoided, cleared.Status)
			assert.Nil(t, cleared.SettledAt)
			assert.NotNil(t, cleared.ClosedAt)
		})
	}
}

// Live 1132/T5: a closed $0 (or abandoned) check still owns an in_kitchen
// ticket while the table reads Available. Liberar must inspect the table, not
// only the current open check, and refuse kitchen_tickets_live.
func TestClearTable_RefusesKitchenTicketsOnAbandonedBillForSameTable(t *testing.T) {
	setupTableFloorTestDB(t)
	biz, table, _ := seedFloorBizAndTables(t)

	orphaned := &Bill{
		BusinessID:  biz.ID,
		TableID:     table.ID,
		BillNumber:  "1132",
		Status:      BillStatusAbandoned,
		TotalAmount: 0,
		Items:       "[]",
	}
	require.NoError(t, db.Create(orphaned).Error)
	seedKitchenTicket(t, orphaned.ID, biz.ID, "G86-36604192", OrderStatusInKitchen)

	_, err := ClearTable(ClearTableInput{BusinessID: biz.ID, TableID: table.ID, Actor: "host"})
	require.ErrorIs(t, err, ErrFloorLiveKitchenTickets, "orphaned in_kitchen ticket must win over no_active_bill")
	require.NotErrorIs(t, err, ErrFloorNoActiveBill)
}

func TestClearTable_RefusesPendingOnAbandonedBillForSameTable(t *testing.T) {
	setupTableFloorTestDB(t)
	biz, table, _ := seedFloorBizAndTables(t)

	orphaned := &Bill{
		BusinessID:  biz.ID,
		TableID:     table.ID,
		BillNumber:  "1139",
		Status:      BillStatusAbandoned,
		TotalAmount: 0,
		Items:       "[]",
	}
	require.NoError(t, db.Create(orphaned).Error)
	seedKitchenTicket(t, orphaned.ID, biz.ID, "G86-1129", OrderStatusPending)

	_, err := ClearTable(ClearTableInput{BusinessID: biz.ID, TableID: table.ID, Actor: "host"})
	require.ErrorIs(t, err, ErrFloorPendingOrders)
	require.NotErrorIs(t, err, ErrFloorLiveKitchenTickets)
	require.NotErrorIs(t, err, ErrFloorNoActiveBill)
}

// #704 adversarial: a host must not seat onto a table kitchen is still
// working, even when the only check is abandoned (T5 / ticket 1123).
func TestSeatWalkIn_RefusesInKitchenOnAbandonedBill(t *testing.T) {
	setupTableFloorTestDB(t)
	biz, table, _ := seedFloorBizAndTables(t)

	orphaned := &Bill{
		BusinessID:  biz.ID,
		TableID:     table.ID,
		BillNumber:  "1132",
		Status:      BillStatusAbandoned,
		TotalAmount: 0,
		Items:       "[]",
	}
	require.NoError(t, db.Create(orphaned).Error)
	seedKitchenTicket(t, orphaned.ID, biz.ID, "G86-36604192", OrderStatusInKitchen)

	_, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: table.ID, Actor: "host"})
	require.ErrorIs(t, err, ErrFloorLiveKitchenTickets)
	require.NotErrorIs(t, err, ErrFloorTableOccupied)

	var opened int64
	require.NoError(t, db.Model(&Bill{}).
		Where("table_id = ? AND status IN ?", table.ID, activeBillStatusStrings()).
		Count(&opened).Error)
	assert.Equal(t, int64(0), opened, "must not open a new check over cooking food")
}

// Transfer onto a table whose abandoned check still has in_kitchen tickets
// would hide expo work behind a different party's check.
func TestTransferActiveBill_RefusesTargetWithInKitchenOnAbandoned(t *testing.T) {
	setupTableFloorTestDB(t)
	biz, source, target := seedFloorBizAndTables(t)

	srcBill, err := SeatWalkIn(SeatWalkInInput{BusinessID: biz.ID, TableID: source.ID, Actor: "host"})
	require.NoError(t, err)

	orphaned := &Bill{
		BusinessID:  biz.ID,
		TableID:     target.ID,
		BillNumber:  "1134",
		Status:      BillStatusAbandoned,
		TotalAmount: 0,
		Items:       "[]",
	}
	require.NoError(t, db.Create(orphaned).Error)
	seedKitchenTicket(t, orphaned.ID, biz.ID, "G86-1124", OrderStatusInKitchen)

	_, err = TransferActiveBill(TransferBillInput{
		BusinessID:    biz.ID,
		SourceTableID: source.ID,
		TargetTableID: target.ID,
		Actor:         "host",
	})
	require.ErrorIs(t, err, ErrFloorLiveKitchenTickets)
	require.NotErrorIs(t, err, ErrFloorTargetOccupied)

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, srcBill.ID).Error)
	assert.Equal(t, source.ID, reloaded.TableID, "source check must stay on the source table")
}
