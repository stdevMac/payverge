package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestStaffDisplayPermissions_UsesCanonicalRolePerms locks AUTH-3: the staff
// "permissions" display must be sourced from the canonical enforcement set
// (server.StaffRolePermissions) plus custom perms — not a separate, drift-prone
// services map. The legacy services map omitted fiscal:*, printers:*, bills:items,
// etc., so the display could mislead an operator about a staffer's real access.
func TestStaffDisplayPermissions_UsesCanonicalRolePerms(t *testing.T) {
	perms := staffDisplayPermissions(database.StaffRoleManager, `["custom:thing"]`)
	require.Contains(t, perms, "fiscal:issue", "manager display must include canonical fiscal:issue")
	require.Contains(t, perms, "bills:items", "manager display must include canonical bills:items")
	require.Contains(t, perms, "custom:thing", "display must include custom permissions")

	kitchen := staffDisplayPermissions(database.StaffRoleKitchen, "")
	require.Contains(t, kitchen, "orders:kitchen")
	require.NotContains(t, kitchen, "fiscal:issue", "kitchen must not show manager-only perms")

	// Malformed custom JSON is ignored (role perms still returned).
	require.NotEmpty(t, staffDisplayPermissions(database.StaffRoleServer, "{not-json"))
}
