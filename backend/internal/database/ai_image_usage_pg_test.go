//go:build integration_postgres

package database

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// setupAIImageUsagePostgres pins GetDB() to a fresh testcontainer Postgres and
// AutoMigrates AIImageUsage. Fails the test when Docker is unavailable — that
// is an environment gap, not a product defect, and is captured honestly.
func setupAIImageUsagePostgres(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Postgres integration test in -short")
	}
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	prev := db
	SetTestDB(pg.DB)
	t.Cleanup(func() { SetTestDB(prev) })

	require.NoError(t, db.AutoMigrate(&AIImageUsage{}))
}

// TestReserveImageGeneration_ConcurrentDoesNotOverIssue_Postgres exercises
// the race that the sqlite harness in ai_image_usage_test.go cannot: that
// harness pins its pool to a single connection, so every reserve there runs
// to completion before the next begins (see
// TestReserveImageGeneration_ConcurrentDoesNotOverIssue's own comment for why
// that test can only count grants rather than race them). Here N goroutines
// hold real separate Postgres connections and race the same conditional
// UPDATE under READ COMMITTED, so this is the one place in the suite where
// the daily-cap predicate's serialization is actually executed rather than
// inferred.
func TestReserveImageGeneration_ConcurrentDoesNotOverIssue_Postgres(t *testing.T) {
	setupAIImageUsagePostgres(t)

	b := &Business{ID: 1}
	_, err := EnsureImageUsageRow(b)
	require.NoError(t, err)

	const limit = 5
	const attempts = 25
	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := 0

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ReserveImageGeneration(b, limit, 1000); err == nil {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, limit, granted,
		"the daily cap must hold across concurrent reserves under real Postgres READ COMMITTED")

	var row AIImageUsage
	require.NoError(t, db.Where("business_id = ?", b.ID).First(&row).Error)
	assert.Equal(t, limit, row.DailyUsed, "daily_used must land exactly at the limit, never over-issue")
}

// Latch tests against real Postgres TIMESTAMPTZ.
//
// TIMESTAMPTZ success-path evidence for WHERE monthly_period_start = ? is
// ReleasesTheAlertItClaimed_Postgres only: it is the path where equality must
// match for the release to clear the latch. LeavesAnAlertItDidNotClaim never
// reaches the release UPDATE (AlertTriggered=false). LeavesTheNextPeriods
// only proves the mismatch → 0 rows case (unscoped release would free the
// wrong latch). On SQLite the equality is free; on pgx a round-trip mismatch
// would make the success path silently match zero rows — that is the failure
// class ReleasesTheAlertItClaimed_Postgres exists to catch.

func TestRefundImageGeneration_ReleasesTheAlertItClaimed_Postgres(t *testing.T) {
	setupAIImageUsagePostgres(t)
	b := &Business{ID: 1}

	_, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	crossing, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	require.True(t, crossing.AlertTriggered, "precondition: the crossing claims the alert")
	require.False(t, crossing.MonthlyPeriodStart.IsZero(),
		"reservation must carry the period it claimed for TIMESTAMPTZ equality")

	require.NoError(t, RefundImageGeneration(crossing))

	var row AIImageUsage
	require.NoError(t, db.Where("business_id = ?", b.ID).First(&row).Error)
	assert.Equal(t, 1, row.MonthlyUsed)
	assert.Nil(t, row.MonthlyAlertSentAt,
		"pgx TIMESTAMPTZ equality on monthly_period_start must release the claiming latch")

	again, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	assert.True(t, again.AlertTriggered,
		"a re-crossing must alert; a silently-missed release would silence the period")
}

func TestRefundImageGeneration_LeavesAnAlertItDidNotClaim_Postgres(t *testing.T) {
	setupAIImageUsagePostgres(t)
	b := &Business{ID: 1}

	_, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	crossing, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	require.True(t, crossing.AlertTriggered)

	later, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	require.False(t, later.AlertTriggered)
	require.NoError(t, RefundImageGeneration(later))

	var row AIImageUsage
	require.NoError(t, db.Where("business_id = ?", b.ID).First(&row).Error)
	assert.NotNil(t, row.MonthlyAlertSentAt,
		"an unrelated refund must not re-arm the latch on Postgres")
	assert.Equal(t, 2, row.MonthlyUsed, "the refund still rolls its own generation back")
}

func TestRefundImageGeneration_LeavesTheNextPeriodsAlertAlone_Postgres(t *testing.T) {
	setupAIImageUsagePostgres(t)
	b := &Business{ID: 1}

	crossing, err := ReserveImageGeneration(b, 100, 1)
	require.NoError(t, err)
	require.True(t, crossing.AlertTriggered)
	require.False(t, crossing.MonthlyPeriodStart.IsZero())

	// Advance the period the same way the SQLite test does: hand-apply the
	// resetStaleWindows effects so the old reservation's period no longer
	// matches the stored column.
	var rolled AIImageUsage
	require.NoError(t, db.Where("business_id = ?", b.ID).First(&rolled).Error)
	require.NoError(t, db.Model(&AIImageUsage{}).
		Where("business_id = ?", b.ID).
		Updates(map[string]interface{}{
			"monthly_used":          0,
			"monthly_period_start":  rolled.MonthlyPeriodStart.AddDate(0, 1, 0),
			"monthly_alert_sent_at": nil,
		}).Error)

	nextPeriod, err := ReserveImageGeneration(b, 100, 1)
	require.NoError(t, err)
	require.True(t, nextPeriod.AlertTriggered, "precondition: new period claims its own latch")

	// Old reservation refunds with its original MonthlyPeriodStart. Mismatch
	// → 0 rows leaves the new latch; a wrongly-unscoped release would free it.
	// Same-period equality success is ReleasesTheAlertItClaimed_Postgres.
	// Counter assert: a no-op refund would leave monthly_used at 1.
	require.NoError(t, RefundImageGeneration(crossing))

	var row AIImageUsage
	require.NoError(t, db.Where("business_id = ?", b.ID).First(&row).Error)
	assert.NotNil(t, row.MonthlyAlertSentAt,
		"a previous-period refund must not release the current period's latch under TIMESTAMPTZ equality")
	assert.Equal(t, 0, row.MonthlyUsed,
		"the stale refund must still return its slot (period-agnostic decrement)")

	again, err := ReserveImageGeneration(b, 100, 1)
	require.NoError(t, err)
	assert.False(t, again.AlertTriggered,
		"the period already alerted; a stale refund must not let it alert twice")

	// Sanity: the stored period is a UTC midnight (no sub-second component),
	// which is the round-trip safety claim the equality relies on.
	assert.Equal(t, 0, row.MonthlyPeriodStart.UTC().Hour())
	assert.Equal(t, 0, row.MonthlyPeriodStart.UTC().Minute())
	assert.Equal(t, 0, row.MonthlyPeriodStart.UTC().Second())
	assert.Equal(t, 0, row.MonthlyPeriodStart.UTC().Nanosecond())
}
