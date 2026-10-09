package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Regression for the CI flake where hospitality fixtures used CloseTime/EndTime
// "23:59" (parsed as 23:59:00). Quote/checkout paths then rejected wall-clock
// times in 23:59:01–23:59:59 with "outside active hours".
func TestHospitalityDeliveryFixturesCoverFinalUTCMinute(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "fixture-final-minute")
	settings := createDeliverySettings(t, db, business.ID, nil)

	var persisted database.DeliverySettings
	require.NoError(t, db.First(&persisted, settings.ID).Error)
	assert.False(t, persisted.DeliveryHoursSameAsBusiness,
		"createDeliverySettings must persist custom hours despite gorm default:true")
	assert.False(t, settings.DeliveryHoursSameAsBusiness)
	assert.Equal(t, "00:00", persisted.DeliveryStartTime)
	assert.Equal(t, "00:00", persisted.DeliveryEndTime)

	now := time.Date(2026, 8, 11, 23, 59, 38, 0, time.UTC)
	err := service.validateDeliveryWindowAt(business, &persisted, &DeliveryQuoteDTO{}, now)
	require.NoError(t, err, "custom 00:00–00:00 fixture must cover 23:59:38")

	err = service.validateDeliveryWindowAt(business, &database.DeliverySettings{
		DeliveryHoursSameAsBusiness: true,
	}, &DeliveryQuoteDTO{}, now)
	require.NoError(t, err, "business-hours 00:00–00:00 fixture must cover 23:59:38")
}
