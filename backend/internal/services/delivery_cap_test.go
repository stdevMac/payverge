package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func seedCappedDeliveryBusiness(t *testing.T, db *gorm.DB, slug string, cap int) *database.Business {
	t.Helper()
	business := createTestHospitalityBusiness(t, db, slug)
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID:          "burger",
		Name:        "Burger",
		Price:       12,
		IsAvailable: true,
	})
	createDeliverySettings(t, db, business.ID, func(settings *database.DeliverySettings) {
		settings.MaxConcurrentDeliveries = cap
		settings.MinimumOrderAmount = 0
	})
	createDeliveryZone(t, db, business.ID, "Everywhere", func(zone *database.DeliveryZone) {
		zone.Boundaries = `{"postal_codes":["99999"]}`
		zone.MinimumOrderAmount = 0
	})
	return business
}

func TestDeliveryCap_LateInTransitStillCountsAndBlocksCheckout(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := seedCappedDeliveryBusiness(t, db, "delivery-cap-late-transit", 1)

	eta := time.Now().Add(-2 * time.Hour)
	order := database.DeliveryOrder{
		BusinessID:            business.ID,
		BillID:                1,
		DeliveryNumber:        "DEL-CAP-LATE-TRANSIT",
		DeliveryType:          database.DeliveryTypeInHouse,
		Status:                database.DeliveryStatusInTransit,
		CustomerName:          "Late Rider",
		CustomerPhone:         "5550100001",
		EstimatedDeliveryTime: &eta,
	}
	require.NoError(t, db.Create(&order).Error)

	count, err := svc.activeDeliveryCount(db, business.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	_, err = svc.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeliveryValidation)
}

func TestDeliveryCap_StalePendingWithExpiredETADoesNotCount(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := seedCappedDeliveryBusiness(t, db, "delivery-cap-stale-pending", 1)

	eta := time.Now().Add(-2 * time.Hour)
	created := time.Now().Add(-4 * time.Hour)
	order := database.DeliveryOrder{
		BusinessID:            business.ID,
		BillID:                1,
		DeliveryNumber:        "DEL-CAP-STALE-PENDING",
		DeliveryType:          database.DeliveryTypeInHouse,
		Status:                database.DeliveryStatusPending,
		CustomerName:          "Abandoned",
		CustomerPhone:         "5550100002",
		EstimatedDeliveryTime: &eta,
	}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Model(&database.DeliveryOrder{}).Where("id = ?", order.ID).
		UpdateColumns(map[string]any{"created_at": created, "updated_at": created}).Error)

	count, err := svc.activeDeliveryCount(db, business.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}

func TestDeliveryCap_SequentialCheckoutStopsAtCap(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	svc := NewDeliveryService(db, nil)
	business := seedCappedDeliveryBusiness(t, db, "delivery-cap-sequential", 1)

	first, err := svc.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NotNil(t, first.DeliveryOrder)

	_, err = svc.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeliveryValidation)

	var rows int64
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Where("business_id = ?", business.ID).
		Count(&rows).Error)
	require.Equal(t, int64(1), rows)
}
