package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Business-hours overnight windows: the after-midnight tail belongs to
// yesterday's row. 2026-06-20 is a Saturday (checked below).
func setBusinessDayHours(t *testing.T, db *gorm.DB, businessID uint, day time.Weekday, open, closeAt string, closed bool) {
	t.Helper()
	res := db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ? AND day_of_week = ?", businessID, int(day)).
		Updates(map[string]interface{}{
			"open_time":  open,
			"close_time": closeAt,
			"is_closed":  closed,
		})
	require.NoError(t, res.Error)
	require.Equal(t, int64(1), res.RowsAffected)
}

func TestDeliveryWindow_BusinessHours_SaturdayTailCoversSundayEarlyMorning(t *testing.T) {
	saturday := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	require.Equal(t, time.Saturday, saturday.Weekday())

	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "del-overnight-sun-tail")
	require.Equal(t, "UTC", business.Timezone)
	setBusinessDayHours(t, db, business.ID, time.Saturday, "22:00", "02:00", false)
	setBusinessDayHours(t, db, business.ID, time.Sunday, "11:00", "22:00", true)

	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, &database.DeliverySettings{
		DeliveryHoursSameAsBusiness: true,
	}, quote, time.Date(2026, 6, 21, 1, 30, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NotNil(t, quote.CutoffAt)
	assert.True(t, quote.CutoffAt.Equal(time.Date(2026, 6, 21, 2, 0, 0, 0, time.UTC)),
		"cutoff %s", quote.CutoffAt)
}

func TestDeliveryWindow_BusinessHours_FridayEarlyMorningDoesNotUseSameDayOvernight(t *testing.T) {
	friday := time.Date(2026, 6, 19, 0, 30, 0, 0, time.UTC)
	require.Equal(t, time.Friday, friday.Weekday())
	require.Equal(t, time.Thursday, friday.AddDate(0, 0, -1).Weekday())

	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "del-overnight-fri-early")
	setBusinessDayHours(t, db, business.ID, time.Thursday, "12:00", "20:00", false)
	setBusinessDayHours(t, db, business.ID, time.Friday, "22:00", "02:00", false)

	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, &database.DeliverySettings{
		DeliveryHoursSameAsBusiness: true,
	}, quote, friday)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside active hours")
	assert.Nil(t, quote.CutoffAt)
}

func TestDeliveryWindow_BusinessHours_FridayEveningOvernightCutoffIsSaturday(t *testing.T) {
	fridayEvening := time.Date(2026, 6, 19, 23, 30, 0, 0, time.UTC)
	require.Equal(t, time.Friday, fridayEvening.Weekday())

	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "del-overnight-fri-evening")
	setBusinessDayHours(t, db, business.ID, time.Thursday, "12:00", "20:00", false)
	setBusinessDayHours(t, db, business.ID, time.Friday, "22:00", "02:00", false)

	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, &database.DeliverySettings{
		DeliveryHoursSameAsBusiness: true,
	}, quote, fridayEvening)
	require.NoError(t, err)
	require.NotNil(t, quote.CutoffAt)
	assert.True(t, quote.CutoffAt.Equal(time.Date(2026, 6, 20, 2, 0, 0, 0, time.UTC)),
		"cutoff %s", quote.CutoffAt)
}

func TestDeliveryWindow_BusinessHours_SundayAfterSaturdayCloseRejected(t *testing.T) {
	sunday := time.Date(2026, 6, 21, 2, 30, 0, 0, time.UTC)
	require.Equal(t, time.Sunday, sunday.Weekday())

	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "del-overnight-sun-after")
	setBusinessDayHours(t, db, business.ID, time.Saturday, "22:00", "02:00", false)
	// Sunday is closed, so 02:30 is past Saturday's 02:00 tail and has no
	// Sunday window of its own. The default 00:00–00:00 fixture would otherwise
	// treat all of Sunday as open.
	setBusinessDayHours(t, db, business.ID, time.Sunday, "11:00", "22:00", true)

	quote := &DeliveryQuoteDTO{}
	err := svc.validateDeliveryWindowAt(business, &database.DeliverySettings{
		DeliveryHoursSameAsBusiness: true,
	}, quote, sunday)
	require.Error(t, err)
	assert.Nil(t, quote.CutoffAt)
}
