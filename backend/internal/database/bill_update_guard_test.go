package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdateBillRejectsNonOpenBill(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 10)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{"status": BillStatusPaid, "paid_amount": 1000}).Error)

	stale := *bill // in-memory copy still says status=open
	err := UpdateBill(&stale, []BillItem{})
	require.ErrorIs(t, err, ErrBillNotPayable,
		"editing items on a paid bill must be rejected inside the transaction")
}

func TestUpdateBillDoesNotClobberPaymentColumns(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 10)
	// A payment lands concurrently after our handler loaded the bill.
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).
		Update("paid_amount", 400).Error)

	stale := *bill // paid_amount is 0 in this stale struct
	stale.Subtotal = 1200
	stale.TotalAmount = 1200
	require.NoError(t, UpdateBill(&stale, []BillItem{}))

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.EqualValues(t, 400, reloaded.PaidAmount,
		"item update must not overwrite concurrently recorded payments")
	require.EqualValues(t, 1200, reloaded.Subtotal, "intended columns must still be written")
}
