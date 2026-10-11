package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

func seedVenueNoticeOwner(t *testing.T, db *gorm.DB, business *database.Business, email string, verified bool) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&database.User{}))
	owner := database.User{Email: email, Name: "Owner", EmailVerified: verified}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", business.ID).Updates(map[string]any{
		"email":   "x@evil.example",
		"user_id": owner.ID,
	}).Error)
}

func TestGuestDeliveryCheckout_VenueNoticeUsesVerifiedOwner(t *testing.T) {
	t.Run("verified owner", func(t *testing.T) {
		db := setupHospitalityServiceTestDB(t)
		emailMock := &mockNotificationDispatcher{}
		svc := NewDeliveryService(db, newTestNotificationManager(emailMock))
		business := setupTipValidationBusiness(t, db, "delivery-owner-verified", 12)
		seedVenueNoticeOwner(t, db, business, "owner@venue.example", true)

		result, err := svc.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
		require.NoError(t, err)
		require.NotNil(t, result.DeliveryOrder)

		require.Empty(t, mailOriginsTo(emailMock, "x@evil.example"), "business.Email must not receive the notice")
		origins := mailOriginsTo(emailMock, "owner@venue.example")
		require.Equal(t, []structs.NotificationMailOrigin{{
			BusinessID:      business.ID,
			DeliveryOrderID: result.DeliveryOrder.ID,
		}}, origins)
	})

	t.Run("unverified owner", func(t *testing.T) {
		db := setupHospitalityServiceTestDB(t)
		emailMock := &mockNotificationDispatcher{}
		svc := NewDeliveryService(db, newTestNotificationManager(emailMock))
		business := setupTipValidationBusiness(t, db, "delivery-owner-unverified", 12)
		seedVenueNoticeOwner(t, db, business, "owner@venue.example", false)

		_, err := svc.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
		require.NoError(t, err)
		require.Empty(t, mailOriginsTo(emailMock, "owner@venue.example"))
		require.Empty(t, mailOriginsTo(emailMock, "x@evil.example"))
	})
}

// TestGuestDeliveryCheckout_NoticeCapSkipsTelegramAndVenueEmail: 200 delivery
// orders already in the last 24h means the next public checkout is the 201st.
// It still persists, but it enqueues no Telegram order.created row and sends
// no venue email. The owner is verified, so a missing cap would have mailed them.
func TestGuestDeliveryCheckout_NoticeCapSkipsTelegramAndVenueEmail(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	emailMock := &mockNotificationDispatcher{}
	svc := NewDeliveryService(db, newTestNotificationManager(emailMock))
	business := setupDeliveryTelegramBusiness(t, db, "delivery-notice-cap")
	seedVenueNoticeOwner(t, db, business, "owner@venue.example", true)

	now := time.Now().UTC().Add(-time.Minute)
	orders := make([]database.DeliveryOrder, maxVenueDeliveryNoticesPerDay)
	for i := range orders {
		orders[i] = database.DeliveryOrder{
			BusinessID:     business.ID,
			BillID:         1,
			DeliveryNumber: fmt.Sprintf("DEL-CAP-%03d", i),
			DeliveryType:   database.DeliveryTypeInHouse,
			Status:         database.DeliveryStatusDelivered,
			CustomerName:   "Prior",
			CustomerPhone:  "5550000000",
			QuoteMetadata:  database.JSONRawMessage(`{}`),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
	}
	require.NoError(t, db.CreateInBatches(&orders, 20).Error)

	var counted int64
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Where("business_id = ? AND created_at >= ?", business.ID, time.Now().Add(-24*time.Hour)).
		Count(&counted).Error)
	require.EqualValues(t, maxVenueDeliveryNoticesPerDay, counted, "fixture must sit inside the notice window")

	checkout, err := svc.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
	require.NoError(t, err)
	require.NotNil(t, checkout.Order)
	require.NotNil(t, checkout.DeliveryOrder)

	var total int64
	require.NoError(t, db.Model(&database.DeliveryOrder{}).Where("business_id = ?", business.ID).Count(&total).Error)
	require.EqualValues(t, maxVenueDeliveryNoticesPerDay+1, total)

	require.EqualValues(t, 0, countOrderCreatedOutboxRows(t, db, business.ID, checkout.Order.ID))
	var telegramRows int64
	require.NoError(t, db.Model(&database.PluginNotificationDelivery{}).
		Where("business_id = ? AND plugin_name = ? AND event_type = ?", business.ID, "telegram", PluginEventOrderCreated).
		Count(&telegramRows).Error)
	require.Zero(t, telegramRows)
	require.Empty(t, mailOriginsTo(emailMock, "owner@venue.example"))
	require.Empty(t, mailOriginsTo(emailMock, "x@evil.example"))
}

func TestGuestDeliveryCheckoutRejectsOversizedCustomerName(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := setupTipValidationBusiness(t, db, "delivery-name-limit", 12)

	req := tipCheckoutRequest(0)
	req.CustomerName = strings.Repeat("a", 5000)
	_, err := service.GuestDeliveryCheckout(business.ID, req)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeliveryValidation)

	var created int64
	require.NoError(t, db.Model(&database.DeliveryOrder{}).Where("business_id = ?", business.ID).Count(&created).Error)
	require.Zero(t, created)

	req.CustomerName = "Ada\u202e"
	_, err = service.GuestDeliveryCheckout(business.ID, req)
	require.ErrorIs(t, err, ErrDeliveryValidation)
}
