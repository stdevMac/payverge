package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRBACRevokeDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:rbac-revoke-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
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
		&database.StaffIdentity{},
		&database.StaffMembership{},
		&database.StaffPermissionDeny{},
		&database.RBACAuditLog{},
		&database.PushSubscription{},
		&session.UserSession{},
	))
	session.GlobalStore = session.NewStore(gormDB)
	return database.GetDBWrapper()
}

func seedStaffForRevoke(t *testing.T, db *database.DB, email string) *database.Staff {
	t.Helper()
	biz := &database.Business{
		BusinessId:     fmt.Sprintf("biz-revoke-%s", email),
		Name:           "Revoke Biz",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.GetGorm().Create(biz).Error)
	staff := &database.Staff{
		BusinessID:   biz.ID,
		Email:        email,
		Name:         "Revoke Target",
		Role:         database.StaffRoleServer,
		IsActive:     true,
		InvitedBy:    "0xowner",
		AuthzVersion: 1,
	}
	require.NoError(t, db.StaffService.Create(staff))
	return staff
}

func createStaffSession(t *testing.T, staffID uint, provider, token string) *session.UserSession {
	t.Helper()
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &staffID,
		TokenHash: session.HashToken(token),
		Provider:  provider,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	return sess
}

// AccessChange mutations must bump authz_version, revoke scoped staff sessions,
// and delete principal-typed staff push subscriptions.
func TestAccessChange_DeactivateStaff_RevokesSessionsAndPushAndBumpsAuthz(t *testing.T) {
	db := setupRBACRevokeDB(t)
	staff := seedStaffForRevoke(t, db, "deact@example.com")
	svc := NewRBACService(db)

	sess := createStaffSession(t, staff.ID, "staff_code", "deact-token")
	require.NoError(t, db.GetGorm().Create(&database.PushSubscription{
		UserID:        0,
		BusinessID:    staff.BusinessID,
		PrincipalType: database.PushPrincipalStaff,
		PrincipalID:   staff.ID,
		Endpoint:      "https://push.example/deact",
		P256dhKey:     "k",
		AuthKey:       "a",
	}).Error)

	require.NoError(t, svc.DeactivateStaff(staff.ID, "0xowner", "left"))

	var reloaded database.Staff
	require.NoError(t, db.GetGorm().First(&reloaded, staff.ID).Error)
	assert.False(t, reloaded.IsActive)
	assert.Equal(t, 2, reloaded.AuthzVersion, "deactivate must bump authz_version")
	var membership database.StaffMembership
	require.NoError(t, db.GetGorm().Where("legacy_staff_id = ?", staff.ID).Take(&membership).Error)
	assert.False(t, membership.IsActive)
	assert.Equal(t, 2, membership.AuthzVersion)

	var persisted session.UserSession
	require.NoError(t, db.GetGorm().First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked, "scoped staff session must be revoked")
	assert.Equal(t, string(session.RevocationReasonAdministrative), persisted.RevocationReason)

	var pushCount int64
	require.NoError(t, db.GetGorm().Model(&database.PushSubscription{}).
		Where("principal_type = ? AND principal_id = ?", database.PushPrincipalStaff, staff.ID).
		Count(&pushCount).Error)
	assert.Equal(t, int64(0), pushCount, "staff principal push subs must be deleted")
}

func TestAccessChange_ReactivateStaffSyncsMembershipForMultiBusinessLogin(t *testing.T) {
	db := setupRBACRevokeDB(t)
	first := seedStaffForRevoke(t, db, "reactivate-multi@example.com")
	secondBusiness := &database.Business{
		BusinessId:     "biz-reactivate-second",
		Name:           "Second Venue",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.GetGorm().Create(secondBusiness).Error)
	second := &database.Staff{
		BusinessID:   secondBusiness.ID,
		Email:        first.Email,
		Name:         "Revoke Target",
		Role:         database.StaffRoleServer,
		IsActive:     true,
		InvitedBy:    "0xowner",
		AuthzVersion: 1,
	}
	require.NoError(t, db.StaffService.Create(second))

	svc := NewRBACService(db)
	require.NoError(t, svc.DeactivateStaff(first.ID, "0xowner", "leave"))
	active, err := db.StaffService.GetActiveByEmail(first.Email)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, second.ID, active[0].ID)

	require.NoError(t, svc.ReactivateStaff(first.ID, "0xowner", "return"))
	var legacy database.Staff
	require.NoError(t, db.GetGorm().First(&legacy, first.ID).Error)
	assert.True(t, legacy.IsActive)
	assert.Equal(t, 3, legacy.AuthzVersion)
	var membership database.StaffMembership
	require.NoError(t, db.GetGorm().Where("legacy_staff_id = ?", first.ID).Take(&membership).Error)
	assert.True(t, membership.IsActive)
	assert.Equal(t, legacy.AuthzVersion, membership.AuthzVersion)

	active, err = db.StaffService.GetActiveByEmail(first.Email)
	require.NoError(t, err)
	require.Len(t, active, 2, "reactivation must restore the venue to membership selection")
}

func TestAccessChange_RoleChange_RevokesSessionsAndBumpsAuthz(t *testing.T) {
	db := setupRBACRevokeDB(t)
	staff := seedStaffForRevoke(t, db, "role@example.com")
	svc := NewRBACService(db)

	sess := createStaffSession(t, staff.ID, "google_staff", "role-token")
	require.NoError(t, svc.ChangeStaffRole(staff.ID, database.StaffRoleManager, "0xowner", "promo"))

	var reloaded database.Staff
	require.NoError(t, db.GetGorm().First(&reloaded, staff.ID).Error)
	assert.Equal(t, database.StaffRoleManager, reloaded.Role)
	assert.Equal(t, 2, reloaded.AuthzVersion)
	var membership database.StaffMembership
	require.NoError(t, db.GetGorm().Where("legacy_staff_id = ?", staff.ID).Take(&membership).Error)
	assert.Equal(t, database.StaffRoleManager, membership.Role)
	assert.Equal(t, 2, membership.AuthzVersion)

	var persisted session.UserSession
	require.NoError(t, db.GetGorm().First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
}

func TestAccessChange_GrantAndDenyPermission_RevokesSessionsAndBumpsAuthz(t *testing.T) {
	db := setupRBACRevokeDB(t)
	staff := seedStaffForRevoke(t, db, "perm@example.com")
	svc := NewRBACService(db)

	sessGrant := createStaffSession(t, staff.ID, "staff_invite", "grant-token")
	require.NoError(t, svc.GrantCustomPermission(staff.ID, "analytics:sales", "0xowner", "need sales"))

	var afterGrant database.Staff
	require.NoError(t, db.GetGorm().First(&afterGrant, staff.ID).Error)
	assert.Equal(t, 2, afterGrant.AuthzVersion)

	var gSess session.UserSession
	require.NoError(t, db.GetGorm().First(&gSess, sessGrant.ID).Error)
	assert.True(t, gSess.Revoked)

	// Fresh session after grant, then deny must revoke again and bump further.
	sessDeny := createStaffSession(t, staff.ID, "staff", "deny-token")
	require.NoError(t, svc.DenyPermission(staff.ID, "bills:read", "0xowner", "floor only"))

	var afterDeny database.Staff
	require.NoError(t, db.GetGorm().First(&afterDeny, staff.ID).Error)
	assert.Equal(t, 3, afterDeny.AuthzVersion)

	var dSess session.UserSession
	require.NoError(t, db.GetGorm().First(&dSess, sessDeny.ID).Error)
	assert.True(t, dSess.Revoked)
}

func TestAccessChange_RevokeCustomAndRemoveDeny_BumpAuthz(t *testing.T) {
	db := setupRBACRevokeDB(t)
	staff := seedStaffForRevoke(t, db, "revokecustom@example.com")
	staff.CustomPermissions = `["analytics:sales"]`
	require.NoError(t, db.GetGorm().Save(staff).Error)
	svc := NewRBACService(db)

	require.NoError(t, svc.RevokeCustomPermission(staff.ID, "analytics:sales", "0xowner", "no longer"))
	var afterRevoke database.Staff
	require.NoError(t, db.GetGorm().First(&afterRevoke, staff.ID).Error)
	assert.Equal(t, 2, afterRevoke.AuthzVersion)

	require.NoError(t, svc.DenyPermission(staff.ID, "orders:write", "0xowner", "temp"))
	require.NoError(t, svc.RemovePermissionDeny(staff.ID, "orders:write", "0xowner", "restored"))
	var afterRemoveDeny database.Staff
	require.NoError(t, db.GetGorm().First(&afterRemoveDeny, staff.ID).Error)
	// deny (+1) then remove deny (+1) on top of prior revoke => 4
	assert.Equal(t, 4, afterRemoveDeny.AuthzVersion)
}

// Unrelated sessions that happen to share the same numeric id (different provider
// class / principal type) must stay ACTIVE after a staff access revoke.
func TestRevokeAccess_IsolationUnrelatedPrincipalSameNumericID(t *testing.T) {
	db := setupRBACRevokeDB(t)
	staff := seedStaffForRevoke(t, db, "iso@example.com")
	svc := NewRBACService(db)

	// Staff session for this staff id.
	staffSess := createStaffSession(t, staff.ID, "staff_code", "staff-iso-token")

	// Unrelated user session: same numeric user_id, different provider class.
	userSess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &staff.ID,
		TokenHash: session.HashToken("user-iso-token"),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	// Unrelated customer-scoped session (provider customer).
	customerSess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &staff.ID,
		TokenHash: session.HashToken("customer-iso-token"),
		Provider:  "customer",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	require.NoError(t, svc.RevokeStaffAccess(staff.ID, "test isolation", "0xowner"))

	var sSess, uSess, cSess session.UserSession
	require.NoError(t, db.GetGorm().First(&sSess, staffSess.ID).Error)
	require.NoError(t, db.GetGorm().First(&uSess, userSess.ID).Error)
	require.NoError(t, db.GetGorm().First(&cSess, customerSess.ID).Error)

	assert.True(t, sSess.Revoked, "staff-provider session must be revoked")
	assert.Equal(t, string(session.RevocationReasonAdministrative), sSess.RevocationReason)
	assert.False(t, uSess.Revoked, "email user session with same numeric id must stay active")
	assert.False(t, cSess.Revoked, "customer session with same numeric id must stay active")
}

func TestRevokeStaffAccess_IdempotentBump(t *testing.T) {
	db := setupRBACRevokeDB(t)
	staff := seedStaffForRevoke(t, db, "idem@example.com")
	svc := NewRBACService(db)

	require.NoError(t, svc.RevokeStaffAccess(staff.ID, "first", "0xowner"))
	require.NoError(t, svc.RevokeStaffAccess(staff.ID, "second", "0xowner"))

	var reloaded database.Staff
	require.NoError(t, db.GetGorm().First(&reloaded, staff.ID).Error)
	assert.Equal(t, 3, reloaded.AuthzVersion)
}
