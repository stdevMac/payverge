package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// seedPayrollListRuns inserts nRuns draft runs with linesPerRun line items.
func seedPayrollListRuns(tb testing.TB, db *gorm.DB, businessID uint, nRuns, linesPerRun int) {
	tb.Helper()
	base := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < nRuns; i++ {
		start := base.AddDate(0, 0, i*14)
		run := &database.PayrollRun{
			BusinessID:  businessID,
			PeriodStart: start,
			PeriodEnd:   start.AddDate(0, 0, 14),
			Status:      database.PayrollRunStatusDraft,
			Currency:    "USD",
			GrossTotal:  int64(10000 * linesPerRun),
			NetTotal:    int64(10000 * linesPerRun),
		}
		if err := db.Create(run).Error; err != nil {
			tb.Fatal(err)
		}
		for j := 0; j < linesPerRun; j++ {
			if err := db.Create(&database.PayrollLineItem{
				PayrollRunID: run.ID,
				BusinessID:   businessID,
				PayeeType:    database.PayrollPayeeTypeContractor,
				PayeeName:    fmt.Sprintf("P-%d-%d", i, j),
				GrossAmount:  10000,
				NetAmount:    10000,
			}).Error; err != nil {
				tb.Fatal(err)
			}
		}
	}
}

func payrollListRouter(ownerAddr string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", ownerAddr)
		c.Set("business_owner_address", ownerAddr)
		c.Next()
	})
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/payroll-runs", server.RoleBasedAccessMiddleware("financial:read"), handler.ListPayrollRuns)
	return router
}

// A request without ?page must not take an unbounded path that hydrates every
// run's line items: it is served by the paged query (LIMIT, no LineItems
// preload) and returns the paged envelope.
func TestListPayrollRuns_WithoutPageIsBoundedAndSkipsLineItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xPayrollBounded")
	seedPayrollListRuns(t, db, business.ID, 120, 3)

	var mu sync.Mutex
	var queries []string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:capture_payroll_list", func(tx *gorm.DB) {
		mu.Lock()
		queries = append(queries, tx.Statement.SQL.String())
		mu.Unlock()
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:capture_payroll_list") })

	w := performAccountingRequest(t, payrollListRouter("0xPayrollBounded"), http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Data struct {
			Runs []struct {
				ID         uint            `json:"id"`
				PayeeCount int             `json:"payee_count"`
				LineItems  json.RawMessage `json:"line_items"`
			} `json:"runs"`
			Total    int64 `json:"total"`
			Page     int   `json:"page"`
			PageSize int   `json:"page_size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, int64(120), body.Data.Total)
	require.Equal(t, 1, body.Data.Page)
	require.LessOrEqual(t, len(body.Data.Runs), 100, "default page must be bounded")
	require.Equal(t, body.Data.PageSize, len(body.Data.Runs))
	for _, run := range body.Data.Runs {
		require.Equal(t, 3, run.PayeeCount)
	}

	mu.Lock()
	defer mu.Unlock()
	sawLimitedRunSelect := false
	for _, q := range queries {
		lower := strings.ToLower(q)
		if strings.Contains(lower, "from `payroll_line_items`") || strings.Contains(lower, "from payroll_line_items") {
			require.Contains(t, lower, "group by", "line items may only be read as an aggregate, got: %s", q)
		}
		if strings.Contains(lower, "payroll_runs") && strings.Contains(lower, "limit") && !strings.Contains(lower, "count(") {
			sawLimitedRunSelect = true
		}
	}
	require.True(t, sawLimitedRunSelect, "run list query must carry a LIMIT; queries: %v", queries)
}

func TestListPayrollRuns_PageSizeIsCapped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xPayrollCap")
	seedPayrollListRuns(t, db, business.ID, 105, 1)

	w := performAccountingRequest(t, payrollListRouter("0xPayrollCap"), http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs?page_size=100000", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Data struct {
			Runs     []json.RawMessage `json:"runs"`
			PageSize int               `json:"page_size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, 100, body.Data.PageSize)
	require.Len(t, body.Data.Runs, 100)
}

func TestListPayrollRuns_InvalidPageRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xPayrollBadPage")
	w := performAccountingRequest(t, payrollListRouter("0xPayrollBadPage"), http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs?page=0", business.ID), nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// BenchmarkListPayrollRunsHandler_NoPage measures GET payroll-runs without
// ?page for a dense business (300 runs x 8 line items).
// Run: go test ./internal/handlers/ -bench BenchmarkListPayrollRunsHandler_NoPage -benchmem -count=3 -run '^$'
func BenchmarkListPayrollRunsHandler_NoPage(b *testing.B) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:bench_payroll_list_nopage_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(db)
	if err := db.AutoMigrate(
		&database.Business{}, &database.User{}, &database.Staff{}, &database.StaffPermissionDeny{},
		&database.PayrollRun{}, &database.PayrollLineItem{},
	); err != nil {
		b.Fatal(err)
	}
	server.InitializeRBAC(database.GetDBWrapper())
	business := &database.Business{
		BusinessId: "bench-payroll-nopage", Name: "Bench", OwnerAddress: "0xBenchNoPage",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222", DefaultCurrency: "USD",
	}
	if err := db.Create(business).Error; err != nil {
		b.Fatal(err)
	}
	seedPayrollListRuns(b, db, business.ID, 300, 8)
	router := payrollListRouter("0xBenchNoPage")
	path := fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs", business.ID)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			b.Fatalf("status %d", w.Code)
		}
	}
}
