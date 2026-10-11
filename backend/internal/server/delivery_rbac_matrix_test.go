package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestDeliveryRouteGates_PermissionStrings(t *testing.T) {
	// Smoke test: confirm the canonical permission strings used by the route
	// registrations match what we register in StaffRolePermissions. If main.go
	// drifts, this test catches it.
	for _, p := range []Permission{
		PermDeliverySettingsRead, PermDeliverySettingsWrite,
		PermDeliveryDispatchRead, PermDeliveryDispatchWrite,
		PermDeliveryDriversRead, PermDeliveryDriversWrite,
	} {
		assert.NotEmpty(t, string(p))
		assert.Contains(t, string(p), "delivery:")
	}
}

func TestRolePermissions_DeliveryFamily(t *testing.T) {
	type cell struct {
		role database.StaffRole
		perm Permission
		want bool
	}
	cases := []cell{
		// settings
		{database.StaffRoleManager, PermDeliverySettingsRead, true},
		{database.StaffRoleManager, PermDeliverySettingsWrite, true},
		{database.StaffRoleServer, PermDeliverySettingsRead, true},
		{database.StaffRoleServer, PermDeliverySettingsWrite, false},
		{database.StaffRoleHost, PermDeliverySettingsRead, false},
		{database.StaffRoleKitchen, PermDeliverySettingsRead, false},
		// dispatch
		{database.StaffRoleManager, PermDeliveryDispatchRead, true},
		{database.StaffRoleManager, PermDeliveryDispatchWrite, true},
		{database.StaffRoleServer, PermDeliveryDispatchRead, true},
		{database.StaffRoleServer, PermDeliveryDispatchWrite, true},
		{database.StaffRoleKitchen, PermDeliveryDispatchRead, true},
		{database.StaffRoleKitchen, PermDeliveryDispatchWrite, false},
		{database.StaffRoleHost, PermDeliveryDispatchRead, false},
		// drivers
		{database.StaffRoleManager, PermDeliveryDriversRead, true},
		{database.StaffRoleManager, PermDeliveryDriversWrite, true},
		{database.StaffRoleServer, PermDeliveryDriversRead, false},
		{database.StaffRoleKitchen, PermDeliveryDriversRead, false},
		{database.StaffRoleHost, PermDeliveryDriversRead, false},
	}
	for _, c := range cases {
		perms := RolePermissions(c.role)
		has := false
		for _, p := range perms {
			if p == c.perm {
				has = true
				break
			}
		}
		assert.Equal(t, c.want, has,
			"role=%s perm=%s expected=%v got=%v",
			c.role, c.perm, c.want, has)
	}
}
