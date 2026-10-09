package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupRBACLimitDB spins up an in-memory sqlite DB with the models the
// RBAC audit log handler needs and returns the wrapper DB.
func setupRBACLimitDB(t testing.TB) *database.DB {
	return setupRBACLimitDBWithLogger(t, nil)
}

func setupRBACLimitDBWithLogger(t testing.TB, gormLogger logger.Interface) *database.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.StaffMembership{},
		&database.RBACAuditLog{},
	))
	return database.GetDBWrapper()
}

func seedRBACAuditLogs(t testing.TB, db *database.DB, businessID uint, staffID uint, n int) {
	t.Helper()
	base := time.Now().Add(-time.Duration(n) * time.Second)
	for i := 0; i < n; i++ {
		entry := database.RBACAuditLog{
			StaffID:    staffID,
			BusinessID: businessID,
			Action:     database.RBACActionRoleChanged,
			OldRole:    "server",
			NewRole:    "host",
			ChangedBy:  "owner@example.com",
			CreatedAt:  base.Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, db.GetGorm().Create(&entry).Error)
	}
}

func TestGetStaffAuditLog_ClampsLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupRBACLimitDB(t)

	business := &database.Business{
		BusinessId:      "biz-audit-2",
		Name:            "Audit Biz 2",
		OwnerAddress:    "0xOwnerAudit2",
		SettlementAddr:  "0x3333333333333333333333333333333333333333",
		TippingAddr:     "0x4444444444444444444444444444444444444444",
		DefaultCurrency: "USD",
	}
	require.NoError(t, db.GetGorm().Create(business).Error)

	staff := &database.Staff{
		BusinessID: business.ID,
		Email:      "s2@example.com",
		Name:       "S2",
		Role:       database.StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, db.GetGorm().Create(staff).Error)

	seedRBACAuditLogs(t, db, business.ID, staff.ID, 300)

	handlers := NewRBACHandlers(db)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerAudit2")
		c.Set("business_owner_address", "0xOwnerAudit2")
		c.Next()
	})
	router.GET("/inside/businesses/:id/staff/:staffId/audit", handlers.GetStaffAuditLog)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/inside/businesses/%s/staff/%d/audit?limit=10000000", business.BusinessId, staff.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		AuditLogs []database.RBACAuditLog `json:"audit_logs"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, 200, len(resp.AuditLogs), "staff audit log limit must clamp to 200")
}

func TestGetStaffAuditLogRejectsRouteBusinessMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupRBACLimitDB(t)

	businessA := &database.Business{
		BusinessId:     "biz-audit-route-a",
		Name:           "Audit Route A",
		OwnerAddress:   "0xOwnerAuditBoth",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.GetGorm().Create(businessA).Error)
	businessB := &database.Business{
		BusinessId:     "biz-audit-route-b",
		Name:           "Audit Route B",
		OwnerAddress:   "0xOwnerAuditBoth",
		SettlementAddr: "0x3333333333333333333333333333333333333333",
		TippingAddr:    "0x4444444444444444444444444444444444444444",
	}
	require.NoError(t, db.GetGorm().Create(businessB).Error)

	staffB := &database.Staff{
		BusinessID: businessB.ID,
		Email:      "staff-b@example.com",
		Name:       "Staff B",
		Role:       database.StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, db.GetGorm().Create(staffB).Error)
	seedRBACAuditLogs(t, db, businessB.ID, staffB.ID, 1)

	handlers := NewRBACHandlers(db)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerAuditBoth")
		c.Next()
	})
	router.GET("/inside/businesses/:id/staff/:staffId/audit", handlers.GetStaffAuditLog)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/inside/businesses/%s/staff/%d/audit", businessA.BusinessId, staffB.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
}

func TestRBACAuditLogUsesProjectedPreloads(t *testing.T) {
	recorder := &analyticsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupRBACLimitDBWithLogger(t, recorder)

	business := &database.Business{
		BusinessId:      fmt.Sprintf("biz-audit-projection-%d", time.Now().UnixNano()),
		Name:            "Audit Projection Biz",
		OwnerAddress:    "0xOwnerAuditProjection",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		Description:     strings.Repeat("large business payload ", 128),
		BannerImages:    strings.Repeat("banner-url,", 128),
	}
	require.NoError(t, db.GetGorm().Create(business).Error)

	staff := &database.Staff{
		BusinessID:           business.ID,
		Email:                "audit-projection@example.com",
		Name:                 "Audit Projection",
		Role:                 database.StaffRoleManager,
		IsActive:             true,
		InvitedBy:            "owner@example.com",
		CustomPermissions:    strings.Repeat(`"menu:write",`, 128),
		PinHash:              strings.Repeat("pin-hash", 128),
		PermissionsUpdatedBy: strings.Repeat("permission-updater", 32),
	}
	require.NoError(t, db.GetGorm().Create(staff).Error)
	seedRBACAuditLogs(t, db, business.ID, staff.ID, 20)

	service := services.NewRBACService(db)
	staffLogs, err := service.GetStaffAuditLog(staff.ID, 20, 0)
	require.NoError(t, err)
	require.Len(t, staffLogs, 20)
	require.Equal(t, "Audit Projection", staffLogs[0].Staff.Name)
	require.Equal(t, "Audit Projection Biz", staffLogs[0].Business.Name)

	assert.Zero(t, recorder.selectStarCount("staff"), "RBAC audit logs should project staff display fields instead of SELECT *")
	assert.Zero(t, recorder.selectStarCount("businesses"), "staff audit logs should project business display fields instead of SELECT *")
}

func BenchmarkGetRBACStaffAuditLogSQLite(b *testing.B) {
	db := setupRBACLimitDBWithLogger(b, logger.Default.LogMode(logger.Silent))

	business := &database.Business{
		BusinessId:      fmt.Sprintf("biz-audit-bench-%d", time.Now().UnixNano()),
		Name:            "Audit Bench Biz",
		OwnerAddress:    "0xOwnerAuditBench",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		Description:     strings.Repeat("large business payload ", 128),
		BannerImages:    strings.Repeat("banner-url,", 128),
	}
	require.NoError(b, db.GetGorm().Create(business).Error)

	staff := &database.Staff{
		BusinessID:           business.ID,
		Email:                "audit-bench@example.com",
		Name:                 "Audit Bench",
		Role:                 database.StaffRoleManager,
		IsActive:             true,
		InvitedBy:            "owner@example.com",
		CustomPermissions:    strings.Repeat(`"menu:write",`, 128),
		PinHash:              strings.Repeat("pin-hash", 128),
		PermissionsUpdatedBy: strings.Repeat("permission-updater", 32),
	}
	require.NoError(b, db.GetGorm().Create(staff).Error)
	seedRBACAuditLogs(b, db, business.ID, staff.ID, 1000)

	service := services.NewRBACService(db)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logs, err := service.GetStaffAuditLog(staff.ID, 200, 0)
		if err != nil {
			b.Fatal(err)
		}
		if len(logs) != 200 {
			b.Fatalf("expected 200 logs, got %d", len(logs))
		}
	}
}
