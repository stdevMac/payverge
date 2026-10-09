package jobs

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupCRMSettlementReconciliationTestDB(t *testing.T) *gorm.DB {
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
	require.NoError(t, createSQLiteReconciliationBillItemsTable(db))

	return db
}

func createSQLiteReconciliationBillItemsTable(db *gorm.DB) error {
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

func createCRMSettlementReconciliationBusiness(t *testing.T, db *gorm.DB) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:     fmt.Sprintf("reconcile-biz-%d", time.Now().UnixNano()),
		OwnerAddress:   "0x1111111111111111111111111111111111111111",
		Name:           "Reconcile Bistro",
		SettlementAddr: "0x2222222222222222222222222222222222222222",
		TippingAddr:    "0x3333333333333333333333333333333333333333",
		IsActive:       true,
		CRMEnabled:     true,
	}
	require.NoError(t, db.Create(business).Error)
	// Mirror the production loyalty migration backfill: every business has a
	// default 1 pt/$ program. Without this, settlement awards 0 points and
	// reconciliation drops behind production behavior.
	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:      business.ID,
		Enabled:         true,
		PointsPerDollar: 1,
	}).Error)
	return business
}

func createCRMSettlementReconciliationCustomer(t *testing.T, db *gorm.DB) *database.Customer {
	t.Helper()

	customer := &database.Customer{
		Email:        fmt.Sprintf("reconcile-customer-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Name:         "Reconcile Customer",
		IsActive:     true,
	}
	require.NoError(t, db.Create(customer).Error)
	return customer
}

func createCRMSettlementReconciliationConnection(t *testing.T, db *gorm.DB, customerID, businessID uint) *database.CustomerBusiness {
	t.Helper()

	connection := &database.CustomerBusiness{
		CustomerID:   customerID,
		BusinessID:   businessID,
		FirstVisitAt: time.Now().Add(-24 * time.Hour),
		IsActive:     true,
	}
	require.NoError(t, db.Create(connection).Error)
	return connection
}

func TestReconcileCRMSettlementVisitsCreatesMissingVisitsIdempotently(t *testing.T) {
	db := setupCRMSettlementReconciliationTestDB(t)
	business := createCRMSettlementReconciliationBusiness(t, db)
	customer := createCRMSettlementReconciliationCustomer(t, db)
	connection := createCRMSettlementReconciliationConnection(t, db, customer.ID, business.ID)
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        7,
		BillNumber:     fmt.Sprintf("PV-RECONCILE-%d", time.Now().UnixNano()),
		Status:         database.BillStatusPaid,
		Items:          `[{"name":"Legacy Soup","quantity":1}]`,
		TotalAmount:    3250,
		PaidAmount:     3250,
		SettlementAddr: "0x2222222222222222222222222222222222222222",
		TippingAddr:    "0x3333333333333333333333333333333333333333",
		CRMCustomerID:  &customer.ID,
	}
	require.NoError(t, db.Create(bill).Error)

	ReconcileCRMSettlementVisits(db, 100)
	ReconcileCRMSettlementVisits(db, 100)

	var visits []database.CustomerVisit
	require.NoError(t, db.Where("customer_business_id = ? AND bill_id = ?", connection.ID, bill.ID).Find(&visits).Error)
	require.Len(t, visits, 1)
	require.InDelta(t, 32.50, visits[0].AmountSpent, 0.001)

	var reloaded database.CustomerBusiness
	require.NoError(t, db.First(&reloaded, connection.ID).Error)
	require.Equal(t, 1, reloaded.VisitCount)
	require.InDelta(t, 32.50, reloaded.TotalSpent, 0.001)
	require.Equal(t, 32, reloaded.LoyaltyPoints)
}

func TestReconcileCRMSettlementVisitsSkipsBillsWithVisitForDifferentConnection(t *testing.T) {
	db := setupCRMSettlementReconciliationTestDB(t)
	business := createCRMSettlementReconciliationBusiness(t, db)
	customerA := createCRMSettlementReconciliationCustomer(t, db)
	customerB := createCRMSettlementReconciliationCustomer(t, db)
	connectionA := createCRMSettlementReconciliationConnection(t, db, customerA.ID, business.ID)
	connectionB := createCRMSettlementReconciliationConnection(t, db, customerB.ID, business.ID)
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        7,
		BillNumber:     fmt.Sprintf("PV-RECONCILE-CROSS-%d", time.Now().UnixNano()),
		Status:         database.BillStatusPaid,
		Items:          `[]`,
		TotalAmount:    3250,
		PaidAmount:     3250,
		SettlementAddr: "0x2222222222222222222222222222222222222222",
		TippingAddr:    "0x3333333333333333333333333333333333333333",
		CRMCustomerID:  &customerB.ID,
	}
	require.NoError(t, db.Create(bill).Error)
	billID := bill.ID
	require.NoError(t, db.Create(&database.CustomerVisit{
		CustomerBusinessID: connectionA.ID,
		BillID:             &billID,
		AmountSpent:        32.50,
		PointsEarned:       32,
		ItemsPurchased:     `[]`,
		VisitDate:          time.Now().Add(-time.Hour),
	}).Error)

	ReconcileCRMSettlementVisits(db, 100)

	var visits []database.CustomerVisit
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&visits).Error)
	require.Len(t, visits, 1)

	var reloadedB database.CustomerBusiness
	require.NoError(t, db.First(&reloadedB, connectionB.ID).Error)
	require.Equal(t, 0, reloadedB.VisitCount)
	require.InDelta(t, 0, reloadedB.TotalSpent, 0.001)
	require.Equal(t, 0, reloadedB.LoyaltyPoints)
}
