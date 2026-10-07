package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func init() {
	gin.SetMode(gin.TestMode)
	// Ensure SSE hub is initialized for tests
	events.GetHub()
}

// setupHandlerTestDB creates an in-memory SQLite DB for handler tests.
func setupHandlerTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	server.InitializeRBAC(database.GetDBWrapper())

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.InventorySettings{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.InventoryMovement{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.BusinessMilestoneEvent{},
		&database.BusinessRevenueAggregate{},
		&database.WebhookEvent{},
		&database.BillHistoryEvent{},
		&database.Order{},
		&database.DeliveryOrder{},
		&database.DeliveryDriver{},
		&database.DeliveryStatusHistory{},
		&database.Customer{},
		&database.CustomerBusiness{},
		&database.CustomerVisit{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.BusinessAlertSettings{},
	))

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
}

func createTestBusiness(t *testing.T) *database.Business {
	t.Helper()
	db := database.GetDB()
	biz := &database.Business{
		BusinessId: fmt.Sprintf("biz-%d", time.Now().UnixNano()),
		Name:       "Test Restaurant",
	}
	require.NoError(t, db.Create(biz).Error)
	return biz
}

func createTestBill(t *testing.T, businessID uint) *database.Bill {
	t.Helper()
	db := database.GetDB()
	bill := &database.Bill{
		BusinessID: businessID,
		BillNumber: "B-test-" + time.Now().Format("150405.000000"),
		Status:     database.BillStatusOpen,
		Items:      "[]",
	}
	require.NoError(t, db.Create(bill).Error)
	return bill
}

func createTestMenu(t *testing.T, businessID uint, items ...database.MenuItem) *database.Menu {
	t.Helper()

	if len(items) == 0 {
		items = []database.MenuItem{
			{ID: "burger", Name: "Burger", Price: 10, IsAvailable: true},
			{ID: "fries", Name: "Fries", Price: 5, IsAvailable: true},
		}
	}

	categories := []database.MenuCategory{
		{
			ID:    "mains",
			Name:  "Mains",
			Items: items,
		},
	}
	payload, err := json.Marshal(categories)
	require.NoError(t, err)

	menu := &database.Menu{
		BusinessID: businessID,
		Categories: string(payload),
		IsActive:   true,
		Version:    1,
	}
	require.NoError(t, database.GetDB().Create(menu).Error)
	return menu
}

// ─── CreateOrder Tests ───

func TestCreateOrder_Success(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)
	createTestMenu(t, biz.ID)

	body := CreateOrderRequest{
		BillID: bill.ID,
		Notes:  "No onions",
		Items: []CreateOrderItemRequest{
			{MenuItemName: "Burger", Quantity: 2, Price: 10.0},
			{MenuItemName: "Fries", Quantity: 1, Price: 5.0},
		},
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	oh := NewOrderHandler(nil)
	oh.CreateOrder(c)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp OrderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, bill.ID, resp.Order.BillID)
	assert.Equal(t, biz.ID, resp.Order.BusinessID)
	assert.Equal(t, database.OrderStatusPending, resp.Order.Status)
	assert.Equal(t, "No onions", resp.Order.Notes)

	var alert database.OperationalAlert
	require.NoError(t, database.GetDB().
		Where("business_id = ? AND alert_type = ? AND resource_type = ? AND resource_id = ?", biz.ID, database.OperationalAlertTypeOrderNew, database.OperationalAlertResourceTypeOrder, resp.Order.ID).
		First(&alert).Error)
	assert.Equal(t, database.OperationalAlertStatusOpen, alert.Status)
}

func TestCreateOrder_EnqueuesTelegramNotification(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
	))
	biz := createTestBusiness(t)
	telegramPlugin := &database.Plugin{
		Name:        "telegram",
		DisplayName: "Telegram Notifications",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(telegramPlugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(biz.ID, telegramPlugin.ID, map[string]interface{}{
		"is_connected": true,
		"chat_id":      "123456789",
		"notification_settings": map[string]interface{}{
			"order_notifications": true,
		},
		"notifications": map[string]interface{}{
			"order_created": true,
		},
	}))
	bill := createTestBill(t, biz.ID)
	createTestMenu(t, biz.ID)

	body := CreateOrderRequest{
		BillID: bill.ID,
		Notes:  "No onions",
		Items: []CreateOrderItemRequest{
			{MenuItemName: "Burger", Quantity: 2, Price: 10.0},
		},
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	NewOrderHandler(nil).CreateOrder(c)

	require.Equal(t, http.StatusCreated, w.Code)
	var delivery database.PluginNotificationDelivery
	require.NoError(t, database.GetDB().Where("business_id = ? AND plugin_name = ? AND event_type = ?", biz.ID, "telegram", services.PluginEventOrderCreated).First(&delivery).Error)
	assert.Equal(t, "telegram", delivery.PluginName)
	assert.Equal(t, services.PluginEventOrderCreated, delivery.EventType)
	assert.Contains(t, delivery.EventID, "order:")
	assert.Equal(t, "No onions", delivery.Payload["notes"])
	assert.Equal(t, bill.ID, uint(delivery.Payload["bill_id"].(float64)))
}

// TestCreateOrder_TelegramNotificationTotalIncludesTaxAndServiceFee is the
// #571 staff-path regression, mirroring
// TestEnqueueDeliveryTelegramOrderCreatedUsesBillTotalCents in
// internal/services/delivery_telegram_outbox_test.go. It exercises the full
// CreateOrder handler (not just the builder) so the tx-ordering finding is
// covered too: bill.TotalAmount is deliberately pre-set to a value distinct
// from both the items-only sum AND the correctly-computed total, to prove
// (a) the payload total is not the items-only subtotal and (b) it is not the
// bill's pre-existing (pre-approval) TotalAmount either — a staff-created
// order's items are only folded into the bill at approval time
// (database.updateOrderStatus), never at creation, so echoing bill.TotalAmount
// here would report a stale figure that excludes the very order being
// announced.
func TestCreateOrder_TelegramNotificationTotalIncludesTaxAndServiceFee(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
	))
	db := database.GetDB()

	biz := createTestBusiness(t)
	biz.TaxRate = 10
	biz.ServiceFeeRate = 5
	require.NoError(t, db.Save(biz).Error)

	telegramPlugin := &database.Plugin{
		Name:        "telegram",
		DisplayName: "Telegram Notifications",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, db.Create(telegramPlugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(biz.ID, telegramPlugin.ID, map[string]interface{}{
		"is_connected": true,
		"chat_id":      "123456789",
		"notification_settings": map[string]interface{}{
			"order_notifications": true,
		},
		"notifications": map[string]interface{}{
			"order_created": true,
		},
	}))

	// Simulate a bill that already carries a prior approved order's total.
	// This figure must stay untouched by CreateOrder and must never leak into
	// the new order's own notification total.
	bill := &database.Bill{
		BusinessID:  biz.ID,
		BillNumber:  "B-staff-tx-order",
		Status:      database.BillStatusOpen,
		Subtotal:    5000,
		TotalAmount: 5750,
		Items:       "[]",
	}
	require.NoError(t, db.Create(bill).Error)
	createTestMenu(t, biz.ID)

	body := CreateOrderRequest{
		BillID: bill.ID,
		Items: []CreateOrderItemRequest{
			{MenuItemName: "Burger", Quantity: 2, Price: 10.0},
		},
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	NewOrderHandler(nil).CreateOrder(c)
	require.Equal(t, http.StatusCreated, w.Code)

	var delivery database.PluginNotificationDelivery
	require.NoError(t, db.Where("business_id = ? AND plugin_name = ? AND event_type = ?", biz.ID, "telegram", services.PluginEventOrderCreated).First(&delivery).Error)

	// 2 Burgers @ $10.00 = $20.00 items subtotal; +10% tax ($2.00) + 5% service
	// ($1.00) = $23.00 payable for this order.
	assert.Equal(t, float64(2300), delivery.Payload["total_cents"], "total_cents must be items + tax + service fee")
	assert.NotEqual(t, float64(2000), delivery.Payload["total_cents"], "total_cents must not be the items-only subtotal")
	assert.NotEqual(t, float64(bill.TotalAmount), delivery.Payload["total_cents"], "total_cents must not echo the bill's stale pre-approval total")

	// Tx-ordering finding: order creation never folds items into the bill —
	// only approval does (database.updateOrderStatus). Confirms bill.TotalAmount
	// was never a safe source for this notification's total.
	var reloadedBill database.Bill
	require.NoError(t, db.First(&reloadedBill, bill.ID).Error)
	assert.Equal(t, int64(5750), reloadedBill.TotalAmount, "bill totals are recomputed at order approval, not at order creation")
}

// TestEnqueueTelegramOrderCreatedNotificationTx_UsesItemsPlusTaxAndServiceFee
// unit-tests the builder directly (mirrors
// TestEnqueueDeliveryTelegramOrderCreatedUsesBillTotalCents): a bill whose
// TotalAmount differs from both the items-only sum and the correctly-computed
// order total must not leak into the payload either way.
func TestEnqueueTelegramOrderCreatedNotificationTx_UsesItemsPlusTaxAndServiceFee(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
	))
	db := database.GetDB()

	biz := createTestBusiness(t)
	biz.TaxRate = 10
	biz.ServiceFeeRate = 5
	require.NoError(t, db.Save(biz).Error)

	bill := database.Bill{
		BusinessID:  biz.ID,
		BillNumber:  "B-staff-builder-total",
		Status:      database.BillStatusOpen,
		Subtotal:    5000,
		TotalAmount: 5750, // prior approved orders' total; must not leak here
		Items:       "[]",
	}
	require.NoError(t, db.Create(&bill).Error)

	order := database.Order{BusinessID: biz.ID, BillID: bill.ID, OrderNumber: "O-staff-builder-total", Status: database.OrderStatusPending}
	require.NoError(t, db.Create(&order).Error)

	items := []database.OrderItem{{MenuItemName: "Burger", Quantity: 2, Price: 10.0, Subtotal: 20.00}}

	require.NoError(t, enqueueTelegramOrderCreatedNotificationTx(db, order, bill, biz, items, "staff"))

	var row database.PluginNotificationDelivery
	require.NoError(t, db.Where("event_id = ?", fmt.Sprintf("order:%d", order.ID)).First(&row).Error)

	assert.EqualValues(t, 2, row.Payload["item_count"])
	// items-only subtotal is 2000 cents; +10% tax (200) + 5% service (100) = 2300.
	assert.Equal(t, float64(2300), row.Payload["total_cents"])
	assert.NotEqual(t, float64(2000), row.Payload["total_cents"], "must not be the items-only subtotal")
	assert.NotEqual(t, float64(bill.TotalAmount), row.Payload["total_cents"], "must not echo the bill's stale pre-approval total")
}

func TestCreateOrder_InvalidBusinessID(t *testing.T) {
	setupHandlerTestDB(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	oh := NewOrderHandler(nil)
	oh.CreateOrder(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateOrder_BillNotFound(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	createTestMenu(t, biz.ID)

	body := CreateOrderRequest{
		BillID: 9999,
		Items:  []CreateOrderItemRequest{{MenuItemName: "X", Quantity: 1, Price: 1.0}},
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	oh := NewOrderHandler(nil)
	oh.CreateOrder(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Bill not found")
}

func TestCreateOrder_BillLookupDatabaseError(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().Exec("DROP TABLE bills").Error)

	body := CreateOrderRequest{
		BillID: 1,
		Items:  []CreateOrderItemRequest{{MenuItemName: "X", Quantity: 1, Price: 1.0}},
	}
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	NewOrderHandler(nil).CreateOrder(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Failed to load bill")
	assert.NotContains(t, w.Body.String(), "no such table")
}

func TestPriceOrderInputsByBusinessID_SnapshotFailureIsPricingDataUnavailable(t *testing.T) {
	setupHandlerTestDB(t)
	services.DisablePricingCacheForTest(t)

	_, _, err := services.PriceOrderInputsByBusinessID(999999, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, services.ErrPricingDataUnavailable)
}

func TestCreateOrder_RejectsClosedBill(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)
	createTestMenu(t, biz.ID)
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).Update("status", database.BillStatusClosed).Error)

	body := CreateOrderRequest{
		BillID: bill.ID,
		Items: []CreateOrderItemRequest{
			{MenuItemName: "Burger", Quantity: 1, Price: 10.0},
		},
	}
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	NewOrderHandler(nil).CreateOrder(c)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "Bill is not open for new orders")
}

func TestCreateOrder_MissingFields(t *testing.T) {
	setupHandlerTestDB(t)

	// Missing required items field
	body := `{"bill_id": 1}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")

	oh := NewOrderHandler(nil)
	oh.CreateOrder(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateOrder_UsesAuthoritativeMenuPricing(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)
	createTestMenu(t, biz.ID, database.MenuItem{
		ID:          "burger",
		Name:        "Burger",
		Price:       14,
		IsAvailable: true,
		Options: []database.MenuItemOption{
			{ID: "cheese", Name: "Extra Cheese", PriceChange: 2},
		},
	})

	body := CreateOrderRequest{
		BillID: bill.ID,
		Items: []CreateOrderItemRequest{
			{
				MenuItemName: "Burger",
				MenuItemID:   "burger",
				Quantity:     2,
				Price:        1,
				Options: []database.MenuItemOption{
					{ID: "cheese", Name: "Extra Cheese", PriceChange: -10},
				},
			},
		},
	}
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	NewOrderHandler(nil).CreateOrder(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	order, items, err := database.GetOrderByID(1)
	require.NoError(t, err)
	require.NotNil(t, order)
	require.Len(t, items, 1)
	assert.Equal(t, 14.0, items[0].Price)
	assert.Equal(t, 32.0, items[0].Subtotal)
	require.Len(t, items[0].Options, 1)
	assert.Equal(t, 2.0, items[0].Options[0].PriceChange)
}

func TestCreateOrder_RejectsUnknownMenuItem(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)
	createTestMenu(t, biz.ID)

	body := CreateOrderRequest{
		BillID: bill.ID,
		Items: []CreateOrderItemRequest{
			{MenuItemName: "Secret Item", Quantity: 1, Price: 1},
		},
	}
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	NewOrderHandler(nil).CreateOrder(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "not found")
}

// ─── GetOrders Tests ───

func TestGetOrders_Success(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	for i := 0; i < 3; i++ {
		order := &database.Order{
			BillID:      bill.ID,
			BusinessID:  biz.ID,
			OrderNumber: "O-test",
			Status:      database.OrderStatusPending,
			CreatedBy:   "guest",
			Items:       "[]",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, db.Create(order).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest("GET", "/", nil)

	GetOrders(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp OrdersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 3, len(resp.Orders))
	assert.Equal(t, int64(3), resp.Total)
}

func TestGetOrders_WithStatusFilter(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	statuses := []database.OrderStatus{database.OrderStatusPending, database.OrderStatusApproved, database.OrderStatusPending}
	for _, s := range statuses {
		order := &database.Order{
			BillID:      bill.ID,
			BusinessID:  biz.ID,
			OrderNumber: "O-test",
			Status:      s,
			CreatedBy:   "guest",
			Items:       "[]",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, db.Create(order).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest("GET", "/?status=pending", nil)

	GetOrders(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp OrdersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, len(resp.Orders))
}

func TestGetOrders_Pagination(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	for i := 0; i < 25; i++ {
		order := &database.Order{
			BillID:      bill.ID,
			BusinessID:  biz.ID,
			OrderNumber: "O-test",
			Status:      database.OrderStatusPending,
			CreatedBy:   "guest",
			Items:       "[]",
			CreatedAt:   time.Now().Add(time.Duration(i) * time.Second),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, db.Create(order).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest("GET", "/?page=1&page_size=10", nil)

	GetOrders(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp PaginatedOrdersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 10, len(resp.Orders))
	assert.Equal(t, int64(25), resp.Total)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 10, resp.PageSize)
	assert.Equal(t, 3, resp.TotalPages)
}

func TestGetOrders_DefaultsToPaginatedResponse(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	for i := 0; i < 25; i++ {
		order := &database.Order{
			BillID:      bill.ID,
			BusinessID:  biz.ID,
			OrderNumber: fmt.Sprintf("O-default-page-%02d", i),
			Status:      database.OrderStatusPending,
			CreatedBy:   "guest",
			Items:       "[]",
			CreatedAt:   time.Now().Add(time.Duration(i) * time.Second),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, db.Create(order).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest("GET", "/", nil)

	GetOrders(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp PaginatedOrdersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 20, len(resp.Orders))
	assert.Equal(t, int64(25), resp.Total)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 20, resp.PageSize)
	assert.Equal(t, 2, resp.TotalPages)
}

// ─── GetOrder Tests ───

func TestGetOrder_Success(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "O-test",
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       `[{"id":"i1","menu_item_name":"Burger","quantity":1,"price":10,"subtotal":10}]`,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(order).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "1"},
		{Key: "orderId", Value: "1"},
	}
	c.Request = httptest.NewRequest("GET", "/", nil)

	GetOrder(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp OrderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, order.ID, resp.Order.ID)
}

func TestGetOrder_WrongBusiness(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "O-test",
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       "[]",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(order).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "9999"}, // Wrong business
		{Key: "orderId", Value: "1"},
	}
	c.Request = httptest.NewRequest("GET", "/", nil)

	GetOrder(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ─── UpdateOrderStatus Tests ───

func TestUpdateOrderStatus_Approve(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	items, _ := json.Marshal([]database.OrderItem{
		{ID: "i1", MenuItemName: "Burger", Price: 10, Quantity: 1, Subtotal: 10},
	})
	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "O-test",
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       string(items),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(order).Error)

	body, _ := json.Marshal(UpdateOrderStatusRequest{
		Status: database.OrderStatusApproved,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "1"},
		{Key: "orderId", Value: "1"},
	}
	c.Request = httptest.NewRequest("PUT", "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_email", "staff-1@example.com")

	UpdateOrderStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp OrderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, database.OrderStatusApproved, resp.Order.Status)
	assert.Equal(t, "staff-1@example.com", resp.Order.ApprovedBy)
}

func TestUpdateOrderStatus_DoesNotEnqueueStatusNotification(t *testing.T) {
	// order.status_changed has no Telegram renderer (it would permanently fail
	// to render), so the status-change path must not enqueue a Telegram
	// delivery at all — even for a connected business with the toggle on.
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
	))
	biz := createTestBusiness(t)
	telegramPlugin := &database.Plugin{
		Name:        "telegram",
		DisplayName: "Telegram Notifications",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(telegramPlugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(biz.ID, telegramPlugin.ID, map[string]interface{}{
		"is_connected": true,
		"chat_id":      "123456789",
		"notification_settings": map[string]interface{}{
			"order_notifications": true,
		},
		"notifications": map[string]interface{}{
			"order_status_changed": true,
		},
	}))
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	items, _ := json.Marshal([]database.OrderItem{
		{ID: "i1", MenuItemName: "Burger", Price: 10, Quantity: 1, Subtotal: 10},
	})
	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "O-test",
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       string(items),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(order).Error)

	body, _ := json.Marshal(UpdateOrderStatusRequest{Status: database.OrderStatusApproved})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", biz.ID)},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest("PUT", "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_email", "staff-1@example.com")

	UpdateOrderStatus(c)

	require.Equal(t, http.StatusOK, w.Code)
	var count int64
	require.NoError(t, database.GetDB().Model(&database.PluginNotificationDelivery{}).
		Where("business_id = ? AND plugin_name = ? AND event_type = ?", biz.ID, "telegram", services.PluginEventOrderStatusChanged).
		Count(&count).Error)
	assert.Equal(t, int64(0), count, "order.status_changed must not be enqueued for Telegram")
}

func TestUpdateOrderStatus_Cancel(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "O-test",
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       `[{"id":"i1","menu_item_name":"Soup","quantity":1,"price":7,"subtotal":7}]`,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(order).Error)

	body, _ := json.Marshal(UpdateOrderStatusRequest{
		Status: database.OrderStatusOrderCancelled,
		Reason: "Guest changed mind",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "1"},
		{Key: "orderId", Value: "1"},
	}
	c.Request = httptest.NewRequest("PUT", "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	UpdateOrderStatus(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp OrderResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, database.OrderStatusOrderCancelled, resp.Order.Status)
	assert.Equal(t, "Guest changed mind", resp.Order.CancelReason)
}

func TestUpdateOrderStatus_AlreadyApproved(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	now := time.Now()
	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "O-test",
		Status:      database.OrderStatusApproved,
		CreatedBy:   "guest",
		ApprovedBy:  "staff-1",
		ApprovedAt:  &now,
		Items:       `[{"id":"i1","menu_item_name":"Burger","quantity":1,"price":10,"subtotal":10}]`,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(order).Error)

	body, _ := json.Marshal(UpdateOrderStatusRequest{
		Status: database.OrderStatusApproved,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "1"},
		{Key: "orderId", Value: "1"},
	}
	c.Request = httptest.NewRequest("PUT", "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_email", "staff-2@example.com")

	UpdateOrderStatus(c)

	// Re-approval is a no-op, should succeed
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdateOrderStatus_ApproveRequiresAuthenticatedApprover(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "O-test",
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       `[{"id":"i1","menu_item_name":"Burger","quantity":1,"price":10,"subtotal":10}]`,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(order).Error)

	body, _ := json.Marshal(UpdateOrderStatusRequest{
		Status: database.OrderStatusApproved,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: "1"},
		{Key: "orderId", Value: "1"},
	}
	c.Request = httptest.NewRequest("PUT", "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	UpdateOrderStatus(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestUpdateOrderStatus_ClosedBillApprovalReturnsConflict(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	require.NoError(t, db.Model(&database.Bill{}).
		Where("id = ?", bill.ID).
		Update("status", database.BillStatusClosed).Error)

	order := &database.Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "O-closed-bill",
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       `[{"id":"i1","menu_item_name":"Burger","quantity":1,"price":10,"subtotal":10}]`,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(order).Error)

	body, _ := json.Marshal(UpdateOrderStatusRequest{
		Status: database.OrderStatusApproved,
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", biz.ID)},
		{Key: "orderId", Value: fmt.Sprintf("%d", order.ID)},
	}
	c.Request = httptest.NewRequest("PUT", "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_email", "staff-1@example.com")

	UpdateOrderStatus(c)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "bill is no longer open")
}

func TestGetOrders_ActiveBillsOnlyExcludesClosedBills(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	openBill := createTestBill(t, biz.ID)
	closedBill := createTestBill(t, biz.ID)

	db := database.GetDB()
	require.NoError(t, db.Model(&database.Bill{}).
		Where("id = ?", closedBill.ID).
		Update("status", database.BillStatusClosed).Error)

	orders := []*database.Order{
		{
			BillID:      openBill.ID,
			BusinessID:  biz.ID,
			OrderNumber: "O-open-bill",
			Status:      database.OrderStatusPending,
			CreatedBy:   "guest",
			Items:       `[]`,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			BillID:      closedBill.ID,
			BusinessID:  biz.ID,
			OrderNumber: "O-closed-bill",
			Status:      database.OrderStatusPending,
			CreatedBy:   "guest",
			Items:       `[]`,
			CreatedAt:   time.Now().Add(time.Second),
			UpdatedAt:   time.Now(),
		},
	}
	require.NoError(t, db.Create(&orders).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest("GET", "/?status=pending&active_bills_only=true", nil)

	GetOrders(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp PaginatedOrdersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Orders, 1)
	assert.Equal(t, "O-open-bill", resp.Orders[0].OrderNumber)
	assert.Equal(t, int64(1), resp.Total)
}

func TestGetOrders_ActiveBillsOnlyKeepsInKitchenOnClosedZeroBill(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	closedBill := createTestBill(t, biz.ID)

	db := database.GetDB()
	require.NoError(t, db.Model(&database.Bill{}).
		Where("id = ?", closedBill.ID).
		Updates(map[string]interface{}{
			"status":       database.BillStatusClosed,
			"total_amount": int64(0),
		}).Error)

	order := &database.Order{
		BillID:      closedBill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "G86-36604192",
		Status:      database.OrderStatusInKitchen,
		CreatedBy:   "kitchen",
		Items:       `[]`,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	require.NoError(t, db.Create(order).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest("GET", "/?status=in_kitchen&active_bills_only=true", nil)

	GetOrders(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp PaginatedOrdersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Orders, 1)
	assert.Equal(t, "G86-36604192", resp.Orders[0].OrderNumber)
	assert.Equal(t, database.OrderStatusInKitchen, resp.Orders[0].Status)
}

func TestGetOrders_StatusListReturnsMatchingOrders(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)
	bill := createTestBill(t, biz.ID)

	db := database.GetDB()
	orders := []*database.Order{
		{
			BillID:      bill.ID,
			BusinessID:  biz.ID,
			OrderNumber: "O-pending",
			Status:      database.OrderStatusPending,
			CreatedBy:   "guest",
			Items:       `[]`,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		{
			BillID:      bill.ID,
			BusinessID:  biz.ID,
			OrderNumber: "O-approved",
			Status:      database.OrderStatusApproved,
			CreatedBy:   "guest",
			Items:       `[]`,
			CreatedAt:   time.Now().Add(time.Second),
			UpdatedAt:   time.Now(),
		},
		{
			BillID:      bill.ID,
			BusinessID:  biz.ID,
			OrderNumber: "O-cancelled",
			Status:      database.OrderStatusOrderCancelled,
			CreatedBy:   "guest",
			Items:       `[]`,
			CreatedAt:   time.Now().Add(2 * time.Second),
			UpdatedAt:   time.Now(),
		},
	}
	require.NoError(t, db.Create(&orders).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest("GET", "/?status=pending,approved&active_bills_only=true", nil)

	GetOrders(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp PaginatedOrdersResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Orders, 2)
	assert.Equal(t, int64(2), resp.Total)
	assert.Equal(t, "O-approved", resp.Orders[0].OrderNumber)
	assert.Equal(t, "O-pending", resp.Orders[1].OrderNumber)
}
