package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// L1-1: staff creates skip the min-advance rule (door walk-ins), but that left
// NO lower bound at all — a reservation dated 2020 was accepted, then became
// invisible under every horizon filter and uncancellable. A slot whose entire
// window has already elapsed can only be an input error and must be rejected;
// a party seated minutes ago (slot still running) must stay bookable.

func TestCreateReservationRejectsFullyElapsedSlotForStaff(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 7, 18, 18, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-past-reject")
	createTestTable(t, db, business.ID, "T1", 4)
	configureReservationSettings(t, business.ID, nil)

	_, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Time Traveler",
		PartySize:       2,
		ReservationTime: now.Add(-3 * time.Hour).Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)

	require.Error(t, err, "a fully elapsed slot must be rejected at create")
	require.Contains(t, err.Error(), "past")
}

func TestCreateReservationAllowsRecentlySeatedWalkIn(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 7, 18, 18, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-walkin-grace")
	createTestTable(t, db, business.ID, "T1", 4)
	configureReservationSettings(t, business.ID, nil)

	// Seated 30 minutes ago with the default 120-minute duration: the slot is
	// still running, so the door workflow must keep working.
	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Walk-in",
		PartySize:       2,
		ReservationTime: now.Add(-30 * time.Minute).Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)

	require.NoError(t, err, "an in-progress walk-in slot must stay bookable at the door")
	require.NotNil(t, created)
}
