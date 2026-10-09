package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compRespEnvelope mirrors the {success,data} shape the compensation handler
// emits. Money is dollars at the API boundary.
type compRespEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		EmploymentType string  `json:"employment_type"`
		HourlyRate     float64 `json:"hourly_rate"`
		AnnualSalary   float64 `json:"annual_salary"`
	} `json:"data"`
}

// ownerCompRouter registers the compensation sub-resource on a router authed as
// the given web3 owner, gated exactly as production (payroll:write, owner-only).
func ownerCompRouter(ownerAddress string, h *StaffCompensationHandler) *gin.Engine {
	router := ownerRouter(ownerAddress)
	router.GET("/inside/businesses/:id/staff/:staffId/compensation",
		server.RoleBasedAccessMiddleware(string(server.PermPayrollWrite)), h.GetCompensation)
	router.PUT("/inside/businesses/:id/staff/:staffId/compensation",
		server.RoleBasedAccessMiddleware(string(server.PermPayrollWrite)), h.UpdateCompensation)
	return router
}

// TestStaffCompensation_OwnerPutThenGetRoundTrips locks the dollars<->cents
// contract: an owner PUTs dollars, the row stores cents, and the GET reflects
// the same dollars.
func TestStaffCompensation_OwnerPutThenGetRoundTrips(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleServer)

	h := NewStaffCompensationHandler(database.GetDBWrapper())
	router := ownerCompRouter("0xOwner", h)

	putBody := map[string]any{
		"employment_type": "hourly",
		"hourly_rate":     18.50,
		"annual_salary":   0,
	}
	wPut := performAccountingRequest(t, router, http.MethodPut,
		fmt.Sprintf("/inside/businesses/%d/staff/%d/compensation", business.ID, staff.ID), putBody)
	require.Equal(t, http.StatusOK, wPut.Code, "body: %s", wPut.Body.String())

	wGet := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/staff/%d/compensation", business.ID, staff.ID), nil)
	require.Equal(t, http.StatusOK, wGet.Code, "body: %s", wGet.Body.String())

	var got compRespEnvelope
	require.NoError(t, json.Unmarshal(wGet.Body.Bytes(), &got))
	assert.True(t, got.Success)
	assert.Equal(t, "hourly", got.Data.EmploymentType)
	assert.Equal(t, 18.5, got.Data.HourlyRate)
	assert.Equal(t, 0.0, got.Data.AnnualSalary)

	// Independently verify the DB stored cents, not dollars.
	var persisted database.Staff
	require.NoError(t, database.GetDB().First(&persisted, staff.ID).Error)
	assert.Equal(t, int64(1850), persisted.HourlyRateCents, "dollars must persist as cents")
	assert.Equal(t, int64(0), persisted.AnnualSalaryCents)
	assert.Equal(t, "hourly", persisted.EmploymentType)
}

// TestStaffCompensation_PutInvalidEmploymentType rejects an out-of-enum value.
func TestStaffCompensation_PutInvalidEmploymentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleServer)

	h := NewStaffCompensationHandler(database.GetDBWrapper())
	router := ownerCompRouter("0xOwner", h)

	w := performAccountingRequest(t, router, http.MethodPut,
		fmt.Sprintf("/inside/businesses/%d/staff/%d/compensation", business.ID, staff.ID),
		map[string]any{"employment_type": "contractor"})
	assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
}

// TestStaffCompensation_PutNegativeRate rejects negative money.
func TestStaffCompensation_PutNegativeRate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleServer)

	h := NewStaffCompensationHandler(database.GetDBWrapper())
	router := ownerCompRouter("0xOwner", h)

	w := performAccountingRequest(t, router, http.MethodPut,
		fmt.Sprintf("/inside/businesses/%d/staff/%d/compensation", business.ID, staff.ID),
		map[string]any{"hourly_rate": -1})
	assert.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
}

// TestStaffCompensation_PutRejectsExcessiveRate caps absurd input so dollars*100
// can never overflow int64 into an implementation-defined cents value.
func TestStaffCompensation_PutRejectsExcessiveRate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleServer)

	h := NewStaffCompensationHandler(database.GetDBWrapper())
	router := ownerCompRouter("0xOwner", h)
	path := fmt.Sprintf("/inside/businesses/%d/staff/%d/compensation", business.ID, staff.ID)

	wHourly := performAccountingRequest(t, router, http.MethodPut, path,
		map[string]any{"hourly_rate": 1e18})
	assert.Equal(t, http.StatusBadRequest, wHourly.Code, "absurd hourly rate must 400")

	wAnnual := performAccountingRequest(t, router, http.MethodPut, path,
		map[string]any{"annual_salary": 1e18})
	assert.Equal(t, http.StatusBadRequest, wAnnual.Code, "absurd annual salary must 400")

	// Nothing was persisted: the row stays at the zero defaults.
	var persisted database.Staff
	require.NoError(t, database.GetDB().First(&persisted, staff.ID).Error)
	assert.Equal(t, int64(0), persisted.HourlyRateCents)
	assert.Equal(t, int64(0), persisted.AnnualSalaryCents)
}

// TestStaffCompensation_PutEchoesPersistedValue locks read-back symmetry: the PUT
// response must equal what a follow-up GET returns (the cents-rounded value), not
// the raw unrounded input.
func TestStaffCompensation_PutEchoesPersistedValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleServer)

	h := NewStaffCompensationHandler(database.GetDBWrapper())
	router := ownerCompRouter("0xOwner", h)
	path := fmt.Sprintf("/inside/businesses/%d/staff/%d/compensation", business.ID, staff.ID)

	// 12.345 rounds to 1234 or 1235 cents depending on float representation; the
	// point is that PUT and GET agree on whatever was persisted.
	wPut := performAccountingRequest(t, router, http.MethodPut, path,
		map[string]any{"employment_type": "hourly", "hourly_rate": 12.345})
	require.Equal(t, http.StatusOK, wPut.Code, "body: %s", wPut.Body.String())
	var putResp compRespEnvelope
	require.NoError(t, json.Unmarshal(wPut.Body.Bytes(), &putResp))

	wGet := performAccountingRequest(t, router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, wGet.Code)
	var getResp compRespEnvelope
	require.NoError(t, json.Unmarshal(wGet.Body.Bytes(), &getResp))

	assert.Equal(t, getResp.Data.HourlyRate, putResp.Data.HourlyRate,
		"PUT must echo the persisted (rounded) value the GET returns")
}

// TestStaffCompensation_ManagerIsForbidden locks the owner-only gate: a manager
// (no payroll:write) must be denied even reading compensation.
func TestStaffCompensation_ManagerIsForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleManager)

	h := NewStaffCompensationHandler(database.GetDBWrapper())
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", staff.ID)
		c.Set("staff_business_id", float64(business.ID))
		c.Next()
	})
	router.GET("/inside/businesses/:id/staff/:staffId/compensation",
		server.RoleBasedAccessMiddleware(string(server.PermPayrollWrite)), h.GetCompensation)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/staff/%d/compensation", business.ID, staff.ID), nil)
	assert.Equal(t, http.StatusForbidden, w.Code, "managers lack payroll:write")
}

// TestStaffCompensation_CrossTenantIsNotFound locks the tenant guard: the owner
// of business A asking for a staffId that lives in business B (via A's own :id,
// which the owner legitimately owns) must get a 404, never B's wages.
func TestStaffCompensation_CrossTenantIsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	businessA := createAccountingHandlerBusiness(t, "0xOwnerA")
	businessB := createAccountingHandlerBusiness(t, "0xOwnerB")
	staffInB := createAccountingRoleStaff(t, businessB.ID, database.StaffRoleServer)

	// Seed B's staff with real wages so a leak would be detectable.
	require.NoError(t, database.GetDB().Model(&database.Staff{}).
		Where("id = ?", staffInB.ID).
		Updates(map[string]interface{}{"employment_type": "hourly", "hourly_rate_cents": 9999}).Error)

	h := NewStaffCompensationHandler(database.GetDBWrapper())
	// Authenticate as owner of A; address path uses A's id (which A owns), but the
	// staffId belongs to B.
	router := ownerCompRouter("0xOwnerA", h)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/staff/%d/compensation", businessA.ID, staffInB.ID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "cross-tenant staff must 404, not leak wages")
	assert.NotContains(t, w.Body.String(), "99.99")
	assert.NotContains(t, w.Body.String(), "9999")
}
