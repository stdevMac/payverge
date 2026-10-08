package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestStaffPermissionBreakdown_RoleOnly(t *testing.T) {
	effective, roleOnly, custom, denied := staffPermissionBreakdown(database.StaffRoleKitchen, "", nil)

	require.Empty(t, custom, "empty custom JSON must yield empty custom_grants")
	require.Empty(t, denied)
	require.NotEmpty(t, roleOnly)
	require.Equal(t, roleOnly, effective, "without custom grants/denies, effective equals role permissions")

	// Canonical kitchen perm from enforcement map.
	require.Contains(t, roleOnly, "orders:kitchen")
	require.NotContains(t, roleOnly, "fiscal:issue")
}

func TestStaffPermissionBreakdown_RolePlusCustom(t *testing.T) {
	customJSON := `["custom:thing","orders:kitchen"]`
	effective, roleOnly, custom, denied := staffPermissionBreakdown(database.StaffRoleServer, customJSON, nil)

	require.Equal(t, []string{"custom:thing", "orders:kitchen"}, custom,
		"custom_grants lists raw custom JSON entries (including role-key duplicates)")
	require.Empty(t, denied)
	require.NotEmpty(t, roleOnly)
	require.Contains(t, roleOnly, "orders:read") // server has orders:read in canonical set

	// Effective = role first, then custom (no dedupe) when no denies.
	require.Equal(t, append(append([]string{}, roleOnly...), custom...), effective)
	require.Contains(t, effective, "custom:thing")
}

func TestStaffPermissionBreakdown_WithDeny(t *testing.T) {
	customJSON := `["analytics:sales"]`
	effective, roleOnly, custom, denied := staffPermissionBreakdown(
		database.StaffRoleManager, customJSON, []string{"bills:read", "analytics:sales"},
	)

	require.Equal(t, []string{"analytics:sales"}, custom)
	require.Equal(t, []string{"bills:read", "analytics:sales"}, denied)
	require.NotContains(t, effective, "bills:read", "role grant subtracted by deny")
	require.NotContains(t, effective, "analytics:sales", "custom grant subtracted by deny")
	require.Contains(t, roleOnly, "bills:read", "role_permissions still lists inherited")
	require.Contains(t, effective, "orders:read", "unrelated role perm remains effective")
}

func TestStaffPermissionBreakdown_DelegatedOwnerOnlyAppearsInEffective(t *testing.T) {
	// bills:refund is owner-only BY DEFAULT (absent from every staff role) but
	// delegable: once an owner grants it to a manager it is effective, so the
	// breakdown reports it under both custom_grants and effective.
	customJSON := `["bills:refund"]`
	effective, _, custom, _ := staffPermissionBreakdown(database.StaffRoleManager, customJSON, nil)
	require.Contains(t, custom, "bills:refund", "custom_grants reports the stored grant")
	require.Contains(t, effective, "bills:refund", "a delegated owner-only-by-default grant is effective")

	// An explicit deny still removes a delegated grant from effective.
	deniedEffective, _, _, _ := staffPermissionBreakdown(database.StaffRoleManager, customJSON, []string{"bills:refund"})
	require.NotContains(t, deniedEffective, "bills:refund", "an explicit deny overrides a delegated grant")
}

func TestStaffPermissionBreakdown_MalformedJSON(t *testing.T) {
	effective, roleOnly, custom, denied := staffPermissionBreakdown(database.StaffRoleManager, "{not-json", nil)

	require.Empty(t, custom, "malformed custom JSON is ignored → empty custom_grants")
	require.Empty(t, denied)
	require.NotEmpty(t, roleOnly)
	require.Equal(t, roleOnly, effective, "malformed custom leaves effective = role only")
	require.Contains(t, roleOnly, "fiscal:issue")
}

func TestStaffPermissionBreakdown_NilVsEmptyCustomSlice(t *testing.T) {
	// Explicit empty array JSON still yields non-nil empty slice for JSON encoding.
	_, _, custom, denied := staffPermissionBreakdown(database.StaffRoleHost, "[]", nil)
	require.NotNil(t, custom)
	require.Empty(t, custom)
	require.NotNil(t, denied)
	require.Empty(t, denied)

	_, _, customMissing, deniedMissing := staffPermissionBreakdown(database.StaffRoleHost, "", nil)
	require.NotNil(t, customMissing)
	require.Empty(t, customMissing)
	require.NotNil(t, deniedMissing)
	require.Empty(t, deniedMissing)
}

func TestStaffPermissionBreakdown_MatchesRolePermissionsSource(t *testing.T) {
	for _, role := range []database.StaffRole{
		database.StaffRoleKitchen,
		database.StaffRoleHost,
		database.StaffRoleServer,
		database.StaffRoleManager,
	} {
		_, roleOnly, _, _ := staffPermissionBreakdown(role, "", nil)
		canonical := server.RolePermissions(role)
		require.Len(t, roleOnly, len(canonical), "role=%s", role)
		for i, p := range canonical {
			require.Equal(t, string(p), roleOnly[i], "role=%s idx=%d", role, i)
		}
	}
}

// setupRBACPermissionsDB is a minimal sqlite fixture for GetStaffPermissions.
func setupRBACPermissionsDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:rbac-perms-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}, &database.StaffPermissionDeny{}))
	return database.GetDBWrapper()
}

func TestGetStaffPermissions_ResponseShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupRBACPermissionsDB(t)

	business := &database.Business{
		BusinessId:      "biz-perms-shape",
		Name:            "Perms Biz",
		OwnerAddress:    "0xOwnerPerms",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, db.GetGorm().Create(business).Error)

	staff := &database.Staff{
		BusinessID:        business.ID,
		Email:             "server@example.com",
		Name:              "Server",
		Role:              database.StaffRoleServer,
		IsActive:          true,
		InvitedBy:         "owner@example.com",
		CustomPermissions: `["custom:thing"]`,
	}
	require.NoError(t, db.GetGorm().Create(staff).Error)
	require.NoError(t, db.GetGorm().Create(&database.StaffPermissionDeny{
		BusinessID: business.ID,
		StaffID:    staff.ID,
		Permission: "menu:read",
		CreatedBy:  "0xOwnerPerms",
	}).Error)

	h := NewRBACHandlers(db)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/businesses/"+business.BusinessId+"/staff/"+fmt.Sprint(staff.ID)+"/permissions", nil)
	c.Params = gin.Params{
		{Key: "id", Value: business.BusinessId},
		{Key: "staffId", Value: fmt.Sprint(staff.ID)},
	}
	// Owner access via wallet address matching business owner.
	c.Set("address", business.OwnerAddress)

	h.GetStaffPermissions(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Permissions     []string `json:"permissions"`
		RolePermissions []string `json:"role_permissions"`
		CustomGrants    []string `json:"custom_grants"`
		CustomDenies    []string `json:"custom_denies"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	require.NotNil(t, body.Permissions)
	require.NotNil(t, body.RolePermissions)
	require.NotNil(t, body.CustomGrants)
	require.NotNil(t, body.CustomDenies)

	assert.Equal(t, []string{"custom:thing"}, body.CustomGrants)
	assert.Equal(t, []string{"menu:read"}, body.CustomDenies)
	assert.Equal(t, body.RolePermissions, serverRolePermStrings(database.StaffRoleServer))
	assert.Contains(t, body.Permissions, "custom:thing")
	assert.NotContains(t, body.Permissions, "menu:read", "denied role perm excluded from effective")
	assert.Contains(t, body.RolePermissions, "menu:read", "role_permissions still lists inherited")
}

func TestGetStaffPermissions_MalformedCustomJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupRBACPermissionsDB(t)

	business := &database.Business{
		BusinessId:      "biz-perms-bad",
		Name:            "Perms Bad",
		OwnerAddress:    "0xOwnerBad",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, db.GetGorm().Create(business).Error)

	staff := &database.Staff{
		BusinessID:        business.ID,
		Email:             "mgr@example.com",
		Name:              "Manager",
		Role:              database.StaffRoleManager,
		IsActive:          true,
		InvitedBy:         "owner@example.com",
		CustomPermissions: "{broken",
	}
	require.NoError(t, db.GetGorm().Create(staff).Error)

	h := NewRBACHandlers(db)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{
		{Key: "id", Value: business.BusinessId},
		{Key: "staffId", Value: fmt.Sprint(staff.ID)},
	}
	c.Set("address", business.OwnerAddress)

	h.GetStaffPermissions(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	// custom_grants must be present as [] (not null) when JSON is malformed.
	cg, ok := body["custom_grants"].([]interface{})
	require.True(t, ok, "custom_grants must be a JSON array, got %T", body["custom_grants"])
	assert.Empty(t, cg)

	rp, ok := body["role_permissions"].([]interface{})
	require.True(t, ok)
	assert.NotEmpty(t, rp)

	perms, ok := body["permissions"].([]interface{})
	require.True(t, ok)
	assert.Equal(t, len(rp), len(perms), "malformed custom → permissions equals role_permissions")
}

func serverRolePermStrings(role database.StaffRole) []string {
	rolePerms := server.RolePermissions(role)
	out := make([]string, 0, len(rolePerms))
	for _, p := range rolePerms {
		out = append(out, string(p))
	}
	return out
}
