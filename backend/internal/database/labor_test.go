package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newLaborTestDB opens an isolated in-memory SQLite DB, migrates the tables
// used by labor-cost readers, registers it as the package DB, and returns the
// wrapper.
func newLaborTestDB(t *testing.T) (*DB, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &PayrollRun{}))
	SetTestDB(gormDB)
	return GetDBWrapper(), gormDB
}

// seedPaidRun creates a PAID payroll run for the given business with the given
// work period and gross total. Status is set explicitly on Create — it is a
// non-empty string column so it persists without a PaidAt timestamp.
func seedPaidRun(t *testing.T, gormDB *gorm.DB, businessID uint, periodStart, periodEnd time.Time, gross, bonus int64) PayrollRun {
	t.Helper()
	run := PayrollRun{
		BusinessID:  businessID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Status:      PayrollRunStatusPaid,
		Currency:    "USD",
		GrossTotal:  gross,
		BonusTotal:  bonus,
	}
	require.NoError(t, gormDB.Create(&run).Error)
	return run
}

// TestGetPaidPayrollRunsOverlapping_BoundariesAndStatus verifies that the
// reader returns exactly the PAID runs whose WORK PERIOD overlaps the window,
// honoring half-open [PeriodStart,PeriodEnd) overlap semantics and the
// status=paid filter (draft/void excluded even when fully inside the window).
func TestGetPaidPayrollRunsOverlapping_BoundariesAndStatus(t *testing.T) {
	db, gormDB := newLaborTestDB(t)

	biz := Business{Name: "Labor Test Bistro"}
	require.NoError(t, gormDB.Create(&biz).Error)

	day := 24 * time.Hour
	now := time.Now().UTC()
	windowStart := now.Add(-7 * day)
	windowEnd := now

	// PAID runs — each with a distinct GrossTotal so the returned set is
	// identifiable, plus a non-zero BonusTotal to confirm both columns flow.
	inside := seedPaidRun(t, gormDB, biz.ID,
		windowStart.Add(1*day), windowStart.Add(2*day), 100, 10) // fully inside -> INCLUDED
	straddleStart := seedPaidRun(t, gormDB, biz.ID,
		windowStart.Add(-1*day), windowStart.Add(1*day), 200, 20) // straddles start -> INCLUDED
	straddleEnd := seedPaidRun(t, gormDB, biz.ID,
		windowEnd.Add(-1*day), windowEnd.Add(1*day), 300, 30) // straddles end -> INCLUDED
	containing := seedPaidRun(t, gormDB, biz.ID,
		windowStart.Add(-1*day), windowEnd.Add(1*day), 400, 40) // fully contains -> INCLUDED

	// Excluded by period (no overlap).
	seedPaidRun(t, gormDB, biz.ID,
		windowStart.Add(-3*day), windowStart.Add(-2*day), 999, 99) // fully before -> EXCLUDED
	seedPaidRun(t, gormDB, biz.ID,
		windowEnd.Add(1*day), windowEnd.Add(2*day), 888, 88) // fully after -> EXCLUDED

	// Excluded by status — both fully inside the window.
	draft := PayrollRun{
		BusinessID:  biz.ID,
		PeriodStart: windowStart.Add(1 * day),
		PeriodEnd:   windowStart.Add(2 * day),
		Status:      PayrollRunStatusDraft,
		Currency:    "USD",
		GrossTotal:  777,
		BonusTotal:  77,
	}
	require.NoError(t, gormDB.Create(&draft).Error)
	void := PayrollRun{
		BusinessID:  biz.ID,
		PeriodStart: windowStart.Add(1 * day),
		PeriodEnd:   windowStart.Add(2 * day),
		Status:      PayrollRunStatusVoid,
		Currency:    "USD",
		GrossTotal:  666,
		BonusTotal:  66,
	}
	require.NoError(t, gormDB.Create(&void).Error)

	rows, err := db.GetPaidPayrollRunsOverlapping(biz.ID, windowStart, windowEnd)
	require.NoError(t, err)
	require.Len(t, rows, 4, "exactly the 4 paid runs overlapping the work-period window must return")

	byID := make(map[uint]PayrollRunCost, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}

	for _, want := range []struct {
		run   PayrollRun
		gross int64
		bonus int64
		label string
	}{
		{inside, 100, 10, "fully inside"},
		{straddleStart, 200, 20, "straddling start"},
		{straddleEnd, 300, 30, "straddling end"},
		{containing, 400, 40, "fully containing"},
	} {
		got, ok := byID[want.run.ID]
		require.True(t, ok, "%s run must be included", want.label)
		assert.Equal(t, want.gross, got.GrossTotal, "%s GrossTotal", want.label)
		assert.Equal(t, want.bonus, got.BonusTotal, "%s BonusTotal", want.label)
		assert.False(t, got.PeriodStart.IsZero(), "%s PeriodStart must be projected", want.label)
		assert.False(t, got.PeriodEnd.IsZero(), "%s PeriodEnd must be projected", want.label)
	}
}

// TestGetPaidPayrollRunsOverlapping_Isolation verifies cross-business isolation:
// a PAID overlapping run in a different business must not appear.
func TestGetPaidPayrollRunsOverlapping_Isolation(t *testing.T) {
	db, gormDB := newLaborTestDB(t)

	bizA := Business{Name: "Alpha Labor", BusinessId: "labor-iso-a"}
	bizB := Business{Name: "Beta Labor", BusinessId: "labor-iso-b"}
	require.NoError(t, gormDB.Create(&bizA).Error)
	require.NoError(t, gormDB.Create(&bizB).Error)

	day := 24 * time.Hour
	now := time.Now().UTC()
	windowStart := now.Add(-7 * day)
	windowEnd := now

	mine := seedPaidRun(t, gormDB, bizA.ID,
		windowStart.Add(1*day), windowStart.Add(2*day), 100, 10)
	// Overlapping PAID run in bizB — must be excluded from bizA's query.
	seedPaidRun(t, gormDB, bizB.ID,
		windowStart.Add(1*day), windowStart.Add(2*day), 500, 50)

	rows, err := db.GetPaidPayrollRunsOverlapping(bizA.ID, windowStart, windowEnd)
	require.NoError(t, err)
	require.Len(t, rows, 1, "only bizA's overlapping run must return")
	assert.Equal(t, mine.ID, rows[0].ID)
	assert.Equal(t, int64(100), rows[0].GrossTotal)
}

// TestGetPaidPayrollRunsOverlapping_SingleQuery asserts the reader issues
// exactly ONE SQL query that touches payroll_runs — no N+1.
func TestGetPaidPayrollRunsOverlapping_SingleQuery(t *testing.T) {
	db, gormDB := newLaborTestDB(t)

	biz := Business{Name: "Throughput Labor"}
	require.NoError(t, gormDB.Create(&biz).Error)

	day := 24 * time.Hour
	now := time.Now().UTC()
	windowStart := now.Add(-7 * day)
	windowEnd := now

	// Seed several overlapping paid runs.
	for i := 0; i < 5; i++ {
		seedPaidRun(t, gormDB, biz.ID,
			windowStart.Add(time.Duration(i)*time.Hour),
			windowStart.Add(time.Duration(i+1)*time.Hour),
			int64((i+1)*100), int64((i+1)*10))
	}

	// NOTE: Scan in GORM calls Rows() internally, which uses the Row callback
	// chain (gorm:row), NOT the Query chain (gorm:query). Registering
	// After("gorm:row") is the correct hook for Scan-based queries.
	cbName := "count_payroll_queries_" + t.Name()
	var movementQueries int
	require.NoError(t, gormDB.Callback().Row().After("gorm:row").
		Register(cbName, func(tx *gorm.DB) {
			if strings.Contains(strings.ToLower(tx.Statement.SQL.String()), "payroll_runs") {
				movementQueries++
			}
		}))
	t.Cleanup(func() { _ = gormDB.Callback().Row().Remove(cbName) })

	rows, err := db.GetPaidPayrollRunsOverlapping(biz.ID, windowStart, windowEnd)
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	require.Equal(t, 1, movementQueries,
		"GetPaidPayrollRunsOverlapping must issue exactly ONE query touching payroll_runs")
}
