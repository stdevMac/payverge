package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRefundBillPayment_ClosedBillStaysClosedAndFreesTable(t *testing.T) {
	wrapper := setupBillSplitTestDB(t, nil)
	bill := createBillSplitBill(t, wrapper, "refund-closed-terminal", 1000)
	closedAt := time.Date(2026, 8, 1, 15, 4, 5, 0, time.UTC)
	const tableID uint = 42
	require.NoError(t, GetDB().Model(bill).Updates(map[string]interface{}{
		"total_amount": int64(1000),
		"paid_amount":  int64(1000),
		"status":       BillStatusClosed,
		"closed_at":    closedAt,
		"table_id":     tableID,
	}).Error)
	payment := &Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xguest",
		Amount:        1000,
		TxHash:        "refund-closed-terminal-tx",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}
	require.NoError(t, GetDB().Create(payment).Error)

	refunded, refundedPayment, err := RefundBillPayment(bill.ID, payment.ID, "staff", "guest requested refund")
	require.NoError(t, err)
	require.NotNil(t, refunded)
	require.Equal(t, PaymentStatusRefunded, refundedPayment.Status)
	require.Equal(t, BillStatusClosed, refunded.Status)
	require.NotNil(t, refunded.ClosedAt)
	require.WithinDuration(t, closedAt, refunded.ClosedAt.UTC(), time.Second)
	require.Equal(t, int64(0), refunded.PaidAmount)

	var reloaded Bill
	require.NoError(t, GetDB().First(&reloaded, bill.ID).Error)
	require.Equal(t, BillStatusClosed, reloaded.Status)
	require.NotNil(t, reloaded.ClosedAt)
	require.WithinDuration(t, closedAt, reloaded.ClosedAt.UTC(), time.Second)
	require.Equal(t, int64(0), reloaded.PaidAmount)

	active, err := HasActiveBillForTableID(tableID)
	require.NoError(t, err)
	require.False(t, active)
}

func TestRefundBillPayment_PaidBillStaysPaid(t *testing.T) {
	wrapper := setupBillSplitTestDB(t, nil)
	bill := createBillSplitBill(t, wrapper, "refund-paid-terminal", 1000)
	closedAt := time.Date(2026, 8, 2, 11, 0, 0, 0, time.UTC)
	require.NoError(t, GetDB().Model(bill).Updates(map[string]interface{}{
		"total_amount": int64(1000),
		"paid_amount":  int64(1000),
		"status":       BillStatusPaid,
		"closed_at":    closedAt,
	}).Error)
	payment := &Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xguest",
		Amount:        1000,
		TxHash:        "refund-paid-terminal-tx",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}
	require.NoError(t, GetDB().Create(payment).Error)

	refunded, _, err := RefundBillPayment(bill.ID, payment.ID, "staff", "guest requested refund")
	require.NoError(t, err)
	require.Equal(t, BillStatusPaid, refunded.Status)
	require.Equal(t, int64(0), refunded.PaidAmount)
	require.NotNil(t, refunded.ClosedAt)
	require.WithinDuration(t, closedAt, refunded.ClosedAt.UTC(), time.Second)

	var reloaded Bill
	require.NoError(t, GetDB().First(&reloaded, bill.ID).Error)
	require.Equal(t, BillStatusPaid, reloaded.Status)
	require.Equal(t, int64(0), reloaded.PaidAmount)
	require.NotNil(t, reloaded.ClosedAt)
}

func TestRefundBillPayment_PartialBillBecomesOpen(t *testing.T) {
	wrapper := setupBillSplitTestDB(t, nil)
	bill := createBillSplitBill(t, wrapper, "refund-partial-to-open", 1000)
	require.NoError(t, GetDB().Model(bill).Updates(map[string]interface{}{
		"total_amount": int64(1000),
		"paid_amount":  int64(400),
		"status":       BillStatusPartial,
	}).Error)
	payment := &Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xguest",
		Amount:        400,
		TxHash:        "refund-partial-to-open-tx",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}
	require.NoError(t, GetDB().Create(payment).Error)

	refunded, _, err := RefundBillPayment(bill.ID, payment.ID, "staff", "guest requested refund")
	require.NoError(t, err)
	require.Equal(t, BillStatusOpen, refunded.Status)
	require.Equal(t, int64(0), refunded.PaidAmount)

	var reloaded Bill
	require.NoError(t, GetDB().First(&reloaded, bill.ID).Error)
	require.Equal(t, BillStatusOpen, reloaded.Status)
	require.Equal(t, int64(0), reloaded.PaidAmount)
	require.Equal(t, int64(1000), reloaded.TotalAmount)
}

func TestRefundedBillStatusUpdates_TerminalStatusesStayPut(t *testing.T) {
	for _, status := range []BillStatus{BillStatusClosed, BillStatusPaid, BillStatusVoided} {
		bill := &Bill{Status: status, TotalAmount: 1000}
		require.Empty(t, refundedBillStatusUpdates(bill, 0), "a refund must not reopen a %s bill", status)
	}
	require.Equal(t, BillStatusOpen, refundedBillStatusUpdates(&Bill{Status: BillStatusPartial, TotalAmount: 1000}, 0)["status"])
}
