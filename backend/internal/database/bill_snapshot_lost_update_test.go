package database

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func appendItemMutation(business *Business, item BillItem) BillItemsMutation {
	return func(bill *Bill, items []BillItem) ([]BillItem, []BillHistoryEvent, error) {
		next := append(items, item)
		ApplyBillTotals(bill, next, business)
		return next, nil, nil
	}
}

func billItemNames(t *testing.T, billID uint) map[string]bool {
	t.Helper()
	var bill Bill
	require.NoError(t, db.First(&bill, billID).Error)
	items, err := billItemsForBillSnapshot(bill.ID, bill.Items)
	require.NoError(t, err)
	names := map[string]bool{}
	for _, item := range items {
		names[item.Name] = true
	}
	return names
}

// BILL-SNAPSHOT-LOST-UPDATE: two writers that each loaded the bill before the
// other committed must both land. The mutation runs against the locked row's
// current items, not the caller's snapshot.
func TestUpdateBillWithHistoryFn_ConcurrentAddsBothPersist(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			item := BillItem{ID: fmt.Sprintf("item_concurrent_%d", i), Name: fmt.Sprintf("Dish %d", i), Price: 5, Quantity: 1}
			NormalizeBillItemLineMoney(&item)
			_, _, errs[i] = UpdateBillWithHistoryFn(bill.ID, appendItemMutation(biz, item))
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	names := billItemNames(t, bill.ID)
	require.Len(t, names, 4)
	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.EqualValues(t, 2000, reloaded.Subtotal, "totals reflect every concurrent line")
}

func TestUpdateBillWithHistoryFn_InterleavedWritersKeepEachOthersItems(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	// Writer A loads first (the old handler shape), then B commits.
	staleA := &Bill{}
	require.NoError(t, db.First(staleA, bill.ID).Error)
	_, _, err := UpdateBillWithHistoryFn(bill.ID, appendItemMutation(biz, BillItem{ID: "item_b", Name: "B", Price: 3, Quantity: 1, Subtotal: 3}))
	require.NoError(t, err)

	// Legacy whole-snapshot write from A's stale view is refused, not applied.
	err = UpdateBillWithHistory(staleA, []BillItem{{ID: "item_a", Name: "A", Price: 4, Quantity: 1, Subtotal: 4}}, nil)
	require.ErrorIs(t, err, ErrBillStale)
	require.Equal(t, map[string]bool{"B": true}, billItemNames(t, bill.ID))

	// The locked-mutation path applies A on top of B.
	_, items, err := UpdateBillWithHistoryFn(bill.ID, appendItemMutation(biz, BillItem{ID: "item_a", Name: "A", Price: 4, Quantity: 1, Subtotal: 4}))
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, map[string]bool{"A": true, "B": true}, billItemNames(t, bill.ID))
}

func TestUpdateBillWithHistoryFn_StatusGate(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	partial := helperBill(t, biz, nil, 10)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", partial.ID).
		Updates(map[string]interface{}{"status": BillStatusPartial, "paid_amount": 400}).Error)
	updated, _, err := UpdateBillWithHistoryFn(partial.ID, appendItemMutation(biz, BillItem{ID: "item_p", Name: "P", Price: 2, Quantity: 1, Subtotal: 2}))
	require.NoError(t, err, "partial bills stay editable")
	require.EqualValues(t, 400, updated.PaidAmount, "payment columns are untouched")

	paid := helperBill(t, biz, nil, 10)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", paid.ID).
		Updates(map[string]interface{}{"status": BillStatusPaid, "paid_amount": 1000}).Error)
	_, _, err = UpdateBillWithHistoryFn(paid.ID, appendItemMutation(biz, BillItem{ID: "item_x", Name: "X", Price: 2, Quantity: 1, Subtotal: 2}))
	require.ErrorIs(t, err, ErrBillNotPayable)
}
