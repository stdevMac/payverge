package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/handlers"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/testperf"
	"github.com/stdevmac/payverge/backend/internal/testperf/genesisdb"
)

// seedRealisticMenuForOrders mirrors the server-package helper of the same
// name but lives in handlers_test so the order bench can share it without an
// import cycle. It writes a 50-item menu into perf-bench-001 so that
// services.PriceOrderInputsByBusinessID can resolve menu_item_id =
// "perf-bench-001-item-001" during the bench loop.
func seedRealisticMenuForOrders(b *testing.B) {
	b.Helper()
	items := make([]database.MenuItem, 50)
	for i := 0; i < 50; i++ {
		items[i] = database.MenuItem{
			ID:          fmt.Sprintf("perf-bench-001-item-%03d", i+1),
			Name:        fmt.Sprintf("Item %d", i+1),
			Description: "A realistic-sized menu item description used for the perf bench.",
			Price:       5.00 + float64(i)*0.5,
			Currency:    "USD",
			IsAvailable: true,
			SortOrder:   i,
		}
	}
	categories := []database.MenuCategory{{
		ID:        "perf-bench-001-cat-1",
		Name:      "Mains",
		SortOrder: 0,
		Items:     items,
	}}
	raw, err := json.Marshal(categories)
	if err != nil {
		b.Fatalf("marshal categories: %v", err)
	}
	db := database.GetDB()
	if err := db.Exec(`
        UPDATE menus SET categories = ?
        WHERE business_id = (SELECT id FROM businesses WHERE business_id = 'perf-bench-001')
    `, string(raw)).Error; err != nil {
		b.Fatalf("update menu: %v", err)
	}
}

type orderCreateBenchTarget struct {
	router *gin.Engine
	url    string
	body   []byte
}

type orderStatusBenchTarget struct {
	router   *gin.Engine
	urls     []string
	body     []byte
	business uint
}

func setupOrderCreateBench(b *testing.B, telegramEnabled bool) orderCreateBenchTarget {
	b.Helper()
	if testing.Short() {
		b.Skip("skipping container bench in -short")
	}
	ctx := context.Background()
	pg, err := genesisdb.Start(ctx)
	if err != nil {
		b.Fatalf("postgres at genesis schema: %v", err)
	}
	b.Cleanup(func() { _ = pg.Terminate(ctx) })

	database.SetTestDB(pg.DB)
	if err := testperf.LoadFixtures(pg.DB); err != nil {
		b.Fatalf("fixtures: %v", err)
	}
	seedRealisticMenuForOrders(b)

	db := database.GetDB()

	// Look up business + table IDs and create an OPEN bill for ordering against.
	var bizID, tableID uint
	if err := db.Raw("SELECT id FROM businesses WHERE business_id = ?", "perf-bench-001").Scan(&bizID).Error; err != nil {
		b.Fatalf("biz lookup: %v", err)
	}
	if err := db.Raw("SELECT id FROM tables WHERE table_code = ?", "perf-bench-001-t01").Scan(&tableID).Error; err != nil {
		b.Fatalf("table lookup: %v", err)
	}
	if telegramEnabled {
		telegramPlugin := &database.Plugin{
			Name:        "telegram",
			DisplayName: "Telegram Notifications",
			Category:    database.PluginCategoryIntegration,
			IsActive:    true,
		}
		if err := db.Create(telegramPlugin).Error; err != nil {
			b.Fatalf("create telegram plugin: %v", err)
		}
		if err := database.EnableBusinessPlugin(bizID, telegramPlugin.ID, map[string]interface{}{
			"is_connected": true,
			"chat_id":      "123456789",
			"notifications": map[string]interface{}{
				"order_created": true,
			},
		}); err != nil {
			b.Fatalf("enable telegram plugin: %v", err)
		}
		services.ResetTelegramNotificationEligibilityCache()
	}
	openBill := database.Bill{
		BusinessID:     bizID,
		TableID:        tableID,
		BillNumber:     "PERF-BENCH-001-OPEN",
		Items:          "[]",
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x000000000000000000000000000000000000bEEF",
		TippingAddr:    "0x000000000000000000000000000000000000bEEF",
	}
	if err := db.Create(&openBill).Error; err != nil {
		b.Fatalf("create open bill: %v", err)
	}

	h := handlers.NewOrderHandler(nil)
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.POST("/api/v1/businesses/:id/orders", h.CreateOrder)

	body := []byte(fmt.Sprintf(`{
        "bill_id": %d,
        "notes": "perf bench",
        "items": [
            {"menu_item_id": "perf-bench-001-item-001", "menu_item_name": "Item 1", "quantity": 1, "price": 5.00}
        ]
	}`, openBill.ID))

	url := fmt.Sprintf("/api/v1/businesses/%d/orders", bizID)
	return orderCreateBenchTarget{router: r, url: url, body: body}
}

// BenchmarkOrderCreate exercises POST /api/v1/businesses/:id/orders against
// a real Postgres in a testcontainer. The seed.sql ships only paid bills, so
// we create an OPEN bill at setup and POST guest orders into it. The handler
// goes through services.PriceOrderInputsByBusinessID which requires a menu
// row with matching menu_item_id, so we also seed a 50-item menu.
func BenchmarkOrderCreate(b *testing.B) {
	target := setupOrderCreateBench(b, false)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, target.url, bytes.NewReader(target.body))
		req.Header.Set("Content-Type", "application/json")
		target.router.ServeHTTP(w, req)
		if w.Code >= 400 {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

// BenchmarkOrderCreateWithTelegramEnabled captures the integration-enabled
// path: the business has a connected Telegram plugin, so order creation also
// writes a notification outbox row.
func BenchmarkOrderCreateWithTelegramEnabled(b *testing.B) {
	target := setupOrderCreateBench(b, true)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, target.url, bytes.NewReader(target.body))
		req.Header.Set("Content-Type", "application/json")
		target.router.ServeHTTP(w, req)
		if w.Code >= 400 {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

func setupOrderStatusApproveBench(b *testing.B) orderStatusBenchTarget {
	b.Helper()
	if testing.Short() {
		b.Skip("skipping container bench in -short")
	}
	ctx := context.Background()
	pg, err := genesisdb.Start(ctx)
	if err != nil {
		b.Fatalf("postgres at genesis schema: %v", err)
	}
	b.Cleanup(func() { _ = pg.Terminate(ctx) })

	database.SetTestDB(pg.DB)
	if err := testperf.LoadFixtures(pg.DB); err != nil {
		b.Fatalf("fixtures: %v", err)
	}

	db := database.GetDB()
	var bizID, tableID uint
	if err := db.Raw("SELECT id FROM businesses WHERE business_id = ?", "perf-bench-001").Scan(&bizID).Error; err != nil {
		b.Fatalf("biz lookup: %v", err)
	}
	if err := db.Raw("SELECT id FROM tables WHERE table_code = ?", "perf-bench-001-t01").Scan(&tableID).Error; err != nil {
		b.Fatalf("table lookup: %v", err)
	}

	if err := db.Create(&database.InventorySettings{
		BusinessID:                bizID,
		InventoryEnabled:          false,
		AutoDeductOnOrderApproval: false,
		LowStockWarningsEnabled:   false,
		AvailabilitySyncMode:      database.InventoryAvailabilityModeWarn,
	}).Error; err != nil {
		b.Fatalf("create inventory settings: %v", err)
	}

	bills := make([]database.Bill, b.N)
	for i := range bills {
		// Counter bills (TableID 0): the schema allows one open bill per
		// table, and each iteration approves an order on its own bill.
		bills[i] = database.Bill{
			BusinessID:     bizID,
			BillNumber:     fmt.Sprintf("PERF-STATUS-BILL-%06d", i),
			Items:          "[]",
			Status:         database.BillStatusOpen,
			SettlementAddr: "0x000000000000000000000000000000000000bEEF",
			TippingAddr:    "0x000000000000000000000000000000000000bEEF",
		}
	}
	if err := db.CreateInBatches(&bills, 500).Error; err != nil {
		b.Fatalf("create bills: %v", err)
	}

	itemsJSON, err := json.Marshal([]database.OrderItem{{
		ID:           "bench-order-line",
		MenuItemID:   "perf-bench-001-item-001",
		MenuItemName: "Item 1",
		ItemType:     "menu_item",
		Quantity:     1,
		Price:        5,
		Subtotal:     5,
	}})
	if err != nil {
		b.Fatalf("marshal order items: %v", err)
	}
	orders := make([]database.Order, b.N)
	for i := range orders {
		orders[i] = database.Order{
			BusinessID:  bizID,
			BillID:      bills[i].ID,
			OrderNumber: fmt.Sprintf("PERF-STATUS-ORDER-%06d", i),
			Status:      database.OrderStatusPending,
			CreatedBy:   "guest",
			Items:       string(itemsJSON),
			Notes:       "bench approval",
		}
	}
	if err := db.CreateInBatches(&orders, 500).Error; err != nil {
		b.Fatalf("create orders: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("staff_email", "bench-staff@example.test")
		c.Next()
	})
	r.PATCH("/api/v1/businesses/:id/orders/:orderId/status", handlers.UpdateOrderStatus)

	urls := make([]string, len(orders))
	for i := range orders {
		urls[i] = fmt.Sprintf("/api/v1/businesses/%d/orders/%d/status", bizID, orders[i].ID)
	}
	return orderStatusBenchTarget{
		router:   r,
		urls:     urls,
		body:     []byte(`{"status":"approved"}`),
		business: bizID,
	}
}

// BenchmarkOrderStatusApprove measures the staff approval path that folds a
// pending guest order into a bill and emits the normal status side effects.
func BenchmarkOrderStatusApprove(b *testing.B) {
	target := setupOrderStatusApproveBench(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, target.urls[i], bytes.NewReader(target.body))
		req.Header.Set("Content-Type", "application/json")
		target.router.ServeHTTP(w, req)
		if w.Code >= 400 {
			b.Fatalf("business %d got %d, body=%s", target.business, w.Code, w.Body.String())
		}
	}
}
