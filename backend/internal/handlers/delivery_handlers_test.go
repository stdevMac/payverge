package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertBusinessEvent(t *testing.T, eventsCh <-chan events.BusinessEvent, eventType string) events.BusinessEvent {
	t.Helper()

	select {
	case event := <-eventsCh:
		assert.Equal(t, eventType, event.Type)
		return event
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s event", eventType)
	}
	return events.BusinessEvent{}
}

func createTestDeliveryOrder(t *testing.T, businessID, billID uint) *database.DeliveryOrder {
	t.Helper()

	deliveryOrder, err := services.NewDeliveryService(database.GetDB(), nil).CreateDeliveryOrder(services.CreateDeliveryOrderRequest{
		BusinessID:          businessID,
		BillID:              billID,
		CustomerName:        "Delivery Customer",
		CustomerPhone:       "5551112222",
		DeliveryAddress:     database.DeliveryAddress{Street: "1 Main", City: "Anywhere", Country: "US"},
		PickupLocation:      database.Location{Latitude: 1, Longitude: 1},
		DropoffLocation:     database.Location{Latitude: 2, Longitude: 2},
		DeliveryFee:         4.99,
		ContactlessDelivery: true,
	})
	require.NoError(t, err)
	return deliveryOrder
}

// claimTestDelivery is a test helper: mutating dispatch routes require a fresh
// claim (L4-8). Owner/manager principal is used so tests stay short.
func claimTestDelivery(t *testing.T, businessID, deliveryID, staffID uint) {
	t.Helper()
	sid := staffID
	_, _, err := services.NewDeliveryService(database.GetDB(), nil).ClaimDeliveryOrder(
		businessID, deliveryID,
		services.ClaimActor{StaffID: &sid, Name: "Test Staff", Role: "server"},
		false,
	)
	require.NoError(t, err)
}

func TestCreateDeliveryOrder_RequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	NewDeliveryHandler(nil).CreateDeliveryOrder(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateDeliveryOrder_ReturnsNotFoundForBillOutsideBusiness(t *testing.T) {
	setupHandlerTestDB(t)

	sourceBusiness := createTestBusiness(t)
	targetBusiness := createTestBusiness(t)
	bill := createTestBill(t, sourceBusiness.ID)

	body, err := json.Marshal(services.CreateDeliveryOrderRequest{
		BillID:          bill.ID,
		CustomerName:    "Delivery Customer",
		CustomerPhone:   "5551112222",
		DeliveryAddress: database.DeliveryAddress{Street: "1 Main", City: "Anywhere", Country: "US"},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", targetBusiness.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.CreateDeliveryOrder(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "bill not found for business")
	assert.NotContains(t, w.Body.String(), "delivery scoped resource not found")
}

func TestCreateDeliveryOrder_ReturnsBadRequestForValidationErrors(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)

	body, err := json.Marshal(services.CreateDeliveryOrderRequest{
		BillID:          bill.ID,
		CustomerName:    "Delivery Customer",
		CustomerPhone:   "5551112222",
		DeliveryAddress: database.DeliveryAddress{Street: "1 Main", City: "Anywhere", Country: "US"},
		DeliveryFee:     -1,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.CreateDeliveryOrder(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "delivery fee cannot be negative")
	assert.NotContains(t, w.Body.String(), "delivery validation failed")
}

func TestCreateDeliveryOrder_AcceptsBusinessSlug(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	business.BusinessId = fmt.Sprintf("delivery-slug-%d", time.Now().UnixNano())
	require.NoError(t, database.GetDB().Save(business).Error)
	bill := createTestBill(t, business.ID)

	body, err := json.Marshal(services.CreateDeliveryOrderRequest{
		BillID:          bill.ID,
		CustomerName:    "Delivery Customer",
		CustomerPhone:   "5551112222",
		DeliveryAddress: database.DeliveryAddress{Street: "1 Main", City: "Anywhere", Country: "US"},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.CreateDeliveryOrder(c)

	require.Equal(t, http.StatusCreated, w.Code)
}

func TestCreateDeliveryOrder_PublishesDeliveryCreatedEvent(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	eventsCh, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	body, err := json.Marshal(services.CreateDeliveryOrderRequest{
		BillID:          bill.ID,
		CustomerName:    "Delivery Customer",
		CustomerPhone:   "5551112222",
		DeliveryAddress: database.DeliveryAddress{Street: "1 Main", City: "Anywhere", Country: "US"},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.CreateDeliveryOrder(c)

	require.Equal(t, http.StatusCreated, w.Code)
	assertBusinessEvent(t, eventsCh, "delivery.created")

	var alert database.OperationalAlert
	require.NoError(t, database.GetDB().
		Where("business_id = ? AND alert_type = ? AND resource_type = ?", business.ID, database.OperationalAlertTypeDeliveryNew, database.OperationalAlertResourceTypeDelivery).
		First(&alert).Error)
	assert.Equal(t, database.OperationalAlertStatusOpen, alert.Status)
}

func TestUpdateDeliveryStatus_PublishesDeliveryUpdatedEvent(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	deliveryOrder := createTestDeliveryOrder(t, business.ID, bill.ID)
	alert := &database.OperationalAlert{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeDeliveryNew,
		ResourceType: database.OperationalAlertResourceTypeDelivery,
		ResourceID:   int64(deliveryOrder.ID),
		Status:       database.OperationalAlertStatusOpen,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "New delivery",
		LastEventAt:  time.Now(),
	}
	require.NoError(t, database.GetDB().Create(alert).Error)
	eventsCh, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	body := bytes.NewBufferString(`{"status":"ready"}`)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "delivery_id", Value: fmt.Sprintf("%d", deliveryOrder.ID)},
	}
	claimTestDelivery(t, business.ID, deliveryOrder.ID, 9)
	c.Request = httptest.NewRequest(http.MethodPut, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_id", uint(9))
	c.Set("staff_name", "Test Staff")
	c.Set("staff_role", "server")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.UpdateDeliveryStatus(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertBusinessEvent(t, eventsCh, "delivery.updated")

	var refreshed database.OperationalAlert
	require.NoError(t, database.GetDB().First(&refreshed, alert.ID).Error)
	assert.Equal(t, database.OperationalAlertStatusResolved, refreshed.Status)
	assert.Equal(t, "staff:9", refreshed.ClaimedByName)
	require.NotNil(t, refreshed.ResolvedAt)
}

func TestUpdateDeliveryStatus_UsesAuthenticatedActorForHistory(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	deliveryOrder := createTestDeliveryOrder(t, business.ID, bill.ID)
	claimTestDelivery(t, business.ID, deliveryOrder.ID, 9)

	body := bytes.NewBufferString(`{"status":"ready","changed_by":"owner:spoofed"}`)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "delivery_id", Value: fmt.Sprintf("%d", deliveryOrder.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPut, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_id", uint(9))
	c.Set("staff_name", "Test Staff")
	c.Set("staff_role", "server")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.UpdateDeliveryStatus(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var history database.DeliveryStatusHistory
	require.NoError(t, database.GetDB().
		Where("delivery_order_id = ? AND status = ?", deliveryOrder.ID, database.DeliveryStatusReady).
		First(&history).Error)
	assert.Equal(t, "staff:9", history.ChangedBy)
}

func TestAssignDriver_PublishesDeliveryUpdatedEvent(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	deliveryOrder := createTestDeliveryOrder(t, business.ID, bill.ID)
	claimTestDelivery(t, business.ID, deliveryOrder.ID, 9)
	driver := &database.DeliveryDriver{
		BusinessID:  business.ID,
		Name:        "Dana Driver",
		Phone:       "5552223333",
		Status:      database.DriverStatusOnline,
		IsAvailable: true,
		IsActive:    true,
		VehicleType: database.VehicleTypeCar,
	}
	require.NoError(t, database.GetDB().Create(driver).Error)
	eventsCh, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	body := bytes.NewBufferString(fmt.Sprintf(`{"driver_id":%d}`, driver.ID))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "delivery_id", Value: fmt.Sprintf("%d", deliveryOrder.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_id", uint(9))
	c.Set("staff_name", "Test Staff")
	c.Set("staff_role", "server")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.AssignDriver(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertBusinessEvent(t, eventsCh, "delivery.updated")
}

func TestGetBusinessDeliveries_ProjectsBillTotalCurrencyAndCoords(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	business.DefaultCurrency = "ARS"
	lat, lng := -34.6165, -58.3719
	business.Latitude = &lat
	business.Longitude = &lng
	require.NoError(t, database.GetDB().Save(business).Error)

	bill := createTestBill(t, business.ID)
	bill.TotalAmount = 15750 // $157.50
	require.NoError(t, database.GetDB().Save(bill).Error)

	delivery, err := services.NewDeliveryService(database.GetDB(), nil).CreateDeliveryOrder(services.CreateDeliveryOrderRequest{
		BusinessID:      business.ID,
		BillID:          bill.ID,
		CustomerName:    "List Guest",
		CustomerPhone:   "5551112222",
		DeliveryAddress: database.DeliveryAddress{Street: "Defensa 1148", City: "CABA", PostalCode: "C1065", Country: "AR"},
		PickupLocation:  database.Location{}, // 0,0 — must hydrate from venue coords
		DropoffLocation: database.Location{Latitude: -34.6037, Longitude: -58.3816},
		DeliveryFee:     29.00,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).GetBusinessDeliveries(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var result map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	rows, ok := result["deliveries"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, rows)
	row, ok := rows[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(delivery.ID), row["id"])
	assert.InDelta(t, 186.50, row["total"], 0.001, "list must emit bill total as dollars (incl. delivery fee), body=%s", w.Body.String())
	assert.Equal(t, "ARS", row["currency"], "list must emit business currency, body=%s", w.Body.String())
	pickup, _ := row["pickup_location"].(map[string]any)
	require.NotNil(t, pickup)
	assert.InDelta(t, lat, pickup["latitude"], 0.0001, "zero pickup must hydrate from venue coordinates")
	assert.InDelta(t, lng, pickup["longitude"], 0.0001)
	drop, _ := row["dropoff_location"].(map[string]any)
	require.NotNil(t, drop)
	assert.InDelta(t, -34.6037, drop["latitude"], 0.0001)
	assert.InDelta(t, -58.3816, drop["longitude"], 0.0001)
}

func TestGetDeliveryOrder_ProjectsBillTotalCurrencyAndCoords(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	business.DefaultCurrency = "ARS"
	lat, lng := -34.6165, -58.3719
	business.Latitude = &lat
	business.Longitude = &lng
	require.NoError(t, database.GetDB().Save(business).Error)

	bill := createTestBill(t, business.ID)
	bill.TotalAmount = 15750
	require.NoError(t, database.GetDB().Save(bill).Error)

	delivery, err := services.NewDeliveryService(database.GetDB(), nil).CreateDeliveryOrder(services.CreateDeliveryOrderRequest{
		BusinessID:      business.ID,
		BillID:          bill.ID,
		CustomerName:    "Detail Guest",
		CustomerPhone:   "5551112222",
		DeliveryAddress: database.DeliveryAddress{Street: "Defensa 1148", City: "CABA", Country: "AR"},
		PickupLocation:  database.Location{},
		DropoffLocation: database.Location{Latitude: -34.6037, Longitude: -58.3816},
		DeliveryFee:     29.00,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "delivery_id", Value: fmt.Sprintf("%d", delivery.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).GetDeliveryOrder(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var row map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &row))
	assert.InDelta(t, 186.50, row["total"], 0.001, "detail must emit bill total as dollars (incl. delivery fee)")
	assert.Equal(t, "ARS", row["currency"])
	pickup, _ := row["pickup_location"].(map[string]any)
	require.NotNil(t, pickup)
	assert.InDelta(t, lat, pickup["latitude"], 0.0001)
	drop, _ := row["dropoff_location"].(map[string]any)
	require.NotNil(t, drop)
	assert.InDelta(t, -34.6037, drop["latitude"], 0.0001)
}

// 896-B on the wire: businesses.default_currency has no DB default, so the
// dispatch list used to emit a blank currency for every business that never
// picked one — the exact symptom the issue reported. And a business that DID
// set a display currency must see that currency here, not the default one.
func TestGetBusinessDeliveries_CurrencyFloorsToUSDAndPrefersDisplay(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	business.DisplayCurrency = ""
	business.DefaultCurrency = ""
	require.NoError(t, database.GetDB().Save(business).Error)

	bill := createTestBill(t, business.ID)
	bill.TotalAmount = 15750
	require.NoError(t, database.GetDB().Save(bill).Error)

	_, err := services.NewDeliveryService(database.GetDB(), nil).CreateDeliveryOrder(services.CreateDeliveryOrderRequest{
		BusinessID:      business.ID,
		BillID:          bill.ID,
		CustomerName:    "Currency Guest",
		CustomerPhone:   "5551112222",
		DeliveryAddress: database.DeliveryAddress{Street: "Defensa 1148", City: "CABA", Country: "AR"},
		DeliveryFee:     29.00,
	})
	require.NoError(t, err)

	listCurrency := func() string {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).GetBusinessDeliveries(c)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var result map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		rows, ok := result["deliveries"].([]any)
		require.True(t, ok)
		require.NotEmpty(t, rows)
		row, ok := rows[0].(map[string]any)
		require.True(t, ok)
		currency, _ := row["currency"].(string)
		return currency
	}

	assert.Equal(t, "USD", listCurrency(),
		"a business with no currency columns must still emit a currency, not a blank chip")

	require.NoError(t, database.GetDB().Model(&database.Business{}).Where("id = ?", business.ID).
		Updates(map[string]any{"default_currency": "ARS", "display_currency": "BRL"}).Error)
	assert.Equal(t, "BRL", listCurrency(),
		"display_currency must win over default_currency, matching bills/orders")
}

func TestGetBusinessDeliveries_RejectsUnknownStatusFilter(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/?status=shipped", nil)

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.GetBusinessDeliveries(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid status")
}

func TestAssignDriver_RejectsAlreadyAssignedWith409(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	deliveryOrder := createTestDeliveryOrder(t, business.ID, bill.ID)

	makeDriver := func(phone string) *database.DeliveryDriver {
		d := &database.DeliveryDriver{
			BusinessID: business.ID, Name: "Driver", Phone: phone,
			Status: database.DriverStatusOnline, IsAvailable: true, IsActive: true,
			VehicleType: database.VehicleTypeCar,
		}
		require.NoError(t, database.GetDB().Create(d).Error)
		return d
	}
	first := makeDriver("5551110000")
	second := makeDriver("5552220000")

	service := services.NewDeliveryService(database.GetDB(), nil)
	require.NoError(t, service.AssignDriverByBusiness(business.ID, deliveryOrder.ID, first.ID))
	claimTestDelivery(t, business.ID, deliveryOrder.ID, 9)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "delivery_id", Value: fmt.Sprintf("%d", deliveryOrder.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(fmt.Sprintf(`{"driver_id":%d}`, second.ID)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_id", uint(9))
	c.Set("staff_name", "Test Staff")
	c.Set("staff_role", "server")

	NewDeliveryHandler(service).AssignDriver(c)
	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "already")
}

func TestCancelDeliveryOrder_PublishesDeliveryCancelledEvent(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	deliveryOrder := createTestDeliveryOrder(t, business.ID, bill.ID)
	claimTestDelivery(t, business.ID, deliveryOrder.ID, 9)
	eventsCh, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	body := bytes.NewBufferString(`{"reason":"Customer requested cancellation"}`)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "delivery_id", Value: fmt.Sprintf("%d", deliveryOrder.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_id", uint(9))
	c.Set("staff_name", "Test Staff")
	c.Set("staff_role", "server")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.CancelDeliveryOrder(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertBusinessEvent(t, eventsCh, "delivery.cancelled")
}

func TestUpdateDeliveryStatus_RequiresClaim(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	deliveryOrder := createTestDeliveryOrder(t, business.ID, bill.ID)

	// Another operator holds the claim.
	claimTestDelivery(t, business.ID, deliveryOrder.ID, 1)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "delivery_id", Value: fmt.Sprintf("%d", deliveryOrder.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"status":"ready"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_id", uint(2))
	c.Set("staff_name", "Other")
	c.Set("staff_role", "server")

	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).UpdateDeliveryStatus(c)
	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "claimed_by_name")
	assert.Contains(t, w.Body.String(), "Test Staff")
}

func TestTrackDelivery_ReturnsPublicTrackingDetails(t *testing.T) {
	setupHandlerTestDB(t)

	db := database.GetDB()
	business := createTestBusiness(t)
	business.Name = "Trackable Restaurant"
	business.CustomURL = "trackable-restaurant"
	require.NoError(t, db.Save(business).Error)

	bill := createTestBill(t, business.ID)
	driver := &database.DeliveryDriver{
		BusinessID:  business.ID,
		Name:        "Dana Driver",
		Phone:       "5552223333",
		Status:      database.DriverStatusOnline,
		IsAvailable: true,
		VehicleType: database.VehicleTypeCar,
	}
	require.NoError(t, db.Create(driver).Error)

	delivery := &database.DeliveryOrder{
		BusinessID:     business.ID,
		BillID:         bill.ID,
		DeliveryNumber: "DEL-TRACK-1",
		DeliveryType:   database.DeliveryTypeInHouse,
		Status:         database.DeliveryStatusAssigned,
		DriverID:       &driver.ID,
		CustomerName:   "Guest Customer",
		CustomerPhone:  "5551110000",
		DeliveryAddress: database.DeliveryAddress{
			Street:  "123 Main",
			City:    "Anywhere",
			Country: "US",
		},
	}
	require.NoError(t, db.Exec(`
		INSERT INTO delivery_orders (
			business_id, bill_id, delivery_number, delivery_type, status, fulfillment_mode,
			driver_id, customer_name, customer_phone, delivery_street, delivery_city,
			delivery_country, quote_metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`,
		delivery.BusinessID,
		delivery.BillID,
		delivery.DeliveryNumber,
		delivery.DeliveryType,
		delivery.Status,
		delivery.FulfillmentMode,
		*delivery.DriverID,
		delivery.CustomerName,
		delivery.CustomerPhone,
		delivery.DeliveryAddress.Street,
		delivery.DeliveryAddress.City,
		delivery.DeliveryAddress.Country,
		[]byte(`{}`),
	).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "delivery_number", Value: delivery.DeliveryNumber}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.TrackDelivery(c)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, delivery.DeliveryNumber, response["delivery_number"])
	assert.Equal(t, string(database.DeliveryStatusAssigned), response["status"])
	assert.Equal(t, business.Name, response["business_name"])
	assert.Equal(t, business.CustomURL, response["business_custom_url"])

	driverPayload, ok := response["driver"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, driver.Name, driverPayload["name"])
	assert.Equal(t, driver.Phone, driverPayload["phone"])

	// M-track: an assigned (not awaiting-payment) order must not leak the bill
	// guest capability through the public tracking endpoint.
	billPayload, ok := response["bill"].(map[string]any)
	require.True(t, ok, "public tracking must include bill payment summary")
	require.NotEmpty(t, bill.PublicToken, "test bill fixture must have public_token")
	assert.NotContains(t, billPayload, "public_token",
		"TrackDelivery must withhold public_token when the order is not awaiting payment")
	assert.NotContains(t, w.Body.String(), bill.PublicToken)
	assert.Equal(t, bill.BillNumber, billPayload["bill_number"])

	// Venue timezone for guest ETA formatting (device TZ must not win).
	if business.Timezone != "" {
		assert.Equal(t, business.Timezone, response["timezone"])
	}
}

// M-track: while an online-prepay order awaits payment the guest pay page
// needs bill.public_token for the /guest/bill capability routes, so it is
// exposed only in that state.
func TestTrackDelivery_ExposesPublicTokenOnlyWhileAwaitingPayment(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.DeliverySettings{}))

	db := database.GetDB()
	business := createTestBusiness(t)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", business.ID).
		Update("settlement_addr", "0x1111111111111111111111111111111111111111").Error)
	bill := createTestBill(t, business.ID)
	require.NotEmpty(t, bill.PublicToken)

	insert := func(number string, status database.DeliveryStatus) {
		require.NoError(t, db.Exec(`
		INSERT INTO delivery_orders (
			business_id, bill_id, delivery_number, delivery_type, status, fulfillment_mode,
			customer_name, customer_phone, delivery_street, delivery_city,
			delivery_country, quote_metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`,
			business.ID, bill.ID, number, database.DeliveryTypeInHouse,
			status, "in_house", "Guest", "5550001111",
			"1 Main", "CABA", "AR", []byte(`{}`),
		).Error)
	}
	insert("DEL-AWAIT-PAY", database.DeliveryStatusConfirmed)
	insert("DEL-ON-ROUTE", database.DeliveryStatusPickedUp)

	track := func(number string) map[string]any {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "delivery_number", Value: number}}
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		NewDeliveryHandler(services.NewDeliveryService(db, nil)).TrackDelivery(c)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		return response
	}

	awaiting := track("DEL-AWAIT-PAY")
	require.Equal(t, true, awaiting["awaiting_payment"], "fixture must be awaiting online payment: %v", awaiting)
	awaitingBill, ok := awaiting["bill"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, bill.PublicToken, awaitingBill["public_token"])

	onRoute := track("DEL-ON-ROUTE")
	assert.Equal(t, false, onRoute["awaiting_payment"])
	onRouteBill, ok := onRoute["bill"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, onRouteBill, "public_token")
}

func TestTrackDelivery_IncludesConfiguredCourierPartners(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.DeliverySettings{}))

	db := database.GetDB()
	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	require.NoError(t, db.Create(&database.DeliverySettings{
		BusinessID:           business.ID,
		ExternalPartnerLinks: database.JSONRawMessage(`[{"name":"PedidosYa","url":"https://www.pedidosya.com.ar","provider_key":"pedidosya"},{"name":"Rappi","url":"https://www.rappi.com.ar","provider_key":"rappi"}]`),
	}).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO delivery_orders (
			business_id, bill_id, delivery_number, delivery_type, status, fulfillment_mode,
			customer_name, customer_phone, delivery_street, delivery_city,
			delivery_country, quote_metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`,
		business.ID, bill.ID, "DEL-COURIER-1", database.DeliveryTypeInHouse,
		database.DeliveryStatusPending, "in_house", "Guest", "5550001111",
		"1 Main", "CABA", "AR", []byte(`{}`),
	).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "delivery_number", Value: "DEL-COURIER-1"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	NewDeliveryHandler(services.NewDeliveryService(db, nil)).TrackDelivery(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	links, ok := response["external_partner_links"].([]any)
	require.True(t, ok, "body=%s", w.Body.String())
	require.Len(t, links, 2)
	first, _ := links[0].(map[string]any)
	assert.Equal(t, "PedidosYa", first["name"])
}

// #897 security: partner links are operator-configured URLs that the guest
// track/checkout surfaces render as anchor hrefs. A stored javascript: URL must
// never reach the wire.
func TestTrackDelivery_OmitsUnsafePartnerLinkSchemes(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.DeliverySettings{}))

	db := database.GetDB()
	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	require.NoError(t, db.Create(&database.DeliverySettings{
		BusinessID: business.ID,
		ExternalPartnerLinks: database.JSONRawMessage(`[` +
			`{"name":"Evil","url":"javascript:alert(1)","provider_key":"pedidosya"},` +
			`{"name":"DataEvil","url":"data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==","provider_key":"rappi"},` +
			`{"name":"Rappi","url":"https://www.rappi.com.ar","provider_key":"rappi","icon_url":"javascript:alert(2)"}` +
			`]`),
	}).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO delivery_orders (
			business_id, bill_id, delivery_number, delivery_type, status, fulfillment_mode,
			customer_name, customer_phone, delivery_street, delivery_city,
			delivery_country, quote_metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`,
		business.ID, bill.ID, "DEL-COURIER-XSS", database.DeliveryTypeInHouse,
		database.DeliveryStatusPending, "in_house", "Guest", "5550001111",
		"1 Main", "CABA", "AR", []byte(`{}`),
	).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "delivery_number", Value: "DEL-COURIER-XSS"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	NewDeliveryHandler(services.NewDeliveryService(db, nil)).TrackDelivery(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "javascript:")
	assert.NotContains(t, w.Body.String(), "data:text/html")

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	links, ok := response["external_partner_links"].([]any)
	require.True(t, ok, "body=%s", w.Body.String())
	require.Len(t, links, 1, "only the https partner survives")
	only, _ := links[0].(map[string]any)
	assert.Equal(t, "Rappi", only["name"])
	assert.Equal(t, "https://www.rappi.com.ar", only["url"])
	_, hasIcon := only["icon_url"]
	assert.False(t, hasIcon, "unsafe icon_url is stripped")
}

func TestGetDeliverySettings_PublicReturnsNotFoundForInactiveBusiness(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.DeliverySettings{}, &database.DeliveryZone{}))

	business := createTestBusiness(t)
	business.IsActive = false
	business.BusinessPageEnabled = true
	require.NoError(t, database.GetDB().Save(business).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.GetDeliverySettings(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "business not found")
}

func TestQuoteDelivery_PublicReturnsNotFoundForInactiveBusiness(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.DeliverySettings{}, &database.DeliveryZone{}))

	business := createTestBusiness(t)
	business.IsActive = false
	business.BusinessPageEnabled = true
	require.NoError(t, database.GetDB().Save(business).Error)

	body, err := json.Marshal(services.DeliveryQuoteRequest{
		OrderSubtotal: 25,
		DeliveryAddress: database.DeliveryAddress{
			Street:  "1 Main",
			City:    "Anywhere",
			Country: "US",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.QuoteDelivery(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "business not found")
}

// seedPublicDeliveryBusiness configures a business that is visible on the
// public delivery storefront (active + page enabled, non-demo). closed marks
// it closed by the server administrator (closed_at set) — the lock a public
// storefront still resolves (a suspended is_active=false venue is a 404).
func seedPublicDeliveryBusiness(t *testing.T, closed bool) *database.Business {
	t.Helper()
	business := createTestBusiness(t)
	business.IsActive = true
	business.BusinessPageEnabled = true
	business.IsDemo = false
	if closed {
		closedAt := time.Now().Add(-time.Hour)
		business.ClosedAt = &closedAt
	}
	require.NoError(t, database.GetDB().Save(business).Error)
	return business
}

// TestGetDeliverySettings_PublicClosedDisablesInHouse: an administrator-closed
// venue keeps its informational storefront but must not advertise in-house
// checkout that the quote/create path refuses.
func TestGetDeliverySettings_PublicClosedDisablesInHouse(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.DeliverySettings{}, &database.DeliveryZone{}))

	business := seedPublicDeliveryBusiness(t, true)
	require.NoError(t, database.GetDB().Create(&database.DeliverySettings{
		BusinessID:             business.ID,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
		ThirdPartyEnabled:      true,
		ExternalPartnerLinks:   database.JSONRawMessage(`[]`),
		DeliveryZones:          database.JSONRawMessage(`[]`),
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.GetDeliverySettings(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["delivery_enabled"])
	assert.Equal(t, false, resp["in_house_delivery_enabled"], "closed venue must not advertise in-house checkout")
}

// TestQuoteDelivery_ClosedBusinessUnavailable: a closed (non-demo) business
// must not quote delivery fees. Mirrors CreateGuestOrder's 403
// business_unavailable contract.
func TestQuoteDelivery_ClosedBusinessUnavailable(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.DeliverySettings{}, &database.DeliveryZone{}))

	business := seedPublicDeliveryBusiness(t, true)

	body, err := json.Marshal(services.DeliveryQuoteRequest{
		OrderSubtotal: 25,
		DeliveryAddress: database.DeliveryAddress{
			Street:  "1 Main",
			City:    "Anywhere",
			Country: "US",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.QuoteDelivery(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"code":"business_unavailable"`)
	assert.Contains(t, w.Body.String(), "not currently accepting orders")
}

// TestGuestDeliveryCheckout_ClosedBusinessUnavailable: a closed non-demo
// business must not create bills/orders/deliveries from public guest delivery
// checkout.
func TestGuestDeliveryCheckout_ClosedBusinessUnavailable(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.DeliverySettings{}, &database.DeliveryZone{}))

	business := seedPublicDeliveryBusiness(t, true)

	body, err := json.Marshal(services.GuestDeliveryCheckoutRequest{
		CustomerName:  "Guest",
		CustomerPhone: "5551112222",
		CustomerEmail: "guest@example.com",
		DeliveryAddress: database.DeliveryAddress{
			Street:  "1 Main",
			City:    "Anywhere",
			Country: "US",
		},
		Items: []services.DeliveryCheckoutItemInput{
			{MenuItemID: "burger", MenuItemName: "Burger", Quantity: 1, Price: 12},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.GuestDeliveryCheckout(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), `"code":"business_unavailable"`)

	// No bill / delivery rows may be created for a closed storefront.
	var billCount, deliveryCount int64
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("business_id = ?", business.ID).Count(&billCount).Error)
	require.NoError(t, database.GetDB().Model(&database.DeliveryOrder{}).Where("business_id = ?", business.ID).Count(&deliveryCount).Error)
	assert.Equal(t, int64(0), billCount, "closed checkout must not create bills")
	assert.Equal(t, int64(0), deliveryCount, "closed checkout must not create deliveries")
}

func TestCreateDriver_RejectsUnknownVehicleType(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)

	body := []byte(`{"name":"Sam","phone":"5551112222","vehicle_type":"spaceship"}`)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.CreateDriver(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid vehicle_type")
}

// L3-42: garbage phones like "abc" must not pass the create path.
func TestCreateDriver_RejectsInvalidPhone(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)

	body := []byte(`{"name":"Sam","phone":"abc","vehicle_type":"car"}`)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.CreateDriver(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid phone")
}

func TestIsValidDriverPhone(t *testing.T) {
	assert.True(t, isValidDriverPhone("5551112222"))
	assert.True(t, isValidDriverPhone("+54 11 5555-1234"))
	assert.False(t, isValidDriverPhone("abc"))
	assert.False(t, isValidDriverPhone("123"))
	assert.False(t, isValidDriverPhone(""))
}

func TestCreateDriver_AcceptsEmptyAndKnownVehicleType(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))

	for _, vt := range []string{"", "bicycle", "car", "van"} {
		body := []byte(fmt.Sprintf(`{"name":"Sam","phone":"5551112222","vehicle_type":%q}`, vt))
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("token_type", "staff")
		handler.CreateDriver(c)
		assert.Equal(t, http.StatusCreated, w.Code, "vehicle_type=%q should be accepted", vt)
	}
}

func TestUpdateDriver_RejectsUnknownVehicleType(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	service := services.NewDeliveryService(database.GetDB(), nil)
	driver := &database.DeliveryDriver{BusinessID: business.ID, Name: "Sam", Phone: "555"}
	require.NoError(t, service.CreateDriver(driver))

	body := []byte(`{"vehicle_type":"hovercraft"}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "driver_id", Value: fmt.Sprintf("%d", driver.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")

	NewDeliveryHandler(service).UpdateDriver(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid vehicle_type")
}

// --- Finding 1: Public guest routes must not leak raw DB/GORM error text ---

// createActivePublicBusiness creates a business that passes the getPublicDeliveryBusiness gate.
func createActivePublicBusiness(t *testing.T) *database.Business {
	t.Helper()
	biz := createTestBusiness(t)
	biz.IsActive = true
	biz.BusinessPageEnabled = true
	require.NoError(t, database.GetDB().Save(biz).Error)
	return biz
}

func TestQuoteDelivery_VenueOwnAddressNotZoneUnavailable(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.DeliverySettings{},
		&database.DeliveryZone{},
		&database.BusinessOperatingHours{},
	))

	biz := createActivePublicBusiness(t)
	biz.Address = database.BusinessAddress{
		Street: "Defensa 1148, San Telmo", City: "Buenos Aires",
		State: "CABA", PostalCode: "C1065", Country: "AR",
	}
	require.NoError(t, database.GetDB().Save(biz).Error)

	settings := &database.DeliverySettings{
		BusinessID:                  biz.ID,
		DeliveryEnabled:             true,
		InHouseDeliveryEnabled:      true,
		MinimumOrderAmount:          1500000,
		FlatDeliveryFee:             290000,
		DeliveryHoursSameAsBusiness: false,
		DeliveryStartTime:           "00:00",
		DeliveryEndTime:             "00:00",
		ExternalPartnerLinks:        database.JSONRawMessage(`[]`),
		DeliveryZones:               database.JSONRawMessage(`[]`),
	}
	require.NoError(t, database.GetDB().Create(settings).Error)
	require.NoError(t, database.GetDB().Model(settings).UpdateColumn(
		"delivery_hours_same_as_business", false,
	).Error)
	require.NoError(t, database.GetDB().Create(&database.DeliveryZone{
		BusinessID:         biz.ID,
		Name:               "CABA",
		Boundaries:         `{"type":"Polygon","coordinates":[]}`,
		DeliveryFee:        290000,
		MinimumOrderAmount: 1500000,
		EstimatedTime:      35,
		IsActive:           true,
	}).Error)

	body, err := json.Marshal(services.DeliveryQuoteRequest{
		OrderSubtotal: 60000,
		DeliveryAddress: database.DeliveryAddress{
			Street: "Defensa 1148", City: "CABA", PostalCode: "C1065", Country: "AR",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).QuoteDelivery(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var quote map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &quote))
	assert.NotEqual(t, "zone_unavailable", quote["reason_code"], "body=%s", w.Body.String())
	assert.Equal(t, true, quote["eligible"], "body=%s", w.Body.String())
}

func TestQuoteDelivery_InternalErrorDoesNotLeakRawMessage(t *testing.T) {
	// setupHandlerTestDB does NOT create the delivery_settings table, so any
	// QuoteDelivery call past the business-gate triggers a raw GORM/SQLite
	// error. The handler must return 500 without surfacing that raw text.
	setupHandlerTestDB(t)
	biz := createActivePublicBusiness(t)

	body, _ := json.Marshal(services.DeliveryQuoteRequest{
		OrderSubtotal:   25,
		DeliveryAddress: database.DeliveryAddress{Street: "1 Main", City: "Anywhere", Country: "US"},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).QuoteDelivery(c)

	// Must be an error response (not 200 or 404).
	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"non-sentinel DB error should surface as 500, not expose raw text")
	// Raw DB/table detail must not reach the unauthenticated caller.
	body2 := w.Body.String()
	assert.NotContains(t, body2, "delivery_settings", "raw table name must not leak")
	assert.NotContains(t, body2, "no such table", "raw SQLite error must not leak")
	assert.NotContains(t, body2, "failed to get", "wrapped error prefix must not leak")
}

// TestGuestDeliveryCheckout_ValidationErrorIsSafe verifies that user-facing
// validation errors from the service (e.g. missing customer name/phone) are
// returned as 400 with a human-readable message and NOT as a raw internal error.
func TestGuestDeliveryCheckout_ValidationErrorIsSafe(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createActivePublicBusiness(t)

	// Send whitespace-only customer_name: passes Gin binding (not empty string)
	// but fails the service's TrimSpace validation, triggering ErrDeliveryValidation.
	reqBody := map[string]any{
		"customer_name":    "   ",
		"customer_phone":   "5551112222",
		"customer_email":   "alice@example.com",
		"delivery_address": map[string]any{"street": "1 Main", "city": "Anywhere", "country": "US"},
		"items": []map[string]any{
			{"menu_item_name": "Burger", "quantity": 1, "price": 10.0},
		},
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).GuestDeliveryCheckout(c)

	// Missing customer_name / phone is a validation error: must be 400, not 500.
	assert.Equal(t, http.StatusBadRequest, w.Code,
		"validation errors from service must return 400")
	// The safe user message must be surfaced (not swallowed entirely).
	assert.Contains(t, w.Body.String(), "customer name and phone are required",
		"validation message must be forwarded to caller")
}

// TestGuestDeliveryCheckout_OversizedCustomerNameIs400: a 5000-rune name is a
// field-limit failure (ErrDeliveryValidation) and must be HTTP 400.
func TestGuestDeliveryCheckout_OversizedCustomerNameIs400(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createActivePublicBusiness(t)

	reqBody := map[string]any{
		"customer_name":    strings.Repeat("a", 5000),
		"customer_phone":   "5551112222",
		"customer_email":   "alice@example.com",
		"delivery_address": map[string]any{"street": "1 Main", "city": "Anywhere", "country": "US"},
		"items": []map[string]any{
			{"menu_item_name": "Burger", "quantity": 1, "price": 10.0},
		},
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).GuestDeliveryCheckout(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "customer_name must be at most 100")
}

// TestGuestDeliveryCheckout_InternalErrorDoesNotLeakRawMessage verifies that a
// non-sentinel DB error (e.g. missing delivery_settings table) surfaces as 500
// without exposing raw SQL or table-name detail to the unauthenticated caller.
// To reach the delivery-settings DB path the menu lookup must not error, so we
// insert a real menu with a matching item before the request.
func TestGuestDeliveryCheckout_InternalErrorDoesNotLeakRawMessage(t *testing.T) {
	setupHandlerTestDB(t)
	// AutoMigrate Menu (not in the base setupHandlerTestDB set).
	require.NoError(t, database.GetDB().AutoMigrate(&database.Menu{}))
	biz := createActivePublicBusiness(t)

	// Insert an active menu with a "Burger" menu item so that
	// PriceOrderInputsByBusinessID can resolve the item and the
	// checkout reaches the QuoteDelivery → GetDeliverySettings path
	// which will fail because delivery_settings is NOT migrated.
	burgerItem := database.MenuItem{
		ID:          fmt.Sprintf("burger-%d", time.Now().UnixNano()),
		Name:        "Burger",
		Price:       10.0,
		IsAvailable: true,
	}
	menuCategories, _ := json.Marshal([]database.MenuCategory{
		{ID: "cat1", Name: "Mains", Items: []database.MenuItem{burgerItem}},
	})
	menu := &database.Menu{
		BusinessID: biz.ID,
		IsActive:   true,
		Categories: string(menuCategories),
	}
	require.NoError(t, database.GetDB().Create(menu).Error)

	reqBody := map[string]any{
		"customer_name":    "Alice",
		"customer_phone":   "5551112222",
		"customer_email":   "alice@example.com",
		"delivery_address": map[string]any{"street": "1 Main", "city": "Anywhere", "country": "US"},
		"items": []map[string]any{
			{"menu_item_name": "Burger", "quantity": 1, "price": 10.0},
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "business_id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).GuestDeliveryCheckout(c)

	// The service fails with a raw DB error (no delivery_settings table).
	// Handler must return 500 without leaking raw error text.
	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"non-sentinel DB error must surface as 500, not expose raw text")
	body2 := w.Body.String()
	assert.NotContains(t, body2, "delivery_settings", "raw table name must not leak")
	assert.NotContains(t, body2, "no such table", "raw SQLite error must not leak")
	assert.NotContains(t, body2, "failed to get", "wrapped error prefix must not leak")
}

// --- Finding 2: CreateDriver must reject a staff_id from another business ---

func TestCreateDriver_RejectsStaffFromAnotherBusiness(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Staff{}))

	ownerBiz := createTestBusiness(t)
	targetBiz := createTestBusiness(t)

	// Create a staff member that belongs to ownerBiz.
	staff := &database.Staff{
		BusinessID: ownerBiz.ID,
		Email:      fmt.Sprintf("staff-%d@example.com", time.Now().UnixNano()),
		Name:       "Other Biz Staff",
		Role:       database.StaffRoleServer,
		InvitedBy:  "0xowner",
	}
	require.NoError(t, database.GetDB().Create(staff).Error)

	// Attempt to create a driver for targetBiz that references ownerBiz's staff.
	body := []byte(fmt.Sprintf(
		`{"name":"Bob","phone":"5559998888","vehicle_type":"car","staff_id":%d}`,
		staff.ID,
	))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", targetBiz.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")

	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).CreateDriver(c)

	assert.Equal(t, http.StatusBadRequest, w.Code,
		"staff_id from a different business must be rejected")
	assert.NotContains(t, w.Body.String(), staff.Email,
		"staff PII from another business must not be disclosed")
}

// --- DEL-LEAK-6: operator settings path must not leak raw DB/GORM error text ---

// TestUpdateDeliverySettings_InternalErrorDoesNotLeakRawMessage verifies that a
// non-sentinel DB error (missing delivery_settings table under
// setupHandlerTestDB) surfaces as a generic 500 rather than echoing the raw
// GORM/SQLite error back to the operator client as a 400.
func TestUpdateDeliverySettings_InternalErrorDoesNotLeakRawMessage(t *testing.T) {
	setupHandlerTestDB(t)
	biz := createTestBusiness(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"delivery_enabled":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")

	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).UpdateDeliverySettings(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"non-sentinel DB error should surface as 500, not a 400 with raw text")
	body := w.Body.String()
	assert.NotContains(t, body, "delivery_settings", "raw table name must not leak")
	assert.NotContains(t, body, "no such table", "raw SQLite error must not leak")
	assert.NotContains(t, body, "failed to get", "wrapped error prefix must not leak")
}

// TestUpdateDeliverySettings_ValidationErrorReturnsSafeMessage pins that
// operator-facing validation failures from validateDeliverySettings keep
// returning 400 with their human-readable message (not a generic 500).
func TestUpdateDeliverySettings_ValidationErrorReturnsSafeMessage(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.DeliverySettings{}, &database.DeliveryZone{}))
	biz := createTestBusiness(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", biz.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"payment_mode":"bitcoin"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")

	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).UpdateDeliverySettings(c)

	assert.Equal(t, http.StatusBadRequest, w.Code,
		"validation errors must stay 400")
	assert.Contains(t, w.Body.String(), "invalid payment_mode",
		"validation message must be forwarded to the operator")
}

// --- DELIV-NOTIF-2: post-commit delivery order.created enqueue must be idempotent ---

// TestNotifyGuestDeliveryOrderCreated_TelegramEnqueueIsIdempotent verifies the
// post-commit outbox write is safe to attempt more than once (retry / replay):
// the ON CONFLICT dedup keyed on (business_id, plugin_name, event_type,
// event_id) must leave exactly one pending delivery row.
// DELIV-NOTIF-2 (resolved): the Telegram order.created outbox row for a guest
// delivery checkout is written INSIDE the service-layer checkout transaction
// (services.GuestDeliveryCheckout). The handler-side notifier must therefore
// NOT enqueue it again — with the ON CONFLICT dedup a replay would be
// harmless, but the handler owning zero Telegram writes is what proves no
// double-send path exists.
func TestNotifyGuestDeliveryOrderCreated_DoesNotEnqueueTelegram(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.PluginNotificationDelivery{}, &database.Plugin{}, &database.BusinessPlugin{}))
	biz := createTestBusiness(t)

	tgPlugin := database.Plugin{Name: "telegram", DisplayName: "Telegram", IsActive: true}
	require.NoError(t, database.GetDB().Create(&tgPlugin).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: biz.ID, PluginID: tgPlugin.ID, IsEnabled: true,
		Config: `{"is_connected":true,"chat_id":"55"}`,
	}).Error)
	services.ResetTelegramNotificationEligibilityCache()

	bill := createTestBill(t, biz.ID)
	order := &database.Order{
		BusinessID:  biz.ID,
		BillID:      bill.ID,
		OrderNumber: fmt.Sprintf("DO-%d", time.Now().UnixNano()),
		Items:       `[{"name":"Burger","quantity":1,"price":10,"subtotal":10}]`,
	}
	require.NoError(t, database.GetDB().Create(order).Error)

	checkout := &services.GuestDeliveryCheckoutDTO{Order: order, Bill: bill}
	notifyGuestDeliveryOrderCreated(checkout)
	notifyGuestDeliveryOrderCreated(checkout)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.PluginNotificationDelivery{}).
		Where("business_id = ? AND event_type = ? AND event_id = ?",
			biz.ID, services.PluginEventOrderCreated, fmt.Sprintf("order:%d", order.ID)).
		Count(&count).Error)
	assert.EqualValues(t, 0, count,
		"handler must not enqueue Telegram order.created — the service-layer checkout transaction owns the outbox write")
}
