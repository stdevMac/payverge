package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func TestFiscalPermissions_ManagerDefaults(t *testing.T) {
	perms := StaffRolePermissions[database.StaffRoleManager]
	require.Contains(t, perms, PermFiscalRead)
	require.Contains(t, perms, PermFiscalWrite)
	require.Contains(t, perms, PermFiscalIssue)
	require.Contains(t, perms, PermFiscalRetry)
	require.Contains(t, perms, PermFiscalExport)
}

// TestFiscalCreditAndCredentialsAreOwnerOnly locks F-RBAC: emitting a nota de
// crédito (tax reversal) and uploading the AFIP private-key bundle are owner-only,
// granted to NO staff role — mirroring the payroll:write precedent. Managers keep
// day-to-day fiscal ops (issue / read / retry / export / settings-write+resend).
func TestFiscalCreditAndCredentialsAreOwnerOnly(t *testing.T) {
	require.Equal(t, Permission("fiscal:credit"), PermFiscalCredit)

	for role, perms := range StaffRolePermissions {
		require.NotContainsf(t, perms, PermFiscalCredit,
			"staff role %q must not hold fiscal:credit (owner-only credit notes)", role)
		require.NotContainsf(t, perms, PermFiscalCredentials,
			"staff role %q must not hold fiscal:credentials (owner-only key upload)", role)
	}

	// Managers retain operational fiscal permissions.
	manager := StaffRolePermissions[database.StaffRoleManager]
	require.Contains(t, manager, PermFiscalRead)
	require.Contains(t, manager, PermFiscalIssue)
	require.Contains(t, manager, PermFiscalRetry)
	require.Contains(t, manager, PermFiscalExport)
	require.Contains(t, manager, PermFiscalWrite) // settings update / validate / resend
}

func TestFiscalPermissions_ServiceRolesDefaultDeny(t *testing.T) {
	for _, role := range []database.StaffRole{
		database.StaffRoleServer,
		database.StaffRoleHost,
		database.StaffRoleKitchen,
	} {
		t.Run(string(role), func(t *testing.T) {
			perms := StaffRolePermissions[role]
			require.NotContains(t, perms, PermFiscalRead)
			require.NotContains(t, perms, PermFiscalWrite)
			require.NotContains(t, perms, PermFiscalIssue)
			require.NotContains(t, perms, PermFiscalRetry)
			require.NotContains(t, perms, PermFiscalExport)
		})
	}
}
