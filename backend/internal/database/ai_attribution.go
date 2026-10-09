package database

import (
	"fmt"
	"math"
	"time"
)

// AiAttributedOrderValue is the bill-level AI attribution aggregate: the value
// of guest orders placed at a table while (or shortly after) an AI-waiter
// conversation that added items to the cart was active there.
//
// HONESTY CONTRACT: there is no per-item AI marker in the schema — this is
// correlation at the conversation→table→order level, not per-item causation.
// Any UI label must say "in guest orders during AI chats", never "AI sold $X".
//
// bill_items.subtotal is stored as float64 DOLLARS (see BillItem in models.go)
// — the one deliberate exception to the cents convention — so the SQL SUM is
// already dollars and is only rounded, never divided, at this boundary.
type AiAttributedOrderValue struct {
	RevenueDollars float64
	OrderCount     int64
}

// aiAttributionGrace pads the conversation window: guests browse with the AI,
// then submit the cart a few minutes after the last message.
const aiAttributionGrace = 30 * time.Minute

// GetAiAttributedOrderValue sums bill_items of non-cancelled guest orders
// created inside [start, end) that fall within an AI conversation window
// (conversation.created_at .. conversation.updated_at + 30min) on the same
// business + table, counting only conversations with cart_items_added > 0.
// Single aggregate statement; EXISTS keeps overlapping conversations from
// double-counting an order.
func GetAiAttributedOrderValue(businessID uint, start, end time.Time) (AiAttributedOrderValue, error) {
	windowEndExpr := "c.updated_at + INTERVAL '30 minutes'"
	if db.Dialector.Name() == "sqlite" {
		windowEndExpr = "datetime(c.updated_at, '+30 minutes')"
	}

	var row struct {
		RevenueDollars float64 `gorm:"column:revenue_dollars"`
		OrderCount     int64   `gorm:"column:order_count"`
	}
	err := db.Raw(fmt.Sprintf(`
		SELECT
			COALESCE(SUM(bi.subtotal), 0) AS revenue_dollars,
			COUNT(DISTINCT o.id) AS order_count
		FROM bill_items bi
		JOIN orders o ON o.id = bi.order_id
		WHERE o.business_id = ?
			AND o.created_by = 'guest'
			AND o.status <> 'cancelled'
			AND o.created_at >= ? AND o.created_at < ?
			AND EXISTS (
				SELECT 1
				FROM ai_waiter_conversations c
				JOIN tables t ON t.business_id = c.business_id AND t.table_code = c.table_code
				JOIN bills b ON b.id = o.bill_id AND b.table_id = t.id
				WHERE c.business_id = o.business_id
					AND c.cart_items_added > 0
					AND o.created_at >= c.created_at
					AND o.created_at <= %s
			)
	`, windowEndExpr), businessID, start, end).Scan(&row).Error
	if err != nil {
		return AiAttributedOrderValue{}, fmt.Errorf("ai attributed order value: %w", err)
	}

	// Keep aiAttributionGrace documented: SQL dialect literals must stay in sync.
	_ = aiAttributionGrace

	return AiAttributedOrderValue{
		RevenueDollars: math.Round(row.RevenueDollars*100) / 100,
		OrderCount:     row.OrderCount,
	}, nil
}

// AiConversationTimeSeries carries the aggregated insight time-series so the
// handler never materializes the raw created_at rows: a day-bucketed count for
// the 7-day trend, an hour-of-day count over the 30-day window, and the 30-day
// total. All keys/counts come from SQL GROUP BY aggregates — no per-row Go loop.
type AiConversationTimeSeries struct {
	// DayCounts maps "YYYY-MM-DD" (business-local calendar day) -> count.
	DayCounts map[string]int
	// HourCounts[h] = number of conversations created in business-local hour h
	// over the full 30-day window.
	HourCounts [24]int
	// Total30d is the count of conversations created in [windowStart, now).
	Total30d int64
}

// GetAiConversationTimeSeries computes the insight time-series with SQL GROUP BY
// aggregates instead of plucking every created_at into Go (the old path
// materialized up to 10k timestamps per request). windowStart bounds the 30-day
// window; sevenDayStart bounds the daily trend. Day/hour buckets are in loc
// (business TZ). A nil loc falls back to UTC. Dialect-dispatched date/hour
// extraction keeps it portable across Postgres (prod) and SQLite (tests).
//
// Decision-14 note: a single GROUP BY (day, hour) over the 30-day window was
// measured and REVERTED on SQLite microbench — it produced more result rows
// (day×hour) and raised B/op + allocs/op vs two narrow aggregates. Keep two
// queries; reclaim comes from LocalizedTimestampExpr caching instead.
func GetAiConversationTimeSeries(businessID uint, windowStart, sevenDayStart time.Time, loc *time.Location) (AiConversationTimeSeries, error) {
	out := AiConversationTimeSeries{DayCounts: map[string]int{}}
	if loc == nil {
		loc = time.UTC
	}

	dialect := db.Dialector.Name()
	hourExpr := aiConversationHourExpr(dialect, loc)
	dayExpr := aiConversationDayExpr(dialect, loc)

	var hourRows []struct {
		Hour  int   `gorm:"column:hour"`
		Count int64 `gorm:"column:cnt"`
	}
	if err := db.Model(&AiWaiterConversation{}).
		Select(hourExpr+" AS hour, COUNT(*) AS cnt").
		Where("business_id = ? AND table_code <> ? AND created_at >= ?", businessID, AiWaiterOperatorTestTableCode, windowStart).
		Group(hourExpr).
		Scan(&hourRows).Error; err != nil {
		return out, fmt.Errorf("ai conversation hour buckets: %w", err)
	}
	for _, r := range hourRows {
		if r.Hour >= 0 && r.Hour < 24 {
			out.HourCounts[r.Hour] += int(r.Count)
		}
		out.Total30d += r.Count
	}

	var dayRows []struct {
		Day   string `gorm:"column:day"`
		Count int64  `gorm:"column:cnt"`
	}
	if err := db.Model(&AiWaiterConversation{}).
		Select(dayExpr+" AS day, COUNT(*) AS cnt").
		Where("business_id = ? AND table_code <> ? AND created_at >= ?", businessID, AiWaiterOperatorTestTableCode, sevenDayStart).
		Group(dayExpr).
		Scan(&dayRows).Error; err != nil {
		return out, fmt.Errorf("ai conversation day buckets: %w", err)
	}
	for _, r := range dayRows {
		out.DayCounts[r.Day] = int(r.Count)
	}

	return out, nil
}

// aiConversationLocalizedTimestamp wraps created_at so day/hour extraction
// buckets in the business calendar. Delegates to LocalizedTimestampExpr so
// LoadLocation + SQL-fragment work is shared/cached with analytics (decision-14).
func aiConversationLocalizedTimestamp(dialect string, loc *time.Location) string {
	return LocalizedTimestampExpr(dialect, "created_at", loc)
}

func aiConversationHourExpr(dialect string, loc *time.Location) string {
	col := aiConversationLocalizedTimestamp(dialect, loc)
	if dialect == "sqlite" {
		return fmt.Sprintf("CAST(strftime('%%H', %s) AS INTEGER)", col)
	}
	return fmt.Sprintf("CAST(EXTRACT(HOUR FROM %s) AS INTEGER)", col)
}

func aiConversationDayExpr(dialect string, loc *time.Location) string {
	col := aiConversationLocalizedTimestamp(dialect, loc)
	if dialect == "sqlite" {
		return fmt.Sprintf("strftime('%%Y-%%m-%%d', %s)", col)
	}
	return fmt.Sprintf("TO_CHAR(%s, 'YYYY-MM-DD')", col)
}
