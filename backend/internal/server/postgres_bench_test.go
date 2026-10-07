package server_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf"
	"github.com/stdevmac/payverge/backend/internal/testperf/genesisdb"
)

var serverBenchPostgres struct {
	once sync.Once
	pg   *testperf.Pg
	err  error
}

// setupServerBenchmarkDB pays the container and schema cost once per server
// test process, then resets all public data and loads fresh fixtures for every
// benchmark sample. The benchmarked writes still own real transactions; only
// repeated container and schema setup is removed from the nightly measurement.
func setupServerBenchmarkDB(b *testing.B) {
	b.Helper()
	if testing.Short() {
		b.Skip("skipping container bench in -short")
	}

	serverBenchPostgres.once.Do(func() {
		ctx := context.Background()
		serverBenchPostgres.pg, serverBenchPostgres.err = genesisdb.Start(ctx)
		if serverBenchPostgres.err != nil {
			return
		}
		database.SetTestDB(serverBenchPostgres.pg.DB)
	})
	if serverBenchPostgres.err != nil {
		b.Fatalf("initialize shared benchmark postgres: %v", serverBenchPostgres.err)
	}

	db := serverBenchPostgres.pg.DB
	database.SetTestDB(db)
	if err := db.Exec(`
DO $$
DECLARE table_list text;
BEGIN
    SELECT string_agg(format('%I.%I', schemaname, tablename), ', ')
      INTO table_list
      FROM pg_tables
     WHERE schemaname = 'public';
    IF table_list IS NOT NULL THEN
        EXECUTE 'TRUNCATE TABLE ' || table_list || ' RESTART IDENTITY CASCADE';
    END IF;
END $$;
`).Error; err != nil {
		b.Fatalf("reset benchmark data: %v", err)
	}
	if err := testperf.LoadFixtures(db); err != nil {
		b.Fatalf("load benchmark fixtures: %v", err)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	if serverBenchPostgres.pg != nil {
		_ = serverBenchPostgres.pg.Terminate(context.Background())
	}
	os.Exit(code)
}
