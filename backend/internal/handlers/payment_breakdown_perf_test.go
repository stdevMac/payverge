package handlers

import (
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

func setupPaymentBreakdownPerfDB(t testing.TB, gormLogger logger.Interface) (*PaymentHandler, string) {
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
		&database.AlternativePayment{},
	))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("payment-breakdown-%d", time.Now().UnixNano()),
		Name:           "Payment Breakdown Perf",
		OwnerAddress:   "0xPaymentBreakdownOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, gormDB.Create(business).Error)

	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  "PAY-BREAKDOWN-1",
		Name:       "Payment Breakdown Table",
		QRCode:     strings.Repeat("qr-payload", 128),
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(table).Error)

	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench","quantity":1},`, 256), ",") + "]"
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "PAYMENT-BREAKDOWN-BILL-001",
		Items:          billItems,
		TotalAmount:    100000,
		PaidAmount:     70000,
		Status:         database.BillStatusPartial,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gormDB.Create(bill).Error)

	confirmedPayments := make([]database.AlternativePayment, 0, 500)
	for i := 0; i < 500; i++ {
		confirmedPayments = append(confirmedPayments, database.AlternativePayment{
			BillID:          bill.ID,
			ParticipantAddr: "guest",
			ParticipantName: fmt.Sprintf("Guest %03d", i),
			Amount:          25,
			PaymentMethod:   database.PaymentMethodCash,
			Status:          database.AltPaymentStatusConfirmed,
			ConfirmedBy:     "owner",
			CreatedAt:       time.Now().Add(-time.Duration(i) * time.Minute),
			UpdatedAt:       time.Now().Add(-time.Duration(i) * time.Minute),
		})
	}
	require.NoError(t, gormDB.Create(&confirmedPayments).Error)

	return NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil), bill.PublicToken
}

func performPaymentBreakdownRequest(handler *PaymentHandler, billNumber string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: billNumber}}
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/"+billNumber+"/payment-breakdown", nil)
	handler.GetBillPaymentBreakdown(c)
	return w
}

func TestGetBillPaymentBreakdownUsesProjectedBillAndAggregate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &paymentDetailsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	handler, billNumber := setupPaymentBreakdownPerfDB(t, recorder)

	recorder.statements = nil
	w := performPaymentBreakdownRequest(handler, billNumber)
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Breakdown struct {
			TotalAmount     float64 `json:"total_amount"`
			CryptoPaid      float64 `json:"crypto_paid"`
			AlternativePaid float64 `json:"alternative_paid"`
			Remaining       float64 `json:"remaining"`
			IsComplete      bool    `json:"is_complete"`
		} `json:"breakdown"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 1000.0, body.Breakdown.TotalAmount)
	assert.Equal(t, 575.0, body.Breakdown.CryptoPaid)
	assert.Equal(t, 125.0, body.Breakdown.AlternativePaid)
	assert.Equal(t, 300.0, body.Breakdown.Remaining)
	assert.False(t, body.Breakdown.IsComplete)
	assert.Zero(t, recorder.selectStarCount("bills"), "payment breakdown should project bill amount/status fields instead of SELECT *")
	assert.Zero(t, recorder.selectCount("businesses"), "payment breakdown should not preload bill business")
	assert.Zero(t, recorder.selectCount("tables"), "payment breakdown should not preload bill table")
	assert.Zero(t, recorder.selectCount("payments"), "payment breakdown should not preload crypto payments")
	assert.Zero(t, recorder.selectStarCount("alternative_payments"), "payment breakdown should aggregate alternative payments instead of hydrating every row")
}

func BenchmarkGetBillPaymentBreakdownSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	handler, billNumber := setupPaymentBreakdownPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := performPaymentBreakdownRequest(handler, billNumber)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}
