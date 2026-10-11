package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/guestsession"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockGuestPaymentVerifier struct {
	err error
	// lastExpectedMicrounits records the settlement floor the handler passed to
	// the verifier, so tests can assert the locked USD amount is used (not the
	// local-currency cents).
	lastExpectedMicrounits int64
	atLeastCalled          bool
}

func (m *mockGuestPaymentVerifier) VerifyUSDCTransfer(_ context.Context, _ string, _ string, expectedAmountMicrounits int64) error {
	m.lastExpectedMicrounits = expectedAmountMicrounits
	return m.err
}

func (m *mockGuestPaymentVerifier) VerifyUSDCTransferAtLeast(_ context.Context, _ string, _ string, expectedAmountMicrounits int64) error {
	m.atLeastCalled = true
	m.lastExpectedMicrounits = expectedAmountMicrounits
	return m.err
}

func (m *mockGuestPaymentVerifier) ChainID() int64 { return baseMainnetChainID }

func (m *mockGuestPaymentVerifier) TokenSymbolForChain() string { return "USDC" }

func setupPaymentRegressionDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.User{},
		&database.Staff{},
		&database.StaffPermissionDeny{},
		&database.Customer{},
		&database.CustomerBusiness{},
		&database.CustomerVisit{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.BillSplitShare{},
		&database.CashRegisterSession{},
		&database.CashRegisterMovement{},
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.BusinessFiscalSettings{},
		&database.FiscalReceipt{},
		&database.FiscalJob{},
		&database.BillHistoryEvent{},
		&database.BusinessMilestoneEvent{},
		&database.BusinessRevenueAggregate{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.BusinessAlertSettings{},
		&session.UserSession{},
		&database.CryptoPaymentQuote{},
	))
	require.NoError(t, createSQLitePaymentRegressionBillItemsTable(gormDB))
	require.NoError(t, createSQLitePaymentRegressionDeliveryOrdersTable(gormDB))
	server.InitializeRBAC(database.GetDBWrapper())

	return gormDB
}

// createSQLitePaymentRegressionDeliveryOrdersTable creates a minimal
// delivery_orders table carrying only the columns the guest payment gates
// project (status, payment_expires_at, indexed bill_id). Payment handlers must
// never hydrate the full delivery aggregate, so the narrow table doubles as an
// access-shape tripwire.
func createSQLitePaymentRegressionDeliveryOrdersTable(db *gorm.DB) error {
	if err := db.Exec(`
CREATE TABLE delivery_orders (
	id integer PRIMARY KEY AUTOINCREMENT,
	business_id integer NOT NULL,
	bill_id integer NOT NULL,
	delivery_number text NOT NULL,
	status text NOT NULL DEFAULT 'pending',
	payment_expires_at datetime,
	created_at datetime,
	updated_at datetime
)`).Error; err != nil {
		return err
	}
	return db.Exec("CREATE INDEX idx_delivery_orders_bill_id ON delivery_orders(bill_id)").Error
}

func createSQLitePaymentRegressionBillItemsTable(db *gorm.DB) error {
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

func createPaymentRegressionBusiness(t *testing.T, suffix string, userID *uint, email string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:      fmt.Sprintf("payment-biz-%s", suffix),
		Name:            fmt.Sprintf("Payment Biz %s", suffix),
		OwnerAddress:    fmt.Sprintf("0x%s", suffix),
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		Email:           email,
		UserID:          userID,
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		SourceLanguage:  "en",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	enablePaymentRegressionPlugin(t, business.ID, "usdc_payment", map[string]interface{}{"enabled": true})
	enablePaymentRegressionPlugin(t, business.ID, "cross_chain_payment", map[string]interface{}{"enabled": true})
	return business
}

func enablePaymentRegressionPlugin(t *testing.T, businessID uint, pluginName string, config map[string]interface{}) {
	t.Helper()

	plugin := database.Plugin{
		Name:        pluginName,
		DisplayName: pluginName,
		Category:    database.PluginCategoryPayment,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().
		Where("name = ?", pluginName).
		FirstOrCreate(&plugin, database.Plugin{Name: pluginName}).Error)
	require.NoError(t, database.GetDB().Model(&plugin).Updates(map[string]interface{}{
		"display_name": plugin.DisplayName,
		"category":     database.PluginCategoryPayment,
		"is_active":    true,
	}).Error)
	require.NoError(t, database.EnableBusinessPlugin(businessID, plugin.ID, config))
}

func seedPaymentRegressionFiscalSettings(t *testing.T, businessID uint) {
	t.Helper()
	enableFiscalRuntimeControl(t, database.GetDB())

	require.NoError(t, database.GetDB().Create(&database.BusinessFiscalSettings{
		BusinessID:   businessID,
		Country:      "AR",
		Provider:     "arca",
		Mode:         database.FiscalModeAutomaticNonBlocking,
		Environment:  "sandbox",
		SetupStatus:  "active",
		PointOfSale:  testIntPtr(1),
		TaxID:        "30711222333",
		TaxCondition: "responsable_inscripto",
	}).Error)
}

func enableFiscalRuntimeControl(t testing.TB, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&runtimecontrol.Control{}))
	require.NoError(t, db.Save(&runtimecontrol.Control{
		Key:       runtimecontrol.ControlFiscal,
		Enabled:   true,
		Owner:     "fiscal-test-owner",
		Reason:    "exercise the explicitly enabled fiscal path",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		UpdatedBy: "fiscal-test",
		UpdatedAt: time.Now().UTC(),
	}).Error)
}

func createPaymentRegressionBill(t *testing.T, businessID uint, total float64) *database.Bill {
	t.Helper()
	totalCents := int64(math.Round(total * 100))

	bill := &database.Bill{
		BusinessID:     businessID,
		BillNumber:     fmt.Sprintf("B-%d-%d", businessID, time.Now().UnixNano()),
		Subtotal:       totalCents,
		TotalAmount:    totalCents,
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return bill
}

func openPaymentRegressionCashSession(t *testing.T, businessID uint) {
	t.Helper()
	session := &database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusOpen,
		OpeningFloatCents: 0,
		OpenedByLabel:     "test",
		OpenedAt:          time.Now().UTC(),
	}
	require.NoError(t, database.GetDB().Create(session).Error)
}

func createPaymentRegressionCustomer(t *testing.T, email string) *database.Customer {
	t.Helper()
	customer := &database.Customer{
		Email:        email,
		PasswordHash: "hash",
		Name:         "Payment Customer",
		IsActive:     true,
	}
	require.NoError(t, database.GetDB().Create(customer).Error)
	return customer
}

func connectPaymentRegressionCustomer(t *testing.T, customerID, businessID uint) *database.CustomerBusiness {
	t.Helper()
	connection := &database.CustomerBusiness{
		CustomerID:     customerID,
		BusinessID:     businessID,
		FirstVisitAt:   time.Now(),
		OptInEmail:     true,
		OptInMarketing: true,
		IsActive:       true,
	}
	require.NoError(t, database.GetDB().Create(connection).Error)
	return connection
}

func createPaymentRegressionCustomerToken(t *testing.T, customer *database.Customer) string {
	t.Helper()
	previousSecret := structs.SecretKey
	structs.SecretKey = []byte("test-secret-key-for-payment-crm-attribution")
	t.Cleanup(func() { structs.SecretKey = previousSecret })

	previousStore := session.GlobalStore
	store := session.NewStore(database.GetDB())
	session.GlobalStore = store
	t.Cleanup(func() { session.GlobalStore = previousStore })

	customerID := customer.ID
	sess, err := store.Create(session.CreateInput{
		UserID:    &customerID,
		Provider:  "customer",
		IPAddress: "127.0.0.1",
		UserAgent: "test",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	})
	require.NoError(t, err)

	token, err := server.GenerateCustomerToken(customer.ID, customer.Email, sess.ID)
	require.NoError(t, err)
	require.NoError(t, store.UpdateTokenHash(sess.ID, session.HashToken(token)))
	return token
}

func assertSinglePaymentCRMVisitForCustomer(t *testing.T, billID, customerID uint) {
	t.Helper()
	var count int64
	require.NoError(t, database.GetDB().Model(&database.CustomerVisit{}).
		Joins("JOIN customer_businesses cb ON cb.id = customer_visits.customer_business_id").
		Where("customer_visits.bill_id = ? AND cb.customer_id = ?", billID, customerID).
		Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func assertNoPaymentCRMVisitForCustomer(t *testing.T, billID, customerID uint) {
	t.Helper()
	var count int64
	require.NoError(t, database.GetDB().Model(&database.CustomerVisit{}).
		Joins("JOIN customer_businesses cb ON cb.id = customer_visits.customer_business_id").
		Where("customer_visits.bill_id = ? AND cb.customer_id = ?", billID, customerID).
		Count(&count).Error)
	require.Zero(t, count)
}

func assertPaymentLedgerParity(t *testing.T, billID uint) {
	t.Helper()

	var bill database.Bill
	require.NoError(t, database.GetDB().First(&bill, billID).Error)

	var paymentSums struct {
		Amount    int64 `gorm:"column:amount"`
		TipAmount int64 `gorm:"column:tip_amount"`
	}
	require.NoError(t, database.GetDB().Model(&database.Payment{}).
		Select("COALESCE(SUM(amount), 0) AS amount, COALESCE(SUM(tip_amount), 0) AS tip_amount").
		Where("bill_id = ? AND status = ?", billID, database.PaymentStatusConfirmed).
		Scan(&paymentSums).Error)

	var alternativePaid int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("bill_id = ? AND status = ? AND payment_method IN ?", billID, database.AltPaymentStatusConfirmed, []database.AlternativePaymentMethod{
			database.PaymentMethodCash,
			database.PaymentMethodCard,
			database.PaymentMethodVenmo,
			database.PaymentMethodOther,
		}).
		Scan(&alternativePaid).Error)

	assert.Equal(t, bill.PaidAmount, paymentSums.Amount+alternativePaid)
	assert.Equal(t, bill.TipAmount, paymentSums.TipAmount)
}

func assertPaymentRegressionBillUnchanged(t *testing.T, bill *database.Bill) {
	t.Helper()

	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), reloaded.PaidAmount)
	assert.Equal(t, int64(0), reloaded.TipAmount)
	assert.Equal(t, database.BillStatusOpen, reloaded.Status)

	var paymentCount int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	assert.Zero(t, paymentCount)
}

func performPaymentRegressionRequest(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	return performPaymentRegressionRequestWithGuestSession(t, router, method, path, body, "")
}

func performPaymentRegressionRequestWithGuestSession(t *testing.T, router *gin.Engine, method, path string, body any, guestSessionID string) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(payload)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method == http.MethodPost && strings.HasSuffix(path, "/request-alternative-payment") {
		req.Header.Set("Idempotency-Key", "payment-regression-alt-request")
	}
	if guestSessionID != "" {
		req.AddCookie(&http.Cookie{
			Name:  guestSplitSessionCookie,
			Value: guestSessionID + "." + guestsession.Sign(guestSessionID),
		})
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestPaymentWriteErrorMapsTxHashConflict(t *testing.T) {
	status, message := paymentWriteError(database.ErrPaymentTxHashConflict)

	assert.Equal(t, http.StatusConflict, status)
	assert.Equal(t, "Payment transaction already recorded", message)
}

func TestPaymentWriteErrorMapsInvalidTipAmount(t *testing.T) {
	status, message := paymentWriteError(database.ErrInvalidTipAmount)

	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "Tip amount cannot be negative", message)
}

func TestBillPaymentLedgerParityAcrossSettlementPaths(t *testing.T) {
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "ledger-parity", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 100)

	_, applied, err := database.ApplyConfirmedPayment(database.ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "0xcrypto",
		Amount:        3000,
		TipAmount:     500,
		TxHash:        "ledger-parity-crypto",
		PaymentMethod: "crypto",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	assertPaymentLedgerParity(t, bill.ID)

	_, applied, err = database.CreateConfirmedAlternativePayment(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "cashier",
		ParticipantName: "Cashier",
		Amount:          2500,
		PaymentMethod:   database.PaymentMethodCash,
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	assertPaymentLedgerParity(t, bill.ID)

	_, applied, err = database.ApplyConfirmedPayment(database.ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "plugin-customer",
		Amount:        4500,
		TipAmount:     250,
		TxHash:        "ledger-parity-plugin-reversal",
		PaymentMethod: "stripe",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	assertPaymentLedgerParity(t, bill.ID)

	require.NoError(t, database.ReversePluginPayment("ledger-parity-plugin-reversal"))
	assertPaymentLedgerParity(t, bill.ID)

	_, applied, err = database.ApplyConfirmedPayment(database.ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "plugin-customer",
		Amount:        2000,
		TipAmount:     125,
		TxHash:        "ledger-parity-plugin-refund",
		PaymentMethod: "paypal",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	assertPaymentLedgerParity(t, bill.ID)

	var refundable database.Payment
	require.NoError(t, database.GetDB().Where("tx_hash = ?", "ledger-parity-plugin-refund").First(&refundable).Error)
	_, _, err = database.RefundBillPayment(bill.ID, refundable.ID, "manager", "guest requested refund")
	require.NoError(t, err)
	assertPaymentLedgerParity(t, bill.ID)
}

func TestProcessCryptoPayment_RejectsSubCentNegativeTip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "crypto-negative-tip", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 50)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-crypto-negative-tip"),
		"amount_paid":      30.0,
		"tip_amount":       -0.004,
		"payment_method":   "USDC",
		"quote_token":      signTestQuote(t, "quote-test-secret", bill.ID, 3000, centsToMicrounits(3000)),
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response server.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	// RespondBindError sanitizes gin gte dumps to a product-safe envelope.
	assert.Equal(t, server.ErrCodeInvalidInput, response.Code)
	assert.Equal(t, "Please check the form and try again.", response.Error)
	assert.NotContains(t, w.Body.String(), "TipAmount")
	assert.NotContains(t, w.Body.String(), "Field validation")
	assertPaymentRegressionBillUnchanged(t, bill)
}

func TestProcessCryptoPayment_ReturnsConflictWhenTransactionHashBelongsToAnotherBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "crypto-conflict", nil, "")
	paidBill := createPaymentRegressionBill(t, business.ID, 50)
	targetBill := createPaymentRegressionBill(t, business.ID, 50)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        paidBill.ID,
		PayerAddr:     "existing",
		Amount:        3000,
		TxHash:        testEVMTxHash("tx-crypto-conflict"),
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", targetBill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-crypto-conflict"),
		"amount_paid":      30.0,
		"tip_amount":       0.0,
		"payment_method":   "USDC",
		"quote_token":      signTestQuote(t, "quote-test-secret", targetBill.ID, 3000, centsToMicrounits(3000)),
	})

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "Payment transaction already recorded")

	reloaded, _, err := database.GetBillByID(targetBill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), reloaded.PaidAmount)
	assert.Equal(t, database.BillStatusOpen, reloaded.Status)
}

func TestProcessCryptoPayment_IsIdempotentByTransactionHash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "crypto", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 50)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	body := map[string]any{
		"transaction_hash": testEVMTxHash("tx-crypto-1"),
		"amount_paid":      30.0,
		"tip_amount":       2.0,
		"payment_method":   "USDC",
		"quote_token":      signTestQuote(t, "quote-test-secret", bill.ID, 3200, centsToMicrounits(3200)),
	}

	first := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), body)
	second := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), body)

	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusOK, second.Code)
	assert.Contains(t, first.Body.String(), bill.BillNumber)
	assert.NotContains(t, first.Body.String(), "\"bill_id\"")

	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3000), reloaded.PaidAmount)
	assert.Equal(t, int64(200), reloaded.TipAmount)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("tx_hash = ?", testEVMTxHash("tx-crypto-1")).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestProcessCryptoPaymentUsesSplitShareAmountAndExtendsHold(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "crypto-split-hold", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 100)
	now := time.Now().UTC()
	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-crypto-split",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    2500,
		IdempotencyKey: "crypto-split-hold",
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.NotNil(t, share.HoldExpiresAt)
	originalExpiry := *share.HoldExpiresAt

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequestWithGuestSession(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-crypto-split-hold"),
		"amount_paid":      100.0,
		"tip_amount":       0.0,
		"payment_method":   "USDC",
		"quote_token":      signTestQuote(t, "quote-test-secret", bill.ID, 2500, centsToMicrounits(2500)),
		"split_share_id":   share.ID,
	}, "guest-crypto-split")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var payment database.Payment
	require.NoError(t, database.GetDB().Where("tx_hash = ?", testEVMTxHash("tx-crypto-split-hold")).First(&payment).Error)
	assert.Equal(t, int64(2500), payment.Amount)
	assert.Equal(t, int64(0), payment.TipAmount)

	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2500), reloaded.PaidAmount)

	var reloadedShare database.BillSplitShare
	require.NoError(t, database.GetDB().First(&reloadedShare, share.ID).Error)
	assert.Equal(t, database.BillSplitShareStatusSettled, reloadedShare.Status)
	require.NotNil(t, reloadedShare.PaymentID)
	assert.Equal(t, payment.ID, *reloadedShare.PaymentID)
	assert.Equal(t, "crypto", reloadedShare.Tender)
	require.NotNil(t, reloadedShare.HoldExpiresAt)
	assert.True(t, reloadedShare.HoldExpiresAt.After(originalExpiry))
}

func TestProcessCryptoPaymentRejectsSplitShareFromAnotherGuestSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "crypto-split-wrong-session", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 100)
	now := time.Now().UTC()
	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-owner",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    2500,
		IdempotencyKey: "crypto-split-wrong-session",
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.NotNil(t, share.HoldExpiresAt)
	originalExpiry := *share.HoldExpiresAt

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequestWithGuestSession(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-crypto-split-wrong-session"),
		"amount_paid":      25.0,
		"tip_amount":       0.0,
		"payment_method":   "USDC",
		"quote_token":      signTestQuote(t, "quote-test-secret", bill.ID, 2500, centsToMicrounits(2500)),
		"split_share_id":   share.ID,
	}, "guest-other")
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	var paymentCount int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("tx_hash = ?", testEVMTxHash("tx-crypto-split-wrong-session")).Count(&paymentCount).Error)
	assert.Zero(t, paymentCount)

	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), reloaded.PaidAmount)

	var reloadedShare database.BillSplitShare
	require.NoError(t, database.GetDB().Select("hold_expires_at").First(&reloadedShare, share.ID).Error)
	require.NotNil(t, reloadedShare.HoldExpiresAt)
	assert.True(t, reloadedShare.HoldExpiresAt.Equal(originalExpiry))
}

func TestProcessCrossChainPaymentUsesSplitShareAmountAndExtendsHold(t *testing.T) {
	enableGuestCrossChainSettlementForTest(t) // dormant rail; see guestCrossChainSettlementEnabled
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "cross-chain-split-hold", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 100)
	now := time.Now().UTC()
	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-cross-chain-split",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    2500,
		IdempotencyKey: "cross-chain-split-hold",
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.NotNil(t, share.HoldExpiresAt)
	originalExpiry := *share.HoldExpiresAt

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/cross-chain-payment", handler.ProcessCrossChainPayment)

	w := performPaymentRegressionRequestWithGuestSession(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/cross-chain-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-cross-chain-split-hold"),
		"amount_paid":      100.0,
		"tip_amount":       0.0,
		"source_chain":     "polygon",
		"source_token":     "USDC",
		"lifi_route_id":    "route-split-hold",
		"quote_token":      signTestCrossChainQuote(t, "quote-test-secret", bill.ID, 2500, centsToMicrounits(2500)),
		"split_share_id":   share.ID,
	}, "guest-cross-chain-split")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var payment database.Payment
	require.NoError(t, database.GetDB().Where("tx_hash = ?", testEVMTxHash("tx-cross-chain-split-hold")).First(&payment).Error)
	assert.Equal(t, int64(2500), payment.Amount)
	assert.Equal(t, int64(0), payment.TipAmount)
	assert.Equal(t, "cross-chain", payment.PaymentMethod)
	assert.Equal(t, "polygon", payment.SourceChain)
	assert.Equal(t, "USDC", payment.SourceToken)

	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2500), reloaded.PaidAmount)

	var reloadedShare database.BillSplitShare
	require.NoError(t, database.GetDB().First(&reloadedShare, share.ID).Error)
	assert.Equal(t, database.BillSplitShareStatusSettled, reloadedShare.Status)
	require.NotNil(t, reloadedShare.PaymentID)
	assert.Equal(t, payment.ID, *reloadedShare.PaymentID)
	assert.Equal(t, "cross-chain", reloadedShare.Tender)
	require.NotNil(t, reloadedShare.HoldExpiresAt)
	assert.True(t, reloadedShare.HoldExpiresAt.After(originalExpiry))
}

func TestProcessCryptoPayment_IgnoresForgedCustomerID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	customerA := createPaymentRegressionCustomer(t, "real-payer@example.com")
	customerB := createPaymentRegressionCustomer(t, "forged-payer@example.com")
	business := createPaymentRegressionBusiness(t, "crypto-crm-forged", nil, "")
	require.NoError(t, database.GetDB().Model(business).Update("crm_enabled", true).Error)
	connectPaymentRegressionCustomer(t, customerA.ID, business.ID)
	connectPaymentRegressionCustomer(t, customerB.ID, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 30)
	require.NoError(t, database.GetDB().Model(bill).Update("crm_customer_id", customerA.ID).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), map[string]any{
		"transaction_hash":   testEVMTxHash("tx-crypto-crm-forged"),
		"amount_paid":        30.0,
		"tip_amount":         0.0,
		"payment_method":     "crypto",
		"blockchain_network": "base",
		"customer_id":        customerB.ID,
		"quote_token":        signTestQuote(t, "quote-test-secret", bill.ID, 3000, centsToMicrounits(3000)),
	})

	require.Equal(t, http.StatusOK, w.Code)
	assertSinglePaymentCRMVisitForCustomer(t, bill.ID, customerA.ID)
	assertNoPaymentCRMVisitForCustomer(t, bill.ID, customerB.ID)
}

func TestProcessCrossChainPayment_IgnoresForgedCustomerID(t *testing.T) {
	enableGuestCrossChainSettlementForTest(t) // dormant rail; see guestCrossChainSettlementEnabled
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	customerA := createPaymentRegressionCustomer(t, "real-cross-payer@example.com")
	customerB := createPaymentRegressionCustomer(t, "forged-cross-payer@example.com")
	business := createPaymentRegressionBusiness(t, "cross-crm-forged", nil, "")
	require.NoError(t, database.GetDB().Model(business).Update("crm_enabled", true).Error)
	connectPaymentRegressionCustomer(t, customerA.ID, business.ID)
	connectPaymentRegressionCustomer(t, customerB.ID, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 30)
	require.NoError(t, database.GetDB().Model(bill).Update("crm_customer_id", customerA.ID).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/cross-chain-payment", handler.ProcessCrossChainPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/cross-chain-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-cross-crm-forged"),
		"amount_paid":      30.0,
		"tip_amount":       0.0,
		"source_chain":     "ethereum",
		"source_token":     "USDC",
		"lifi_route_id":    "route-cross-forged",
		"customer_id":      customerB.ID,
		"quote_token":      signTestCrossChainQuote(t, "quote-test-secret", bill.ID, 3000, centsToMicrounits(3000)),
	})

	require.Equal(t, http.StatusOK, w.Code)
	assertSinglePaymentCRMVisitForCustomer(t, bill.ID, customerA.ID)
	assertNoPaymentCRMVisitForCustomer(t, bill.ID, customerB.ID)
}

func TestGuestPaymentAttachesAuthenticatedCustomerWhenBillUnclaimed(t *testing.T) {
	enableGuestCrossChainSettlementForTest(t) // dormant rail; see guestCrossChainSettlementEnabled
	cases := []struct {
		name string
		path string
		body map[string]any
	}{
		{
			name: "crypto",
			path: "/guest/bill/%s/crypto-payment",
			body: map[string]any{
				"transaction_hash":   testEVMTxHash("tx-crypto-auth-crm"),
				"amount_paid":        25.0,
				"tip_amount":         0.0,
				"payment_method":     "crypto",
				"blockchain_network": "base",
			},
		},
		{
			name: "cross-chain",
			path: "/guest/bill/%s/cross-chain-payment",
			body: map[string]any{
				"transaction_hash": testEVMTxHash("tx-cross-auth-crm"),
				"amount_paid":      25.0,
				"tip_amount":       0.0,
				"source_chain":     "ethereum",
				"source_token":     "USDC",
				"lifi_route_id":    "route-auth-crm",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
			setupPaymentRegressionDB(t)

			customer := createPaymentRegressionCustomer(t, tc.name+"-payer@example.com")
			business := createPaymentRegressionBusiness(t, "auth-crm-"+tc.name, nil, "")
			require.NoError(t, database.GetDB().Model(business).Update("crm_enabled", true).Error)
			connectPaymentRegressionCustomer(t, customer.ID, business.ID)
			bill := createPaymentRegressionBill(t, business.ID, 25)
			token := createPaymentRegressionCustomerToken(t, customer)

			handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
			router := gin.New()
			router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)
			router.POST("/guest/bill/:bill_token/cross-chain-payment", handler.ProcessCrossChainPayment)

			body := tc.body
			body["quote_token"] = signTestQuoteForMethod(t, "quote-test-secret", bill.ID, testQuoteMethodForPath(tc.path), 2500, centsToMicrounits(2500), time.Now())
			payload, err := json.Marshal(body)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf(tc.path, bill.PublicToken), bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: "customer_token", Value: token})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var updated database.Bill
			require.NoError(t, database.GetDB().First(&updated, bill.ID).Error)
			require.NotNil(t, updated.CRMCustomerID)
			require.Equal(t, customer.ID, *updated.CRMCustomerID)
			assertSinglePaymentCRMVisitForCustomer(t, bill.ID, customer.ID)
		})
	}
}

func TestProcessCrossChainPayment_DoesNotExposeInternalBillID(t *testing.T) {
	enableGuestCrossChainSettlementForTest(t) // dormant rail; see guestCrossChainSettlementEnabled
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "cross-public", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 30)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/cross-chain-payment", handler.ProcessCrossChainPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/cross-chain-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-cross-public-1"),
		"amount_paid":      30.0,
		"tip_amount":       0.0,
		"source_chain":     "ethereum",
		"source_token":     "USDC",
		"lifi_route_id":    "route-1",
		"quote_token":      signTestCrossChainQuote(t, "quote-test-secret", bill.ID, 3000, centsToMicrounits(3000)),
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), bill.BillNumber)
	assert.NotContains(t, w.Body.String(), "\"bill_id\"")
}

func TestProcessCrossChainPayment_RejectsSubCentNegativeTip(t *testing.T) {
	enableGuestCrossChainSettlementForTest(t) // dormant rail; see guestCrossChainSettlementEnabled
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "cross-negative-tip", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 50)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/cross-chain-payment", handler.ProcessCrossChainPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/cross-chain-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-cross-negative-tip"),
		"amount_paid":      30.0,
		"tip_amount":       -0.004,
		"source_chain":     "ethereum",
		"source_token":     "USDC",
		"lifi_route_id":    "route-negative-tip",
		"quote_token":      signTestCrossChainQuote(t, "quote-test-secret", bill.ID, 3000, centsToMicrounits(3000)),
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response server.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	// RespondBindError sanitizes gin gte dumps to a product-safe envelope.
	assert.Equal(t, server.ErrCodeInvalidInput, response.Code)
	assert.Equal(t, "Please check the form and try again.", response.Error)
	assert.NotContains(t, w.Body.String(), "TipAmount")
	assert.NotContains(t, w.Body.String(), "Field validation")
	assertPaymentRegressionBillUnchanged(t, bill)
}

func TestDuplicateGuestPayments_DoNotRepublishPaymentReceivedEvent(t *testing.T) {
	enableGuestCrossChainSettlementForTest(t) // dormant rail; see guestCrossChainSettlementEnabled
	cases := []struct {
		name string
		path string
		body map[string]any
	}{
		{
			name: "crypto",
			path: "/guest/bill/%s/crypto-payment",
			body: map[string]any{
				"transaction_hash": testEVMTxHash("tx-crypto-event-1"),
				"amount_paid":      30.0,
				"tip_amount":       0.0,
				"payment_method":   "USDC",
			},
		},
		{
			name: "cross-chain",
			path: "/guest/bill/%s/cross-chain-payment",
			body: map[string]any{
				"transaction_hash": testEVMTxHash("tx-cross-event-1"),
				"amount_paid":      30.0,
				"tip_amount":       0.0,
				"source_chain":     "ethereum",
				"source_token":     "USDC",
				"lifi_route_id":    "route-1",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
			setupPaymentRegressionDB(t)

			business := createPaymentRegressionBusiness(t, "event-"+tc.name, nil, "")
			bill := createPaymentRegressionBill(t, business.ID, 30)

			handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
			router := gin.New()
			router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)
			router.POST("/guest/bill/:bill_token/cross-chain-payment", handler.ProcessCrossChainPayment)

			eventsCh, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
			defer cancel()
			_ = drainBusinessEvents(eventsCh)

			body := tc.body
			body["quote_token"] = signTestQuoteForMethod(t, "quote-test-secret", bill.ID, testQuoteMethodForPath(tc.path), 3000, centsToMicrounits(3000), time.Now())

			first := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf(tc.path, bill.PublicToken), body)
			second := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf(tc.path, bill.PublicToken), body)

			assert.Equal(t, http.StatusOK, first.Code)
			assert.Equal(t, http.StatusOK, second.Code)

			require.Eventually(t, func() bool {
				return drainBusinessEvents(eventsCh) >= 1
			}, 200*time.Millisecond, 10*time.Millisecond)

			assert.Equal(t, 0, drainBusinessEvents(eventsCh), "duplicate payment should not emit a second event")
		})
	}
}

func TestProcessCryptoPayment_RejectsUnverifiedTransfers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "verify", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 25)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{err: fmt.Errorf("no matching transfer")}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-invalid"),
		"amount_paid":      25.0,
		"tip_amount":       0.0,
		"payment_method":   "USDC",
		"quote_token":      signTestQuote(t, "quote-test-secret", bill.ID, 2500, centsToMicrounits(2500)),
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Unable to verify payment transaction")
}

// F-CONFIRM: a transfer that matches but is not yet deep enough must be a
// *retryable* response (425 + awaiting_confirmations), not a hard failure, so
// the guest waits and retries the same idempotent tx hash instead of being told
// the payment failed.
func TestProcessCryptoPayment_AwaitingConfirmationsIsRetryable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "confirm", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 25)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{err: blockchain.ErrAwaitingConfirmations}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-pending"),
		"amount_paid":      25.0,
		"tip_amount":       0.0,
		"payment_method":   "USDC",
		"quote_token":      signTestQuote(t, "quote-test-secret", bill.ID, 2500, centsToMicrounits(2500)),
	})

	assert.Equal(t, http.StatusTooEarly, w.Code)
	assert.Contains(t, w.Body.String(), "awaiting_confirmations")
}

// F-QUOTEBIND: a quote token carries the business it was minted for, and the
// settlement path rejects a token whose business does not match the bill's —
// defense-in-depth + self-describing audit on top of the HMAC + bill binding.
func TestProcessCryptoPayment_RejectsQuoteForDifferentBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "qbind", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 25)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	// Correctly signed, correct bill/amount, but bound to a DIFFERENT business.
	badToken := signCryptoQuote(cryptoQuoteClaims{
		BusinessID:     business.ID + 999,
		BillID:         bill.ID,
		LocalCents:     2500,
		USDMicrounits:  centsToMicrounits(2500),
		SettlementAddr: bill.SettlementAddr,
		ChainID:        baseMainnetChainID,
		Token:          "USDC",
		PaymentMethod:  guestPaymentMethodUSDC,
		Iat:            time.Now().Unix(),
		Exp:            time.Now().Add(time.Minute).Unix(),
	}, []byte("quote-test-secret"))

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-qbind"),
		"amount_paid":      25.0,
		"tip_amount":       0.0,
		"payment_method":   "USDC",
		"quote_token":      badToken,
	})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "quote")
}

func TestProcessCryptoPayment_DoesNotAttachCustomerWhenTransferUnverified(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	customer := createPaymentRegressionCustomer(t, "unverified-auth-payer@example.com")
	business := createPaymentRegressionBusiness(t, "verify-auth-crm", nil, "")
	require.NoError(t, database.GetDB().Model(business).Update("crm_enabled", true).Error)
	connectPaymentRegressionCustomer(t, customer.ID, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 25)
	token := createPaymentRegressionCustomerToken(t, customer)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{err: fmt.Errorf("no matching transfer")}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	payload, err := json.Marshal(map[string]any{
		"transaction_hash": testEVMTxHash("tx-invalid-auth-crm"),
		"amount_paid":      25.0,
		"tip_amount":       0.0,
		"payment_method":   "USDC",
		"quote_token":      signTestQuote(t, "quote-test-secret", bill.ID, 2500, centsToMicrounits(2500)),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "customer_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	require.Nil(t, reloaded.CRMCustomerID)
	assertNoPaymentCRMVisitForCustomer(t, bill.ID, customer.ID)
}

func TestProcessCryptoPayment_RequiresEnabledUSDCPaymentPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "crypto-disabled", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 25)
	require.NoError(t, database.UpdateBusinessPluginConfig(business.ID, "usdc_payment", map[string]interface{}{"enabled": false}))

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-disabled"),
		"amount_paid":      25.0,
		"tip_amount":       0.0,
		"payment_method":   "USDC",
		"quote_token":      signTestQuote(t, "quote-test-secret", bill.ID, 2500, centsToMicrounits(2500)),
	})

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Contains(t, w.Body.String(), cryptoPaymentUnavailableMessage())
	assertPaymentRegressionBillUnchanged(t, bill)
}

func TestProcessCrossChainPayment_RequiresEnabledCrossChainPlugin(t *testing.T) {
	enableGuestCrossChainSettlementForTest(t) // dormant rail; see guestCrossChainSettlementEnabled
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "cross-disabled", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 25)
	require.NoError(t, database.UpdateBusinessPluginConfig(business.ID, "cross_chain_payment", map[string]interface{}{"enabled": false}))

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/cross-chain-payment", handler.ProcessCrossChainPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/cross-chain-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-cross-disabled"),
		"amount_paid":      25.0,
		"tip_amount":       0.0,
		"source_chain":     "ethereum",
		"source_token":     "USDC",
		"lifi_route_id":    "route-disabled",
		"quote_token":      signTestCrossChainQuote(t, "quote-test-secret", bill.ID, 2500, centsToMicrounits(2500)),
	})

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Contains(t, w.Body.String(), cryptoPaymentUnavailableMessage())
	assertPaymentRegressionBillUnchanged(t, bill)
}

func TestProcessCryptoPayment_RejectsPaidBills(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "paid", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 25)
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(map[string]any{
		"status":      database.BillStatusPaid,
		"paid_amount": bill.TotalAmount,
	}).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), map[string]any{
		"transaction_hash": testEVMTxHash("tx-paid"),
		"amount_paid":      1.0,
		"tip_amount":       0.0,
		"payment_method":   "USDC",
		"quote_token":      signTestQuote(t, "quote-test-secret", bill.ID, 100, centsToMicrounits(100)),
	})

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "Bill is not open for payment")
}

func TestAlternativePaymentRoutes_EmailOnlyUserCannotManageBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "alt-email", nil, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 25)

	pending := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Guest",
		Amount:          1250,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusPending,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(pending).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.GET("/inside/bills/:bill_id/pending-alternative-payments", handler.GetPendingAlternativePayments)

	w := performPaymentRegressionRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/bills/%d/pending-alternative-payments", bill.ID), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestMarkAlternativePayment_ConfirmsPendingRequestWithoutCreatingDuplicate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(7)
	business := createPaymentRegressionBusiness(t, "alt-confirm", &ownerID, "owner@example.com")
	openPaymentRegressionCashSession(t, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 20)

	pending := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Jane",
		Amount:          1250,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusPending,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(pending).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)
	confirmedBefore := metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestConfirmed)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"request_id":            pending.ID,
		"participant_address":   "guest",
		"amount":                "12.50",
		"payment_method":        "cash",
		"business_confirmation": true,
	})

	assert.Equal(t, http.StatusOK, w.Code)

	var payments []database.AlternativePayment
	require.NoError(t, database.GetDB().Order("id asc").Find(&payments).Error)
	require.Len(t, payments, 1)
	assert.Equal(t, pending.ID, payments[0].ID)
	assert.Equal(t, database.AltPaymentStatusConfirmed, payments[0].Status)
	assert.Equal(t, "Jane", payments[0].ParticipantName)
	assert.Equal(t, "owner@example.com", payments[0].ConfirmedBy)
	require.NotNil(t, payments[0].ConfirmedAt)

	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1250), reloaded.PaidAmount)
	assert.Equal(t, float64(1), metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestConfirmed)-confirmedBefore)
}

func TestMarkAlternativePayment_RejectsStalePendingRequestWithoutExpiresAt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(71)
	business := createPaymentRegressionBusiness(t, "alt-confirm-stale", &ownerID, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 20)
	createdAt := time.Now().UTC().Add(-25 * time.Hour)
	pending := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Stale card",
		Amount:          2000,
		BillAmountCents: 2000,
		PaymentMethod:   database.PaymentMethodCard,
		Status:          database.AltPaymentStatusPending,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	require.NoError(t, database.GetDB().Create(pending).Error)
	require.NoError(t, database.GetDB().Model(pending).Updates(map[string]any{
		"created_at": createdAt,
		"updated_at": createdAt,
		"expires_at": nil,
	}).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"request_id":            pending.ID,
		"participant_address":   "guest",
		"amount":                "20.00",
		"payment_method":        "card",
		"business_confirmation": true,
	})

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var response server.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "payment_request_expired", response.Code)
	assert.Equal(t, "Payment request has expired", response.Error)

	assertPaymentRegressionBillUnchanged(t, bill)
	var reloaded database.AlternativePayment
	require.NoError(t, database.GetDB().First(&reloaded, pending.ID).Error)
	assert.Equal(t, database.AltPaymentStatusPending, reloaded.Status)
	assert.Nil(t, reloaded.ConfirmedAt)
}

func TestMarkAlternativePayment_ConfirmsFreshPendingRequestWithoutExpiresAt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(72)
	business := createPaymentRegressionBusiness(t, "alt-confirm-fresh-age", &ownerID, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 20)
	createdAt := time.Now().UTC().Add(-23 * time.Hour)
	pending := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Fresh card",
		Amount:          2000,
		BillAmountCents: 2000,
		PaymentMethod:   database.PaymentMethodCard,
		Status:          database.AltPaymentStatusPending,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	require.NoError(t, database.GetDB().Create(pending).Error)
	require.NoError(t, database.GetDB().Model(pending).Updates(map[string]any{
		"created_at": createdAt,
		"updated_at": createdAt,
		"expires_at": nil,
	}).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"request_id":            pending.ID,
		"participant_address":   "guest",
		"amount":                "20.00",
		"payment_method":        "card",
		"business_confirmation": true,
	})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2000), reloaded.PaidAmount)
}

func TestGuestSplitAlternativePaymentRequestSettlesShareAfterStaffConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(9)
	business := createPaymentRegressionBusiness(t, "split-alt-confirm", &ownerID, "owner@example.com")
	openPaymentRegressionCashSession(t, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 20)
	now := time.Now().UTC()
	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-split-cashier",
		DisplayName:    "Alex",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    500,
		IdempotencyKey: "split-alt-hold",
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	publicRouter := gin.New()
	publicRouter.POST("/guest/bill/:bill_token/request-alternative-payment", handler.RequestAlternativePayment)

	requestResp := performPaymentRegressionRequestWithGuestSession(t, publicRouter, http.MethodPost, fmt.Sprintf("/guest/bill/%s/request-alternative-payment", bill.PublicToken), map[string]any{
		"amount":           "5.00",
		"tip_amount":       "1.25",
		"payment_method":   "cash",
		"participant_name": "Alex",
		"split_share_id":   share.ID,
	}, "guest-split-cashier")
	require.Equal(t, http.StatusOK, requestResp.Code)

	var pending database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusPending).First(&pending).Error)
	require.Equal(t, int64(500), pending.Amount)
	var requestedShare database.BillSplitShare
	require.NoError(t, database.GetDB().First(&requestedShare, share.ID).Error)
	require.Equal(t, int64(125), requestedShare.TipCents)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	operatorRouter := gin.New()
	operatorRouter.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	operatorRouter.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	confirmResp := performPaymentRegressionRequest(t, operatorRouter, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"request_id":            pending.ID,
		"participant_address":   "guest",
		"amount":                "5.00",
		"payment_method":        "cash",
		"business_confirmation": true,
	})
	require.Equal(t, http.StatusOK, confirmResp.Code)

	select {
	case event := <-ch:
		require.Equal(t, "bill.split.updated", event.Type)
		require.Contains(t, string(event.Data), fmt.Sprintf(`"bill_number":"%s"`, bill.BillNumber))
		require.Contains(t, string(event.Data), `"paid_cents":500`)
	case <-time.After(time.Second):
		t.Fatal("expected split confirmation to publish bill.split.updated")
	}

	var reloadedShare database.BillSplitShare
	require.NoError(t, database.GetDB().First(&reloadedShare, share.ID).Error)
	require.Equal(t, database.BillSplitShareStatusSettled, reloadedShare.Status)
	require.Equal(t, int64(125), reloadedShare.TipCents)
	require.NotNil(t, reloadedShare.AlternativePaymentID)
	require.Equal(t, pending.ID, *reloadedShare.AlternativePaymentID)
	require.Equal(t, "cash", reloadedShare.Tender)

	reloadedBill, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	require.Equal(t, int64(500), reloadedBill.PaidAmount)
	require.Equal(t, int64(125), reloadedBill.TipAmount)
	require.Equal(t, database.BillStatusPartial, reloadedBill.Status)
}

func TestMarkAlternativePayment_DirectDollarAmountPersistsAsCents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(23)
	business := createPaymentRegressionBusiness(t, "alt-direct-dollars", &ownerID, "owner@example.com")
	openPaymentRegressionCashSession(t, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 20)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"participant_address":   "guest",
		"amount":                "12.50",
		"payment_method":        "cash",
		"business_confirmation": true,
	})

	require.Equal(t, http.StatusOK, w.Code)

	var altPayment database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusConfirmed).First(&altPayment).Error)
	assert.Equal(t, int64(1250), altPayment.Amount)

	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1250), reloaded.PaidAmount)
}

func TestMarkAlternativePayment_RejectsFractionalCentDollarAmount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(24)
	business := createPaymentRegressionBusiness(t, "alt-fractional-cent", &ownerID, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 20)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"participant_address":   "guest",
		"amount":                "12.345",
		"payment_method":        "cash",
		"business_confirmation": true,
	})

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "cents precision")
}

func TestMarkAlternativePayment_PendingConfirmationEnqueuesFiscalJobWithAlternativePaymentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(21)
	business := createPaymentRegressionBusiness(t, "alt-pending-fiscal", &ownerID, "owner@example.com")
	openPaymentRegressionCashSession(t, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 12.50)
	seedPaymentRegressionFiscalSettings(t, business.ID)

	pending := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Jane",
		Amount:          1250,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusPending,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(pending).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"request_id":            pending.ID,
		"participant_address":   "guest",
		"amount":                "12.50",
		"payment_method":        "cash",
		"business_confirmation": true,
	})

	require.Equal(t, http.StatusOK, w.Code)

	var jobs []database.FiscalJob
	require.NoError(t, database.GetDB().Where("business_id = ? AND bill_id = ?", business.ID, bill.ID).Find(&jobs).Error)
	require.Len(t, jobs, 1)
	require.NotNil(t, jobs[0].AlternativePaymentID)
	assert.Equal(t, pending.ID, *jobs[0].AlternativePaymentID)
	assert.Nil(t, jobs[0].PaymentID)
	// The alt-payment id is recorded on the job's reference column, but the issue
	// idempotency key is per-BILL (F-DUPISSUE) so it does not embed the payment
	// dimension — that is what prevents two facturas for one bill.
	assert.Equal(t, fmt.Sprintf("business:%d:bill:%d:action:issue_receipt:split:none", business.ID, bill.ID), jobs[0].IdempotencyKey)
}

func TestMarkAlternativePayment_DirectConfirmationEnqueuesFiscalJobWithAlternativePaymentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(22)
	business := createPaymentRegressionBusiness(t, "alt-direct-fiscal", &ownerID, "owner@example.com")
	openPaymentRegressionCashSession(t, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 8.75)
	seedPaymentRegressionFiscalSettings(t, business.ID)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"participant_address":   "guest",
		"amount":                "8.75",
		"payment_method":        "cash",
		"business_confirmation": true,
	})

	require.Equal(t, http.StatusOK, w.Code)

	var altPayment database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusConfirmed).First(&altPayment).Error)

	var jobs []database.FiscalJob
	require.NoError(t, database.GetDB().Where("business_id = ? AND bill_id = ?", business.ID, bill.ID).Find(&jobs).Error)
	require.Len(t, jobs, 1)
	require.NotNil(t, jobs[0].AlternativePaymentID)
	assert.Equal(t, altPayment.ID, *jobs[0].AlternativePaymentID)
	assert.Nil(t, jobs[0].PaymentID)
}

func TestMarkAlternativePayment_DirectPaymentReturnsConflictForNonPayableBill(t *testing.T) {
	cases := []struct {
		name   string
		status database.BillStatus
	}{
		{name: "paid", status: database.BillStatusPaid},
		{name: "closed", status: database.BillStatusClosed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			setupPaymentRegressionDB(t)

			ownerID := uint(17)
			business := createPaymentRegressionBusiness(t, "alt-direct-"+tc.name, &ownerID, "owner@example.com")
			openPaymentRegressionCashSession(t, business.ID)
			bill := createPaymentRegressionBill(t, business.ID, 20)
			require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(map[string]any{
				"status":      tc.status,
				"paid_amount": bill.TotalAmount,
			}).Error)

			handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("token_type", "user")
				c.Set("user_id", ownerID)
				c.Set("email", "owner@example.com")
				c.Next()
			})
			router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

			w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
				"participant_address":   "guest",
				"amount":                "5.00",
				"payment_method":        "cash",
				"business_confirmation": true,
			})

			assert.Equal(t, http.StatusConflict, w.Code)
			var response server.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, "payment_failed", response.Code)
			assert.Equal(t, "Bill is not open for payment", response.Error)

			var paymentCount int64
			require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusConfirmed).Count(&paymentCount).Error)
			assert.Zero(t, paymentCount)
		})
	}
}

func TestMarkAlternativePayment_DirectPaymentReturnsConflictForOverpayment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(19)
	business := createPaymentRegressionBusiness(t, "alt-direct-overpay", &ownerID, "owner@example.com")
	openPaymentRegressionCashSession(t, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 20)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"participant_address":   "guest",
		"amount":                "25.00",
		"payment_method":        "cash",
		"business_confirmation": true,
	})

	assert.Equal(t, http.StatusConflict, w.Code)
	var response server.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "payment_failed", response.Code)
	assert.Equal(t, "Payment exceeds remaining bill balance", response.Error)
	assertPaymentRegressionBillUnchanged(t, bill)

	var alternativePaymentCount int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).Where("bill_id = ?", bill.ID).Count(&alternativePaymentCount).Error)
	assert.Zero(t, alternativePaymentCount)
}

func TestMarkAlternativePayment_PendingRequestReturnsConflictForNonPayableBill(t *testing.T) {
	cases := []struct {
		name   string
		status database.BillStatus
	}{
		{name: "paid", status: database.BillStatusPaid},
		{name: "closed", status: database.BillStatusClosed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			setupPaymentRegressionDB(t)

			ownerID := uint(18)
			business := createPaymentRegressionBusiness(t, "alt-pending-"+tc.name, &ownerID, "owner@example.com")
			openPaymentRegressionCashSession(t, business.ID)
			bill := createPaymentRegressionBill(t, business.ID, 20)
			require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(map[string]any{
				"status":      tc.status,
				"paid_amount": bill.TotalAmount,
			}).Error)

			pending := &database.AlternativePayment{
				BillID:          bill.ID,
				ParticipantAddr: "guest",
				ParticipantName: "Jane",
				Amount:          500,
				PaymentMethod:   database.PaymentMethodCash,
				Status:          database.AltPaymentStatusPending,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			require.NoError(t, database.GetDB().Create(pending).Error)

			handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("token_type", "user")
				c.Set("user_id", ownerID)
				c.Set("email", "owner@example.com")
				c.Next()
			})
			router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

			w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
				"request_id":            pending.ID,
				"participant_address":   "guest",
				"amount":                "5.00",
				"payment_method":        "cash",
				"business_confirmation": true,
			})

			assert.Equal(t, http.StatusConflict, w.Code)
			var response server.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, "payment_failed", response.Code)
			assert.Equal(t, "Bill is not open for payment", response.Error)

			var reloaded database.AlternativePayment
			require.NoError(t, database.GetDB().First(&reloaded, pending.ID).Error)
			assert.Equal(t, database.AltPaymentStatusPending, reloaded.Status)
			assert.Empty(t, reloaded.ConfirmedBy)
			assert.Nil(t, reloaded.ConfirmedAt)
		})
	}
}

func TestMarkAlternativePayment_PendingRequestReturnsConflictForOverpayment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(20)
	business := createPaymentRegressionBusiness(t, "alt-pending-overpay", &ownerID, "owner@example.com")
	openPaymentRegressionCashSession(t, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 20)

	pending := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Jane",
		Amount:          2500,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusPending,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(pending).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"request_id":            pending.ID,
		"participant_address":   "guest",
		"amount":                "25.00",
		"payment_method":        "cash",
		"business_confirmation": true,
	})

	assert.Equal(t, http.StatusConflict, w.Code)
	var response server.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "payment_failed", response.Code)
	assert.Equal(t, "Payment exceeds remaining bill balance", response.Error)
	assertPaymentRegressionBillUnchanged(t, bill)

	var reloaded database.AlternativePayment
	require.NoError(t, database.GetDB().First(&reloaded, pending.ID).Error)
	assert.Equal(t, database.AltPaymentStatusPending, reloaded.Status)
	assert.Empty(t, reloaded.ConfirmedBy)
	assert.Nil(t, reloaded.ConfirmedAt)
}

func TestAlternativePaymentEndpoints_ExcludePluginTrackingRecords(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	ownerID := uint(8)
	business := createPaymentRegressionBusiness(t, "alt-plugin-filter", &ownerID, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 30)

	pendingPlugin := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "plugin-payment-1",
		ParticipantName: "stripe",
		Amount:          15,
		PaymentMethod:   database.AlternativePaymentMethod("stripe"),
		Status:          database.AltPaymentStatusPending,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	confirmedPlugin := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "plugin-payment-2",
		ParticipantName: "stripe",
		Amount:          12,
		PaymentMethod:   database.AlternativePaymentMethod("stripe"),
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	pendingCash := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Guest",
		Amount:          10,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusPending,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	confirmedCash := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Guest",
		Amount:          8,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(pendingPlugin).Error)
	require.NoError(t, database.GetDB().Create(confirmedPlugin).Error)
	require.NoError(t, database.GetDB().Create(pendingCash).Error)
	require.NoError(t, database.GetDB().Create(confirmedCash).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)

	pendingRouter := gin.New()
	pendingRouter.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	pendingRouter.GET("/inside/bills/:bill_id/pending-alternative-payments", handler.GetPendingAlternativePayments)

	pendingResp := performPaymentRegressionRequest(
		t,
		pendingRouter,
		http.MethodGet,
		fmt.Sprintf("/inside/bills/%d/pending-alternative-payments", bill.ID),
		nil,
	)
	assert.Equal(t, http.StatusOK, pendingResp.Code)
	assert.Contains(t, pendingResp.Body.String(), "\"payment_method\":\"cash\"")
	assert.NotContains(t, pendingResp.Body.String(), "\"payment_method\":\"stripe\"")

	confirmedRouter := gin.New()
	confirmedRouter.GET("/guest/bill/:bill_token/alternative-payments", handler.GetBillAlternativePayments)

	confirmedResp := performPaymentRegressionRequest(
		t,
		confirmedRouter,
		http.MethodGet,
		fmt.Sprintf("/guest/bill/%s/alternative-payments", bill.PublicToken),
		nil,
	)
	assert.Equal(t, http.StatusOK, confirmedResp.Code)
	assert.Contains(t, confirmedResp.Body.String(), "\"payment_method\":\"cash\"")
	assert.NotContains(t, confirmedResp.Body.String(), "\"payment_method\":\"stripe\"")
}

func TestGuestAlternativePaymentRoutes_UseBillNumber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "guest-alt", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 48)

	confirmedCash := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Guest",
		Amount:          12,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(confirmedCash).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/request-alternative-payment", handler.RequestAlternativePayment)
	router.GET("/guest/bill/:bill_token/alternative-payments", handler.GetBillAlternativePayments)
	router.GET("/guest/bill/:bill_token/payment-breakdown", handler.GetBillPaymentBreakdown)

	requestResp := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/request-alternative-payment", bill.PublicToken), map[string]any{
		"amount":           "5.00",
		"payment_method":   "cash",
		"participant_name": "Alex",
	})
	assert.Equal(t, http.StatusOK, requestResp.Code)

	var pending database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusPending).First(&pending).Error)
	assert.Equal(t, "Alex", pending.ParticipantName)
	assert.Equal(t, int64(500), pending.Amount)

	paymentsResp := performPaymentRegressionRequest(t, router, http.MethodGet, fmt.Sprintf("/guest/bill/%s/alternative-payments", bill.PublicToken), nil)
	assert.Equal(t, http.StatusOK, paymentsResp.Code)
	assert.Contains(t, paymentsResp.Body.String(), "\"payment_method\":\"cash\"")

	breakdownResp := performPaymentRegressionRequest(t, router, http.MethodGet, fmt.Sprintf("/guest/bill/%s/payment-breakdown", bill.PublicToken), nil)
	assert.Equal(t, http.StatusOK, breakdownResp.Code)
	assert.Contains(t, breakdownResp.Body.String(), "\"alternative_paid\":0.12")
}

func TestGetBillAlternativePayments_GuestProjectionOmitsStaffFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "guest-alt-projection", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 48)

	confirmedCash := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "0xabc",
		ParticipantName: "Guest",
		Amount:          1250,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		ConfirmedBy:     "owner@example.com",
		ResolvedBy:      "staff:7",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(confirmedCash).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.GET("/guest/bill/:bill_token/alternative-payments", handler.GetBillAlternativePayments)

	paymentsResp := performPaymentRegressionRequest(t, router, http.MethodGet, fmt.Sprintf("/guest/bill/%s/alternative-payments", bill.PublicToken), nil)
	assert.Equal(t, http.StatusOK, paymentsResp.Code)
	body := paymentsResp.Body.String()
	assert.Contains(t, body, "\"payment_method\":\"cash\"")
	assert.Contains(t, body, "\"amount\":12.5")
	assert.Contains(t, body, "\"confirmed_at\"")
	assert.NotContains(t, body, "confirmed_by")
	assert.NotContains(t, body, "owner@example.com")
	assert.NotContains(t, body, "resolved_by")
	assert.NotContains(t, body, "participant_address")
	assert.NotContains(t, body, "0xabc")
}

func TestGetBillPaymentBreakdown_LogsInconsistentAlternativePayments(t *testing.T) {
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "breakdown-inconsistent", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 20)
	require.NoError(t, database.GetDB().Model(bill).Update("paid_amount", int64(500)).Error)

	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Guest",
		Amount:          1200,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}).Error)

	var logBuffer bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logBuffer)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
	})

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	breakdown, err := handler.getBillPaymentBreakdown(bill.ID)

	require.NoError(t, err)
	assert.Equal(t, int64(0), breakdown.CryptoPaid)
	assert.Equal(t, int64(1200), breakdown.AlternativePaid)
	assert.Contains(t, logBuffer.String(), "payment breakdown inconsistency")
	assert.Contains(t, logBuffer.String(), fmt.Sprintf("bill_id=%d", bill.ID))
	assert.Contains(t, logBuffer.String(), "paid_amount_cents=500")
}

// signTestQuote signs a payer-bound guest USDC quote whose persisted EXACT
// amount is usdMicrounits (see persistTestCryptoQuote), so fixture verifiers
// can keep transferring the amount the test names.
func signTestQuote(t testing.TB, secret string, billID uint, localCents, usdMicrounits int64) string {
	t.Helper()
	return signTestQuoteIssuedAt(t, secret, billID, localCents, usdMicrounits, time.Now())
}

// signTestCrossChainQuote is signTestQuote for the cross-chain rail.
func signTestCrossChainQuote(t testing.TB, secret string, billID uint, localCents, usdMicrounits int64) string {
	t.Helper()
	return signTestQuoteForMethod(t, secret, billID, guestPaymentMethodCrossChain, localCents, usdMicrounits, time.Now())
}

func TestProcessCryptoPayment_NonUSDBillSettlesWithLockedQuote(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "crypto-eur", nil, "")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).Update("default_currency", "EUR").Error)
	bill := createPaymentRegressionBill(t, business.ID, 100) // 100.00 EUR -> 10000 cents

	// Quote: 10000 EUR cents locked at 108_000_000 USDC micro-units ($108).
	token := signTestQuote(t, "quote-test-secret", bill.ID, 10_000, 108_000_000)

	verifier := &mockGuestPaymentVerifier{}
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, verifier, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken),
		map[string]any{
			"transaction_hash": testEVMTxHash("tx-eur-locked"),
			"amount_paid":      100.0,
			"tip_amount":       0.0,
			"payment_method":   "USDC",
			"quote_token":      token,
		})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	// Ledger stays in local (EUR) cents.
	assert.Equal(t, int64(10_000), reloaded.PaidAmount)

	// The on-chain amount must be the LOCKED USD amount from the quote, NOT the
	// local-currency cents — this is the non-USD settlement fix. Payer binding
	// makes it an exact match, never "at least".
	assert.False(t, verifier.atLeastCalled, "payer-bound quotes verify the exact amount")
	assert.Equal(t, int64(108_000_000), verifier.lastExpectedMicrounits)
	assert.NotEqual(t, centsToMicrounits(10_000), verifier.lastExpectedMicrounits)
}

func TestProcessCryptoPayment_RejectsTamperedQuote(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "crypto-tampered", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 50)
	token := signTestQuote(t, "WRONG-secret", bill.ID, 3000, 30_000_000)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken),
		map[string]any{
			"transaction_hash": testEVMTxHash("tx-tampered"),
			"amount_paid":      30.0,
			"tip_amount":       0.0,
			"payment_method":   "USDC",
			"quote_token":      token,
		})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "quote")
	assertPaymentRegressionBillUnchanged(t, bill)
}

func TestProcessCryptoPayment_RejectsQuoteAmountMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "crypto-mismatch", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 100)
	// Quote covers only 1000 cents but the request claims to pay 10000.
	token := signTestQuote(t, "quote-test-secret", bill.ID, 1_000, 10_000_000)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken),
		map[string]any{
			"transaction_hash": testEVMTxHash("tx-mismatch"),
			"amount_paid":      100.0,
			"tip_amount":       0.0,
			"payment_method":   "USDC",
			"quote_token":      token,
		})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertPaymentRegressionBillUnchanged(t, bill)
}

func TestProcessCryptoPayment_RejectsMissingQuote(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "crypto-noquote", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 50)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken),
		map[string]any{
			"transaction_hash": testEVMTxHash("tx-noquote"),
			"amount_paid":      30.0,
			"tip_amount":       0.0,
			"payment_method":   "USDC",
		})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assertPaymentRegressionBillUnchanged(t, bill)
}

func TestProcessCrossChainPayment_NonUSDBillSettlesWithLockedQuote(t *testing.T) {
	enableGuestCrossChainSettlementForTest(t) // dormant rail; see guestCrossChainSettlementEnabled
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "xchain-eur", nil, "")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).Update("default_currency", "EUR").Error)
	bill := createPaymentRegressionBill(t, business.ID, 100)
	token := signTestCrossChainQuote(t, "quote-test-secret", bill.ID, 10_000, 108_000_000)

	verifier := &mockGuestPaymentVerifier{}
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, verifier, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/cross-chain-payment", handler.ProcessCrossChainPayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/cross-chain-payment", bill.PublicToken),
		map[string]any{
			"transaction_hash": testEVMTxHash("tx-xchain-eur"),
			"amount_paid":      100.0,
			"tip_amount":       0.0,
			"source_chain":     "base",
			"source_token":     "USDC",
			"quote_token":      token,
		})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(10_000), reloaded.PaidAmount)
	// The cross-chain floor must be the LOCKED USD amount, not local cents.
	assert.Equal(t, int64(108_000_000), verifier.lastExpectedMicrounits)
	assert.NotEqual(t, centsToMicrounits(10_000), verifier.lastExpectedMicrounits)
}

func TestMarkAlternativePayment_StaffServerCanRecordCash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)
	server.InitializeRBAC(database.GetDBWrapper())

	ownerID := uint(91)
	business := createPaymentRegressionBusiness(t, "alt-staff-server", &ownerID, "owner@example.com")
	openPaymentRegressionCashSession(t, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 18.00)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleServer))
		c.Set("staff_id", uint(7))
		c.Set("staff_name", "Floor Server")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"amount":                "18.00",
		"payment_method":        "cash",
		"business_confirmation": true,
		"participant_name":      "Table walk-in",
	})

	require.Equal(t, http.StatusOK, w.Code)

	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, database.BillStatusPaid, reloaded.Status)
	assert.Equal(t, int64(1800), reloaded.PaidAmount)

	var altPayment database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusConfirmed).First(&altPayment).Error)
	assert.Equal(t, database.PaymentMethodCash, altPayment.PaymentMethod)
	assert.Equal(t, "counter", altPayment.ParticipantAddr)
	assert.Equal(t, "Table walk-in", altPayment.ParticipantName)
}

func TestMarkAlternativePayment_StaffServerCanRecordCashWithTip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)
	server.InitializeRBAC(database.GetDBWrapper())

	ownerID := uint(92)
	business := createPaymentRegressionBusiness(t, "alt-staff-tip", &ownerID, "owner@example.com")
	openPaymentRegressionCashSession(t, business.ID)
	bill := createPaymentRegressionBill(t, business.ID, 20.00)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleServer))
		c.Set("staff_id", uint(8))
		c.Set("staff_name", "Floor Server")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"amount":                "20.00",
		"tip_amount":            "3.00",
		"payment_method":        "cash",
		"business_confirmation": true,
	})

	require.Equal(t, http.StatusOK, w.Code)

	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, database.BillStatusPaid, reloaded.Status)
	assert.Equal(t, int64(2000), reloaded.PaidAmount)
	assert.Equal(t, int64(300), reloaded.TipAmount)
}

func TestMarkAlternativePayment_RejectsCashWithoutOpenSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)
	server.InitializeRBAC(database.GetDBWrapper())

	ownerID := uint(93)
	business := createPaymentRegressionBusiness(t, "alt-cash-no-session", &ownerID, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 18.00)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"amount":                "18.00",
		"payment_method":        "cash",
		"business_confirmation": true,
		"participant_name":      "Table walk-in",
	})

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var response server.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "cash_session_required", response.Code)
	assert.Equal(t, "Open a cash register session before recording cash", response.Error)

	assertPaymentRegressionBillUnchanged(t, bill)
	var paymentCount int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	assert.Zero(t, paymentCount)
}

func TestMarkAlternativePayment_DemoHouseRailOpensSeededDrawer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)
	server.InitializeRBAC(database.GetDBWrapper())

	ownerID := uint(769)
	business := createPaymentRegressionBusiness(t, "demo-dinner-cash", &ownerID, "owner@example.com")
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]any{
		"is_demo": true,
		"kind":    database.BusinessKindDemo,
	}).Error)
	closedAt := time.Date(2026, 8, 21, 4, 0, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 20000,
		CashSalesCents:    118230,
		OpenedByLabel:     "demo-manager",
		OpenedAt:          closedAt.Add(-2 * time.Hour),
		ClosedByLabel:     "demo-manager",
		ClosedAt:          &closedAt,
	}).Error)
	bill := createPaymentRegressionBill(t, business.ID, 18.00)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"amount":                "18.00",
		"payment_method":        "cash",
		"business_confirmation": true,
		"participant_name":      "Table walk-in",
	})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, database.BillStatusPaid, reloaded.Status)
	assert.Equal(t, int64(1800), reloaded.PaidAmount)

	open, err := database.FindOpenCashRegisterSessionForBusinessTx(database.GetDB(), business.ID)
	require.NoError(t, err)
	require.NotNil(t, open)
	assert.Equal(t, "demo-dinner-drawer", open.OpenedByLabel)
}

func TestMarkAlternativePayment_AllowsCardWithoutOpenSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)
	server.InitializeRBAC(database.GetDBWrapper())

	ownerID := uint(94)
	business := createPaymentRegressionBusiness(t, "alt-card-no-session", &ownerID, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 18.00)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/alternative-payment", handler.MarkAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/bills/%d/alternative-payment", bill.ID), map[string]any{
		"amount":                "18.00",
		"payment_method":        "card",
		"business_confirmation": true,
	})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, database.BillStatusPaid, reloaded.Status)
	assert.Equal(t, int64(1800), reloaded.PaidAmount)
}

func TestRequestAlternativePaymentDoesNotRaisePaymentReceivedAlert(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)
	// The regression harness does not migrate alert tables; without this the
	// alert insert fails silently and the test could never catch it.
	require.NoError(t, database.GetDB().AutoMigrate(&database.OperationalAlert{}, &database.OperationalAlertEvent{}))

	business := createPaymentRegressionBusiness(t, "altreq-alert", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 20.00)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/request-alternative-payment", handler.RequestAlternativePayment)

	rec := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/request-alternative-payment", bill.PublicToken), map[string]any{
			"amount":           "5.00",
			"payment_method":   "cash",
			"participant_name": "Alex",
		})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var count int64
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("business_id = ? AND alert_type = ?", business.ID, database.OperationalAlertTypePaymentReceived).
		Count(&count).Error)
	require.Zero(t, count, "a payment REQUEST must not raise a payment_received alert")
}
