package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
)

func roleHasPerm(role database.StaffRole, perm Permission) bool {
	for _, p := range StaffRolePermissions[role] {
		if p == perm {
			return true
		}
	}
	return false
}

func TestAiWaiterPermissionMatrix(t *testing.T) {
	cases := []struct {
		role                         database.StaffRole
		read, reply, insights, write bool
	}{
		{database.StaffRoleManager, true, true, true, true},
		{database.StaffRoleServer, true, true, false, false},
		{database.StaffRoleHost, true, true, false, false},
		{database.StaffRoleKitchen, false, false, false, false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.read, roleHasPerm(tc.role, PermAIWaiterRead), "%s read", tc.role)
		assert.Equal(t, tc.reply, roleHasPerm(tc.role, PermAIWaiterReply), "%s reply", tc.role)
		assert.Equal(t, tc.insights, roleHasPerm(tc.role, PermAIWaiterInsights), "%s insights", tc.role)
		assert.Equal(t, tc.write, roleHasPerm(tc.role, PermAIWaiterWrite), "%s write", tc.role)
	}
}
