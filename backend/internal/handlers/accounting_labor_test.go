package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/reporting"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupLaborHandlerDB reuses the menu-engineering harness (which already brings
// up bill_items + recipe tables for the prime-cost food-cost composition) and
// ensures payroll_runs is migrated, which the labor calculator's
// GetPaidPayrollRunsOverlapping path reads.
func setupLaborHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB := setupMenuEngineeringDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.PayrollRun{}))
	return gormDB
}

func registerLaborRoute(router *gin.Engine, handler *AccountingHandler) {
	router.GET("/inside/businesses/:id/accounting/labor-cost",
		server.RoleBasedAccessMiddleware("financial:read"), handler.GetLaborCost)
}

// laborCostResponse parses just the three percentage fields needed to assert the
// prime-cost invariant plus the data-presence flags.
type laborCostResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Period          string  `json:"period"`
		LaborCost       float64 `json:"labor_cost"`
		NetSales        float64 `json:"net_sales"`
		LaborCostPct    float64 `json:"labor_cost_pct"`
		PayrollRunCount int     `json:"payroll_run_count"`
		HasData         bool    `json:"has_data"`
		FoodCostPct     float64 `json:"food_cost_pct"`
		PrimeCostPct    float64 `json:"prime_cost_pct"`
	} `json:"data"`
}

// TestLaborCost_OwnerGets200AndPrimeCostInvariant seeds one PAID payroll run
// overlapping the rolling "week" window and asserts the owner gets a 200 with a
// non-zero labor cost and the prime-cost invariant prime == labor + food.
func TestLaborCost_OwnerGets200AndPrimeCostInvariant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupLaborHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xLaborOwner")

	// PAID run whose work period [now-3d, now+1d] overlaps the week window
	// [now-7d, now): overlap 3d of a 4d period -> frac 0.75, contrib $750.
	run := &database.PayrollRun{
		BusinessID:  business.ID,
		Status:      database.PayrollRunStatusPaid,
		PeriodStart: time.Now().Add(-3 * 24 * time.Hour),
		PeriodEnd:   time.Now().Add(24 * time.Hour),
		GrossTotal:  100000,
		Currency:    "USD",
	}
	require.NoError(t, database.GetDB().Create(run).Error)

	router := ownerRouter("0xLaborOwner")
	handler := NewAccountingHandler(database.GetDBWrapper())
	registerLaborRoute(router, handler)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/labor-cost", business.ID), nil)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	// contributions must serialize as an array literal, never null.
	assert.Contains(t, w.Body.String(), `"contributions":[`, "contributions must be an array, not null")

	var resp laborCostResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp.Success)
	assert.True(t, resp.Data.HasData, "a paid overlapping run means has_data")
	assert.Greater(t, resp.Data.LaborCost, 0.0, "prorated labor cost must be positive")
	assert.InDelta(t, resp.Data.LaborCostPct+resp.Data.FoodCostPct, resp.Data.PrimeCostPct, 0.0001,
		"prime_cost_pct must equal labor_cost_pct + food_cost_pct")
}

// TestLaborCost_BadPeriodIs400 verifies an unsupported period (labor rejects
// day-granular windows) maps to 400 via labor.ErrUnsupportedPeriod.
func TestLaborCost_BadPeriodIs400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupLaborHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xLaborOwnerBad")

	router := ownerRouter("0xLaborOwnerBad")
	handler := NewAccountingHandler(database.GetDBWrapper())
	registerLaborRoute(router, handler)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/labor-cost?period=day", business.ID), nil)

	assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
}

// TestLaborCost_NoPayrollHasDataFalse verifies a business with no payroll runs
// returns 200 with has_data:false and a zero run count.
func TestLaborCost_NoPayrollHasDataFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupLaborHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xLaborOwnerEmpty")

	router := ownerRouter("0xLaborOwnerEmpty")
	handler := NewAccountingHandler(database.GetDBWrapper())
	registerLaborRoute(router, handler)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/labor-cost", business.ID), nil)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp laborCostResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.True(t, resp.Success)
	assert.False(t, resp.Data.HasData, "no payroll runs means has_data is false")
	assert.Equal(t, 0, resp.Data.PayrollRunCount, "no payroll runs means a zero run count")
}

// TestLaborCost_CrossTenantIsForbidden locks the IDOR guard: the owner of
// business A requesting business B's id must not receive a 200 or B's data.
func TestLaborCost_CrossTenantIsForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupLaborHandlerDB(t)
	_ = createAccountingHandlerBusiness(t, "0xLaborOwnerA")
	businessB := createAccountingHandlerBusiness(t, "0xLaborOwnerB")

	// Authenticated as owner of A, requesting B's id.
	router := ownerRouter("0xLaborOwnerA")
	handler := NewAccountingHandler(database.GetDBWrapper())
	registerLaborRoute(router, handler)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/labor-cost", businessB.ID), nil)

	assert.NotEqual(t, http.StatusOK, w.Code, "cross-tenant request must not succeed")
	assert.Equal(t, http.StatusForbidden, w.Code, "loadBusiness denies non-owned business with 403")
	assert.NotContains(t, w.Body.String(), `"success":true`, "must not return B's data envelope")
}

// staffLaborRouter registers GetLaborCost WITHOUT the financial:read route gate
// so the IN-HANDLER owner/financial:read dollar gate is what is exercised. A
// per-request middleware hydrates a staff context (CheckBusinessAccess passes on
// staff_business_id).
func staffLaborRouter(role database.StaffRole, staffID, businessID uint, h *AccountingHandler) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(role))
		c.Set("staff_id", staffID)
		c.Set("staff_business_id", businessID)
		c.Next()
	})
	router.GET("/inside/businesses/:id/accounting/labor-cost", h.GetLaborCost)
	return router
}

// seedApprovedActualHours seeds a staff member with a primary pay rate and one
// approved 8h-minus-30m (=450 min, 7.5h) time entry inside the active week
// window, so the actuals basis computes 7.5h * $20 = $150.
func seedApprovedActualHours(t *testing.T, businessID, staffID uint) {
	t.Helper()
	require.NoError(t, database.GetDB().Create(&database.StaffPosition{
		BusinessID: businessID, StaffID: staffID, PositionID: 1, PayRateCents: 2000, IsPrimary: true,
	}).Error)
	win, err := reporting.ResolveWindowAt("week", nil, nil, time.UTC, time.Now())
	require.NoError(t, err)
	in := win.End.Add(-10 * time.Hour)
	if in.Before(win.Start) {
		in = win.Start.Add(time.Minute)
	}
	out := in.Add(8 * time.Hour)
	require.NoError(t, database.GetDB().Create(&database.TimeEntry{
		BusinessID: businessID, StaffID: staffID, ClockInAt: in, ClockOutAt: &out, BreakMinutes: 30,
		Status: database.TimeEntryStatusApproved, Source: database.TimeEntrySourceManagerManual,
	}).Error)
}

// TestLaborCost_DefaultBasisIsByteCompatiblePayroll locks that the default (no
// basis / basis=payroll) response keeps the prior payroll shape and adds none of
// the actuals-only fields.
func TestLaborCost_DefaultBasisIsByteCompatiblePayroll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupLaborHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xLaborDefault")

	router := ownerRouter("0xLaborDefault")
	handler := NewAccountingHandler(database.GetDBWrapper())
	registerLaborRoute(router, handler)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/labor-cost", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	body := w.Body.String()
	assert.Contains(t, body, `"payroll_run_count"`)
	assert.Contains(t, body, `"contributions":[`)
	assert.NotContains(t, body, `"basis"`, "default response must not carry the actuals basis marker")
	assert.NotContains(t, body, `"staff_contributions"`)
	assert.NotContains(t, body, `"worked_hours"`)
}

// TestLaborCost_ActualBasisOwnerSeesDollars: the owner GET ?basis=actual gets the
// approved-hours labor cost, per-person dollars, and the variance dollar figure.
func TestLaborCost_ActualBasisOwnerSeesDollars(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupLaborHandlerDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.StaffPosition{}, &database.TimeEntry{}))
	business := createAccountingHandlerBusiness(t, "0xActualOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleServer)
	seedApprovedActualHours(t, business.ID, staff.ID)

	router := ownerRouter("0xActualOwner")
	handler := NewAccountingHandler(database.GetDBWrapper())
	registerLaborRoute(router, handler)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/labor-cost?basis=actual", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	body := w.Body.String()
	assert.Contains(t, body, `"basis":"actual"`)
	assert.Contains(t, body, `"staff_contributions":[`)

	var resp struct {
		Data struct {
			Basis              string  `json:"basis"`
			LaborCost          float64 `json:"labor_cost"`
			WorkedHours        float64 `json:"worked_hours"`
			LaborCostPct       float64 `json:"labor_cost_pct"`
			HasData            bool    `json:"has_data"`
			StaffContributions []struct {
				StaffID       uint    `json:"staff_id"`
				WorkedMinutes int     `json:"worked_minutes"`
				LaborCost     float64 `json:"labor_cost"`
			} `json:"staff_contributions"`
			Variance map[string]interface{} `json:"variance"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "actual", resp.Data.Basis)
	assert.True(t, resp.Data.HasData)
	assert.InDelta(t, 150.0, resp.Data.LaborCost, 0.01)
	assert.InDelta(t, 7.5, resp.Data.WorkedHours, 0.001)
	require.Len(t, resp.Data.StaffContributions, 1)
	assert.Equal(t, 450, resp.Data.StaffContributions[0].WorkedMinutes)
	assert.InDelta(t, 150.0, resp.Data.StaffContributions[0].LaborCost, 0.01)
	require.NotNil(t, resp.Data.Variance)
	_, hasVarianceDollar := resp.Data.Variance["labor_cost"]
	assert.True(t, hasVarianceDollar, "owner sees the variance dollar figure")
}

// TestLaborCost_ActualBasisNonFinancialHidesDollars: a caller WITHOUT
// financial:read (kitchen role) sees worked hours + labor-% only — never a
// labor-$ amount, per-person dollars, or net sales.
func TestLaborCost_ActualBasisNonFinancialHidesDollars(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupLaborHandlerDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.StaffPosition{}, &database.TimeEntry{}))
	business := createAccountingHandlerBusiness(t, "0xActualNF")
	srv := createAccountingRoleStaff(t, business.ID, database.StaffRoleServer)
	kitchen := createAccountingRoleStaff(t, business.ID, database.StaffRoleKitchen)
	seedApprovedActualHours(t, business.ID, srv.ID)

	handler := NewAccountingHandler(database.GetDBWrapper())
	router := staffLaborRouter(database.StaffRoleKitchen, kitchen.ID, business.ID, handler)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/labor-cost?basis=actual", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	body := w.Body.String()
	// Hours + percentage are visible.
	assert.Contains(t, body, `"worked_hours"`)
	assert.Contains(t, body, `"labor_cost_pct"`)
	// No dollar AMOUNT anywhere (aggregate labor_cost, per-person labor_cost,
	// net_sales, and variance labor_cost are all gated off).
	assert.NotContains(t, body, `"labor_cost":`, "non-financial caller must not see a labor-$ amount")
	assert.NotContains(t, body, `"net_sales":`, "non-financial caller must not see net sales $")
	assert.NotContains(t, body, `"payroll_basis_labor_cost":`, "non-financial caller must not see the payroll-$ baseline")
}
