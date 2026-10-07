package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestCreateReservationGuestRejectsSuppliedDuration(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-guest-duration-reject")
	createTestTable(t, db, business.ID, "T-guest-dur", 4)
	settings := configureReservationSettings(t, business.ID, nil)
	require.Equal(t, 90, settings.DefaultDuration)

	target := nextDayAt(18, 0)
	// Any supplied duration is rejected: matching default, a fit custom
	// length, or a huge probe that would otherwise trip operating hours.
	for _, minutes := range []int{settings.DefaultDuration, 180, 100000} {
		minutes := minutes
		t.Run(fmt.Sprintf("%dmin", minutes), func(t *testing.T) {
			created, err := service.CreateReservation(business.ID, CreateReservationInput{
				CustomerName:    "Guest Duration Probe",
				CustomerPhone:   "5550000180",
				CustomerEmail:   "guest-duration@example.com",
				PartySize:       2,
				ReservationTime: target.Format(time.RFC3339),
				Duration:        intPtr(minutes),
			}, "customer", true)
			require.ErrorIs(t, err, ErrGuestDurationNotAllowed)
			require.Nil(t, created)
		})
	}

	var count int64
	require.NoError(t, db.Model(&database.TableReservation{}).
		Where("business_id = ?", business.ID).Count(&count).Error)
	require.EqualValues(t, 0, count)
}

func TestCreateReservationGuestUsesDefaultDurationWhenOmitted(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-guest-duration-default")
	createTestTable(t, db, business.ID, "T-guest-def", 4)
	settings := configureReservationSettings(t, business.ID, nil)

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Guest Default Duration",
		CustomerPhone:   "5550000090",
		CustomerEmail:   "guest-default-duration@example.com",
		PartySize:       2,
		ReservationTime: nextDayAt(18, 30).Format(time.RFC3339),
	}, "customer", true)
	require.NoError(t, err)
	require.Equal(t, settings.DefaultDuration, created.Duration)
	require.NotEqual(t, 100000, created.Duration)
}

func TestCreateReservationStaffHonoursCustomDuration(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-staff-duration")
	createTestTable(t, db, business.ID, "T-staff-dur", 4)
	configureReservationSettings(t, business.ID, nil)

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Staff Custom Duration",
		CustomerPhone:   "5550000045",
		CustomerEmail:   "staff-duration@example.com",
		PartySize:       2,
		ReservationTime: nextDayAt(19, 0).Format(time.RFC3339),
		Duration:        intPtr(45),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)
	require.Equal(t, 45, created.Duration)
}
