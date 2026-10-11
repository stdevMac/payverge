package services

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestGuestDeliveryCheckoutRequestDoesNotBindCustomerIDFromPublicJSON(t *testing.T) {
	var request GuestDeliveryCheckoutRequest

	require.NoError(t, json.Unmarshal([]byte(`{
		"customer_id": 123,
		"customer_name": "Guest",
		"customer_phone": "5551112222",
		"delivery_address": {
			"street": "1 Main St",
			"city": "Dubai",
			"country": "AE"
		},
		"items": [{"menu_item_name": "Burger", "quantity": 1, "price": 12}]
	}`), &request))

	require.Nil(t, request.CustomerID)
}

func TestDeliveryServiceQuoteDeliveryUsesStructuredZonesAndMinimums(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-quote")
	createDeliverySettings(t, db, business.ID, nil)
	zone := createDeliveryZone(t, db, business.ID, "Downtown", nil)

	address := database.DeliveryAddress{
		Street:     "123 Main Street",
		City:       "Testville",
		State:      "CA",
		PostalCode: "12345",
		Country:    "US",
	}

	belowMinimum, err := service.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal:   18,
		DeliveryAddress: address,
	})
	require.NoError(t, err)
	assert.False(t, belowMinimum.Eligible)
	assert.Equal(t, "below_minimum", belowMinimum.ReasonCode)
	require.NotNil(t, belowMinimum.Zone)
	assert.Equal(t, zone.ID, belowMinimum.Zone.ID)
	assert.Equal(t, 20.0, belowMinimum.MinimumOrderAmount)
	assert.Equal(t, 7.0, belowMinimum.DeliveryFee)
	assert.True(t, belowMinimum.PartnerFallbackAvailable)

	quoted, err := service.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal:   25,
		DeliveryAddress: address,
	})
	require.NoError(t, err)
	assert.True(t, quoted.Eligible)
	assert.Equal(t, "quote_available", quoted.ReasonCode)
	require.NotNil(t, quoted.Zone)
	assert.Equal(t, zone.ID, quoted.Zone.ID)
	assert.Equal(t, 38, quoted.EstimatedTotalMinutes)
}

// TestQuoteDelivery_VenueOwnAddressSurvivesUnmatchableGeoJSONZone is the live
// Bodegón dinner-rush case (#892): settings show a CABA zone, but the stored
// boundaries are GeoJSON the matcher cannot read. Quoting the venue's OWN
// address (Defensa 1148 / C1065) must not return zone_unavailable.
func TestQuoteDelivery_VenueOwnAddressSurvivesUnmatchableGeoJSONZone(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "bodegon-own-address")
	business.Address = database.BusinessAddress{
		Street: "Defensa 1148, San Telmo", City: "Buenos Aires",
		State: "CABA", PostalCode: "C1065", Country: "AR",
	}
	require.NoError(t, db.Save(business).Error)

	createDeliverySettings(t, db, business.ID, func(s *database.DeliverySettings) {
		s.MinimumOrderAmount = 1500000 // AR$15.000 stored as cents
		s.FlatDeliveryFee = 290000
	})
	createDeliveryZone(t, db, business.ID, "CABA", func(z *database.DeliveryZone) {
		z.Boundaries = `{"type":"Polygon","coordinates":[]}`
		z.DeliveryFee = 290000
		z.MinimumOrderAmount = 1500000
	})

	own := database.DeliveryAddress{
		Street: "Defensa 1148", City: "CABA", PostalCode: "C1065", Country: "AR",
	}
	quote, err := service.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal:   60000,
		DeliveryAddress: own,
	})
	require.NoError(t, err)
	require.NotEqual(t, "zone_unavailable", quote.ReasonCode,
		"venue own address must not be zone_unavailable; got %q (%s)", quote.ReasonCode, quote.Message)
	require.True(t, quote.Eligible, "own address should be eligible, got %q (%s)", quote.ReasonCode, quote.Message)
	require.NotNil(t, quote.Zone)

	ushuaia, err := service.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal: 60000,
		DeliveryAddress: database.DeliveryAddress{
			Street: "San Martín 100", City: "Ushuaia", PostalCode: "V9410", Country: "AR",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "zone_unavailable", ushuaia.ReasonCode,
		"far-away addresses must still fail closed when the only zone is unmatchable GeoJSON")
}

// #892 repair: the own-address fallback must not override an explicit
// exclusion. The operator here covers C14* and deliberately left the venue's
// own C1065 out — quoting the venue address must still fail closed rather than
// borrowing the excluded zone's AR$99 fee.
func TestQuoteDelivery_VenueOwnAddressFailsClosedWhenZoneRulesExcludeIt(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "bodegon-excluded-postal")
	business.Address = database.BusinessAddress{
		Street: "Defensa 1148, San Telmo", City: "Buenos Aires",
		State: "CABA", PostalCode: "C1065", Country: "AR",
	}
	require.NoError(t, db.Save(business).Error)

	createDeliverySettings(t, db, business.ID, func(s *database.DeliverySettings) {
		s.MinimumOrderAmount = 1500000
		s.FlatDeliveryFee = 290000
	})
	createDeliveryZone(t, db, business.ID, "Palermo", func(z *database.DeliveryZone) {
		z.Boundaries = `{"postal_codes":["C14*"]}`
		z.DeliveryFee = 9900000
		z.MinimumOrderAmount = 0
	})

	quote, err := service.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal: 60000,
		DeliveryAddress: database.DeliveryAddress{
			Street: "Defensa 1148", City: "CABA", PostalCode: "C1065", Country: "AR",
		},
	})
	require.NoError(t, err)
	assert.False(t, quote.Eligible)
	assert.Equal(t, "zone_unavailable", quote.ReasonCode,
		"an explicitly excluded venue postal must not be quoted from the excluding zone")
}

// #892 repair: the neighbor at Defensa 114 is not the venue at Defensa 1148 and
// must not inherit the own-address fallback.
func TestQuoteDelivery_NeighborNumberDoesNotInheritVenueFallback(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "bodegon-neighbor-number")
	business.Address = database.BusinessAddress{
		Street: "Defensa 1148, San Telmo", City: "Buenos Aires",
		State: "CABA", PostalCode: "C1065", Country: "AR",
	}
	require.NoError(t, db.Save(business).Error)

	createDeliverySettings(t, db, business.ID, func(s *database.DeliverySettings) {
		s.MinimumOrderAmount = 1500000
		s.FlatDeliveryFee = 290000
	})
	createDeliveryZone(t, db, business.ID, "CABA", func(z *database.DeliveryZone) {
		z.Boundaries = `{"type":"Polygon","coordinates":[]}`
		z.DeliveryFee = 290000
		z.MinimumOrderAmount = 1500000
	})

	neighbor, err := service.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal: 60000,
		DeliveryAddress: database.DeliveryAddress{
			Street: "Defensa 114", City: "CABA", PostalCode: "C1065", Country: "AR",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "zone_unavailable", neighbor.ReasonCode,
		"Defensa 114 is not the venue's Defensa 1148")

	own, err := service.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal: 60000,
		DeliveryAddress: database.DeliveryAddress{
			Street: "Defensa 1148", City: "CABA", PostalCode: "C1065", Country: "AR",
		},
	})
	require.NoError(t, err)
	assert.True(t, own.Eligible, "the venue's own address must still quote (%s)", own.ReasonCode)
}

func TestDeliveryServicePublicFlowsRejectInactiveOrHiddenBusinesses(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)

	makeRequest := func() GuestDeliveryCheckoutRequest {
		return GuestDeliveryCheckoutRequest{
			CustomerName:  "Delivery Guest",
			CustomerPhone: "5557777777",
			CustomerEmail: "guest@example.com",
			DeliveryAddress: database.DeliveryAddress{
				Street:     "500 Market Street",
				City:       "Anywhere",
				State:      "CA",
				PostalCode: "99999",
				Country:    "US",
			},
			Items: []DeliveryCheckoutItemInput{
				{
					MenuItemName: "Burger",
					Quantity:     1,
					Price:        12,
				},
			},
		}
	}

	testCases := []struct {
		name   string
		mutate func(*database.Business)
	}{
		{
			name: "inactive business",
			mutate: func(b *database.Business) {
				b.IsActive = false
			},
		},
		{
			name: "business page disabled",
			mutate: func(b *database.Business) {
				b.BusinessPageEnabled = false
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			business := createTestHospitalityBusiness(t, db, "delivery-public-"+strings.ReplaceAll(tc.name, " ", "-"))
			createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
				ID:          "burger",
				Name:        "Burger",
				Price:       12,
				IsAvailable: true,
			})
			createDeliverySettings(t, db, business.ID, func(settings *database.DeliverySettings) {
				settings.MinimumOrderAmount = 10
			})
			createDeliveryZone(t, db, business.ID, "Everywhere", func(zone *database.DeliveryZone) {
				zone.Boundaries = "{}"
				zone.MinimumOrderAmount = 1000 // $10.00
			})

			tc.mutate(business)
			require.NoError(t, db.Save(business).Error)

			_, err := service.GetDeliverySettingsDTO(business.ID, true)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrDeliveryScopedNotFound)

			_, err = service.QuoteDelivery(business.ID, DeliveryQuoteRequest{
				OrderSubtotal:   25,
				DeliveryAddress: makeRequest().DeliveryAddress,
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrDeliveryScopedNotFound)

			_, err = service.GuestDeliveryCheckout(business.ID, makeRequest())
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrDeliveryScopedNotFound)
		})
	}
}

func TestDeliveryServiceGuestCheckoutCreatesBillOrderAndDelivery(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-checkout")
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID:          "burger",
		Name:        "Burger",
		Price:       12,
		IsAvailable: true,
	})
	createDeliverySettings(t, db, business.ID, func(settings *database.DeliverySettings) {
		settings.MinimumOrderAmount = 10
	})
	zone := createDeliveryZone(t, db, business.ID, "Everywhere", func(zone *database.DeliveryZone) {
		zone.Boundaries = `{"postal_codes":["99999"]}`
		zone.MinimumOrderAmount = 1000 // $10.00
		zone.EstimatedTime = 25
	})

	checkout, err := service.GuestDeliveryCheckout(business.ID, GuestDeliveryCheckoutRequest{
		CustomerName:  "Delivery Guest",
		CustomerPhone: "5557777777",
		CustomerEmail: "delivery@example.com",
		DeliveryAddress: database.DeliveryAddress{
			Street:     "500 Market Street",
			City:       "Anywhere",
			State:      "CA",
			PostalCode: "99999",
			Country:    "US",
		},
		Items: []DeliveryCheckoutItemInput{
			{
				MenuItemName: "Burger",
				Quantity:     2,
				Price:        12,
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, checkout.Bill)
	require.NotNil(t, checkout.Order)
	require.NotNil(t, checkout.DeliveryOrder)
	assert.Equal(t, business.ID, checkout.Bill.BusinessID)
	assert.Equal(t, business.ID, checkout.Order.BusinessID)
	assert.Equal(t, business.ID, checkout.DeliveryOrder.BusinessID)
	require.NotNil(t, checkout.DeliveryOrder.ZoneID)
	assert.Equal(t, zone.ID, *checkout.DeliveryOrder.ZoneID)
	assert.Contains(t, checkout.TrackingURL, checkout.DeliveryOrder.DeliveryNumber)

	var deliveryCount int64
	var historyCount int64
	require.NoError(t, db.Model(&database.DeliveryOrder{}).Where("business_id = ?", business.ID).Count(&deliveryCount).Error)
	require.NoError(t, db.Model(&database.DeliveryStatusHistory{}).Where("delivery_order_id = ?", checkout.DeliveryOrder.ID).Count(&historyCount).Error)
	assert.EqualValues(t, 1, deliveryCount)
	assert.EqualValues(t, 1, historyCount)
}

func TestDeliveryServiceGetDeliveryOrderByBusinessRejectsWrongBusiness(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	businessOne := createTestHospitalityBusiness(t, db, "delivery-scope-a")
	businessTwo := createTestHospitalityBusiness(t, db, "delivery-scope-b")
	createTestHospitalityMenu(t, db, businessOne.ID, database.MenuItem{
		ID:          "fries",
		Name:        "Fries",
		Price:       11,
		IsAvailable: true,
	})
	createDeliverySettings(t, db, businessOne.ID, nil)
	createDeliveryZone(t, db, businessOne.ID, "Scoped Zone", func(zone *database.DeliveryZone) {
		zone.Boundaries = `{"postal_codes":["00000"]}`
		zone.MinimumOrderAmount = 500 // $5.00
	})

	checkout, err := service.GuestDeliveryCheckout(businessOne.ID, GuestDeliveryCheckoutRequest{
		CustomerName:  "Scope Guest",
		CustomerPhone: "5558888888",
		CustomerEmail: "scope@example.com",
		DeliveryAddress: database.DeliveryAddress{
			Street:     "1 Scope Street",
			City:       "Anywhere",
			PostalCode: "00000",
			Country:    "US",
		},
		Items: []DeliveryCheckoutItemInput{
			{
				MenuItemName: "Fries",
				Quantity:     1,
				Price:        11,
			},
		},
	})
	require.NoError(t, err)

	_, err = service.GetDeliveryOrderByBusiness(businessTwo.ID, checkout.DeliveryOrder.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delivery order not found")
}

func TestDeliveryServiceGuestCheckoutRejectsCustomerOutsideBusinessConnection(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-customer-scope")
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID:          "burger",
		Name:        "Burger",
		Price:       12,
		IsAvailable: true,
	})
	createDeliverySettings(t, db, business.ID, func(settings *database.DeliverySettings) {
		settings.MinimumOrderAmount = 10
	})
	createDeliveryZone(t, db, business.ID, "Everywhere", func(zone *database.DeliveryZone) {
		zone.Boundaries = "{}"
		zone.MinimumOrderAmount = 1000 // $10.00
	})

	customer := &database.Customer{
		Email:    "unrelated-delivery@example.com",
		Name:     "Unrelated Delivery Customer",
		IsActive: true,
	}
	require.NoError(t, db.Create(customer).Error)

	_, err := service.GuestDeliveryCheckout(business.ID, GuestDeliveryCheckoutRequest{
		CustomerID:    &customer.ID,
		CustomerName:  "Delivery Guest",
		CustomerPhone: "5557777777",
		CustomerEmail: "guest@example.com",
		DeliveryAddress: database.DeliveryAddress{
			Street:     "500 Market Street",
			City:       "Anywhere",
			State:      "CA",
			PostalCode: "99999",
			Country:    "US",
		},
		Items: []DeliveryCheckoutItemInput{
			{
				MenuItemName: "Burger",
				Quantity:     1,
				Price:        12,
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "customer is not connected to this business")

	var billCount int64
	var deliveryCount int64
	require.NoError(t, db.Model(&database.Bill{}).Where("business_id = ?", business.ID).Count(&billCount).Error)
	require.NoError(t, db.Model(&database.DeliveryOrder{}).Where("business_id = ?", business.ID).Count(&deliveryCount).Error)
	assert.Zero(t, billCount)
	assert.Zero(t, deliveryCount)
}

func TestDeliveryServiceAssignDriverByBusinessAssignsOnlyScopedOrders(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-driver")
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID:          "pizza",
		Name:        "Pizza",
		Price:       16,
		IsAvailable: true,
	})
	createDeliverySettings(t, db, business.ID, nil)
	createDeliveryZone(t, db, business.ID, "Driver Zone", func(zone *database.DeliveryZone) {
		zone.Boundaries = `{"postal_codes":["00000"]}`
		zone.MinimumOrderAmount = 500 // $5.00
	})

	checkout, err := service.GuestDeliveryCheckout(business.ID, GuestDeliveryCheckoutRequest{
		CustomerName:  "Driver Guest",
		CustomerPhone: "5559999999",
		CustomerEmail: "driver-guest@example.com",
		DeliveryAddress: database.DeliveryAddress{
			Street:     "42 Driver Road",
			City:       "Anywhere",
			PostalCode: "00000",
			Country:    "US",
		},
		Items: []DeliveryCheckoutItemInput{
			{
				MenuItemName: "Pizza",
				Quantity:     1,
				Price:        16,
			},
		},
	})
	require.NoError(t, err)

	driver := &database.DeliveryDriver{
		BusinessID:    business.ID,
		Name:          "Pat Driver",
		Phone:         "5551212121",
		Email:         "driver@example.com",
		Status:        database.DriverStatusOnline,
		IsAvailable:   true,
		VehicleType:   database.VehicleTypeCar,
		LicenseNumber: "LIC-1",
	}
	require.NoError(t, db.Create(driver).Error)

	// Online pending (default when settlement addr is set) cannot be assigned
	// until Accept+pay; put the delivery in a fulfillment-ready state so this
	// test isolates business-scoped assign ownership.
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Where("id = ?", checkout.DeliveryOrder.ID).
		Update("status", database.DeliveryStatusReady).Error)

	require.NoError(t, service.AssignDriverByBusiness(business.ID, checkout.DeliveryOrder.ID, driver.ID))

	updatedDelivery, err := service.GetDeliveryOrderByBusiness(business.ID, checkout.DeliveryOrder.ID)
	require.NoError(t, err)
	require.NotNil(t, updatedDelivery.DriverID)
	assert.Equal(t, driver.ID, *updatedDelivery.DriverID)
	assert.Equal(t, database.DeliveryStatusAssigned, updatedDelivery.Status)

	var updatedDriver database.DeliveryDriver
	require.NoError(t, db.First(&updatedDriver, driver.ID).Error)
	assert.Equal(t, database.DriverStatusBusy, updatedDriver.Status)
	require.NotNil(t, updatedDriver.CurrentDeliveryID)
	assert.Equal(t, checkout.DeliveryOrder.ID, *updatedDriver.CurrentDeliveryID)
}

func TestDeliveryServiceAssignDriverByBusinessRejectsCrossBusinessDriver(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	businessOne := createTestHospitalityBusiness(t, db, "delivery-driver-a")
	businessTwo := createTestHospitalityBusiness(t, db, "delivery-driver-b")
	createTestHospitalityMenu(t, db, businessOne.ID, database.MenuItem{
		ID:          "pizza",
		Name:        "Pizza",
		Price:       16,
		IsAvailable: true,
	})
	createDeliverySettings(t, db, businessOne.ID, nil)
	createDeliveryZone(t, db, businessOne.ID, "Driver Zone", func(zone *database.DeliveryZone) {
		zone.Boundaries = `{"postal_codes":["00000"]}`
		zone.MinimumOrderAmount = 500 // $5.00
	})

	checkout, err := service.GuestDeliveryCheckout(businessOne.ID, GuestDeliveryCheckoutRequest{
		CustomerName:  "Driver Guest",
		CustomerPhone: "5550000000",
		CustomerEmail: "driver-guest2@example.com",
		DeliveryAddress: database.DeliveryAddress{
			Street:     "42 Driver Road",
			City:       "Anywhere",
			PostalCode: "00000",
			Country:    "US",
		},
		Items: []DeliveryCheckoutItemInput{
			{
				MenuItemName: "Pizza",
				MenuItemID:   "pizza",
				Quantity:     1,
				Price:        16,
			},
		},
	})
	require.NoError(t, err)

	// Past intake so the ownership/driver-scope check is what fails, not the
	// online-pending payment gate.
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Where("id = ?", checkout.DeliveryOrder.ID).
		Update("status", database.DeliveryStatusReady).Error)

	driver := &database.DeliveryDriver{
		BusinessID:    businessTwo.ID,
		Name:          "Other Driver",
		Phone:         "5553333333",
		Email:         "other@example.com",
		Status:        database.DriverStatusOnline,
		IsAvailable:   true,
		VehicleType:   database.VehicleTypeCar,
		LicenseNumber: "LIC-2",
	}
	require.NoError(t, db.Create(driver).Error)

	err = service.AssignDriverByBusiness(businessOne.ID, checkout.DeliveryOrder.ID, driver.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "driver not found")
}

func TestDeliveryServiceGuestCheckoutUsesAuthoritativeMenuPricing(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-authoritative-price")
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID:          "burger",
		Name:        "Burger",
		Price:       18,
		IsAvailable: true,
		Options: []database.MenuItemOption{
			{ID: "cheese", Name: "Extra Cheese", PriceChange: 3},
		},
	})
	createDeliverySettings(t, db, business.ID, func(settings *database.DeliverySettings) {
		settings.MinimumOrderAmount = 10
		settings.FlatDeliveryFee = 5
	})
	createDeliveryZone(t, db, business.ID, "Everywhere", func(zone *database.DeliveryZone) {
		zone.Boundaries = `{"postal_codes":["99999"]}`
		zone.MinimumOrderAmount = 1000 // $10.00
		zone.DeliveryFee = 500         // $5.00
	})

	checkout, err := service.GuestDeliveryCheckout(business.ID, GuestDeliveryCheckoutRequest{
		CustomerName:  "Delivery Guest",
		CustomerPhone: "5551231234",
		CustomerEmail: "auth-price@example.com",
		DeliveryAddress: database.DeliveryAddress{
			Street:     "500 Market Street",
			City:       "Anywhere",
			State:      "CA",
			PostalCode: "99999",
			Country:    "US",
		},
		Items: []DeliveryCheckoutItemInput{
			{
				MenuItemName: "Burger",
				MenuItemID:   "burger",
				Quantity:     2,
				Price:        1,
				Options: []database.MenuItemOption{
					{ID: "cheese", Name: "Extra Cheese", PriceChange: -100},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, checkout.Bill)
	assert.Equal(t, int64(4200), checkout.Bill.Subtotal)

	order, items, err := database.GetOrderByID(checkout.Order.ID)
	require.NoError(t, err)
	require.NotNil(t, order)
	require.Len(t, items, 1)
	assert.Equal(t, 18.0, items[0].Price)
	assert.Equal(t, 42.0, items[0].Subtotal)
	require.Len(t, items[0].Options, 1)
	assert.Equal(t, 3.0, items[0].Options[0].PriceChange)
}

func TestDeliveryServiceGuestCheckoutRejectsNegativeTip(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-negative-tip")
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID:          "burger",
		Name:        "Burger",
		Price:       12,
		IsAvailable: true,
	})
	createDeliverySettings(t, db, business.ID, nil)
	createDeliveryZone(t, db, business.ID, "Everywhere", func(zone *database.DeliveryZone) {
		zone.Boundaries = "{}"
	})

	_, err := service.GuestDeliveryCheckout(business.ID, GuestDeliveryCheckoutRequest{
		CustomerName:  "Tip Guest",
		CustomerPhone: "5557777777",
		CustomerEmail: "tip@example.com",
		DriverTip:     -5,
		DeliveryAddress: database.DeliveryAddress{
			Street:     "500 Market Street",
			City:       "Anywhere",
			State:      "CA",
			PostalCode: "99999",
			Country:    "US",
		},
		Items: []DeliveryCheckoutItemInput{
			{
				MenuItemName: "Burger",
				MenuItemID:   "burger",
				Quantity:     1,
				Price:        12,
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "driver tip cannot be negative")
}

func TestDeliveryServiceCreateDeliveryOrderRejectsBillOutsideBusiness(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	businessOne := createTestHospitalityBusiness(t, db, "delivery-create-a")
	businessTwo := createTestHospitalityBusiness(t, db, "delivery-create-b")

	bill := &database.Bill{
		BusinessID: businessOne.ID,
		BillNumber: "B-delivery-create",
		Status:     database.BillStatusOpen,
		Items:      "[]",
	}
	require.NoError(t, db.Create(bill).Error)

	_, err := service.CreateDeliveryOrder(CreateDeliveryOrderRequest{
		BusinessID:      businessTwo.ID,
		BillID:          bill.ID,
		DeliveryType:    database.DeliveryTypeInHouse,
		CustomerName:    "Cross Scope",
		CustomerPhone:   "5550001111",
		DeliveryAddress: database.DeliveryAddress{Street: "1 Test", City: "Scope", Country: "US"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bill not found")
}

func TestDeliveryServiceUpdateDriverByBusinessRejectsWrongBusiness(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	businessOne := createTestHospitalityBusiness(t, db, "delivery-update-a")
	businessTwo := createTestHospitalityBusiness(t, db, "delivery-update-b")

	driver := &database.DeliveryDriver{
		BusinessID:    businessOne.ID,
		Name:          "Scoped Driver",
		Phone:         "5552222222",
		Email:         "driver@example.com",
		Status:        database.DriverStatusOnline,
		IsAvailable:   true,
		VehicleType:   database.VehicleTypeCar,
		LicenseNumber: "LIC-3",
	}
	require.NoError(t, db.Create(driver).Error)

	err := service.UpdateDriverByBusiness(businessTwo.ID, driver.ID, struct {
		Name string `json:"name"`
	}{
		Name: "Hijacked Driver",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "driver not found")

	var persisted database.DeliveryDriver
	require.NoError(t, db.First(&persisted, driver.ID).Error)
	assert.Equal(t, "Scoped Driver", persisted.Name)
}

func TestDeliveryServiceDeleteDriverByBusinessRejectsWrongBusiness(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	businessOne := createTestHospitalityBusiness(t, db, "delivery-delete-a")
	businessTwo := createTestHospitalityBusiness(t, db, "delivery-delete-b")

	driver := &database.DeliveryDriver{
		BusinessID:    businessOne.ID,
		Name:          "Scoped Driver",
		Phone:         "5554444444",
		Email:         "driver-delete@example.com",
		Status:        database.DriverStatusOnline,
		IsAvailable:   true,
		VehicleType:   database.VehicleTypeCar,
		LicenseNumber: "LIC-4",
	}
	require.NoError(t, db.Create(driver).Error)

	err := service.DeleteDriverByBusiness(businessTwo.ID, driver.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "driver not found")

	var count int64
	require.NoError(t, db.Model(&database.DeliveryDriver{}).Where("id = ?", driver.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestDeliveryServiceGetDeliverySettingsDTOTreatsWhitespaceEmptyPartnerLinksAsUnavailable(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-partner-links")
	createDeliverySettings(t, db, business.ID, func(settings *database.DeliverySettings) {
		settings.ExternalPartnerLinks = database.JSONRawMessage("[ ]")
	})

	settings, err := service.GetDeliverySettingsDTO(business.ID, true)
	require.NoError(t, err)
	assert.False(t, settings.PartnerFallbackAvailable)
}

// TestGetBusinessDeliveriesPagination checks that limit/offset slicing and total count are correct.
// #897 security: a stored javascript:/data: partner URL must never reach a
// guest surface. The quote and the public settings DTO are the two projections
// the unauthenticated storefront reads.
func TestDeliveryServicePartnerLinksDropUnsafeSchemesOnPublicProjections(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-partner-xss")
	createDeliverySettings(t, db, business.ID, func(settings *database.DeliverySettings) {
		settings.ExternalPartnerLinks = database.JSONRawMessage(`[` +
			`{"name":"Evil","url":"javascript:alert(1)"},` +
			`{"name":"DataEvil","url":"data:text/html,<script>alert(1)</script>"},` +
			`{"name":"NoScheme","url":"//evil.example/x"},` +
			`{"name":"Rappi","url":"https://www.rappi.com.ar","icon_url":"javascript:alert(2)"}` +
			`]`)
	})
	createDeliveryZone(t, db, business.ID, "Downtown", nil)

	dto, err := service.GetDeliverySettingsDTO(business.ID, true)
	require.NoError(t, err)
	assert.NotContains(t, string(dto.ExternalPartnerLinks), "javascript:")
	assert.NotContains(t, string(dto.ExternalPartnerLinks), "data:text/html")
	var settingsLinks []map[string]any
	require.NoError(t, json.Unmarshal(dto.ExternalPartnerLinks, &settingsLinks))
	require.Len(t, settingsLinks, 1)
	assert.Equal(t, "Rappi", settingsLinks[0]["name"])
	_, hasIcon := settingsLinks[0]["icon_url"]
	assert.False(t, hasIcon, "unsafe icon_url is stripped")

	quote, err := service.QuoteDelivery(business.ID, DeliveryQuoteRequest{
		OrderSubtotal: 1,
		DeliveryAddress: database.DeliveryAddress{
			Street: "Nowhere 1", City: "Nowhere", PostalCode: "99999", Country: "US",
		},
	})
	require.NoError(t, err)
	assert.NotContains(t, string(quote.ExternalPartnerLinks), "javascript:")
	assert.NotContains(t, string(quote.ExternalPartnerLinks), "data:text/html")
	var quoteLinks []map[string]any
	require.NoError(t, json.Unmarshal(quote.ExternalPartnerLinks, &quoteLinks))
	require.Len(t, quoteLinks, 1)
	assert.Equal(t, "https://www.rappi.com.ar", quoteLinks[0]["url"])
	assert.True(t, quote.PartnerFallbackAvailable, "one real partner survives")
}

// #897 security: a settings save that carries an unsafe partner URL must not
// persist it, so no later read can resurrect the href.
func TestDeliveryServiceUpdateSettingsDropsUnsafePartnerLinkURLs(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-partner-write")
	// Third-party only: the zone snapshot is irrelevant to the partner-link gate.
	createDeliverySettings(t, db, business.ID, func(settings *database.DeliverySettings) {
		settings.InHouseDeliveryEnabled = false
	})

	links := json.RawMessage(`[{"name":"Evil","url":"javascript:alert(1)"},{"name":"Rappi","url":"https://www.rappi.com.ar"}]`)
	_, err := service.UpdateDeliverySettingsDTO(business.ID, UpdateDeliverySettingsInput{
		ExternalPartnerLinks: &links,
	})
	require.NoError(t, err)

	var stored database.DeliverySettings
	require.NoError(t, db.Where("business_id = ?", business.ID).First(&stored).Error)
	assert.NotContains(t, string(stored.ExternalPartnerLinks), "javascript:")
	assert.Contains(t, string(stored.ExternalPartnerLinks), "https://www.rappi.com.ar")
}

// #897 security: third_party_enabled cannot be satisfied by a link no guest
// surface will ever render.
func TestDeliveryServicePartnerLinkCountIgnoresUnsafeURLs(t *testing.T) {
	count, err := countPartnerLinks(json.RawMessage(`[{"name":"Evil","url":"javascript:alert(1)"},{"name":"Rappi","url":"https://www.rappi.com.ar"}]`))
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestGetBusinessDeliveriesPagination(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := createTestHospitalityBusiness(t, db, "delivery-pagination")
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID:          "pizza",
		Name:        "Pizza",
		Price:       15,
		IsAvailable: true,
	})
	createDeliverySettings(t, db, business.ID, nil)
	createDeliveryZone(t, db, business.ID, "Anywhere", func(zone *database.DeliveryZone) {
		zone.Boundaries = `{"postal_codes":["10001"]}`
		zone.MinimumOrderAmount = 500 // $5.00
		zone.EstimatedTime = 20
	})

	// Create 5 delivery orders.
	for i := 0; i < 5; i++ {
		_, err := service.GuestDeliveryCheckout(business.ID, GuestDeliveryCheckoutRequest{
			CustomerName:  "Paginated Guest",
			CustomerPhone: "5550000000",
			CustomerEmail: "pg@example.com",
			DeliveryAddress: database.DeliveryAddress{
				Street:     "1 Test Lane",
				City:       "Testville",
				State:      "CA",
				PostalCode: "10001",
				Country:    "US",
			},
			Items: []DeliveryCheckoutItemInput{
				{MenuItemName: "Pizza", Quantity: 1, Price: 15},
			},
		})
		require.NoError(t, err)
	}

	// Page 1: first 3 of 5.
	page1, err := service.GetBusinessDeliveries(business.ID, DeliveryListParams{Limit: 3, Offset: 0})
	require.NoError(t, err)
	assert.EqualValues(t, 5, page1.Total)
	assert.Len(t, page1.Deliveries, 3)
	assert.True(t, page1.HasMore)

	// Page 2: last 2 of 5.
	page2, err := service.GetBusinessDeliveries(business.ID, DeliveryListParams{Limit: 3, Offset: 3})
	require.NoError(t, err)
	assert.EqualValues(t, 5, page2.Total)
	assert.Len(t, page2.Deliveries, 2)
	assert.False(t, page2.HasMore)

	// Default params: all 5 returned (limit defaults to 100).
	all, err := service.GetBusinessDeliveries(business.ID, DeliveryListParams{})
	require.NoError(t, err)
	assert.EqualValues(t, 5, all.Total)
	assert.Len(t, all.Deliveries, 5)
	assert.False(t, all.HasMore)

	// Limit clamped to max.
	clamped, err := service.GetBusinessDeliveries(business.ID, DeliveryListParams{Limit: 9999})
	require.NoError(t, err)
	assert.Len(t, clamped.Deliveries, 5)
}

// TestComputeDeliveryFeeFreeZoneOverridesFlatFee guards against silently
// overcharging guests in a deliberately free-delivery zone. A matched zone whose
// DeliveryFee is $0 is authoritative ("lowest fee wins"); it must NOT fall back
// to settings.FlatDeliveryFee.
func TestComputeDeliveryFeeFreeZoneOverridesFlatFee(t *testing.T) {
	svc := &DeliveryService{}
	settings := &database.DeliverySettings{
		FlatDeliveryFee:     800, // $8.00 — must not be billed when a $0 zone matches
		FreeDeliveryMinimum: 0,
	}
	freeZone := &DeliveryZoneDTO{
		Name:        "Free zone",
		DeliveryFee: 0, // deliberately free
		IsActive:    true,
	}

	fee := svc.computeDeliveryFee(settings, freeZone, 25.00)

	assert.Equal(t, 0.0, fee, "matched $0 zone must bill $0, not the flat fee")
}

// TestComputeDeliveryFeePaidZoneOverridesFlatFee documents the unchanged
// behavior for a positive zone fee: the zone fee wins over the flat fee.
func TestComputeDeliveryFeePaidZoneOverridesFlatFee(t *testing.T) {
	svc := &DeliveryService{}
	settings := &database.DeliverySettings{FlatDeliveryFee: 800, FreeDeliveryMinimum: 0}
	zone := &DeliveryZoneDTO{Name: "Paid zone", DeliveryFee: 3.50, IsActive: true}

	fee := svc.computeDeliveryFee(settings, zone, 25.00)

	assert.Equal(t, 3.50, fee, "matched zone fee must override the flat fee")
}

// TestComputeDeliveryFeeNoZoneUsesFlatFee keeps the fall-back path covered: with
// no matched zone, the configured flat fee applies.
func TestComputeDeliveryFeeNoZoneUsesFlatFee(t *testing.T) {
	svc := &DeliveryService{}
	settings := &database.DeliverySettings{FlatDeliveryFee: 800, FreeDeliveryMinimum: 0}

	fee := svc.computeDeliveryFee(settings, nil, 25.00)

	assert.Equal(t, 8.00, fee, "with no matched zone the flat fee applies")
}
