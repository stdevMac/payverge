package services

// Tests for Finding 1: overnight delivery-hours window.
//
// When a delivery window's close < open (crosses midnight, e.g. 22:00–02:00),
// the after-midnight portion was incorrectly treated as outside active hours.
// validateDeliveryWindowAt now evaluates the previous-day span so after-midnight
// times count as inside active hours.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestValidateDeliveryWindow_OvernightAfterMidnightIsInside confirms that a
// time in the after-midnight tail of an overnight window (e.g. 01:00 when the
// window is 22:00–02:00) is treated as inside active hours.
//
// Pre-fix: `now.Before(openTime)` fired (01:00 < 22:00 today) and rejected the
// request even though the window was still open.
func TestValidateDeliveryWindow_OvernightAfterMidnightIsInside(t *testing.T) {
	svc := &DeliveryService{}

	// Business timezone UTC so we control "now" deterministically.
	business := &database.Business{Timezone: "UTC"}

	// Simulate "now" = 01:30 UTC.
	// The delivery window is 22:00–02:00 (crosses midnight).
	// At 01:30 we are in the after-midnight tail → should be INSIDE.
	nowAt0130 := time.Date(2026, 6, 19, 1, 30, 0, 0, time.UTC)

	settings := &database.DeliverySettings{
		DeliveryHoursSameAsBusiness: false,
		DeliveryStartTime:           "22:00",
		DeliveryEndTime:             "02:00",
	}

	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, settings, quote, nowAt0130)
	assert.NoError(t, err, "01:30 is inside the overnight 22:00–02:00 window")
	require.NotNil(t, quote.CutoffAt, "CutoffAt must be set for an eligible overnight window")
	// CutoffAt should be 02:00 on the same day (today) because now is in the
	// after-midnight tail, not 02:00 the next day.
	assert.Equal(t, 2, quote.CutoffAt.Hour(), "CutoffAt hour should be 02:00")
}

// TestValidateDeliveryWindow_OvernightBeforeOpenIsOutside confirms that a time
// before the overnight window opens (e.g. 20:00 when window is 22:00–02:00) is
// correctly rejected.
func TestValidateDeliveryWindow_OvernightBeforeOpenIsOutside(t *testing.T) {
	svc := &DeliveryService{}
	business := &database.Business{Timezone: "UTC"}

	nowAt2000 := time.Date(2026, 6, 19, 20, 0, 0, 0, time.UTC)
	settings := &database.DeliverySettings{
		DeliveryHoursSameAsBusiness: false,
		DeliveryStartTime:           "22:00",
		DeliveryEndTime:             "02:00",
	}
	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, settings, quote, nowAt2000)
	assert.Error(t, err, "20:00 is before the overnight 22:00–02:00 window")
}

// TestValidateDeliveryWindow_OvernightAfterCloseIsOutside confirms that a time
// after the close of an overnight window (e.g. 03:00 when window is 22:00–02:00)
// is correctly rejected.
func TestValidateDeliveryWindow_OvernightAfterCloseIsOutside(t *testing.T) {
	svc := &DeliveryService{}
	business := &database.Business{Timezone: "UTC"}

	nowAt0300 := time.Date(2026, 6, 19, 3, 0, 0, 0, time.UTC)
	settings := &database.DeliverySettings{
		DeliveryHoursSameAsBusiness: false,
		DeliveryStartTime:           "22:00",
		DeliveryEndTime:             "02:00",
	}
	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, settings, quote, nowAt0300)
	assert.Error(t, err, "03:00 is after the close of the overnight 22:00–02:00 window")
}

// TestValidateDeliveryWindow_OvernightDuringEveningIsInside confirms that a time
// during the evening opening of an overnight window (e.g. 23:00 when window is
// 22:00–02:00) is correctly accepted.
func TestValidateDeliveryWindow_OvernightDuringEveningIsInside(t *testing.T) {
	svc := &DeliveryService{}
	business := &database.Business{Timezone: "UTC"}

	nowAt2300 := time.Date(2026, 6, 19, 23, 0, 0, 0, time.UTC)
	settings := &database.DeliverySettings{
		DeliveryHoursSameAsBusiness: false,
		DeliveryStartTime:           "22:00",
		DeliveryEndTime:             "02:00",
	}
	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, settings, quote, nowAt2300)
	assert.NoError(t, err, "23:00 is inside the overnight 22:00–02:00 window")
	require.NotNil(t, quote.CutoffAt)
	// When now is in the evening portion, CutoffAt should be 02:00 next day.
	assert.Equal(t, 2, quote.CutoffAt.Hour(), "CutoffAt hour should be 02:00")
}
