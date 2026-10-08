package services

import (
	"github.com/stdevmac/payverge/backend/internal/database"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateDriverByBusiness_MapClears verifies that:
// 1. A map-based update with an empty vehicle_plate clears the stored plate (not a no-op).
// 2. license_number is persisted through UpdateDriverByBusiness.
// 3. GetDriverByBusiness returns the freshly-persisted state.
func TestUpdateDriverByBusiness_MapClears(t *testing.T) {
	setupDeliveryTestDB(t)
	db := database.GetDB()
	svc := NewDeliveryService(db, nil)

	// Seed a driver with a plate and a license number.
	driver := database.DeliveryDriver{
		BusinessID:    100,
		Name:          "Test Driver",
		Phone:         "555-0100",
		LicenseNumber: "LIC-ORIG",
		VehicleType:   database.VehicleTypeCar,
		VehiclePlate:  "OLD-PLATE",
		IsAvailable:   true,
	}
	require.NoError(t, db.Create(&driver).Error)

	// Update: clear the plate and change the license number.
	updateMap := map[string]interface{}{
		"name":           "Test Driver",
		"phone":          "555-0100",
		"email":          "",
		"license_number": "LIC-NEW",
		"vehicle_type":   string(database.VehicleTypeCar),
		"vehicle_plate":  "", // explicitly clearing the plate
	}
	require.NoError(t, svc.UpdateDriverByBusiness(100, driver.ID, updateMap))

	// GetDriverByBusiness must reflect the persisted state.
	got, err := svc.GetDriverByBusiness(100, driver.ID)
	require.NoError(t, err)

	assert.Equal(t, "", got.VehiclePlate, "empty vehicle_plate should clear the stored value, not be a no-op")
	assert.Equal(t, "LIC-NEW", got.LicenseNumber, "updated license_number must persist")
}

// TestUpdateDriverByBusiness_LicenseNumberPersists covers the original R11 bug:
// license_number was absent from the handler binding struct so it was never sent
// to the service, and the struct-based GORM Updates silently dropped it anyway.
func TestUpdateDriverByBusiness_LicenseNumberPersists(t *testing.T) {
	setupDeliveryTestDB(t)
	db := database.GetDB()
	svc := NewDeliveryService(db, nil)

	driver := database.DeliveryDriver{
		BusinessID:  100,
		Name:        "Licensed Driver",
		Phone:       "555-0200",
		VehicleType: database.VehicleTypeScooter,
	}
	require.NoError(t, db.Create(&driver).Error)

	updateMap := map[string]interface{}{
		"name":           "Licensed Driver",
		"phone":          "555-0200",
		"email":          "",
		"license_number": "DL-ABC123",
		"vehicle_type":   string(database.VehicleTypeScooter),
		"vehicle_plate":  "XYZ-999",
	}
	require.NoError(t, svc.UpdateDriverByBusiness(100, driver.ID, updateMap))

	got, err := svc.GetDriverByBusiness(100, driver.ID)
	require.NoError(t, err)
	assert.Equal(t, "DL-ABC123", got.LicenseNumber)
	assert.Equal(t, "XYZ-999", got.VehiclePlate)
}

// TestUpdateDriverByBusiness_AvailabilityOnlyLeavesOtherFieldsUntouched pins the
// partial-update contract used by the availability toggle: a map containing ONLY
// is_available must not wipe name/email/license_number/vehicle_plate.
func TestUpdateDriverByBusiness_AvailabilityOnlyLeavesOtherFieldsUntouched(t *testing.T) {
	setupDeliveryTestDB(t)
	db := database.GetDB()
	svc := NewDeliveryService(db, nil)

	driver := database.DeliveryDriver{
		BusinessID:    100,
		Name:          "Toggle Driver",
		Phone:         "555-0300",
		Email:         "toggle@example.com",
		LicenseNumber: "LIC-KEEP",
		VehicleType:   database.VehicleTypeCar,
		VehiclePlate:  "KEEP-123",
		IsAvailable:   true,
	}
	require.NoError(t, db.Create(&driver).Error)

	// The availability toggle sends only is_available.
	require.NoError(t, svc.UpdateDriverByBusiness(100, driver.ID, map[string]interface{}{
		"is_available": false,
	}))

	got, err := svc.GetDriverByBusiness(100, driver.ID)
	require.NoError(t, err)
	assert.False(t, got.IsAvailable, "is_available must be updated")
	assert.Equal(t, "Toggle Driver", got.Name, "name must be untouched")
	assert.Equal(t, "555-0300", got.Phone, "phone must be untouched")
	assert.Equal(t, "toggle@example.com", got.Email, "email must be untouched")
	assert.Equal(t, "LIC-KEEP", got.LicenseNumber, "license_number must be untouched")
	assert.Equal(t, database.VehicleTypeCar, got.VehicleType, "vehicle_type must be untouched")
	assert.Equal(t, "KEEP-123", got.VehiclePlate, "vehicle_plate must be untouched")
}

// TestUpdateDriverByBusiness_AvailabilitySyncsStatusButPreservesBusy pins the
// availability-toggle status sync (R3-DL-1 display coherence): a status-bearing
// update (as the handler now builds) flips an idle driver's status to
// online/offline in step with is_available, but must NOT demote a driver who is
// currently on a delivery ("busy") — that status is owned by assign/release.
func TestUpdateDriverByBusiness_AvailabilitySyncsStatusButPreservesBusy(t *testing.T) {
	setupDeliveryTestDB(t)
	db := database.GetDB()
	svc := NewDeliveryService(db, nil)

	// Idle driver: toggling available on syncs status → online.
	idle := database.DeliveryDriver{
		BusinessID: 100, Name: "Idle", Phone: "1", VehicleType: database.VehicleTypeCar,
		IsAvailable: false, Status: database.DriverStatusOffline,
	}
	require.NoError(t, db.Create(&idle).Error)
	require.NoError(t, svc.UpdateDriverByBusiness(100, idle.ID, map[string]interface{}{
		"is_available": true,
		"status":       database.DriverStatusOnline,
	}))
	gotIdle, err := svc.GetDriverByBusiness(100, idle.ID)
	require.NoError(t, err)
	assert.True(t, gotIdle.IsAvailable)
	assert.Equal(t, database.DriverStatusOnline, gotIdle.Status, "idle driver status should sync to online")

	// Busy driver (mid-delivery): toggling availability must persist is_available
	// but keep status = busy.
	activeDelivery := uint(4242)
	busy := database.DeliveryDriver{
		BusinessID: 100, Name: "Busy", Phone: "2", VehicleType: database.VehicleTypeCar,
		IsAvailable: true, Status: database.DriverStatusBusy, CurrentDeliveryID: &activeDelivery,
	}
	require.NoError(t, db.Create(&busy).Error)
	require.NoError(t, svc.UpdateDriverByBusiness(100, busy.ID, map[string]interface{}{
		"is_available": false,
		"status":       database.DriverStatusOffline,
	}))
	gotBusy, err := svc.GetDriverByBusiness(100, busy.ID)
	require.NoError(t, err)
	assert.False(t, gotBusy.IsAvailable, "is_available must still persist for a busy driver")
	assert.Equal(t, database.DriverStatusBusy, gotBusy.Status, "a mid-delivery driver must NOT be demoted from busy")
}

// TestUpdateDriverByBusiness_NotFound confirms the not-found path returns an error.
func TestUpdateDriverByBusiness_NotFound(t *testing.T) {
	setupDeliveryTestDB(t)
	db := database.GetDB()
	svc := NewDeliveryService(db, nil)

	err := svc.UpdateDriverByBusiness(100, 99999, map[string]interface{}{
		"name":  "Ghost",
		"phone": "000",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
