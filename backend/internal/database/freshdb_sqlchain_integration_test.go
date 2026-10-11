//go:build integration_postgres

package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// TestFreshDB_GenesisBootstrapStandsAlone is the positive standalone-bootstrap
// gate: on a truly empty Postgres, BootstrapGenesisSchema alone produces a
// versioned database at the embedded migration HEAD with required tables.
//
// The genesis baseline is the whole schema; numbered migrations only layer
// changes made after it. This proves the baseline stands alone on an empty DB.
func TestFreshDB_GenesisBootstrapStandsAlone(t *testing.T) {
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	// Precondition: empty public schema.
	class, err := ClassifyDatabase(pg.DB)
	require.NoError(t, err)
	require.Equal(t, DBEmpty, class, "container must start empty")

	require.NoError(t, BootstrapGenesisSchema(pg.DB), "BootstrapGenesisSchema on empty DB")

	class, err = ClassifyDatabase(pg.DB)
	require.NoError(t, err)
	require.Equal(t, DBVersioned, class, "after bootstrap: DBVersioned")

	head := GenesisMigrationHead()
	require.GreaterOrEqual(t, head, int64(0), "embedded genesis migration head")
	require.NoError(t, VerifySchemaAtVersion(pg.DB, head), "VerifySchemaAtVersion")

	// The ledger records the baseline head: no row when the head is 0, else
	// exactly one clean row at HEAD.
	var rowCount int64
	require.NoError(t, pg.DB.Raw(`SELECT count(*) FROM public.schema_migrations`).Scan(&rowCount).Error)
	if head == 0 {
		require.Zero(t, rowCount, "a head-0 baseline leaves the ledger empty")
	} else {
		require.Equal(t, int64(1), rowCount, "exactly one schema_migrations row")
		var version int64
		var dirty bool
		require.NoError(t, pg.DB.Raw(`SELECT version, dirty FROM public.schema_migrations LIMIT 1`).Row().Scan(&version, &dirty))
		require.Equal(t, head, version)
		require.False(t, dirty, "dirty must be false")
	}

	var runtimeControlRows []struct {
		Key     string
		Enabled bool
	}
	require.NoError(t, pg.DB.Raw(`SELECT key, enabled FROM runtime_controls`).Scan(&runtimeControlRows).Error)
	runtimeControls := make(map[string]bool, len(runtimeControlRows))
	for _, row := range runtimeControlRows {
		runtimeControls[row.Key] = row.Enabled
	}
	require.Equal(t, map[string]bool{
		"maintenance_mode":     false,
		"read_only_mode":       false,
		"payments_enabled":     true,
		"fiscal_enabled":       false,
		"ai_enabled":           true,
		"uploads_enabled":      true,
		"guest_orders_enabled": true,
	}, runtimeControls, "fresh databases must include the launch-control seed rows")
}
