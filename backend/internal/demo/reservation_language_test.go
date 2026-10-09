package demo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Issue 853: the QA-cited reservation ("Federico Ibarra, party 5, Mesa 4,
// 21:00 ART") is demo-seeded, not booked through the API. The generator wrote
// no Language at all, so every showroom booking on an es-AR venue took the
// column default "en" and the host texts, confirmations and name board all
// followed an English locale.

func demoReservationsFor(t *testing.T, db *gorm.DB, businessID uint) []database.TableReservation {
	t.Helper()
	var rows []database.TableReservation
	require.NoError(t, db.Where("business_id = ? AND confirmation_code LIKE ?",
		businessID, demoReservationCodePrefix(businessID)+"%").
		Order("reservation_time ASC").Find(&rows).Error)
	return rows
}

func orderedDemoBusinesses(t *testing.T, db *gorm.DB, adminID uint) []database.Business {
	t.Helper()
	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", adminID).Order("id ASC").Find(&businesses).Error)
	require.Len(t, businesses, 2)
	return businesses
}

// New rows must be stamped from the venue's default language.
func TestDemoReservationsCarryVenueDefaultLanguage(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "reservation-language@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 3})
	require.NoError(t, mustEnsure(svc, admin.ID))

	for _, business := range orderedDemoBusinesses(t, db, admin.ID) {
		require.Equal(t, "es", business.DefaultLanguage, "showroom venues are Buenos Aires venues")
		rows := demoReservationsFor(t, db, business.ID)
		require.NotEmpty(t, rows, "business %d seeded no reservations", business.ID)
		for _, row := range rows {
			require.Equalf(t, "es", row.Language,
				"reservation %s on an es venue must not be English", row.ConfirmationCode)
		}
	}
}

// The generator returns early on any day whose confirmation code already
// exists, so a reseed can never repair the live English rows on its own — and
// a re-ensure on a fully simulated instance does not even re-enter
// generateDays. The heal has to run on every ensure.
func TestDemoReservationLanguageHealsExistingEnglishRows(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "reservation-language-heal@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 3})
	require.NoError(t, mustEnsure(svc, admin.ID))

	businesses := orderedDemoBusinesses(t, db, admin.ID)
	core := businesses[0]

	// Reproduce the live prod state: seeded rows stamped with the column
	// default instead of the venue language.
	require.NoError(t, db.Model(&database.TableReservation{}).
		Where("business_id = ? AND confirmation_code LIKE ?", core.ID, demoReservationCodePrefix(core.ID)+"%").
		Update("language", "en").Error)

	// A guest who actually booked on the demo venue and chose English. Its
	// confirmation code is not one the generator owns, so the heal must leave
	// it exactly as the guest left it.
	guest := database.TableReservation{
		BusinessID: core.ID, CustomerName: "Guest Booking", CustomerPhone: "+5491100000000",
		CustomerEmail: "guest@example.com", PartySize: 2,
		ReservationTime: fixedNow().Add(24 * time.Hour), Duration: 120,
		Status: "confirmed", Source: "customer", ConfirmationCode: "QK7F2M9X",
		Language: "en", CreatedBy: "customer",
	}
	require.NoError(t, db.Create(&guest).Error)

	require.NoError(t, mustEnsure(svc, admin.ID))

	rows := demoReservationsFor(t, db, core.ID)
	require.NotEmpty(t, rows)
	for _, row := range rows {
		require.Equalf(t, "es", row.Language,
			"ensure must heal seeded reservation %s off the English default", row.ConfirmationCode)
	}

	var healedGuest database.TableReservation
	require.NoError(t, db.First(&healedGuest, guest.ID).Error)
	require.Equal(t, "en", healedGuest.Language, "a guest's own locale choice is not the seeder's to rewrite")
}

// The stamp follows the venue, not a hardcoded "es": an English-default venue
// (the US showcase shape) keeps English reservations, and its twin es venue is
// unaffected.
func TestDemoReservationLanguageFollowsAnEnglishVenue(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "reservation-language-en@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 3})
	require.NoError(t, mustEnsure(svc, admin.ID))

	businesses := orderedDemoBusinesses(t, db, admin.ID)
	english, spanish := businesses[0], businesses[1]

	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", english.ID).
		Updates(map[string]interface{}{"default_language": "en", "source_language": "en"}).Error)
	// Force the seeded rows to the wrong language in both directions so the
	// assertions cannot pass on leftover state.
	require.NoError(t, db.Model(&database.TableReservation{}).
		Where("business_id = ?", english.ID).Update("language", "es").Error)
	require.NoError(t, db.Model(&database.TableReservation{}).
		Where("business_id = ?", spanish.ID).Update("language", "en").Error)

	require.NoError(t, mustEnsure(svc, admin.ID))

	for _, row := range demoReservationsFor(t, db, english.ID) {
		require.Equalf(t, "en", row.Language, "en venue reservation %s", row.ConfirmationCode)
	}
	for _, row := range demoReservationsFor(t, db, spanish.ID) {
		require.Equalf(t, "es", row.Language, "es venue reservation %s", row.ConfirmationCode)
	}
}

// The heal must be a no-op once the rows are correct — ensure runs hourly.
func TestDemoReservationLanguageHealIsIdempotent(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "reservation-language-idempotent@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 3})
	require.NoError(t, mustEnsure(svc, admin.ID))

	core := orderedDemoBusinesses(t, db, admin.ID)[0]
	before := demoReservationsFor(t, db, core.ID)
	require.NoError(t, mustEnsure(svc, admin.ID))
	after := demoReservationsFor(t, db, core.ID)

	require.Len(t, after, len(before), "the heal must not mint or drop rows")
	for i := range before {
		require.Equal(t, before[i].ID, after[i].ID)
		require.Equal(t, "es", after[i].Language)
	}
}

func TestDemoLocaleDrivesReservationLanguage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		business database.Business
		want     string
	}{
		{"es venue", database.Business{DefaultLanguage: "es"}, "es"},
		{"es-AR venue", database.Business{DefaultLanguage: "es-AR"}, "es"},
		{"en venue", database.Business{DefaultLanguage: "en"}, "en"},
		{"falls back to source language", database.Business{SourceLanguage: "es"}, "es"},
		{"unset", database.Business{}, "en"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, demoLocale(&tc.business))
		})
	}
}
