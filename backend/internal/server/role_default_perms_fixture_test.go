package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func roleDefaultFixturePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// backend/internal/server → internal → backend → repo root (three levels up).
	// ../../.. is correct: server → internal → backend parent = monorepo root.
	// (The plan's ../../../.. overshoots to the parent of the repo.)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	return filepath.Join(root, "frontend/src/constants/__fixtures__/role-default-permissions.json")
}

func TestRoleDefaultPermissionsFixture_MatchesStaffRolePermissions(t *testing.T) {
	raw, err := os.ReadFile(roleDefaultFixturePath(t))
	require.NoError(t, err)

	var fixture map[string][]string
	require.NoError(t, json.Unmarshal(raw, &fixture))

	roles := []database.StaffRole{
		database.StaffRoleManager,
		database.StaffRoleServer,
		database.StaffRoleHost,
		database.StaffRoleKitchen,
	}
	for _, role := range roles {
		want := RolePermissions(role)
		got := fixture[string(role)]
		require.NotNil(t, got, "missing role %s in fixture", role)

		wantS := make([]string, len(want))
		for i, p := range want {
			wantS[i] = string(p)
		}
		sort.Strings(wantS)
		gotCopy := append([]string(nil), got...)
		sort.Strings(gotCopy)
		require.Equal(t, wantS, gotCopy, "fixture drift for role %s", role)
	}
}
