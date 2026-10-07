package database

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// setupOpsRetentionPostgres points the package-global db at an isolated
// Postgres built the way startup builds an empty database: the genesis
// baseline plus every pending numbered migration.
func setupOpsRetentionPostgres(tb testing.TB) {
	tb.Helper()
	if testing.Short() {
		tb.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}
	ctx := context.Background()
	pg, err := testperf.StartIsolatedPostgres(ctx)
	if err != nil {
		tb.Fatalf("postgres: %v (is Docker running?)", err)
	}
	tb.Cleanup(func() { _ = pg.Terminate(ctx) })
	require.NoError(tb, BootstrapGenesisSchema(pg.DB), "bootstrap genesis")
	host, port, user, pass, name, sslmode, err := parsePostgresDSN(pg.DSN)
	require.NoError(tb, err, "parse DSN")
	require.NoError(tb, RunMigrations(host, port, user, pass, name, sslmode, opsRetentionMigrationsDir()), "pending migrations")

	prev := db
	SetTestDB(pg.DB)
	tb.Cleanup(func() { SetTestDB(prev) })
}

func opsRetentionMigrationsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
}

// explainNoSeqScan returns the plan for sqlText with sequential scans priced
// out, so a plan that still scans the table proves no usable index exists.
func explainNoSeqScan(t *testing.T, sqlText string) string {
	t.Helper()
	var plan []string
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL enable_seqscan = off`).Error; err != nil {
			return err
		}
		return tx.Raw("EXPLAIN " + sqlText).Scan(&plan).Error
	})
	require.NoError(t, err)
	return strings.Join(plan, "\n")
}

// The janitor's oldest-first selection and its child-row delete must be index
// reads: every hourly pass otherwise scans every ops thread and request row.
func TestOpsAssistantRetention_AccessShape_Postgres(t *testing.T) {
	setupOpsRetentionPostgres(t)

	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	selectSQL := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var ids []uint
		return inactiveOpsAssistantThreadIDs(tx, cutoff, 500).Pluck("id", &ids)
	})
	plan := explainNoSeqScan(t, selectSQL)
	require.Contains(t, plan, "idx_ops_assistant_threads_last_message_at", "selection must walk the last_message_at index:\n%s", plan)
	require.NotContains(t, plan, "Seq Scan on ops_assistant_threads", plan)

	deleteSQL := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Where("thread_id IN ?", []uint{1, 2, 3}).Delete(&OpsAssistantRequest{})
	})
	plan = explainNoSeqScan(t, deleteSQL)
	require.Contains(t, plan, "idx_ops_assistant_requests_thread_id", "request delete must use the thread_id index:\n%s", plan)
	require.NotContains(t, plan, "Seq Scan on ops_assistant_requests", plan)

	// Nothing archives ops threads, so the column and its index are gone.
	require.False(t, db.Migrator().HasColumn(&OpsAssistantThread{}, "archived_at"), "ops_assistant_threads.archived_at must be dropped")
	require.False(t, db.Migrator().HasIndex(&OpsAssistantThread{}, "idx_ops_assistant_threads_archived"), "archived index must be dropped")
}

// BenchmarkDeleteInactiveOpsAssistantThreads measures one 500-thread janitor
// pass against a table of 20k live threads (each with a request row).
func BenchmarkDeleteInactiveOpsAssistantThreads(b *testing.B) {
	setupOpsRetentionPostgres(b)

	const live, expiredPerPass = 20000, 500
	var businessID uint
	require.NoError(b, db.Raw(`
INSERT INTO businesses (business_id, owner_address, name, settlement_addr, tipping_addr, created_at, updated_at)
VALUES ('ops-retention-bench', '0xbeef', 'Ops Retention Bench', '0xbeef', '0xbeef', NOW(), NOW())
RETURNING id`).Scan(&businessID).Error)
	seed := func(n int, lastMessageAt time.Time, prefix string) {
		require.NoError(b, db.Exec(fmt.Sprintf(`
WITH t AS (
  INSERT INTO ops_assistant_threads (business_id, title, locale, last_message_at, created_at, updated_at)
  SELECT $2::bigint, 'bench', 'en', $1::timestamptz + (g * interval '1 second'), NOW(), NOW()
  FROM generate_series(1, %d) g
  RETURNING id
)
INSERT INTO ops_assistant_requests (business_id, thread_id, client_request_id, status)
SELECT $2::bigint, id, '%s-' || id, 'completed' FROM t`, n, prefix), lastMessageAt, businessID).Error)
	}
	seed(live, time.Now().UTC().Add(-time.Hour), "live")
	require.NoError(b, db.Exec(`ANALYZE ops_assistant_threads; ANALYZE ops_assistant_requests`).Error)

	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		seed(expiredPerPass, cutoff.Add(-24*time.Hour), fmt.Sprintf("old%d", i))
		b.StartTimer()
		res, err := DeleteInactiveOpsAssistantThreads(cutoff, expiredPerPass)
		if err != nil {
			b.Fatal(err)
		}
		if res.ThreadsDeleted != expiredPerPass {
			b.Fatalf("deleted %d threads, want %d", res.ThreadsDeleted, expiredPerPass)
		}
	}
}
