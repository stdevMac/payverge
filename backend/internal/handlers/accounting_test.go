package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAccountingHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.User{},
		&database.Staff{},
		&database.StaffPermissionDeny{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.WithdrawalHistory{},
		&database.AlternativePayment{},
		&database.BusinessMilestoneEvent{},
		&database.BusinessRevenueAggregate{},
		&database.ExchangeRate{},
		&database.ManualLedgerEntry{},
		&database.AccountingPeriodLock{},
		&database.PayrollRun{},
		&database.PayrollLineItem{},
	))
	server.InitializeRBAC(database.GetDBWrapper())
	return gormDB
}

func createAccountingHandlerBusiness(t *testing.T, ownerAddress string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:      fmt.Sprintf("biz-%s", ownerAddress),
		Name:            "Accounting Biz",
		OwnerAddress:    ownerAddress,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func createAccountingRoleStaff(t *testing.T, businessID uint, role database.StaffRole) *database.Staff {
	t.Helper()

	staff := &database.Staff{
		BusinessID: businessID,
		Email:      fmt.Sprintf("%s@example.com", role),
		Name:       string(role),
		Role:       role,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, database.GetDB().Create(staff).Error)
	return staff
}

func createAccountingHandlerExchangeRate(t *testing.T, fromCurrency, toCurrency string, rate float64, fetchedAt time.Time) *database.ExchangeRate {
	t.Helper()

	record := &database.ExchangeRate{
		FromCurrency: fromCurrency,
		ToCurrency:   toCurrency,
		Rate:         rate,
		Source:       "test",
		FetchedAt:    fetchedAt,
	}
	require.NoError(t, database.GetDB().Create(record).Error)
	return record
}

func performAccountingRequest(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var requestBody *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		requestBody = bytes.NewReader(payload)
	} else {
		requestBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, requestBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestAccountingRoutes_OwnerCanReadSummary(t *testing.T) {
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

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/summary", server.RoleBasedAccessMiddleware("financial:read"), handler.GetSummary)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/accounting/summary?start=2026-01-01&end=2026-01-31", business.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAccountingRoutes_OwnerCanReadSummaryByBusinessSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwnerSlug")
	business.BusinessId = fmt.Sprintf("accounting-slug-%d", time.Now().UnixNano())
	require.NoError(t, database.GetDB().Save(business).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerSlug")
		c.Set("business_owner_address", "0xOwnerSlug")
		c.Next()
	})

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/summary", server.RoleBasedAccessMiddleware("financial:read"), handler.GetSummary)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%s/accounting/summary?start=2026-01-01&end=2026-01-31", business.BusinessId), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAccountingRoutes_ManagerCanCreateManualEntry(t *testing.T) {
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

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.POST("/inside/businesses/:id/accounting/entries", server.RoleBasedAccessMiddleware("financial:write"), handler.CreateManualEntry)

	w := performAccountingRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/accounting/entries", business.ID), map[string]any{
		"entry_type":  "expense",
		"category":    "inventory",
		"amount":      25.5,
		"currency":    "USD",
		"occurred_at": time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"description": "Produce restock",
	})

	assert.Equal(t, http.StatusCreated, w.Code)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.ManualLedgerEntry{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAccountingRoutes_ManagerCannotCreateUnconvertibleManualEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleManager)

	createAccountingHandlerExchangeRate(t, "USDC", "ARS", 1000, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", staff.ID)
		c.Set("staff_business_id", float64(business.ID))
		c.Next()
	})

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.POST("/inside/businesses/:id/accounting/entries", server.RoleBasedAccessMiddleware("financial:write"), handler.CreateManualEntry)

	w := performAccountingRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/accounting/entries", business.ID), map[string]any{
		"entry_type":  "income",
		"category":    "service",
		"amount":      25.0,
		"currency":    "BTC",
		"occurred_at": time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"description": "Manual crypto income",
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "cannot be converted")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.ManualLedgerEntry{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestAccountingRoutes_SummarySkipsLegacyUnconvertibleEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	business.DefaultCurrency = "ARS"
	require.NoError(t, database.GetDB().Save(business).Error)

	start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	createAccountingHandlerExchangeRate(t, "USDC", "USD", 1, start)
	createAccountingHandlerExchangeRate(t, "USDC", "ARS", 1000, start)

	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "service",
		Amount:      10,
		Currency:    "USD",
		OccurredAt:  start.Add(2 * time.Hour),
		Description: "Convertible manual income",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.ManualLedgerEntry{
		BusinessID:  business.ID,
		EntryType:   database.AccountingEntryTypeIncome,
		Category:    "service",
		Amount:      10,
		Currency:    "BTC",
		OccurredAt:  start.Add(3 * time.Hour),
		Description: "Legacy BTC manual income",
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwner")
		c.Set("business_owner_address", "0xOwner")
		c.Next()
	})

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/summary", server.RoleBasedAccessMiddleware("financial:read"), handler.GetSummary)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/accounting/summary?start=2026-03-01&end=2026-03-31", business.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"skipped_manual_entries\":1")
	assert.Contains(t, w.Body.String(), "\"code\":\"fx_rate_missing\"")
	assert.Contains(t, w.Body.String(), "\"scope\":\"manual_income\"")
}

func TestAccountingRoutes_ServerIsForbidden(t *testing.T) {
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

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/summary", server.RoleBasedAccessMiddleware("financial:read"), handler.GetSummary)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/accounting/summary?start=2026-01-01&end=2026-01-31", business.ID), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAccountingRoutes_ManagerCannotAccessOtherBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	otherBusiness := createAccountingHandlerBusiness(t, "0xOwnerB")
	staff := createAccountingRoleStaff(t, otherBusiness.ID, database.StaffRoleManager)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", staff.ID)
		c.Set("staff_business_id", float64(otherBusiness.ID))
		c.Next()
	})

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/summary", server.RoleBasedAccessMiddleware("financial:read"), handler.GetSummary)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/accounting/summary?start=2026-01-01&end=2026-01-31", business.ID), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestPayrollWritePermissionIsOwnerOnly locks PAY-1: payroll WRITES are gated by
// a dedicated payroll:write permission that no default staff role carries (owners
// bypass all checks; an owner may still grant it to a specific manager via custom
// permissions). Payroll READS stay on financial:read, which managers keep.
func TestPayrollWritePermissionIsOwnerOnly(t *testing.T) {
	assert.Equal(t, "payroll:write", string(server.PermPayrollWrite))

	for role, perms := range server.StaffRolePermissions {
		for _, p := range perms {
			assert.NotEqualf(t, server.PermPayrollWrite, p,
				"role %s must not carry payroll:write by default (owner-only)", role)
		}
	}

	// Managers must retain financial:read so payroll stays viewable to them.
	managerPerms := server.StaffRolePermissions[database.StaffRoleManager]
	assert.Containsf(t, managerPerms, server.PermFinancialRead,
		"manager must keep financial:read for payroll visibility")
}

// TestAccountingRoutes_ManagerWithGrantedPayrollWriteMarkPaidStoresAuditActor is the
// PAY-1 successor to the old manager-mark-paid test: a manager can only mark paid
// when explicitly granted payroll:write, and the audit actor is still recorded.
func TestAccountingRoutes_ManagerWithGrantedPayrollWriteMarkPaidStoresAuditActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleManager)

	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusDraft,
		GrossTotal:  100,
		NetTotal:    100,
	}
	require.NoError(t, database.GetDB().Create(run).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", staff.ID)
		c.Set("staff_business_id", float64(business.ID))
		// Owner has explicitly delegated payroll to this manager.
		c.Set("staff_custom_permissions", `["payroll:write"]`)
		c.Next()
	})

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.POST("/inside/businesses/:id/accounting/payroll-runs/:runId/mark-paid", server.RoleBasedAccessMiddleware(string(server.PermPayrollWrite)), handler.MarkPayrollRunPaid)

	w := performAccountingRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs/%d/mark-paid", business.ID, run.ID), map[string]any{})
	assert.Equal(t, http.StatusOK, w.Code)

	var persisted database.PayrollRun
	require.NoError(t, database.GetDB().First(&persisted, run.ID).Error)
	assert.Equal(t, database.PayrollRunStatusPaid, persisted.Status)
	require.NotNil(t, persisted.PaidByStaffID)
	assert.Equal(t, staff.ID, *persisted.PaidByStaffID)
}

// TestAccountingRoutes_ManagerCannotMarkPaidWithoutPayrollWrite verifies a plain
// manager (no granted payroll:write) is forbidden from marking payroll paid and
// the run is left untouched.
func TestAccountingRoutes_ManagerCannotMarkPaidWithoutPayrollWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")
	staff := createAccountingRoleStaff(t, business.ID, database.StaffRoleManager)

	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusDraft,
		GrossTotal:  100,
		NetTotal:    100,
	}
	require.NoError(t, database.GetDB().Create(run).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", staff.ID)
		c.Set("staff_business_id", float64(business.ID))
		c.Next()
	})

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.POST("/inside/businesses/:id/accounting/payroll-runs/:runId/mark-paid", server.RoleBasedAccessMiddleware(string(server.PermPayrollWrite)), handler.MarkPayrollRunPaid)

	w := performAccountingRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs/%d/mark-paid", business.ID, run.ID), map[string]any{})
	assert.Equal(t, http.StatusForbidden, w.Code)

	var persisted database.PayrollRun
	require.NoError(t, database.GetDB().First(&persisted, run.ID).Error)
	assert.Equal(t, database.PayrollRunStatusDraft, persisted.Status, "forbidden request must not mutate the run")
}

// TestAccountingRoutes_OwnerCanMarkPaid verifies the business owner (who bypasses
// the permission system) can mark a run paid even on the payroll:write-gated route.
func TestAccountingRoutes_OwnerCanMarkPaid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusDraft,
		GrossTotal:  100,
		NetTotal:    100,
	}
	require.NoError(t, database.GetDB().Create(run).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwner")
		c.Set("business_owner_address", "0xOwner")
		c.Next()
	})

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.POST("/inside/businesses/:id/accounting/payroll-runs/:runId/mark-paid", server.RoleBasedAccessMiddleware(string(server.PermPayrollWrite)), handler.MarkPayrollRunPaid)

	w := performAccountingRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs/%d/mark-paid", business.ID, run.ID), map[string]any{})
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAccountingRoutes_ManagerCanListPayrollRuns verifies payroll READS remain
// available to managers (financial:read), even though writes are owner-only.
func TestAccountingRoutes_ManagerCanListPayrollRuns(t *testing.T) {
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

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/payroll-runs", server.RoleBasedAccessMiddleware("financial:read"), handler.ListPayrollRuns)

	w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs?start=2026-03-01&end=2026-03-31", business.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestAccountingRoutes_OwnerCanDeleteDraftRun verifies the owner can delete a
// draft payroll run via the payroll:write-gated DELETE route.
func TestAccountingRoutes_OwnerCanDeleteDraftRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusDraft,
		GrossTotal:  100,
		NetTotal:    100,
	}
	require.NoError(t, database.GetDB().Create(run).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwner")
		c.Set("business_owner_address", "0xOwner")
		c.Next()
	})

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.DELETE("/inside/businesses/:id/accounting/payroll-runs/:runId", server.RoleBasedAccessMiddleware(string(server.PermPayrollWrite)), handler.DeletePayrollRun)

	w := performAccountingRequest(t, router, http.MethodDelete, fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs/%d", business.ID, run.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.PayrollRun{}).Where("id = ?", run.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

// TestAccountingRoutes_OwnerCanVoidPaidRun verifies the owner can void a paid run
// and the audit actor is recorded.
func TestAccountingRoutes_OwnerCanVoidPaidRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xOwner")

	paidAt := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC)
	run := &database.PayrollRun{
		BusinessID:  business.ID,
		PeriodStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		Status:      database.PayrollRunStatusPaid,
		PaidAt:      &paidAt,
		GrossTotal:  100,
		NetTotal:    100,
	}
	require.NoError(t, database.GetDB().Create(run).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwner")
		c.Set("business_owner_address", "0xOwner")
		c.Next()
	})

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.POST("/inside/businesses/:id/accounting/payroll-runs/:runId/void", server.RoleBasedAccessMiddleware(string(server.PermPayrollWrite)), handler.VoidPayrollRun)

	w := performAccountingRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs/%d/void", business.ID, run.ID), map[string]any{})
	assert.Equal(t, http.StatusOK, w.Code)

	var persisted database.PayrollRun
	require.NoError(t, database.GetDB().First(&persisted, run.ID).Error)
	assert.Equal(t, database.PayrollRunStatusVoid, persisted.Status)
	require.NotNil(t, persisted.VoidedAt)
}

// TestListPayrollRuns_DBErrorDoesNotLeakRawMessage verifies that a non-sentinel
// DB error from ListPayrollRuns is returned as 500 with a static client-safe
// message and does NOT expose the raw GORM/SQL error string to the caller.
func TestListPayrollRuns_DBErrorDoesNotLeakRawMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Open a minimal in-memory SQLite DB that only has the Business and Staff
	// tables — PayrollRun is intentionally absent so any list query fails.
	dsn := fmt.Sprintf("file:%s_nopayroll?mode=memory&cache=shared", t.Name())
	bareDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := bareDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, bareDB.AutoMigrate(
		&database.Business{},
		&database.User{},
		&database.Staff{},
	))

	database.SetTestDB(bareDB)
	server.InitializeRBAC(database.GetDBWrapper())

	business := &database.Business{
		BusinessId:      "leak-test-biz",
		Name:            "Leak Test",
		OwnerAddress:    "0xLeakOwner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, bareDB.Create(business).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xLeakOwner")
		c.Set("business_owner_address", "0xLeakOwner")
		c.Next()
	})

	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/accounting/payroll-runs", server.RoleBasedAccessMiddleware("financial:read"), handler.ListPayrollRuns)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs?start=2026-01-01&end=2026-01-31", business.ID), nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "DB error must surface as 500")

	body := w.Body.String()
	// Raw SQL / GORM internals must not reach the caller.
	assert.NotContains(t, body, "no such table", "raw SQLite error must not leak")
	assert.NotContains(t, body, "payroll_runs", "raw table name must not leak")
	assert.NotContains(t, body, "failed to", "wrapped error prefix must not leak")
}

// maxPayrollRunDetailBodyBytes is the defendable wire ceiling for GET
// payroll-run detail with 5 line items + actor staff. Pre-fix payloads were
// ~34 KB from recursive empty GORM relations (empty Business + nested
// PayrollRun on every line). Post-fix measured 2253 bytes for the full
// HTTP envelope; ceiling is that measurement with ~80% headroom (see
// summary.md L6-16 entry).
const maxPayrollRunDetailBodyBytes = 4096

// TestGetPayrollRun_DetailOmitsEmptyRelationStructs is B-10 (Finding L6-16):
// GET /accounting/payroll-runs/:runId must not recursively serialize zero-value
// GORM relations (business / payroll_run / staff nests) while still emitting
// FE-required fields (dollar line amounts, actor names, totals, status, dates).
func TestGetPayrollRun_DetailOmitsEmptyRelationStructs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xPayrollDetail")
	createdBy := createAccountingRoleStaff(t, business.ID, database.StaffRoleManager)
	// Rename for a stable audit-name assertion (createAccountingRoleStaff uses role as name).
	require.NoError(t, database.GetDB().Model(createdBy).Update("name", "Creator Manager").Error)
	createdBy.Name = "Creator Manager"

	paidBy := &database.Staff{
		BusinessID: business.ID,
		Email:      "payer@example.com",
		Name:       "Payer Manager",
		Role:       database.StaffRoleManager,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, database.GetDB().Create(paidBy).Error)

	voidedBy := &database.Staff{
		BusinessID: business.ID,
		Email:      "voider@example.com",
		Name:       "Voider Manager",
		Role:       database.StaffRoleManager,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, database.GetDB().Create(voidedBy).Error)

	// Five payee staff so line items carry staff_id references (relation not preloaded).
	payeeStaff := make([]*database.Staff, 5)
	for i := 0; i < 5; i++ {
		s := &database.Staff{
			BusinessID: business.ID,
			Email:      fmt.Sprintf("payee%d@example.com", i),
			Name:       fmt.Sprintf("Payee %d", i),
			Role:       database.StaffRoleServer,
			IsActive:   true,
			InvitedBy:  "owner@example.com",
		}
		require.NoError(t, database.GetDB().Create(s).Error)
		payeeStaff[i] = s
	}

	periodStart := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	paidAt := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	voidedAt := time.Date(2026, time.March, 17, 9, 0, 0, 0, time.UTC)
	userID := uint(42)

	run := &database.PayrollRun{
		BusinessID:       business.ID,
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		Status:           database.PayrollRunStatusVoid,
		Currency:         "USD",
		GrossTotal:       50000, // $500.00
		BonusTotal:       2500,  // $25.00
		DeductionTotal:   1000,  // $10.00
		NetTotal:         51500, // $515.00
		Notes:            "detail-drawer-fixture",
		CreatedByUserID:  &userID,
		CreatedByStaffID: &createdBy.ID,
		PaidAt:           &paidAt,
		PaidByUserID:     &userID,
		PaidByStaffID:    &paidBy.ID,
		VoidedAt:         &voidedAt,
		VoidedByUserID:   &userID,
		VoidedByStaffID:  &voidedBy.ID,
	}
	require.NoError(t, database.GetDB().Create(run).Error)

	for i := 0; i < 5; i++ {
		sid := payeeStaff[i].ID
		require.NoError(t, database.GetDB().Create(&database.PayrollLineItem{
			PayrollRunID:    run.ID,
			BusinessID:      business.ID,
			PayeeType:       database.PayrollPayeeTypeStaff,
			StaffID:         &sid,
			PayeeName:       payeeStaff[i].Name,
			GrossAmount:     10000, // $100.00
			BonusAmount:     500,   // $5.00
			DeductionAmount: 200,   // $2.00
			NetAmount:       10300, // $103.00
			Notes:           fmt.Sprintf("line-%d", i),
		}).Error)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xPayrollDetail")
		c.Set("business_owner_address", "0xPayrollDetail")
		c.Next()
	})
	handler := NewAccountingHandler(database.GetDBWrapper())
	router.GET(
		"/inside/businesses/:id/accounting/payroll-runs/:runId",
		server.RoleBasedAccessMiddleware("financial:read"),
		handler.GetPayrollRun,
	)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/payroll-runs/%d", business.ID, run.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	body := w.Body.Bytes()
	bodyStr := string(body)

	// (a) No empty relation structs — assert specific absent zero-value nests.
	assert.NotContains(t, bodyStr, `"business":{"id":0`,
		"must not emit zero-value business relation (run or line items)")
	assert.NotContains(t, bodyStr, `"payroll_run":{"id":0`,
		"must not emit recursive zero-value payroll_run on line items")
	assert.NotContains(t, bodyStr, `"payroll_run":{`,
		"line items must not nest payroll_run at all")
	// Empty staff nest (unloaded payee staff on line items) — zero-id object.
	assert.NotContains(t, bodyStr, `"staff":{"id":0`,
		"must not emit zero-value staff relation on line items")
	// Full empty business dump also shows settlement_address zero-shape keys.
	assert.NotContains(t, bodyStr, `"settlement_address":""`,
		"empty Business embeds leak settlement_address; must be scrubbed")

	// (b) Body size under defendable ceiling for a 5-line run.
	assert.LessOrEqual(t, len(body), maxPayrollRunDetailBodyBytes,
		"5-line payroll detail body is %d bytes (ceiling %d); empty relation bloat regressed",
		len(body), maxPayrollRunDetailBodyBytes)

	// (c) FE + audit drawer fields present with dollar amounts and actor names.
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.True(t, envelope.Success)

	var data map[string]any
	require.NoError(t, json.Unmarshal(envelope.Data, &data))

	assert.Equal(t, "void", data["status"])
	assert.Equal(t, "USD", data["currency"])
	assert.InDelta(t, 500.0, data["gross_total"], 0.001)
	assert.InDelta(t, 25.0, data["bonus_total"], 0.001)
	assert.InDelta(t, 10.0, data["deduction_total"], 0.001)
	assert.InDelta(t, 515.0, data["net_total"], 0.001)
	assert.NotEmpty(t, data["period_start"])
	assert.NotEmpty(t, data["period_end"])
	assert.NotEmpty(t, data["paid_at"])
	assert.NotEmpty(t, data["voided_at"])
	assert.Equal(t, "detail-drawer-fixture", data["notes"])

	createdStaff, ok := data["created_by_staff"].(map[string]any)
	require.True(t, ok, "created_by_staff must be present")
	assert.Equal(t, "Creator Manager", createdStaff["name"])

	paidStaff, ok := data["paid_by_staff"].(map[string]any)
	require.True(t, ok, "paid_by_staff must be present")
	assert.Equal(t, "Payer Manager", paidStaff["name"])

	voidedStaff, ok := data["voided_by_staff"].(map[string]any)
	require.True(t, ok, "voided_by_staff must be present")
	assert.Equal(t, "Voider Manager", voidedStaff["name"])

	lines, ok := data["line_items"].([]any)
	require.True(t, ok)
	require.Len(t, lines, 5)
	for i, raw := range lines {
		line, ok := raw.(map[string]any)
		require.Truef(t, ok, "line %d", i)
		assert.Equal(t, fmt.Sprintf("Payee %d", i), line["payee_name"])
		assert.InDelta(t, 100.0, line["gross_amount"], 0.001)
		assert.InDelta(t, 5.0, line["bonus_amount"], 0.001)
		assert.InDelta(t, 2.0, line["deduction_amount"], 0.001)
		assert.InDelta(t, 103.0, line["net_amount"], 0.001)
		assert.Equal(t, "staff", line["payee_type"])
		// Line-level nested relations must be absent (not empty objects).
		_, hasBiz := line["business"]
		assert.Falsef(t, hasBiz, "line %d must not include business key", i)
		_, hasRun := line["payroll_run"]
		assert.Falsef(t, hasRun, "line %d must not include payroll_run key", i)
		_, hasStaff := line["staff"]
		assert.Falsef(t, hasStaff, "line %d must not include staff key", i)
	}

	// Top-level business relation must also be absent when unloaded.
	_, hasTopBiz := data["business"]
	assert.False(t, hasTopBiz, "run must not include unloaded business relation")

	// Sanity: response must still look like a real detail payload, not truncated junk.
	assert.True(t, strings.Contains(bodyStr, `"line_items"`), "line_items key required")
	t.Logf("B-10 payroll detail body size: %d bytes", len(body))
}
