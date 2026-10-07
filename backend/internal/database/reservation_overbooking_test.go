package database

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestCreateReservationTx_RejectsConflictUnderLock guards against the
// check-then-insert double-booking race. The initial availability check runs
// outside the transaction, so two concurrent bookings for the same table+slot
// can both pass it; CreateReservationTx must re-validate the assigned table
// under the row lock and reject a slot that is already taken.
func TestCreateReservationTx_RejectsConflictUnderLock(t *testing.T) {
	setupReservationPerfTestDB(t, nil)
	biz := helperReservationPerfBusiness(t)

	table := &Table{BusinessID: biz.ID, Name: "T1", TableCode: "t1-code", Capacity: 4}
	require.NoError(t, db.Create(table).Error)

	slot := time.Date(2030, 1, 2, 19, 0, 0, 0, time.UTC)

	// Existing confirmed reservation occupies the table for [19:00, 20:00).
	existing := &TableReservation{
		BusinessID: biz.ID, TableID: &table.ID, CustomerName: "First", PartySize: 2,
		ReservationTime: slot, Duration: 60, Status: "confirmed", Source: "staff",
		ConfirmationCode: "EXIST-1",
	}
	require.NoError(t, db.Create(existing).Error)

	// A second reservation for the SAME table overlapping the slot must be
	// rejected by the under-lock re-check, even though it skipped the initial
	// availability check (simulating the concurrent winner having just committed).
	conflict := &TableReservation{
		BusinessID: biz.ID, TableID: &table.ID, CustomerName: "Second", PartySize: 2,
		ReservationTime: slot.Add(30 * time.Minute), Duration: 60, Status: "confirmed", Source: "staff",
		ConfirmationCode: "CONFLICT-1",
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		return CreateReservationTx(tx, conflict, 0)
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrReservationSlotUnavailable), "expected ErrReservationSlotUnavailable, got %v", err)

	// The conflicting reservation must not have been inserted.
	var count int64
	require.NoError(t, db.Model(&TableReservation{}).Where("confirmation_code = ?", "CONFLICT-1").Count(&count).Error)
	require.Zero(t, count, "conflicting reservation must not be persisted")

	// A non-overlapping slot on the same table still succeeds.
	ok := &TableReservation{
		BusinessID: biz.ID, TableID: &table.ID, CustomerName: "Third", PartySize: 2,
		ReservationTime: slot.Add(3 * time.Hour), Duration: 60, Status: "confirmed", Source: "staff",
		ConfirmationCode: "OK-1",
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateReservationTx(tx, ok, 0)
	}))
}

// TestUpdateReservationTx_RejectsConflictUnderLock guards the same double-booking
// race on the update path: reassigning a reservation onto a table+slot already
// taken by another reservation must be rejected (while excluding the reservation
// itself, so saving unchanged or non-table edits still succeed).
func TestUpdateReservationTx_RejectsConflictUnderLock(t *testing.T) {
	setupReservationPerfTestDB(t, nil)
	biz := helperReservationPerfBusiness(t)

	table := &Table{BusinessID: biz.ID, Name: "T1", TableCode: "t1-code", Capacity: 4}
	require.NoError(t, db.Create(table).Error)

	slot := time.Date(2030, 3, 4, 18, 0, 0, 0, time.UTC)

	// Reservation A holds [18:00, 19:00) on the table.
	resA := &TableReservation{
		BusinessID: biz.ID, TableID: &table.ID, CustomerName: "A", PartySize: 2,
		ReservationTime: slot, Duration: 60, Status: "confirmed", Source: "staff", ConfirmationCode: "A-1",
	}
	require.NoError(t, db.Create(resA).Error)

	// Reservation B is at a later, non-conflicting slot.
	resB := &TableReservation{
		BusinessID: biz.ID, TableID: &table.ID, CustomerName: "B", PartySize: 2,
		ReservationTime: slot.Add(4 * time.Hour), Duration: 60, Status: "confirmed", Source: "staff", ConfirmationCode: "B-1",
	}
	require.NoError(t, db.Create(resB).Error)

	// Reassigning B onto A's slot must be rejected.
	resB.ReservationTime = slot.Add(15 * time.Minute)
	err := db.Transaction(func(tx *gorm.DB) error {
		return UpdateReservationTx(tx, resB, 0)
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrReservationSlotUnavailable), "expected ErrReservationSlotUnavailable, got %v", err)

	// Editing A in place (same slot, excludes itself) must still succeed.
	resA.CustomerName = "A renamed"
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return UpdateReservationTx(tx, resA, 0)
	}))
}

// TestCreateReservationTx_ConcurrentLastSlotOnlyOneWins is the multi-goroutine
// race for the last open table+slot. Both callers pass the outside-tx
// availability check conceptually; under-lock re-validation must allow exactly
// one insert. Uses SQLite test DB (serializable-enough for this lock path).
func TestCreateReservationTx_ConcurrentLastSlotOnlyOneWins(t *testing.T) {
	setupReservationPerfTestDB(t, nil)
	biz := helperReservationPerfBusiness(t)

	table := &Table{BusinessID: biz.ID, Name: "Last Slot", TableCode: "last-slot", Capacity: 4}
	require.NoError(t, db.Create(table).Error)

	slot := time.Date(2031, 6, 15, 20, 0, 0, 0, time.UTC)
	const n = 8
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res := &TableReservation{
				BusinessID:       biz.ID,
				TableID:          &table.ID,
				CustomerName:     fmt.Sprintf("Racer-%d", i),
				PartySize:        2,
				ReservationTime:  slot,
				Duration:         60,
				Status:           "confirmed",
				Source:           "guest",
				ConfirmationCode: fmt.Sprintf("RACE-%d-%d", time.Now().UnixNano(), i),
			}
			errs <- db.Transaction(func(tx *gorm.DB) error {
				return CreateReservationTx(tx, res, 0)
			})
		}(i)
	}
	wg.Wait()
	close(errs)

	var wins, losses int
	for err := range errs {
		if err == nil {
			wins++
			continue
		}
		require.Truef(t, errors.Is(err, ErrReservationSlotUnavailable),
			"unexpected error from concurrent create: %v", err)
		losses++
	}
	require.Equal(t, 1, wins, "exactly one concurrent booker must win the last slot")
	require.Equal(t, n-1, losses, "all other concurrent bookers must be rejected")

	var count int64
	require.NoError(t, db.Model(&TableReservation{}).
		Where("business_id = ? AND table_id = ? AND reservation_time = ?", biz.ID, table.ID, slot).
		Count(&count).Error)
	require.Equal(t, int64(1), count, "exactly one reservation row for the contested slot")
}
