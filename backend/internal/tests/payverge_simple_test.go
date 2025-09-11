package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"payverge/internal/database"
)

type PayvergeSimpleTestSuite struct {
	suite.Suite
	db *gorm.DB
}

func (suite *PayvergeSimpleTestSuite) SetupSuite() {
	// Create a temporary test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(suite.T(), err)
	
	suite.db = db
	database.InitTestDB(db)
	
	// Auto-migrate all models
	err = db.AutoMigrate(
		&database.Business{},
		&database.Menu{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
	)
	assert.NoError(suite.T(), err)
}

func (suite *PayvergeSimpleTestSuite) TearDownSuite() {
	if suite.db != nil {
		sqlDB, _ := suite.db.DB()
		sqlDB.Close()
	}
}

func (suite *PayvergeSimpleTestSuite) SetupTest() {
	// Clean up data before each test
	suite.db.Exec("DELETE FROM payments")
	suite.db.Exec("DELETE FROM bills")
	suite.db.Exec("DELETE FROM tables")
	suite.db.Exec("DELETE FROM menus")
	suite.db.Exec("DELETE FROM businesses")
}

func TestPayvergeSimpleTestSuite(t *testing.T) {
	suite.Run(t, new(PayvergeSimpleTestSuite))
}

// Test Business CRUD operations
func (suite *PayvergeSimpleTestSuite) TestBusinessCRUD() {
	business := &database.Business{
		OwnerAddress:     "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1",
		Name:             "Test Restaurant",
		Logo:             "https://example.com/logo.png",
		Address: database.BusinessAddress{
			Street:     "123 Main St",
			City:       "Test City",
			State:      "TS",
			PostalCode: "12345",
			Country:    "US",
		},
		SettlementAddr:   "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c2",
		TippingAddr:      "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c3",
		TaxRate:          0.08,
		ServiceFeeRate:   0.15,
		TaxInclusive:     false,
		ServiceInclusive: false,
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Test creation
	err := database.CreateBusiness(business)
	assert.NoError(suite.T(), err)
	assert.NotZero(suite.T(), business.ID)

	// Test retrieval by ID
	retrievedBusiness, err := database.GetBusinessByID(business.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), business.Name, retrievedBusiness.Name)
	assert.Equal(suite.T(), business.OwnerAddress, retrievedBusiness.OwnerAddress)

	// Test retrieval by owner address
	businesses, err := database.GetBusinessByOwnerAddress(business.OwnerAddress)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), businesses, 1)

	// Test update
	business.Name = "Updated Restaurant"
	err = database.UpdateBusiness(business)
	assert.NoError(suite.T(), err)

	updatedBusiness, err := database.GetBusinessByID(business.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Updated Restaurant", updatedBusiness.Name)

	// Test soft delete
	err = database.DeleteBusiness(business.ID)
	assert.NoError(suite.T(), err)

	deletedBusiness, err := database.GetBusinessByID(business.ID)
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), deletedBusiness.IsActive)
}

// Test Menu operations
func (suite *PayvergeSimpleTestSuite) TestMenuOperations() {
	// Create business first
	business := &database.Business{
		OwnerAddress:   "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1",
		Name:           "Test Restaurant",
		SettlementAddr: "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c2",
		TippingAddr:    "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c3",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := database.CreateBusiness(business)
	assert.NoError(suite.T(), err)

	// Create menu categories
	categories := []database.MenuCategory{
		{
			Name:        "Appetizers",
			Description: "Start your meal right",
			Items: []database.MenuItem{
				{
					Name:        "Caesar Salad",
					Description: "Fresh romaine lettuce",
					Price:       12.99,
					IsAvailable: true,
				},
			},
		},
	}

	menu := &database.Menu{
		BusinessID: business.ID,
		IsActive:   true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Test menu creation
	err = database.CreateMenu(menu, categories)
	assert.NoError(suite.T(), err)
	assert.NotZero(suite.T(), menu.ID)

	// Test menu retrieval
	retrievedMenu, parsedCategories, err := database.GetMenuByBusinessID(business.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), menu.BusinessID, retrievedMenu.BusinessID)
	assert.Len(suite.T(), parsedCategories, 1)
	assert.Equal(suite.T(), "Appetizers", parsedCategories[0].Name)
}

// Test Table operations
func (suite *PayvergeSimpleTestSuite) TestTableOperations() {
	// Create business first
	business := &database.Business{
		OwnerAddress:   "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1",
		Name:           "Test Restaurant",
		SettlementAddr: "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c2",
		TippingAddr:    "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c3",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := database.CreateBusiness(business)
	assert.NoError(suite.T(), err)

	table := &database.Table{
		BusinessID: business.ID,
		Name:       "Table 1",
		TableCode:  "TBL001",
		IsActive:   true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Test table creation
	err = database.CreateTable(table)
	assert.NoError(suite.T(), err)
	assert.NotZero(suite.T(), table.ID)

	// Test table retrieval by ID
	retrievedTable, err := database.GetTableByID(table.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), table.Name, retrievedTable.Name)
	assert.Equal(suite.T(), table.TableCode, retrievedTable.TableCode)

	// Test table retrieval by business ID
	tables, err := database.GetTablesByBusinessID(business.ID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), tables, 1)

	// Test table retrieval by code
	tableByCode, err := database.GetTableByCode(table.TableCode)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), table.ID, tableByCode.ID)
}

// Test Bill operations
func (suite *PayvergeSimpleTestSuite) TestBillOperations() {
	// Setup business and table
	business := &database.Business{
		OwnerAddress:   "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1",
		Name:           "Test Restaurant",
		SettlementAddr: "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c2",
		TippingAddr:    "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c3",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := database.CreateBusiness(business)
	assert.NoError(suite.T(), err)

	table := &database.Table{
		BusinessID: business.ID,
		Name:       "Table 1",
		TableCode:  "TBL001",
		IsActive:   true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	err = database.CreateTable(table)
	assert.NoError(suite.T(), err)

	// Create bill items
	items := []database.BillItem{
		{
			Name:     "Caesar Salad",
			Price:    12.99,
			Quantity: 2,
		},
	}

	bill := &database.Bill{
		BusinessID:       business.ID,
		TableID:          table.ID,
		BillNumber:       "BILL001",
		Subtotal:         25.98,
		TaxAmount:        2.08,
		ServiceFeeAmount: 3.90,
		TotalAmount:      31.96,
		Status:           database.BillStatusOpen,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Test bill creation
	err = database.CreateBill(bill, items)
	assert.NoError(suite.T(), err)
	assert.NotZero(suite.T(), bill.ID)

	// Test bill retrieval
	retrievedBill, parsedItems, err := database.GetBillByID(bill.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), bill.TableID, retrievedBill.TableID)
	assert.Equal(suite.T(), bill.TotalAmount, retrievedBill.TotalAmount)
	assert.Len(suite.T(), parsedItems, 1)
	assert.Equal(suite.T(), "Caesar Salad", parsedItems[0].Name)
}

// Test Payment operations
func (suite *PayvergeSimpleTestSuite) TestPaymentOperations() {
	// Setup business, table, and bill
	business := &database.Business{
		OwnerAddress:   "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1",
		Name:           "Test Restaurant",
		SettlementAddr: "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c2",
		TippingAddr:    "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c3",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := database.CreateBusiness(business)
	assert.NoError(suite.T(), err)

	table := &database.Table{
		BusinessID: business.ID,
		Name:       "Table 1",
		TableCode:  "TBL001",
		IsActive:   true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	err = database.CreateTable(table)
	assert.NoError(suite.T(), err)

	bill := &database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  "BILL001",
		Subtotal:    50.00,
		TotalAmount: 60.00,
		Status:      database.BillStatusOpen,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	emptyItems := []database.BillItem{}
	err = database.CreateBill(bill, emptyItems)
	assert.NoError(suite.T(), err)

	payment := &database.Payment{
		BillID:    bill.ID,
		PayerAddr: "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c4",
		Amount:    60.00,
		TipAmount: 10.00,
		TxHash:    "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		Status:    database.PaymentStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Test payment creation
	err = database.CreatePayment(payment)
	assert.NoError(suite.T(), err)
	assert.NotZero(suite.T(), payment.ID)

	// Test payment retrieval by transaction hash
	retrievedPayment, err := database.GetPaymentByTxHash(payment.TxHash)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), payment.BillID, retrievedPayment.BillID)
	assert.Equal(suite.T(), payment.Amount, retrievedPayment.Amount)

	// Test payment status update
	err = database.UpdatePaymentStatus(payment.ID, database.PaymentStatusConfirmed)
	assert.NoError(suite.T(), err)

	// Verify status was updated
	updatedPayment, err := database.GetPaymentByTxHash(payment.TxHash)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), database.PaymentStatusConfirmed, updatedPayment.Status)
}
