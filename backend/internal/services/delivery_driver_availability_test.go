package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestDriverAssignableWithoutManualOnlineStep is the R3-DL-1 regression: there
// is no production route that flips a driver to DriverStatusOnline, so gating
// assignment/availability on status="online" left every real driver permanently
// unassignable ("No drivers available"). Availability now keys off
// is_available && is_active && current_delivery_id IS NULL. A driver created
// with the default status (offline) but marked available must be assignable and
// must appear in GetAvailableDrivers.
func TestDriverAssignableWithoutManualOnlineStep(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-avail")
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID:          "pizza",
		Name:        "Pizza",
		Price:       16,
		IsAvailable: true,
	})
	createDeliverySettings(t, db, business.ID, nil)
	createDeliveryZone(t, db, business.ID, "Avail Zone", func(zone *database.DeliveryZone) {
		zone.Boundaries = `{"postal_codes":["00000"]}`
		zone.MinimumOrderAmount = 500
	})

	checkout, err := service.GuestDeliveryCheckout(business.ID, GuestDeliveryCheckoutRequest{
		CustomerName:  "Avail Guest",
		CustomerPhone: "5559999999",
		CustomerEmail: "avail-guest@example.com",
		DeliveryAddress: database.DeliveryAddress{
			Street:     "42 Avail Road",
			City:       "Anywhere",
			PostalCode: "00000",
			Country:    "US",
		},
		Items: []DeliveryCheckoutItemInput{
			{MenuItemName: "Pizza", Quantity: 1, Price: 16},
		},
	})
	require.NoError(t, err)

	// Driver created with the DEFAULT status (offline) — no manual "online" step.
	// Only is_available is toggled on, exactly as the operator availability toggle does.
	driver := &database.DeliveryDriver{
		BusinessID:    business.ID,
		Name:          "Offline-but-Available",
		Phone:         "5551212121",
		Email:         "avail-driver@example.com",
		Status:        database.DriverStatusOffline,
		IsAvailable:   true,
		IsActive:      true,
		VehicleType:   database.VehicleTypeCar,
		LicenseNumber: "LIC-AVAIL",
	}
	require.NoError(t, db.Create(driver).Error)

	// The driver must surface as available despite status != online.
	available, err := service.GetAvailableDrivers(business.ID)
	require.NoError(t, err)
	require.Len(t, available, 1, "an available+active driver with no active delivery must be assignable regardless of status column")
	assert.Equal(t, driver.ID, available[0].ID)

	// Guest checkout stamps payment_mode_stored=online (test biz has a
	// settlement addr). Online pending cannot be assigned until Accept+pay;
	// advance past intake so this test isolates driver availability.
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Where("id = ?", checkout.DeliveryOrder.ID).
		Update("status", database.DeliveryStatusReady).Error)

	// And assignment must succeed with no separate online step.
	require.NoError(t, service.AssignDriverByBusiness(business.ID, checkout.DeliveryOrder.ID, driver.ID))

	updatedDelivery, err := service.GetDeliveryOrderByBusiness(business.ID, checkout.DeliveryOrder.ID)
	require.NoError(t, err)
	require.NotNil(t, updatedDelivery.DriverID)
	assert.Equal(t, driver.ID, *updatedDelivery.DriverID)
	assert.Equal(t, database.DeliveryStatusAssigned, updatedDelivery.Status)

	// Now busy with a delivery: no longer available for a second assignment.
	available, err = service.GetAvailableDrivers(business.ID)
	require.NoError(t, err)
	assert.Empty(t, available, "a driver already carrying a delivery must drop out of the available pool")
}
