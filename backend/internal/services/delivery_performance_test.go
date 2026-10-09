package services

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPerformanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Per-test isolated in-memory DB. cache=shared would leak state across tests.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.Business{},
		&database.DeliveryDriver{},
		&database.DeliveryOrder{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	biz := database.Business{Name: "Perf Test", Email: "perf@example.com"}
	biz.ID = 200
	db.Create(&biz)
	database.SetTestDB(db)
	return db
}

func makePerfDriver(t *testing.T, db *gorm.DB, name string) database.DeliveryDriver {
	t.Helper()
	d := database.DeliveryDriver{
		BusinessID: 200,
		Name:       name,
		Phone:      "+1",
		IsActive:   true,
		Status:     database.DriverStatusOnline,
	}
	if err := db.Create(&d).Error; err != nil {
		t.Fatalf("create driver: %v", err)
	}
	return d
}

func makePerfOrder(t *testing.T, db *gorm.DB, driverID uint, status database.DeliveryStatus, fields func(*database.DeliveryOrder)) database.DeliveryOrder {
	t.Helper()
	id := driverID
	o := database.DeliveryOrder{
		BusinessID:     200,
		BillID:         1,
		DriverID:       &id,
		DeliveryNumber: "DEL-" + string(status) + "-" + name(t) + "-" + time.Now().Format("150405.000000000"),
		Status:         status,
		CustomerName:   "Customer",
		CustomerPhone:  "+1",
		DeliveryAddress: database.DeliveryAddress{
			Street: "1 Test St",
			City:   "Testville",
		},
		QuoteMetadata: database.JSONRawMessage("{}"),
	}
	if fields != nil {
		fields(&o)
	}
	if err := db.Create(&o).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}
	return o
}

// name returns the test name for unique delivery numbers.
func name(t *testing.T) string {
	t.Helper()
	return t.Name()
}

func ptrTime(v time.Time) *time.Time {
	return &v
}

func ptrInt(v int) *int {
	return &v
}

func TestComputeDriverPerformance_EmptyDriver(t *testing.T) {
	db := setupPerformanceTestDB(t)
	driver := makePerfDriver(t, db, "Empty")

	svc := &DeliveryService{db: db}
	dto, err := svc.computeDriverPerformance(&driver, time.Now().UTC())
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if dto.CompletedAllTime != 0 || dto.InProgressCount != 0 || dto.CancelledCount != 0 {
		t.Errorf("counters should be zero, got %+v", dto)
	}
	if dto.AvgPickupMinutes != nil || dto.AvgDeliveryMinutes != nil || dto.OnTimeRate != nil || dto.AverageRating != nil {
		t.Errorf("nil samples should yield nil pointers, got %+v", dto)
	}
	if dto.GrossFeesCollected != 0 || dto.GrossTipsCollected != 0 {
		t.Errorf("gross totals should be zero")
	}
}

func TestComputeDriverPerformance_CountsAndAggregates(t *testing.T) {
	db := setupPerformanceTestDB(t)
	driver := makePerfDriver(t, db, "Aggregate")

	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startOfDay := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	yesterday := startOfDay.AddDate(0, 0, -1).Add(8 * time.Hour)
	sixDaysAgo := startOfDay.AddDate(0, 0, -6).Add(10 * time.Hour)
	twoWeeksAgo := startOfDay.AddDate(0, 0, -14).Add(8 * time.Hour)

	// Delivered today on-time, 30 min total, 5min pickup, 4-star rating, $5 fee + $2 tip
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		assigned := startOfDay.Add(11 * time.Hour)
		pickup := assigned.Add(5 * time.Minute)
		delivered := assigned.Add(30 * time.Minute)
		eta := assigned.Add(35 * time.Minute) // 5 min slack — on time
		o.AssignedAt = &assigned
		o.ActualPickupTime = &pickup
		o.ActualDeliveryTime = &delivered
		o.EstimatedDeliveryTime = &eta
		o.DeliveryFee = 500
		o.DriverTip = 200
		o.CustomerRating = ptrInt(4)
	})

	// Delivered yesterday late, 50 min, 10min pickup, no rating, $7 fee
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		assigned := yesterday
		pickup := assigned.Add(10 * time.Minute)
		delivered := assigned.Add(50 * time.Minute)
		eta := assigned.Add(40 * time.Minute) // 10 min late
		o.AssignedAt = &assigned
		o.ActualPickupTime = &pickup
		o.ActualDeliveryTime = &delivered
		o.EstimatedDeliveryTime = &eta
		o.DeliveryFee = 700
	})

	// Delivered 6 days ago — within week, 40 min, 5-star rating
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		assigned := sixDaysAgo
		pickup := assigned.Add(8 * time.Minute)
		delivered := assigned.Add(40 * time.Minute)
		eta := assigned.Add(45 * time.Minute) // on time
		o.AssignedAt = &assigned
		o.ActualPickupTime = &pickup
		o.ActualDeliveryTime = &delivered
		o.EstimatedDeliveryTime = &eta
		o.CustomerRating = ptrInt(5)
	})

	// Delivered 14 days ago — beyond week
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		assigned := twoWeeksAgo
		delivered := assigned.Add(45 * time.Minute)
		o.AssignedAt = &assigned
		o.ActualDeliveryTime = &delivered
	})

	// Active — assigned, no pickup yet
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusAssigned, func(o *database.DeliveryOrder) {
		assigned := now.Add(-10 * time.Minute)
		o.AssignedAt = &assigned
		o.DeliveryFee = 600
	})

	// Active — picked up
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusPickedUp, func(o *database.DeliveryOrder) {
		assigned := now.Add(-15 * time.Minute)
		pickup := assigned.Add(5 * time.Minute)
		o.AssignedAt = &assigned
		o.ActualPickupTime = &pickup
	})

	// Cancelled — should count toward cancellations only
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusCancelled, func(o *database.DeliveryOrder) {
		assigned := yesterday
		o.AssignedAt = &assigned
		o.CancellationReason = "customer changed mind"
	})

	svc := &DeliveryService{db: db}
	dto, err := svc.computeDriverPerformance(&driver, now)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	if dto.CompletedAllTime != 4 {
		t.Errorf("CompletedAllTime: want 4, got %d", dto.CompletedAllTime)
	}
	if dto.CompletedToday != 1 {
		t.Errorf("CompletedToday: want 1, got %d", dto.CompletedToday)
	}
	if dto.CompletedWeek != 3 {
		t.Errorf("CompletedWeek: want 3 (today + yesterday + 6d), got %d", dto.CompletedWeek)
	}
	if dto.CancelledCount != 1 {
		t.Errorf("CancelledCount: want 1, got %d", dto.CancelledCount)
	}
	if dto.InProgressCount != 2 {
		t.Errorf("InProgressCount: want 2, got %d", dto.InProgressCount)
	}
	if len(dto.ActiveQueue) != 2 {
		t.Errorf("ActiveQueue length: want 2, got %d", len(dto.ActiveQueue))
	}

	// Money: only delivered orders. Fees 500 + 700 + 0 + 0 = 1200 cents = $12.00
	if dto.GrossFeesCollected != 12.00 {
		t.Errorf("GrossFeesCollected: want 12.00, got %.2f", dto.GrossFeesCollected)
	}
	if dto.GrossTipsCollected != 2.00 {
		t.Errorf("GrossTipsCollected: want 2.00, got %.2f", dto.GrossTipsCollected)
	}

	// Avg pickup: (5 + 10 + 8) / 3 = 7.667 min (4th delivery has no pickup — excluded)
	if dto.AvgPickupMinutes == nil {
		t.Fatal("AvgPickupMinutes should not be nil")
	}
	if v := *dto.AvgPickupMinutes; v < 7.6 || v > 7.7 {
		t.Errorf("AvgPickupMinutes: want ~7.667, got %.3f", v)
	}

	// Avg delivery: (30 + 50 + 40 + 45) / 4 = 41.25 min
	if dto.AvgDeliveryMinutes == nil {
		t.Fatal("AvgDeliveryMinutes should not be nil")
	}
	if v := *dto.AvgDeliveryMinutes; v < 41.2 || v > 41.3 {
		t.Errorf("AvgDeliveryMinutes: want ~41.25, got %.3f", v)
	}

	// On-time: 3 samples (today + 6d + yesterday). 2 on-time → 0.6667
	if dto.OnTimeRate == nil {
		t.Fatal("OnTimeRate should not be nil")
	}
	if v := *dto.OnTimeRate; v < 0.66 || v > 0.67 {
		t.Errorf("OnTimeRate: want ~0.667, got %.3f", v)
	}

	// Rating: (4 + 5) / 2 = 4.5
	if dto.AverageRating == nil {
		t.Fatal("AverageRating should not be nil")
	}
	if v := *dto.AverageRating; v != 4.5 {
		t.Errorf("AverageRating: want 4.5, got %.2f", v)
	}
}

func TestListDriverPerformance_SortedByCompletedWeek(t *testing.T) {
	db := setupPerformanceTestDB(t)
	now := time.Now().UTC()

	alice := makePerfDriver(t, db, "Alice")
	bob := makePerfDriver(t, db, "Bob")
	carol := makePerfDriver(t, db, "Carol")

	// Alice: 1 delivery this week
	makePerfOrder(t, db, alice.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		when := now.Add(-2 * 24 * time.Hour)
		o.AssignedAt = &when
		o.ActualDeliveryTime = &when
	})
	// Bob: 3 deliveries this week
	for i := 0; i < 3; i++ {
		makePerfOrder(t, db, bob.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
			when := now.Add(-time.Duration(i+1) * 24 * time.Hour)
			o.AssignedAt = &when
			o.ActualDeliveryTime = &when
		})
	}
	// Carol: 0 deliveries this week, 1 ancient one
	makePerfOrder(t, db, carol.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		when := now.Add(-30 * 24 * time.Hour)
		o.AssignedAt = &when
		o.ActualDeliveryTime = &when
	})

	svc := &DeliveryService{db: db}
	list, err := svc.ListDriverPerformance(200)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("want 3 entries, got %d", len(list))
	}
	if list[0].DriverName != "Bob" {
		t.Errorf("expected Bob first (3 this week), got %q", list[0].DriverName)
	}
	if list[1].DriverName != "Alice" {
		t.Errorf("expected Alice second (1 this week), got %q", list[1].DriverName)
	}
	if list[2].DriverName != "Carol" {
		t.Errorf("expected Carol last (0 this week), got %q", list[2].DriverName)
	}
}

func TestListDriverPerformanceAggregatesHistoryAndActiveQueue(t *testing.T) {
	db := setupPerformanceTestDB(t)
	driver := makePerfDriver(t, db, "Driver A")
	now := time.Now().UTC()

	makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		assigned := now.AddDate(0, 0, -20)
		delivered := assigned.Add(35 * time.Minute)
		o.AssignedAt = &assigned
		o.ActualDeliveryTime = &delivered
	})
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		delivered := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
		assigned := delivered.Add(-20 * time.Minute)
		o.AssignedAt = &assigned
		o.ActualDeliveryTime = &delivered
	})
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusCancelled, nil)
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusFailed, nil)
	active := makePerfOrder(t, db, driver.ID, database.DeliveryStatusInTransit, func(o *database.DeliveryOrder) {
		assigned := now.Add(-15 * time.Minute)
		o.AssignedAt = &assigned
	})

	service := &DeliveryService{db: db}
	performance, err := service.ListDriverPerformance(200)
	require.NoError(t, err)
	require.Len(t, performance, 1)

	dto := performance[0]
	require.Equal(t, 2, dto.CompletedAllTime)
	require.Equal(t, 1, dto.CompletedToday)
	require.Equal(t, 1, dto.CompletedWeek)
	require.Equal(t, 1, dto.CancelledCount)
	require.Equal(t, 1, dto.FailedCount)
	require.Equal(t, 1, dto.InProgressCount)
	require.Len(t, dto.ActiveQueue, 1)
	require.Equal(t, active.ID, dto.ActiveQueue[0].DeliveryID)
}

func TestListDriverPerformanceExcludesNonPositiveDurationsFromAverages(t *testing.T) {
	db := setupPerformanceTestDB(t)
	driver := makePerfDriver(t, db, "Driver A")
	now := time.Now().UTC()

	makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		assigned := now.Add(-2 * time.Hour)
		pickup := assigned.Add(10 * time.Minute)
		delivered := assigned.Add(30 * time.Minute)
		o.AssignedAt = &assigned
		o.ActualPickupTime = &pickup
		o.ActualDeliveryTime = &delivered
	})
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		assigned := now.Add(-90 * time.Minute)
		o.AssignedAt = &assigned
		o.ActualPickupTime = &assigned
		o.ActualDeliveryTime = &assigned
	})
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		assigned := now.Add(-60 * time.Minute)
		pickup := assigned.Add(-5 * time.Minute)
		delivered := assigned.Add(-10 * time.Minute)
		o.AssignedAt = &assigned
		o.ActualPickupTime = &pickup
		o.ActualDeliveryTime = &delivered
	})

	service := &DeliveryService{db: db}
	performance, err := service.ListDriverPerformance(200)
	require.NoError(t, err)
	require.Len(t, performance, 1)

	dto := performance[0]
	require.NotNil(t, dto.AvgPickupMinutes)
	require.NotNil(t, dto.AvgDeliveryMinutes)
	require.InDelta(t, 10.0, *dto.AvgPickupMinutes, 0.001)
	require.InDelta(t, 30.0, *dto.AvgDeliveryMinutes, 0.001)
}

func TestListDriverPerformance_LoadsDeliveriesInBatch(t *testing.T) {
	db := setupPerformanceTestDB(t)
	now := time.Now().UTC()

	for _, name := range []string{"Alice", "Bob", "Carol"} {
		driver := makePerfDriver(t, db, name)
		makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
			o.AssignedAt = ptrTime(now.Add(-time.Hour))
			o.ActualDeliveryTime = ptrTime(now)
		})
	}

	queryCount := 0
	callbackName := "payverge:test_delivery_performance_query_counter"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		queryCount++
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
	})

	svc := &DeliveryService{db: db}
	list, err := svc.ListDriverPerformance(200)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("want 3 entries, got %d", len(list))
	}
	if queryCount > 3 {
		t.Fatalf("driver performance should load drivers, business timezone, and deliveries in a bounded query set; got %d queries", queryCount)
	}
}

func TestGetDriverPerformance_NotFound(t *testing.T) {
	db := setupPerformanceTestDB(t)
	svc := &DeliveryService{db: db}

	_, err := svc.GetDriverPerformance(200, 9999)
	if err == nil {
		t.Fatal("expected error for missing driver")
	}
}

func TestComputeDriverPerformance_TimezoneAwareBoundary(t *testing.T) {
	db := setupPerformanceTestDB(t)
	driver := makePerfDriver(t, db, "TZDriver")

	// Simulated "now" in Asia/Dubai (UTC+4): Sunday 2026-05-17 02:00 local =
	// Saturday 2026-05-16 22:00 UTC. A delivery completed at 03:00 UTC on
	// 2026-05-17 (07:00 local Sunday) must count as "today" in Dubai but
	// would fall outside today if we naively used UTC midnight.
	dubai, err := time.LoadLocation("Asia/Dubai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := time.Date(2026, 5, 17, 2, 0, 0, 0, dubai)

	// Delivered 03:00 UTC on 2026-05-17 = 07:00 Dubai. Should count as today.
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
		assigned := time.Date(2026, 5, 17, 6, 30, 0, 0, dubai)
		delivered := time.Date(2026, 5, 17, 7, 0, 0, 0, dubai)
		o.AssignedAt = &assigned
		o.ActualDeliveryTime = &delivered
	})

	svc := &DeliveryService{db: db}
	dto, err := svc.computeDriverPerformance(&driver, now)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if dto.CompletedToday != 1 {
		t.Errorf("CompletedToday should be 1 with Dubai-local boundaries, got %d", dto.CompletedToday)
	}
}

func TestComputeDriverPerformance_MoneySumExactInCents(t *testing.T) {
	db := setupPerformanceTestDB(t)
	driver := makePerfDriver(t, db, "MoneyDriver")
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

	// Three deliveries with prices that lose precision when summed as floats:
	// $5.99 + $5.99 + $5.99 = $17.97 exactly only when summed as cents.
	for i := 0; i < 3; i++ {
		makePerfOrder(t, db, driver.ID, database.DeliveryStatusDelivered, func(o *database.DeliveryOrder) {
			assigned := now.Add(-time.Hour * time.Duration(i+1))
			delivered := assigned.Add(15 * time.Minute)
			o.AssignedAt = &assigned
			o.ActualDeliveryTime = &delivered
			o.DeliveryFee = 599
			o.DriverTip = 101
		})
	}

	svc := &DeliveryService{db: db}
	dto, err := svc.computeDriverPerformance(&driver, now)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if dto.GrossFeesCollected != 17.97 {
		t.Errorf("GrossFeesCollected should be exactly 17.97, got %.10f", dto.GrossFeesCollected)
	}
	if dto.GrossTipsCollected != 3.03 {
		t.Errorf("GrossTipsCollected should be exactly 3.03, got %.10f", dto.GrossTipsCollected)
	}
}

func TestComputeDriverPerformance_FailedDeliveriesCounted(t *testing.T) {
	db := setupPerformanceTestDB(t)
	driver := makePerfDriver(t, db, "FailMode")
	now := time.Now().UTC()

	makePerfOrder(t, db, driver.ID, database.DeliveryStatusFailed, func(o *database.DeliveryOrder) {
		o.AssignedAt = ptrTime(now.Add(-2 * time.Hour))
	})
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusFailed, func(o *database.DeliveryOrder) {
		o.AssignedAt = ptrTime(now.Add(-1 * time.Hour))
	})
	makePerfOrder(t, db, driver.ID, database.DeliveryStatusCancelled, nil)

	svc := &DeliveryService{db: db}
	dto, err := svc.computeDriverPerformance(&driver, now)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if dto.FailedCount != 2 {
		t.Errorf("FailedCount: want 2, got %d", dto.FailedCount)
	}
	if dto.CancelledCount != 1 {
		t.Errorf("CancelledCount: want 1, got %d", dto.CancelledCount)
	}
	if dto.CompletedAllTime != 0 {
		t.Errorf("CompletedAllTime: want 0, got %d", dto.CompletedAllTime)
	}
}

func TestQueueItemFromOrder_AddressShortening(t *testing.T) {
	if got := shortenAddress("", ""); got != "" {
		t.Errorf("empty/empty: got %q", got)
	}
	if got := shortenAddress("1 Main", ""); got != "1 Main" {
		t.Errorf("street only: got %q", got)
	}
	if got := shortenAddress("", "City"); got != "City" {
		t.Errorf("city only: got %q", got)
	}
	if got := shortenAddress("1 Main", "City"); got != "1 Main, City" {
		t.Errorf("both: got %q", got)
	}
}
