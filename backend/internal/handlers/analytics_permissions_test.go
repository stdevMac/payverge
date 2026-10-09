package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsRoutes_ServerCannotAccessLiveBills(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleServer)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleServer))
		c.Set("staff_id", staff.ID)
		c.Set("staff_business_id", float64(business.ID))
		c.Next()
	})

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/live-bills", server.RoleBasedAccessMiddleware("overview:kpi"), handler.GetLiveBills)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/live-bills", business.ID), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAnalyticsRoutes_ManagerCanAccessLiveBills(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleManager)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", staff.ID)
		c.Set("staff_business_id", float64(business.ID))
		c.Next()
	})

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/live-bills", server.RoleBasedAccessMiddleware("overview:kpi"), handler.GetLiveBills)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/live-bills", business.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"success\":true")
}

func TestAnalyticsRoutes_DashboardSummaryIncludesPartialActiveBills(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xDashboardOwner")
	now := time.Now().UTC()

	table := &database.Table{
		BusinessID: business.ID,
		Name:       "Dashboard Table",
		TableCode:  "dashboard-table",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	createAnalyticsHandlerActiveBill(t, business.ID, table.ID, "DASHBOARD-OPEN", database.BillStatusOpen, now.Add(-30*time.Minute))
	createAnalyticsHandlerActiveBill(t, business.ID, table.ID, "DASHBOARD-PARTIAL", database.BillStatusPartial, now.Add(-20*time.Minute))
	createAnalyticsHandlerActiveBill(t, business.ID, table.ID, "DASHBOARD-PAID", database.BillStatusPaid, now.Add(-10*time.Minute))

	router := newOwnerAnalyticsTestRouter("0xDashboardOwner")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/dashboard", server.RoleBasedAccessMiddleware("overview:kpi"), handler.GetDashboardSummary)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/dashboard", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data struct {
			Live struct {
				ActiveBills int `json:"active_bills"`
			} `json:"live"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 2, response.Data.Live.ActiveBills)
}

func TestDashboardSummaryIncludesOpenChecksAndKitchenTickets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Order{}))
	business := createAccountingHandlerBusiness(t, "0xTodayFloor")
	now := time.Now().UTC()
	open := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     fmt.Sprintf("OPEN-LIVE-%d", now.UnixNano()),
		TotalAmount:    2500,
		Status:         database.BillStatusOpen,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		CreatedAt:      now.Add(-20 * time.Minute),
		UpdatedAt:      now.Add(-20 * time.Minute),
	}
	require.NoError(t, database.GetDB().Create(open).Error)
	require.NoError(t, database.GetDB().Create(&database.Order{
		BillID:      open.ID,
		BusinessID:  business.ID,
		OrderNumber: "KITCHEN-1",
		Status:      database.OrderStatusInKitchen,
		Items:       "[]",
		CreatedAt:   now.Add(-10 * time.Minute),
		UpdatedAt:   now.Add(-10 * time.Minute),
	}).Error)

	router := newOwnerAnalyticsTestRouter("0xTodayFloor")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/dashboard", server.RoleBasedAccessMiddleware("overview:kpi"), handler.GetDashboardSummary)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/dashboard", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Data struct {
			Today struct {
				Revenue          float64 `json:"revenue"`
				CollectedRevenue float64 `json:"collected_revenue"`
				FloorRemaining   float64 `json:"floor_remaining"`
				OrderCount       int64   `json:"order_count"`
			} `json:"today"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.InDelta(t, 0, response.Data.Today.Revenue, 0.01, "today sales are collected only")
	assert.InDelta(t, 0, response.Data.Today.CollectedRevenue, 0.01)
	assert.InDelta(t, 25.0, response.Data.Today.FloorRemaining, 0.01, "open remaining is labeled separately")
	assert.EqualValues(t, 1, response.Data.Today.OrderCount, "today's orders must count kitchen tickets, not paid bills")
}

// 2026-08-20 still-broken start state: live checks/tickets were opened days ago
// so the first-pass created_at=today window still reported $0 / 0 orders.
func TestDashboardSummaryIncludesOlderLiveFloorNotJustCreatedToday(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Order{}))
	business := createAccountingHandlerBusiness(t, "0xStaleFloor")
	now := time.Now().UTC()
	openedDaysAgo := now.Add(-4 * 24 * time.Hour)
	open := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     fmt.Sprintf("STALE-OPEN-%d", now.UnixNano()),
		TotalAmount:    7507,
		PaidAmount:     0,
		Status:         database.BillStatusOpen,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		CreatedAt:      openedDaysAgo,
		UpdatedAt:      openedDaysAgo,
	}
	require.NoError(t, database.GetDB().Create(open).Error)
	require.NoError(t, database.GetDB().Create(&database.Order{
		BillID:      open.ID,
		BusinessID:  business.ID,
		OrderNumber: "STALE-KITCHEN",
		Status:      database.OrderStatusPending,
		Items:       "[]",
		CreatedAt:   openedDaysAgo.Add(time.Hour),
		UpdatedAt:   openedDaysAgo.Add(time.Hour),
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.Order{
		BillID:      open.ID,
		BusinessID:  business.ID,
		OrderNumber: "STALE-IN-KITCHEN",
		Status:      database.OrderStatusInKitchen,
		Items:       "[]",
		CreatedAt:   openedDaysAgo.Add(2 * time.Hour),
		UpdatedAt:   openedDaysAgo.Add(2 * time.Hour),
	}).Error)

	router := newOwnerAnalyticsTestRouter("0xStaleFloor")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/dashboard", server.RoleBasedAccessMiddleware("overview:kpi"), handler.GetDashboardSummary)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/dashboard", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Data struct {
			Today struct {
				Revenue          float64 `json:"revenue"`
				CollectedRevenue float64 `json:"collected_revenue"`
				FloorRemaining   float64 `json:"floor_remaining"`
				OrderCount       int64   `json:"order_count"`
				Bills            int     `json:"bills"`
			} `json:"today"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.InDelta(t, 0, response.Data.Today.CollectedRevenue, 0.01, "no paid tenders today")
	assert.InDelta(t, 75.07, response.Data.Today.FloorRemaining, 0.01, "multi-day open remaining must land on the floor")
	assert.InDelta(t, 0, response.Data.Today.Revenue, 0.01, "today sales stay collected; leftover remaining is not sold")
	assert.EqualValues(t, 2, response.Data.Today.OrderCount, "today's orders must count live kitchen tickets opened days ago")
	assert.Equal(t, 0, response.Data.Today.Bills, "paid-today bill count stays collected-only")
}

// Live #703 2026-08-21: leftover remaining ($5.82 open + $18.04 on a 6-day-old
// partial) was sold as today's sales, and dashboard.week (7d) disagreed with
// sales?period=week (ISO week).
func TestDashboardAndSalesAgreeCollectedTodayAndISOWeek(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xIssue703")

	frozen := time.Date(2026, time.August, 21, 16, 20, 0, 0, time.UTC) // Friday
	lastSaturday := time.Date(2026, time.August, 15, 18, 0, 0, 0, time.UTC)
	thisWednesday := time.Date(2026, time.August, 19, 14, 0, 0, 0, time.UTC)

	openToday := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     fmt.Sprintf("OPEN-1143-%d", frozen.UnixNano()),
		TotalAmount:    582,
		PaidAmount:     0,
		Status:         database.BillStatusOpen,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		CreatedAt:      frozen.Add(-2 * time.Hour),
		UpdatedAt:      frozen.Add(-2 * time.Hour),
	}
	stalePartial := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     fmt.Sprintf("PARTIAL-761-%d", frozen.UnixNano()),
		TotalAmount:    3608,
		PaidAmount:     1804,
		Status:         database.BillStatusPartial,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		CreatedAt:      lastSaturday,
		UpdatedAt:      lastSaturday,
	}
	require.NoError(t, database.GetDB().Create(openToday).Error)
	require.NoError(t, database.GetDB().Create(stalePartial).Error)

	createAnalyticsHandlerRecognizedPayment(t, business.ID, 8000, 0, lastSaturday, "703_7d_only")
	createAnalyticsHandlerRecognizedPayment(t, business.ID, 1950, 840, thisWednesday, "703_iso_week")

	svc := analytics.NewAnalyticsService(database.GetDBWrapper()).WithClock(func() time.Time { return frozen })
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	handler.analytics = svc

	router := newOwnerAnalyticsTestRouter("0xIssue703")
	router.GET("/inside/businesses/:id/analytics/dashboard", server.RoleBasedAccessMiddleware("overview:kpi"), handler.GetDashboardSummary)
	router.GET("/inside/businesses/:id/analytics/sales", server.RoleBasedAccessMiddleware("analytics:sales"), handler.GetSalesAnalytics)

	dash := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/dashboard", business.ID), nil)
	require.Equal(t, http.StatusOK, dash.Code, dash.Body.String())
	var dashboard struct {
		Data struct {
			Today struct {
				Revenue          float64 `json:"revenue"`
				CollectedRevenue float64 `json:"collected_revenue"`
				FloorRemaining   float64 `json:"floor_remaining"`
				Bills            int     `json:"bills"`
			} `json:"today"`
			Week struct {
				Revenue float64 `json:"revenue"`
				Bills   int     `json:"bills"`
				Tips    float64 `json:"tips"`
			} `json:"week"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(dash.Body.Bytes(), &dashboard))
	assert.InDelta(t, 0, dashboard.Data.Today.Revenue, 0.01, "leftover remaining is not today's sales")
	assert.InDelta(t, 0, dashboard.Data.Today.CollectedRevenue, 0.01)
	assert.InDelta(t, 23.86, dashboard.Data.Today.FloorRemaining, 0.01, "5.82 + 18.04 remaining")
	assert.Equal(t, 0, dashboard.Data.Today.Bills)
	assert.InDelta(t, 19.50, dashboard.Data.Week.Revenue, 0.01, "ISO week excludes last Saturday")
	assert.InDelta(t, 8.40, dashboard.Data.Week.Tips, 0.01)

	todaySales := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/sales?period=today", business.ID), nil)
	require.Equal(t, http.StatusOK, todaySales.Code, todaySales.Body.String())
	var todayReport struct {
		Data struct {
			TotalRevenue     float64 `json:"total_revenue"`
			CollectedRevenue float64 `json:"collected_revenue"`
			FloorRemaining   float64 `json:"floor_remaining"`
			BillCount        int     `json:"bill_count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(todaySales.Body.Bytes(), &todayReport))
	assert.InDelta(t, 0, todayReport.Data.TotalRevenue, 0.01)
	assert.InDelta(t, 0, todayReport.Data.CollectedRevenue, 0.01)
	assert.InDelta(t, 23.86, todayReport.Data.FloorRemaining, 0.01)
	assert.Equal(t, 0, todayReport.Data.BillCount)

	weekSales := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/sales?period=week", business.ID), nil)
	require.Equal(t, http.StatusOK, weekSales.Code, weekSales.Body.String())
	var weekReport struct {
		Data struct {
			TotalRevenue float64 `json:"total_revenue"`
			BillCount    int     `json:"bill_count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(weekSales.Body.Bytes(), &weekReport))
	assert.InDelta(t, dashboard.Data.Week.Revenue, weekReport.Data.TotalRevenue, 0.01, "week endpoints must agree")
	assert.InDelta(t, 19.50, weekReport.Data.TotalRevenue, 0.01)
}

func createAnalyticsHandlerRecognizedPayment(t *testing.T, businessID uint, amountCents, tipCents int64, at time.Time, suffix string) {
	t.Helper()
	bill := &database.Bill{
		BusinessID:     businessID,
		BillNumber:     fmt.Sprintf("PAID-%s-%d", suffix, at.UnixNano()),
		TotalAmount:    amountCents,
		PaidAmount:     amountCents,
		TipAmount:      tipCents,
		Status:         database.BillStatusPaid,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		CreatedAt:      at,
		UpdatedAt:      at,
		ClosedAt:       &at,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0x" + suffix,
		Amount:        amountCents,
		TipAmount:     tipCents,
		TxHash:        "tx_" + suffix,
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &at,
		CreatedAt:     at,
		UpdatedAt:     at,
	}).Error)
}

func TestDashboardSummaryLabelsUntabledDeliveryChecks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xUntabledFloor")
	now := time.Now().UTC()

	for i := 1; i <= 4; i++ {
		table := &database.Table{
			BusinessID: business.ID,
			Name:       fmt.Sprintf("T%d", i),
			TableCode:  fmt.Sprintf("untabled-t-%d-%d", i, now.UnixNano()),
			IsActive:   true,
		}
		require.NoError(t, database.GetDB().Create(table).Error)
		createAnalyticsHandlerActiveBill(t, business.ID, table.ID, fmt.Sprintf("TABLE-%d", i), database.BillStatusOpen, now.Add(-time.Duration(i)*time.Minute))
	}
	createAnalyticsHandlerActiveBill(t, business.ID, 0, "DELIVERY-762", database.BillStatusOpen, now.Add(-5*time.Minute))

	router := newOwnerAnalyticsTestRouter("0xUntabledFloor")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/dashboard", server.RoleBasedAccessMiddleware("overview:kpi"), handler.GetDashboardSummary)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/dashboard", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Data struct {
			Live struct {
				ActiveBills   int `json:"active_bills"`
				OpenTables    int `json:"open_tables"`
				UntabledBills int `json:"untabled_bills"`
			} `json:"live"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 5, response.Data.Live.ActiveBills, "badge/bills-open count includes delivery")
	assert.Equal(t, 4, response.Data.Live.OpenTables, "floor count stays distinct tables")
	assert.Equal(t, 1, response.Data.Live.UntabledBills, "delivery table_id=0 must be labeled, not dropped")
}

func TestAnalyticsRoutes_DashboardSummariesBatchReturnsRequestedBusinesses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	first := createAccountingHandlerBusiness(t, "0xDashboardBatchOwner")
	second := &database.Business{
		BusinessId:      "biz-0xDashboardBatchOwner-2",
		Name:            "Accounting Biz 2",
		OwnerAddress:    "0xDashboardBatchOwner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, database.GetDB().Create(second).Error)

	router := newOwnerAnalyticsTestRouter("0xDashboardBatchOwner")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.POST(
		"/inside/analytics/dashboard-summaries",
		server.AllowUserLevelPermissions(),
		server.RoleBasedAccessMiddleware("overview:kpi"),
		handler.GetDashboardSummaries,
	)

	w := performAccountingRequest(t, router, http.MethodPost, "/inside/analytics/dashboard-summaries", map[string]interface{}{
		"business_ids": []uint{first.ID, second.ID},
	})
	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Success bool `json:"success"`
		Data    map[string]struct {
			Today struct {
				Revenue float64 `json:"revenue"`
			} `json:"today"`
			Live struct {
				ActiveBills int `json:"active_bills"`
			} `json:"live"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Contains(t, response.Data, fmt.Sprint(first.ID))
	assert.Contains(t, response.Data, fmt.Sprint(second.ID))
}

// TestAnalyticsRoutes_DashboardSummariesBatchSkipsSuspendedBusiness verifies
// that one administrator-suspended business does not fail the whole batch —
// operational businesses still return summaries (the cross-business
// /dashboard overview pattern).
func TestAnalyticsRoutes_DashboardSummariesBatchSkipsSuspendedBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	active := createAccountingHandlerBusiness(t, "0xMixedSubOwner")
	expired := &database.Business{
		BusinessId:      "biz-0xMixedSubOwner-suspended",
		Name:            "Suspended Biz",
		OwnerAddress:    "0xMixedSubOwner",
		SettlementAddr:  "0x5555555555555555555555555555555555555555",
		TippingAddr:     "0x6666666666666666666666666666666666666666",
		DefaultCurrency: "USD",
		IsActive:        true,
	}
	require.NoError(t, database.GetDB().Create(expired).Error)
	// gorm default:true overrides IsActive=false on Create; suspend explicitly.
	require.NoError(t, database.GetDB().Model(expired).UpdateColumn("is_active", false).Error)

	router := newOwnerAnalyticsTestRouter("0xMixedSubOwner")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.POST(
		"/inside/analytics/dashboard-summaries",
		server.AllowUserLevelPermissions(),
		server.RoleBasedAccessMiddleware("overview:kpi"),
		handler.GetDashboardSummaries,
	)

	w := performAccountingRequest(t, router, http.MethodPost, "/inside/analytics/dashboard-summaries", map[string]interface{}{
		"business_ids": []uint{active.ID, expired.ID},
	})
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var response struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Contains(t, response.Data, fmt.Sprint(active.ID))
	assert.NotContains(t, response.Data, fmt.Sprint(expired.ID))
}

// TestAnalyticsRoutes_DashboardSummariesBatchRejectsCrossTenantBusinessID locks
// AN-ISO-1: the batch summaries endpoint takes caller-supplied business ids, so
// it MUST per-id access-check each one. A caller who lists a business belonging
// to a different tenant must get a 403, and that tenant's numbers must never
// appear in the body. This is the single control gating caller-supplied ids; if
// a refactor moved/weakened the per-id CheckBusinessAccess, this test fails.
func TestAnalyticsRoutes_DashboardSummariesBatchRejectsCrossTenantBusinessID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	mine := createAccountingHandlerBusiness(t, "0xBatchCallerOwner")
	theirs := &database.Business{
		BusinessId:      "biz-0xOtherTenantOwner",
		Name:            "Other Tenant Biz",
		OwnerAddress:    "0xOtherTenantOwner",
		SettlementAddr:  "0x3333333333333333333333333333333333333333",
		TippingAddr:     "0x4444444444444444444444444444444444444444",
		DefaultCurrency: "USD",
	}
	require.NoError(t, database.GetDB().Create(theirs).Error)

	router := newOwnerAnalyticsTestRouter("0xBatchCallerOwner")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.POST(
		"/inside/analytics/dashboard-summaries",
		server.AllowUserLevelPermissions(),
		server.RoleBasedAccessMiddleware("overview:kpi"),
		handler.GetDashboardSummaries,
	)

	w := performAccountingRequest(t, router, http.MethodPost, "/inside/analytics/dashboard-summaries", map[string]interface{}{
		"business_ids": []uint{mine.ID, theirs.ID},
	})

	require.Equal(t, http.StatusForbidden, w.Code, "requesting another tenant's business id must be forbidden")
	assert.NotContains(t, w.Body.String(), fmt.Sprint(theirs.ID),
		"the other tenant's business id/numbers must not leak into the response body")
}

func TestAnalyticsRoutes_DashboardSummariesBatchAllowsPlatformAdminListedDemo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	owned := createAccountingHandlerBusiness(t, "0xPlatformAdminWallet")
	demoOwnerID := uint(99)
	listedDemo := &database.Business{
		BusinessId:      "demo-admin-1-core",
		Name:            "Payverge Core Demo Kitchen",
		OwnerAddress:    "0xDemoSeedOwner",
		IsDemo:          true,
		Kind:            database.BusinessKindDemo,
		DemoOwnerUserID: &demoOwnerID,
		DefaultCurrency: "USD",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, database.GetDB().Create(listedDemo).Error)
	unownedReal := &database.Business{
		BusinessId:      "real-other-tenant",
		Name:            "Other Real Venue",
		OwnerAddress:    "0xNotThisAdmin",
		Kind:            database.BusinessKindReal,
		DefaultCurrency: "USD",
		SettlementAddr:  "0x3333333333333333333333333333333333333333",
		TippingAddr:     "0x4444444444444444444444444444444444444444",
	}
	require.NoError(t, database.GetDB().Create(unownedReal).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "email")
		c.Set("role", "admin")
		c.Set("user_role", "admin")
		c.Set("user_id", uint(8))
		c.Set("address", "0xPlatformAdminWallet")
		c.Next()
	})
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.POST(
		"/inside/analytics/dashboard-summaries",
		server.AllowUserLevelPermissions(),
		server.RoleBasedAccessMiddleware("overview:kpi"),
		handler.GetDashboardSummaries,
	)

	ok := performAccountingRequest(t, router, http.MethodPost, "/inside/analytics/dashboard-summaries", map[string]interface{}{
		"business_ids": []uint{owned.ID, listedDemo.ID},
	})
	require.Equal(t, http.StatusOK, ok.Code, "body: %s", ok.Body.String())
	assert.Contains(t, ok.Body.String(), fmt.Sprint(owned.ID))
	assert.Contains(t, ok.Body.String(), fmt.Sprint(listedDemo.ID))

	forbidden := performAccountingRequest(t, router, http.MethodPost, "/inside/analytics/dashboard-summaries", map[string]interface{}{
		"business_ids": []uint{unownedReal.ID},
	})
	require.Equal(t, http.StatusForbidden, forbidden.Code)
	assert.NotContains(t, forbidden.Body.String(), fmt.Sprint(unownedReal.ID))
}

func TestAnalyticsRoutes_DashboardSummariesBatchAllowsDemoOwnerProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	ownerID := uint(8)
	demo := &database.Business{
		BusinessId:      "demo-owned-by-qa",
		Name:            "Payverge AI Pro Demo Lounge",
		OwnerAddress:    "0xSeedNotCaller",
		IsDemo:          true,
		Kind:            database.BusinessKindDemo,
		DemoOwnerUserID: &ownerID,
		DefaultCurrency: "USD",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, database.GetDB().Create(demo).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("address", "0xQANotOwnerAddress")
		c.Next()
	})
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.POST(
		"/inside/analytics/dashboard-summaries",
		server.AllowUserLevelPermissions(),
		server.RoleBasedAccessMiddleware("overview:kpi"),
		handler.GetDashboardSummaries,
	)

	w := performAccountingRequest(t, router, http.MethodPost, "/inside/analytics/dashboard-summaries", map[string]interface{}{
		"business_ids": []uint{demo.ID},
	})
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.Contains(t, w.Body.String(), fmt.Sprint(demo.ID))
}

func TestAnalyticsBuildTodayByStaffUsesRecognizedNetPayments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xStaffLedgerOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleManager)
	now := time.Date(2026, time.May, 11, 14, 0, 0, 0, time.UTC)

	closedAt := now.Add(-time.Hour)
	confirmedAt := closedAt.Add(-30 * time.Minute)
	staffBill := createAnalyticsHandlerClosedStaffBill(t, business.ID, staff.ID, "STAFF-LEDGER-CONFIRMED", 10000, 1000, 500, database.BillStatusPaid, confirmedAt, closedAt)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        staffBill.ID,
		PayerAddr:     "0xstaffledgerconfirmed",
		Amount:        1000,
		TipAmount:     100,
		TxHash:        "staff_ledger_confirmed",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	reversedAt := closedAt.Add(-10 * time.Minute)
	reversedBill := createAnalyticsHandlerClosedStaffBill(t, business.ID, staff.ID, "STAFF-LEDGER-REVERSED", 10000, 10000, 0, database.BillStatusPaid, confirmedAt, closedAt)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        reversedBill.ID,
		PayerAddr:     "0xstaffledgerreversed",
		Amount:        10000,
		TxHash:        "staff_ledger_reversed",
		Status:        database.PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	stats, err := handler.buildTodayByStaff(business, staff.ID, now)
	require.NoError(t, err)

	assert.Equal(t, int64(1), stats["bills_paid"])
	assert.InDelta(t, 10.0, stats["revenue_from_paid"], 0.01)
	assert.InDelta(t, 1.0, stats["tips_from_paid"], 0.01)
}

// TestAnalyticsBuildTodayByStaffNetsTodayRefundOfPriorPayment locks AN-MON-1 for
// the per-staff today panel: a refund processed TODAY of a payment confirmed on a
// prior day must reduce today's recognized revenue for that staff member, exactly
// like a chain reversal would. Before the fix, the refunded payment was invisible
// to recognition, so today's revenue was overstated (the refund never landed).
func TestAnalyticsBuildTodayByStaffNetsTodayRefundOfPriorPayment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xStaffRefundOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleManager)
	now := time.Date(2026, time.May, 12, 14, 0, 0, 0, time.UTC)

	closedAt := now.Add(-time.Hour)
	keptConfirmedAt := closedAt.Add(-20 * time.Minute)
	priorConfirmedAt := now.AddDate(0, 0, -1)
	refundedAt := closedAt.Add(-5 * time.Minute)

	bill := createAnalyticsHandlerClosedStaffBill(t, business.ID, staff.ID, "STAFF-LEDGER-REFUND", 14000, 10000, 500, database.BillStatusPaid, priorConfirmedAt, closedAt)

	// Payment kept: $100 confirmed today.
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xstaffkept",
		Amount:        10000,
		TipAmount:     500,
		TxHash:        "staff_ledger_refund_kept",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &keptConfirmedAt,
		CreatedAt:     keptConfirmedAt,
		UpdatedAt:     keptConfirmedAt,
	}).Error)

	// Payment refunded today, but originally confirmed yesterday: its negative must
	// land in today's window; its positive recognition belongs to yesterday.
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xstaffrefunded",
		Amount:        4000,
		TxHash:        "staff_ledger_refund_reversed",
		Status:        database.PaymentStatusRefunded,
		PaymentMethod: "crypto",
		ConfirmedAt:   &priorConfirmedAt,
		ReversedAt:    &refundedAt,
		CreatedAt:     priorConfirmedAt,
		UpdatedAt:     refundedAt,
	}).Error)

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	stats, err := handler.buildTodayByStaff(business, staff.ID, now)
	require.NoError(t, err)

	assert.Equal(t, int64(1), stats["bills_paid"])
	// $100 recognized today minus the $40 refunded today = $60 net.
	assert.InDelta(t, 60.0, stats["revenue_from_paid"], 0.01)
	assert.InDelta(t, 5.0, stats["tips_from_paid"], 0.01)
}

func TestAnalyticsBuildTodayByStaffExcludesPriorDayPartialPayments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xStaffLedgerCarryoverOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleManager)
	now := time.Date(2026, time.May, 11, 14, 0, 0, 0, time.UTC)

	closedAt := now.Add(-time.Hour)
	yesterdayPaymentAt := now.AddDate(0, 0, -1)
	todayPaymentAt := closedAt.Add(-15 * time.Minute)
	bill := createAnalyticsHandlerClosedStaffBill(t, business.ID, staff.ID, "STAFF-LEDGER-CARRYOVER", 10000, 10000, 0, database.BillStatusPaid, yesterdayPaymentAt, closedAt)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xstaffledgercarryoverold",
		Amount:        8000,
		TxHash:        "staff_ledger_carryover_old",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &yesterdayPaymentAt,
		CreatedAt:     yesterdayPaymentAt,
		UpdatedAt:     yesterdayPaymentAt,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xstaffledgercarryovernew",
		Amount:        2000,
		TipAmount:     250,
		TxHash:        "staff_ledger_carryover_new",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &todayPaymentAt,
		CreatedAt:     todayPaymentAt,
		UpdatedAt:     todayPaymentAt,
	}).Error)

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	stats, err := handler.buildTodayByStaff(business, staff.ID, now)
	require.NoError(t, err)

	assert.Equal(t, int64(1), stats["bills_paid"])
	assert.InDelta(t, 20.0, stats["revenue_from_paid"], 0.01)
	assert.InDelta(t, 2.5, stats["tips_from_paid"], 0.01)
}

// TestAnalyticsBuildTodayByStaffSurfacesQueryErrors guards the analytics-001
// silent-failure fix: when an underlying query fails, buildTodayByStaff must
// return a non-nil error so the dashboard surfaces a 500, rather than reporting
// zero bills/revenue/tips as though the staff member had no activity.
func TestAnalyticsBuildTodayByStaffSurfacesQueryErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xStaffLedgerErrorOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleManager)
	now := time.Date(2026, time.May, 11, 14, 0, 0, 0, time.UTC)

	// Force the recognized-payments query to fail by removing the payments
	// table out from under it. Previously the unchecked .Scan would swallow
	// this and return zeroed aggregates.
	require.NoError(t, database.GetDB().Migrator().DropTable(&database.Payment{}))

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	stats, err := handler.buildTodayByStaff(business, staff.ID, now)

	require.Error(t, err, "buildTodayByStaff must surface DB errors, not silently return zeros")
	assert.Nil(t, stats)
}

func TestAnalyticsRoutes_LiveBillsHandlesMissingTableWithoutNPlusOnePanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xLiveBillsOwner")
	now := time.Now().UTC()

	createAnalyticsHandlerActiveBill(t, business.ID, 0, "LIVE-PARTIAL-NO-TABLE", database.BillStatusPartial, now.Add(-20*time.Minute))

	router := newOwnerAnalyticsTestRouter("0xLiveBillsOwner")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/live-bills", server.RoleBasedAccessMiddleware("overview:kpi"), handler.GetLiveBills)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/live-bills", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []struct {
			Status    database.BillStatus `json:"status"`
			TableName string              `json:"table_name"`
			TableCode string              `json:"table_code"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 1)
	assert.Equal(t, database.BillStatusPartial, response.Data[0].Status)
	assert.Empty(t, response.Data[0].TableName)
	assert.Empty(t, response.Data[0].TableCode)
}

func TestAnalyticsRoutes_ServerCannotExportSalesData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleServer)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleServer))
		c.Set("staff_id", staff.ID)
		c.Set("staff_business_id", float64(business.ID))
		c.Next()
	})

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/reports/export", server.RoleBasedAccessMiddleware("reports:export"), handler.ExportSalesData)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/reports/export?period=week&format=csv", business.ID), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func createAnalyticsHandlerActiveBill(t *testing.T, businessID uint, tableID uint, number string, status database.BillStatus, at time.Time) {
	t.Helper()

	require.NoError(t, database.GetDB().Create(&database.Bill{
		BusinessID:     businessID,
		TableID:        tableID,
		BillNumber:     fmt.Sprintf("%s-%d", number, time.Now().UnixNano()),
		TotalAmount:    1000,
		Status:         status,
		SettlementAddr: "settlement-" + number,
		TippingAddr:    "tipping-" + number,
		CreatedAt:      at,
		UpdatedAt:      at,
	}).Error)
}

func createAnalyticsHandlerClosedStaffBill(t *testing.T, businessID uint, staffID uint, number string, total int64, paid int64, tip int64, status database.BillStatus, createdAt time.Time, closedAt time.Time) *database.Bill {
	t.Helper()

	bill := &database.Bill{
		BusinessID:       businessID,
		BillNumber:       fmt.Sprintf("%s-%d", number, time.Now().UnixNano()),
		Subtotal:         total,
		TotalAmount:      total,
		PaidAmount:       paid,
		TipAmount:        tip,
		Status:           status,
		SettlementAddr:   "settlement-" + number,
		TippingAddr:      "tipping-" + number,
		CreatedAt:        createdAt,
		UpdatedAt:        closedAt,
		ClosedAt:         &closedAt,
		ClosedByStaffID:  &staffID,
		CreatedByStaffID: &staffID,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return bill
}

func TestAnalyticsRoutes_ExportSalesDataRangeLimitReturnsClientError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwner")
		c.Set("business_owner_address", "0xOwner")
		c.Next()
	})

	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/reports/export", server.RoleBasedAccessMiddleware("reports:export"), handler.ExportSalesData)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/reports/export?period=year&format=csv", business.ID), nil)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Contains(t, w.Body.String(), "recognized payment export range is too large")
}

func TestAnalyticsRoutes_SalesInvalidPeriodReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := newOwnerAnalyticsTestRouter("0xOwner")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/sales", server.RoleBasedAccessMiddleware("analytics:sales"), handler.GetSalesAnalytics)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/sales?period=fortnight", business.ID), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "unsupported period: fortnight")
}

func TestAnalyticsRoutes_SalesCustomPeriodWithoutDateReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := newOwnerAnalyticsTestRouter("0xOwner")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/sales", server.RoleBasedAccessMiddleware("analytics:sales"), handler.GetSalesAnalytics)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/sales?period=custom", business.ID), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "date is required when period=custom")
}

func TestAnalyticsRoutes_TipAndItemInvalidPeriodsReturnBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := newOwnerAnalyticsTestRouter("0xOwner")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/tips", server.RoleBasedAccessMiddleware("analytics:tips"), handler.GetTipAnalytics)
	router.GET("/inside/businesses/:id/analytics/items", server.RoleBasedAccessMiddleware("analytics:items"), handler.GetItemAnalytics)

	tipResponse := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/tips?period=forever", business.ID), nil)
	assert.Equal(t, http.StatusBadRequest, tipResponse.Code)
	assert.Contains(t, tipResponse.Body.String(), "unsupported period: forever")

	itemResponse := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/analytics/items?period=forever", business.ID), nil)
	assert.Equal(t, http.StatusBadRequest, itemResponse.Code)
	assert.Contains(t, itemResponse.Body.String(), "unsupported period: forever")
}

func TestAnalyticsRoutes_ExportUnsupportedFormatReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	router := newOwnerAnalyticsTestRouter("0xOwner")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/reports/export", server.RoleBasedAccessMiddleware("reports:export"), handler.ExportSalesData)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/reports/export?period=week&format=xlsx", business.ID), nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "unsupported format: xlsx")
}

func newOwnerAnalyticsTestRouter(ownerAddress string) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", ownerAddress)
		c.Set("business_owner_address", ownerAddress)
		c.Next()
	})
	return router
}
