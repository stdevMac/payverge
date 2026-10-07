package crm

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupCRMSettlementTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Customer{},
		&database.CustomerBusiness{},
		&database.Bill{},
		&database.CustomerVisit{},
		&database.LoyaltyProgram{},
		&database.LoyaltyTier{},
	))
	require.NoError(t, createSQLiteBillItemsTable(db))

	return db
}

func createSQLiteBillItemsTable(db *gorm.DB) error {
	return db.Exec(`
CREATE TABLE bill_items (
	id text PRIMARY KEY,
	bill_id integer NOT NULL,
	menu_item_id text DEFAULT '',
	name text NOT NULL,
	price real NOT NULL,
	quantity integer NOT NULL,
	options text,
	item_type text DEFAULT 'menu_item',
	bundle_id integer,
	parent_bundle_id integer,
	source_offer_id integer,
	order_id INTEGER,
	subtotal real NOT NULL,
	created_at datetime
)`).Error
}

func createCRMSettlementBusiness(t *testing.T, db *gorm.DB, crmEnabled bool) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:      fmt.Sprintf("biz-%d", time.Now().UnixNano()),
		OwnerAddress:    "0x1111111111111111111111111111111111111111",
		Name:            "Settlement Bistro",
		SettlementAddr:  "0x2222222222222222222222222222222222222222",
		TippingAddr:     "0x3333333333333333333333333333333333333333",
		IsActive:        true,
		CRMEnabled:      crmEnabled,
		DefaultCurrency: "USD",
	}
	require.NoError(t, db.Create(business).Error)
	return business
}

func createCRMSettlementCustomer(t *testing.T, db *gorm.DB) *database.Customer {
	t.Helper()

	customer := &database.Customer{
		Email:        fmt.Sprintf("customer-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Name:         "Settlement Customer",
		IsActive:     true,
	}
	require.NoError(t, db.Create(customer).Error)
	return customer
}

func createCRMSettlementConnection(t *testing.T, db *gorm.DB, customerID, businessID uint, active bool) *database.CustomerBusiness {
	t.Helper()

	connection := &database.CustomerBusiness{
		CustomerID:    customerID,
		BusinessID:    businessID,
		FirstVisitAt:  time.Now().Add(-24 * time.Hour),
		LoyaltyPoints: 7,
		TotalSpent:    12.25,
		VisitCount:    2,
		IsActive:      active,
	}
	require.NoError(t, db.Create(connection).Error)
	if !active {
		require.NoError(t, db.Model(connection).Update("is_active", false).Error)
		connection.IsActive = false
	}
	return connection
}

func createCRMSettlementBill(t *testing.T, db *gorm.DB, businessID uint, customerID *uint, status database.BillStatus, totalAmount int64, items string) *database.Bill {
	t.Helper()

	bill := &database.Bill{
		BusinessID:     businessID,
		TableID:        42,
		BillNumber:     fmt.Sprintf("PV-%d", time.Now().UnixNano()),
		Status:         status,
		Items:          items,
		TotalAmount:    totalAmount,
		PaidAmount:     totalAmount,
		SettlementAddr: "0x2222222222222222222222222222222222222222",
		TippingAddr:    "0x3333333333333333333333333333333333333333",
		CRMCustomerID:  customerID,
	}
	require.NoError(t, db.Create(bill).Error)
	return bill
}

func TestRecordBillSettlementVisitRejectsPersistedNonIncreasingLoyaltyThresholds(t *testing.T) {
	db := setupCRMSettlementTestDB(t)
	service := NewService(db)
	business := createCRMSettlementBusiness(t, db, true)
	program := &database.LoyaltyProgram{BusinessID: business.ID, Enabled: true, PointsPerDollar: 1}
	require.NoError(t, db.Create(program).Error)
	// Persist the invalid ladder directly to model the pre-existing live state.
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Bronze",
		MinLifetimeSpentCents: 25000,
		SortOrder:             0,
	}).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Silver",
		MinLifetimeSpentCents: 10000,
		SortOrder:             1,
	}).Error)

	customer := createCRMSettlementCustomer(t, db)
	connection := createCRMSettlementConnection(t, db, customer.ID, business.ID, true)
	bill := createCRMSettlementBill(t, db, business.ID, &customer.ID, database.BillStatusPaid, 4567, "")

	err := service.RecordBillSettlementVisit(bill.ID)

	require.ErrorIs(t, err, ErrInvalidLoyaltyTierLadder)
	require.EqualError(t, err, "load loyalty program for CRM settlement: invalid loyalty tier ladder")
	var visitCount int64
	require.NoError(t, db.Model(&database.CustomerVisit{}).Where("bill_id = ?", bill.ID).Count(&visitCount).Error)
	require.Zero(t, visitCount, "invalid persisted ladders must not create settlement visits")

	var reloaded database.CustomerBusiness
	require.NoError(t, db.First(&reloaded, connection.ID).Error)
	require.Equal(t, connection.VisitCount, reloaded.VisitCount)
	require.Equal(t, connection.LoyaltyPoints, reloaded.LoyaltyPoints)
	require.InDelta(t, connection.TotalSpent, reloaded.TotalSpent, 0.001)
}

func TestRecordBillSettlementVisitLocksCustomerBusinessBeforeTierAssignment(t *testing.T) {
	db := setupCRMSettlementTestDB(t)
	service := NewService(db)
	business := createCRMSettlementBusiness(t, db, true)
	program := &database.LoyaltyProgram{BusinessID: business.ID, Enabled: true, PointsPerDollar: 1}
	require.NoError(t, db.Create(program).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Bronze",
		MinLifetimeSpentCents: 0,
		SortOrder:             0,
	}).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Silver",
		MinLifetimeSpentCents: 10000,
		SortOrder:             1,
	}).Error)
	customer := createCRMSettlementCustomer(t, db)
	createCRMSettlementConnection(t, db, customer.ID, business.ID, true)
	bill := createCRMSettlementBill(t, db, business.ID, &customer.ID, database.BillStatusPaid, 10000, "")

	var observed, locked atomic.Bool
	callbackName := fmt.Sprintf("payverge:test:capture_customer_business_lock:%s", t.Name())
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if observed.Load() || tx.Statement == nil || tx.Statement.Schema == nil ||
			tx.Statement.Schema.Table != "customer_businesses" {
			return
		}
		observed.Store(true)
		if locking, ok := tx.Statement.Clauses["FOR"]; ok {
			if lock, ok := locking.Expression.(clause.Locking); ok && lock.Strength == "UPDATE" {
				locked.Store(true)
			}
		}
	}))
	defer func() { _ = db.Callback().Query().Remove(callbackName) }()

	require.NoError(t, service.RecordBillSettlementVisit(bill.ID))
	require.True(t, observed.Load(), "settlement must read the customer-business row")
	require.True(t, locked.Load(), "customer-business read must use FOR UPDATE to serialize tier assignment")
}

func TestRecordBillSettlementVisitRollsBackDuplicateRepairFailure(t *testing.T) {
	db := setupCRMSettlementTestDB(t)
	service := NewService(db)
	business := createCRMSettlementBusiness(t, db, true)
	program := &database.LoyaltyProgram{BusinessID: business.ID, Enabled: true, PointsPerDollar: 1}
	require.NoError(t, db.Create(program).Error)
	for _, tier := range []database.LoyaltyTier{
		{LoyaltyProgramID: program.ID, Name: "Bronze", MinLifetimeSpentCents: 0, SortOrder: 0},
		{LoyaltyProgramID: program.ID, Name: " bronze ", MinLifetimeSpentCents: 0, SortOrder: 1},
	} {
		require.NoError(t, db.Create(&tier).Error)
	}
	customer := createCRMSettlementCustomer(t, db)
	connection := createCRMSettlementConnection(t, db, customer.ID, business.ID, true)
	bill := createCRMSettlementBill(t, db, business.ID, &customer.ID, database.BillStatusPaid, 4567, "")

	callbackName := fmt.Sprintf("payverge:test:fail_loyalty_tier_delete:%s", t.Name())
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "loyalty_tiers" {
			tx.AddError(fmt.Errorf("forced loyalty repair delete failure"))
		}
	}))
	defer func() { _ = db.Callback().Delete().Remove(callbackName) }()

	err := service.RecordBillSettlementVisit(bill.ID)

	require.Error(t, err)
	require.Contains(t, err.Error(), "repair loyalty tier ladder")
	var tierCount int64
	require.NoError(t, db.Model(&database.LoyaltyTier{}).Where("loyalty_program_id = ?", program.ID).Count(&tierCount).Error)
	require.Equal(t, int64(2), tierCount, "failed duplicate repair must roll back its delete")
	var visitCount int64
	require.NoError(t, db.Model(&database.CustomerVisit{}).Where("bill_id = ?", bill.ID).Count(&visitCount).Error)
	require.Zero(t, visitCount)
	var reloaded database.CustomerBusiness
	require.NoError(t, db.First(&reloaded, connection.ID).Error)
	require.Equal(t, connection.VisitCount, reloaded.VisitCount)
}

func TestRecordBillSettlementVisitCreatesVisitAndStatsIdempotently(t *testing.T) {
	db := setupCRMSettlementTestDB(t)
	service := NewService(db)
	business := createCRMSettlementBusiness(t, db, true)
	// 1 point/$ baseline so points are awarded; matches legacy behavior pre-loyalty wiring.
	require.NoError(t, db.Create(&database.LoyaltyProgram{BusinessID: business.ID, Enabled: true, PointsPerDollar: 1}).Error)
	customer := createCRMSettlementCustomer(t, db)
	connection := createCRMSettlementConnection(t, db, customer.ID, business.ID, true)
	bill := createCRMSettlementBill(t, db, business.ID, &customer.ID, database.BillStatusPaid, 4567, `[{"legacy":"ignored"}]`)
	require.NoError(t, db.Create(&database.BillItem{
		ID:       "settlement-item-1",
		BillID:   bill.ID,
		Name:     "Margherita Pizza",
		Price:    23.45,
		Quantity: 2,
		Subtotal: 46.90,
	}).Error)

	require.NoError(t, service.RecordBillSettlementVisit(bill.ID))
	require.NoError(t, service.RecordBillSettlementVisit(bill.ID))

	var visits []database.CustomerVisit
	require.NoError(t, db.Where("customer_business_id = ? AND bill_id = ?", connection.ID, bill.ID).Find(&visits).Error)
	require.Len(t, visits, 1)
	require.Equal(t, connection.ID, visits[0].CustomerBusinessID)
	require.NotNil(t, visits[0].BillID)
	require.Equal(t, bill.ID, *visits[0].BillID)
	require.NotNil(t, visits[0].TableID)
	require.Equal(t, bill.TableID, *visits[0].TableID)
	require.InDelta(t, 45.67, visits[0].AmountSpent, 0.001)
	require.Equal(t, 45, visits[0].PointsEarned)
	require.NotZero(t, visits[0].VisitDate)

	var purchasedItems []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(visits[0].ItemsPurchased), &purchasedItems))
	require.Len(t, purchasedItems, 1)
	require.Equal(t, "Margherita Pizza", purchasedItems[0]["name"])

	var reloaded database.CustomerBusiness
	require.NoError(t, db.First(&reloaded, connection.ID).Error)
	require.InDelta(t, 57.92, reloaded.TotalSpent, 0.001)
	require.Equal(t, 3, reloaded.VisitCount)
	require.Equal(t, 52, reloaded.LoyaltyPoints)
	require.NotNil(t, reloaded.LastVisitAt)
}

func TestRecordBillSettlementVisitSkipsWhenBillAlreadyHasVisitForDifferentConnection(t *testing.T) {
	db := setupCRMSettlementTestDB(t)
	service := NewService(db)
	business := createCRMSettlementBusiness(t, db, true)
	customerA := createCRMSettlementCustomer(t, db)
	customerB := createCRMSettlementCustomer(t, db)
	connectionA := createCRMSettlementConnection(t, db, customerA.ID, business.ID, true)
	connectionB := createCRMSettlementConnection(t, db, customerB.ID, business.ID, true)
	bill := createCRMSettlementBill(t, db, business.ID, &customerB.ID, database.BillStatusPaid, 4567, "")
	billID := bill.ID
	require.NoError(t, db.Create(&database.CustomerVisit{
		CustomerBusinessID: connectionA.ID,
		BillID:             &billID,
		AmountSpent:        45.67,
		PointsEarned:       45,
		ItemsPurchased:     `[]`,
		VisitDate:          time.Now().Add(-time.Hour),
	}).Error)

	require.NoError(t, service.RecordBillSettlementVisit(bill.ID))

	var visitCount int64
	require.NoError(t, db.Model(&database.CustomerVisit{}).Where("bill_id = ?", bill.ID).Count(&visitCount).Error)
	require.EqualValues(t, 1, visitCount)

	var reloadedB database.CustomerBusiness
	require.NoError(t, db.First(&reloadedB, connectionB.ID).Error)
	require.InDelta(t, connectionB.TotalSpent, reloadedB.TotalSpent, 0.001)
	require.Equal(t, connectionB.VisitCount, reloadedB.VisitCount)
	require.Equal(t, connectionB.LoyaltyPoints, reloadedB.LoyaltyPoints)
	require.Nil(t, reloadedB.LastVisitAt)
}

func TestRecordBillSettlementVisitSkipsAnonymousBills(t *testing.T) {
	db := setupCRMSettlementTestDB(t)
	service := NewService(db)
	business := createCRMSettlementBusiness(t, db, true)
	bill := createCRMSettlementBill(t, db, business.ID, nil, database.BillStatusPaid, 1500, "")

	require.NoError(t, service.RecordBillSettlementVisit(bill.ID))

	var visitCount int64
	require.NoError(t, db.Model(&database.CustomerVisit{}).Count(&visitCount).Error)
	require.Zero(t, visitCount)
}

func TestRecordBillSettlementVisitSkipsOpenBills(t *testing.T) {
	db := setupCRMSettlementTestDB(t)
	service := NewService(db)
	business := createCRMSettlementBusiness(t, db, true)
	customer := createCRMSettlementCustomer(t, db)
	createCRMSettlementConnection(t, db, customer.ID, business.ID, true)
	bill := createCRMSettlementBill(t, db, business.ID, &customer.ID, database.BillStatusOpen, 1500, "")

	require.NoError(t, service.RecordBillSettlementVisit(bill.ID))

	var visitCount int64
	require.NoError(t, db.Model(&database.CustomerVisit{}).Count(&visitCount).Error)
	require.Zero(t, visitCount)
}

func TestRecordBillSettlementVisitSkipsCRMDisabledBusinesses(t *testing.T) {
	db := setupCRMSettlementTestDB(t)
	service := NewService(db)
	business := createCRMSettlementBusiness(t, db, false)
	customer := createCRMSettlementCustomer(t, db)
	connection := createCRMSettlementConnection(t, db, customer.ID, business.ID, true)
	bill := createCRMSettlementBill(t, db, business.ID, &customer.ID, database.BillStatusPaid, 1500, "")

	require.NoError(t, service.RecordBillSettlementVisit(bill.ID))

	var visitCount int64
	require.NoError(t, db.Model(&database.CustomerVisit{}).Count(&visitCount).Error)
	require.Zero(t, visitCount)

	var reloaded database.CustomerBusiness
	require.NoError(t, db.First(&reloaded, connection.ID).Error)
	require.Equal(t, connection.VisitCount, reloaded.VisitCount)
	require.InDelta(t, connection.TotalSpent, reloaded.TotalSpent, 0.001)
}

func TestRecordBillSettlementVisitSkipsMissingOrInactiveConnections(t *testing.T) {
	tests := []struct {
		name             string
		createConnection bool
		activeConnection bool
	}{
		{name: "missing connection", createConnection: false},
		{name: "inactive connection", createConnection: true, activeConnection: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupCRMSettlementTestDB(t)
			service := NewService(db)
			business := createCRMSettlementBusiness(t, db, true)
			customer := createCRMSettlementCustomer(t, db)
			if tt.createConnection {
				createCRMSettlementConnection(t, db, customer.ID, business.ID, tt.activeConnection)
			}
			bill := createCRMSettlementBill(t, db, business.ID, &customer.ID, database.BillStatusPaid, 1500, "")

			require.NoError(t, service.RecordBillSettlementVisit(bill.ID))

			var visitCount int64
			require.NoError(t, db.Model(&database.CustomerVisit{}).Count(&visitCount).Error)
			require.Zero(t, visitCount)
		})
	}
}

func TestRecordBillSettlementVisitItemsPurchasedFallbacks(t *testing.T) {
	tests := []struct {
		name          string
		billItemsJSON string
		wantJSON      string
	}{
		{
			name:          "preserves existing bill items JSON",
			billItemsJSON: `[{"name":"Legacy Salad","quantity":1}]`,
			wantJSON:      `[{"name":"Legacy Salad","quantity":1}]`,
		},
		{
			name:          "defaults to empty JSON array",
			billItemsJSON: "",
			wantJSON:      `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupCRMSettlementTestDB(t)
			service := NewService(db)
			business := createCRMSettlementBusiness(t, db, true)
			customer := createCRMSettlementCustomer(t, db)
			connection := createCRMSettlementConnection(t, db, customer.ID, business.ID, true)
			bill := createCRMSettlementBill(t, db, business.ID, &customer.ID, database.BillStatusPaid, 990, tt.billItemsJSON)

			require.NoError(t, service.RecordBillSettlementVisit(bill.ID))

			var visit database.CustomerVisit
			require.NoError(t, db.Where("customer_business_id = ? AND bill_id = ?", connection.ID, bill.ID).First(&visit).Error)
			require.JSONEq(t, tt.wantJSON, visit.ItemsPurchased)
		})
	}
}

func TestRecordBillSettlementVisitAwardsPointsAndUpdatesTier(t *testing.T) {
	db := setupCRMSettlementTestDB(t)
	service := NewService(db)
	business := createCRMSettlementBusiness(t, db, true)

	program := &database.LoyaltyProgram{BusinessID: business.ID, Enabled: true, PointsPerDollar: 2}
	require.NoError(t, db.Create(program).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{LoyaltyProgramID: program.ID, Name: "Bronze", MinLifetimeSpentCents: 0, SortOrder: 0}).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{LoyaltyProgramID: program.ID, Name: "Silver", MinLifetimeSpentCents: 20000, SortOrder: 1}).Error)

	customer := createCRMSettlementCustomer(t, db)
	// Existing lifetime spend of 150 dollars; new bill of 100 dollars → 250 lifetime → Silver.
	connection := &database.CustomerBusiness{
		CustomerID:    customer.ID,
		BusinessID:    business.ID,
		FirstVisitAt:  time.Now().Add(-48 * time.Hour),
		LoyaltyPoints: 0,
		TotalSpent:    150,
		VisitCount:    1,
		IsActive:      true,
	}
	require.NoError(t, db.Create(connection).Error)

	bill := createCRMSettlementBill(t, db, business.ID, &customer.ID, database.BillStatusPaid, 10000, "")

	require.NoError(t, service.RecordBillSettlementVisit(bill.ID))

	var updated database.CustomerBusiness
	require.NoError(t, db.First(&updated, connection.ID).Error)
	require.Equal(t, 200, updated.LoyaltyPoints, "100 spent × 2 pts/$ = 200")
	require.Equal(t, "Silver", updated.LoyaltyTier, "150 + 100 = 250 lifetime → Silver")

	var visit database.CustomerVisit
	require.NoError(t, db.Where("customer_business_id = ? AND bill_id = ?", connection.ID, bill.ID).First(&visit).Error)
	require.Equal(t, 200, visit.PointsEarned)
}

// TestRecordBillSettlementVisitTierUsesRoundedCents guards against float→cents
// truncation when computing the lifetime spend used for tier assignment. The
// stored TotalSpent is a float64 dollar amount; converting it with a bare
// int64(TotalSpent*100) truncates (e.g. 0.57 is stored as ~0.5699999999, so
// *100 = 56.9999… → 56, a cent short). At a tier boundary that denies a
// customer a tier they have legitimately earned.
func TestRecordBillSettlementVisitTierUsesRoundedCents(t *testing.T) {
	db := setupCRMSettlementTestDB(t)
	service := NewService(db)
	business := createCRMSettlementBusiness(t, db, true)

	program := &database.LoyaltyProgram{BusinessID: business.ID, Enabled: true, PointsPerDollar: 0}
	require.NoError(t, db.Create(program).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{LoyaltyProgramID: program.ID, Name: "Bronze", MinLifetimeSpentCents: 0, SortOrder: 0}).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{LoyaltyProgramID: program.ID, Name: "Silver", MinLifetimeSpentCents: 20000, SortOrder: 1}).Error)

	customer := createCRMSettlementCustomer(t, db)
	// Prior lifetime spend of exactly $0.57. In float64, 0.57*100 == 56.9999999…,
	// so int64(TotalSpent*100) truncates to 56 cents — one cent short of 57.
	connection := &database.CustomerBusiness{
		CustomerID:   customer.ID,
		BusinessID:   business.ID,
		FirstVisitAt: time.Now().Add(-48 * time.Hour),
		TotalSpent:   0.57,
		VisitCount:   1,
		IsActive:     true,
	}
	require.NoError(t, db.Create(connection).Error)

	// New bill of $199.43 → exact lifetime of $200.00 = 20000 cents → Silver.
	bill := createCRMSettlementBill(t, db, business.ID, &customer.ID, database.BillStatusPaid, 19943, "")

	require.NoError(t, service.RecordBillSettlementVisit(bill.ID))

	var updated database.CustomerBusiness
	require.NoError(t, db.First(&updated, connection.ID).Error)
	require.Equal(t, "Silver", updated.LoyaltyTier,
		"$0.57 + $199.43 = $200.00 must reach the 20000-cent Silver tier; float truncation leaves it at 19999")
}

func TestRecordBillSettlementVisitAwardsZeroPointsWhenProgramMissing(t *testing.T) {
	db := setupCRMSettlementTestDB(t)
	service := NewService(db)
	business := createCRMSettlementBusiness(t, db, true)
	customer := createCRMSettlementCustomer(t, db)
	connection := createCRMSettlementConnection(t, db, customer.ID, business.ID, true)
	bill := createCRMSettlementBill(t, db, business.ID, &customer.ID, database.BillStatusPaid, 4567, "")

	require.NoError(t, service.RecordBillSettlementVisit(bill.ID))

	var updated database.CustomerBusiness
	require.NoError(t, db.First(&updated, connection.ID).Error)
	// No loyalty program → 0 points awarded; tier remains blank.
	require.Equal(t, connection.LoyaltyPoints, updated.LoyaltyPoints)
	require.Equal(t, "", updated.LoyaltyTier)
}
