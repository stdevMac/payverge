package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// seedTableReservationHistory gives one table history past bookings (one per
// day going back) plus one live booking that starts 3h before probe and
// lasts 4h, so it overlaps a probe at probe.
func seedTableReservationHistory(tb testing.TB, history int, probe time.Time) uint {
	businessID := setupHostStandTableStatusDB(tb)
	table := Table{BusinessID: businessID, Name: "Hist", TableCode: fmt.Sprintf("HIST-%d", time.Now().UnixNano()), Capacity: 4, IsActive: true}
	require.NoError(tb, db.Create(&table).Error)
	tid := table.ID
	rows := make([]TableReservation, 0, history+1)
	for i := 1; i <= history; i++ {
		rows = append(rows, TableReservation{
			BusinessID: businessID, TableID: &tid, CustomerName: strings.Repeat("n", 40), PartySize: 2,
			ReservationTime: probe.Add(-time.Duration(i) * 24 * time.Hour), Duration: 90, Status: "confirmed",
			ConfirmationCode: fmt.Sprintf("H%d-%d", i, time.Now().UnixNano()),
		})
	}
	rows = append(rows, TableReservation{
		BusinessID: businessID, TableID: &tid, CustomerName: "Live", PartySize: 2,
		ReservationTime: probe.Add(-3 * time.Hour), Duration: 240, Status: "confirmed",
		ConfirmationCode: fmt.Sprintf("LIVE-%d", time.Now().UnixNano()),
	})
	require.NoError(tb, db.CreateInBatches(rows, 200).Error)
	return tid
}

// TestCheckTableAvailabilityBoundsHistoryScan is the I4 guard: the conflict
// check that runs under the table row lock must bound reservation_time from
// below (no full-history scan) and read only the scheduling columns, while
// still catching a booking that started before the probe and overlaps it.
func TestCheckTableAvailabilityBoundsHistoryScan(t *testing.T) {
	probe := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	tid := seedTableReservationHistory(t, 30, probe)
	rec := &orderListSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	tx := db.Session(&gorm.Session{Logger: rec})

	ok, err := checkTableAvailabilityWithBufferTx(tx, tid, probe, 90, 0, 0)
	require.NoError(t, err)
	assert.False(t, ok, "a booking that started 3h earlier and runs 4h must still conflict")

	ok, err = checkTableAvailabilityWithBufferTx(tx, tid, probe.Add(2*time.Hour), 60, 0, 0)
	require.NoError(t, err)
	assert.True(t, ok, "the slot after the live booking ends is free")

	var stmt string
	for _, s := range rec.statements {
		if rec.statementSelectsFrom("table_reservations", s) {
			stmt = strings.ToLower(s)
		}
	}
	require.NotEmpty(t, stmt)
	assert.False(t, strings.HasPrefix(stmt, "select *"), "must not SELECT * reservations: %s", stmt)
	assert.NotContains(t, stmt, "customer_name", "conflict check must not read guest PII")
	assert.Contains(t, stmt, "reservation_time >=", "conflict check must bound history from below")
}

func TestReservationConflictLowerBoundCoversMaxDuration(t *testing.T) {
	start := time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC)
	got := ReservationConflictLowerBound(start, 15)
	assert.Equal(t, start.Add(-time.Duration(MaxReservationDurationMinutes+15)*time.Minute), got)
}

func BenchmarkCheckTableAvailabilityHistorySQLite(b *testing.B) {
	probe := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Minute)
	tid := seedTableReservationHistory(b, 2000, probe)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := checkTableAvailabilityWithBufferTx(db, tid, probe.Add(2*time.Hour), 60, 0, 0); err != nil {
			b.Fatal(err)
		}
	}
}
