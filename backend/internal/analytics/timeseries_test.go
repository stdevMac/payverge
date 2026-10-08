package analytics

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDayExpressionForDialect(t *testing.T) {
	// UTC (or nil) preserves the original byte-identical expressions.
	require.Equal(t, "strftime('%Y-%m-%d', recognized_events.recognized_at)",
		dayExpressionForDialect("sqlite", "recognized_events.recognized_at", time.UTC))
	require.Equal(t, "to_char(recognized_events.recognized_at, 'YYYY-MM-DD')",
		dayExpressionForDialect("postgres", "recognized_events.recognized_at", time.UTC))

	// A business timezone shifts the column into local wall-clock before
	// extracting the date: Postgres via AT TIME ZONE (DST-correct), SQLite via a
	// fixed-offset datetime() (test path; Dubai is UTC+4 = 14400s).
	dubai, err := time.LoadLocation("Asia/Dubai")
	require.NoError(t, err)
	require.Equal(t, "to_char((recognized_events.recognized_at)::timestamptz AT TIME ZONE 'Asia/Dubai', 'YYYY-MM-DD')",
		dayExpressionForDialect("postgres", "recognized_events.recognized_at", dubai))
	require.Equal(t, "strftime('%Y-%m-%d', datetime(recognized_events.recognized_at, '+14400 seconds'))",
		dayExpressionForDialect("sqlite", "recognized_events.recognized_at", dubai))
}

// TestGetDailySeriesBucketsDaysInBusinessTimezone locks AN-MON-2 (Layer B, SQL
// day bucketing): a Dubai (UTC+4) sale at 22:00 UTC is 02:00 the next day in
// Dubai, so the daily series must place it in the Dubai calendar day, not the
// UTC one.
func TestGetDailySeriesBucketsDaysInBusinessTimezone(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	dubai, err := time.LoadLocation("Asia/Dubai")
	require.NoError(t, err)
	service := NewAnalyticsService(db)
	business := database.Business{Name: "Dubai Series Bistro"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	// 2026-05-11 22:00 UTC == 2026-05-12 02:00 Dubai → Dubai day 2026-05-12.
	mkPaidBill(t, db, business.ID, "TZ-D", 2400, 0, time.Date(2026, 5, 11, 22, 0, 0, 0, time.UTC))

	from := time.Date(2026, 5, 10, 0, 0, 0, 0, dubai)
	to := time.Date(2026, 5, 12, 0, 0, 0, 0, dubai)
	series, err := service.GetDailySeries(business.ID, from, to)
	require.NoError(t, err)

	byDay := map[string]float64{}
	for _, b := range series.Buckets {
		byDay[b.Date] = b.Revenue
	}
	assert.InDelta(t, 24.0, byDay["2026-05-12"], 0.01, "sale must bucket into Dubai day 2026-05-12")
	assert.Zero(t, byDay["2026-05-11"], "sale must not bucket into UTC day 2026-05-11")
}

func TestGetDailySeriesAggregatesPerDayInSingleQuery(t *testing.T) {
	recorder := &recordingAnalyticsLogger{}
	db := setupAnalyticsTestDBWithLogger(t, recorder)
	service := NewAnalyticsService(db)
	business := database.Business{Name: "Series Bistro"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	day1 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)

	// Day 1: one paid bill, $30 revenue + $5 tip.
	mkPaidBill(t, db, business.ID, "S-1", 3000, 500, day1)
	// Day 3: one paid bill, $20 revenue + $0 tip. Day 2 deliberately empty.
	mkPaidBill(t, db, business.ID, "S-2", 2000, 0, day3)

	recorder.Reset()
	series, err := service.GetDailySeries(business.ID, day1, day3)
	require.NoError(t, err)

	// Zero-filled: day 1, 2, 3 all present and in order.
	require.Len(t, series.Buckets, 3)
	assert.Equal(t, "2026-05-10", series.Buckets[0].Date)
	assert.Equal(t, "2026-05-11", series.Buckets[1].Date)
	assert.Equal(t, "2026-05-12", series.Buckets[2].Date)

	assert.InDelta(t, 30.0, series.Buckets[0].Revenue, 0.01)
	assert.InDelta(t, 5.0, series.Buckets[0].Tips, 0.01)
	assert.Equal(t, 1, series.Buckets[0].Bills)
	assert.InDelta(t, 30.0, series.Buckets[0].AverageTicket, 0.01)

	assert.InDelta(t, 0.0, series.Buckets[1].Revenue, 0.01) // empty middle day
	assert.Equal(t, 0, series.Buckets[1].Bills)

	assert.InDelta(t, 20.0, series.Buckets[2].Revenue, 0.01)
	assert.Equal(t, "2026-05-10", series.Range.From)
	assert.Equal(t, "2026-05-12", series.Range.To)

	// Access shape: a single recognized-payment GROUP BY query, no N+1.
	sqls := recorder.SQLs()
	require.Len(t, sqls, 1, "daily series must use one recognized-payment query")
	assert.Contains(t, sqls[0], "WITH recognized_events AS")
	assert.Contains(t, sqls[0], "GROUP BY")
}

func TestGetDailySeriesBucketsByStoredDayBoundary(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	service := NewAnalyticsService(db)
	business := database.Business{Name: "Boundary Bistro"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	// 23:30 UTC on day 1 and 00:30 UTC on day 2 must land in different buckets.
	late := time.Date(2026, 5, 10, 23, 30, 0, 0, time.UTC)
	early := time.Date(2026, 5, 11, 0, 30, 0, 0, time.UTC)
	mkPaidBill(t, db, business.ID, "B-1", 1000, 0, late)
	mkPaidBill(t, db, business.ID, "B-2", 2000, 0, early)

	series, err := service.GetDailySeries(business.ID, late, early)
	require.NoError(t, err)
	require.Len(t, series.Buckets, 2)
	assert.InDelta(t, 10.0, series.Buckets[0].Revenue, 0.01) // 2026-05-10
	assert.InDelta(t, 20.0, series.Buckets[1].Revenue, 0.01) // 2026-05-11
}

// mkPaidBill seeds a confirmed crypto payment on a paid bill, so that the
// recognized-payments CTE picks it up via the confirmed_at branch.
func mkPaidBill(t *testing.T, db *database.DB, businessID uint, billNo string, amountCents, tipCents int64, at time.Time) {
	t.Helper()
	bill := database.Bill{
		BusinessID: businessID, BillNumber: billNo,
		TotalAmount: amountCents, PaidAmount: amountCents,
		Status: database.BillStatusPaid, ClosedAt: &at,
		CreatedAt: at.Add(-time.Hour), UpdatedAt: at,
	}
	require.NoError(t, db.GetGorm().Create(&bill).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: bill.ID, PayerAddr: "0x" + billNo,
		Amount: amountCents, TipAmount: tipCents,
		TxHash: "series_" + billNo, Status: database.PaymentStatusConfirmed,
		PaymentMethod: "crypto", ConfirmedAt: &at, CreatedAt: at, UpdatedAt: at,
	}).Error)
}
