package database

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A shared-cache in-memory sqlite pinned to one connection, so the concurrency
// test shares a pool without "database is locked" while sqlite still serialises
// writes and the conditional-UPDATE invariant stays deterministic. The package's
// own setupTestDB (bill_items_test.go) does not give that, which is why these
// tests carry their own setup.
//
// AutoMigrate is fine here — the production ban (TestAutoMigrateSourceGate)
// covers startup paths, not test scaffolding.
func setupAIImageUsageTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file:ai_image_usage_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	_ = gormDB.Migrator().DropTable(&AIImageUsage{})
	require.NoError(t, gormDB.AutoMigrate(&AIImageUsage{}))

	db = gormDB
}

// The package has no createTestBusiness helper, and these tests only need an ID.
func testBusiness(id uint) *Business { return &Business{ID: id} }

// TestEnsureImageUsageRow_UpsertsWithoutClobbering pins first-use creation as an
// atomic upsert. A SELECT-then-INSERT lets two concurrent first generations for
// the same business both miss the read, and one then trips the business_id
// unique index — a duplicate-key 500 on a brand new business's very first
// request, which is exactly the bulk-generation path this feature exists for.
// This harness serialises writes so it cannot stage that race; what it can pin
// is the statement shape (the INSERT carries ON CONFLICT) and the DO NOTHING
// semantics (re-entry returns the stored row and leaves the counters alone).
func TestEnsureImageUsageRow_UpsertsWithoutClobbering(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(7)

	var mu sync.Mutex
	var inserts []string
	require.NoError(t, GetDB().Callback().Create().After("gorm:create").
		Register("test:capture_usage_insert", func(tx *gorm.DB) {
			sql := tx.Statement.SQL.String()
			if strings.Contains(sql, "ai_image_usage") {
				mu.Lock()
				inserts = append(inserts, sql)
				mu.Unlock()
			}
		}))

	first, err := EnsureImageUsageRow(b)
	require.NoError(t, err)
	require.Len(t, inserts, 1, "first use issues exactly one INSERT")
	assert.Contains(t, inserts[0], "ON CONFLICT",
		"first-use creation must be an atomic upsert, not a SELECT followed by an INSERT")

	// Spend one generation, then re-enter on the existing row.
	_, err = ReserveImageGeneration(b, 10, 100)
	require.NoError(t, err)

	second, err := EnsureImageUsageRow(b)
	require.NoError(t, err, "re-entry on an existing row must not surface a duplicate-key error")
	assert.Equal(t, first.ID, second.ID, "the upsert must return the stored row, not a fresh one")
	assert.Equal(t, 1, second.DailyUsed, "DO NOTHING must not clobber the stored counters")
	assert.Equal(t, 1, second.MonthlyUsed)

	var rows int64
	require.NoError(t, GetDB().Model(&AIImageUsage{}).
		Where("business_id = ?", b.ID).Count(&rows).Error)
	assert.EqualValues(t, 1, rows, "exactly one usage row per business")
}

func TestReserveImageGeneration_BlocksAtDailyLimit(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	for i := 0; i < 3; i++ {
		_, err := ReserveImageGeneration(b, 3, 100)
		require.NoError(t, err, "reservation %d should succeed", i+1)
	}

	_, err := ReserveImageGeneration(b, 3, 100)
	var limitErr ImageDailyLimitError
	require.True(t, errors.As(err, &limitErr), "expected ImageDailyLimitError, got %v", err)
	assert.Equal(t, 3, limitErr.Limit)
	assert.True(t, limitErr.ResetsAt.After(time.Now().UTC()), "reset must be in the future")
}

func TestReserveImageGeneration_RollsDailyWindow(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	_, err := ReserveImageGeneration(b, 1, 100)
	require.NoError(t, err)

	// Backdate the stored window one day; the next reserve must roll it.
	require.NoError(t, GetDB().Model(&AIImageUsage{}).
		Where("business_id = ?", b.ID).
		UpdateColumn("daily_period_start", time.Now().UTC().AddDate(0, 0, -1)).Error)

	_, err = ReserveImageGeneration(b, 1, 100)
	require.NoError(t, err, "a new UTC day must reset daily_used")

	var row AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&row).Error)
	assert.Equal(t, 1, row.DailyUsed, "daily counter resets to this reservation only")
	assert.Equal(t, 2, row.MonthlyUsed, "monthly counter spans the day boundary")
}

// TestReserveImageGeneration_ConcurrentDoesNotOverIssue guards the coarse
// regression of dropping the `daily_used < ?` predicate altogether. It cannot
// distinguish the conditional UPDATE from a transactional read-modify-write:
// the harness pins the pool to one connection and database/sql pins that
// connection to a Tx for its lifetime, so every reserve runs to completion
// before the next one begins. The predicate itself is pinned by
// TestReserveImageGeneration_RefusalIsTheUpdatePredicate below.
func TestReserveImageGeneration_ConcurrentDoesNotOverIssue(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	// Create the row up front so the goroutines race only the reserve path. An
	// insert race would otherwise be absorbed into the failed-attempt count and
	// could starve the granted == limit assertion of survivors.
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

	assert.Equal(t, limit, granted, "the daily cap must hold across concurrent reserves")
}

// TestReserveImageGeneration_RefusalIsTheUpdatePredicate pins *where* the cap is
// enforced, which is the property the concurrency test above cannot see. A
// transactional read-then-write ("SELECT daily_used; if < limit then UPDATE")
// produces identical counts on this single-connection sqlite harness yet
// over-issues under Postgres READ COMMITTED, so the safety claim needs an
// assertion on the statement shape: the increment must carry its own
// `daily_used < ?` predicate, and the refusal must come from that UPDATE
// matching zero rows rather than from an application-level read.
func TestReserveImageGeneration_RefusalIsTheUpdatePredicate(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	var mu sync.Mutex
	var increments []string
	require.NoError(t, GetDB().Callback().Update().After("gorm:update").
		Register("test:capture_reserve_increment", func(tx *gorm.DB) {
			sql := tx.Statement.SQL.String()
			if strings.Contains(sql, "daily_used + 1") {
				mu.Lock()
				increments = append(increments, sql)
				mu.Unlock()
			}
		}))

	_, err := ReserveImageGeneration(b, 1, 100)
	require.NoError(t, err)
	require.Len(t, increments, 1, "a granted reserve issues exactly one incrementing UPDATE")
	assert.Contains(t, increments[0], "daily_used < ?",
		"the cap must be a predicate on the incrementing UPDATE, not a separate read")

	var before AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&before).Error)

	_, err = ReserveImageGeneration(b, 1, 100)
	var limitErr ImageDailyLimitError
	require.True(t, errors.As(err, &limitErr), "expected ImageDailyLimitError, got %v", err)
	require.Len(t, increments, 2,
		"the refused reserve must still attempt the guarded UPDATE, not short-circuit on a read")
	assert.Contains(t, increments[1], "daily_used < ?")

	var after AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&after).Error)
	assert.Equal(t, before.DailyUsed, after.DailyUsed, "a refused reserve must not move daily_used")
	assert.Equal(t, before.MonthlyUsed, after.MonthlyUsed, "a refused reserve must not move monthly_used")
	assert.Equal(t, before.MonthlyAlertSentAt, after.MonthlyAlertSentAt,
		"a refused reserve must not claim the alert latch")
}

func TestRefundImageGeneration_DecrementsBothCounters(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	res, err := ReserveImageGeneration(b, 10, 100)
	require.NoError(t, err)
	require.NoError(t, RefundImageGeneration(res))

	var row AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&row).Error)
	assert.Equal(t, 0, row.DailyUsed)
	assert.Equal(t, 0, row.MonthlyUsed)
}

func TestRefundImageGeneration_NeverGoesNegative(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	res, err := ReserveImageGeneration(b, 10, 100)
	require.NoError(t, err)
	require.NoError(t, RefundImageGeneration(res))
	require.NoError(t, RefundImageGeneration(res), "double refund must be a no-op, not an error")

	var row AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&row).Error)
	assert.Equal(t, 0, row.DailyUsed)
	assert.Equal(t, 0, row.MonthlyUsed)
}

// Refunding the reservation that claimed the alert must release the claim.
// Otherwise one failed provider call at the threshold silences the internal
// monthly alert for the rest of the period: monthly_used drops back below the
// threshold, but the latch stays set, so no later crossing can ever alert.
func TestRefundImageGeneration_ReleasesTheAlertItClaimed(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	// Threshold of 2: the second reservation crosses it and claims the alert.
	_, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	crossing, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	require.True(t, crossing.AlertTriggered, "precondition: the crossing claims the alert")

	// That generation fails at the provider and is refunded.
	require.NoError(t, RefundImageGeneration(crossing))

	var row AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&row).Error)
	assert.Equal(t, 1, row.MonthlyUsed, "the refund rolls the crossing back")
	assert.Nil(t, row.MonthlyAlertSentAt,
		"refunding the claiming reservation must clear the latch")

	// Re-crossing the threshold has to alert again.
	again, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	assert.True(t, again.AlertTriggered,
		"a re-crossing must alert; a stuck latch would silence the period")
}

// A refund that did not claim the alert must leave the latch alone: the period
// has already alerted, and re-alerting on every oscillation around the
// threshold would make the signal noise.
func TestRefundImageGeneration_LeavesAnAlertItDidNotClaim(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	_, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	crossing, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	require.True(t, crossing.AlertTriggered)

	// A later reservation fails; it never claimed the alert.
	later, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	require.False(t, later.AlertTriggered)
	require.NoError(t, RefundImageGeneration(later))

	var row AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&row).Error)
	assert.NotNil(t, row.MonthlyAlertSentAt,
		"the period already alerted; an unrelated refund must not re-arm it")
	// Assert the refund itself ran. Without this the test passes against a
	// RefundImageGeneration that does nothing at all, since "latch untouched"
	// is also what a no-op produces.
	assert.Equal(t, 2, row.MonthlyUsed, "the unrelated refund still rolls its own generation back")
}

// The latch release must not ride on the counter predicate. A slow provider
// spanning UTC midnight leaves a claiming reservation outstanding while the
// daily window rolls underneath it and a sibling refund drains the counters to
// zero. The claimer's own refund then finds `daily_used > 0` false — and if the
// latch clear shares that WHERE, it is silently skipped and the function still
// returns nil. The period is then latched with nothing above the threshold:
// exactly the silence the release was added to prevent.
func TestRefundImageGeneration_ReleasesTheAlertEvenWhenCountersAreDrained(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	// Threshold of 1: the first reservation crosses it and claims the alert.
	crossing, err := ReserveImageGeneration(b, 100, 1)
	require.NoError(t, err)
	require.True(t, crossing.AlertTriggered, "precondition: the crossing claims the alert")

	// Its provider call hangs past UTC midnight; the daily window goes stale.
	require.NoError(t, GetDB().Model(&AIImageUsage{}).
		Where("business_id = ?", b.ID).
		UpdateColumn("daily_period_start", time.Now().UTC().AddDate(0, 0, -1)).Error)

	// A generation on the new day resets daily_used to 1 and does not claim.
	sibling, err := ReserveImageGeneration(b, 100, 1)
	require.NoError(t, err)
	require.False(t, sibling.AlertTriggered)

	// The sibling fails too, draining daily_used back to 0 while the original
	// crossing is still outstanding.
	require.NoError(t, RefundImageGeneration(sibling))
	var drained AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&drained).Error)
	require.Equal(t, 0, drained.DailyUsed, "precondition: the counter predicate now misses")

	// Only now does the original crossing fail and refund.
	require.NoError(t, RefundImageGeneration(crossing))

	var row AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&row).Error)
	assert.Nil(t, row.MonthlyAlertSentAt,
		"the claiming refund must release the latch even when the counters cannot move")

	again, err := ReserveImageGeneration(b, 100, 1)
	require.NoError(t, err)
	assert.True(t, again.AlertTriggered,
		"a re-crossing must alert; a trapped latch would silence the period")
}

// The release must also be scoped to the period that claimed the latch. A
// monthly roll zeroes the count and clears the latch, so a reservation still
// outstanding from the previous period has nothing left to release. If its
// refund lands after a new crossing has armed the new period's latch, an
// unscoped release frees a latch belonging to a genuine, un-refunded alert —
// and that period then alerts a second time for a single crossing.
func TestRefundImageGeneration_LeavesTheNextPeriodsAlertAlone(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	// Threshold of 1: the first reservation crosses it and claims the alert.
	crossing, err := ReserveImageGeneration(b, 100, 1)
	require.NoError(t, err)
	require.True(t, crossing.AlertTriggered, "precondition: the crossing claims the alert")

	// Its provider call hangs past the billing anchor and the window rolls. The
	// roll is applied by hand — advancing the stored period exactly as
	// resetStaleWindows would — because the natural triggers (the clock crossing
	// the anchor, or SyncImageUsageAnchor moving it) cannot be staged
	// deterministically from a unit test. Dating the new window forward also
	// keeps resetStaleWindows from rolling it again on the next reserve.
	var rolled AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&rolled).Error)
	require.NoError(t, GetDB().Model(&AIImageUsage{}).
		Where("business_id = ?", b.ID).
		Updates(map[string]interface{}{
			"monthly_used":          0,
			"monthly_period_start":  rolled.MonthlyPeriodStart.AddDate(0, 1, 0),
			"monthly_alert_sent_at": nil,
		}).Error)

	// The next generation lands in the new period and re-crosses the threshold,
	// claiming a fresh latch of its own.
	nextPeriod, err := ReserveImageGeneration(b, 100, 1)
	require.NoError(t, err)
	require.True(t, nextPeriod.AlertTriggered, "precondition: the new period claims its own alert")

	// Only now does the previous period's generation fail and refund.
	// monthly_used is 1 from nextPeriod; the period-agnostic decrement must
	// still run (O1). Without this assert the test is green against a no-op
	// RefundImageGeneration — "latch untouched" is also what a no-op produces.
	require.NoError(t, RefundImageGeneration(crossing))

	var row AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&row).Error)
	assert.NotNil(t, row.MonthlyAlertSentAt,
		"a refund from the previous period must not release the current period's latch")
	assert.Equal(t, 0, row.MonthlyUsed,
		"the stale refund must still return its slot (period-agnostic decrement)")

	// The surviving latch has to keep doing its job: one alert per period.
	again, err := ReserveImageGeneration(b, 100, 1)
	require.NoError(t, err)
	assert.False(t, again.AlertTriggered,
		"the period already alerted; a stale refund must not let it alert twice")
}

func TestReserveImageGeneration_AlertsOncePerPeriod(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	// Threshold of 2: the second reservation crosses it, the third does not.
	first, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	assert.False(t, first.AlertTriggered, "below threshold must not alert")

	second, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	assert.True(t, second.AlertTriggered, "the crossing reservation alerts")

	third, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	assert.False(t, third.AlertTriggered, "the latch suppresses repeats within the period")

	// Roll the monthly window; the latch must clear and alert again.
	require.NoError(t, GetDB().Model(&AIImageUsage{}).
		Where("business_id = ?", b.ID).
		UpdateColumn("monthly_period_start", time.Now().UTC().AddDate(0, -2, 0)).Error)

	fourth, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	assert.False(t, fourth.AlertTriggered, "a fresh period starts below the threshold")

	fifth, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	assert.True(t, fifth.AlertTriggered, "the new period alerts on its own crossing")
}

// TestResetStaleWindows_MonthlyRollIsOneStatement pins the invariant the latch
// scope rests on: after row creation, the only writer of monthly_period_start
// is resetStaleWindows, and it nulls monthly_alert_sent_at in the *same*
// UPDATE. Splitting those into two statements still makes
// TestReserveImageGeneration_AlertsOncePerPeriod pass (both effects still
// occur), but a partial failure or a future author inserting work between them
// would leave the latch set against a new period — silently, the same failure
// class the period-scoped refund release was written to prevent. Capture the
// SQL the same way TestReserveImageGeneration_RefusalIsTheUpdatePredicate does.
func TestResetStaleWindows_MonthlyRollIsOneStatement(t *testing.T) {
	setupAIImageUsageTestDB(t)
	b := testBusiness(1)

	// Threshold of 2 so the post-roll reserve (monthly_used = 1) does not
	// immediately re-claim the latch and mask a missing null.
	_, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	crossing, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	require.True(t, crossing.AlertTriggered, "precondition: latch is claimed")
	var pre AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&pre).Error)
	require.NotNil(t, pre.MonthlyAlertSentAt, "precondition: latch is set")

	// Stale monthly window forces resetStaleWindows to advance the period.
	staleStart := time.Now().UTC().AddDate(0, -2, 0)
	require.NoError(t, GetDB().Model(&AIImageUsage{}).
		Where("business_id = ?", b.ID).
		UpdateColumn("monthly_period_start", staleStart).Error)

	var mu sync.Mutex
	var monthlyRolls []string
	require.NoError(t, GetDB().Callback().Update().After("gorm:update").
		Register("test:capture_monthly_roll", func(tx *gorm.DB) {
			sql := tx.Statement.SQL.String()
			// The monthly-roll UPDATE is the one that *writes* a new period
			// start under a staleness predicate. Counter increments and the
			// latch claim do not match this shape.
			if strings.Contains(sql, "monthly_period_start") &&
				strings.Contains(sql, "monthly_period_start < ?") {
				mu.Lock()
				monthlyRolls = append(monthlyRolls, sql)
				mu.Unlock()
			}
		}))

	rolled, err := ReserveImageGeneration(b, 100, 2)
	require.NoError(t, err)
	assert.False(t, rolled.AlertTriggered,
		"the first generation of a fresh period sits below the threshold")

	require.Len(t, monthlyRolls, 1,
		"a monthly roll must issue exactly one period-advancing UPDATE")
	// Pin the SET clause, not a bare column mention. A second-statement null
	// that still "mentions" monthly_alert_sent_at via a no-op identity
	// assignment (monthly_alert_sent_at = monthly_alert_sent_at) in the first
	// UPDATE would pass a Contains check while leaving a split-write window.
	// GORM binds nil as `monthly_alert_sent_at`=?; an Expr identity does not.
	rollSQL := monthlyRolls[0]
	upper := strings.ToUpper(rollSQL)
	setIdx := strings.Index(upper, " SET ")
	whereIdx := strings.Index(upper, " WHERE ")
	require.Greater(t, whereIdx, setIdx, "roll SQL must have SET … WHERE: %s", rollSQL)
	setClause := rollSQL[setIdx:whereIdx]
	assert.Contains(t, setClause, "monthly_period_start",
		"period advance must be in the SET clause of the roll UPDATE")
	assert.Contains(t, setClause, "`monthly_alert_sent_at`=?",
		"latch null must be a bound SET (nil → ?), not a column mention or identity Expr, in the same period-advancing UPDATE")

	var post AIImageUsage
	require.NoError(t, GetDB().Where("business_id = ?", b.ID).First(&post).Error)
	assert.Nil(t, post.MonthlyAlertSentAt, "the roll must still clear the latch")
	assert.Equal(t, 1, post.MonthlyUsed,
		"the roll zeroes monthly_used and this reserve is the only generation in the new period")
	assert.True(t, post.MonthlyPeriodStart.After(staleStart),
		"period must advance off the deliberately backdated value")
}

// ---------------------------------------------------------------------------
// Period calculation tests (no DB needed) — carried over from the deleted
// credit-ledger suite against the renamed helper.
// ---------------------------------------------------------------------------

func TestComputeImageUsagePeriodStart_AnchorMidMonth(t *testing.T) {
	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	got := computeImageUsagePeriodStart(now, 15)
	want := time.Date(2026, time.February, 15, 0, 0, 0, 0, time.UTC) // anchor 15 not reached in March yet
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestComputeImageUsagePeriodStart_Day31ClampsFeb(t *testing.T) {
	now := time.Date(2026, time.February, 20, 0, 0, 0, 0, time.UTC)
	got := computeImageUsagePeriodStart(now, 31)
	want := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// TestImageUsageAnchorTracksCreationDay pins the monthly window to the day the
// business was created: the row is created with a window starting on that
// anchor day and never in the future.
func TestImageUsageAnchorTracksCreationDay(t *testing.T) {
	setupAIImageUsageTestDB(t)

	const bizID = uint(50)
	b := &Business{ID: bizID, CreatedAt: time.Date(2025, time.June, 15, 9, 0, 0, 0, time.UTC)}

	row, err := EnsureImageUsageRow(b)
	require.NoError(t, err)
	require.Equal(t, 15, row.MonthlyAnchorDay, "anchor should track the creation day")
	require.Equal(t, 15, row.MonthlyPeriodStart.UTC().Day(),
		"the stored window must start on the anchor day computeImageUsagePeriodStart derived")
	require.False(t, row.MonthlyPeriodStart.After(time.Now().UTC()),
		"the current window cannot start in the future")
}
