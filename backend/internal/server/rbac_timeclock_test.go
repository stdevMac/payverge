package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// TestTimeclockPermissions_RoleDefaults locks the Slice-4 perm defaults: every
// staff role may punch the clock (timeclock:punch), but only the manager holds
// timeclock:manage (review/approve/manual entry). Owners bypass permission
// checks entirely, so they are not represented in StaffRolePermissions.
func TestTimeclockPermissions_RoleDefaults(t *testing.T) {
	mgr := StaffRolePermissions[database.StaffRoleManager]
	require.True(t, hasPerm(mgr, PermTimeclockPunch), "manager should punch")
	require.True(t, hasPerm(mgr, PermTimeclockManage), "manager should manage timesheets")

	for _, role := range []database.StaffRole{database.StaffRoleServer, database.StaffRoleHost, database.StaffRoleKitchen} {
		perms := StaffRolePermissions[role]
		require.Truef(t, hasPerm(perms, PermTimeclockPunch), "role %s should punch the clock", role)
		// Non-managers must NOT review/approve timesheets.
		require.Falsef(t, hasPerm(perms, PermTimeclockManage), "role %s must not manage timesheets", role)
	}
}
