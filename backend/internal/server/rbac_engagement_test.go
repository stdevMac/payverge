package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
)

func TestEngagementPermDefaults(t *testing.T) {
	has := func(role database.StaffRole, p Permission) bool {
		for _, x := range StaffRolePermissions[role] {
			if x == p {
				return true
			}
		}
		return false
	}
	// All-staff perms: complete / doc:read / recognition:send / poll:vote.
	for _, role := range []database.StaffRole{
		database.StaffRoleManager, database.StaffRoleServer, database.StaffRoleHost, database.StaffRoleKitchen,
	} {
		require.True(t, has(role, PermChecklistComplete), "%s lacks checklist:complete", role)
		require.True(t, has(role, PermDocRead), "%s lacks doc:read", role)
		require.True(t, has(role, PermRecognitionSend), "%s lacks recognition:send", role)
		require.True(t, has(role, PermPollVote), "%s lacks poll:vote", role)
	}
	// Manager-only perms.
	require.True(t, has(database.StaffRoleManager, PermChecklistManage))
	require.True(t, has(database.StaffRoleManager, PermDocManage))
	require.True(t, has(database.StaffRoleManager, PermPollManage))
	// Line roles must NOT hold manage perms.
	for _, role := range []database.StaffRole{database.StaffRoleServer, database.StaffRoleHost, database.StaffRoleKitchen} {
		require.False(t, has(role, PermChecklistManage), "%s must not hold checklist:manage", role)
		require.False(t, has(role, PermDocManage), "%s must not hold doc:manage", role)
		require.False(t, has(role, PermPollManage), "%s must not hold poll:manage", role)
	}
}
