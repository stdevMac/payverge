//go:build integration_postgres

package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/testperf"
	"github.com/stdevmac/payverge/backend/schema/genesis"
)

func startEmptyPostgres(t *testing.T) *testperf.Pg {
	t.Helper()
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	return pg
}

func countSchemaMigrations(t *testing.T, pg *testperf.Pg) int64 {
	t.Helper()
	var n int64
	require.NoError(t, pg.DB.Raw(`SELECT count(*) FROM public.schema_migrations`).Scan(&n).Error)
	return n
}

func readSchemaMigration(t *testing.T, pg *testperf.Pg) (version int64, dirty bool) {
	t.Helper()
	require.NoError(t, pg.DB.Raw(`SELECT version, dirty FROM public.schema_migrations LIMIT 1`).
		Row().Scan(&version, &dirty))
	return version, dirty
}

// requireLedgerAt asserts the schema_migrations ledger records head: empty
// for a head-0 baseline (no numbered migrations), else one clean row.
func requireLedgerAt(t *testing.T, pg *testperf.Pg, head int64) {
	t.Helper()
	if head == 0 {
		require.Zero(t, countSchemaMigrations(t, pg), "a head-0 ledger has no rows")
		return
	}
	require.Equal(t, int64(1), countSchemaMigrations(t, pg), "exactly one schema_migrations row")
	version, dirty := readSchemaMigration(t, pg)
	require.Equal(t, head, version)
	require.False(t, dirty)
}

func assertRequiredTables(t *testing.T, pg *testperf.Pg) {
	t.Helper()
	require.NoError(t, verifyRequiredTables(pg.DB))
}

func craftVersionJSON(t *testing.T, head int64, schemaSHA string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]interface{}{
		"migration_head": head,
		"schema_sha256":  schemaSHA,
		"postgres_major": 18,
		"generator":      "test",
	})
	require.NoError(t, err)
	return b
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestBootstrapGenesis_EmptyDBSucceeds(t *testing.T) {
	pg := startEmptyPostgres(t)

	class, err := ClassifyDatabase(pg.DB)
	require.NoError(t, err)
	require.Equal(t, DBEmpty, class)

	require.NoError(t, BootstrapGenesisSchema(pg.DB))

	requireLedgerAt(t, pg, GenesisMigrationHead())
	assertRequiredTables(t, pg)

	class, err = ClassifyDatabase(pg.DB)
	require.NoError(t, err)
	require.Equal(t, DBVersioned, class)
}

func TestBootstrapGenesis_NonEmptyRefused(t *testing.T) {
	pg := startEmptyPostgres(t)
	migsDir := freshDBMigrationsDir(t)
	require.NoError(t, ReconcileReferenceSchema(pg.DB, pg.DSN, migsDir))

	class, err := ClassifyDatabase(pg.DB)
	require.NoError(t, err)
	require.Equal(t, DBVersioned, class)

	// Reconcile applies every numbered migration, not only the genesis head.
	latest, latestErr := LatestMigrationVersion(migsDir)
	require.NoError(t, latestErr)
	requireLedgerAt(t, pg, latest)

	// Non-empty established DB: bootstrap is an idempotent no-op.
	require.NoError(t, BootstrapGenesisSchema(pg.DB))
	requireLedgerAt(t, pg, latest)
}

// A hand-made schema whose tool created an empty schema_migrations table is
// not the genesis baseline: without the full genesis table set it must stay
// legacy so startup refuses it, and bootstrap must not touch it.
func TestClassifyDatabase_EmptyLedgerWithoutGenesisTablesIsLegacy(t *testing.T) {
	pg := startEmptyPostgres(t)
	require.NoError(t, pg.DB.Exec(`
		CREATE TABLE public.schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL);
		CREATE TABLE public.businesses (id bigserial PRIMARY KEY, name text NOT NULL)`).Error)

	class, err := ClassifyDatabase(pg.DB)
	require.NoError(t, err)
	require.Equal(t, DBLegacy, class)

	require.NoError(t, BootstrapGenesisSchema(pg.DB))
	var bills bool
	require.NoError(t, pg.DB.Raw(`SELECT to_regclass('public.bills') IS NOT NULL`).Scan(&bills).Error)
	require.False(t, bills, "bootstrap must not apply the baseline to a non-empty database")
}

// The same hand-made schema with a ledger row belongs to golang-migrate and
// classifies as versioned; VerifySchemaAtVersion is what guards its contents.
func TestClassifyDatabase_LedgerRowIsVersioned(t *testing.T) {
	pg := startEmptyPostgres(t)
	require.NoError(t, pg.DB.Exec(`
		CREATE TABLE public.schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL);
		INSERT INTO public.schema_migrations VALUES (1, false);
		CREATE TABLE public.businesses (id bigserial PRIMARY KEY, name text NOT NULL)`).Error)

	class, err := ClassifyDatabase(pg.DB)
	require.NoError(t, err)
	require.Equal(t, DBVersioned, class)
}

// A bootstrapped baseline matches the fingerprint recorded in version.json;
// one added column is drift and fails verification.
func TestVerifySchemaFingerprint_DetectsDrift(t *testing.T) {
	pg := startEmptyPostgres(t)
	require.NoError(t, BootstrapGenesisSchema(pg.DB))
	head := GenesisMigrationHead()

	sha, err := VerifySchemaFingerprint(pg.DB, head)
	require.NoError(t, err)
	require.NotEmpty(t, sha)

	require.NoError(t, pg.DB.Exec(`ALTER TABLE public.bills ADD COLUMN drift_probe text`).Error)
	_, err = VerifySchemaFingerprint(pg.DB, head)
	require.ErrorContains(t, err, "schema drift")
}

func TestBootstrapGenesis_SecondRunIdempotent(t *testing.T) {
	pg := startEmptyPostgres(t)

	require.NoError(t, BootstrapGenesisSchema(pg.DB))
	require.NoError(t, BootstrapGenesisSchema(pg.DB))

	requireLedgerAt(t, pg, GenesisMigrationHead())
	assertRequiredTables(t, pg)
}

func TestBootstrapGenesis_BaselineFailureRollsBack(t *testing.T) {
	pg := startEmptyPostgres(t)

	badSQL := "CREATE TABLE ok_marker(id int);\nTHIS IS NOT VALID SQL;"
	vjson := craftVersionJSON(t, 139, sha256Hex(badSQL))

	err := bootstrapGenesis(pg.DB, badSQL, vjson)
	require.Error(t, err)

	count, err := countPublicBaseTables(pg.DB)
	require.NoError(t, err)
	require.Equal(t, int64(0), count, "failed baseline must roll back; ok_marker must not remain")
}

func TestBootstrapGenesis_SHAMismatchRefused(t *testing.T) {
	pg := startEmptyPostgres(t)

	vjson := craftVersionJSON(t, 139, "0000000000000000000000000000000000000000000000000000000000000000")
	err := bootstrapGenesis(pg.DB, genesis.SchemaSQL, vjson)
	require.Error(t, err)
	require.Contains(t, err.Error(), "SHA")

	count, err := countPublicBaseTables(pg.DB)
	require.NoError(t, err)
	require.Equal(t, int64(0), count, "SHA mismatch must not touch the DB")
}

func TestBootstrapGenesis_ConcurrentSingleWinner(t *testing.T) {
	pg := startEmptyPostgres(t)

	const n = 2
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			errs[i] = BootstrapGenesisSchema(pg.DB)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d", i)
	}

	requireLedgerAt(t, pg, GenesisMigrationHead())
	assertRequiredTables(t, pg)
}

func TestBootstrapGenesis_ResetsSessionSettings(t *testing.T) {
	pg := startEmptyPostgres(t)

	sqlDB, err := pg.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	// Give the database non-zero defaults so a leaked pg_dump "SET ... = 0"
	// is observable, then drop the idle connection so the next one starts
	// with those defaults.
	for _, stmt := range []string{
		`ALTER DATABASE ` + currentDatabaseName(t, pg) + ` SET statement_timeout = '30s'`,
		`ALTER DATABASE ` + currentDatabaseName(t, pg) + ` SET lock_timeout = '10s'`,
		`ALTER DATABASE ` + currentDatabaseName(t, pg) + ` SET idle_in_transaction_session_timeout = '60s'`,
	} {
		require.NoError(t, pg.DB.Exec(stmt).Error)
	}
	sqlDB.SetMaxIdleConns(0)
	sqlDB.SetMaxIdleConns(1)
	require.Equal(t, "30s", showSetting(t, pg, "statement_timeout"))

	settings := []string{
		"statement_timeout",
		"lock_timeout",
		"idle_in_transaction_session_timeout",
		"row_security",
	}
	before := make(map[string]string, len(settings))
	for _, name := range settings {
		before[name] = showSetting(t, pg, name)
	}

	require.NoError(t, BootstrapGenesisSchema(pg.DB))

	for _, name := range settings {
		require.Equal(t, before[name], showSetting(t, pg, name), name)
	}
	require.Contains(t, showSetting(t, pg, "search_path"), "public")
}

func showSetting(t *testing.T, pg *testperf.Pg, name string) string {
	t.Helper()
	var value string
	require.NoError(t, pg.DB.Raw("SHOW "+name).Row().Scan(&value))
	return value
}

func currentDatabaseName(t *testing.T, pg *testperf.Pg) string {
	t.Helper()
	var name string
	require.NoError(t, pg.DB.Raw("SELECT current_database()").Row().Scan(&name))
	return `"` + name + `"`
}
