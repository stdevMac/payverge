package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/assert"
)

// newRBACTestMiddleware creates a minimal RBACMiddleware for unit tests that
// do not require database access (staff custom permissions are not tested here).
func newRBACTestMiddleware() *RBACMiddleware {
	return &RBACMiddleware{db: nil}
}

func TestRBAC_RequireAnyPermissionsRecordsOnlyGrantedCandidates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := newRBACTestMiddleware()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleHost))
	})
	router.GET("/", r.RequireAnyPermissions("print:bill", "print:receipt", "orders:kitchen"), func(c *gin.Context) {
		granted, _ := c.Get(ContextAuthorizedAnyPermissions)
		c.JSON(http.StatusOK, gin.H{"granted": granted})
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"granted":["print:bill"]}`, w.Body.String())
}

// TestRBAC_Web3_MatchingAddress verifies that a web3 token holder is granted
// permissions when their address matches the business_owner_address set by
// HybridAuthMiddleware.
func TestRBAC_Web3_MatchingAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set("token_type", "web3")
	c.Set("address", "0xOwnerAddress")
	c.Set("business_owner_address", "0xOwnerAddress")

	r := newRBACTestMiddleware()
	assert.True(t, r.hasPermissions(c, string(PermBusinessRead)),
		"web3 owner with matching address should have permissions")
}

// TestRBAC_Web3_MismatchedAddress verifies that a web3 token holder is DENIED
// when their address does NOT match business_owner_address. This guards against
// middleware-ordering bypass where the address check was skipped.
func TestRBAC_Web3_MismatchedAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set("token_type", "web3")
	c.Set("address", "0xAttackerAddress")
	c.Set("business_owner_address", "0xOwnerAddress")

	r := newRBACTestMiddleware()
	assert.False(t, r.hasPermissions(c, string(PermBusinessRead)),
		"web3 token with mismatched address must be denied")
}

// TestRBAC_Web3_CaseInsensitiveAddress verifies that Ethereum address comparison
// is case-insensitive (checksummed vs lowercase).
func TestRBAC_Web3_CaseInsensitiveAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set("token_type", "web3")
	c.Set("address", "0xABCDEF1234567890abcdef1234567890ABCDEF12")
	c.Set("business_owner_address", "0xabcdef1234567890abcdef1234567890abcdef12")

	r := newRBACTestMiddleware()
	assert.True(t, r.hasPermissions(c, string(PermBusinessRead)),
		"web3 address comparison should be case-insensitive")
}

// TestRBAC_Web3_NoBusinessOwnerAddress verifies that when business_owner_address
// is absent, an authenticated web3 user does not satisfy arbitrary permission
// middleware just because an address is present.
func TestRBAC_Web3_NoBusinessOwnerAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set("token_type", "web3")
	c.Set("address", "0xSomeAddress")
	// business_owner_address intentionally NOT set (no business in URL path)

	r := newRBACTestMiddleware()
	assert.False(t, r.hasPermissions(c, string(PermBusinessRead)),
		"web3 token without verified business scope must not satisfy permission routes")
}

func TestRBAC_Web3_UserLevelRouteRequiresExplicitClassification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set("token_type", "web3")
	c.Set("address", "0xSomeAddress")
	c.Set(ContextAllowUserLevelPermissions, true)

	r := newRBACTestMiddleware()
	assert.True(t, r.hasPermissions(c, string(PermOverviewRead)),
		"web3 user-level permission access must require explicit route classification")
}

// TestRBAC_User_MatchingUserID verifies that an OAuth user is granted permissions
// when their user_id matches the business_owner_user_id set by HybridAuthMiddleware.
func TestRBAC_User_MatchingUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set("token_type", "user")
	c.Set("user_id", float64(42)) // JWT claims come in as float64
	c.Set("business_owner_user_id", uint(42))

	r := newRBACTestMiddleware()
	assert.True(t, r.hasPermissions(c, string(PermBusinessRead)),
		"OAuth user with matching user_id should have permissions")
}

// TestRBAC_User_MismatchedUserID verifies that an OAuth user is DENIED when
// their user_id does NOT match business_owner_user_id.
func TestRBAC_User_MismatchedUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set("token_type", "user")
	c.Set("user_id", float64(99))
	c.Set("business_owner_user_id", uint(42))

	r := newRBACTestMiddleware()
	assert.False(t, r.hasPermissions(c, string(PermBusinessRead)),
		"OAuth user with mismatched user_id must be denied")
}

// SEC-8 / #305: OAuth must fail closed when business_owner_user_id is unset,
// matching the web3 path that requires AllowUserLevelPermissions().
func TestRBAC_User_NoBusinessOwnerContextFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set("token_type", "user")
	c.Set("user_id", float64(42))
	// intentionally omit business_owner_user_id

	r := newRBACTestMiddleware()
	assert.False(t, r.hasPermissions(c, string(PermTablesWrite)),
		"OAuth user without verified business scope must be denied on permission routes")
}

func TestRBAC_User_UserLevelRouteRequiresExplicitClassification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set("token_type", "user")
	c.Set("user_id", float64(42))
	c.Set(ContextAllowUserLevelPermissions, true)

	r := newRBACTestMiddleware()
	assert.True(t, r.hasPermissions(c, string(PermOverviewRead)),
		"OAuth user-level permission access must require explicit route classification")
}

// TestRBAC_Admin_AllPermissions verifies that platform admins bypass all checks.
func TestRBAC_Admin_AllPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set("token_type", "web3")
	c.Set("role", "admin")
	// No address or business_owner_address set — admin should still pass.

	r := newRBACTestMiddleware()
	assert.True(t, r.hasPermissions(c, string(PermFinancialWithdraw)),
		"platform admin should have all permissions regardless of business ownership")
}

// TestRBAC_Staff_RolePermissions verifies that staff members are evaluated via
// role-based permission checks (not the web3/user short-circuits).
func TestRBAC_Staff_RolePermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set("token_type", "staff")
	c.Set("staff_role", string(database.StaffRoleKitchen))
	// No staff_id so custom permissions won't be fetched.

	r := newRBACTestMiddleware()
	assert.True(t, r.hasPermissions(c, string(PermOrdersKitchen)),
		"kitchen staff should have orders:kitchen permission")
	assert.False(t, r.hasPermissions(c, string(PermFinancialWithdraw)),
		"kitchen staff must not have financial:withdraw permission")
}

func TestRBAC_RequirePermissions_DenialUsesCanonicalErrorCode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := newRBACTestMiddleware()
	router := gin.New()
	router.GET("/protected", r.RequirePermissions(string(PermFinancialRead)), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response ErrorResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	// H1: an RBAC permission/role deny now carries AUTH_INSUFFICIENT_ROLE (was
	// the generic AUTH_FORBIDDEN) so the FE localizes the role-specific message.
	assert.Equal(t, ErrCodeInsufficientRole, response.Code)
	assert.Equal(t, "Insufficient permissions for this action", response.Error)
	assert.Equal(t, []interface{}{string(PermFinancialRead)}, response.Params["required_permissions"])
}
