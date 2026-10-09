package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// TestInventoryAlertInsertIsClaim asserts the log insert is the claim: a second
// insert for the same (business,item,alert_type,day) does NOT create a row, so
// the caller knows not to re-send.
//
// The dedup relies on a Postgres GENERATED ALWAYS AS column (alert_day) backed
// by a UNIQUE index, so this test requires a live Postgres container — it
// cannot run on SQLite. The test uses the same container-test convention as the
// rest of the inventory scheduler tests.
func TestInventoryAlertInsertIsClaim(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}

	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	// AutoMigrate creates the table but NOT the generated column or unique index;
	// those ship in the genesis schema and must be applied manually here so the
	// insert-claim dedup is active during the test.
	require.NoError(t, pg.DB.AutoMigrate(
		&database.Business{},
		&database.InventoryItem{},
		&database.InventoryAlertLog{},
	))
	require.NoError(t, pg.DB.Exec(`
		ALTER TABLE inventory_alert_logs
		    ADD COLUMN IF NOT EXISTS alert_day DATE
		    GENERATED ALWAYS AS ((alerted_at AT TIME ZONE 'UTC')::date) STORED
	`).Error)
	require.NoError(t, pg.DB.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_inventory_alert_logs_daily
		    ON inventory_alert_logs (business_id, inventory_item_id, alert_type, alert_day)
	`).Error)

	// Seed the parent business so the FK on inventory_alert_logs is satisfied.
	biz := database.Business{Name: "Claim Test Biz"}
	require.NoError(t, pg.DB.Create(&biz).Error)

	s := &InventoryAlertScheduler{db: pg.DB}
	now := time.Now().UTC()

	first := s.claimAlertLog(biz.ID, 10, "low_stock", now)
	second := s.claimAlertLog(biz.ID, 10, "low_stock", now)

	if !first || second {
		t.Fatalf("expected first=true second=false, got first=%v second=%v", first, second)
	}
}

// TestInventoryAlertClaimAllowsDifferentDay verifies that a claim for a new day
// succeeds even if an alert was already sent for the same (business, item, type)
// on a previous day — i.e., the dedup does not suppress forever.
func TestInventoryAlertClaimAllowsDifferentDay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}

	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	require.NoError(t, pg.DB.AutoMigrate(
		&database.Business{},
		&database.InventoryItem{},
		&database.InventoryAlertLog{},
	))
	require.NoError(t, pg.DB.Exec(`
		ALTER TABLE inventory_alert_logs
		    ADD COLUMN IF NOT EXISTS alert_day DATE
		    GENERATED ALWAYS AS ((alerted_at AT TIME ZONE 'UTC')::date) STORED
	`).Error)
	require.NoError(t, pg.DB.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_inventory_alert_logs_daily
		    ON inventory_alert_logs (business_id, inventory_item_id, alert_type, alert_day)
	`).Error)

	biz := database.Business{Name: "Claim Test Biz Day"}
	require.NoError(t, pg.DB.Create(&biz).Error)

	s := &InventoryAlertScheduler{db: pg.DB}
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	today := time.Now().UTC()

	// Alert sent yesterday — should succeed.
	claimedYesterday := s.claimAlertLog(biz.ID, 20, "low_stock", yesterday)
	require.True(t, claimedYesterday, "first claim (yesterday) must succeed")

	// Same item, same type, but a new calendar day — must succeed again.
	claimedToday := s.claimAlertLog(biz.ID, 20, "low_stock", today)
	require.True(t, claimedToday, "claim for a new day must succeed even after yesterday's alert")
}
