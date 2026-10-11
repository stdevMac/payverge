package accounting

import (
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

// RemainingDueOnTable is the leftover-remaining predicate shared by
// GetSummary.collection_gap and GET unpaid-bills: non-void bills with
// total_amount > paid_amount. Callers add the time window:
//
//	collection_gap: RemainingDueInRange — created_at in [start, end)
//	unpaid-bills:   RemainingDueInRange for the page, plus RemainingDueCarriedOver
//	                for leftover remainings created before start
//
// prefix is "" or a table alias such as "bills" (no trailing dot).
func RemainingDueOnTable(q *gorm.DB, businessID uint, prefix string) *gorm.DB {
	p := colPrefix(prefix)
	return q.Where(
		p+"business_id = ? AND "+p+"status <> ? AND "+p+"total_amount > "+p+"paid_amount",
		businessID, database.BillStatusVoided,
	)
}

// RemainingDueInRange is collection_gap: leftover remainings created in [start, end).
func RemainingDueInRange(q *gorm.DB, businessID uint, start, end time.Time, prefix string) *gorm.DB {
	p := colPrefix(prefix)
	return RemainingDueOnTable(q, businessID, prefix).
		Where(p+"created_at >= ? AND "+p+"created_at < ?", start, end)
}

// RemainingDueCarriedOver is the unpaid-bills companion aggregate (#799):
// leftover remainings created BEFORE start — pre-range debt that a
// range-scoped list would otherwise hide (#222 / L6-23). Callers surface its
// count + outstanding sum next to the in-range rows.
func RemainingDueCarriedOver(q *gorm.DB, businessID uint, start time.Time, prefix string) *gorm.DB {
	p := colPrefix(prefix)
	return RemainingDueOnTable(q, businessID, prefix).
		Where(p+"created_at < ?", start)
}

func colPrefix(prefix string) string {
	if prefix == "" {
		return ""
	}
	return prefix + "."
}
