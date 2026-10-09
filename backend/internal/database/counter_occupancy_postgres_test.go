package database

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func insertCounterOccupancyBill(t *testing.T, gdb *gorm.DB, businessID, counterID uint, status BillStatus, number string) uint {
	t.Helper()
	var id uint
	require.NoError(t, gdb.Raw(`
		INSERT INTO bills (business_id, counter_id, bill_number, public_token, status, settlement_addr, tipping_addr)
		VALUES (?, ?, ?, ?, ?, '', '')
		RETURNING id
	`, businessID, counterID, number, "token-"+number, status).Scan(&id).Error)
	return id
}

func TestCounterOccupancyConcurrencyAndLifecycle_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short")
	}
	gdb := startGenesisPostgres(t).DB
	require.Equal(t, uint(1), seedGenesisBusiness(t, gdb, "counter-first"))
	require.Equal(t, uint(2), seedGenesisBusiness(t, gdb, "counter-second"))
	require.NoError(t, gdb.Exec(`
		INSERT INTO counters (id, business_id, counter_number, name, is_active)
		VALUES (10, 1, 1, 'C1', TRUE), (20, 2, 1, 'Other C1', TRUE), (30, 1, 2, 'Inactive', FALSE)
	`).Error)

	// The schema itself refuses two active bills on one counter.
	occupying := insertCounterOccupancyBill(t, gdb, 1, 10, BillStatusOpen, "occupying")
	secondActive := gdb.Exec(`
		INSERT INTO bills (business_id, counter_id, bill_number, public_token, status, settlement_addr, tipping_addr)
		VALUES (1, 10, 'second-active', 'token-second-active', 'partial', '', '')
	`).Error
	require.ErrorContains(t, secondActive, "idx_bills_active_per_counter",
		"a second active bill on the same counter must violate the active-counter unique index")
	require.NoError(t, gdb.Exec(`UPDATE bills SET status = 'paid' WHERE id = ?`, occupying).Error)

	previousDB := db
	SetTestDB(gdb)
	t.Cleanup(func() { SetTestDB(previousDB) })

	start := make(chan struct{})
	errs := make([]error, 2)
	bills := []*Bill{
		{BusinessID: 1, CounterID: counterIDPtr(10), BillNumber: "concurrent-a", Status: BillStatusOpen},
		{BusinessID: 1, CounterID: counterIDPtr(10), BillNumber: "concurrent-b", Status: BillStatusOpen},
	}
	var wg sync.WaitGroup
	for i := range bills {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			errs[index] = CreateBill(bills[index], nil)
		}(i)
	}
	close(start)
	wg.Wait()

	var success, conflict int
	var activeBillID uint
	var conflictBillID uint
	for i, createErr := range errs {
		if createErr == nil {
			success++
			activeBillID = bills[i].ID
			continue
		}
		if errors.Is(createErr, ErrActiveBillExists) {
			conflict++
			var activeErr *ActiveBillConflictError
			require.ErrorAs(t, createErr, &activeErr)
			require.NotZero(t, activeErr.BillID)
			conflictBillID = activeErr.BillID
			continue
		}
		require.NoError(t, createErr)
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	require.Equal(t, activeBillID, conflictBillID)

	available, err := GetAvailableCounters(1)
	require.NoError(t, err)
	require.Empty(t, available, "an active bill occupies the counter even when the cache is stale")
	require.NoError(t, gdb.Exec(`UPDATE counters SET current_bill_id = NULL WHERE id = 10`).Error)
	available, err = GetAvailableCounters(1)
	require.NoError(t, err)
	require.Empty(t, available, "availability must be derived from active bills, not the cache pointer")

	// open -> partial remains occupied; paid/closed/voided release. A reversal
	// back to open reacquires the pointer, while a rolled-back release changes nothing.
	for _, status := range []BillStatus{BillStatusPartial, BillStatusPaid, BillStatusOpen, BillStatusClosed, BillStatusOpen, BillStatusVoided} {
		require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
			return updateBillLifecycleTx(tx, activeBillID, map[string]interface{}{"status": status})
		}))
		available, err = GetAvailableCounters(1)
		require.NoError(t, err)
		if billStatusIsActive(status) {
			require.Empty(t, available, "status %s must occupy the counter", status)
		} else {
			require.Len(t, available, 1, "status %s must release the counter", status)
		}
	}

	require.NoError(t, gdb.Exec(`UPDATE counters SET current_bill_id = ? WHERE id = 10`, activeBillID).Error)
	available, err = GetAvailableCounters(1)
	require.NoError(t, err)
	require.Len(t, available, 1, "a stale pointer to a terminal bill must not hide an available counter")

	require.EqualError(t, gdb.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, updateBillLifecycleTx(tx, activeBillID, map[string]interface{}{"status": BillStatusOpen}))
		return fmt.Errorf("force rollback")
	}), "force rollback")
	available, err = GetAvailableCounters(1)
	require.NoError(t, err)
	require.Len(t, available, 1, "rolled-back reversal must preserve the terminal status and release")

	// Validation belongs inside the locked creation transaction, not only in handlers.
	require.ErrorIs(t, CreateBill(&Bill{BusinessID: 1, CounterID: counterIDPtr(20), BillNumber: "cross-business", Status: BillStatusOpen}, nil), ErrCounterBusinessMismatch)
	require.ErrorIs(t, CreateBill(&Bill{BusinessID: 1, CounterID: counterIDPtr(30), BillNumber: "inactive", Status: BillStatusOpen}, nil), ErrCounterInactive)

}

func counterIDPtr(value uint) *uint { return &value }
