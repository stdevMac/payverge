package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupCancelTestDB initialises an in-memory SQLite database for cancel
// handler tests and migrates the minimum schema.
func setupCancelTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.StaffInvitation{},
		&database.InventorySettings{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.InventoryMovement{},
		&database.Table{},
		&database.Bill{},
		&database.BillHistoryEvent{},
		&database.Order{},
		&database.DeliveryOrder{},
		&database.DeliveryStatusHistory{},
		&database.DeliveryDriver{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.BusinessAlertSettings{},
	))

	// BillItem uses a TEXT primary key which SQLite cannot handle via GORM
	// AutoMigrate; create the table manually to match what the DB layer expects.
	gormDB.Exec("DROP TABLE IF EXISTS bill_items")
	require.NoError(t, gormDB.Exec(`
		CREATE TABLE bill_items (
			id TEXT PRIMARY KEY,
			bill_id INTEGER NOT NULL,
			menu_item_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			price REAL NOT NULL,
			quantity INTEGER NOT NULL,
			options TEXT,
			item_type TEXT DEFAULT 'menu_item',
			bundle_id INTEGER,
			parent_bundle_id INTEGER,
			source_offer_id INTEGER,
			order_id INTEGER,
			subtotal REAL NOT NULL,
			created_at DATETIME
		)
	`).Error)
	InitializeRBAC(database.GetDBWrapper())
	return gormDB
}

// createCancelTestOrder inserts a minimal Order with the given status.
func createCancelTestOrder(t *testing.T, businessID, billID uint, status database.OrderStatus) *database.Order {
	t.Helper()
	order := &database.Order{
		BusinessID:  businessID,
		BillID:      billID,
		OrderNumber: fmt.Sprintf("O%d-%d", businessID, time.Now().UnixNano()%100000000),
		Status:      status,
		Items:       "[]",
		CreatedBy:   "guest",
	}
	require.NoError(t, database.GetDB().Create(order).Error)
	return order
}

// createCancelTestBill inserts a minimal open Bill.
func createCancelTestBill(t *testing.T, businessID, tableID uint) *database.Bill {
	t.Helper()
	bill := &database.Bill{
		BusinessID:     businessID,
		TableID:        tableID,
		BillNumber:     fmt.Sprintf("B-%d-%d", businessID, time.Now().UnixNano()),
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return bill
}

// ─── CancelOrder (operator) ─────────────────────────────────────────────────

func TestCancelOrder_RequiresReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerReq", "Reason Required Biz")
	table := &database.Table{BusinessID: business.ID, Name: "T1", TableCode: "tc-req-1", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)
	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusPending)

	// No reason field in body.
	body, _ := json.Marshal(map[string]any{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xOwnerReq")
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CancelOrder(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCancelOrder_CannotCancelDeliveredOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerDel", "Delivered Biz")
	table := &database.Table{BusinessID: business.ID, Name: "T2", TableCode: "tc-del-1", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)
	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusOrderDelivered)

	body, _ := json.Marshal(map[string]any{"reason": "too late"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xOwnerDel")
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CancelOrder(c)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "delivered")
}

func TestCancelOrder_CannotCancelAlreadyCancelledOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerCan", "Already Cancelled Biz")
	table := &database.Table{BusinessID: business.ID, Name: "T3", TableCode: "tc-can-1", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)
	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusOrderCancelled)

	body, _ := json.Marshal(map[string]any{"reason": "again"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xOwnerCan")
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CancelOrder(c)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "already cancelled")
}

func TestCancelOrder_SucceedsForPendingOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerPend", "Pending Cancel Biz")
	table := &database.Table{BusinessID: business.ID, Name: "T4", TableCode: "tc-pend-1", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)
	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusPending)

	body, _ := json.Marshal(map[string]any{"reason": "customer changed mind"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xOwnerPend")
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CancelOrder(c)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify DB state.
	var updated database.Order
	require.NoError(t, database.GetDB().First(&updated, order.ID).Error)
	assert.Equal(t, database.OrderStatusOrderCancelled, updated.Status)
	assert.Equal(t, "customer changed mind", updated.CancelReason)
	assert.NotNil(t, updated.CancelledAt)
}

func TestCancelOrder_SucceedsForApprovedOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerAppr", "Approved Cancel Biz")
	table := &database.Table{BusinessID: business.ID, Name: "T5", TableCode: "tc-appr-1", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)
	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusApproved)

	body, _ := json.Marshal(map[string]any{"reason": "item unavailable"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xOwnerAppr")
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CancelOrder(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCancelOrder_RejectsCrossBusinessOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	ownerBiz := createOwnedBusiness(t, "0xOwnerX", "Owner Biz X")
	otherBiz := createOwnedBusiness(t, "0xOwnerY", "Owner Biz Y")
	table := &database.Table{BusinessID: otherBiz.ID, Name: "T6", TableCode: "tc-cross-1", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, otherBiz.ID, table.ID)
	order := createCancelTestOrder(t, otherBiz.ID, bill.ID, database.OrderStatusPending)

	body, _ := json.Marshal(map[string]any{"reason": "cross-biz attempt"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xOwnerX")
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", ownerBiz.ID)},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CancelOrder(c)

	// Order belongs to otherBiz but request is for ownerBiz → 404.
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ─── GuestCancelOrder ───────────────────────────────────────────────────────

func TestGuestCancelOrder_OnlyAllowedWhenPending(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerGst", "Guest Cancel Biz")
	table := &database.Table{BusinessID: business.ID, Name: "GT1", TableCode: "gc-table-1", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)

	cases := []struct {
		status    database.OrderStatus
		wantCode  int
		wantWire  string
		wantError string
	}{
		{database.OrderStatusApproved, http.StatusConflict, "order_already_accepted", "already been accepted"},
		{database.OrderStatusInKitchen, http.StatusConflict, "order_already_accepted", "already been accepted"},
		{database.OrderStatusOrderReady, http.StatusConflict, "order_already_accepted", "already been accepted"},
		{database.OrderStatusOrderDelivered, http.StatusConflict, "order_already_accepted", "already been accepted"},
		{database.OrderStatusOrderCancelled, http.StatusConflict, "order_already_cancelled", "already been cancelled"},
	}

	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			order := createCancelTestOrder(t, business.ID, bill.ID, tc.status)

			body, _ := json.Marshal(map[string]any{"reason": "guest cancel attempt"})
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{
				{Key: "code", Value: table.TableCode},
				{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
			}
			c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			GuestCancelOrder(c)

			assert.Equal(t, tc.wantCode, w.Code, "status=%s", tc.status)
			assert.Contains(t, w.Body.String(), tc.wantError)
			assert.Contains(t, w.Body.String(), tc.wantWire)
		})
	}
}

func TestGuestCancelOrder_SucceedsForPendingOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerGstOK", "Guest OK Biz")
	table := &database.Table{BusinessID: business.ID, Name: "GT2", TableCode: "gc-table-ok", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)
	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusPending)

	body, _ := json.Marshal(map[string]any{"reason": "changed my mind"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "code", Value: table.TableCode},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	GuestCancelOrder(c)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify DB state.
	var updated database.Order
	require.NoError(t, database.GetDB().First(&updated, order.ID).Error)
	assert.Equal(t, database.OrderStatusOrderCancelled, updated.Status)
	assert.Equal(t, "guest", updated.CancelledBy)
	assert.Equal(t, "changed my mind", updated.CancelReason)
	assert.NotNil(t, updated.CancelledAt)
}

// FIND-045: guest atomic checkout puts items on the bill immediately. Cancel
// of a still-pending guest order must strip those items and recompute totals
// so the guest is not charged for a cancelled order.
func TestGuestCancelOrder_RemovesAlreadyBilledPendingItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerGstBill", "Guest Bill Biz")
	business.TaxRate = 10
	business.ServiceFeeRate = 4
	require.NoError(t, database.GetDB().Save(business).Error)

	table := &database.Table{BusinessID: business.ID, Name: "GT Bill", TableCode: "gc-table-bill", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := createCancelTestBill(t, business.ID, table.ID)
	// Simulate guest checkout totals: $5.00 item → 500 cents + tax/service.
	bill.Subtotal = 500
	bill.TaxAmount = 50
	bill.ServiceFeeAmount = 20
	bill.TotalAmount = 570
	bill.Items = `[{"id":"item-tea-1","bill_id":0,"menu_item_id":"demo-tea","name":"Iced Tea","price":5,"quantity":1,"item_type":"menu_item","subtotal":5}]`
	require.NoError(t, database.GetDB().Save(bill).Error)

	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusPending)
	// Stamp relational bill_items with order_id the way GuestCheckout does.
	require.NoError(t, database.GetDB().Exec(`
		INSERT INTO bill_items (id, bill_id, menu_item_id, name, price, quantity, item_type, order_id, subtotal, created_at)
		VALUES (?, ?, 'demo-tea', 'Iced Tea', 5, 1, 'menu_item', ?, 5, CURRENT_TIMESTAMP)
	`, "item-tea-1", bill.ID, order.ID).Error)

	// Sibling order item on same bill must survive cancel of `order`.
	sibling := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusPending)
	require.NoError(t, database.GetDB().Exec(`
		INSERT INTO bill_items (id, bill_id, menu_item_id, name, price, quantity, item_type, order_id, subtotal, created_at)
		VALUES (?, ?, 'demo-bowl', 'Harvest Bowl', 18.5, 1, 'menu_item', ?, 18.5, CURRENT_TIMESTAMP)
	`, "item-bowl-1", bill.ID, sibling.ID).Error)
	// Recompute bill to include both lines (guest path would have summed).
	bill.Subtotal = 500 + 1850
	bill.TaxAmount = 235
	bill.ServiceFeeAmount = 94
	bill.TotalAmount = 500 + 1850 + 235 + 94
	bill.Items = `[{"id":"item-tea-1","menu_item_id":"demo-tea","name":"Iced Tea","price":5,"quantity":1,"item_type":"menu_item","subtotal":5,"order_id":` + fmt.Sprintf("%d", order.ID) + `},{"id":"item-bowl-1","menu_item_id":"demo-bowl","name":"Harvest Bowl","price":18.5,"quantity":1,"item_type":"menu_item","subtotal":18.5,"order_id":` + fmt.Sprintf("%d", sibling.ID) + `}]`
	require.NoError(t, database.GetDB().Save(bill).Error)

	body, _ := json.Marshal(map[string]any{"reason": "changed mind"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "code", Value: table.TableCode},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	GuestCancelOrder(c)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var remaining []database.BillItem
	require.NoError(t, database.GetDB().Where("bill_id = ?", bill.ID).Find(&remaining).Error)
	require.Len(t, remaining, 1, "only sibling order item should remain")
	assert.Equal(t, "item-bowl-1", remaining[0].ID)
	assert.Equal(t, sibling.ID, *remaining[0].OrderID)

	var saved database.Bill
	require.NoError(t, database.GetDB().First(&saved, bill.ID).Error)
	// Bowl 1850c + tax 10% + service 4%
	assert.Equal(t, int64(1850), saved.Subtotal)
	assert.Equal(t, int64(185), saved.TaxAmount)
	assert.Equal(t, int64(74), saved.ServiceFeeAmount)
	assert.Equal(t, int64(1850+185+74), saved.TotalAmount)

	// Tea row gone from relational store.
	var teaCount int64
	require.NoError(t, database.GetDB().Model(&database.BillItem{}).
		Where("bill_id = ? AND order_id = ?", bill.ID, order.ID).Count(&teaCount).Error)
	assert.Equal(t, int64(0), teaCount)
}

func TestCancelPendingGuestOrderReturnsFalseWhenOrderNoLongerPending(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerGstRace", "Guest Race Biz")
	table := &database.Table{BusinessID: business.ID, Name: "GT Race", TableCode: "gc-table-race", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)
	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusApproved)

	now := time.Now().UTC()
	updated, err := database.CancelPendingGuestOrder(order, "too late", now)

	require.NoError(t, err)
	assert.False(t, updated)

	var persisted database.Order
	require.NoError(t, database.GetDB().First(&persisted, order.ID).Error)
	assert.Equal(t, database.OrderStatusApproved, persisted.Status)
	assert.Empty(t, persisted.CancelledBy)
	assert.Nil(t, persisted.CancelledAt)
}

func TestGuestCancelOrder_NoReasonIsAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerGstNoR", "Guest No Reason Biz")
	table := &database.Table{BusinessID: business.ID, Name: "GT3", TableCode: "gc-table-noreason", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)
	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusPending)

	// Body with no reason field.
	body, _ := json.Marshal(map[string]any{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "code", Value: table.TableCode},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	GuestCancelOrder(c)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGuestCancelOrder_RejectsCrossBusinessOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	bizA := createOwnedBusiness(t, "0xOwnerA2", "Biz A2")
	bizB := createOwnedBusiness(t, "0xOwnerB2", "Biz B2")
	tableA := &database.Table{BusinessID: bizA.ID, Name: "TA", TableCode: "gc-table-a2", IsActive: true}
	require.NoError(t, database.GetDB().Create(tableA).Error)
	tableB := &database.Table{BusinessID: bizB.ID, Name: "TB", TableCode: "gc-table-b2", IsActive: true}
	require.NoError(t, database.GetDB().Create(tableB).Error)
	billB := createCancelTestBill(t, bizB.ID, tableB.ID)
	orderB := createCancelTestOrder(t, bizB.ID, billB.ID, database.OrderStatusPending)

	// Guest uses table code for bizA but orderId belongs to bizB.
	body, _ := json.Marshal(map[string]any{"reason": "oops"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "code", Value: tableA.TableCode},
		{Key: "orderId", Value: fmt.Sprintf("%d", orderB.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	GuestCancelOrder(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGuestCancelOrder_RejectsSameBusinessDifferentTableOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerSameBiz", "Guest Same Biz")
	tableA := &database.Table{BusinessID: business.ID, Name: "TA", TableCode: "gc-same-biz-a", IsActive: true}
	require.NoError(t, database.GetDB().Create(tableA).Error)
	tableB := &database.Table{BusinessID: business.ID, Name: "TB", TableCode: "gc-same-biz-b", IsActive: true}
	require.NoError(t, database.GetDB().Create(tableB).Error)
	billB := createCancelTestBill(t, business.ID, tableB.ID)
	orderB := createCancelTestOrder(t, business.ID, billB.ID, database.OrderStatusPending)

	body, _ := json.Marshal(map[string]any{"reason": "wrong table"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "code", Value: tableA.TableCode},
		{Key: "orderId", Value: fmt.Sprintf("%d", orderB.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	GuestCancelOrder(c)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var updated database.Order
	require.NoError(t, database.GetDB().First(&updated, orderB.ID).Error)
	assert.Equal(t, database.OrderStatusPending, updated.Status)
}

// ─── resolveCancelActor ─────────────────────────────────────────────────────

func TestResolveCancelActor_Staff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("staff_id", uint(42))

	actor := resolveCancelActor(c)
	assert.Equal(t, "staff:42", actor)
}

func TestResolveCancelActor_Owner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xABCDEF")

	actor := resolveCancelActor(c)
	assert.Equal(t, "owner:0xABCDEF", actor)
}

func TestResolveCancelActor_UserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", uint(7))

	actor := resolveCancelActor(c)
	assert.Equal(t, "user:7", actor)
}

func TestResolveCancelActor_Fallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	actor := resolveCancelActor(c)
	assert.Equal(t, "system", actor)
}

func TestCancelOrder_DeliveryLinkedRoutesThroughLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)
	SetDeliveryService(services.NewDeliveryService(database.GetDB(), nil))
	t.Cleanup(func() { SetDeliveryService(nil) })

	business := createOwnedBusiness(t, "0xOwnerDlv", "Delivery Cancel Biz")
	table := &database.Table{BusinessID: business.ID, Name: "TD", TableCode: "tc-dlv-1", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)
	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusPending)

	delivery := &database.DeliveryOrder{
		BusinessID:     business.ID,
		BillID:         bill.ID,
		OrderID:        &order.ID,
		DeliveryNumber: fmt.Sprintf("DEL-%d", time.Now().UnixNano()),
		DeliveryType:   database.DeliveryTypeInHouse,
		Status:         database.DeliveryStatusPending,
		CustomerName:   "Guest",
		CustomerPhone:  "555",
	}
	require.NoError(t, database.GetDB().Create(delivery).Error)

	body, _ := json.Marshal(map[string]any{"reason": "out of stock"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xOwnerDlv")
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CancelOrder(c)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var freshDelivery database.DeliveryOrder
	require.NoError(t, database.GetDB().First(&freshDelivery, delivery.ID).Error)
	assert.Equal(t, database.DeliveryStatusCancelled, freshDelivery.Status, "delivery leg must be cancelled, not left live")
	assert.Equal(t, "out of stock", freshDelivery.CancellationReason)

	var freshOrder database.Order
	require.NoError(t, database.GetDB().First(&freshOrder, order.ID).Error)
	assert.Equal(t, database.OrderStatusOrderCancelled, freshOrder.Status)

	var freshBill database.Bill
	require.NoError(t, database.GetDB().First(&freshBill, bill.ID).Error)
	assert.Contains(t, []database.BillStatus{database.BillStatusClosed, database.BillStatusVoided, database.BillStatusAbandoned}, freshBill.Status, "unpaid delivery bill is terminal after the rejection")
}

// TestPublishOrderCancelledEvent_CarriesBothIDKeys is the X-7 contract test:
// both order.cancelled emitters (this one and the delivery lifecycle's) must
// carry BOTH `order_id` and `id` plus bill_id/status/reason/cancelled_by, so
// subscribers keying on either shape see every cancellation.
func TestPublishOrderCancelledEvent_CarriesBothIDKeys(t *testing.T) {
	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(999, 0)
	defer cancel()

	publishOrderCancelledEvent(999, 123, 456, "out of stock", "staff:9")

	select {
	case ev := <-ch:
		require.Equal(t, "order.cancelled", ev.Type)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(ev.Data, &payload))
		assert.Equal(t, float64(123), payload["order_id"])
		assert.Equal(t, float64(123), payload["id"])
		assert.Equal(t, float64(456), payload["bill_id"])
		assert.Equal(t, "cancelled", payload["status"])
		assert.Equal(t, "out of stock", payload["reason"])
		assert.Equal(t, "staff:9", payload["cancelled_by"])
	case <-time.After(2 * time.Second):
		t.Fatal("order.cancelled event not received")
	}
}

// helper: open needs-approval alert for an order.
func createOrderNewAlert(t *testing.T, businessID uint, orderID uint) *database.OperationalAlert {
	t.Helper()
	alert := &database.OperationalAlert{
		BusinessID:   businessID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   int64(orderID),
		Status:       database.OperationalAlertStatusOpen,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "New order",
	}
	require.NoError(t, database.GetDB().Create(alert).Error)
	return alert
}

func TestGuestCancelOrder_WritesBillHistoryAndResolvesAlert(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerGstHyg", "Guest Hygiene Biz")
	table := &database.Table{BusinessID: business.ID, Name: "GT-H", TableCode: "gc-table-hyg", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)
	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusPending)
	alert := createOrderNewAlert(t, business.ID, order.ID)

	body, _ := json.Marshal(map[string]any{"reason": "changed my mind"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "code", Value: table.TableCode},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	GuestCancelOrder(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// X-5: guest cancellation is visible in the bill audit trail.
	var events []database.BillHistoryEvent
	require.NoError(t, database.GetDB().Where("bill_id = ?", bill.ID).Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, database.BillHistoryEventOrderCanceled, events[0].EventType)
	assert.Equal(t, "guest", events[0].Actor)
	require.NotNil(t, events[0].OrderID)
	assert.Equal(t, order.ID, *events[0].OrderID)

	// B-5: needs-approval alert resolved.
	var updatedAlert database.OperationalAlert
	require.NoError(t, database.GetDB().First(&updatedAlert, alert.ID).Error)
	assert.Equal(t, database.OperationalAlertStatusResolved, updatedAlert.Status)
}

func TestCancelOrder_ResolvesNeedsApprovalAlert(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCancelTestDB(t)

	owner := "0xOwnerOpHyg"
	business := createOwnedBusiness(t, owner, "Operator Hygiene Biz")
	table := &database.Table{BusinessID: business.ID, Name: "GT-OH", TableCode: "gc-table-ophyg", IsActive: true}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := createCancelTestBill(t, business.ID, table.ID)
	order := createCancelTestOrder(t, business.ID, bill.ID, database.OrderStatusPending)
	alert := createOrderNewAlert(t, business.ID, order.ID)

	body, _ := json.Marshal(map[string]any{"reason": "customer left"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", owner)

	CancelOrder(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updatedAlert database.OperationalAlert
	require.NoError(t, database.GetDB().First(&updatedAlert, alert.ID).Error)
	assert.Equal(t, database.OperationalAlertStatusResolved, updatedAlert.Status)
}
