package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// nextWeekdayAt returns the next UTC occurrence of weekday at hour:minute
// that is at least a day ahead, so min-advance and "today" never swallow it.
func nextWeekdayAt(weekday time.Weekday, hour, minute int) time.Time {
	now := time.Now().UTC()
	days := (int(weekday) - int(now.Weekday()) + 7) % 7
	if days == 0 {
		days = 7
	}
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC).AddDate(0, 0, days)
}

func TestReservationMissingWeekdayHoursAreClosed(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-monday-only")
	createTestTable(t, db, business.ID, "Main", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.DefaultDuration = 90
		settings.SlotIntervalMinutes = 30
		settings.MinAdvanceMinutes = 0
		settings.ServiceBufferMinutes = 0
	})

	require.NoError(t, db.Where("business_id = ?", business.ID).Delete(&database.BusinessOperatingHours{}).Error)
	require.NoError(t, db.Create(&database.BusinessOperatingHours{
		BusinessID: business.ID,
		DayOfWeek:  int(time.Monday),
		OpenTime:   "11:00",
		CloseTime:  "22:00",
		IsClosed:   false,
	}).Error)

	monday := nextWeekdayAt(time.Monday, 18, 0)
	tuesday := nextWeekdayAt(time.Tuesday, 18, 0)
	publicBusiness := publicBusinessForTest(t, business.ID)

	tuesdayAvailability, err := service.GetPublicAvailability(publicBusiness, tuesday, 2)
	require.NoError(t, err)
	assert.Empty(t, tuesdayAvailability.AvailableSlots)

	_, err = service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Tuesday Guest",
		CustomerPhone:   "5550100202",
		CustomerEmail:   "tuesday@example.test",
		PartySize:       2,
		ReservationTime: tuesday.Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")

	mondayAvailability, err := service.GetPublicAvailability(publicBusiness, monday, 2)
	require.NoError(t, err)
	require.NotEmpty(t, mondayAvailability.AvailableSlots)
	openSlots := 0
	for _, slot := range mondayAvailability.AvailableSlots {
		if slot.AvailableTables > 0 {
			openSlots++
		}
	}
	assert.Greater(t, openSlots, 0)
}
