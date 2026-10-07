package analytics

import (
	"fmt"
	"time"
)

// GetDailySeries returns one zero-filled bucket per day in [from, to],
// aggregating recognized payments in a single GROUP BY query. Mirrors the
// recognized-payments CTE pattern used by GetPaymentWindowSummary.
//
// Window widening: recognizedPaymentBillAmountsSubquerySQL uses half-open
// [from, to) comparisons. To ensure that every event whose stored timestamp
// falls on any day within [from_day, to_day] is included, the query's
// effective window is widened to [start-of-from-day, start-of-to-day+24h)
// before being passed to the subquery builder.  enumerateDays still receives
// the original from/to so the bucket labels are not affected.
func (s *AnalyticsService) GetDailySeries(businessID uint, from, to time.Time) (*DailySeries, error) {
	// Widen the time window to cover whole days so the half-open [from,to)
	// window includes every event on every day in the series range.
	queryFrom := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	toDay := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location())
	queryTo := toDay.AddDate(0, 0, 1) // next local midnight (DST-safe), matches enumerateDays

	recognizedSQL, args := s.recognizedPaymentBillAmountsSubquerySQL(businessID, queryFrom, queryTo)
	recognizedCTE := s.recognizedEventsCTE(recognizedSQL)
	// Bucket each day label in the window's location (business TZ) so a
	// late-local sale isn't filed under the UTC calendar day.
	dayExpr := dayExpressionForDialect(s.db.GetGorm().Name(), "recognized_events.recognized_at", from.Location())

	var rows []struct {
		Day              string `gorm:"column:day"`
		RevenueCents     int64  `gorm:"column:revenue_cents"`
		TipCents         int64  `gorm:"column:tip_cents"`
		TransactionCount int    `gorm:"column:transaction_count"`
		BillCount        int    `gorm:"column:bill_count"`
	}
	if err := s.db.GetGorm().Raw(fmt.Sprintf(`
		WITH %s
		SELECT
			%s AS day,
			COALESCE(SUM(recognized_events.amount_cents), 0) AS revenue_cents,
			COALESCE(SUM(recognized_events.tip_cents), 0) AS tip_cents,
			COUNT(*) AS transaction_count,
			COUNT(DISTINCT recognized_events.bill_id) AS bill_count
		FROM recognized_events
		GROUP BY day
		ORDER BY day
	`, recognizedCTE, dayExpr), args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to aggregate daily series: %w", err)
	}

	byDay := make(map[string]DailyBucket, len(rows))
	for _, r := range rows {
		revenue := centsToDollars(r.RevenueCents)
		bucket := DailyBucket{
			Date:         r.Day,
			Revenue:      revenue,
			Tips:         centsToDollars(r.TipCents),
			Bills:        r.BillCount,
			Transactions: r.TransactionCount,
		}
		if r.BillCount > 0 {
			bucket.AverageTicket = revenue / float64(r.BillCount)
		}
		byDay[r.Day] = bucket
	}

	days := enumerateDays(from, to)
	buckets := make([]DailyBucket, 0, len(days))
	for _, day := range days {
		if b, ok := byDay[day]; ok {
			buckets = append(buckets, b)
		} else {
			buckets = append(buckets, DailyBucket{Date: day})
		}
	}

	return &DailySeries{
		Buckets: buckets,
		Range:   DateRange{From: days[0], To: days[len(days)-1]},
	}, nil
}

// dayExpressionForDialect returns a SQL expression that renders the date
// portion ("YYYY-MM-DD") of a timestamp column in the business's local calendar
// (loc), parallel to hourExpressionForDialect. loc==UTC (or nil) buckets on the
// stored UTC value, preserving the original SQL.
func dayExpressionForDialect(dialectName, column string, loc *time.Location) string {
	col := localizedTimestampExpr(dialectName, column, loc)
	if dialectName == "sqlite" {
		return fmt.Sprintf("strftime('%%Y-%%m-%%d', %s)", col)
	}
	return fmt.Sprintf("to_char(%s, 'YYYY-MM-DD')", col)
}

// DailyBucket is one day's aggregated analytics, money in dollars.
type DailyBucket struct {
	Date          string  `json:"date"`
	Revenue       float64 `json:"revenue"`
	Tips          float64 `json:"tips"`
	Bills         int     `json:"bills"`
	Transactions  int     `json:"transactions"`
	AverageTicket float64 `json:"average_ticket"`
}

// DateRange is the inclusive window the series covers.
type DateRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// DailySeries is a continuous (zero-filled) per-day series.
type DailySeries struct {
	Buckets []DailyBucket `json:"buckets"`
	Range   DateRange     `json:"range"`
}

// dateLayout is the canonical day key/format used across the series.
const dateLayout = "2006-01-02"

// enumerateDays returns every day key from `from` to `to` inclusive,
// stepping in `from`'s location so boundaries match parsePeriod semantics.
func enumerateDays(from, to time.Time) []string {
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, to.Location())
	var days []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		days = append(days, d.Format(dateLayout))
	}
	return days
}
