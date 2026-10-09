package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseReservationDateTimeVenueLocalSevenPM(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	// August is EDT (UTC-4): 7pm venue == 23:00Z, never 19:00Z.
	summer, err := ParseReservationDateTime("2026-08-19T19:00", ny)
	require.NoError(t, err)
	require.Equal(t, "2026-08-19T23:00:00Z", summer.UTC().Format(time.RFC3339))
	require.NotEqual(t, "2026-08-19T19:00:00Z", summer.UTC().Format(time.RFC3339))

	// January is EST (UTC-5): 7pm venue == 00:00Z next day.
	winter, err := ParseReservationDateTime("2026-01-15T19:00", ny)
	require.NoError(t, err)
	require.Equal(t, "2026-01-16T00:00:00Z", winter.UTC().Format(time.RFC3339))
}

func TestParseReservationDateTimeKeepsRFC3339Instant(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	parsed, err := ParseReservationDateTime("2026-08-19T23:00:00Z", ny)
	require.NoError(t, err)
	require.Equal(t, "2026-08-19T23:00:00Z", parsed.UTC().Format(time.RFC3339))
}

func TestCreateReservationStoresNewYorkSevenPMAsUTCInstant(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-ny-seven")
	business.Timezone = "America/New_York"
	require.NoError(t, db.Model(business).Update("timezone", "America/New_York").Error)
	createTestTable(t, db, business.ID, "T5", 4)
	configureReservationSettings(t, business.ID, nil)

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Franky",
		PartySize:       2,
		ReservationTime: "2026-08-19T19:00",
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)
	require.Equal(t, "2026-08-19T23:00:00Z", created.ReservationTime.UTC().Format(time.RFC3339))
}
