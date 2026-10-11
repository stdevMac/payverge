package operational_alerts

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestSyncStaleOccupiedTableAlertDeduplicatesAndResolves(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	openedAt := time.Now().UTC().Add(-13 * time.Hour)

	require.NoError(t, svc.CreateStaleOccupiedTableAlert(
		context.Background(), business.ID, 9, "Table 9", 44, openedAt,
	))
	require.NoError(t, svc.CreateStaleOccupiedTableAlert(
		context.Background(), business.ID, 9, "Table 9", 44, openedAt,
	))
	var active int64
	require.NoError(t, db.Model(&database.OperationalAlert{}).Where(
		"business_id = ? AND alert_type = ? AND resource_type = ? AND resource_id = ? AND status IN ?",
		business.ID,
		database.OperationalAlertType("table_stale_occupied"),
		database.OperationalAlertResourceTypeTable,
		9,
		[]database.OperationalAlertStatus{
			database.OperationalAlertStatusOpen,
			database.OperationalAlertStatusClaimed,
		},
	).Count(&active).Error)
	require.Equal(t, int64(1), active)

	require.NoError(t, svc.ResolveStaleOccupiedTableAlert(context.Background(), business.ID, 9))
	require.NoError(t, db.Model(&database.OperationalAlert{}).Where(
		"business_id = ? AND alert_type = ? AND resource_type = ? AND resource_id = ? AND status IN ?",
		business.ID,
		database.OperationalAlertType("table_stale_occupied"),
		database.OperationalAlertResourceTypeTable,
		9,
		[]database.OperationalAlertStatus{
			database.OperationalAlertStatusOpen,
			database.OperationalAlertStatusClaimed,
		},
	).Count(&active).Error)
	require.Zero(t, active)
}
