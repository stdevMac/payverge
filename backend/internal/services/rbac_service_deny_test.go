package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRBACServiceDenyDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:rbac-svc-deny-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.StaffMembership{},
		&database.StaffPermissionDeny{},
		&database.RBACAuditLog{},
	))
	return database.GetDBWrapper()
}

func seedStaffForDeny(t *testing.T, db *database.DB) *database.Staff {
	t.Helper()
	biz := &database.Business{
		BusinessId:      "biz-deny-svc",
		Name:            "Deny Biz",
		OwnerAddress:    "0xowner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, db.GetGorm().Create(biz).Error)
	staff := &database.Staff{
		BusinessID:        biz.ID,
		Email:             "deny@example.com",
		Name:              "Deny Target",
		Role:              database.StaffRoleServer,
		IsActive:          true,
		InvitedBy:         "0xowner",
		CustomPermissions: `["analytics:sales"]`,
	}
	require.NoError(t, db.GetGorm().Create(staff).Error)
	return staff
}

func TestDenyPermission_UpsertAndAudit(t *testing.T) {
	db := setupRBACServiceDenyDB(t)
	staff := seedStaffForDeny(t, db)
	svc := NewRBACService(db)

	require.NoError(t, svc.DenyPermission(staff.ID, "bills:read", "0xowner", "floor only"))
	// Idempotent upsert — second call must not error.
	require.NoError(t, svc.DenyPermission(staff.ID, "bills:read", "0xowner", "floor only again"))

	// Grant is untouched.
	var reloaded database.Staff
	require.NoError(t, db.GetGorm().First(&reloaded, staff.ID).Error)
	assert.Equal(t, `["analytics:sales"]`, reloaded.CustomPermissions)

	denies, err := svc.ListPermissionDenies(staff.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"bills:read"}, denies)

	var n int64
	require.NoError(t, db.GetGorm().Model(&database.StaffPermissionDeny{}).
		Where("staff_id = ? AND permission = ?", staff.ID, "bills:read").Count(&n).Error)
	assert.Equal(t, int64(1), n, "unique upsert must keep a single deny row")

	var audits int64
	require.NoError(t, db.GetGorm().Model(&database.RBACAuditLog{}).
		Where("staff_id = ? AND action = ?", staff.ID, database.RBACActionPermissionDenied).Count(&audits).Error)
	assert.GreaterOrEqual(t, audits, int64(2))
}

func TestRemovePermissionDeny_DeletesAndAudits(t *testing.T) {
	db := setupRBACServiceDenyDB(t)
	staff := seedStaffForDeny(t, db)
	svc := NewRBACService(db)

	require.NoError(t, svc.DenyPermission(staff.ID, "orders:write", "0xowner", "temp"))
	require.NoError(t, svc.RemovePermissionDeny(staff.ID, "orders:write", "0xowner", "restored"))

	denies, err := svc.ListPermissionDenies(staff.ID)
	require.NoError(t, err)
	assert.Empty(t, denies)

	err = svc.RemovePermissionDeny(staff.ID, "orders:write", "0xowner", "again")
	require.Error(t, err)

	var audits int64
	require.NoError(t, db.GetGorm().Model(&database.RBACAuditLog{}).
		Where("staff_id = ? AND action = ?", staff.ID, database.RBACActionPermissionDenyRemoved).Count(&audits).Error)
	assert.Equal(t, int64(1), audits)
}
