//go:build integration_postgres

package database

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

func execSQLFile(t *testing.T, dsn, path string) {
	t.Helper()
	command := exec.CommandContext(context.Background(), "psql", dsn, "-v", "ON_ERROR_STOP=1", "-f", path)
	output, err := command.CombinedOutput()
	require.NoErrorf(t, err, "execute %s:\n%s", path, string(output))
}

func TestDemoSeed_PostgresIdempotentAndVerified(t *testing.T) {
	ctx := context.Background()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	pg.DB = pg.DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})

	require.NoError(t, BootstrapGenesisSchema(pg.DB))
	// Apply the numbered migrations past the genesis head, exactly like a
	// booted stack (and the e2e seed step) does, so the seed runs against the
	// current schema and not the older baseline (000227 drops tables).
	require.NoError(t, runMigrationsForContainerDSN(t, pg.DSN, freshDBMigrationsDir(t)))
	seedPath := filepath.Join("..", "..", "scripts", "demo_seed.sql")
	verifyPath := filepath.Join("..", "..", "scripts", "verify_demo_seed.sql")
	execSQLFile(t, pg.DSN, seedPath)
	execSQLFile(t, pg.DSN, verifyPath)

	var firstCount int
	require.NoError(t, pg.DB.Raw(`SELECT count(*) FROM businesses WHERE business_id IN ('demo-core-business', 'demo-ai-pro-business')`).Scan(&firstCount).Error)
	require.Equal(t, 2, firstCount)

	execSQLFile(t, pg.DSN, seedPath)
	execSQLFile(t, pg.DSN, verifyPath)

	var secondCount int
	require.NoError(t, pg.DB.Raw(`SELECT count(*) FROM businesses WHERE business_id IN ('demo-core-business', 'demo-ai-pro-business')`).Scan(&secondCount).Error)
	require.Equal(t, firstCount, secondCount)
}
