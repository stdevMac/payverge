package server

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func contains(perms []string, want string) bool {
	for _, p := range perms {
		if p == want {
			return true
		}
	}
	return false
}

// TestResolveContextPermissions_Web3OwnerAllAccess: a verified web3 owner holds
// all permissions, so the SSE handler must give them the un-scoped stream.
func TestResolveContextPermissions_Web3OwnerAllAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rbacMiddleware = &RBACMiddleware{db: nil}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "web3")
	c.Set("address", "0xOwner")
	c.Set("business_owner_address", "0xowner")

	_, allAccess := ResolveContextPermissions(c)
	assert.True(t, allAccess, "verified web3 owner must resolve to all-access")
}

// TestResolveContextPermissions_Web3MismatchDenied: a web3 token whose address
// does not match the business owner must NOT get all-access (it returns deny).
func TestResolveContextPermissions_Web3MismatchDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rbacMiddleware = &RBACMiddleware{db: nil}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "web3")
	c.Set("address", "0xAttacker")
	c.Set("business_owner_address", "0xowner")

	perms, allAccess := ResolveContextPermissions(c)
	assert.False(t, allAccess, "mismatched web3 address must not be all-access")
	assert.Empty(t, perms)
}

// TestResolveContextPermissions_KitchenStaffScoped: a Kitchen staff role must
// resolve to its concrete permission set (NOT all-access), and that set must
// include orders/bills but exclude financial:read and reservations:read.
func TestResolveContextPermissions_KitchenStaffScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rbacMiddleware = &RBACMiddleware{db: nil}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "staff")
	c.Set("staff_role", string(database.StaffRoleKitchen))

	perms, allAccess := ResolveContextPermissions(c)
	require.False(t, allAccess, "kitchen staff must NOT resolve to all-access")

	// Has what it needs.
	assert.True(t, contains(perms, "orders:read"), "kitchen must have orders:read")
	assert.True(t, contains(perms, "bills:read"), "kitchen must have bills:read")
	assert.True(t, contains(perms, "delivery:dispatch:read"))
	assert.True(t, contains(perms, "overview:read"))

	// Denied the sensitive ones — this is what keeps payment/reservation SSE
	// frames off the kitchen stream.
	assert.False(t, contains(perms, "financial:read"), "kitchen must NOT have financial:read")
	assert.False(t, contains(perms, "reservations:read"), "kitchen must NOT have reservations:read")
}

// TestResolveContextPermissions_ManagerHasFinancialRead: a Manager role holds
// financial:read, so it stays subscribed to payment.received frames.
func TestResolveContextPermissions_ManagerHasFinancialRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rbacMiddleware = &RBACMiddleware{db: nil}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "staff")
	c.Set("staff_role", string(database.StaffRoleManager))

	perms, allAccess := ResolveContextPermissions(c)
	require.False(t, allAccess)
	assert.True(t, contains(perms, "financial:read"), "manager must have financial:read")
	assert.True(t, contains(perms, "reservations:read"))
}

// TestResolveContextPermissions_NoTokenDenied: a context with no token_type must
// resolve to deny (no permissions, no all-access).
func TestResolveContextPermissions_NoTokenDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rbacMiddleware = &RBACMiddleware{db: nil}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	perms, allAccess := ResolveContextPermissions(c)
	assert.False(t, allAccess)
	assert.Empty(t, perms)
}

// TestResolveContextEventPermissions_WildcardGrantExactDeny proves that SSE
// evaluates the concrete event permission with the same grant-plus-deny
// semantics as REST. A raw effective-list filter would incorrectly retain the
// financial:* wildcard and leak payment events despite the exact deny.
func TestResolveContextEventPermissions_WildcardGrantExactDeny(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rbacMiddleware = &RBACMiddleware{db: nil}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "staff")
	c.Set("staff_role", string(database.StaffRoleKitchen))
	c.Set("staff_id", uint(42))
	c.Set("staff_custom_permissions", `["financial:*"]`)
	c.Set("staff_permission_denies", []string{"financial:read"})

	checker, allAccess := ResolveContextEventPermissions(c)
	require.False(t, allAccess)
	require.NotNil(t, checker)
	assert.False(t, checker("financial:read"), "exact deny must override financial:* for payment events")
	assert.True(t, checker("financial:write"), "an allowed sibling covered by financial:* must remain effective")
	assert.True(t, checker("orders:read"), "unrelated role permissions must remain effective")
}

func TestResolveContextEventPermissions_NonDelegableOwnerOnlyDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rbacMiddleware = &RBACMiddleware{db: nil}

	const permission = "sse:test-owner-only"
	OwnerOnlyPermissions[Permission(permission)] = struct{}{}
	t.Cleanup(func() { delete(OwnerOnlyPermissions, Permission(permission)) })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "staff")
	c.Set("staff_role", string(database.StaffRoleKitchen))
	c.Set("staff_id", uint(43))
	c.Set("staff_custom_permissions", `["sse:test-owner-only"]`)
	c.Set("staff_permission_denies", []string{})

	checker, allAccess := ResolveContextEventPermissions(c)
	require.False(t, allAccess)
	require.NotNil(t, checker)
	assert.False(t, checker(permission), "non-delegable owner-only permissions must never become effective for staff")
}

func TestResolveContextEventPermissions_ConsolidatesFallbackQueries(t *testing.T) {
	gormDB := setupEffectiveDenyTestDB(t)
	rbacMiddleware = &RBACMiddleware{db: database.NewDB()}

	staff := database.Staff{
		BusinessID:        7,
		Email:             "sse-query-shape@example.test",
		Name:              "SSE Query Shape",
		Role:              database.StaffRoleKitchen,
		IsActive:          true,
		InvitedBy:         "0xowner",
		CustomPermissions: `["financial:*"]`,
	}
	require.NoError(t, gormDB.Create(&staff).Error)
	require.NoError(t, gormDB.Create(&database.StaffPermissionDeny{
		BusinessID: staff.BusinessID,
		StaffID:    staff.ID,
		Permission: "financial:read",
		CreatedBy:  "0xowner",
	}).Error)

	queryCount := 0
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register("count_sse_permission_queries", func(_ *gorm.DB) {
		queryCount++
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "staff")
	c.Set("staff_role", string(staff.Role))
	c.Set("staff_id", staff.ID)
	// Omit the hydrated grant/deny keys to exercise the compatibility fallback.

	checker, allAccess := ResolveContextEventPermissions(c)
	require.False(t, allAccess)
	require.NotNil(t, checker)
	assert.Equal(t, 2, queryCount, "one grant query plus one deny query should resolve the whole stream")

	for _, required := range []string{"financial:read", "financial:write", "orders:read", "bills:read"} {
		_ = checker(required)
	}
	assert.Equal(t, 2, queryCount, "checking every concrete event permission must not issue per-topic queries")
}
