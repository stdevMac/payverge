package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedFloorWithFutureBookings creates tables tables, each holding bookings
// future confirmed reservations spread over the coming weeks.
func seedFloorWithFutureBookings(tb testing.TB, tables, bookings int) (uint, []uint) {
	businessID := setupHostStandTableStatusDB(tb)
	now := time.Now().UTC()
	var ids []uint
	var rows []TableReservation
	for i := 0; i < tables; i++ {
		table := Table{BusinessID: businessID, Name: fmt.Sprintf("T%d", i), TableCode: fmt.Sprintf("FLOOR-%d-%d", i, now.UnixNano()), Capacity: 4, IsActive: true}
		require.NoError(tb, db.Create(&table).Error)
		ids = append(ids, table.ID)
		tid := table.ID
		// Insert the later bookings first so row order is not time order.
		for j := bookings - 1; j >= 0; j-- {
			rows = append(rows, TableReservation{
				BusinessID: businessID, TableID: &tid, CustomerName: "Guest", PartySize: 2,
				ReservationTime: now.Add(time.Duration(2+j*12) * time.Hour), Duration: 90,
				Status: "confirmed", ConfirmationCode: fmt.Sprintf("F%d-%d-%d", i, j, now.UnixNano()),
			})
		}
	}
	require.NoError(tb, db.CreateInBatches(rows, 200).Error)
	return businessID, ids
}

// TestGetTablesWithStatusReturnsOnlyNextReservationPerTable is the I3 guard:
// the floor poll must not ship every future booking per table. The host
// stand only renders the next one, so the read returns at most one
// reservation per table, and it is the soonest.
func TestGetTablesWithStatusReturnsOnlyNextReservationPerTable(t *testing.T) {
	businessID, _ := seedFloorWithFutureBookings(t, 3, 40)

	rows, err := GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	soonest := time.Now().UTC().Add(2 * time.Hour)
	for _, row := range rows {
		assert.Equal(t, "reserved", row["status"])
		assert.Equal(t, 1, row["reservations_count"])
		res := row["reservations"].([]TableReservation)
		require.Len(t, res, 1)
		assert.WithinDuration(t, soonest, res[0].ReservationTime, time.Minute, "must be the next booking")
	}
}

func BenchmarkGetTablesWithStatusFutureBookingsSQLite(b *testing.B) {
	businessID, _ := seedFloorWithFutureBookings(b, 20, 60)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetTablesWithStatus(businessID, false); err != nil {
			b.Fatal(err)
		}
	}
}
