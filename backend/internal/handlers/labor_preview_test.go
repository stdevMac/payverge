package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newLaborPreviewTestServer wires the labor-preview route against an isolated
// in-memory DB (with the StaffPosition + BusinessScheduleSettings tables the
// scheduled calculator reads) and hydrates the staff gin-context from the actor so
// ResolveContextPermissions classifies manager (financial:read) vs plain staff.
func newLaborPreviewTestServer(t *testing.T) (do func(role string, staffID uint, url string) *httptest.ResponseRecorder, g *gorm.DB, cleanup func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{}, &database.Staff{}, &database.StaffPermissionDeny{}, &database.Position{}, &database.StaffPosition{},
		&database.Schedule{}, &database.Shift{}, &database.BusinessScheduleSettings{},
	))
	database.SetTestDB(gormDB)
	server.InitializeRBAC(database.GetDBWrapper())

	h := NewScheduleHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", c.GetHeader("X-Test-Role"))
		if v, err := strconv.ParseUint(c.GetHeader("X-Test-Staff"), 10, 64); err == nil {
			c.Set("staff_id", uint(v))
		}
		c.Next()
	})
	r.GET("/b/:id/schedule/:scheduleId/labor-preview", h.LaborPreview)

	do = func(role string, staffID uint, url string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		require.NoError(t, err)
		req.Header.Set("X-Test-Role", role)
		req.Header.Set("X-Test-Staff", strconv.FormatUint(uint64(staffID), 10))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	return do, gormDB, func() { _ = sqlDB.Close() }
}

func seedLaborPreviewFixture(t *testing.T, g *gorm.DB) {
	t.Helper()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.Position{ID: 3, BusinessID: 1, Name: "Server"}).Error)
	require.NoError(t, g.Create(&database.StaffPosition{BusinessID: 1, StaffID: 7, PositionID: 3, PayRateCents: 2000, IsPrimary: true}).Error)
	// Draft schedule for the current week; one 8h assigned shift = $160 scheduled labor.
	require.NoError(t, g.Create(&database.Schedule{ID: 9, BusinessID: 1, WeekStart: time.Now().UTC().Truncate(24 * time.Hour), Status: database.ScheduleStatusDraft}).Error)
	start := time.Now().UTC()
	staffID := uint(7)
	require.NoError(t, g.Create(&database.Shift{
		BusinessID: 1, ScheduleID: 9, StaffID: &staffID, PositionID: 3,
		StartsAt: start, EndsAt: start.Add(8 * time.Hour), CreatedByStaffID: 1,
	}).Error)
}

// TestLaborPreviewMoneyIsolation is the slice's #1 check: a non-financial caller
// gets hours + warnings with ZERO dollar fields and no labor_cost_pct, while a
// financial caller (manager holds financial:read) additionally gets
// labor_cost/sales_target/labor_cost_pct.
func TestLaborPreviewMoneyIsolation(t *testing.T) {
	do, g, cleanup := newLaborPreviewTestServer(t)
	defer cleanup()
	seedLaborPreviewFixture(t, g)

	url := "/b/1/schedule/9/labor-preview?salesTarget=1000"

	// --- Non-financial caller (server: has neither financial:read nor would reach
	// the route in prod, but the handler gate is what we prove). ---
	wsrv := do("server", 7, url)
	require.Equal(t, http.StatusOK, wsrv.Code, wsrv.Body.String())
	safeBody := wsrv.Body.String()
	// labor_cost_pct legitimately contains "labor_cost" as a substring — scrub it
	// before asserting the dollar field is absent.
	scrubbed := strings.ReplaceAll(safeBody, "labor_cost_pct", "X_PCT")
	require.NotContains(t, scrubbed, "labor_cost", "non-financial response must not carry a labor_cost dollar field")
	require.NotContains(t, scrubbed, "sales_target", "non-financial response must not echo the sales target dollars")
	require.NotContains(t, strings.ToLower(scrubbed), "pay_rate")
	require.NotContains(t, scrubbed, "cents")
	require.NotContains(t, safeBody, "$")
	// labor_cost_pct is a rate derived from dollars — it stays on the financial
	// DTO only. The safe body still carries hours, warnings, and can_see_dollars.
	require.NotContains(t, safeBody, "labor_cost_pct")
	var safeEnvelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wsrv.Body.Bytes(), &safeEnvelope))
	_, hasLaborPct := safeEnvelope.Data["labor_cost_pct"]
	require.False(t, hasLaborPct, "non-financial JSON must omit the labor_cost_pct key")
	require.Contains(t, safeBody, "total_hours")
	require.Contains(t, safeBody, "warnings")
	require.Contains(t, safeBody, "\"can_see_dollars\":false")

	// schedule:write without financial:read (manager role, financial:read denied).
	require.NoError(t, g.Create(&database.StaffPermissionDeny{
		BusinessID: 1, StaffID: 11, Permission: "financial:read", CreatedBy: "owner",
	}).Error)
	wrestricted := do("manager", 11, url)
	require.Equal(t, http.StatusOK, wrestricted.Code, wrestricted.Body.String())
	require.NotContains(t, wrestricted.Body.String(), "labor_cost_pct")
	var restricted struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wrestricted.Body.Bytes(), &restricted))
	_, restrictedHasPct := restricted.Data["labor_cost_pct"]
	require.False(t, restrictedHasPct, "schedule:write-only caller must omit labor_cost_pct")
	require.Contains(t, wrestricted.Body.String(), "total_hours")
	require.Contains(t, wrestricted.Body.String(), "\"can_see_dollars\":false")

	// --- Financial caller (manager holds financial:read per rbac.go). ---
	wmgr := do("manager", 1, url)
	require.Equal(t, http.StatusOK, wmgr.Code, wmgr.Body.String())
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			LaborCostPct  float64 `json:"labor_cost_pct"`
			CanSeeDollars bool    `json:"can_see_dollars"`
			LaborCost     float64 `json:"labor_cost"`
			SalesTarget   float64 `json:"sales_target"`
			TotalHours    float64 `json:"total_hours"`
			Lines         []struct {
				PositionID uint    `json:"position_id"`
				LaborCost  float64 `json:"labor_cost"`
				Hours      float64 `json:"hours"`
			} `json:"lines"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wmgr.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.True(t, resp.Data.CanSeeDollars, "financial caller gets can_see_dollars=true")
	require.InDelta(t, 8.0, resp.Data.TotalHours, 0.01, "8h scheduled")
	require.InDelta(t, 160.0, resp.Data.LaborCost, 0.01, "8h x $20 = $160")
	require.InDelta(t, 1000.0, resp.Data.SalesTarget, 0.01)
	require.InDelta(t, 0.16, resp.Data.LaborCostPct, 0.001, "160/1000")
	require.Len(t, resp.Data.Lines, 1)
	require.InDelta(t, 160.0, resp.Data.Lines[0].LaborCost, 0.01)
}

// TestLaborPreviewScheduleNotFound returns 404 for a cross-tenant / unknown schedule.
func TestLaborPreviewScheduleNotFound(t *testing.T) {
	do, g, cleanup := newLaborPreviewTestServer(t)
	defer cleanup()
	seedLaborPreviewFixture(t, g)

	w := do("manager", 1, "/b/1/schedule/999/labor-preview")
	require.Equal(t, http.StatusNotFound, w.Code)
}

// TestLaborPreviewSalesTargetValidation rejects a negative/malformed sales target.
func TestLaborPreviewSalesTargetValidation(t *testing.T) {
	do, g, cleanup := newLaborPreviewTestServer(t)
	defer cleanup()
	seedLaborPreviewFixture(t, g)

	require.Equal(t, http.StatusBadRequest, do("manager", 1, "/b/1/schedule/9/labor-preview?salesTarget=-5").Code)
	require.Equal(t, http.StatusBadRequest, do("manager", 1, "/b/1/schedule/9/labor-preview?salesTarget=abc").Code)
	for _, raw := range []string{"Inf", "-Inf", "NaN", "1e9"} {
		w := do("manager", 1, "/b/1/schedule/9/labor-preview?salesTarget="+raw)
		require.Equal(t, http.StatusBadRequest, w.Code, "salesTarget=%s body=%s", raw, w.Body.String())
		require.Contains(t, w.Body.String(), "salesTarget must be a finite number between 0 and 100000000")
	}
}
