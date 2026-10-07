package database

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupVoidKitchenTestDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf("file:void-kitchen-%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })

	require.NoError(t, gormDB.AutoMigrate(
		&Business{},
		&Bill{},
		&BillHistoryEvent{},
		&CustomerBusiness{},
		&Order{},
		&OperationalAlert{},
		&OperationalAlertEvent{},
	))
}

func seedVoidableBill(t *testing.T, businessID uint, number string) *Bill {
	t.Helper()
	bill := &Bill{
		BusinessID: businessID,
		BillNumber: number,
		Status:     BillStatusOpen,
		Items:      "[]",
		PaidAmount: 0,
	}
	require.NoError(t, db.Create(bill).Error)
	return bill
}

// #704 twin door: the host hits Liberar, gets kitchen_tickets_live, walks to
// Bills and voids the check "to kill it". VoidBill only cancelled *pending*
// orders, so approved / in_kitchen / ready tickets stayed attached to a voided
// bill. Expo keeps plating, inventory is already deducted, and there is no open
// check left to charge against. Same class as bill 1132, different door.
func TestVoidBill_RefusesWhileKitchenTicketsAreLive(t *testing.T) {
	setupVoidKitchenTestDB(t)
	const businessID uint = 1

	for _, status := range []OrderStatus{OrderStatusApproved, OrderStatusInKitchen, OrderStatusOrderReady} {
		t.Run(string(status), func(t *testing.T) {
			bill := seedVoidableBill(t, businessID, "VOID-KITCHEN-"+string(status))
			live := seedKitchenTicket(t, bill.ID, businessID, "G86-VK-"+string(status), status)

			_, err := VoidBill(bill.ID, "manager", "host wanted the table back")
			require.ErrorIs(t, err, ErrBillVoidLiveKitchenTickets,
				"voiding must refuse while expo still owes this check food")

			var reloaded Bill
			require.NoError(t, db.First(&reloaded, bill.ID).Error)
			assert.Equal(t, BillStatusOpen, reloaded.Status,
				"check must stay open so the fired food still has something to charge against")
			assert.Nil(t, reloaded.ClosedAt)

			var reloadedOrder Order
			require.NoError(t, db.First(&reloadedOrder, live.ID).Error)
			assert.Equal(t, status, reloadedOrder.Status, "refusing must not silently cancel cooking food")
		})
	}
}

// The refusal must be narrow: never-fired pending sends and terminal tickets
// are not live kitchen work. Voiding still works and still auto-cancels the
// pending queue the way CloseBillWithHistory does.
func TestVoidBill_SucceedsWithOnlyPendingAndTerminalOrders(t *testing.T) {
	setupVoidKitchenTestDB(t)
	const businessID uint = 1

	bill := seedVoidableBill(t, businessID, "VOID-KITCHEN-OK")
	pending := seedKitchenTicket(t, bill.ID, businessID, "G86-VK-PEND", OrderStatusPending)
	delivered := seedKitchenTicket(t, bill.ID, businessID, "G86-VK-DELIV", OrderStatusOrderDelivered)
	cancelled := seedKitchenTicket(t, bill.ID, businessID, "G86-VK-CANC", OrderStatusOrderCancelled)

	voided, err := VoidBill(bill.ID, "manager", "duplicate check")
	require.NoError(t, err)
	require.NotNil(t, voided)
	assert.Equal(t, BillStatusVoided, voided.Status)

	var reloadedPending Order
	require.NoError(t, db.First(&reloadedPending, pending.ID).Error)
	assert.Equal(t, OrderStatusOrderCancelled, reloadedPending.Status,
		"a pending order on a voided bill is unapprovable forever — it must be auto-cancelled")
	assert.Equal(t, CancelReasonBillClosed, reloadedPending.CancelReason)

	for _, seeded := range []*Order{delivered, cancelled} {
		var reloaded Order
		require.NoError(t, db.First(&reloaded, seeded.ID).Error)
		assert.Equal(t, seeded.Status, reloaded.Status, "terminal tickets are history — void must not rewrite them")
	}
}
