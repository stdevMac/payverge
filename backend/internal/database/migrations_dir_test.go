package database

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// migrationsDir returns the absolute path to backend/migrations, resolved from
// this test file's location so it is independent of the test working directory.
func migrationsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	// thisFile = backend/internal/database/<file>.go → ../../migrations
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
}

// startGenesisPostgres returns an isolated Postgres database brought to the
// production schema exactly as startup does on an empty database: the embedded
// genesis baseline, then any pending numbered migrations. Tests that exercise
// SQL-only invariants (partial unique indexes, CHECKs, triggers) run against
// it instead of hand-built tables.
func startGenesisPostgres(t *testing.T) *testperf.Pg {
	t.Helper()
	ctx := context.Background()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	if err != nil {
		t.Fatalf("isolated postgres: %v (is Docker running, or can TEST_DATABASE_URL create child databases?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	require.NoError(t, BootstrapGenesisSchema(pg.DB), "bootstrap genesis schema")
	host, port, user, pass, name, sslmode, err := parsePostgresDSN(pg.DSN)
	require.NoError(t, err)
	require.NoError(t, RunMigrations(host, port, user, pass, name, sslmode, migrationsDir(t)), "apply pending migrations")
	return pg
}

// seedGenesisBusiness inserts a minimal business so rows with a business FK
// can be created on a genesis database.
func seedGenesisBusiness(t *testing.T, gdb *gorm.DB, slug string) uint {
	t.Helper()
	business := Business{
		BusinessId:   slug,
		OwnerAddress: "0x1111111111111111111111111111111111111111",
		Name:         slug,
		IsActive:     true,
	}
	require.NoError(t, gdb.Create(&business).Error)
	return business.ID
}
