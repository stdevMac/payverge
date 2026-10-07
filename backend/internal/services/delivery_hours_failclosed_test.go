package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Tests for DEL-SM-4 (delivery hours failed OPEN on malformed time strings and
// settings validation never checked HH:MM format) and DEL-SM-6 (ineligible
// outside-hours quotes in the custom-hours branch still carried CutoffAt,
// violating the R14 "no fabricated numbers on ineligible quotes" rule).

// TestValidateDeliveryWindow_MalformedCustomHoursFailClosed: a persisted
// malformed custom window ("10 PM") must be treated as OUTSIDE hours (fail
// closed), not as "no window configured" (fail open, 24/7 delivery).
func TestValidateDeliveryWindow_MalformedCustomHoursFailClosed(t *testing.T) {
	svc := &DeliveryService{}
	business := &database.Business{Timezone: "UTC"}
	now := time.Date(2026, 7, 6, 3, 0, 0, 0, time.UTC) // 3 a.m.

	cases := []struct {
		name       string
		start, end string
	}{
		{name: "12-hour format start", start: "10 PM", end: "22:00"},
		{name: "dot separator end", start: "10:00", end: "22.00"},
		{name: "no separator", start: "0800", end: "22:00"},
		{name: "out-of-range hour", start: "25:00", end: "22:00"},
		{name: "out-of-range minute", start: "10:99", end: "22:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := &database.DeliverySettings{
				DeliveryHoursSameAsBusiness: false,
				DeliveryStartTime:           tc.start,
				DeliveryEndTime:             tc.end,
			}
			quote := &DeliveryQuoteDTO{}
			err := svc.validateDeliveryWindowAt(business, settings, quote, now)
			assert.Error(t, err, "malformed configured hours must fail CLOSED")
			assert.Nil(t, quote.CutoffAt, "no cutoff may be fabricated from malformed hours")
		})
	}
}

// TestValidateDeliveryWindow_MalformedBusinessHoursFailClosed: same rule for
// the hours-same-as-business branch — a malformed configured open/close time
// must reject, not silently ignore the window.
func TestValidateDeliveryWindow_MalformedBusinessHoursFailClosed(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-bizhours-malformed")
	// Corrupt today's configured hours.
	now := time.Now().UTC()
	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ? AND day_of_week = ?", business.ID, int(now.Weekday())).
		Update("open_time", "9am").Error)

	settings := &database.DeliverySettings{DeliveryHoursSameAsBusiness: true}
	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, settings, quote, now)
	assert.Error(t, err, "malformed business operating hours must fail CLOSED for delivery eligibility")
	assert.Nil(t, quote.CutoffAt)
}

// TestValidateDeliveryWindow_MissingCustomHoursFailClosed: an EMPTY custom
// window (same_as=false with blank start/end) must reject delivery — not treat
// "no window configured" as 24/7 open. Live repro: operators uncheck business
// hours without setting times and guests can quote/checkout on closed days.
func TestValidateDeliveryWindow_MissingCustomHoursFailClosed(t *testing.T) {
	svc := &DeliveryService{}
	business := &database.Business{Timezone: "UTC"}
	now := time.Date(2026, 7, 23, 16, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		start, end string
	}{
		{name: "both empty", start: "", end: ""},
		{name: "start only empty", start: "", end: "22:00"},
		{name: "end only empty", start: "10:00", end: ""},
		{name: "whitespace both", start: "  ", end: "\t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := &database.DeliverySettings{
				DeliveryHoursSameAsBusiness: false,
				DeliveryStartTime:           tc.start,
				DeliveryEndTime:             tc.end,
			}
			quote := &DeliveryQuoteDTO{}
			err := svc.validateDeliveryWindowAt(business, settings, quote, now)
			require.Error(t, err, "empty custom delivery hours must fail CLOSED")
			assert.Contains(t, err.Error(), "currently unavailable")
			assert.Nil(t, quote.CutoffAt, "ineligible empty-window quote must not carry CutoffAt")
		})
	}
}

// TestValidateDeliveryWindow_MissingBusinessDayFailClosed: when delivery hours
// follow business operating hours, a missing weekday row must reject quotes
// (fail CLOSED), not treat "no hours configured for today" as 24/7 open.
// Aligns with FE useBusinessOpenStatus (missing day → closed).
func TestValidateDeliveryWindow_MissingBusinessDayFailClosed(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-day-missing")
	// Pin a known weekday so delete + assert stay deterministic.
	now := time.Date(2026, 7, 23, 16, 0, 0, 0, time.UTC) // Thursday
	require.NoError(t, db.Where("business_id = ? AND day_of_week = ?", business.ID, int(now.Weekday())).
		Delete(&database.BusinessOperatingHours{}).Error)

	settings := &database.DeliverySettings{DeliveryHoursSameAsBusiness: true}
	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, settings, quote, now)
	require.Error(t, err, "missing business operating-hours day must fail CLOSED")
	assert.Contains(t, err.Error(), "currently unavailable")
	assert.Nil(t, quote.CutoffAt, "ineligible missing-day quote must not carry CutoffAt")
}

// TestValidateDeliveryWindow_BusinessDayIsClosed: when delivery hours follow
// business operating hours, an IsClosed=true day must reject quotes (live
// LOG-015: day closed → outside_delivery_hours / "currently unavailable").
func TestValidateDeliveryWindow_BusinessDayIsClosed(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-day-closed")
	// Pin noon UTC so open/close windows are unambiguous if IsClosed were false.
	now := time.Date(2026, 7, 23, 16, 0, 0, 0, time.UTC) // Thursday
	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ? AND day_of_week = ?", business.ID, int(now.Weekday())).
		Updates(map[string]interface{}{
			"is_closed":  true,
			"open_time":  "11:00",
			"close_time": "23:00",
		}).Error)

	settings := &database.DeliverySettings{DeliveryHoursSameAsBusiness: true}
	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, settings, quote, now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "currently unavailable")
	assert.Nil(t, quote.CutoffAt)
}

// TestValidateDeliveryWindow_BusinessDayOutsideClockHours: open day but now
// after close_time must reject (live LOG-015 after-hours path).
func TestValidateDeliveryWindow_BusinessDayOutsideClockHours(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-day-after-hours")
	now := time.Date(2026, 7, 23, 16, 0, 0, 0, time.UTC) // Thursday 16:00 UTC
	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ? AND day_of_week = ?", business.ID, int(now.Weekday())).
		Updates(map[string]interface{}{
			"is_closed":  false,
			"open_time":  "11:00",
			"close_time": "12:00",
		}).Error)

	settings := &database.DeliverySettings{DeliveryHoursSameAsBusiness: true}
	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, settings, quote, now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside active hours")
	assert.Nil(t, quote.CutoffAt)
}

// TestUpdateDeliverySettingsRejectsMalformedHours: the settings-save path must
// reject non-HH:MM windows so operators can never persist a malformed window
// in the first place.
func TestUpdateDeliverySettingsRejectsMalformedHours(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-hours-save")
	createDeliverySettings(t, db, business.ID, func(settings *database.DeliverySettings) {
		// Third-party only so validateDeliverySettings' zone requirement does not
		// interfere with the hours assertion under test.
		settings.InHouseDeliveryEnabled = false
	})

	strPtr := func(s string) *string { return &s }
	// DeliverySettings.DeliveryHoursSameAsBusiness has gorm default:true, so an
	// operator configuring a custom window sends the flag alongside the times.
	customHours := false

	badCases := []struct {
		name       string
		start, end *string
		wantSubstr string
	}{
		{name: "12-hour start", start: strPtr("10 PM"), end: strPtr("22:00"), wantSubstr: "HH:MM"},
		{name: "dot separator end", start: strPtr("10:00"), end: strPtr("22.00"), wantSubstr: "HH:MM"},
		{name: "out-of-range hour", start: strPtr("25:00"), end: strPtr("22:00"), wantSubstr: "HH:MM"},
		{name: "out-of-range minute", start: strPtr("10:75"), end: strPtr("22:00"), wantSubstr: "HH:MM"},
		{name: "one side empty", start: strPtr("10:00"), end: strPtr(""), wantSubstr: "required"},
		{name: "both empty", start: strPtr(""), end: strPtr(""), wantSubstr: "required"},
	}
	for _, tc := range badCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.UpdateDeliverySettingsDTO(business.ID, UpdateDeliverySettingsInput{
				DeliveryHoursSameAsBusiness: &customHours,
				DeliveryStartTime:           tc.start,
				DeliveryEndTime:             tc.end,
			})
			require.Error(t, err, "invalid delivery hours must be rejected on save")
			assert.Contains(t, err.Error(), tc.wantSubstr)
		})
	}

	// Valid HH:MM windows still save.
	updated, err := service.UpdateDeliverySettingsDTO(business.ID, UpdateDeliverySettingsInput{
		DeliveryHoursSameAsBusiness: &customHours,
		DeliveryStartTime:           strPtr("08:30"),
		DeliveryEndTime:             strPtr("22:00"),
	})
	require.NoError(t, err)
	assert.Equal(t, "08:30", updated.DeliveryStartTime)
	assert.Equal(t, "22:00", updated.DeliveryEndTime)

	// Clearing both sides while custom mode is on is rejected (empty custom
	// used to mean unrestricted 24/7 delivery).
	_, err = service.UpdateDeliverySettingsDTO(business.ID, UpdateDeliverySettingsInput{
		DeliveryHoursSameAsBusiness: &customHours,
		DeliveryStartTime:           strPtr(""),
		DeliveryEndTime:             strPtr(""),
	})
	require.Error(t, err, "empty custom delivery hours must be rejected on save")
	assert.Contains(t, err.Error(), "required")

	// Switching back to business hours (same_as=true) is still allowed without times.
	sameAs := true
	restored, err := service.UpdateDeliverySettingsDTO(business.ID, UpdateDeliverySettingsInput{
		DeliveryHoursSameAsBusiness: &sameAs,
		DeliveryStartTime:           strPtr(""),
		DeliveryEndTime:             strPtr(""),
	})
	require.NoError(t, err)
	assert.True(t, restored.DeliveryHoursSameAsBusiness)
}

// TestValidateDeliveryWindow_OutsideCustomHoursCarriesNoCutoff pins DEL-SM-6:
// an ineligible outside-hours result in the custom-hours branch must NOT carry
// a CutoffAt — mirroring the business-hours branch and the R14 rule that
// ineligible quotes carry no fabricated numbers.
func TestValidateDeliveryWindow_OutsideCustomHoursCarriesNoCutoff(t *testing.T) {
	svc := &DeliveryService{}
	business := &database.Business{Timezone: "UTC"}
	settings := &database.DeliverySettings{
		DeliveryHoursSameAsBusiness: false,
		DeliveryStartTime:           "10:00",
		DeliveryEndTime:             "22:00",
	}

	// 3 a.m. — outside the 10:00–22:00 window.
	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, settings, quote, time.Date(2026, 7, 6, 3, 0, 0, 0, time.UTC))
	require.Error(t, err)
	assert.Nil(t, quote.CutoffAt, "ineligible outside-hours quote must not carry CutoffAt (R14)")

	// Inside the window the cutoff is still populated.
	quote = &DeliveryQuoteDTO{}
	err = svc.validateDeliveryWindowAt(business, settings, quote, time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NotNil(t, quote.CutoffAt, "eligible quote keeps its cutoff")
	assert.Equal(t, 22, quote.CutoffAt.Hour())
}
