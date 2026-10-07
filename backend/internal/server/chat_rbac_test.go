package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
)

// TestChatPermissionMatrix locks the Slice-7 chat role defaults: every staff role
// can read+send; only manager (and owner, who bypasses) can announce+moderate.
func TestChatPermissionMatrix(t *testing.T) {
	cases := []struct {
		role                           database.StaffRole
		read, send, announce, moderate bool
	}{
		{database.StaffRoleManager, true, true, true, true},
		{database.StaffRoleServer, true, true, false, false},
		{database.StaffRoleHost, true, true, false, false},
		{database.StaffRoleKitchen, true, true, false, false},
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.read, roleHasPerm(tc.role, PermChatRead), "%s chat:read", tc.role)
		assert.Equalf(t, tc.send, roleHasPerm(tc.role, PermChatSend), "%s chat:send", tc.role)
		assert.Equalf(t, tc.announce, roleHasPerm(tc.role, PermChatAnnounce), "%s chat:announce", tc.role)
		assert.Equalf(t, tc.moderate, roleHasPerm(tc.role, PermChatModerate), "%s chat:moderate", tc.role)
	}
}
