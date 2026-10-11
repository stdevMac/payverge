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

func setupPendingAlternativePaymentsPerfDB(t testing.TB, gormLogger logger.Interface) (*PaymentHandler, uint, string) {
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

	ownerAddress := "0xPendingAltOwner"
	business := &database.Business{
		BusinessId:     fmt.Sprintf("pending-alt-%d", time.Now().UnixNano()),
		Name:           "Pending Alternative Payment Perf",
		OwnerAddress:   ownerAddress,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, gormDB.Create(business).Error)

	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  "PENDING-ALT-1",
		Name:       "Pending Alternative Table",
		QRCode:     strings.Repeat("qr-payload", 128),
		IsActive:   true,
	}
	require.NoError(t, gormDB.Create(table).Error)

	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench","quantity":1},`, 256), ",") + "]"
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "PENDING-ALT-BILL-001",
		Items:          billItems,
		TotalAmount:    100000,
		PaidAmount:     0,
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gormDB.Create(bill).Error)

	pendingPayments := make([]database.AlternativePayment, 0, 500)
	for i := 0; i < 500; i++ {
		pendingPayments = append(pendingPayments, database.AlternativePayment{
			BillID:          bill.ID,
			ParticipantAddr: "guest",
			ParticipantName: fmt.Sprintf("Guest %03d", i),
			Amount:          25,
			PaymentMethod:   database.PaymentMethodCash,
			Status:          database.AltPaymentStatusPending,
			CreatedAt:       time.Now().Add(-time.Duration(i) * time.Minute),
			UpdatedAt:       time.Now().Add(-time.Duration(i) * time.Minute),
		})
	}
	require.NoError(t, gormDB.Create(&pendingPayments).Error)

	return NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil), bill.ID, ownerAddress
}

func performPendingAlternativePaymentsRequest(handler *PaymentHandler, billID uint, ownerAddress string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_id", Value: fmt.Sprint(billID)}}
	c.Set("token_type", "web3")
	c.Set("address", ownerAddress)
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/bills/%d/pending-alternative-payments", billID), nil)
	handler.GetPendingAlternativePayments(c)
	return w
}

func TestGetPendingAlternativePaymentsUsesProjectedBillAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &paymentDetailsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	handler, billID, ownerAddress := setupPendingAlternativePaymentsPerfDB(t, recorder)

	recorder.statements = nil
	w := performPendingAlternativePaymentsRequest(handler, billID, ownerAddress)
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		PendingPayments []map[string]interface{} `json:"pending_payments"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.PendingPayments, 500)
	assert.Zero(t, recorder.selectStarCount("bills"), "pending alternative payment polling should not hydrate full bill rows")
	assert.Zero(t, recorder.selectStarCount("businesses"), "bill management auth should project business ownership fields")
	assert.Zero(t, recorder.selectCount("tables"), "pending alternative payment polling should not preload bill table")
	assert.Zero(t, recorder.selectCount("payments"), "pending alternative payment polling should not preload crypto payments")
}

func BenchmarkGetPendingAlternativePaymentsSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	handler, billID, ownerAddress := setupPendingAlternativePaymentsPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := performPendingAlternativePaymentsRequest(handler, billID, ownerAddress)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}
