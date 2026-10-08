package handlers

import (
	"bytes"
	"encoding/json"
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

func (r *paymentDetailsSQLRecorder) selectWhereMentionsColumn(table string, column string) int {
	count := 0
	needle := strings.ToLower(column)
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if !(strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table)) {
			continue
		}
		whereIndex := strings.Index(normalized, " where ")
		if whereIndex < 0 {
			continue
		}
		whereClause := normalized[whereIndex:]
		if strings.Contains(whereClause, "`"+needle+"`") ||
			strings.Contains(whereClause, `"`+needle+`"`) ||
			strings.Contains(whereClause, "."+needle) ||
			strings.Contains(whereClause, " "+needle+" ") {
			count++
		}
	}
	return count
}

func setupCryptoPaymentPerfDB(t testing.TB, gormLogger logger.Interface) (*PaymentHandler, string, uint) {
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
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.BusinessMilestoneEvent{},
		&database.BusinessRevenueAggregate{},
		&database.CryptoPaymentQuote{},
	))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("crypto-payment-%d", time.Now().UnixNano()),
		Name:           "Crypto Payment Perf",
		OwnerAddress:   "0xCryptoPaymentOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, gormDB.Create(business).Error)

	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  "CRYPTO-PAY-1",
		Name:       "Crypto Payment Table",
		QRCode:     strings.Repeat("qr-payload", 128),
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(table).Error)

	plugin := &database.Plugin{
		Name:        "usdc_payment",
		DisplayName: "USDC Payment",
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, gormDB.Create(plugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{"enabled": true}))

	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench","quantity":1},`, 256), ",") + "]"
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "CRYPTO-PAY-BILL-001",
		Items:          billItems,
		Subtotal:       1_000_000_000_000,
		TotalAmount:    1_000_000_000_000,
		PaidAmount:     0,
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gormDB.Create(bill).Error)

	return NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil), bill.PublicToken, bill.ID
}

func performCryptoPaymentPerfRequest(handler *PaymentHandler, billNumber string, txHash string, quoteToken string) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(map[string]interface{}{
		"transaction_hash": txHash,
		"amount_paid":      1.0,
		"tip_amount":       0.0,
		"payment_method":   "USDC",
		"quote_token":      quoteToken,
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: billNumber}}
	c.Request = httptest.NewRequest(http.MethodPost, "/guest/bill/"+billNumber+"/crypto-payment", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.ProcessCryptoPayment(c)
	return w
}

func TestProcessCryptoPaymentUsesProjectedBillLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	recorder := &paymentDetailsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	handler, billNumber, billID := setupCryptoPaymentPerfDB(t, recorder)
	token := signTestQuote(t, "quote-test-secret", billID, 100, centsToMicrounits(100))

	recorder.statements = nil
	w := performCryptoPaymentPerfRequest(handler, billNumber, testEVMTxHash("tx-crypto-perf-test"), token)
	require.Equal(t, http.StatusOK, w.Code)
	assert.LessOrEqual(t, recorder.selectStarCount("bills"), 1, "crypto payment should only full-load the transactional bill lock")
	assert.Zero(t, recorder.selectStarCount("businesses"), "crypto payment subscription check should project subscription fields")
	assert.Zero(t, recorder.selectCount("tables"), "crypto payment should not preload bill table")
	assert.Zero(t, recorder.selectWhereMentionsColumn("payments", "bill_id"), "crypto payment should not preload all bill payments")
}

func BenchmarkProcessCryptoPaymentSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	b.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	handler, billNumber, billID := setupCryptoPaymentPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Every settlement consumes its quote, so each iteration needs a fresh
		// persisted quote. Issuing it is not part of the measured path.
		b.StopTimer()
		token := signTestQuote(b, "quote-test-secret", billID, 100, centsToMicrounits(100))
		b.StartTimer()
		w := performCryptoPaymentPerfRequest(handler, billNumber, testEVMTxHash(fmt.Sprintf("tx-crypto-perf-%d", i)), token)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}
