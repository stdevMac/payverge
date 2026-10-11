package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

func TestCheckBusinessAccess_Web3Owner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xOwner123")

	biz := &database.Business{OwnerAddress: "0xOwner123"}
	assert.True(t, CheckBusinessAccess(c, biz))
}

func TestCheckBusinessAccess_OAuthOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", float64(42))

	userID := uint(42)
	biz := &database.Business{UserID: &userID}
	assert.True(t, CheckBusinessAccess(c, biz))
}

func TestCheckBusinessOwnership_OAuthOwnerUintContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", uint(42))

	userID := uint(42)
	biz := &database.Business{UserID: &userID}
	assert.True(t, CheckBusinessOwnership(c, biz))
}

func TestCheckBusinessOwnership_EmailOnlyDoesNotGrantOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("email", "owner@example.com")

	biz := &database.Business{Email: "owner@example.com"}
	assert.False(t, CheckBusinessOwnership(c, biz))
}

func TestCheckBusinessAccess_StaffMember(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("staff_business_id", float64(7))

	biz := &database.Business{}
	biz.ID = 7
	assert.True(t, CheckBusinessAccess(c, biz))
}

func TestCheckBusinessAccess_NotOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xOtherUser")

	biz := &database.Business{OwnerAddress: "0xOwner123"}
	assert.False(t, CheckBusinessAccess(c, biz))
}

func TestCheckBusinessAccess_PlatformAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", float64(8))
	c.Set("role", "admin")

	ownerID := uint(1)
	listedDemo := &database.Business{
		OwnerAddress: "0xOwner123",
		UserID:       &ownerID,
		IsDemo:       true,
		Kind:         database.BusinessKindDemo,
	}
	assert.True(t, CheckBusinessAccess(c, listedDemo))
	assert.False(t, CheckBusinessOwnership(c, listedDemo), "admin impersonate is access, not ownership")
	assert.True(t, actorCanOpenListedBusiness(c, listedDemo))

	live := &database.Business{OwnerAddress: "0xOwner123", UserID: &ownerID}
	assert.False(t, CheckBusinessAccess(c, live), "platform admin must not open an unowned live tenant")
	assert.False(t, actorCanOpenListedBusiness(c, live))
}

func TestCheckBusinessOwnership_DemoOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", uint(8))

	ownerID := uint(1)
	demoOwner := uint(8)
	biz := &database.Business{UserID: &ownerID, DemoOwnerUserID: &demoOwner}
	assert.True(t, CheckBusinessOwnership(c, biz))
	assert.True(t, CheckBusinessAccess(c, biz))
}

// makeTestContext returns a gin.Context backed by an httptest.ResponseRecorder
// so we can inspect the status code written by helpers under test.
func makeTestContext(method, path string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)
	c.Params = params
	return c, w
}

// setupOwnershipTestDB returns an in-memory SQLite DB with the Business model
// migrated. Mirrors the existing sqlite-based unit-test pattern in this package.
func setupOwnershipTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&database.Business{}))
	database.SetTestDB(gdb)
	return gdb
}

func TestRequireBusinessAccess_ReturnsBusinessForOwner(t *testing.T) {
	gdb := setupOwnershipTestDB(t)
	owner := "0x0000000000000000000000000000000000000001"
	biz := &database.Business{BusinessId: "owner-biz", OwnerAddress: owner, Name: "Test", IsActive: true}
	require.NoError(t, gdb.Create(biz).Error)

	c, w := makeTestContext("GET", "/x", gin.Params{{Key: "id", Value: biz.BusinessId}})
	c.Set("address", owner)

	got, ok := requireBusinessAccess(c, "id")
	require.True(t, ok, "owner should pass requireBusinessAccess")
	require.NotNil(t, got)
	assert.Equal(t, biz.ID, got.ID)
	assert.Equal(t, http.StatusOK, w.Code, "should not write any error status on success")
}

func TestRequireBusinessAccess_ReturnsBusinessForStaff(t *testing.T) {
	gdb := setupOwnershipTestDB(t)
	biz := &database.Business{BusinessId: "staff-biz", OwnerAddress: "0xowner", Name: "Test", IsActive: true}
	require.NoError(t, gdb.Create(biz).Error)

	c, w := makeTestContext("GET", "/x", gin.Params{{Key: "id", Value: biz.BusinessId}})
	c.Set("token_type", "staff")
	c.Set("staff_business_id", biz.ID)

	got, ok := requireBusinessAccess(c, "id")
	require.True(t, ok, "staff with matching business_id should pass")
	require.NotNil(t, got)
	assert.Equal(t, biz.ID, got.ID)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireBusinessAccess_403ForUnaffiliatedUser(t *testing.T) {
	gdb := setupOwnershipTestDB(t)
	biz := &database.Business{BusinessId: "lonely-biz", OwnerAddress: "0xowner", Name: "Test", IsActive: true}
	require.NoError(t, gdb.Create(biz).Error)

	c, w := makeTestContext("GET", "/x", gin.Params{{Key: "id", Value: biz.BusinessId}})
	c.Set("address", "0xstranger")

	before := testutil.ToFloat64(metrics.TenantAuthorizationMismatches.WithLabelValues("business_access"))
	got, ok := requireBusinessAccess(c, "id")
	assert.False(t, ok)
	assert.Nil(t, got)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, before+1, testutil.ToFloat64(metrics.TenantAuthorizationMismatches.WithLabelValues("business_access")))
}

func TestRequireBusinessAccess_404ForUnknownBusiness(t *testing.T) {
	_ = setupOwnershipTestDB(t)

	c, w := makeTestContext("GET", "/x", gin.Params{{Key: "id", Value: "does-not-exist"}})
	c.Set("address", "0xanyone")

	got, ok := requireBusinessAccess(c, "id")
	assert.False(t, ok)
	assert.Nil(t, got)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRequireBusinessOwnership_403ForStaffEvenWhenStaffOfThisBusiness(t *testing.T) {
	gdb := setupOwnershipTestDB(t)
	biz := &database.Business{BusinessId: "owner-only-biz", OwnerAddress: "0xowner", Name: "Test", IsActive: true}
	require.NoError(t, gdb.Create(biz).Error)

	c, w := makeTestContext("GET", "/x", gin.Params{{Key: "id", Value: biz.BusinessId}})
	c.Set("token_type", "staff")
	c.Set("staff_business_id", biz.ID)

	before := testutil.ToFloat64(metrics.TenantAuthorizationMismatches.WithLabelValues("business_ownership"))
	got, ok := requireBusinessOwnership(c, "id")
	assert.False(t, ok, "staff must not pass requireBusinessOwnership")
	assert.Nil(t, got)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, before+1, testutil.ToFloat64(metrics.TenantAuthorizationMismatches.WithLabelValues("business_ownership")))
}

// TestGetBusiness_StaffGetsScopedProjection verifies that a staff member
// accessing GetBusiness does not receive owner secrets (wallet addresses,
// Stripe IDs, contact email). (RBAC-BIZRECORD-LEAK-01)
func TestGetBusiness_StaffGetsScopedProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupOwnershipTestDB(t)
	require.NoError(t, gdb.AutoMigrate(&database.BusinessDesignSettings{}, &database.BusinessAiSettings{}))

	onboardTime := time.Now().Add(-24 * time.Hour)
	owner := "0x0000000000000000000000000000000000000001"
	biz := &database.Business{
		BusinessId:            "sensitive-biz",
		OwnerAddress:          owner,
		Name:                  "Sensitive Biz",
		IsActive:              true,
		SettlementAddr:        "0xSETTLE_SECRET",
		TippingAddr:           "0xTIP_SECRET",
		Email:                 "owner@secret.test",
		Phone:                 "+1-555-SECRET",
		DefaultCurrency:       "USD",
		OnboardingCompletedAt: &onboardTime,
	}
	require.NoError(t, gdb.Create(biz).Error)

	c, w := makeTestContext("GET", "/businesses/"+biz.BusinessId,
		gin.Params{{Key: "id", Value: biz.BusinessId}})
	c.Set("token_type", "staff")
	c.Set("staff_business_id", biz.ID)

	GetBusiness(c)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// No owner secrets.
	assert.NotContains(t, body, "0xSETTLE_SECRET")
	assert.NotContains(t, body, "0xTIP_SECRET")
	assert.NotContains(t, body, "owner@secret.test")
	assert.NotContains(t, body, `"settlement_address"`)
	assert.NotContains(t, body, `"tipping_address"`)
	assert.NotContains(t, body, `"owner_address"`)
	assert.NotContains(t, body, `"owner_name"`)

	// Operational fields must still be present.
	assert.Contains(t, body, `"name":"Sensitive Biz"`)
	assert.Contains(t, body, `"default_currency"`)
	assert.Contains(t, body, `"onboarding_completed_at"`,
		"staff must see onboarding_completed_at for the OnboardingHub")
}

// TestGetBusiness_OwnerGetsFullRecord verifies the positive case:
// an owner still receives the full Business struct including
// settlement/tipping addresses.
func TestGetBusiness_OwnerGetsFullRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupOwnershipTestDB(t)
	require.NoError(t, gdb.AutoMigrate(&database.BusinessDesignSettings{}, &database.BusinessAiSettings{}))

	owner := "0x0000000000000000000000000000000000000001"
	biz := &database.Business{
		BusinessId:      "owner-biz-full",
		OwnerAddress:    owner,
		Name:            "Owner Biz",
		IsActive:        true,
		SettlementAddr:  "0xSETTLE_OWNER",
		TippingAddr:     "0xTIP_OWNER",
		Email:           "owner@owner.test",
		DefaultCurrency: "USD",
	}
	require.NoError(t, gdb.Create(biz).Error)

	c, w := makeTestContext("GET", "/businesses/"+biz.BusinessId,
		gin.Params{{Key: "id", Value: biz.BusinessId}})
	c.Set("address", owner)

	GetBusiness(c)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	assert.Contains(t, body, "0xSETTLE_OWNER", "owner must see settlement address")
	assert.Contains(t, body, "0xTIP_OWNER", "owner must see tipping address")
	assert.Contains(t, body, "owner@owner.test", "owner must see email")
}
