package services

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Access-shape regression tests for DELIV-PERF-3 / DEL-SM-3: the
// business-scoped status-update/assign/cancel wrappers used to hydrate the
// full dispatch aggregate (Business + Driver + Customer + unbounded
// StatusHistory preloads) just to check ownership, after which the mutation
// re-read the row under FOR UPDATE anyway. The ownership check must be a
// narrow delivery_orders projection with no relation preloads.

func seedOwnershipDelivery(t testing.TB, db *gorm.DB, historyRows int) (businessID, deliveryID uint) {
	t.Helper()

	business := &database.Business{
		BusinessId:     fmt.Sprintf("ownership-%s", t.Name()),
		Name:           "Ownership Perf",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)

	bill := &database.Bill{BusinessID: business.ID, BillNumber: fmt.Sprintf("OWN-BILL-%s", t.Name()), Status: database.BillStatusOpen, Items: "[]"}
	require.NoError(t, db.Create(bill).Error)

	delivery := &database.DeliveryOrder{
		BusinessID:      business.ID,
		BillID:          bill.ID,
		DeliveryNumber:  fmt.Sprintf("DEL-OWN-%s", t.Name()),
		DeliveryType:    database.DeliveryTypeInHouse,
		Status:          database.DeliveryStatusPreparing,
		FulfillmentMode: "in_house",
		CustomerName:    "Ownership Customer",
		// CustomerEmail deliberately empty: notifyStatusChange short-circuits, so
		// the recorder only sees the ownership check + mutation reads.
		CustomerPhone:   "5552223333",
		DeliveryAddress: database.DeliveryAddress{Street: "1 Main", City: "Anywhere", Country: "US"},
		QuoteMetadata:   database.JSONRawMessage(`{}`),
	}
	require.NoError(t, db.Create(delivery).Error)

	histories := make([]database.DeliveryStatusHistory, 0, historyRows)
	for i := 0; i < historyRows; i++ {
		histories = append(histories, database.DeliveryStatusHistory{
			DeliveryOrderID: delivery.ID,
			Status:          database.DeliveryStatusPreparing,
			Notes:           fmt.Sprintf("history row %d", i),
			ChangedBy:       "system",
		})
	}
	require.NoError(t, db.Create(&histories).Error)

	return business.ID, delivery.ID
}

// TestUpdateDeliveryStatusByBusinessOwnershipCheckIsNarrow asserts the
// dangerous access shape is gone: advancing a delivery status through the
// business-scoped wrapper must not SELECT the status-history, business,
// customer, or driver relations just to verify ownership.
func TestUpdateDeliveryStatusByBusinessOwnershipCheckIsNarrow(t *testing.T) {
	recorder := &deliverySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupDeliveryPerfTestDB(t, recorder)
	require.NoError(t, db.AutoMigrate(&database.DeliveryStatusHistory{}, &database.Customer{}))
	businessID, deliveryID := seedOwnershipDelivery(t, db, 8)
	service := NewDeliveryService(db, nil)

	recorder.statements = nil
	require.NoError(t, service.UpdateDeliveryStatusByBusiness(businessID, deliveryID, database.DeliveryStatusReady, nil, "operator"))

	assert.Zero(t, recorder.selectCount("delivery_status_histories"),
		"ownership check must not preload the unbounded status-history aggregate")
	assert.Zero(t, recorder.selectCount("businesses"),
		"ownership check must not hydrate the Business relation")
	assert.Zero(t, recorder.selectCount("customers"),
		"ownership check must not hydrate the Customer relation")
	assert.Zero(t, recorder.selectCount("delivery_drivers"),
		"status update must not hydrate the Driver relation just to check ownership")
	assert.LessOrEqual(t, recorder.selectCount("delivery_orders"), 2,
		"expected only the narrow ownership probe plus the FOR UPDATE mutation read")
}

// TestUpdateDeliveryStatusByBusinessStillRejectsWrongBusiness keeps the
// authz behavior pinned across the projection change.
func TestUpdateDeliveryStatusByBusinessStillRejectsWrongBusiness(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&database.DeliveryStatusHistory{}, &database.Customer{}))
	businessID, deliveryID := seedOwnershipDelivery(t, db, 1)
	service := NewDeliveryService(db, nil)

	err := service.UpdateDeliveryStatusByBusiness(businessID+999, deliveryID, database.DeliveryStatusReady, nil, "operator")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delivery order not found")

	// The scoped row is untouched.
	var reloaded database.DeliveryOrder
	require.NoError(t, db.First(&reloaded, deliveryID).Error)
	assert.Equal(t, database.DeliveryStatusPreparing, reloaded.Status)

	// Cancel wrapper enforces the same scope.
	err = service.CancelDeliveryOrderByBusiness(businessID+999, deliveryID, "nope", "operator")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delivery order not found")
}

// BenchmarkUpdateDeliveryStatusByBusinessNoop measures the business-scoped
// status-update wrapper on a delivery carrying a realistic status-history tail.
// The no-op advance (same status) isolates the ownership-check + locked-read
// cost without mutating state between iterations. Run with -benchmem; compare
// before/after the narrow-projection fix (DELIV-PERF-3 perf gate).
func BenchmarkUpdateDeliveryStatusByBusinessNoop(b *testing.B) {
	db := setupDeliveryPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	if err := db.AutoMigrate(&database.DeliveryStatusHistory{}, &database.Customer{}); err != nil {
		b.Fatal(err)
	}
	businessID, deliveryID := seedOwnershipDelivery(b, db, 25)
	service := NewDeliveryService(db, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := service.UpdateDeliveryStatusByBusiness(businessID, deliveryID, database.DeliveryStatusPreparing, nil, "bench"); err != nil {
			b.Fatal(err)
		}
	}
}
