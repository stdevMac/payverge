package demo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// When the ensure pass fails (e.g. the forced reseed hits an FK blocker),
// the admin demo summary must degrade to the persisted instance — with
// status=failed and last_error populated — instead of an opaque 500 that
// hides why the showroom is stale.
func TestSummaryForAdminSurfacesEnsureFailure(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-degrade@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed-v1", BaselineDays: 2})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	// Force the next ensure into the reseed path, then sabotage it.
	require.NoError(t, db.Model(&database.DemoInstance{}).
		Where("admin_user_id = ?", admin.ID).
		Update("seed_version", "").Error)
	require.NoError(t, db.Exec("DROP TABLE menus").Error)

	summary, err := svc.SummaryForAdmin(context.Background(), admin.ID, true)
	require.NoError(t, err, "summary must degrade to the persisted instance, not 500")
	require.NotNil(t, summary.Instance)
	require.Equal(t, database.DemoInstanceStatusFailed, summary.Instance.Status)
	require.NotEmpty(t, summary.Instance.LastError)
}
