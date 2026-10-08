package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGrantCustomPermission_AuditWriteFailure_RollsBackAtomically proves the
// Priority-1 contract: an authorization mutation, its MANDATORY RBACAuditLog
// record, and the authz_version bump form ONE atomic unit. If the audit write
// fails, the permission grant and the version bump must roll back and the call
// must return an error — not silently succeed with a missing audit trail.
//
// Failure injection: drop the rbac_audit_logs table so the audit Create errors
// mid-operation.
func TestGrantCustomPermission_AuditWriteFailure_RollsBackAtomically(t *testing.T) {
	db := setupRBACRevokeDB(t)
	staff := seedStaffForRevoke(t, db, "grant-atomic@example.com")

	require.NoError(t, db.GetGorm().Migrator().DropTable(&database.RBACAuditLog{}),
		"precondition: drop audit table to inject a write failure")

	svc := NewRBACService(db)
	err := svc.GrantCustomPermission(staff.ID, "bills:refund", "0xowner", "test-grant")

	require.Error(t, err,
		"grant must fail when its mandatory audit record cannot be written")

	var reloaded database.Staff
	require.NoError(t, db.GetGorm().First(&reloaded, staff.ID).Error)
	assert.NotContains(t, reloaded.CustomPermissions, "bills:refund",
		"permission grant must roll back when the audit write fails")
	assert.Equal(t, 1, reloaded.AuthzVersion,
		"authz_version must not bump when the atomic grant rolls back")
}

// TestDenyPermission_AuditWriteFailure_RollsBackAtomically proves the same
// atomic contract for explicit denies: a failed mandatory audit write must roll
// back the deny row and leave authz_version untouched.
func TestDenyPermission_AuditWriteFailure_RollsBackAtomically(t *testing.T) {
	db := setupRBACRevokeDB(t)
	staff := seedStaffForRevoke(t, db, "deny-atomic@example.com")

	require.NoError(t, db.GetGorm().Migrator().DropTable(&database.RBACAuditLog{}),
		"precondition: drop audit table to inject a write failure")

	svc := NewRBACService(db)
	err := svc.DenyPermission(staff.ID, "bills:refund", "0xowner", "test-deny")

	require.Error(t, err,
		"deny must fail when its mandatory audit record cannot be written")

	var denyCount int64
	require.NoError(t, db.GetGorm().Model(&database.StaffPermissionDeny{}).
		Where("staff_id = ? AND permission = ?", staff.ID, "bills:refund").
		Count(&denyCount).Error)
	assert.Equal(t, int64(0), denyCount,
		"deny row must roll back when the audit write fails")

	var reloaded database.Staff
	require.NoError(t, db.GetGorm().First(&reloaded, staff.ID).Error)
	assert.Equal(t, 1, reloaded.AuthzVersion,
		"authz_version must not bump when the atomic deny rolls back")
}
