//go:build integration_postgres

package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// startRollbackRehearsalDB returns an isolated PostgreSQL 15 handle + DSN. With
// TEST_DATABASE_URL it creates a disposable child database; otherwise it starts
// a throwaway Testcontainers server. The supplied parent database is never the
// target of destructive down migrations.
func startRollbackRehearsalDB(t *testing.T, ctx context.Context) (*gorm.DB, string) {
	t.Helper()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	if err != nil {
		t.Fatalf("isolated postgres: %v (is Docker running, or can TEST_DATABASE_URL create child databases?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	return pg.DB, pg.DSN
}

// TestRollbackRehearsal_EachMigration walks every numbered migration layered on
// top of the genesis baseline DOWN one at a time, asserting each .down.sql
// applies cleanly, strictly decreases the version and never leaves a dirty
// marker, then rolls the range forward through the production RunMigrations
// path and asserts the catalog returns exactly to the genesis-plus-pending
// fingerprint. With no numbered migrations the window is empty and the test
// only proves bootstrap + RunMigrations are idempotent.
func TestRollbackRehearsal_EachMigration(t *testing.T) {
	ctx := context.Background()
	gdb, dsn := startRollbackRehearsalDB(t, ctx)

	require.NoError(t, BootstrapGenesisSchema(gdb), "bootstrap genesis (HEAD)")

	migrationsDir := freshDBMigrationsDir(t)
	host, port, user, pass, name, sslmode, err := parsePostgresDSN(dsn)
	require.NoError(t, err, "parse DSN")
	require.NoError(t,
		RunMigrations(host, port, user, pass, name, sslmode, migrationsDir),
		"apply pending post-genesis migrations before fingerprinting")
	headFP, err := SchemaFingerprint(gdb)
	require.NoError(t, err, "fingerprint genesis + pending HEAD")

	// The genesis baseline is regenerated after every migration, so it
	// already contains each numbered migration. Walk all of them down (to just
	// below the oldest one still on disk), not only those above the baseline
	// head: otherwise the window is always empty and no .down.sql ever runs.
	latest, err := LatestMigrationVersion(migrationsDir)
	require.NoError(t, err)
	floor := oldestMigrationFloor(t, migrationsDir)
	if uint(latest) > floor {
		m, err := migrate.New(fmt.Sprintf("file://%s", migrationsDir), dsn)
		require.NoError(t, err, "open migrate")
		prevVersion, dirty, err := m.Version()
		require.NoError(t, err, "schema_migrations must be readable after pending apply")
		require.False(t, dirty, "pending apply must not be dirty")
		require.Equal(t, uint(latest), prevVersion, "pending apply must reach the latest migration")

		for prevVersion > floor {
			require.NoErrorf(t, m.Steps(-1), "down migration %d must apply cleanly", prevVersion)
			v, dirtyNow, verr := m.Version()
			if errors.Is(verr, migrate.ErrNilVersion) {
				v, dirtyNow, verr = 0, false, nil
			}
			require.NoErrorf(t, verr, "version after down from %d", prevVersion)
			require.Falsef(t, dirtyNow, "down migration %d left a dirty version", prevVersion)
			require.Lessf(t, v, prevVersion, "down migration from %d must strictly decrease the version", prevVersion)
			prevVersion = v
		}
		require.Equalf(t, floor, prevVersion, "down-walk must land exactly below the oldest migration (v%d)", floor)
		_, _ = m.Close()
	}

	require.NoError(t,
		RunMigrations(host, port, user, pass, name, sslmode, migrationsDir),
		"forward re-application of migrations must succeed")

	rolledFP, err := SchemaFingerprint(gdb)
	require.NoError(t, err, "fingerprint after round trip")
	require.Empty(t, DiffFingerprints(headFP, rolledFP),
		"down→up rehearsal must restore the genesis+pending catalog (no orphaned/missing objects)")
}

// oldestMigrationFloor returns one below the oldest numbered migration in dir
// (0 when 000001 is still on disk, or when there are none).
func oldestMigrationFloor(t *testing.T, dir string) uint {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var oldest uint
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			continue
		}
		v, err := strconv.ParseUint(prefix, 10, 64)
		require.NoErrorf(t, err, "migration %s has no numeric prefix", name)
		if oldest == 0 || uint(v) < oldest {
			oldest = uint(v)
		}
	}
	if oldest == 0 {
		return 0
	}
	return oldest - 1
}
