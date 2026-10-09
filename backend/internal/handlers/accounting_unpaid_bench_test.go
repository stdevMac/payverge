package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// BenchmarkGetUnpaidBills is the L6-23 Backend Performance Gate microbench.
//
// L6-23 changed the Outstanding predicate from range-scoped
// (`created_at >= start AND created_at < end`) to as-of-end (`created_at < end`
// only), so the COUNT + page query now consider every bill created before the
// window end instead of only the ones inside it. The gate question is what that
// widened predicate costs on a realistic bill table. Run BEFORE against the
// merge-base handler (0bd51b5d7) and AFTER against the branch version; numbers
// live in summary.md.
//
// Shape mirrors the L6-10 growth bench: deterministic local SQLite, one warm
// call outside the timer, -benchmem -count=3.
func BenchmarkGetUnpaidBills(b *testing.B) {
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:bench_unpaid_%d?mode=memory&cache=shared", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		b.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	prev := database.GetDB()
	database.SetTestDB(gormDB)
	b.Cleanup(func() { database.SetTestDB(prev) })

	if err := gormDB.AutoMigrate(
		&database.Business{}, &database.Table{}, &database.Bill{},
	); err != nil {
		b.Fatal(err)
	}
	// Mirror the genesis index so the SQLite microbench measures the same
	// access shape as production after the decision-14 outstanding index.
	if err := gormDB.Exec(`
		CREATE INDEX IF NOT EXISTS idx_bills_outstanding_as_of
		ON bills (business_id, created_at DESC)
		WHERE status != 'voided' AND total_amount > paid_amount
	`).Error; err != nil {
		b.Fatal(err)
	}

	business := &database.Business{
		BusinessId:      "bench-unpaid",
		Name:            "Bench Unpaid",
		OwnerAddress:    "0xBenchUnpaid",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	if err := gormDB.Create(business).Error; err != nil {
		b.Fatal(err)
	}

	tableIDs := make([]uint, 0, 40)
	for i := 0; i < 40; i++ {
		tbl := &database.Table{
			BusinessID: business.ID,
			Name:       fmt.Sprintf("T%d", i+1),
			TableCode:  fmt.Sprintf("bench-unpaid-t%d", i+1),
		}
		if err := gormDB.Create(tbl).Error; err != nil {
			b.Fatal(err)
		}
		tableIDs = append(tableIDs, tbl.ID)
	}

	// 24 months of history ending inside the benchmarked window: ~6000 bills,
	// a third of them still outstanding. The range-scoped BEFORE predicate only
	// touches the final month; the as-of AFTER predicate touches all of it.
	const totalBills = 6000
	windowEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	rows := make([]*database.Bill, 0, 500)
	flush := func() {
		if len(rows) == 0 {
			return
		}
		if err := gormDB.Create(&rows).Error; err != nil {
			b.Fatal(err)
		}
		rows = rows[:0]
	}
	for i := 0; i < totalBills; i++ {
		total := int64(2000 + (i%50)*137)
		paid := total
		status := database.BillStatusPaid
		switch i % 3 {
		case 1: // closed tab, partially collected → outstanding (receivables)
			paid = total / 2
			status = database.BillStatusClosed
		case 2: // voided (excluded by the predicate either way)
			paid = 0
			status = database.BillStatusVoided
		}
		rows = append(rows, &database.Bill{
			BusinessID:  business.ID,
			TableID:     tableIDs[i%len(tableIDs)],
			BillNumber:  fmt.Sprintf("BENCH-UNPAID-%d", i),
			TotalAmount: total,
			PaidAmount:  paid,
			Status:      status,
		})
		if len(rows) == 500 {
			flush()
		}
	}
	flush()

	// Spread created_at deterministically across the 730 days ending at the
	// window end, so ~1/24 of the table sits inside the benchmarked month and
	// the rest is older debt only the as-of predicate reaches.
	if err := gormDB.Exec(`
		UPDATE bills
		SET created_at = datetime(?, '-' || (id % 730) || ' days')
		WHERE business_id = ?`,
		windowEnd.Format("2006-01-02 15:04:05"), business.ID,
	).Error; err != nil {
		b.Fatal(err)
	}

	handler := NewAccountingHandler(database.GetDBWrapper())
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xBenchUnpaid")
		c.Set("business_owner_address", "0xBenchUnpaid")
		c.Next()
	})
	router.GET("/inside/businesses/:id/accounting/unpaid-bills", handler.GetUnpaidBills)

	path := fmt.Sprintf(
		"/inside/businesses/%d/accounting/unpaid-bills?start=2026-05-01&end=2026-05-31&page=1&page_size=20",
		business.ID,
	)

	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Warm — and assert the route actually answers with a full page, so a
	// broken seed can never silently benchmark an empty result set.
	warm := call()
	if warm.Code != http.StatusOK {
		b.Fatalf("warm request failed: %d %s", warm.Code, warm.Body.String())
	}
	var warmBody struct {
		Data struct {
			Bills []UnpaidBillRow `json:"bills"`
			Total int64           `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(warm.Body.Bytes(), &warmBody); err != nil {
		b.Fatal(err)
	}
	if len(warmBody.Data.Bills) != 20 || warmBody.Data.Total < 20 {
		b.Fatalf("bench seed too thin: %d rows on page, total %d",
			len(warmBody.Data.Bills), warmBody.Data.Total)
	}
	// For the record: the as-of predicate matches 2000 outstanding rows here;
	// the merge-base range-scoped predicate matched 93 (one month's slice).

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if w := call(); w.Code != http.StatusOK {
			b.Fatalf("request failed: %d", w.Code)
		}
	}
}
