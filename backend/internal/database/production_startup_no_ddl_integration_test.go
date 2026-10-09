//go:build integration_postgres

package database

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// TestProductionStartup_WorksWithoutDDLPrivileges proves a second production
// startup can run migrations-at-head and read-only schema verification through
// an application role that owns no objects and has no CREATE privilege. This
// catches even transient CREATE/ALTER attempts; fingerprint equality alone can
// miss DDL that is later reversed.
func TestProductionStartup_WorksWithoutDDLPrivileges(t *testing.T) {
	ctx := context.Background()
	// Always an isolated database: the test bootstraps a fresh schema and must
	// never run against a shared TEST_DATABASE_URL parent.
	pg, err := testperf.StartIsolatedPostgres(ctx)
	if err != nil {
		t.Fatalf("isolated postgres: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	ownerDB, ownerDSN := pg.DB, pg.DSN

	require.NoError(t, BootstrapGenesisSchema(ownerDB), "BootstrapGenesisSchema")

	host, port, owner, ownerPass, dbName, sslmode, err := parsePostgresDSN(ownerDSN)
	require.NoError(t, err)
	require.NoError(t, RunMigrations(host, port, owner, ownerPass, dbName, sslmode, freshDBMigrationsDir(t)))
	latest, err := LatestMigrationVersion(freshDBMigrationsDir(t))
	require.NoError(t, err)
	require.NoError(t, VerifySchemaAtVersion(ownerDB, int64(latest)))

	sha1, err := SchemaFingerprintSHA(ownerDB)
	require.NoError(t, err, "SchemaFingerprintSHA before production tail")
	require.NotEmpty(t, sha1)

	const startupUser = "payverge_startup_noddl"
	const startupPassword = "test-only-password"
	require.NoError(t, ownerDB.Exec(fmt.Sprintf(`CREATE ROLE %s LOGIN PASSWORD '%s'`, startupUser, startupPassword)).Error)
	// Roles are cluster-wide; drop it before the isolated database goes away so
	// a rerun against the same server starts clean.
	t.Cleanup(func() {
		_ = ownerDB.Exec(fmt.Sprintf(`DROP OWNED BY %s`, startupUser)).Error
		_ = ownerDB.Exec(fmt.Sprintf(`DROP ROLE IF EXISTS %s`, startupUser)).Error
	})
	require.NoError(t, ownerDB.Exec(fmt.Sprintf(`GRANT CONNECT ON DATABASE %s TO %s`, quoteIdent(dbName), startupUser)).Error)
	require.NoError(t, ownerDB.Exec(fmt.Sprintf(`GRANT USAGE ON SCHEMA public TO %s`, startupUser)).Error)
	require.NoError(t, ownerDB.Exec(fmt.Sprintf(`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO %s`, startupUser)).Error)
	require.NoError(t, ownerDB.Exec(fmt.Sprintf(`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO %s`, startupUser)).Error)
	require.NoError(t, ownerDB.Exec(fmt.Sprintf(`REVOKE CREATE ON SCHEMA public FROM %s`, startupUser)).Error)

	restrictedDSN := replaceDSNUser(t, ownerDSN, startupUser, startupPassword)
	restricted, err := gorm.Open(postgres.Open(restrictedDSN), &gorm.Config{})
	require.NoError(t, err)

	// The exact migration invocation used by cmd/app on every startup. At HEAD it
	// must not attempt CREATE/ALTER, because the restricted role owns no objects.
	require.NoError(t, RunMigrations(host, port, startupUser, startupPassword, dbName, sslmode, freshDBMigrationsDir(t)))
	require.NoError(t, VerifySchemaAtVersion(restricted, int64(latest)))
	_, err = VerifySchemaFingerprint(restricted, int64(latest))
	require.NoError(t, err, "a no-DDL startup role verifies the genesis fingerprint")

	sha2, err := SchemaFingerprintSHA(ownerDB)
	require.NoError(t, err, "SchemaFingerprintSHA after production tail")
	require.Equal(t, sha1, sha2)
}

func replaceDSNUser(t *testing.T, dsn, user, password string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.User = url.UserPassword(user, password)
	return u.String()
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
