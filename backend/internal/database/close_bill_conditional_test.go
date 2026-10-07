package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestCloseBillWithHistory_RejectsStalePaidAmount races a payment between the
// locked read and the close UPDATE. The callback changes paid_amount on the
// open transaction connection before the close SQL runs, so the UPDATE's
// paid_amount predicate no longer matches.
func TestCloseBillWithHistory_RejectsStalePaidAmount(t *testing.T) {
	setupOrderTestDB(t)
	require.NoError(t, GetDB().AutoMigrate(&AlternativePayment{}, &BillSplitShare{}))

	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 25)
	require.NoError(t, GetDB().Model(bill).Update("paid_amount", bill.TotalAmount).Error)
	bill.PaidAmount = bill.TotalAmount
	require.Greater(t, bill.TotalAmount, int64(0))
	require.Equal(t, bill.TotalAmount, bill.PaidAmount)

	const callbackName = "test:concurrent_payment"
	fired := false
	require.NoError(t, GetDB().Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if fired || !isBillCloseUpdate(tx) {
			return
		}
		fired = true
		if tx.Statement.ConnPool == nil {
			_ = tx.AddError(gorm.ErrInvalidDB)
			return
		}
		ctx := tx.Statement.Context
		if ctx == nil {
			ctx = context.Background()
		}
		_, err := tx.Statement.ConnPool.ExecContext(ctx,
			"UPDATE bills SET paid_amount = paid_amount - 100 WHERE id = ?", bill.ID)
		if err != nil {
			_ = tx.AddError(err)
		}
	}))
	t.Cleanup(func() {
		GetDB().Callback().Update().Remove(callbackName)
	})

	err := CloseBillWithHistory(bill.ID, nil)
	require.ErrorIs(t, err, ErrBillNotOpen)
	require.True(t, fired, "close UPDATE callback did not run")

	var reloaded Bill
	require.NoError(t, GetDB().First(&reloaded, bill.ID).Error)
	require.Equal(t, BillStatusOpen, reloaded.Status)
}

func isBillCloseUpdate(tx *gorm.DB) bool {
	if tx == nil || tx.Statement == nil {
		return false
	}
	table := tx.Statement.Table
	if tx.Statement.Schema != nil && tx.Statement.Schema.Table != "" {
		table = tx.Statement.Schema.Table
	}
	if table != "bills" {
		return false
	}
	updates, ok := tx.Statement.Dest.(map[string]interface{})
	if !ok {
		return false
	}
	_, hasStatus := updates["status"]
	return hasStatus
}
