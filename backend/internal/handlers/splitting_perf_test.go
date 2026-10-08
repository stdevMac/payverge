package handlers

import (
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

func setupSplittingPerfDB(t testing.TB, gormLogger logger.Interface) (*SplittingHandler, string) {
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
	require.NoError(t, createSQLitePaymentRegressionBillItemsTable(gormDB))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("split-perf-%d", time.Now().UnixNano()),
		Name:           "Split Perf",
		OwnerAddress:   "0xSplitPerfOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, gormDB.Create(business).Error)

	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  "SPLIT-PERF-1",
		Name:       "Split Perf Table",
		QRCode:     strings.Repeat("qr-payload", 128),
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(table).Error)

	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench","quantity":1,"price":12.5,"subtotal":12.5},`, 256), ",") + "]"
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "SPLIT-PERF-BILL-001",
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
			TxHash:    fmt.Sprintf("split_perf_payment_%03d", i),
			Status:    database.PaymentStatusConfirmed,
		}).Error)
	}

	return NewSplittingHandler(database.GetDBWrapper()), bill.PublicToken
}

func performSplitOptionsRequest(handler *SplittingHandler, billNumber string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: billNumber}}
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/"+billNumber+"/split/options", nil)
	handler.GetBillSplitOptions(c)
	return w
}

func TestGetBillSplitOptionsUsesProjectedBillLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &paymentDetailsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	handler, billNumber := setupSplittingPerfDB(t, recorder)

	recorder.statements = nil
	w := performSplitOptionsRequest(handler, billNumber)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Zero(t, recorder.selectStarCount("bills"), "split options should project bill fields instead of SELECT *")
	assert.Zero(t, recorder.selectCount("businesses"), "split options should not preload business")
	assert.Zero(t, recorder.selectCount("tables"), "split options should not preload table")
	assert.Zero(t, recorder.selectCount("payments"), "split options should not preload payments")
}

func BenchmarkGetBillSplitOptionsSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	handler, billNumber := setupSplittingPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := performSplitOptionsRequest(handler, billNumber)
		if w.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}
