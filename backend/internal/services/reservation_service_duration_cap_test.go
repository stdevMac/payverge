package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// The conflict reads bound their history scan by the longest allowed
// booking, so a longer duration must be rejected up front (I4).
func TestValidateReservationDurationRejectsBeyondConflictLookback(t *testing.T) {
	assert.Error(t, validateReservationDuration(0))
	assert.NoError(t, validateReservationDuration(90))
	assert.NoError(t, validateReservationDuration(database.MaxReservationDurationMinutes))
	err := validateReservationDuration(database.MaxReservationDurationMinutes + 1)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "duration must")
	}
}

func TestUpdateSettingsRejectsDefaultDurationPlusServiceBufferOverCap(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-duration-buffer-settings")
	configureReservationSettings(t, business.ID, nil)

	duration := database.MaxReservationDurationMinutes
	buffer := 30
	_, err := service.UpdateSettings(business.ID, UpdateReservationSettingsInput{
		DefaultDuration:      &duration,
		ServiceBufferMinutes: &buffer,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default_duration plus service_buffer_minutes must be at most 1440 minutes")
	var validationErr *ReservationSettingsValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, err.Error(), validationErr.Error())
}

func TestStaffCreateRejectsDurationBeyondCap(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-duration-cap-staff")
	createTestTable(t, db, business.ID, "T-cap", 4)
	configureReservationSettings(t, business.ID, nil)

	duration := 2000
	_, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Long Booking",
		CustomerPhone:   "5550002000",
		PartySize:       2,
		ReservationTime: nextDayAt(18, 0).Format(time.RFC3339),
		Duration:        &duration,
		Source:          "staff",
	}, "host", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duration must be at most 1440 minutes")
}

func TestStaffCreateRejectsDurationPlusServiceBufferOverCap(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-duration-plus-buffer")
	createTestTable(t, db, business.ID, "T-buf", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.ServiceBufferMinutes = 15
	})

	duration := 1430
	_, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Buffered Booking",
		CustomerPhone:   "5550001430",
		PartySize:       2,
		ReservationTime: nextDayAt(12, 0).Format(time.RFC3339),
		Duration:        &duration,
		Source:          "staff",
	}, "host", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duration plus the 15-minute service buffer must be at most 1440 minutes")
}
