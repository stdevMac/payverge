package database

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DbConfig struct {
	DatabasePath string `json:"database_path"`
}

func NewConfig(databasePath string) *DbConfig {
	return &DbConfig{
		DatabasePath: databasePath,
	}
}

var (
	db *gorm.DB
)

// DB wraps the GORM database instance
type DB struct {
	conn *gorm.DB
	BusinessService           *BusinessService
	TableService              *TableService
	BillService               *BillService
	PaymentService            *PaymentService
	AlternativePaymentService *AlternativePaymentService
	MenuService               *MenuService
	CodeService               *CodeService
	StaffService              *StaffService
	StaffInvitationService    *StaffInvitationService
	StaffLoginCodeService     *StaffLoginCodeService
}

// NewDB creates a new DB instance
func NewDB() *DB {
	return &DB{conn: db}
}

// InitTestDB initializes the database for testing
func InitTestDB(testDB *gorm.DB) {
	db = testDB
}

func InitDB(config *DbConfig) {
	// Ensure the directory exists
	dir := filepath.Dir(config.DatabasePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatal("Failed to create database directory:", err)
	}

	// Open SQLite database
	database, err := gorm.Open(sqlite.Open(config.DatabasePath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	db = database

	// Auto-migrate all models
	if err := autoMigrate(); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	log.Println("Database connected and migrated successfully")
}

// GetDB returns the GORM database instance
func GetDB() *gorm.DB {
	return db
}

// GetDBWrapper returns a DB wrapper instance
func GetDBWrapper() *DB {
	return &DB{
		conn: db,
		BusinessService:           NewBusinessService(),
		TableService:              NewTableService(),
		BillService:               NewBillService(),
		PaymentService:            NewPaymentService(),
		AlternativePaymentService: NewAlternativePaymentService(),
		MenuService:               NewMenuService(),
		CodeService:               NewCodeService(),
		StaffService:              NewStaffService(),
		StaffInvitationService:    NewStaffInvitationService(),
		StaffLoginCodeService:     NewStaffLoginCodeService(),
	}
}

// Database methods for the DB wrapper

// GetBill retrieves a bill by ID
func (d *DB) GetBill(id uint) (*Bill, error) {
	bill, _, err := GetBillByID(id)
	return bill, err
}

// UpdateBill updates a bill
func (d *DB) UpdateBill(bill *Bill) error {
	return UpdateBill(bill, []BillItem{})
}

// GetTable retrieves a table by ID
func (d *DB) GetTable(id uint) (*Table, error) {
	return GetTableByID(id)
}

// GetBillsByStatus retrieves bills by status
func (d *DB) GetBillsByStatus(status BillStatus) ([]*Bill, error) {
	var bills []Bill
	if err := d.conn.Where("status = ?", status).Find(&bills).Error; err != nil {
		return nil, err
	}
	result := make([]*Bill, len(bills))
	for i := range bills {
		result[i] = &bills[i]
	}
	return result, nil
}

// GetBillItems retrieves bill items by bill ID
func (db *DB) GetBillItems(billID uint) ([]BillItem, error) {
	var items []BillItem
	err := db.conn.Where("bill_id = ?", billID).Find(&items).Error
	return items, err
}

// GetBillsByDateRange retrieves bills for a business within a date range
func (db *DB) GetBillsByDateRange(businessID uint, startDate, endDate time.Time) ([]Bill, error) {
	var bills []Bill
	err := db.conn.Where("business_id = ? AND created_at >= ? AND created_at < ?", 
		businessID, startDate, endDate).Find(&bills).Error
	return bills, err
}

// GetBillsByBusinessAndStatus gets bills by business ID and status
func (db *DB) GetBillsByBusinessAndStatus(businessID uint, status BillStatus) ([]Bill, error) {
	var bills []Bill
	err := db.conn.Where("business_id = ? AND status = ?", businessID, status).Find(&bills).Error
	return bills, err
}

// GetPaymentsByDateRange retrieves payments for a business within a date range
func (db *DB) GetPaymentsByDateRange(businessID uint, startDate, endDate time.Time) ([]Payment, error) {
	var payments []Payment
	err := db.conn.Joins("JOIN bills ON payments.bill_id = bills.id").
		Where("bills.business_id = ? AND payments.created_at >= ? AND payments.created_at < ?", 
			businessID, startDate, endDate).
		Find(&payments).Error
	return payments, err
}

// GetBusinessByID retrieves a business by its ID
func (db *DB) GetBusinessByID(id uint) (*Business, error) {
	var business Business
	if err := db.conn.First(&business, id).Error; err != nil {
		return nil, err
	}
	return &business, nil
}

// GetTableByID retrieves a table by its ID
func (db *DB) GetTableByID(id uint) (*Table, error) {
	var table Table
	if err := db.conn.First(&table, id).Error; err != nil {
		return nil, err
	}
	return &table, nil
}

// autoMigrate creates all necessary tables
func autoMigrate() error {
	return db.AutoMigrate(
		&User{},
		&Code{},
		&ErrorLog{},
		&FaucetTransaction{},
		&Subscriber{},
		&MultisigTx{},
		// Payverge models
		&Business{},
		&Menu{},
		&Table{},
		&Bill{},
		&Payment{},
		&AlternativePayment{},
		// Staff management models
		&Staff{},
		&StaffInvitation{},
		&StaffLoginCode{},
		// Kitchen management models
		&KitchenOrder{},
		&KitchenOrderItem{},
	)
}
