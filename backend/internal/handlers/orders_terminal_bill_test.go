package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestCreateOrder_RejectsBillClosedAfterPreCheck leaves the bill open for the
// handler's first read, then closes it on the in-transaction FOR UPDATE read.
// SQLite renders that lock as a no-op, but the clause is still on the statement.
func TestCreateOrder_RejectsBillClosedAfterPreCheck(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)
	createTestMenu(t, biz.ID)

	const callbackName = "test:close_bill_on_lock"
	fired := false
	require.NoError(t, database.GetDB().Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if fired || !queryLocksBills(tx) {
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
			"UPDATE bills SET status = ? WHERE id = ?", database.BillStatusClosed, bill.ID)
		if err != nil {
			_ = tx.AddError(err)
		}
	}))
	t.Cleanup(func() {
		database.GetDB().Callback().Query().Remove(callbackName)
	})

	w := postStaffOrder(t, biz.ID, bill.ID)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Bill is not open for new orders")
	require.True(t, fired, "locked bill read did not run")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Where("bill_id = ?", bill.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestCreateOrder_AcceptsPartialBill(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)
	createTestMenu(t, biz.ID)
	require.NoError(t, database.GetDB().Model(bill).Update("status", database.BillStatusPartial).Error)

	w := postStaffOrder(t, biz.ID, bill.ID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Where("bill_id = ?", bill.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func postStaffOrder(t *testing.T, businessID, billID uint) *httptest.ResponseRecorder {
	t.Helper()
	body := CreateOrderRequest{
		BillID: billID,
		Items: []CreateOrderItemRequest{
			{MenuItemName: "Burger", MenuItemID: "burger", Quantity: 1, Price: 10},
		},
	}
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_email", "staff@example.test")

	NewOrderHandler(nil).CreateOrder(c)
	return w
}

func queryLocksBills(tx *gorm.DB) bool {
	if tx == nil || tx.Statement == nil || tx.Statement.Schema == nil {
		return false
	}
	if tx.Statement.Schema.Table != "bills" {
		return false
	}
	locking, ok := tx.Statement.Clauses["FOR"]
	if !ok {
		return false
	}
	lock, ok := locking.Expression.(clause.Locking)
	return ok && lock.Strength == "UPDATE"
}
