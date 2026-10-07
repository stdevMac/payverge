package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

func setupAuthzRevocationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:authz-rev-%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	structs.SecretKey = []byte("test-secret-key-for-testing-purposes")
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.StaffPermissionDeny{},
		&session.UserSession{},
	))
	session.GlobalStore = session.NewStore(gormDB)
	return gormDB
}

func TestGenerateStaffToken_EmbedsAuthzVersion(t *testing.T) {
	setupAuthzRevocationTestDB(t)
	staff := &database.Staff{
		ID:           42,
		BusinessID:   7,
		Email:        "tok@example.com",
		Name:         "Tok",
		Role:         database.StaffRoleServer,
		IsActive:     true,
		AuthzVersion: 5,
	}
	token, err := GenerateStaffToken(staff)
	require.NoError(t, err)

	claims, err := VerifyStaffToken(token)
	require.NoError(t, err)
	assert.Equal(t, float64(5), claims["authz_version"], "JWT must embed staff.AuthzVersion at issuance")
}

func TestHydrateLiveStaffContext_StaleAuthzVersion_SessionRevoked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAuthzRevocationTestDB(t)

	biz := &database.Business{
		BusinessId:     "biz-stale-authz",
		Name:           "Stale Authz",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(biz).Error)
	staff := &database.Staff{
		BusinessID:   biz.ID,
		Email:        "stale@example.com",
		Name:         "Stale",
		Role:         database.StaffRoleServer,
		IsActive:     true,
		InvitedBy:    "0xowner",
		AuthzVersion: 1,
	}
	require.NoError(t, db.Create(staff).Error)

	// Mint at version 1, then bump DB to version 2 (simulating access change).
	token, err := GenerateStaffToken(staff)
	require.NoError(t, err)
	require.NoError(t, db.Model(staff).UpdateColumn("authz_version", 2).Error)

	claims, err := VerifyStaffToken(token)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	_, ok := hydrateLiveStaffContext(c, claims)
	assert.False(t, ok)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, ErrCodeSessionUnknown, body["code"], "stale authz_version must return session-revoked machine code")
}

func TestHydrateLiveStaffContext_MatchingAuthzVersion_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAuthzRevocationTestDB(t)

	biz := &database.Business{
		BusinessId:     "biz-ok-authz",
		Name:           "OK Authz",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(biz).Error)
	staff := &database.Staff{
		BusinessID:   biz.ID,
		Email:        "ok@example.com",
		Name:         "OK",
		Role:         database.StaffRoleServer,
		IsActive:     true,
		InvitedBy:    "0xowner",
		AuthzVersion: 3,
	}
	require.NoError(t, db.Create(staff).Error)

	token, err := GenerateStaffToken(staff)
	require.NoError(t, err)
	claims, err := VerifyStaffToken(token)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	got, ok := hydrateLiveStaffContext(c, claims)
	require.True(t, ok)
	require.NotNil(t, got)
	assert.Equal(t, staff.ID, got.ID)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHydrateLiveStaffContext_DenyStoreFailureFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAuthzRevocationTestDB(t)

	biz := &database.Business{
		BusinessId:     "biz-deny-store-down",
		Name:           "Deny Store Down",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(biz).Error)
	staff := &database.Staff{
		BusinessID:   biz.ID,
		Email:        "deny-store-down@example.com",
		Name:         "Deny Store Down",
		Role:         database.StaffRoleManager,
		IsActive:     true,
		InvitedBy:    "0xowner",
		AuthzVersion: 1,
	}
	require.NoError(t, db.Create(staff).Error)

	token, err := GenerateStaffToken(staff)
	require.NoError(t, err)
	claims, err := VerifyStaffToken(token)
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(&database.StaffPermissionDeny{}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	_, ok := hydrateLiveStaffContext(c, claims)
	assert.False(t, ok)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.True(t, c.IsAborted(), "deny-store failure must abort before RBAC evaluation")
}

func TestHydrateLiveStaffContext_InactiveStaff_SessionRevoked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAuthzRevocationTestDB(t)

	biz := &database.Business{
		BusinessId:     "biz-inactive-authz",
		Name:           "Inactive",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(biz).Error)
	staff := &database.Staff{
		BusinessID:   biz.ID,
		Email:        "inactive@example.com",
		Name:         "Inactive",
		Role:         database.StaffRoleServer,
		IsActive:     true,
		InvitedBy:    "0xowner",
		AuthzVersion: 1,
	}
	require.NoError(t, db.Create(staff).Error)
	// GORM omits bool zero values on Create when default:true — force inactive.
	require.NoError(t, db.Model(staff).Update("is_active", false).Error)

	// Force a token claim even though staff is inactive.
	claims := jwt.MapClaims{
		"staff_id":      float64(staff.ID),
		"type":          "staff",
		"authz_version": float64(1),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	_, ok := hydrateLiveStaffContext(c, claims)
	assert.False(t, ok)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, ErrCodeSessionUnknown, body["code"])
}

func TestRemoveStaff_BumpsAuthzVersionViaChokepoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerAuthz", "Authz Remove Biz")
	staff := createStaffMember(t, business.ID, "remove-authz@example.com", "Remove Authz")
	// Ensure authz_version column is present with default 1.
	require.NoError(t, db.Model(staff).UpdateColumn("authz_version", 1).Error)

	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &staff.ID,
		TokenHash: session.HashToken("remove-authz-token"),
		Provider:  "staff_code",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "staffId", Value: fmt.Sprintf("%d", staff.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set("address", business.OwnerAddress)

	RemoveStaff(c)
	assert.Equal(t, http.StatusOK, w.Code)

	var reloaded database.Staff
	require.NoError(t, db.First(&reloaded, staff.ID).Error)
	assert.False(t, reloaded.IsActive)
	assert.GreaterOrEqual(t, reloaded.AuthzVersion, 2, "RemoveStaff must go through revoke chokepoint and bump authz_version")

	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
}
