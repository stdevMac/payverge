package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A booking that arrives without a usable locale must inherit the venue's
// default language, not English. Live QA found tonight's reservation at an
// es/ARS venue stamped `language: en`, which then drives the guest
// confirmation/reminder emails in the wrong language.
func TestCreateReservationLanguageFallsBackToVenueDefault(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 8, 23, 18, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-language-default")
	require.NoError(t, db.Model(business).Update("default_language", "es").Error)
	createTestTable(t, db, business.ID, "Mesa 4", 6)
	configureReservationSettings(t, business.ID, nil)

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Federico",
		PartySize:       5,
		ReservationTime: now.Add(3 * time.Hour).Format(time.RFC3339),
		Source:          "online",
	}, "guest", false)
	require.NoError(t, err)
	require.Equal(t, "es", created.Language)
}

// An unsupported locale string is not an explicit guest choice — it must fall
// through to the venue default rather than short-circuiting to English.
func TestCreateReservationLanguageIgnoresUnsupportedRequest(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 8, 23, 18, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-language-unsupported")
	require.NoError(t, db.Model(business).Update("default_language", "es-AR").Error)
	createTestTable(t, db, business.ID, "Mesa 5", 6)
	configureReservationSettings(t, business.ID, nil)

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Federico",
		PartySize:       4,
		ReservationTime: now.Add(3 * time.Hour).Format(time.RFC3339),
		Source:          "online",
		Language:        "xx-YY",
	}, "guest", false)
	require.NoError(t, err)
	require.Equal(t, "es-AR", created.Language)
}

// An explicit, supported guest locale always wins over the venue default.
func TestCreateReservationLanguagePrefersExplicitGuestChoice(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 8, 23, 18, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-language-explicit")
	require.NoError(t, db.Model(business).Update("default_language", "es").Error)
	createTestTable(t, db, business.ID, "Mesa 6", 6)
	configureReservationSettings(t, business.ID, nil)

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Visitor",
		PartySize:       2,
		ReservationTime: now.Add(3 * time.Hour).Format(time.RFC3339),
		Source:          "online",
		Language:        "en",
	}, "guest", false)
	require.NoError(t, err)
	require.Equal(t, "en", created.Language)
}

// With no locale anywhere the stamp stays English.
func TestCreateReservationLanguageDefaultsToEnglishWithoutVenueDefault(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 8, 23, 18, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-language-none")
	require.NoError(t, db.Model(business).Update("default_language", "").Error)
	createTestTable(t, db, business.ID, "Mesa 7", 6)
	configureReservationSettings(t, business.ID, nil)

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Visitor",
		PartySize:       2,
		ReservationTime: now.Add(3 * time.Hour).Format(time.RFC3339),
		Source:          "online",
	}, "guest", false)
	require.NoError(t, err)
	require.Equal(t, "en", created.Language)
}
