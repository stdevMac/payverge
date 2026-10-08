package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func TestBillsPaymentRBACManagerAndServerHavePermission(t *testing.T) {
	require.Contains(t, StaffRolePermissions[database.StaffRoleManager], PermBillsPayment)
	require.Contains(t, StaffRolePermissions[database.StaffRoleServer], PermBillsPayment)
}

func TestBillsPaymentRBACHostAndKitchenLackPermission(t *testing.T) {
	require.NotContains(t, StaffRolePermissions[database.StaffRoleHost], PermBillsPayment)
	require.NotContains(t, StaffRolePermissions[database.StaffRoleKitchen], PermBillsPayment)
}
