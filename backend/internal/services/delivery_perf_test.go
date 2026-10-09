package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type deliverySQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *deliverySQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *deliverySQLRecorder) selectCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func setupDeliveryPerfTestDB(t testing.TB, gormLogger logger.Interface) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	db, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	database.SetTestDB(db)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.Bill{},
		&database.Order{},
		&database.DeliveryDriver{},
		&database.DeliveryZone{},
		&database.DeliveryOrder{},
	))

	return db
}

func seedDeliveryListData(t testing.TB, db *gorm.DB, deliveryCount int) uint {
	t.Helper()

	business := &database.Business{
		BusinessId:     fmt.Sprintf("delivery-perf-%s", t.Name()),
		Name:           "Delivery Perf",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, db.Create(business).Error)

	driver := &database.DeliveryDriver{
		BusinessID:    business.ID,
		Name:          "Dana Driver",
		Phone:         "5550001111",
		VehicleType:   database.VehicleTypeCar,
		Status:        database.DriverStatusOnline,
		IsAvailable:   true,
		IsActive:      true,
		AverageRating: 4.8,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	require.NoError(t, db.Create(driver).Error)

	zone := &database.DeliveryZone{
		BusinessID:          business.ID,
		Name:                "Downtown",
		DeliveryFee:         499,
		MinimumOrderAmount:  1000,
		EstimatedTime:       30,
		CutoffBufferMinutes: 10,
		IsActive:            true,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}
	require.NoError(t, db.Create(zone).Error)

	base := time.Now().Add(-24 * time.Hour)
	bills := make([]database.Bill, 0, deliveryCount)
	for i := 0; i < deliveryCount; i++ {
		bills = append(bills, database.Bill{
			BusinessID:  business.ID,
			BillNumber:  fmt.Sprintf("BILL-%04d", i),
			Status:      database.BillStatusOpen,
			TotalAmount: int64(2500 + i),
			CreatedAt:   base.Add(time.Duration(i) * time.Minute),
			UpdatedAt:   base.Add(time.Duration(i) * time.Minute),
		})
	}
	require.NoError(t, db.Create(&bills).Error)

	orders := make([]database.Order, 0, deliveryCount)
	for i := 0; i < deliveryCount; i++ {
		orders = append(orders, database.Order{
			BusinessID:  business.ID,
			BillID:      bills[i].ID,
			OrderNumber: fmt.Sprintf("ORD-%04d", i),
			Status:      database.OrderStatusApproved,
			CreatedAt:   base.Add(time.Duration(i) * time.Minute),
			UpdatedAt:   base.Add(time.Duration(i) * time.Minute),
		})
	}
	require.NoError(t, db.Create(&orders).Error)

	deliveries := make([]database.DeliveryOrder, 0, deliveryCount)
	for i := 0; i < deliveryCount; i++ {
		orderID := orders[i].ID
		driverID := driver.ID
		zoneID := zone.ID
		deliveries = append(deliveries, database.DeliveryOrder{
			BusinessID:            business.ID,
			BillID:                bills[i].ID,
			OrderID:               &orderID,
			ZoneID:                &zoneID,
			DeliveryNumber:        fmt.Sprintf("DEL-%04d", i),
			DeliveryType:          database.DeliveryTypeInHouse,
			Status:                database.DeliveryStatusReady,
			Priority:              database.PriorityNormal,
			DriverID:              &driverID,
			CustomerName:          fmt.Sprintf("Customer %04d", i),
			CustomerPhone:         "5552223333",
			DeliveryAddress:       database.DeliveryAddress{Street: "1 Main", City: "Anywhere", Country: "US"},
			DeliveryFee:           499,
			ContactlessDelivery:   true,
			EstimatedDeliveryTime: deliveryPerfPtrTime(base.Add(time.Duration(i+45) * time.Minute)),
			DeliveryInstructions:  strings.Repeat("leave at door ", 16),
			CreatedAt:             base.Add(time.Duration(i) * time.Minute),
			UpdatedAt:             base.Add(time.Duration(i) * time.Minute),
			QuoteMetadata:         database.JSONRawMessage(`{}`),
			FulfillmentMode:       "in_house",
			ExternalTrackingURL:   fmt.Sprintf("https://tracking.example/%04d", i),
			CutoffAt:              deliveryPerfPtrTime(base.Add(time.Duration(i+30) * time.Minute)),
		})
	}
	require.NoError(t, db.Create(&deliveries).Error)
	return business.ID
}

func seedDeliveryDriversData(t testing.TB, db *gorm.DB, driverCount int) uint {
	t.Helper()

	business := &database.Business{
		BusinessId:     fmt.Sprintf("delivery-drivers-perf-%s", t.Name()),
		Name:           "Delivery Drivers Perf",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, db.Create(business).Error)

	staff := make([]database.Staff, 0, driverCount)
	for i := 0; i < driverCount; i++ {
		staff = append(staff, database.Staff{
			BusinessID:           business.ID,
			Email:                fmt.Sprintf("driver-%04d@example.com", i),
			Name:                 fmt.Sprintf("Driver Staff %04d", i),
			Role:                 database.StaffRoleServer,
			IsActive:             true,
			InvitedBy:            "0xowner",
			CustomPermissions:    strings.Repeat(`["delivery:dispatch:read","delivery:drivers:read"]`, 64),
			PinHash:              strings.Repeat("bcrypt-hash", 32),
			PermissionsUpdatedBy: "0xowner",
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		})
	}
	require.NoError(t, db.Create(&staff).Error)

	drivers := make([]database.DeliveryDriver, 0, driverCount)
	for i := 0; i < driverCount; i++ {
		staffID := staff[i].ID
		drivers = append(drivers, database.DeliveryDriver{
			BusinessID:          business.ID,
			StaffID:             &staffID,
			Name:                fmt.Sprintf("Driver %04d", i),
			Phone:               fmt.Sprintf("555%07d", i),
			Email:               fmt.Sprintf("driver-%04d@example.com", i),
			VehicleType:         database.VehicleTypeCar,
			VehiclePlate:        fmt.Sprintf("DRV%04d", i),
			Status:              database.DriverStatusOnline,
			IsAvailable:         i%3 != 0,
			IsActive:            true,
			TotalDeliveries:     i * 3,
			CompletedDeliveries: i * 2,
			AverageRating:       4.5,
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		})
	}
	require.NoError(t, db.Create(&drivers).Error)

	return business.ID
}

func deliveryPerfPtrTime(v time.Time) *time.Time {
	return &v
}

func TestGetBusinessDeliveriesSkipsUnusedBillAndOrderPreloads(t *testing.T) {
	recorder := &deliverySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupDeliveryPerfTestDB(t, recorder)
	businessID := seedDeliveryListData(t, db, 25)

	recorder.statements = nil
	result, err := NewDeliveryService(db, nil).GetBusinessDeliveries(businessID, DeliveryListParams{Limit: 20})
	require.NoError(t, err)
	require.Len(t, result.Deliveries, 20)
	require.NotNil(t, result.Deliveries[0].Driver)
	require.Zero(t, recorder.selectCount("bills"), "delivery list should not preload bills that the dispatch UI does not render")
	require.Zero(t, recorder.selectCount("orders"), "delivery list should not preload orders that the dispatch UI does not render")
}

func BenchmarkGetBusinessDeliveriesSQLite(b *testing.B) {
	db := setupDeliveryPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	businessID := seedDeliveryListData(b, db, 500)
	service := NewDeliveryService(db, nil)
	params := DeliveryListParams{Limit: 100}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := service.GetBusinessDeliveries(businessID, params)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Deliveries) != 100 {
			b.Fatalf("expected 100 deliveries, got %d", len(result.Deliveries))
		}
	}
}

func TestGetBusinessDriversSkipsUnusedStaffPreload(t *testing.T) {
	recorder := &deliverySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupDeliveryPerfTestDB(t, recorder)
	businessID := seedDeliveryDriversData(t, db, 25)

	recorder.statements = nil
	drivers, err := NewDeliveryService(db, nil).GetBusinessDrivers(businessID)
	require.NoError(t, err)
	require.Len(t, drivers, 25)
	require.NotNil(t, drivers[0].StaffID)
	require.Nil(t, drivers[0].Staff)
	require.Zero(t, recorder.selectCount("staff"), "driver list should not preload staff rows that the driver UI does not render")
}

func BenchmarkGetBusinessDriversSQLite(b *testing.B) {
	db := setupDeliveryPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	businessID := seedDeliveryDriversData(b, db, 500)
	service := NewDeliveryService(db, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		drivers, err := service.GetBusinessDrivers(businessID)
		if err != nil {
			b.Fatal(err)
		}
		if len(drivers) != 500 {
			b.Fatalf("expected 500 drivers, got %d", len(drivers))
		}
	}
}

func seedAssignableDelivery(t testing.TB, db *gorm.DB) (businessID, deliveryID, driverID uint) {
	t.Helper()

	business := &database.Business{
		BusinessId:     fmt.Sprintf("assign-perf-%s", t.Name()),
		Name:           "Assign Perf",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)

	bill := &database.Bill{BusinessID: business.ID, BillNumber: "ASSIGN-BILL", Status: database.BillStatusOpen, Items: "[]"}
	require.NoError(t, db.Create(bill).Error)

	driver := &database.DeliveryDriver{
		BusinessID:  business.ID,
		Name:        "Dana Driver",
		Phone:       "5550001111",
		VehicleType: database.VehicleTypeCar,
		Status:      database.DriverStatusOnline,
		IsAvailable: true,
		IsActive:    true,
	}
	require.NoError(t, db.Create(driver).Error)

	delivery := &database.DeliveryOrder{
		BusinessID:      business.ID,
		BillID:          bill.ID,
		DeliveryNumber:  fmt.Sprintf("DEL-ASSIGN-%s", t.Name()),
		DeliveryType:    database.DeliveryTypeInHouse,
		Status:          database.DeliveryStatusReady,
		FulfillmentMode: "in_house",
		CustomerName:    "Assign Customer",
		CustomerPhone:   "5552223333",
		DeliveryAddress: database.DeliveryAddress{Street: "1 Main", City: "Anywhere", Country: "US"},
		QuoteMetadata:   database.JSONRawMessage(`{}`),
	}
	require.NoError(t, db.Create(delivery).Error)

	return business.ID, delivery.ID, driver.ID
}

func TestAssignDriverRejectsAlreadyAssignedDelivery(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&database.DeliveryStatusHistory{}))
	_, deliveryID, driverID := seedAssignableDelivery(t, db)
	service := NewDeliveryService(db, nil)

	require.NoError(t, service.AssignDriver(deliveryID, driverID))

	// Second driver, also online — re-assigning the same delivery must conflict.
	other := &database.DeliveryDriver{
		BusinessID: func() uint { var d database.DeliveryOrder; db.First(&d, deliveryID); return d.BusinessID }(),
		Name:       "Other Driver", Phone: "5559998888", VehicleType: database.VehicleTypeCar,
		Status: database.DriverStatusOnline, IsAvailable: true, IsActive: true,
	}
	require.NoError(t, db.Create(other).Error)

	err := service.AssignDriver(deliveryID, other.ID)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrDeliveryAlreadyAssigned),
		"re-assigning an already-assigned delivery must return ErrDeliveryAlreadyAssigned")

	// The original assignment must be untouched.
	var reloaded database.DeliveryOrder
	require.NoError(t, db.First(&reloaded, deliveryID).Error)
	require.NotNil(t, reloaded.DriverID)
	require.Equal(t, driverID, *reloaded.DriverID)
}

// txCountingConnPool wraps a gorm.ConnPool and counts BeginTx calls. GORM
// issues BEGIN/COMMIT at the database/sql driver layer, not through the logger
// Trace callback, so we cannot count transactions by scanning logged SQL.
// Instead we intercept ConnPool.BeginTx — GORM calls it exactly once per
// db.Transaction / Begin, which is the deterministic single-transaction signal.
type txCountingConnPool struct {
	gorm.ConnPool
	beginner gorm.TxBeginner
	begins   *int
}

func (p *txCountingConnPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	*p.begins++
	return p.beginner.BeginTx(ctx, opts)
}

func TestAssignDriverWritesInSingleTransaction(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&database.DeliveryStatusHistory{}))
	_, deliveryID, driverID := seedAssignableDelivery(t, db)

	beginner, ok := db.Statement.ConnPool.(gorm.TxBeginner)
	require.True(t, ok, "underlying conn pool must support BeginTx to count transactions")
	begins := 0
	db.Statement.ConnPool = &txCountingConnPool{
		ConnPool: db.Statement.ConnPool,
		beginner: beginner,
		begins:   &begins,
	}

	require.NoError(t, NewDeliveryService(db, nil).AssignDriver(deliveryID, driverID))

	require.Equal(t, 1, begins, "assignment must run in exactly one transaction")

	// Delivery row reflects the assignment; driver row went busy — both within the tx.
	var d database.DeliveryOrder
	require.NoError(t, db.First(&d, deliveryID).Error)
	require.Equal(t, database.DeliveryStatusAssigned, d.Status)
	require.NotNil(t, d.DriverID)
	var drv database.DeliveryDriver
	require.NoError(t, db.First(&drv, driverID).Error)
	require.Equal(t, database.DriverStatusBusy, drv.Status)
	require.NotNil(t, drv.CurrentDeliveryID)
}

func BenchmarkAssignDriverSQLite(b *testing.B) {
	db := setupDeliveryPerfTestDB(b, logger.Default.LogMode(logger.Silent))
	require.NoError(b, db.AutoMigrate(&database.DeliveryStatusHistory{}))
	_, deliveryID, driverID := seedAssignableDelivery(b, db)
	service := NewDeliveryService(db, nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := service.AssignDriver(deliveryID, driverID); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		// Reset to an assignable state for the next iteration. UnassignDriver was
		// removed (DEL-SM-9), so reset the rows directly — this is bench plumbing,
		// not production behavior.
		if err := db.Model(&database.DeliveryOrder{}).Where("id = ?", deliveryID).
			Updates(map[string]interface{}{"driver_id": nil, "status": database.DeliveryStatusReady}).Error; err != nil {
			b.Fatal(err)
		}
		if err := db.Model(&database.DeliveryDriver{}).Where("id = ?", driverID).
			Updates(map[string]interface{}{"status": database.DriverStatusOnline, "current_delivery_id": nil}).Error; err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
}
