package services

// Tests for Finding 2: per-zone CutoffBufferMinutes applied to CutoffAt.
//
// The matched zone's CutoffBufferMinutes is fully plumbed but was never applied
// to the computed CutoffAt. QuoteDelivery now subtracts CutoffBufferMinutes from
// CutoffAt after zone resolution and re-evaluates eligibility against the
// buffered cutoff.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestQuoteDelivery_ZoneCutoffBufferShiftsCutoffAt verifies that when a matched
// zone has CutoffBufferMinutes > 0, the quote's CutoffAt is shifted back by that
// many minutes relative to the raw delivery window close time.
//
// Pre-fix: CutoffBufferMinutes was stored and surfaced in the DTO but never
// subtracted from CutoffAt inside QuoteDelivery.
//
// Operating hours are wall-clock relative (open 4h ago, close 2h from now) so
// the eligibility precondition holds at any UTC time of day — a fixed all-day
// window with a 30-min buffer fails near midnight (effective cutoff already past).
// Today's and yesterday's rows both carry it, for windows that cross midnight.
func TestQuoteDelivery_ZoneCutoffBufferShiftsCutoffAt(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "cutoff-buffer-shifts")
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID: "burger", Name: "Burger", Price: 15, IsAvailable: true,
	})

	now := time.Now().In(time.UTC)
	openStr := now.Add(-4 * time.Hour).Format("15:04")
	closeAt := now.Add(2 * time.Hour)
	closeStr := closeAt.Format("15:04")
	// Yesterday gets the same window: between 00:00 and 04:00 UTC the window
	// crosses midnight, and the after-midnight tail of an overnight window is
	// read from yesterday's row, not today's.
	for _, day := range []int{int(now.Weekday()), int(now.AddDate(0, 0, -1).Weekday())} {
		require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
			Where("business_id = ? AND day_of_week = ?", business.ID, day).
			UpdateColumns(map[string]interface{}{
				"open_time":  openStr,
				"close_time": closeStr,
				"is_closed":  false,
			}).Error)
	}

	createDeliverySettings(t, db, business.ID, func(s *database.DeliverySettings) {
		s.DeliveryHoursSameAsBusiness = true
		s.MinimumOrderAmount = 0
	})
	// Zone with a 30-minute cutoff buffer.
	createDeliveryZone(t, db, business.ID, "Buffer Zone", func(z *database.DeliveryZone) {
		z.Boundaries = `{"postal_codes":["99999"]}`
		z.CutoffBufferMinutes = 30
		z.MinimumOrderAmount = 0
	})

	quote, err := svc.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal: 20,
		DeliveryAddress: database.DeliveryAddress{
			Street:     "123 Main Street",
			PostalCode: "99999",
			City:       "Testville",
			Country:    "US",
		},
	})
	require.NoError(t, err)
	require.True(t, quote.Eligible, "quote should be eligible: %s", quote.ReasonCode)
	require.NotNil(t, quote.CutoffAt, "CutoffAt must be set")

	// Raw window closes at closeAt. After applying the 30-min buffer,
	// CutoffAt should be closeAt − 30 minutes.
	expected := closeAt.Add(-30 * time.Minute)
	assert.Equal(t, expected.Hour(), quote.CutoffAt.UTC().Hour(), "CutoffAt hour should match close−30m")
	assert.Equal(t, expected.Minute(), quote.CutoffAt.UTC().Minute(), "CutoffAt minute should match close−30m")
}

// TestQuoteDelivery_ZoneCutoffBufferBlocksLateOrders verifies that when
// CutoffBufferMinutes pushes the effective cutoff before now, the quote is
// rejected with reason_code "outside_delivery_hours" rather than returning
// an eligible quote.
//
// Pre-fix: the buffer was never applied, so a guest placing an order 10 minutes
// before the window end (within a 30-min buffer) would receive an eligible quote
// and complete checkout even though the kitchen could not fulfil the order.
//
// The business has DeliveryHoursSameAsBusiness (the GORM/column default) so we
// wire business operating hours that close 10 minutes from now. A 30-minute
// zone buffer makes the effective cutoff 20 minutes in the past → rejected.
func TestQuoteDelivery_ZoneCutoffBufferBlocksLateOrders(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "cutoff-buffer-blocks")
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID: "pizza", Name: "Pizza", Price: 15, IsAvailable: true,
	})

	// Craft business operating hours that close in 10 minutes from now so the
	// raw window is still "open". The zone's 30-minute buffer then makes the
	// effective cutoff 20 minutes in the past.
	now := time.Now().In(time.UTC)
	// Open well before now (4 h ago) so the raw check does not reject.
	openStr := now.Add(-4 * time.Hour).Format("15:04")
	// Close 10 minutes from now — the raw window is open; the 30-min buffer closes it.
	closeStr := now.Add(10 * time.Minute).Format("15:04")

	// Override the business operating hours for today's weekday.
	today := int(now.Weekday())
	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ? AND day_of_week = ?", business.ID, today).
		UpdateColumns(map[string]interface{}{
			"open_time":  openStr,
			"close_time": closeStr,
			"is_closed":  false,
		}).Error)

	// DeliveryHoursSameAsBusiness defaults to true at the column level (GORM
	// default:true). createDeliverySettings stores it correctly via explicit Save.
	createDeliverySettings(t, db, business.ID, func(s *database.DeliverySettings) {
		s.DeliveryHoursSameAsBusiness = true
		s.MinimumOrderAmount = 0
	})
	// 30-minute cutoff buffer; effective cutoff = closeStr − 30 min = 20 min ago.
	createDeliveryZone(t, db, business.ID, "Late Block Zone", func(z *database.DeliveryZone) {
		z.Boundaries = `{"postal_codes":["10001"]}`
		z.CutoffBufferMinutes = 30
		z.MinimumOrderAmount = 0
	})

	quote, err := svc.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal: 20,
		DeliveryAddress: database.DeliveryAddress{
			Street:     "123 Main Street",
			PostalCode: "10001",
			City:       "Testville",
			Country:    "US",
		},
	})
	require.NoError(t, err)
	assert.False(t, quote.Eligible, "should be ineligible: effective cutoff (closeStr−30min) is in the past")
	assert.Equal(t, "outside_delivery_hours", quote.ReasonCode,
		"reason code should be outside_delivery_hours when buffer blocks the order")
}

// TestQuoteDelivery_ZeroBufferHasNoEffect asserts that a zone with
// CutoffBufferMinutes == 0 behaves identically to the previous implementation:
// the CutoffAt is set to the raw window end without adjustment.
func TestQuoteDelivery_ZeroBufferHasNoEffect(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "cutoff-buffer-zero")
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID: "burger", Name: "Burger", Price: 15, IsAvailable: true,
	})

	// Business hours default to 00:00–00:00 (overnight 24h fixture window).
	createDeliverySettings(t, db, business.ID, func(s *database.DeliverySettings) {
		s.DeliveryHoursSameAsBusiness = true
		s.MinimumOrderAmount = 0
	})
	createDeliveryZone(t, db, business.ID, "No Buffer Zone", func(z *database.DeliveryZone) {
		z.Boundaries = `{"postal_codes":["20002"]}`
		z.CutoffBufferMinutes = 0 // explicit zero — must be a no-op
		z.MinimumOrderAmount = 0
	})

	quote, err := svc.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal: 20,
		DeliveryAddress: database.DeliveryAddress{
			Street:     "123 Main Street",
			PostalCode: "20002",
			City:       "Testville",
			Country:    "US",
		},
	})
	require.NoError(t, err)
	assert.True(t, quote.Eligible, "zero buffer must not block an otherwise eligible quote")
	require.NotNil(t, quote.CutoffAt)
	// Overnight equal open/close expands close to next-day 00:00; zero buffer leaves that cutoff alone.
	assert.Equal(t, 0, quote.CutoffAt.UTC().Hour())
	assert.Equal(t, 0, quote.CutoffAt.UTC().Minute())
}
