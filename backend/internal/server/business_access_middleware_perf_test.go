package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type billAccessSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *billAccessSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *billAccessSQLRecorder) selectCount(table string) int {
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

func (r *billAccessSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select *") {
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

func setupBillAccessPerfDB(t testing.TB, gormLogger logger.Interface) uint {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Table{}, &database.Bill{}, &database.Payment{}))
	require.NoError(t, createBillAccessPerfItemsTable(gormDB))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("bill-access-perf-%d", time.Now().UnixNano()),
		Name:           "Bill Access Perf",
		OwnerAddress:   "0xBillAccessPerfOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, gormDB.Create(business).Error)

	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  "BILL-ACCESS-PERF-1",
		Name:       "Bill Access Perf Table",
		QRCode:     strings.Repeat("qr-payload", 128),
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(table).Error)

	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench","quantity":1,"price":12.5,"subtotal":12.5},`, 256), ",") + "]"
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "BILL-ACCESS-PERF-001",
		Items:          billItems,
		Subtotal:       1_250_000,
		TaxAmount:      125_000,
		TotalAmount:    1_375_000,
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gormDB.Create(bill).Error)

	for i := 0; i < 250; i++ {
		require.NoError(t, gormDB.Create(&database.Payment{
			BillID:    bill.ID,
			PayerAddr: fmt.Sprintf("0xpayer%03d", i),
			Amount:    1250,
			TxHash:    fmt.Sprintf("bill_access_payment_%03d", i),
			Status:    database.PaymentStatusConfirmed,
		}).Error)
	}

	return bill.ID
}

func createBillAccessPerfItemsTable(gormDB *gorm.DB) error {
	if err := gormDB.Exec("DROP TABLE IF EXISTS bill_items").Error; err != nil {
		return err
	}
	return gormDB.Exec(`CREATE TABLE bill_items (
		id TEXT PRIMARY KEY,
		bill_id INTEGER NOT NULL,
		menu_item_id TEXT DEFAULT '',
		name TEXT NOT NULL,
		price REAL NOT NULL,
		quantity INTEGER NOT NULL,
		options TEXT,
		item_type TEXT DEFAULT 'menu_item',
		bundle_id INTEGER,
		parent_bundle_id INTEGER,
		source_offer_id INTEGER,
		order_id INTEGER,
		subtotal REAL NOT NULL,
		created_at DATETIME
	)`).Error
}

func performBillAccessMiddlewareRequest(billID uint) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "web3")
			c.Set("address", "0xBillAccessPerfOwner")
		},
		RequireBillBusinessAccess(),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/bills/%d", billID), nil)
	r.ServeHTTP(w, req)
	return w
}

func TestRequireBillBusinessAccessUsesProjectedBusinessAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &billAccessSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	billID := setupBillAccessPerfDB(t, recorder)

	recorder.statements = nil
	w := performBillAccessMiddlewareRequest(billID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Zero(t, recorder.selectStarCount("bills"), "bill access middleware should project bill/business ownership fields instead of SELECT *")
	assert.Zero(t, recorder.selectStarCount("businesses"), "bill access middleware should project lock and ownership fields instead of SELECT *")
	assert.Zero(t, recorder.selectCount("tables"), "bill access middleware should not preload table")
	assert.Zero(t, recorder.selectCount("payments"), "bill access middleware should not preload payments")
	assert.Zero(t, recorder.selectCount("bill_items"), "bill access middleware should not load bill items")
}

func BenchmarkRequireBillBusinessAccessSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	billID := setupBillAccessPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := performBillAccessMiddlewareRequest(billID)
		if w.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestGetBill_ReusesPreloadedBusinessForRBAC(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &billAccessSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	billID := setupBillAccessPerfDB(t, recorder)
	require.NoError(t, database.GetDB().AutoMigrate(&database.BillHistoryEvent{}))

	router := gin.New()
	router.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "web3")
			c.Set("address", "0xBillAccessPerfOwner")
		},
		GetBill,
	)

	recorder.statements = nil
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/bills/%d", billID), nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.LessOrEqual(t, recorder.selectCount("businesses"), 1,
		"GetBill should reuse the preloaded business from GetBillByID instead of issuing a second full business reload")
	assert.Zero(t, recorder.selectStarCount("businesses"))
}
