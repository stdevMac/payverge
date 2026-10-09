package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOperationalAlertModelsRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(
		&Business{},
		&Staff{},
		&User{},
		&OperationalAlert{},
		&OperationalAlertEvent{},
		&BusinessAlertSettings{},
	))

	business := Business{
		BusinessId:     "restaurant-1",
		OwnerAddress:   "0xowner",
		Name:           "Restaurant 1",
		SettlementAddr: "0xsettlement",
		TippingAddr:    "0xtipping",
	}
	require.NoError(t, db.Create(&business).Error)

	lastEventAt := time.Date(2026, 6, 4, 15, 30, 0, 0, time.UTC)
	alert := OperationalAlert{
		BusinessID:   business.ID,
		AlertType:    OperationalAlertTypeOrderNew,
		ResourceType: OperationalAlertResourceTypeOrder,
		ResourceID:   77,
		Status:       OperationalAlertStatusOpen,
		Priority:     OperationalAlertPriorityUrgent,
		Title:        "New order",
		Body:         "Order 77 needs attention",
		LastEventAt:  lastEventAt,
		Metadata:     JSONRawMessage("{}"),
	}
	require.NoError(t, db.Create(&alert).Error)

	event := OperationalAlertEvent{
		AlertID:    alert.ID,
		BusinessID: business.ID,
		EventType:  OperationalAlertEventTypeCreated,
		ActorName:  "system",
		Metadata:   JSONRawMessage("{}"),
	}
	require.NoError(t, db.Create(&event).Error)

	var loaded OperationalAlert
	require.NoError(t, db.Preload("Events").First(&loaded, alert.ID).Error)

	assert.Equal(t, OperationalAlertTypeOrderNew, loaded.AlertType)
	assert.Equal(t, OperationalAlertStatusOpen, loaded.Status)
	require.Len(t, loaded.Events, 1)
	assert.Equal(t, OperationalAlertEventTypeCreated, loaded.Events[0].EventType)
	assert.Equal(t, "system", loaded.Events[0].ActorName)
}

func TestDefaultBusinessAlertSettings(t *testing.T) {
	settings := DefaultBusinessAlertSettings(10)

	assert.Equal(t, uint(10), settings.BusinessID)
	assert.True(t, settings.Enabled)
	assert.True(t, settings.BrowserNotificationsEnabled)
	// #267: sound opt-in; no 8s looping alarm by default.
	assert.False(t, settings.SoundEnabled)
	assert.InDelta(t, 0.8, settings.Volume, 0.0001)
	assert.Equal(t, DefaultBusinessAlertRepeatIntervalSeconds, settings.RepeatIntervalSeconds)
	assert.Equal(t, MinBusinessAlertRepeatIntervalSeconds, settings.RepeatIntervalSeconds)
	assert.Equal(t, 30, settings.RepeatIntervalSeconds)

	assert.True(t, settings.EventSettings.OrderNew.Enabled)
	assert.True(t, settings.EventSettings.OrderNew.Repeating)
	assert.Equal(t, OperationalAlertPriorityUrgent, settings.EventSettings.OrderNew.Priority)

	assert.True(t, settings.EventSettings.PaymentReceived.Enabled)
	assert.False(t, settings.EventSettings.PaymentReceived.Repeating)
	assert.Equal(t, OperationalAlertPriorityNormal, settings.EventSettings.PaymentReceived.Priority)
}
