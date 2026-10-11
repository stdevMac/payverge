//go:build integration_postgres

package database

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// TestFreshDB_HasMoneyInvariants boots a real empty Postgres through the
// production startup path (genesis bootstrap, then RunMigrations) and asserts
// the SQL-only money invariants exist afterwards.
func TestFreshDB_HasMoneyInvariants(t *testing.T) {
	ctx := context.Background()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	class, err := ClassifyDatabase(pg.DB)
	require.NoError(t, err)
	require.Equal(t, DBEmpty, class)
	require.NoError(t, BootstrapGenesisSchema(pg.DB), "genesis bootstrap failed on fresh DB")
	require.NoError(t, runMigrationsForContainerDSN(t, pg.DSN, freshDBMigrationsDir(t)), "RunMigrations failed on fresh DB")
	latest, err := LatestMigrationVersion(freshDBMigrationsDir(t))
	require.NoError(t, err)
	require.NoError(t, VerifySchemaAtVersion(pg.DB, latest))

	requireIndexExists(t, pg.DB, "idx_bills_active_per_table")
	requireIndexExists(t, pg.DB, "idx_bill_split_shares_payment_settled")
	requireCheckConstraintExists(t, pg.DB, "bill_split_shares_amount_cents_positive")
}

// freshDBMigrationsDir returns the absolute path to backend/migrations, resolved
// from this test file's location (independent of the working directory).
func freshDBMigrationsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	// thisFile = backend/internal/database/<file>.go → ../../migrations
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
}

// runMigrationsForContainerDSN calls RunMigrations by parsing the postgres DSN
// that testcontainers produces (postgres://user:pass@host:port/db?...).
func runMigrationsForContainerDSN(t *testing.T, dsn, migsDir string) error {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("parse container DSN: %w", err)
	}
	host := u.Hostname()
	port := u.Port()
	user := u.User.Username()
	pass, _ := u.User.Password()
	dbName := u.Path[1:] // strip leading "/"
	sslMode := u.Query().Get("sslmode")
	if sslMode == "" {
		sslMode = "disable"
	}
	return RunMigrations(host, port, user, pass, dbName, sslMode, migsDir)
}

// requireIndexExists asserts that a named index exists in pg_indexes.
func requireIndexExists(t *testing.T, gdb *gorm.DB, indexName string) {
	t.Helper()
	var exists bool
	err := gdb.Raw(
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = ?)`,
		indexName,
	).Scan(&exists).Error
	require.NoError(t, err, "querying pg_indexes for %q", indexName)
	require.True(t, exists, "expected index %q to exist after fresh-DB startup", indexName)
}

// requireCheckConstraintExists asserts that a named CHECK constraint exists in pg_constraint.
func requireCheckConstraintExists(t *testing.T, gdb *gorm.DB, constraintName string) {
	t.Helper()
	var exists bool
	err := gdb.Raw(
		`SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = ? AND contype = 'c')`,
		constraintName,
	).Scan(&exists).Error
	require.NoError(t, err, "querying pg_constraint for %q", constraintName)
	require.True(t, exists, "expected CHECK constraint %q to exist after fresh-DB startup", constraintName)
}
