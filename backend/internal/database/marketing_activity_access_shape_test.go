package database

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// marketingActivityBenchSeq gives each benchmark run a fresh in-memory DSN so a
// shared-cache SQLite DB never accumulates rows across -count runs / b.N loops.
var marketingActivityBenchSeq atomic.Uint64

// TestMarketingActivityListProjectionAccessShape proves the Library list does its
// filtering/bounding in SQL: explicit projection (no SELECT *), a LIMIT, and a
// status filter in the WHERE — not app-side. Single list query, not N+1.
func TestMarketingActivityListProjectionAccessShape(t *testing.T) {
	d, cap := captureDB(t, &Business{}, &MarketingActivity{})
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 60; i++ {
		status := "posted"
		if i%2 == 1 {
			status = "dismissed"
		}
		require.NoError(t, db.Create(&MarketingActivity{
			BusinessID: 1, SuggestionID: fmt.Sprintf("1:offer:o%d", i),
			Play: "offer", Status: status, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}).Error)
	}

	cap.mu.Lock()
	cap.sqls = nil
	cap.mu.Unlock()

	rows, total, err := d.ListMarketingActivities(MarketingActivityQuery{
		BusinessID: 1, Status: "posted", Page: 1, PerPage: 50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(30), total)
	require.LessOrEqual(t, len(rows), 50, "result set is bounded by per_page")

	q := cap.matching("marketing_activities")
	require.NotEmpty(t, q, "expected a marketing_activities SELECT")
	// Two statements are expected: the COUNT and the bounded list SELECT.
	require.LessOrEqual(t, len(q), 2, "single count + single list query, not N+1")
	var listSQL string
	for _, s := range q {
		if !strings.Contains(s, "count(") && !strings.Contains(s, "COUNT(") {
			listSQL = s
		}
	}
	require.NotEmpty(t, listSQL, "expected a non-count list SELECT")
	require.NotContains(t, listSQL, "SELECT *", "explicit projection, not SELECT *")
	require.Contains(t, listSQL, "creative_snapshot", "snapshot must be part of the explicit projection")
	require.Contains(t, listSQL, "status", "status filter must be in SQL")
	require.Contains(t, listSQL, "LIMIT", "list must be bounded by a LIMIT")
}

// TestHandledMarketingActivitiesAccessShape proves the engine's handled-
// opportunity exclusion is one bounded, projected query (suggestion_id,
// status, posted_at; WHERE status; LIMIT), not a full-row scan.
func TestHandledMarketingActivitiesAccessShape(t *testing.T) {
	d, cap := captureDB(t, &Business{}, &MarketingActivity{})
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	ids := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("1:offer:o%d", i)
		ids = append(ids, id)
		require.NoError(t, db.Create(&MarketingActivity{
			BusinessID: 1, SuggestionID: id,
			Play: "offer", Status: "dismissed",
		}).Error)
	}
	cap.mu.Lock()
	cap.sqls = nil
	cap.mu.Unlock()

	rows, err := d.HandledMarketingActivities(1, ids, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	require.Len(t, rows, 10)

	q := cap.matching("marketing_activities")
	require.Len(t, q, 1, "single bounded query, not N+1")
	require.NotContains(t, q[0], "SELECT *", "projects the handled-row columns only")
	require.Contains(t, q[0], "suggestion_id", "projects suggestion_id")
	require.Contains(t, q[0], "status", "handled filter must be in SQL")
	require.Contains(t, q[0], "LIMIT", "bounded by the candidate count")
}

// BenchmarkListMarketingActivities captures the Library list scan cost.
func BenchmarkListMarketingActivities(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:BenchmarkListMarketingActivities_%d?mode=memory&cache=shared", marketingActivityBenchSeq.Add(1))
	gdb := mustOpenSQLiteForBench(b, dsn)
	defer func() { SetTestDB(prev) }()
	require.NoError(b, gdb.AutoMigrate(&Business{}, &MarketingActivity{}))
	SetTestDB(gdb)
	require.NoError(b, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 500; i++ {
		status := "posted"
		if i%2 == 1 {
			status = "dismissed"
		}
		_ = db.Create(&MarketingActivity{
			BusinessID: 1, SuggestionID: fmt.Sprintf("1:offer:o%d", i),
			Play: "offer", Status: status, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}).Error
	}
	d := GetDBWrapper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = d.ListMarketingActivities(MarketingActivityQuery{BusinessID: 1, Page: 1, PerPage: 50})
	}
}

// mustOpenSQLiteForBench opens an isolated in-memory SQLite DB for a benchmark,
// mirroring the concrete open used by BenchmarkGetActiveShiftsForDay
// (SetMaxOpenConns(1), Error-level logger).
func mustOpenSQLiteForBench(b *testing.B, dsn string) *gorm.DB {
	b.Helper()
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		b.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	return gdb
}
