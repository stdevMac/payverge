package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// #798: leftover 762 sat abandoned with paid_amount 0 and a settled_at stamp,
// and payment_ledger buckets a bill by settled_at first — so a check nobody
// paid dated itself into the ledger as a settlement. The sweeper settles no
// money, so it must never write settled_at, and abandoning is terminal: the
// hourly pass must not re-abandon a bill it already swept.
func TestBillLifecycle_AbandonNeverStampsSettlementAndRunsOnce(t *testing.T) {
	g := newBillLifecycleDB(t)
	const businessID = uint(1798)
	stale := time.Now().Add(-30 * time.Hour)
	bill := seedOpenBill(t, g, businessID, "SETTLED-798", stale, 11)

	s := NewBillLifecycleSweeper(g, BillLifecycleConfig{
		DefaultThreshold: 24 * time.Hour,
		BatchSize:        50,
		Notify:           func(uint, database.Bill, time.Duration) {},
	})

	n, err := s.RunOnce(time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var got database.Bill
	require.NoError(t, g.First(&got, bill.ID).Error)
	require.Equal(t, database.BillStatusAbandoned, got.Status)
	require.Nil(t, got.SettledAt,
		"the sweeper moves no money — abandoning must never stamp settled_at")
	require.NotNil(t, got.ClosedAt,
		"abandoned is terminal; closed_at must be stamped alongside abandoned_at")
	require.NotNil(t, got.AbandonedAt)
	firstAbandonedAt := *got.AbandonedAt

	// A second sweep an hour later must be a no-op: no re-abandon, no second
	// history event, no settlement stamp appearing late.
	n, err = s.RunOnce(time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, 0, n, "an already-abandoned bill must not be swept again")

	require.NoError(t, g.First(&got, bill.ID).Error)
	require.Nil(t, got.SettledAt)
	require.Equal(t, firstAbandonedAt.UTC(), got.AbandonedAt.UTC(),
		"abandoned_at must not be rewritten by a later pass")

	var events int64
	require.NoError(t, g.Model(&database.BillHistoryEvent{}).
		Where("bill_id = ? AND event_type = ?", bill.ID, database.BillHistoryEventBillAbandoned).
		Count(&events).Error)
	require.Equal(t, int64(1), events,
		"bill.abandoned must be recorded once, not once per hourly pass")
}
