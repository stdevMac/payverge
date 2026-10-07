package server_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

type serverFlowBenchDB struct {
	businessID uint
	tableID    uint
}

type guestFlowBenchTarget struct {
	router *gin.Engine
	urls   []string
	bodies [][]byte
}

func setupServerFlowBenchDB(b *testing.B) serverFlowBenchDB {
	b.Helper()
	if testing.Short() {
		b.Skip("skipping container bench in -short")
	}

	setupServerBenchmarkDB(b)

	db := database.GetDB()
	var target serverFlowBenchDB
	if err := db.Raw("SELECT id FROM businesses WHERE business_id = ?", "perf-bench-001").Scan(&target.businessID).Error; err != nil {
		b.Fatalf("business lookup: %v", err)
	}
	if err := db.Raw("SELECT id FROM tables WHERE table_code = ?", "perf-bench-001-t01").Scan(&target.tableID).Error; err != nil {
		b.Fatalf("table lookup: %v", err)
	}
	return target
}

func seedServerFlowBills(b *testing.B, businessID, tableID uint, count int, prefix string, status database.BillStatus) []database.Bill {
	b.Helper()
	bills := make([]database.Bill, count)
	for i := range bills {
		total := int64(1000 + (i%20)*125)
		paid := int64(0)
		if status == database.BillStatusPaid {
			paid = total
		}
		bills[i] = database.Bill{
			BusinessID:     businessID,
			TableID:        tableID,
			BillNumber:     fmt.Sprintf("%s-%06d", prefix, i),
			Items:          "[]",
			Subtotal:       total,
			TotalAmount:    total,
			PaidAmount:     paid,
			Status:         status,
			SettlementAddr: "0x000000000000000000000000000000000000bEEF",
			TippingAddr:    "0x000000000000000000000000000000000000bEEF",
		}
	}
	if err := database.GetDB().CreateInBatches(&bills, 500).Error; err != nil {
		b.Fatalf("create bills: %v", err)
	}
	return bills
}

func enableGuestFlowBenchBusiness(b *testing.B, businessID uint) {
	b.Helper()
	if err := database.GetDB().Model(&database.Business{}).
		Where("id = ?", businessID).
		Updates(map[string]any{
			"kitchen_enabled": true,
			"orders_enabled":  true,
		}).Error; err != nil {
		b.Fatalf("enable guest ordering: %v", err)
	}
}

func BenchmarkGetBusinessBills(b *testing.B) {
	target := setupServerFlowBenchDB(b)
	seedServerFlowBills(b, target.businessID, target.tableID, 500, "PERF-LIST-BILL", database.BillStatusPaid)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/api/v1/businesses/:id/bills", server.GetBusinessBills)
	url := fmt.Sprintf("/api/v1/businesses/%d/bills?page_size=50", target.businessID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

func BenchmarkGuestTableLoad(b *testing.B) {
	setupServerFlowBenchDB(b)
	seedRealisticMenu(b)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/api/v1/guest/table/:code", server.GetTableByCodePublic)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/guest/table/perf-bench-001-t01", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

func BenchmarkGuestBusinessByTableCode(b *testing.B) {
	setupServerFlowBenchDB(b)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/api/v1/guest/table/:code/business", server.GetBusinessByTableCode)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/guest/table/perf-bench-001-t01/business", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

func BenchmarkGuestMenuByTableCode(b *testing.B) {
	setupServerFlowBenchDB(b)
	seedRealisticMenu(b)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/api/v1/guest/table/:code/menu", server.GetMenuByTableCode)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/guest/table/perf-bench-001-t01/menu", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

func BenchmarkGuestOpenBillByTableCode(b *testing.B) {
	target := setupServerFlowBenchDB(b)
	seedServerFlowBills(b, target.businessID, target.tableID, 1, "PERF-GUEST-OPEN-BILL", database.BillStatusOpen)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/api/v1/guest/table/:code/bill", server.GetOpenBillByTableCode)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/guest/table/perf-bench-001-t01/bill", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

func setupGuestCreateBillBench(b *testing.B) guestFlowBenchTarget {
	target := setupServerFlowBenchDB(b)
	enableGuestFlowBenchBusiness(b, target.businessID)
	services.ResetPricingCache()

	tables := make([]database.Table, b.N)
	for i := range tables {
		tables[i] = database.Table{
			BusinessID: target.businessID,
			TableCode:  fmt.Sprintf("perf-bench-create-bill-%06d", i),
			Name:       fmt.Sprintf("CB%03d", i),
			Capacity:   4,
			IsActive:   true,
		}
	}
	if err := database.GetDB().CreateInBatches(&tables, 500).Error; err != nil {
		b.Fatalf("create guest bill tables: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.POST("/api/v1/guest/table/:code/bill", server.CreateBillByTableCode)

	urls := make([]string, len(tables))
	for i := range tables {
		urls[i] = fmt.Sprintf("/api/v1/guest/table/%s/bill", tables[i].TableCode)
	}
	return guestFlowBenchTarget{router: r, urls: urls}
}

func BenchmarkGuestCreateBillByTableCode(b *testing.B) {
	target := setupGuestCreateBillBench(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, target.urls[i], nil)
		target.router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

func setupGuestCreateOrderBench(b *testing.B) guestFlowBenchTarget {
	target := setupServerFlowBenchDB(b)
	enableGuestFlowBenchBusiness(b, target.businessID)
	seedRealisticMenu(b)
	services.ResetPricingCache()

	bills := seedServerFlowBills(b, target.businessID, target.tableID, b.N, "PERF-GUEST-ORDER-BILL", database.BillStatusOpen)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.POST("/api/v1/guest/table/:code/order", server.CreateGuestOrder)

	urls := make([]string, len(bills))
	bodies := make([][]byte, len(bills))
	for i := range bills {
		urls[i] = "/api/v1/guest/table/perf-bench-001-t01/order"
		bodies[i] = []byte(fmt.Sprintf(`{
			"bill_id": %d,
			"notes": "guest order bench",
			"items": [
				{"menu_item_id": "perf-bench-001-item-001", "menu_item_name": "Item 1", "quantity": 1, "price": 5.00}
			]
		}`, bills[i].ID))
	}
	return guestFlowBenchTarget{router: r, urls: urls, bodies: bodies}
}

func BenchmarkGuestCreateOrder(b *testing.B) {
	target := setupGuestCreateOrderBench(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, target.urls[i], bytes.NewReader(target.bodies[i]))
		req.Header.Set("Content-Type", "application/json")
		// Atomic checkout requires a client-supplied idempotency id; keep it
		// unique per iteration so the bench measures real checkout work, not
		// the idempotent-replay fast path.
		req.Header.Set("X-Request-Id", fmt.Sprintf("bench-guest-order-%d", i))
		target.router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

// BenchmarkMarkBillAsPaid removed: the unmounted MarkBillAsPaid Gin handler
// was deleted. Live payment settlement is exercised under internal/handlers
// (MarkAlternativePayment) rather than this package.
