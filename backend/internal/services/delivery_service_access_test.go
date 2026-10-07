package services

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupDeliveryAccessTestDB builds an isolated in-memory DB with the full set of
// relations that GetDeliveryOrder preloads (Business, Bill, Order, Driver,
// Customer, StatusHistory) so the access-shape test can prove the wide Business
// row never reaches the dispatch detail response.
func setupDeliveryAccessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Order{},
		&database.DeliveryDriver{},
		&database.Customer{},
		&database.DeliveryOrder{},
		&database.DeliveryStatusHistory{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// seedDeliveryWithAllRelations creates a delivery order whose Business row carries
// the sensitive Stripe/onboarding fields that must NOT leak to dispatch staff,
// plus a Driver, Customer, Bill, Order and a status-history row.
func seedDeliveryWithAllRelations(t testing.TB, db *gorm.DB) database.DeliveryOrder {
	t.Helper()

	biz := database.Business{
		Name:            "Dispatch Diner",
		DefaultCurrency: "USD",
		DisplayCurrency: "USD",
		CustomURL:       "dispatch-diner",
		OnboardingState: database.JSONRawMessage(`{"step":"menu","secret":"do-not-leak"}`),
	}
	if err := db.Create(&biz).Error; err != nil {
		t.Fatalf("create business: %v", err)
	}

	bill := database.Bill{BusinessID: biz.ID, BillNumber: "BILL-ACCESS-1"}
	if err := db.Create(&bill).Error; err != nil {
		t.Fatalf("create bill: %v", err)
	}

	order := database.Order{BusinessID: biz.ID, BillID: bill.ID}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}

	driver := database.DeliveryDriver{
		BusinessID: biz.ID,
		Name:       "Dana Driver",
		Phone:      "+15550000001",
		Email:      "dana@example.com",
		IsActive:   true,
		Status:     database.DriverStatusOnline,
	}
	if err := db.Create(&driver).Error; err != nil {
		t.Fatalf("create driver: %v", err)
	}

	customer := database.Customer{
		Email:        "patron@example.com",
		Name:         "Pat Patron",
		Phone:        "+15550000002",
		PasswordHash: "should-never-serialize",
	}
	if err := db.Create(&customer).Error; err != nil {
		t.Fatalf("create customer: %v", err)
	}

	do := database.DeliveryOrder{
		BusinessID:     biz.ID,
		BillID:         bill.ID,
		OrderID:        &order.ID,
		DriverID:       &driver.ID,
		CustomerID:     &customer.ID,
		DeliveryNumber: "DEL-ACCESS-" + time.Now().Format("150405.000000000"),
		DeliveryType:   database.DeliveryTypeInHouse,
		Status:         database.DeliveryStatusPending,
		CustomerName:   "Pat Patron",
		CustomerPhone:  "+15550000002",
		DeliveryAddress: database.DeliveryAddress{
			Street: "1 Dispatch Way",
			City:   "Dispatchville",
		},
		QuoteMetadata: database.JSONRawMessage("{}"),
	}
	if err := db.Create(&do).Error; err != nil {
		t.Fatalf("create delivery order: %v", err)
	}

	hist := database.DeliveryStatusHistory{
		DeliveryOrderID: do.ID,
		Status:          database.DeliveryStatusPending,
		Notes:           "Order created",
		ChangedBy:       "system",
	}
	if err := db.Create(&hist).Error; err != nil {
		t.Fatalf("create status history: %v", err)
	}

	return do
}

// TestGetDeliveryOrder_DoesNotLeakPrivateBusinessFields is the load-bearing access-shape
// guard for PRELOAD-02: the operator dispatch detail read must NOT hydrate the
// Business row's onboarding_state blob, while still keeping the
// display columns the dispatch UI relies on (name, currency).
func TestGetDeliveryOrder_DoesNotLeakPrivateBusinessFields(t *testing.T) {
	db := setupDeliveryAccessTestDB(t)
	seeded := seedDeliveryWithAllRelations(t, db)

	svc := &DeliveryService{db: db}
	got, err := svc.GetDeliveryOrder(seeded.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Business)

	// Sensitive financial / onboarding fields must be empty (never projected).
	require.Empty(t, string(got.Business.OnboardingState), "onboarding_state blob must not leak to dispatch staff")

	// Display columns the dispatch UI renders must survive the projection.
	require.Equal(t, "Dispatch Diner", got.Business.Name)
	require.Equal(t, "USD", got.Business.DefaultCurrency)

	// Driver is projected but must still carry the columns the operator UI reads.
	require.NotNil(t, got.Driver)
	require.Equal(t, "Dana Driver", got.Driver.Name)

	// Customer, when projected, must not over-fetch the credential column.
	if got.Customer != nil {
		require.Empty(t, got.Customer.PasswordHash, "customer password hash must never be hydrated")
	}
}

// BenchmarkGetDeliveryOrder measures the dispatch detail read over a fully
// related delivery. Run with -benchmem; compared before/after the projection
// to record the over-fetch reduction (PRELOAD-02 perf gate).
func BenchmarkGetDeliveryOrder(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Order{},
		&database.DeliveryDriver{},
		&database.Customer{},
		&database.DeliveryOrder{},
		&database.DeliveryStatusHistory{},
	); err != nil {
		b.Fatalf("migrate: %v", err)
	}

	seeded := seedDeliveryWithAllRelations(b, db)

	svc := &DeliveryService{db: db}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := svc.GetDeliveryOrder(seeded.ID)
		if err != nil {
			b.Fatalf("get: %v", err)
		}
		if got == nil {
			b.Fatal("nil order")
		}
	}
}
