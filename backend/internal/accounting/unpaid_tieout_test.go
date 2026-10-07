package accounting

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// RemainingDueAsOf is leftover remainings created before end (no lower bound).
// It is an independent predicate the unpaid tie-out compares against
// collection_gap; production unpaid-bills uses RemainingDueInRange and
// RemainingDueCarriedOver.
func RemainingDueAsOf(q *gorm.DB, businessID uint, end time.Time, prefix string) *gorm.DB {
	p := colPrefix(prefix)
	return RemainingDueOnTable(q, businessID, prefix).
		Where(p+"created_at < ?", end)
}

// TestCollectionGap_MatchesOutstandingSum seeds bills and asserts
// GetSummary.collection_gap equals sum of (total-paid) for non-void outstanding
// bills in the same window — the Wave 6 KPI tie-out.
func TestCollectionGap_MatchesOutstandingSum(t *testing.T) {
	setupAccountingTestDB(t)
	biz := createAccountingBusiness(t, "0xOutOwner", "outstanding-tie")
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)

	// Bill 1: closed, total 10000 cents, paid 3000 → outstanding 70.00
	b1 := createAccountingBill(t, biz.ID, "OUT-1", 100, start.Add(time.Hour))
	require.NoError(t, database.GetDB().Model(b1).Updates(map[string]interface{}{
		"paid_amount": 3000,
		"status":      database.BillStatusClosed,
	}).Error)

	// Bill 2: fully paid → not outstanding
	b2 := createAccountingBill(t, biz.ID, "OUT-2", 50, start.Add(2*time.Hour))
	require.NoError(t, database.GetDB().Model(b2).Updates(map[string]interface{}{
		"paid_amount": 5000,
		"status":      database.BillStatusPaid,
	}).Error)

	// Bill 3: closed unpaid full 25.00
	b3 := createAccountingBill(t, biz.ID, "OUT-3", 25, start.Add(3*time.Hour))
	require.NoError(t, database.GetDB().Model(b3).Updates(map[string]interface{}{
		"status": database.BillStatusClosed,
	}).Error)

	// Bill 4: still-open live table — remaining is part of collection_gap (#651).
	_ = createAccountingBill(t, biz.ID, "OUT-OPEN", 200, start.Add(4*time.Hour))

	svc := NewService(database.GetDBWrapper())
	summary, err := svc.GetSummary(biz.ID, start, end)
	require.NoError(t, err)

	type sumRow struct{ Cents int64 }
	var sum sumRow
	require.NoError(t, database.GetDB().Table("bills").
		Select("COALESCE(SUM(total_amount - paid_amount), 0) AS cents").
		Where("business_id = ? AND status != ? AND created_at >= ? AND created_at < ? AND total_amount > paid_amount",
			biz.ID, database.BillStatusVoided, start, end).
		Scan(&sum).Error)

	require.InDelta(t, float64(sum.Cents)/100.0, summary.CollectionGap, 0.01,
		"collection_gap must equal remaining due on non-void bills in the window")
	// 70 + 25 + 200 open = 295
	require.InDelta(t, 295.0, summary.CollectionGap, 0.01)
}

func placeBillAt(t *testing.T, billID uint, at time.Time) {
	t.Helper()
	require.NoError(t, database.GetDB().Model(&database.Bill{}).
		Where("id = ?", billID).
		UpdateColumn("created_at", at).Error)
}

// TestCollectionGap_RangeScoped_OutstandingAsOfEnd is the #222 scope-split
// gate: Last 30 days collection_gap is leftover remainings created in
// [start, end). Outstanding as-of end still includes older leftover
// remainings. A bill after end belongs on neither surface.
func TestCollectionGap_RangeScoped_OutstandingAsOfEnd(t *testing.T) {
	setupAccountingTestDB(t)
	biz := createAccountingBusiness(t, "0xScopeOwner", "scope-split")

	// Inclusive Last 30 days: 2026-07-16 through 2026-08-14.
	start := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC) // exclusive

	beforeStart := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	inRange := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	afterEnd := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	// Jul 8 closed unpaid — production #222 demo shape ($36.68).
	oldDebt := createAccountingBill(t, biz.ID, "JUL-8", 36.68, beforeStart)
	require.NoError(t, database.GetDB().Model(oldDebt).Updates(map[string]interface{}{
		"status": database.BillStatusClosed,
	}).Error)
	placeBillAt(t, oldDebt.ID, beforeStart)

	inWindow := createAccountingBill(t, biz.ID, "JUL-20", 10, inRange)
	require.NoError(t, database.GetDB().Model(inWindow).Updates(map[string]interface{}{
		"status": database.BillStatusClosed,
	}).Error)
	placeBillAt(t, inWindow.ID, inRange)

	after := createAccountingBill(t, biz.ID, "AUG-20", 50, afterEnd)
	require.NoError(t, database.GetDB().Model(after).Updates(map[string]interface{}{
		"status": database.BillStatusClosed,
	}).Error)
	placeBillAt(t, after.ID, afterEnd)

	openTab := createAccountingBill(t, biz.ID, "OPEN", 80, inRange)
	placeBillAt(t, openTab.ID, inRange)

	svc := NewService(database.GetDBWrapper())
	summary, err := svc.GetSummary(biz.ID, start, end)
	require.NoError(t, err)

	// Period collection gap: Jul 20 closed ($10) + in-window open tab ($80).
	// Jul 8 is older debt and stays out of the range-scoped KPI.
	require.InDelta(t, 90.0, summary.CollectionGap, 0.01,
		"collection_gap is [start, end) remaining due including open/partial")

	type sumRow struct{ Cents int64 }
	var asOf sumRow
	require.NoError(t, RemainingDueAsOf(database.GetDB().Table("bills"), biz.ID, end, "").
		Select("COALESCE(SUM(total_amount - paid_amount), 0) AS cents").
		Scan(&asOf).Error)

	// Outstanding as-of end: Jul 8 ($36.68) + Jul 20 ($10) + open tab ($80).
	// Not after-end. Older leftover remaining is why this differs from the KPI.
	require.InDelta(t, 126.68, float64(asOf.Cents)/100.0, 0.01,
		"outstanding as-of end must include the pre-range Jul 8 leftover remaining")
	require.NotEqual(t, summary.CollectionGap, float64(asOf.Cents)/100.0,
		"collection_gap and outstanding as-of must stay different when older debt exists")
}
