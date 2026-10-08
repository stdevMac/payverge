package operational_alerts

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// Issue #819: GET /inside/businesses/:id/alerts?status=open,claimed returned
// 500 on every dashboard paint. The seating-SLA filter in ListAlerts applied
// `metadata NOT LIKE ?` directly to the jsonb column — Postgres has no
// text-pattern operator for jsonb ("operator does not exist: jsonb ~~
// unknown"), so every list query failed in production while SQLite tests
// (text-affinity metadata) kept passing. This regression test runs the real
// dialect.
func TestListActiveAlertsOpenClaimedOnPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short")
	}
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	db := pg.DB
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.User{},
		&database.Staff{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
	))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("ops-alert-pg-%d", time.Now().UnixNano()),
		Name:           "Ops Alert PG",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)

	svc := NewService(db)

	// One fresh service call (jsonb metadata) and one non-service alert:
	// both must come back from the open,claimed dashboard list.
	_, err = svc.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeServiceCall,
		ResourceType: database.OperationalAlertResourceTypeTable,
		ResourceID:   7,
		Title:        "Water for table 7",
		Metadata:     map[string]any{"reason": "water"},
	})
	require.NoError(t, err)
	_, err = svc.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   44,
		Title:        "New order #44",
	})
	require.NoError(t, err)

	// The exact status set the dashboard bell polls with.
	items, err := svc.ListActiveAlerts(ctx, business.ID, []database.OperationalAlertStatus{
		database.OperationalAlertStatusOpen,
		database.OperationalAlertStatusClaimed,
	}, nil)
	require.NoError(t, err, "open,claimed alert list must not fail on the production dialect")
	require.Len(t, items, 2)

	// Stale unassigned water past the seating SLA is hidden by the same
	// filter — the jsonb-safe clause must still exclude it.
	stale := time.Now().Add(-2 * ServiceCallTTL)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("business_id = ? AND alert_type = ?", business.ID, database.OperationalAlertTypeServiceCall).
		Update("last_event_at", stale).Error)
	items, err = svc.ListActiveAlerts(ctx, business.ID, []database.OperationalAlertStatus{
		database.OperationalAlertStatusOpen,
		database.OperationalAlertStatusClaimed,
	}, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, database.OperationalAlertTypeOrderNew, items[0].AlertType)
}
