package services

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
)

// Host "table options" must never recommend a table that is occupied right now,
// even when the booking sits beyond the near-term occupancy window. Live QA hit
// this on a 21:00 ART party of five booked at 16:25 ART: Mesa 9 was serving an
// open bill, yet came back recommended=true because the 2h near-term guard did
// not apply that far out.
func TestGetTableOptionsDoesNotRecommendOccupiedTableForFarFutureBooking(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	require.NoError(t, err)
	// 2026-08-23 16:25 ART — the moment the host opened the table picker.
	now := time.Date(2026, 8, 23, 16, 25, 0, 0, loc).UTC()
	service.now = func() time.Time { return now }

	business := createTestHospitalityBusiness(t, db, "reservation-far-future-occupancy")
	require.NoError(t, db.Model(business).Update("timezone", "America/Argentina/Buenos_Aires").Error)
	mesa9 := createTestTable(t, db, business.ID, "Mesa 9", 6)
	mesa7 := createTestTable(t, db, business.ID, "Mesa 7", 6)
	configureReservationSettings(t, business.ID, nil)

	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        mesa9.ID,
		BillNumber:     "occupied-mesa-9",
		Status:         database.BillStatusOpen,
		SettlementAddr: "settlement",
		TippingAddr:    "tipping",
		CreatedAt:      now.Add(-5 * time.Hour),
	}).Error)

	// 21:00 ART the same evening: 4h35m out, well past ReservationNearTermWindow.
	reservationTime := time.Date(2026, 8, 23, 21, 0, 0, 0, loc).UTC()
	require.True(t, reservationTime.After(now.Add(ReservationNearTermWindow)))

	options, err := service.GetTableOptions(business.ID, reservationTime, 120, 5, 0)
	require.NoError(t, err)
	byID := make(map[uint]ReservationTableOptionDTO, len(options))
	for _, option := range options {
		byID[option.ID] = option
	}

	occupiedOption := byID[mesa9.ID]
	require.Equal(t, database.TableOccupancyOccupied, occupiedOption.OccupancyState)
	require.True(t, occupiedOption.ReservationAvailable, "no competing reservation should hold Mesa 9")
	// Far-future bookings must stay bookable on an occupied table (the host can
	// still pick it deliberately) but must never be *recommended*.
	require.False(t, occupiedOption.RequiresOccupancyOverride, "far-future booking must not need an occupancy override")
	require.False(t, occupiedOption.Recommended, "an occupied table must never be recommended")

	require.True(t, byID[mesa7.ID].Recommended, "the free same-capacity table stays recommended")
}
