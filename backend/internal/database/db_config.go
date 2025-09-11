package database

import (
	"log"
	"os"
	"path/filepath"

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
	return &DB{conn: db}
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
func (d *DB) GetBillItems(billID uint) ([]BillItem, error) {
	_, items, err := GetBillByID(billID)
	return items, err
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
	)
}
