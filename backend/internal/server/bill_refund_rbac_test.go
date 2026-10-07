package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestBillRefundIsOwnerOnly locks F-REFUND-RBAC: void/refund move/cancel money,
// and the handler (authorizeBillManagement) already restricts them to the
// business owner. The route permission must match that reality — bills:refund
// is owner-only and held by NO staff role — so the route gate can't drift to
// silently grant a Server refund power. Operational bill ops (close) stay with
// staff.
func TestBillRefundIsOwnerOnly(t *testing.T) {
	require.Equal(t, Permission("bills:refund"), PermBillsRefund)

	for role, perms := range StaffRolePermissions {
		require.NotContainsf(t, perms, PermBillsRefund,
			"staff role %q must not hold bills:refund (owner-only void/refund)", role)
	}

	// Staff keep day-to-day bill operations.
	manager := StaffRolePermissions[database.StaffRoleManager]
	require.Contains(t, manager, PermBillsClose)
	require.Contains(t, manager, PermBillsWrite)
	server := StaffRolePermissions[database.StaffRoleServer]
	require.Contains(t, server, PermBillsClose)
}
