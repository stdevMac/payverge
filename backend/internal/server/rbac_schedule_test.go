package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func hasPerm(perms []Permission, want Permission) bool {
	for _, p := range perms {
		if p == want {
			return true
		}
	}
	return false
}

func TestSchedulePermissions_RoleDefaults(t *testing.T) {
	mgr := StaffRolePermissions[database.StaffRoleManager]
	require.True(t, hasPerm(mgr, PermScheduleRead))
	require.True(t, hasPerm(mgr, PermScheduleWrite))
	require.True(t, hasPerm(mgr, PermSchedulePublish))
	require.True(t, hasPerm(mgr, PermScheduleApprove))
	require.True(t, hasPerm(mgr, PermScheduleSelf))

	for _, role := range []database.StaffRole{database.StaffRoleServer, database.StaffRoleHost, database.StaffRoleKitchen} {
		perms := StaffRolePermissions[role]
		require.True(t, hasPerm(perms, PermScheduleRead), "role %s should read schedule", role)
		require.True(t, hasPerm(perms, PermScheduleSelf), "role %s should self-serve", role)
		// Non-managers must NOT build/publish/approve schedules.
		require.False(t, hasPerm(perms, PermScheduleWrite), "role %s must not write schedule", role)
		require.False(t, hasPerm(perms, PermScheduleApprove), "role %s must not approve", role)
	}
}
