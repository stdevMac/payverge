package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func setupDeliveryTrackingPerfDB(t testing.TB, gormLogger logger.Interface) (*database.Business, string) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())), cfg)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	database.SetTestDB(gormDB)
	server.InitializeRBAC(database.GetDBWrapper())
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Order{},
		&database.Customer{},
		&database.DeliveryOrder{},
		&database.DeliveryDriver{},
		&database.DeliveryStatusHistory{},
	))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("delivery-track-%d", time.Now().UnixNano()),
		Name:           "Trackable Restaurant",
		CustomURL:      "trackable-restaurant",
		OwnerAddress:   "0xdeliverytrack",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		Description:    strings.Repeat("large business payload ", 128),
		BannerImages:   strings.Repeat("banner-url,", 128),
	}
	require.NoError(t, gormDB.Create(business).Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "DELIVERY-TRACK-BILL",
		Status:      database.BillStatusOpen,
		Items:       strings.Repeat(`{"id":"item","name":"Bench"},`, 256),
		TotalAmount: 5000,
	}
	require.NoError(t, gormDB.Create(bill).Error)

	requestID := fmt.Sprintf("delivery-track-%d", time.Now().UnixNano())
	order := &database.Order{
		BillID:          bill.ID,
		BusinessID:      business.ID,
		OrderNumber:     "DELIVERY-TRACK-ORDER",
		Status:          database.OrderStatusApproved,
		CreatedBy:       "guest",
		ClientRequestID: &requestID,
		Items:           strings.Repeat(`{"id":"order-item","name":"Bench"},`, 256),
		Notes:           strings.Repeat("large order note ", 128),
		CancelReason:    strings.Repeat("large cancel reason ", 128),
	}
	require.NoError(t, gormDB.Create(order).Error)

	customer := &database.Customer{
		Email:           fmt.Sprintf("delivery-track-%d@example.com", time.Now().UnixNano()),
		Name:            "Delivery Customer",
		Phone:           "5551110000",
		PasswordHash:    strings.Repeat("password-hash", 128),
		ProfileImageURL: strings.Repeat("profile-url", 128),
	}
	require.NoError(t, gormDB.Create(customer).Error)

	driver := &database.DeliveryDriver{
		BusinessID:      business.ID,
		Name:            "Dana Driver",
		Phone:           "5552223333",
		Email:           "driver@example.com",
		LicenseNumber:   strings.Repeat("license", 64),
		VehiclePlate:    "CAR-123",
		Status:          database.DriverStatusOnline,
		IsAvailable:     true,
		VehicleType:     database.VehicleTypeCar,
		TotalDeliveries: 100,
		IsActive:        true,
	}
	require.NoError(t, gormDB.Create(driver).Error)

	eta := time.Now().UTC().Add(35 * time.Minute)
	delivery := &database.DeliveryOrder{
		BusinessID:            business.ID,
		BillID:                bill.ID,
		OrderID:               &order.ID,
		CustomerID:            &customer.ID,
		DriverID:              &driver.ID,
		DeliveryNumber:        "DEL-TRACK-PERF",
		DeliveryType:          database.DeliveryTypeInHouse,
		Status:                database.DeliveryStatusAssigned,
		FulfillmentMode:       "in_house",
		CustomerName:          customer.Name,
		CustomerPhone:         customer.Phone,
		CustomerEmail:         customer.Email,
		DeliveryInstructions:  strings.Repeat("leave at door ", 128),
		SignatureURL:          strings.Repeat("signature-url", 128),
		PhotoURL:              strings.Repeat("photo-url", 128),
		ExternalTrackingURL:   "https://tracking.example/del-track-perf",
		EstimatedDeliveryTime: &eta,
		QuoteMetadata:         database.JSONRawMessage(`{}`),
	}
	require.NoError(t, gormDB.Create(delivery).Error)

	for i := 0; i < 25; i++ {
		require.NoError(t, gormDB.Create(&database.DeliveryStatusHistory{
			DeliveryOrderID: delivery.ID,
			Status:          database.DeliveryStatusAssigned,
			Notes:           strings.Repeat("tracking history note ", 16),
			ChangedBy:       "system",
			CreatedAt:       time.Now().UTC().Add(-time.Duration(i) * time.Minute),
		}).Error)
	}

	return business, delivery.DeliveryNumber
}

func performDeliveryTrackingRequest(t testing.TB, handler *DeliveryHandler, deliveryNumber string) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "delivery_number", Value: deliveryNumber}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/delivery/%s/track", deliveryNumber), nil)
	handler.TrackDelivery(c)
	return w
}

func TestTrackDeliveryUsesPublicProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &analyticsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business, deliveryNumber := setupDeliveryTrackingPerfDB(t, recorder)

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	w := performDeliveryTrackingRequest(t, handler, deliveryNumber)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), business.Name)

	assert.Zero(t, recorder.selectStarCount("delivery_orders"), "public tracking should project delivery fields instead of SELECT *")
	assert.Zero(t, recorder.selectStarCount("businesses"), "public tracking should project business display fields instead of SELECT *")
	assert.Zero(t, recorder.selectStarCount("delivery_drivers"), "public tracking should project driver display fields instead of SELECT *")
	// The delivery overhaul (2026-06-10 spec §10) extended public tracking to
	// expose bill payment state: exactly one projected bill read is by design.
	assert.Zero(t, recorder.selectStarCount("bills"), "public tracking's bill payment-state read must stay a narrow projection, not SELECT *")
	assert.Equal(t, 1, recorder.selectCount("bills"), "public tracking reads bill payment state in exactly one projected query")
	assert.Zero(t, recorder.selectCount("orders"), "public tracking should not load order aggregates")
	assert.Zero(t, recorder.selectCount("customers"), "public tracking should not load customer aggregates")
	assert.Zero(t, recorder.selectCount("delivery_status_history"), "public tracking response does not use status history")
}

func BenchmarkTrackDeliverySQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	_, deliveryNumber := setupDeliveryTrackingPerfDB(b, logger.Default.LogMode(logger.Silent))
	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := performDeliveryTrackingRequest(b, handler, deliveryNumber)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}
