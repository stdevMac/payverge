package database

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DbConfig struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
	SSLMode  string `json:"sslmode"`
}

func NewConfig(host, port, user, password, dbname, sslmode string) *DbConfig {
	return &DbConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		DBName:   dbname,
		SSLMode:  sslmode,
	}
}

var (
	db *gorm.DB
)

// DB wraps the GORM database instance
type DB struct {
	conn                      *gorm.DB
	BusinessService           *BusinessService
	TableService              *TableService
	BillService               *BillService
	PaymentService            *PaymentService
	AlternativePaymentService *AlternativePaymentService
	MenuService               *MenuService
	StaffService              *StaffService
	StaffInvitationService    *StaffInvitationService
	StaffLoginCodeService     *StaffLoginCodeService
	CurrencyService           *CurrencyService
	LanguageService           *LanguageService
	TranslationService        *TranslationService
}

// NewDB creates a new DB instance
func NewDB() *DB {
	return &DB{conn: db}
}

// NewDBWithConn wraps an explicit connection (the maintenance CLI opens its
// own) instead of the package-global one.
func NewDBWithConn(conn *gorm.DB) *DB {
	return &DB{conn: conn}
}

// GetDB returns the underlying GORM database connection
func (d *DB) GetDB() *gorm.DB {
	return d.conn
}

// InitTestDB initializes the database for testing
func InitTestDB(testDB *gorm.DB) {
	db = testDB
	for _, fn := range onDBChange {
		fn()
	}
}

func newGORMLogger(writer logger.Writer, level logger.LogLevel, slowThreshold time.Duration) logger.Interface {
	return logger.New(writer, logger.Config{
		SlowThreshold:             slowThreshold,
		LogLevel:                  level,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	})
}

func InitDB(config *DbConfig) {
	// Build PostgreSQL connection string
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=UTC",
		config.Host, config.User, config.Password, config.DBName, config.Port, config.SSLMode)

	// Logger selection: default is unchanged Info-mode. Opt-in via DB_SLOW_QUERY_LOG=true
	// switches to a slow-query logger that warns on queries above 100ms, useful for
	// perf benchmarks and incident investigation.
	slowThreshold := 200 * time.Millisecond
	if os.Getenv("DB_SLOW_QUERY_LOG") == "true" {
		slowThreshold = 100 * time.Millisecond
	}
	gormLogger := newGORMLogger(log.New(os.Stdout, "[gorm] ", log.LstdFlags), logger.Warn, slowThreshold)

	// Open PostgreSQL database. We do NOT enable gorm.Config.PrepareStmt — pgx
	// already caches prepared statements at the driver layer, and stacking
	// GORM's cache on top measurably regressed BenchmarkOrderCreate (~+75%
	// ns/op in local measurements). Keep the trust at the driver.
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	db = database

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("Failed to get underlying sql.DB:", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	sqlDB.SetConnMaxIdleTime(3 * time.Minute)

	// Schema ownership is the genesis baseline plus numbered migrations, both
	// applied in cmd/app/main.go after InitDB. GORM AutoMigrate never runs here.
	log.Println("Database connected successfully")
}

// GetDB returns the GORM database instance
func GetDB() *gorm.DB {
	return db
}

// GetDBWrapper returns a DB instance with all services initialized
func GetDBWrapper() *DB {
	return &DB{
		conn:                      db,
		BusinessService:           NewBusinessService(),
		TableService:              NewTableService(),
		BillService:               NewBillService(),
		PaymentService:            NewPaymentService(),
		AlternativePaymentService: NewAlternativePaymentService(),
		MenuService:               NewMenuService(),
		StaffService:              NewStaffService(),
		StaffInvitationService:    NewStaffInvitationService(),
		StaffLoginCodeService:     NewStaffLoginCodeService(),
		CurrencyService:           NewCurrencyService(db),
		LanguageService:           NewLanguageService(db),
		TranslationService:        NewTranslationService(db),
	}
}

// GetGorm returns the underlying GORM database instance
func (d *DB) GetGorm() *gorm.DB {
	return d.conn
}

// onDBChange holds callbacks fired whenever the package-level db is swapped
// via SetTestDB. Used by higher-level packages (e.g. services) to invalidate
// in-process caches when the underlying database changes — avoids stale
// reads when tests reset the DB between cases.
var onDBChange []func()

// RegisterOnDBChange records a callback to be invoked on every SetTestDB.
// Safe to call from package init.
func RegisterOnDBChange(fn func()) {
	onDBChange = append(onDBChange, fn)
}

// SetTestDB sets the package-level db for testing from external packages.
func SetTestDB(gormDB *gorm.DB) {
	db = gormDB
	for _, fn := range onDBChange {
		fn()
	}
}

// Database methods for the DB wrapper

// GetBill retrieves a bill by ID
func (d *DB) GetBill(id uint) (*Bill, error) {
	bill, _, err := GetBillByID(id)
	return bill, err
}

// UpdateBill updates a bill while preserving existing items
func (d *DB) UpdateBill(bill *Bill) error {
	// Get existing bill to preserve items
	var existingBill Bill
	if err := db.First(&existingBill, bill.ID).Error; err != nil {
		return fmt.Errorf("failed to get existing bill: %w", err)
	}

	// Parse existing items to preserve them
	var existingItems []BillItem
	if existingBill.Items != "" {
		if err := json.Unmarshal([]byte(existingBill.Items), &existingItems); err != nil {
			// If parsing fails, use empty array but log the error
			fmt.Printf("Warning: failed to parse existing items for bill %d: %v\n", bill.ID, err)
			existingItems = []BillItem{}
		}
	}

	return UpdateBill(bill, existingItems)
}

// GetTable retrieves a table by ID
func (d *DB) GetTable(id uint) (*Table, error) {
	return GetTableByID(id)
}

// GetBillItems retrieves a bill's items, preferring the relational bill_items
// table (the source of truth) and falling back to the bills.items JSON snapshot
// when the table is empty or unavailable — matching every other bill-item read
// path. Previously this read the JSON snapshot directly, making it the last
// reader that could diverge from the relational rows.
func (db *DB) GetBillItems(billID uint) ([]BillItem, error) {
	return billItemsForBillSnapshotLazyByBillID(billID)
}

// GetActiveBillsByBusinessID retrieves full Bill rows that are still in progress
// for a business (status IN ('open','partial')). It does NOT apply a column
// projection — the caller receives all columns. Use GetActiveBillCountByBusinessID
// when only the count is needed, and GetActiveBillSummariesByBusinessID when only
// scalar summary fields are needed.
func (db *DB) GetActiveBillsByBusinessID(businessID uint) ([]Bill, error) {
	var bills []Bill
	err := db.conn.Where("business_id = ? AND status IN ?", businessID, activeBillStatusStrings()).Find(&bills).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get active bills: %w", err)
	}
	return bills, nil
}

// GetActiveBillCountByBusinessID returns the count of in-progress bills for a
// business (status IN ('open','partial')) without hydrating any bill rows. Use
// this instead of len(GetActiveBillsByBusinessID) when only the count is needed
// (e.g. Director context metrics).
func (db *DB) GetActiveBillCountByBusinessID(businessID uint) (int64, error) {
	var n int64
	err := db.conn.Model(&Bill{}).
		Where("business_id = ? AND status IN ?", businessID, activeBillStatusStrings()).
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count active bills: %w", err)
	}
	return n, nil
}

// CountOccupiedTablesByBusinessID returns how many distinct tables currently
// have an active (open/partial) bill. Counter/delivery bills with table_id = 0
// are excluded — they must not inflate the "Open tables" live metric.
func (db *DB) CountOccupiedTablesByBusinessID(businessID uint) (int64, error) {
	var n int64
	err := db.conn.Model(&Bill{}).
		Where("business_id = ? AND status IN ? AND table_id > 0", businessID, activeBillStatusStrings()).
		Distinct("table_id").
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count occupied tables: %w", err)
	}
	return n, nil
}

type ActiveBillSummary struct {
	ID          uint       `json:"id"`
	BusinessID  uint       `json:"business_id"`
	TableID     uint       `json:"table_id"`
	CounterID   *uint      `json:"counter_id"`
	BillNumber  string     `json:"bill_number"`
	TotalAmount int64      `json:"total_amount"`
	PaidAmount  int64      `json:"paid_amount"`
	TipAmount   int64      `json:"tip_amount"`
	Status      BillStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// DefaultActiveBillSummaryLimit bounds the live-bills / dashboard summary surface
// so a single busy business can't return an unbounded list every poll cycle
// (the page-level bills sibling is already capped). A restaurant with more than
// this many simultaneously-open bills is well past what the live board renders.
const DefaultActiveBillSummaryLimit = 200

// GetActiveBillSummariesByBusinessID retrieves only the active bill fields
// needed by dashboard/live summary surfaces. It is bounded to
// DefaultActiveBillSummaryLimit rows (newest first) so the poll payload stays
// small; callers that need the capped signal should use
// GetActiveBillSummariesByBusinessIDBounded.
func (db *DB) GetActiveBillSummariesByBusinessID(businessID uint) ([]ActiveBillSummary, error) {
	bills, _, err := db.GetActiveBillSummariesByBusinessIDBounded(businessID, DefaultActiveBillSummaryLimit)
	return bills, err
}

// GetActiveBillSummariesByBusinessIDBounded retrieves at most `limit` active bill
// summary rows (newest first) and reports whether the result was capped (there
// were more matching rows than returned). A limit <= 0 falls back to the default
// cap. A limit above DefaultActiveBillSummaryLimit is clamped to that cap so a
// caller cannot widen the query. The `capped` flag lets the live-bills endpoint
// surface an honest "showing N of more" banner instead of silently truncating.
func (db *DB) GetActiveBillSummariesByBusinessIDBounded(businessID uint, limit int) ([]ActiveBillSummary, bool, error) {
	if limit <= 0 || limit > DefaultActiveBillSummaryLimit {
		limit = DefaultActiveBillSummaryLimit
	}
	var bills []ActiveBillSummary
	// Over-fetch by one so a full page tells us capping happened without a
	// separate COUNT.
	err := db.conn.Model(&Bill{}).
		Select("id", "business_id", "table_id", "counter_id", "bill_number", "total_amount", "paid_amount", "tip_amount", "status", "created_at", "updated_at").
		Where("business_id = ? AND status IN ?", businessID, activeBillStatusStrings()).
		Order("created_at DESC, id DESC").
		Limit(limit + 1).
		Find(&bills).Error
	if err != nil {
		return nil, false, fmt.Errorf("failed to get active bill summaries: %w", err)
	}
	capped := false
	if len(bills) > limit {
		capped = true
		bills = bills[:limit]
	}
	return bills, capped, nil
}

// GetBusinessByID retrieves a business by its ID
func (db *DB) GetBusinessByID(id uint) (*Business, error) {
	var business Business
	if err := db.conn.First(&business, id).Error; err != nil {
		return nil, err
	}
	return &business, nil
}

// GetBusinessDefaults returns the default currency and language for a business
func (db *DB) GetBusinessDefaults(businessID uint) (string, string, error) {
	var business Business
	if err := db.conn.Select("default_currency, default_language").Where("id = ?", businessID).First(&business).Error; err != nil {
		return "", "", err
	}
	return business.DefaultCurrency, business.DefaultLanguage, nil
}

// GetTableByID retrieves a table by its ID
func (db *DB) GetTableByID(id uint) (*Table, error) {
	var table Table
	if err := db.conn.First(&table, id).Error; err != nil {
		return nil, err
	}
	return &table, nil
}
