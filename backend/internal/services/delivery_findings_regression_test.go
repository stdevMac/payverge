package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

// ---------------------------------------------------------------------------
// DEL-SM-1: AssignDriver must honor the delivery state machine. A "confirmed"
// delivery is an unpaid prepay order — only the system payment path may advance
// it. Assigning a driver directly would jump it to "assigned" without payment.
// ---------------------------------------------------------------------------

func TestAssignDriverRejectsUnpaidConfirmedDelivery(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&database.DeliveryStatusHistory{}))
	_, deliveryID, driverID := seedAssignableDelivery(t, db)

	// Force the delivery into the unpaid-prepay state.
	require.NoError(t, db.Model(&database.DeliveryOrder{}).
		Where("id = ?", deliveryID).
		Update("status", database.DeliveryStatusConfirmed).Error)

	err := NewDeliveryService(db, nil).AssignDriver(deliveryID, driverID)
	require.ErrorIs(t, err, ErrInvalidDeliveryTransition,
		"assigning a driver to an unpaid 'confirmed' delivery must be rejected by the state machine")

	// Delivery row untouched: still confirmed, no driver, no assigned_at.
	var d database.DeliveryOrder
	require.NoError(t, db.First(&d, deliveryID).Error)
	require.Equal(t, database.DeliveryStatusConfirmed, d.Status)
	require.Nil(t, d.DriverID)
	require.Nil(t, d.AssignedAt)

	// Driver row untouched: not busy, no current delivery.
	var drv database.DeliveryDriver
	require.NoError(t, db.First(&drv, driverID).Error)
	require.Nil(t, drv.CurrentDeliveryID)
	require.NotEqual(t, database.DriverStatusBusy, drv.Status)

	// No spurious "assigned" history row.
	var histCount int64
	require.NoError(t, db.Model(&database.DeliveryStatusHistory{}).
		Where("delivery_order_id = ? AND status = ?", deliveryID, database.DeliveryStatusAssigned).
		Count(&histCount).Error)
	require.Zero(t, histCount)
}

func TestAssignDriverStillAllowedFromMachinePermittedStates(t *testing.T) {
	for _, from := range []database.DeliveryStatus{
		database.DeliveryStatusPending,
		database.DeliveryStatusPreparing,
		database.DeliveryStatusReady,
	} {
		t.Run(string(from), func(t *testing.T) {
			db := setupDeliveryPerfTestDB(t, nil)
			require.NoError(t, db.AutoMigrate(&database.DeliveryStatusHistory{}))
			_, deliveryID, driverID := seedAssignableDelivery(t, db)
			require.NoError(t, db.Model(&database.DeliveryOrder{}).
				Where("id = ?", deliveryID).Update("status", from).Error)

			require.NoError(t, NewDeliveryService(db, nil).AssignDriver(deliveryID, driverID))

			var d database.DeliveryOrder
			require.NoError(t, db.First(&d, deliveryID).Error)
			require.Equal(t, database.DeliveryStatusAssigned, d.Status)
			require.NotNil(t, d.DriverID)
		})
	}
}

// ---------------------------------------------------------------------------
// DEL-AUTHZ-1 / DEL-SM-2 / DELIV-PERF-2: the dispatch LIST read must project
// the Driver preload exactly like the detail read — no total_earnings,
// license_number, vehicle_plate, or current_location for delivery:dispatch:read.
// ---------------------------------------------------------------------------

func TestGetBusinessDeliveriesDoesNotLeakDriverSensitiveFields(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	businessID := seedDeliveryListData(t, db, 5)

	// Widen the seeded driver with the sensitive columns that must not leak.
	locTS := time.Now()
	require.NoError(t, db.Model(&database.DeliveryDriver{}).
		Where("business_id = ?", businessID).
		Updates(map[string]interface{}{
			"email":             "dana@example.com",
			"license_number":    "LIC-SECRET-9",
			"vehicle_plate":     "PLATE-SECRET-7",
			"total_earnings":    9876.54,
			"current_latitude":  40.712800,
			"current_longitude": -74.006000,
			"current_timestamp": locTS,
		}).Error)

	result, err := NewDeliveryService(db, nil).GetBusinessDeliveries(businessID, DeliveryListParams{Limit: 5})
	require.NoError(t, err)
	require.NotEmpty(t, result.Deliveries)
	require.NotNil(t, result.Deliveries[0].Driver)

	payload, err := json.Marshal(result)
	require.NoError(t, err)
	body := string(payload)

	require.NotContains(t, body, "LIC-SECRET-9", "driver license_number must not leak on the dispatch list")
	require.NotContains(t, body, "PLATE-SECRET-7", "driver vehicle_plate must not leak on the dispatch list")
	require.NotContains(t, body, "9876.54", "driver total_earnings must not leak on the dispatch list")
	require.NotContains(t, body, "-74.006", "driver current_location must not leak on the dispatch list")

	// Columns the dispatch UI renders must survive the projection (parity with
	// preloadDeliveryDispatchDriverSummary on the detail read).
	require.Equal(t, "Dana Driver", result.Deliveries[0].Driver.Name)
	require.NotEmpty(t, result.Deliveries[0].Driver.Phone)
	require.NotEmpty(t, result.Deliveries[0].Driver.Email)
}

// ---------------------------------------------------------------------------
// DELIV-PERF-1: GetDriverPerformance must compute KPIs with SQL aggregates
// (like ListDriverPerformance) instead of hydrating the driver's entire
// all-time delivery history as full rows.
// ---------------------------------------------------------------------------

func seedDriverPerformanceHistory(t testing.TB, deliveredCount int) (businessID, driverID uint, recorder *deliverySQLRecorder, svc *DeliveryService) {
	t.Helper()
	recorder = &deliverySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupDeliveryPerfTestDB(t, recorder)

	business := &database.Business{
		BusinessId:     fmt.Sprintf("driver-perf-%s", t.Name()),
		Name:           "Driver Perf",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)

	driver := &database.DeliveryDriver{
		BusinessID:  business.ID,
		Name:        "Perf Driver",
		Phone:       "5550001111",
		VehicleType: database.VehicleTypeCar,
		Status:      database.DriverStatusOnline,
		IsAvailable: true,
		IsActive:    true,
	}
	require.NoError(t, db.Create(driver).Error)

	now := time.Now().UTC()
	orders := make([]database.DeliveryOrder, 0, deliveredCount+3)
	for i := 0; i < deliveredCount; i++ {
		assigned := now.Add(-time.Duration(i+3) * time.Hour)
		picked := assigned.Add(10 * time.Minute)
		delivered := assigned.Add(30 * time.Minute)
		estimated := assigned.Add(40 * time.Minute)
		rating := 5
		orders = append(orders, database.DeliveryOrder{
			BusinessID:            business.ID,
			BillID:                1,
			DriverID:              &driver.ID,
			DeliveryNumber:        fmt.Sprintf("DEL-PERF-%s-%05d", t.Name(), i),
			DeliveryType:          database.DeliveryTypeInHouse,
			Status:                database.DeliveryStatusDelivered,
			CustomerName:          "Customer",
			CustomerPhone:         "+1",
			DeliveryAddress:       database.DeliveryAddress{Street: "1 Main", City: "Anywhere"},
			DeliveryFee:           500,
			DriverTip:             100,
			AssignedAt:            &assigned,
			ActualPickupTime:      &picked,
			ActualDeliveryTime:    &delivered,
			EstimatedDeliveryTime: &estimated,
			CustomerRating:        &rating,
			QuoteMetadata:         database.JSONRawMessage(`{}`),
		})
	}
	// One cancelled, one failed, one active in-transit.
	orders = append(orders,
		database.DeliveryOrder{
			BusinessID: business.ID, BillID: 1, DriverID: &driver.ID,
			DeliveryNumber: fmt.Sprintf("DEL-PERF-%s-CAN", t.Name()),
			DeliveryType:   database.DeliveryTypeInHouse,
			Status:         database.DeliveryStatusCancelled,
			CustomerName:   "Customer", CustomerPhone: "+1",
			DeliveryAddress: database.DeliveryAddress{Street: "1 Main", City: "Anywhere"},
			QuoteMetadata:   database.JSONRawMessage(`{}`),
		},
		database.DeliveryOrder{
			BusinessID: business.ID, BillID: 1, DriverID: &driver.ID,
			DeliveryNumber: fmt.Sprintf("DEL-PERF-%s-FAIL", t.Name()),
			DeliveryType:   database.DeliveryTypeInHouse,
			Status:         database.DeliveryStatusFailed,
			CustomerName:   "Customer", CustomerPhone: "+1",
			DeliveryAddress: database.DeliveryAddress{Street: "1 Main", City: "Anywhere"},
			QuoteMetadata:   database.JSONRawMessage(`{}`),
		},
		database.DeliveryOrder{
			BusinessID: business.ID, BillID: 1, DriverID: &driver.ID,
			DeliveryNumber: fmt.Sprintf("DEL-PERF-%s-ACT", t.Name()),
			DeliveryType:   database.DeliveryTypeInHouse,
			Status:         database.DeliveryStatusInTransit,
			CustomerName:   "Active Customer", CustomerPhone: "+1",
			DeliveryAddress: database.DeliveryAddress{Street: "2 Main", City: "Anywhere"},
			DeliveryFee:     700,
			QuoteMetadata:   database.JSONRawMessage(`{}`),
		},
	)
	require.NoError(t, db.CreateInBatches(&orders, 200).Error)

	return business.ID, driver.ID, recorder, NewDeliveryService(db, nil)
}

func TestGetDriverPerformanceUsesAggregatesNotFullHistory(t *testing.T) {
	businessID, driverID, recorder, svc := seedDriverPerformanceHistory(t, 40)

	recorder.statements = nil
	dto, err := svc.GetDriverPerformance(businessID, driverID)
	require.NoError(t, err)

	// KPI correctness must match the seeded history.
	require.Equal(t, 40, dto.CompletedAllTime)
	require.Equal(t, 1, dto.CancelledCount)
	require.Equal(t, 1, dto.FailedCount)
	require.Equal(t, 1, dto.InProgressCount)
	require.Len(t, dto.ActiveQueue, 1)
	require.Equal(t, "Active Customer", dto.ActiveQueue[0].CustomerName)
	require.InDelta(t, 40*5.00, dto.GrossFeesCollected, 0.001)
	require.InDelta(t, 40*1.00, dto.GrossTipsCollected, 0.001)
	require.NotNil(t, dto.AvgPickupMinutes)
	require.InDelta(t, 10.0, *dto.AvgPickupMinutes, 0.01)
	require.NotNil(t, dto.AvgDeliveryMinutes)
	require.InDelta(t, 30.0, *dto.AvgDeliveryMinutes, 0.01)
	require.NotNil(t, dto.OnTimeRate)
	require.InDelta(t, 1.0, *dto.OnTimeRate, 0.001)
	require.NotNil(t, dto.AverageRating)
	require.InDelta(t, 5.0, *dto.AverageRating, 0.001)

	// Access shape: every SELECT against delivery_orders must be either the
	// SQL aggregate (SUM ...) or the bounded active-queue read (status IN ...).
	// A full-row, all-time history hydration has neither.
	sawAggregate := false
	for _, statement := range recorder.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") || !strings.Contains(normalized, "delivery_orders") {
			continue
		}
		isAggregate := strings.Contains(normalized, "sum(")
		isQueue := strings.Contains(normalized, "status in")
		if isAggregate {
			sawAggregate = true
		}
		require.True(t, isAggregate || isQueue,
			"GetDriverPerformance must not hydrate the full delivery history; offending query: %s", statement)
	}
	require.True(t, sawAggregate, "GetDriverPerformance must compute KPIs via a SQL aggregate query")
}

// BenchmarkGetDriverPerformanceSQLite measures the single-driver scorecard read
// over a 1000-delivery all-time history. Run with -benchmem before/after the
// aggregate rewrite (DELIV-PERF-1 perf gate).
func BenchmarkGetDriverPerformanceSQLite(b *testing.B) {
	businessID, driverID, _, svc := seedDriverPerformanceHistory(b, 1000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dto, err := svc.GetDriverPerformance(businessID, driverID)
		if err != nil {
			b.Fatal(err)
		}
		if dto.CompletedAllTime != 1000 {
			b.Fatalf("expected 1000 completed, got %d", dto.CompletedAllTime)
		}
	}
}

// ---------------------------------------------------------------------------
// DEL-PAY-05: staff CreateDeliveryOrder accepts a delivery_fee but never added
// it to the linked bill — the fee was never charged. It must land on the bill
// total in cents, exactly like the guest checkout path.
// ---------------------------------------------------------------------------

func TestCreateDeliveryOrderAddsFeeToBillTotal(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&database.DeliveryStatusHistory{}))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("fee-bill-%s", t.Name()),
		Name:           "Fee Bill",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)
	bill := &database.Bill{BusinessID: business.ID, BillNumber: "FEE-BILL-1", Status: database.BillStatusOpen, Items: "[]", TotalAmount: 2000}
	require.NoError(t, db.Create(bill).Error)

	svc := NewDeliveryService(db, nil)
	created, err := svc.CreateDeliveryOrder(CreateDeliveryOrderRequest{
		BusinessID:    business.ID,
		BillID:        bill.ID,
		CustomerName:  "Pat",
		CustomerPhone: "+15550001111",
		DeliveryFee:   5.50, // dollars on the wire
	})
	require.NoError(t, err)
	require.EqualValues(t, 550, created.DeliveryFee, "delivery order stores the fee in cents")

	var reloaded database.Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.EqualValues(t, 2550, reloaded.TotalAmount,
		"the delivery fee must be added to the linked bill total (cents) so it is actually charged")
}

func TestCreateDeliveryOrderZeroFeeLeavesBillUnchanged(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&database.DeliveryStatusHistory{}))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("fee-zero-%s", t.Name()),
		Name:           "Fee Zero",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)
	bill := &database.Bill{BusinessID: business.ID, BillNumber: "FEE-BILL-0", Status: database.BillStatusOpen, Items: "[]", TotalAmount: 2000}
	require.NoError(t, db.Create(bill).Error)

	_, err := NewDeliveryService(db, nil).CreateDeliveryOrder(CreateDeliveryOrderRequest{
		BusinessID:    business.ID,
		BillID:        bill.ID,
		CustomerName:  "Pat",
		CustomerPhone: "+15550001111",
	})
	require.NoError(t, err)

	var reloaded database.Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.EqualValues(t, 2000, reloaded.TotalAmount)
}
