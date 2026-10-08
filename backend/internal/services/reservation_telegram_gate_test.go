package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Reservation Telegram enqueues must pass the same eligibility gate as guest
// orders / manual payments / inventory (ShouldEnqueueTelegramNotification):
// businesses without a connected Telegram config must not get outbox rows
// enqueued into the void.
func TestEnqueueTelegramReservationEvents_RespectEligibilityGate(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	require.NoError(t, db.GetGorm().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	ResetTelegramNotificationEligibilityCache()

	reservation := &database.TableReservation{
		ID:               101,
		BusinessID:       business.ID,
		CustomerName:     "Gate Test",
		PartySize:        2,
		ReservationTime:  time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC),
		Status:           "pending",
		ConfirmationCode: "GATE01",
		UpdatedAt:        time.Now().UTC(),
		Business:         *business,
	}

	countDeliveries := func() int64 {
		var n int64
		require.NoError(t, db.GetGorm().Model(&database.PluginNotificationDelivery{}).Count(&n).Error)
		return n
	}

	// No telegram plugin configured → both emit sites must skip the outbox.
	enqueueTelegramReservationCreated(reservation, true)
	enqueueTelegramReservationStatusChanged(reservation, 1)
	require.EqualValues(t, 0, countDeliveries(), "ineligible business must not enqueue reservation notifications")

	// Connect Telegram for the business → both emit sites enqueue.
	plugin := database.Plugin{Name: "telegram", DisplayName: "Telegram", IsActive: true, Category: "integration"}
	require.NoError(t, db.GetGorm().Create(&plugin).Error)
	require.NoError(t, db.GetGorm().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   plugin.ID,
		IsEnabled:  true,
		Config:     `{"is_connected":true,"chat_id":"12345"}`,
	}).Error)
	ResetTelegramNotificationEligibilityCache()

	enqueueTelegramReservationCreated(reservation, true)
	enqueueTelegramReservationStatusChanged(reservation, 2)
	require.EqualValues(t, 2, countDeliveries(), "eligible business enqueues both reservation notifications")
}
