package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCashRegisterRBACOwnerCanReadAndOperate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterRBACTestDB(t)

	assertCashRegisterMiddlewareAccess(t, func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwner")
		c.Set("business_owner_address", "0xOwner")
	}, true)
}

func setupCashRegisterRBACTestDB(t *testing.T) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dsnName)), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	InitializeRBAC(database.GetDBWrapper())
}

func TestCashRegisterRBACManagerCanReadAndOperate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterRBACTestDB(t)

	assertCashRegisterMiddlewareAccess(t, cashRegisterStaffContext(database.StaffRoleManager), true)
	assertCashRegisterRolePermissions(t, database.StaffRoleManager, true)
}

func TestCashRegisterRBACServerCanReadAndOperate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterRBACTestDB(t)

	assertCashRegisterMiddlewareAccess(t, cashRegisterStaffContext(database.StaffRoleServer), true)
	assertCashRegisterRolePermissions(t, database.StaffRoleServer, true)
}

func TestCashRegisterRBACHostCannotReadOrOperate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterRBACTestDB(t)

	assertCashRegisterMiddlewareAccess(t, cashRegisterStaffContext(database.StaffRoleHost), false)
	assertCashRegisterRolePermissions(t, database.StaffRoleHost, false)
}

func TestCashRegisterRBACKitchenCannotReadOrOperate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterRBACTestDB(t)

	assertCashRegisterMiddlewareAccess(t, cashRegisterStaffContext(database.StaffRoleKitchen), false)
	assertCashRegisterRolePermissions(t, database.StaffRoleKitchen, false)
}

func assertCashRegisterMiddlewareAccess(t *testing.T, setContext func(*gin.Context), wantAllowed bool) {
	t.Helper()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		setContext(c)
		c.Next()
	})
	router.GET("/businesses/:id/cash-register/current", RoleBasedAccessMiddleware(string(PermCashRegisterRead)), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.POST("/businesses/:id/cash-register/sessions", RoleBasedAccessMiddleware(string(PermCashRegisterOperate)), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	readReq := httptest.NewRequest(http.MethodGet, "/businesses/123/cash-register/current", nil)
	readResp := httptest.NewRecorder()
	router.ServeHTTP(readResp, readReq)
	require.Equal(t, cashRegisterExpectedStatus(wantAllowed), readResp.Code)

	operateReq := httptest.NewRequest(http.MethodPost, "/businesses/123/cash-register/sessions", nil)
	operateResp := httptest.NewRecorder()
	router.ServeHTTP(operateResp, operateReq)
	require.Equal(t, cashRegisterExpectedStatus(wantAllowed), operateResp.Code)
}

func cashRegisterStaffContext(role database.StaffRole) func(*gin.Context) {
	return func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(role))
		c.Set("staff_id", uint(42))
		c.Set("staff_business_id", float64(123))
		// Production staff hydration always supplies the authoritative deny set.
		c.Set("staff_permission_denies", []string{})
	}
}

func cashRegisterExpectedStatus(allowed bool) int {
	if allowed {
		return http.StatusNoContent
	}
	return http.StatusForbidden
}

func assertCashRegisterRolePermissions(t *testing.T, role database.StaffRole, wantAllowed bool) {
	t.Helper()

	perms := StaffRolePermissions[role]
	if wantAllowed {
		require.Contains(t, perms, PermCashRegisterRead)
		require.Contains(t, perms, PermCashRegisterOperate)
		return
	}
	require.NotContains(t, perms, PermCashRegisterRead)
	require.NotContains(t, perms, PermCashRegisterOperate)
}
