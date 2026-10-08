package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/accounting"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type unpaidSQLCapture struct {
	logger.Interface
	sqls []string
}

func (c *unpaidSQLCapture) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	c.sqls = append(c.sqls, sql)
}

// TestUnpaidBillRow_MoneyIsDollars asserts the wire shape (float64 dollars).
func TestUnpaidBillRow_MoneyIsDollars(t *testing.T) {
	row := UnpaidBillRow{Outstanding: 12.34, Total: 20, Paid: 7.66}
	require.InDelta(t, 12.34, row.Outstanding, 0.001)
	require.InDelta(t, row.Total-row.Paid, row.Outstanding, 0.001)
}

// TestPeriodLock_BlocksPayrollPeriodEnd: payroll mutations gate on period end.
func TestPeriodLock_BlocksPayrollPeriodEnd(t *testing.T) {
	through := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	require.True(t, accounting.IsDateLocked(&through, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)))
	require.True(t, accounting.IsDateLocked(&through, time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)))
	require.False(t, accounting.IsDateLocked(&through, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)))
}

func createUnpaidTestBill(t *testing.T, businessID uint, n int, totalCents, paidCents int64, status database.BillStatus, createdAt time.Time) *database.Bill {
	t.Helper()
	bill := &database.Bill{
		BusinessID:  businessID,
		BillNumber:  fmt.Sprintf("UB-%s-%d", t.Name(), n),
		TotalAmount: totalCents,
		PaidAmount:  paidCents,
		Status:      status,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	// CreatedAt is GORM-managed on create; place the bill in the window explicitly.
	require.NoError(t, database.GetDB().Model(bill).UpdateColumn("created_at", createdAt).Error)
	return bill
}

// TestGetUnpaidBills_HandlerShape covers the live handler end to end: the
// range-scoped leftover-remaining predicate (status <> voided, total > paid,
// created_at in [start, end)), outstanding-desc ordering, page_size clamp to
// 100, and the pagination envelope. #799: the date filter is honored — bills
// created before `start` leave the list and surface as carried_over instead
// (preserving the #222 / L6-23 requirement that pre-range debt stays visible).
// #770: open / partial / abandoned remainings are listed — they are the
// checks that make collection_gap.
func TestGetUnpaidBills_HandlerShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xUnpaidOwner")

	in := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	beforeStart := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	afterEnd := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	unpaidSmall := createUnpaidTestBill(t, business.ID, 1, 50_00, 20_00, database.BillStatusClosed, in)  // outstanding 30.00
	unpaidBig := createUnpaidTestBill(t, business.ID, 2, 200_00, 0, database.BillStatusClosed, in)       // outstanding 200.00
	createUnpaidTestBill(t, business.ID, 3, 80_00, 80_00, database.BillStatusPaid, in)                   // fully paid → excluded
	createUnpaidTestBill(t, business.ID, 4, 90_00, 0, database.BillStatusVoided, in)                     // voided → excluded
	createUnpaidTestBill(t, business.ID, 5, 70_00, 0, database.BillStatusClosed, beforeStart)            // pre-start → carried_over, not listed (#799)
	createUnpaidTestBill(t, business.ID, 6, 40_00, 0, database.BillStatusClosed, afterEnd)               // after end → excluded
	liveOpen := createUnpaidTestBill(t, business.ID, 7, 120_00, 0, database.BillStatusOpen, in)          // leftover remaining
	livePartial := createUnpaidTestBill(t, business.ID, 8, 60_00, 10_00, database.BillStatusPartial, in) // leftover remaining
	abandoned := createUnpaidTestBill(t, business.ID, 9, 45_00, 0, database.BillStatusAbandoned, in)     // leftover remaining

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xUnpaidOwner")
		c.Set("business_owner_address", "0xUnpaidOwner")
		c.Next()
	})
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/unpaid-bills", server.RoleBasedAccessMiddleware("financial:read"), handler.GetUnpaidBills)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/unpaid-bills?start=2026-05-01&end=2026-05-31&page=1&page_size=500", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			Bills             []UnpaidBillRow `json:"bills"`
			Total             int64           `json:"total"`
			Page              int             `json:"page"`
			PageSize          int             `json:"page_size"`
			TotalPages        int             `json:"total_pages"`
			CarriedOver       int64           `json:"carried_over"`
			CarriedOverAmount float64         `json:"carried_over_amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.Equal(t, int64(5), resp.Data.Total)
	require.Equal(t, 100, resp.Data.PageSize, "page_size must clamp to 100")
	require.Equal(t, 1, resp.Data.TotalPages)
	require.Len(t, resp.Data.Bills, 5)
	// Ordered by outstanding DESC: 200 closed, 120 open, 50 partial, 45 abandoned, 30 closed.
	require.Equal(t, unpaidBig.ID, resp.Data.Bills[0].ID)
	require.Equal(t, liveOpen.ID, resp.Data.Bills[1].ID)
	require.Equal(t, livePartial.ID, resp.Data.Bills[2].ID)
	require.Equal(t, abandoned.ID, resp.Data.Bills[3].ID)
	require.Equal(t, unpaidSmall.ID, resp.Data.Bills[4].ID)
	require.Equal(t, string(database.BillStatusClosed), resp.Data.Bills[0].Status)
	require.Equal(t, string(database.BillStatusOpen), resp.Data.Bills[1].Status)
	require.Equal(t, string(database.BillStatusPartial), resp.Data.Bills[2].Status)
	require.Equal(t, string(database.BillStatusAbandoned), resp.Data.Bills[3].Status)
	require.InDelta(t, 200.0, resp.Data.Bills[0].Outstanding, 0.001)
	require.InDelta(t, 120.0, resp.Data.Bills[1].Outstanding, 0.001)
	require.InDelta(t, 50.0, resp.Data.Bills[2].Outstanding, 0.001)
	require.InDelta(t, 45.0, resp.Data.Bills[3].Outstanding, 0.001)
	require.InDelta(t, 30.0, resp.Data.Bills[4].Outstanding, 0.001)
	// Money is dollars on the wire.
	require.InDelta(t, 50.0, resp.Data.Bills[4].Total, 0.001)
	require.InDelta(t, 20.0, resp.Data.Bills[4].Paid, 0.001)
	// The pre-start leftover ($70 closed) is not hidden — it is named (#222).
	require.Equal(t, int64(1), resp.Data.CarriedOver)
	require.InDelta(t, 70.0, resp.Data.CarriedOverAmount, 0.001)
}

// TestGetUnpaidBills_SingleDayFilter_ScopedWithCarriedOver reproduces #799:
// unpaid-bills?start=2026-08-22&end=2026-08-22 returned the same six 30-day
// leftovers as any other range — the day filter was ignored while the summary
// for the same dates returned zeros. The list must be range-scoped like the
// collection_gap KPI, and pre-range debt must stay visible (#222 / L6-23) as
// carried_over count + amount instead of fake in-range rows.
func TestGetUnpaidBills_SingleDayFilter_ScopedWithCarriedOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xDayFilter")

	today := time.Date(2026, 8, 22, 13, 0, 0, 0, time.UTC)
	daysAgo := func(d int) time.Time { return today.AddDate(0, 0, -d) }

	// Six older leftovers spread over the prior month — the "same 6 as 30d" set.
	var carried int64
	for i := 1; i <= 6; i++ {
		createUnpaidTestBill(t, business.ID, i, int64(i)*10_00, 0, database.BillStatusClosed, daysAgo(i*4))
		carried += int64(i) * 10_00
	}
	todayDebt := createUnpaidTestBill(t, business.ID, 7, 25_00, 5_00, database.BillStatusOpen, today)
	createUnpaidTestBill(t, business.ID, 8, 99_00, 0, database.BillStatusClosed, today.AddDate(0, 0, 2)) // future → excluded

	router := unpaidBillsRouter(t, "0xDayFilter")
	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/unpaid-bills?start=2026-08-22&end=2026-08-22&page=1&page_size=50", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			Bills             []UnpaidBillRow `json:"bills"`
			Total             int64           `json:"total"`
			CarriedOver       int64           `json:"carried_over"`
			CarriedOverAmount float64         `json:"carried_over_amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Only today's leftover is IN the single-day window.
	require.Equal(t, int64(1), resp.Data.Total, "single-day filter must not replay the whole leftover book")
	require.Len(t, resp.Data.Bills, 1)
	require.Equal(t, todayDebt.ID, resp.Data.Bills[0].ID)
	require.InDelta(t, 20.0, resp.Data.Bills[0].Outstanding, 0.001)

	// The six pre-range leftovers are named, not hidden.
	require.Equal(t, int64(6), resp.Data.CarriedOver)
	require.InDelta(t, float64(carried)/100.0, resp.Data.CarriedOverAmount, 0.001)
}

// TestGetUnpaidBills_CountSkipsTableJoin is the decision-14 access-shape gate:
// COUNT must not LEFT JOIN tables (label is page-only). The page Scan may join.
func TestGetUnpaidBills_CountSkipsTableJoin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cap := &unpaidSQLCapture{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:unpaid_count_shape_%d?mode=memory&cache=shared", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: cap})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	prev := database.GetDB()
	database.SetTestDB(gormDB)
	t.Cleanup(func() { database.SetTestDB(prev) })
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{}, &database.User{}, &database.Staff{},
		&database.StaffPermissionDeny{}, &database.Table{}, &database.Bill{},
	))
	server.InitializeRBAC(database.GetDBWrapper())

	business := createAccountingHandlerBusiness(t, "0xCountShape")
	inWindow := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	createUnpaidTestBill(t, business.ID, 1, 100_00, 0, database.BillStatusClosed, inWindow)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xCountShape")
		c.Set("business_owner_address", "0xCountShape")
		c.Next()
	})
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/unpaid-bills", server.RoleBasedAccessMiddleware("financial:read"), handler.GetUnpaidBills)

	cap.sqls = nil
	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/unpaid-bills?start=2026-05-01&end=2026-05-31&page=1&page_size=20", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	var countSQLs, pageSQLs []string
	for _, raw := range cap.sqls {
		low := strings.ToLower(raw)
		// GORM quotes table names with `bills` / "bills" depending on dialect.
		if !strings.Contains(low, "bills") {
			continue
		}
		if strings.Contains(low, "count(") || strings.Contains(low, "count(*)") {
			countSQLs = append(countSQLs, raw)
			require.NotContainsf(t, low, "join", "COUNT must not join tables: %s", raw)
			require.Contains(t, low, "voided", "COUNT must use leftover-remaining (status <> voided): %s", raw)
			require.NotContains(t, low, "status = \"closed\"", "COUNT must not be closed-only: %s", raw)
			require.NotContains(t, low, "status = 'closed'", "COUNT must not be closed-only: %s", raw)
		} else if strings.Contains(low, "left join") || strings.Contains(low, "tables.name") {
			pageSQLs = append(pageSQLs, raw)
		}
	}
	require.NotEmpty(t, countSQLs, "expected a bills COUNT query, got %v", cap.sqls)
	require.NotEmpty(t, pageSQLs, "expected a page SELECT with table label, got %v", cap.sqls)
}

func decodeUnpaidBills(t *testing.T, body []byte) (bills []UnpaidBillRow, total int64) {
	t.Helper()
	var resp struct {
		Data struct {
			Bills []UnpaidBillRow `json:"bills"`
			Total int64           `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp.Data.Bills, resp.Data.Total
}

func unpaidBillsRouter(t *testing.T, address string) *gin.Engine {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", address)
		c.Set("business_owner_address", address)
		c.Next()
	})
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/unpaid-bills", server.RoleBasedAccessMiddleware("financial:read"), handler.GetUnpaidBills)
	return router
}

// TestGetUnpaidBills_ListsCollectionGapRemainings is the #770 gate: mixed
// open / abandoned / partial leftover remainings that make collection_gap
// must appear on unpaid-bills. Voided and fully-paid stay out. Older leftover
// remainings move to carried_over (#799) so the listed rows reconcile 1:1
// with the range-scoped collection_gap KPI.
func TestGetUnpaidBills_ListsCollectionGapRemainings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xGapList")

	in := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	beforeStart := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	afterEnd := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	openBill := createUnpaidTestBill(t, business.ID, 1, 20_00, 0, database.BillStatusOpen, in)
	abandonedBill := createUnpaidTestBill(t, business.ID, 2, 35_00, 5_00, database.BillStatusAbandoned, in)
	partialBill := createUnpaidTestBill(t, business.ID, 3, 40_00, 10_00, database.BillStatusPartial, in)
	closedBill := createUnpaidTestBill(t, business.ID, 4, 15_00, 0, database.BillStatusClosed, in)
	createUnpaidTestBill(t, business.ID, 5, 99_00, 0, database.BillStatusVoided, in)
	createUnpaidTestBill(t, business.ID, 6, 50_00, 50_00, database.BillStatusPaid, in)
	oldAbandoned := createUnpaidTestBill(t, business.ID, 7, 12_00, 0, database.BillStatusAbandoned, beforeStart)
	createUnpaidTestBill(t, business.ID, 8, 80_00, 0, database.BillStatusOpen, afterEnd)

	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	summary, err := accounting.NewService(database.GetDBWrapper()).GetSummary(business.ID, start, end)
	require.NoError(t, err)
	require.InDelta(t, 95.0, summary.CollectionGap, 0.001,
		"collection_gap = open 20 + abandoned 30 + partial 30 + closed 15")

	router := unpaidBillsRouter(t, "0xGapList")
	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/unpaid-bills?start=2026-05-01&end=2026-05-31&page=1&page_size=50", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	bills, total := decodeUnpaidBills(t, w.Body.Bytes())
	require.Equal(t, int64(4), total, "four in-window leftover remainings; the older abandoned is carried_over")

	byID := map[uint]UnpaidBillRow{}
	var listed float64
	for _, b := range bills {
		byID[b.ID] = b
		listed += b.Outstanding
	}
	require.Contains(t, byID, openBill.ID, "open leftover remaining must be listed")
	require.Contains(t, byID, abandonedBill.ID, "abandoned leftover remaining must be listed")
	require.Contains(t, byID, partialBill.ID, "partial leftover remaining must be listed")
	require.Contains(t, byID, closedBill.ID, "closed leftover remaining must be listed")
	require.NotContains(t, byID, oldAbandoned.ID, "pre-range leftover moves to carried_over (#799)")
	require.Equal(t, string(database.BillStatusOpen), byID[openBill.ID].Status)
	require.Equal(t, string(database.BillStatusAbandoned), byID[abandonedBill.ID].Status)
	require.Equal(t, string(database.BillStatusPartial), byID[partialBill.ID].Status)
	require.InDelta(t, summary.CollectionGap, listed, 0.001,
		"sum of unpaid-bills rows must equal collection_gap for the same window")

	var resp struct {
		Data struct {
			CarriedOver       int64   `json:"carried_over"`
			CarriedOverAmount float64 `json:"carried_over_amount"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, int64(1), resp.Data.CarriedOver, "older abandoned leftover is named")
	require.InDelta(t, 12.0, resp.Data.CarriedOverAmount, 0.001)
}
