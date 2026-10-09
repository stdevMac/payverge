package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestFilePermissions_DefaultsAndExplicitGrant(t *testing.T) {
	manager := StaffRolePermissions[database.StaffRoleManager]
	require.Contains(t, manager, PermFilesUpload)
	require.Contains(t, manager, PermFilesDelete)

	for _, role := range []database.StaffRole{
		database.StaffRoleServer,
		database.StaffRoleHost,
		database.StaffRoleKitchen,
	} {
		assert.NotContains(t, StaffRolePermissions[role], PermFilesUpload)
		assert.NotContains(t, StaffRolePermissions[role], PermFilesDelete)
	}

	_ = setupEffectiveDenyTestDB(t)
	rbac := newEffectiveDenyRBAC()
	explicitlyGranted := staffCtx(
		t,
		database.StaffRoleKitchen,
		91,
		`["files:upload","files:delete"]`,
		nil,
	)
	assert.True(t, rbac.checkStaffPermissions(explicitlyGranted, []string{"files:upload"}))
	assert.True(t, rbac.checkStaffPermissions(explicitlyGranted, []string{"files:delete"}))
}
