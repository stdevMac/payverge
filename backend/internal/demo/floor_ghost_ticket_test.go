package demo

// #904: the US showroom floor showed Table 1 "occupied" for nine days with a
// pending ticket and NO bill at all, and venue 2 carried an in_kitchen ticket
// 71h old plus a 46-day-old pending one. Table status is derived, not stored:
// GetTablesWithStatus marks a table occupied when an unfinished order hangs off
// it even though its check is already terminal (#704 — expo may still be
// cooking after a $0 close). That grace was written for minutes, not weeks, and
// the demo storefront is public, so every abandoned guest checkout leaves a
// ticket that occupies a demo table forever.
//
// The showroom must retire its own dead tickets. These tests pin the exact
// boundary: a ticket stranded on a CLOSED check is retired, while a leftover
// check that is still ACTIVE — open, or partially paid, which is the shape the
// QA walk-out fixtures on the AR venues actually carry (#852: bills 1333/1335
// sit at status=partial with remaining 18550 of 37100; bill 1702 sits at open)
// — keeps its ticket and its occupied table untouched. Both statuses are
// pinned, because they are two distinct members of ActiveBillStatuses() and a
// predicate narrowed to `status <> 'open'` must fail this suite rather than
// free a live QA table.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

type ghostTicketFixture struct {
	name       string
	billStatus database.BillStatus
	billAge    time.Duration
	orderState database.OrderStatus
	wantStatus database.OrderStatus
	wantTable  string
}

// demoTableIDs returns demo tables that carry no active check of their own, so
// a fixture placed on one is the only thing that can occupy it.
func demoTableIDs(t *testing.T, db *gorm.DB, businessID uint) []uint {
	t.Helper()
	var ids []uint
	require.NoError(t, db.Model(&database.Table{}).
		Where("business_id = ?", businessID).
		Where("id NOT IN (?)", db.Model(&database.Bill{}).
			Select("table_id").
			Where("business_id = ? AND status IN ?", businessID, database.ActiveBillStatuses())).
		Where("id NOT IN (?)", db.Model(&database.Order{}).
			Select("bills.table_id").
			Joins("JOIN bills ON bills.id = orders.bill_id").
			Where("orders.business_id = ? AND orders.status IN ?", businessID,
				append(database.KitchenLiveOrderStatuses(), database.OrderStatusPending))).
		Order("id ASC").Pluck("id", &ids).Error)
	require.GreaterOrEqual(t, len(ids), 6, "demo venue must seed enough free tables for the fixtures")
	return ids
}

// seedGhostTicket puts one bill + one unfinished order on a table that has no
// other active check, exactly like a guest who walked out mid-checkout.
func seedGhostTicket(t *testing.T, db *gorm.DB, businessID, tableID uint, fx ghostTicketFixture, now time.Time) uint {
	t.Helper()
	stamp := now.Add(-fx.billAge)
	bill := database.Bill{
		BusinessID:  businessID,
		TableID:     tableID,
		BillNumber:  fmt.Sprintf("%s-b%d", fx.name, businessID),
		Notes:       "QA leftover — do not settle",
		Items:       "[]",
		Subtotal:    12000,
		TotalAmount: 12000,
		Status:      fx.billStatus,
		CreatedAt:   stamp,
		UpdatedAt:   stamp,
	}
	switch fx.billStatus {
	case database.BillStatusPaid:
		bill.PaidAmount = bill.TotalAmount
	case database.BillStatusPartial:
		// Live shape of 1333/1335: part of the check is settled and the rest is
		// still owed, which is precisely why the sweeper must leave it alone.
		bill.PaidAmount = bill.TotalAmount / 2
	}
	require.NoError(t, db.Create(&bill).Error)
	requestID := fmt.Sprintf("%s-b%d-req", fx.name, businessID)
	order := database.Order{
		BillID:          bill.ID,
		BusinessID:      businessID,
		OrderNumber:     fmt.Sprintf("%s-b%d", fx.name, businessID),
		Status:          fx.orderState,
		CreatedBy:       "guest",
		ClientRequestID: &requestID,
		Items:           "[]",
		CreatedAt:       stamp,
		UpdatedAt:       stamp,
	}
	require.NoError(t, db.Create(&order).Error)
	return order.ID
}

func tableStatusByID(t *testing.T, businessID uint) map[uint]string {
	t.Helper()
	rows, err := database.GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	statuses := map[uint]string{}
	for _, row := range rows {
		table, ok := row["table"].(database.Table)
		require.True(t, ok, "table row must carry the table model")
		status, ok := row["status"].(string)
		require.True(t, ok, "table row must carry a status")
		statuses[table.ID] = status
	}
	return statuses
}

func TestRetiresGhostTicketsStrandedOnTerminalChecks(t *testing.T) {
	db := newDemoServiceTestDB(t)
	database.InitTestDB(db)
	admin := seedAdmin(t, db, "demo-ghost-ticket@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 5})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	fixtures := []ghostTicketFixture{
		{
			// The live symptom: Table 1 occupied nine days, bills=0.
			name: "ghost-pending-closed", billStatus: database.BillStatusClosed, billAge: 9 * 24 * time.Hour,
			orderState: database.OrderStatusPending, wantStatus: database.OrderStatusOrderCancelled, wantTable: "available",
		},
		{
			// Food that was paid for was served — cancelling would rewrite history.
			name: "ghost-kitchen-paid", billStatus: database.BillStatusPaid, billAge: 3 * 24 * time.Hour,
			orderState: database.OrderStatusInKitchen, wantStatus: database.OrderStatusOrderDelivered, wantTable: "available",
		},
		{
			// The 15-minute pay-online window kills the check; the ticket must go with it.
			name: "ghost-pending-abandoned", billStatus: database.BillStatusAbandoned, billAge: 48 * time.Hour,
			orderState: database.OrderStatusPending, wantStatus: database.OrderStatusOrderCancelled, wantTable: "available",
		},
		{
			// QA walk-out fixture shape (AR Mesa 9/10): the check is still OPEN
			// and unpaid, so the table is legitimately occupied. Never touch it.
			name: "qa-leftover-open", billStatus: database.BillStatusOpen, billAge: 9 * 24 * time.Hour,
			orderState: database.OrderStatusPending, wantStatus: database.OrderStatusPending, wantTable: "occupied",
		},
		{
			// The status the live QA fixtures actually carry (#852: bills
			// 1333/1335 are `partial`, not `open`). Partially settled means the
			// guest is mid-payment, so the ticket and the table must survive the
			// sweep exactly like the open case.
			name: "qa-leftover-partial", billStatus: database.BillStatusPartial, billAge: 9 * 24 * time.Hour,
			orderState: database.OrderStatusPending, wantStatus: database.OrderStatusPending, wantTable: "occupied",
		},
		{
			// #704's actual case: the check closed minutes ago and expo is still
			// cooking. Inside the grace window the ticket and the table stand.
			name: "live-expo-just-closed", billStatus: database.BillStatusClosed, billAge: 10 * time.Minute,
			orderState: database.OrderStatusInKitchen, wantStatus: database.OrderStatusInKitchen, wantTable: "occupied",
		},
	}

	businesses := demoBusinesses(t, db, admin.ID)
	require.NotEmpty(t, businesses)
	type placed struct {
		orderID uint
		tableID uint
		fixture ghostTicketFixture
	}
	byBusiness := map[uint][]placed{}
	for _, business := range businesses {
		tableIDs := demoTableIDs(t, db, business.ID)
		require.GreaterOrEqualf(t, len(tableIDs), len(fixtures),
			"business %d seeds %d free tables, fixtures need %d", business.ID, len(tableIDs), len(fixtures))
		for i, fx := range fixtures {
			tableID := tableIDs[len(tableIDs)-1-i]
			orderID := seedGhostTicket(t, db, business.ID, tableID, fx, fixedNow())
			byBusiness[business.ID] = append(byBusiness[business.ID], placed{orderID, tableID, fx})
		}
	}

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	for businessID, rows := range byBusiness {
		statuses := tableStatusByID(t, businessID)
		for _, row := range rows {
			var order database.Order
			require.NoError(t, db.First(&order, row.orderID).Error)
			require.Equalf(t, row.fixture.wantStatus, order.Status,
				"business %d, fixture %s: unexpected ticket state", businessID, row.fixture.name)
			require.Equalf(t, row.fixture.wantTable, statuses[row.tableID],
				"business %d, fixture %s: table %d reads %q", businessID, row.fixture.name, row.tableID, statuses[row.tableID])
		}
	}
}

// The hourly append is the only pass that runs between deploys, so a ghost that
// appears at 20:00 must not survive until the next release.
func TestHourlyAppendRetiresGhostTickets(t *testing.T) {
	db := newDemoServiceTestDB(t)
	database.InitTestDB(db)
	admin := seedAdmin(t, db, "demo-ghost-ticket-hourly@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 5})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	businesses := demoBusinesses(t, db, admin.ID)
	require.NotEmpty(t, businesses)
	ghosts := map[uint]uint{}
	for _, business := range businesses {
		tableIDs := demoTableIDs(t, db, business.ID)
		fx := ghostTicketFixture{
			name:       fmt.Sprintf("hourly-ghost-%d", business.ID),
			billStatus: database.BillStatusClosed,
			billAge:    30 * time.Hour,
			orderState: database.OrderStatusPending,
		}
		ghosts[business.ID] = seedGhostTicket(t, db, business.ID, tableIDs[len(tableIDs)-1], fx, fixedNow())
	}

	require.NoError(t, svc.AppendDueDays(context.Background()))

	for businessID, orderID := range ghosts {
		var order database.Order
		require.NoError(t, db.First(&order, orderID).Error)
		require.Equalf(t, database.OrderStatusOrderCancelled, order.Status,
			"business %d: hourly append must retire the ghost ticket", businessID)
		require.NotNil(t, order.CancelledAt, "a retired ticket must record when it was retired")
		require.NotEmpty(t, order.CancelReason, "a retired ticket must say why")
	}
}
