package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

type returnFixture struct {
	bill   *database.Bill
	plugin *testPaymentPlugin
}

// setupReturnFixture seeds a USD business, a tabled bill in billStatus, an
// enabled provider config, a pending tracker, and a return-capture double
// answering with currency.
func setupReturnFixture(t *testing.T, provider string, billStatus database.BillStatus, trackerAddr, captureID, currency string) returnFixture {
	t.Helper()
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.Table{}, &database.Bill{}, &database.Payment{}, &database.AlternativePayment{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	table := &database.Table{BusinessID: business.ID, TableCode: fmt.Sprintf("RU-%d", time.Now().UnixNano()), Name: "Return"}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  fmt.Sprintf("B-return-unsettleable-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 2500,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	if billStatus != database.BillStatusOpen {
		require.NoError(t, database.GetDB().Model(bill).Update("status", billStatus).Error)
		bill.Status = billStatus
	}

	pluginRecord := &database.Plugin{Name: provider, DisplayName: provider, Category: database.PluginCategoryPayment, IsActive: true}
	require.NoError(t, database.GetDB().Create(pluginRecord).Error)
	config := map[string]interface{}{
		"client_id":     "client-id-12345678901234567890",
		"client_secret": "client-secret-12345678901234567890",
		"webhook_id":    "WH123456789",
	}
	if provider == "mercadopago" {
		config = map[string]interface{}{
			"access_token":   "APP_USR-1234567890-test-token",
			"public_key":     "APP_USR-1234567890-test-key",
			"webhook_secret": "whsec",
			"base_url":       "https://api.staging.example",
			"environment":    "sandbox",
		}
	}
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRecord.ID, config))

	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: trackerAddr,
		ParticipantName: provider,
		Amount:          2500,
		PaymentMethod:   database.AlternativePaymentMethod(provider),
		Status:          database.AltPaymentStatusPending,
	}).Error)

	mock := &testPaymentPlugin{
		name: provider,
		returnResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: captureID,
			BillID:    bill.ID,
			Amount:    2500,
			Currency:  currency,
			Metadata:  map[string]interface{}{"payverge_tracker_id": trackerAddr},
		},
	}
	original, hadOriginal := plugins.GlobalRegistry.GetPlugin(provider)
	plugins.GlobalRegistry.RegisterPlugin(mock)
	t.Cleanup(func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(original)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin(provider)
		}
	})
	return returnFixture{bill: bill, plugin: mock}
}

func requireFailedReturnRedirect(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusFound, w.Code)
	location, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "failed", location.Query().Get("payment"))
	require.Empty(t, location.Query().Get("payment_id"), "a failed return must not open the status checker")
}

func requireNoLedgerRow(t *testing.T, billID uint) {
	t.Helper()
	var count int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).Where("bill_id = ?", billID).Count(&count).Error)
	require.Zero(t, count)
}

// MercadoPago captures at checkout, so a return onto a bill that staff closed
// meanwhile holds guest funds that can never settle: refund, redirect failed.
func TestHandleMercadoPagoReturn_ClosedBillCaptureIsRefunded(t *testing.T) {
	fx := setupReturnFixture(t, "mercadopago", database.BillStatusClosed, "mp_tracker_closed", "555001", "USD")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/mercadopago/return?bill_id=%d&payment_id=555001&status=approved", fx.bill.ID), nil)
	NewPluginHandlers(nil, nil).HandleMercadoPagoReturn(c)

	requireFailedReturnRedirect(t, w)
	require.Equal(t, 1, fx.plugin.refundCalls)
	require.Equal(t, "555001", fx.plugin.lastRefundPaymentID)
	require.Equal(t, int64(0), fx.plugin.lastRefundAmount, "full refund")
	requireNoLedgerRow(t, fx.bill.ID)

	var refreshed database.Bill
	require.NoError(t, database.GetDB().First(&refreshed, fx.bill.ID).Error)
	require.Equal(t, database.BillStatusClosed, refreshed.Status)
	require.Zero(t, refreshed.PaidAmount)
}

func TestHandleMercadoPagoReturn_CurrencyMismatchCaptureIsRefunded(t *testing.T) {
	fx := setupReturnFixture(t, "mercadopago", database.BillStatusOpen, "mp_tracker_ccy", "555002", "ARS")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/mercadopago/return?bill_id=%d&payment_id=555002&status=approved", fx.bill.ID), nil)
	NewPluginHandlers(nil, nil).HandleMercadoPagoReturn(c)

	requireFailedReturnRedirect(t, w)
	require.Equal(t, 1, fx.plugin.refundCalls)
	require.Equal(t, "555002", fx.plugin.lastRefundPaymentID)
	requireNoLedgerRow(t, fx.bill.ID)

	var refreshed database.Bill
	require.NoError(t, database.GetDB().First(&refreshed, fx.bill.ID).Error)
	require.Equal(t, database.BillStatusOpen, refreshed.Status)
	require.Zero(t, refreshed.PaidAmount)
}

// A PayPal order is only authorized until the return path captures it, so a
// terminal bill must not be captured at all (nothing to refund).
func TestHandlePayPalReturn_ClosedBillIsNotCaptured(t *testing.T) {
	fx := setupReturnFixture(t, "paypal", database.BillStatusClosed, "ORDER-CLOSED", "capture-closed", "USD")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/paypal/return?bill_id=%d&token=ORDER-CLOSED&PayerID=payer-1", fx.bill.ID), nil)
	NewPluginHandlers(nil, nil).HandlePayPalReturn(c)

	requireFailedReturnRedirect(t, w)
	require.Empty(t, fx.plugin.lastReturnOrder, "a terminal bill must not drive a PayPal capture")
	require.Zero(t, fx.plugin.refundCalls)
	requireNoLedgerRow(t, fx.bill.ID)
}

// A capture the bill cannot absorb (here: the bill was already paid in full by
// another tender after the guest left for PayPal) is refunded.
func TestHandlePayPalReturn_OverpayCaptureIsRefunded(t *testing.T) {
	fx := setupReturnFixture(t, "paypal", database.BillStatusOpen, "ORDER-OVER", "capture-over", "USD")
	require.NoError(t, database.GetDB().Model(fx.bill).Updates(map[string]interface{}{
		"paid_amount": 2000,
		"status":      database.BillStatusPartial,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/paypal/return?bill_id=%d&token=ORDER-OVER&PayerID=payer-1", fx.bill.ID), nil)
	NewPluginHandlers(nil, nil).HandlePayPalReturn(c)

	requireFailedReturnRedirect(t, w)
	require.Equal(t, "ORDER-OVER", fx.plugin.lastReturnOrder)
	require.Equal(t, 1, fx.plugin.refundCalls)
	require.Equal(t, "capture-over", fx.plugin.lastRefundPaymentID)
	requireNoLedgerRow(t, fx.bill.ID)
}

func seedSettledReturnLedgerRow(t *testing.T, fx returnFixture, captureID string) {
	t.Helper()
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:        fx.bill.ID,
		PayerAddr:     "plugin",
		Amount:        2500,
		TxHash:        "plugin_" + captureID,
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "mercadopago",
	}).Error)
}

func requireSettledReturnNotRefunded(t *testing.T, fx returnFixture, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, 0, fx.plugin.refundCalls, "a capture already on the ledger must never be refunded")
	if loc, err := url.Parse(w.Header().Get("Location")); err == nil {
		require.NotEqual(t, "failed", loc.Query().Get("payment"), "no failed redirect for a settled capture")
	}
	var alerts int64
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).Count(&alerts).Error)
	require.Zero(t, alerts, "no reconciliation alert for a settled capture")
}

// The webhook settled the capture first; a return carrying a mismatched
// currency must not refund the funds that landed.
func TestMercadoPagoReturn_SettledPaymentNeverRefunded(t *testing.T) {
	fx := setupReturnFixture(t, "mercadopago", database.BillStatusOpen, "mp_tracker_settled", "555003", "ARS")
	require.NoError(t, database.GetDB().AutoMigrate(&database.OperationalAlert{}))
	seedSettledReturnLedgerRow(t, fx, "555003")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/mercadopago/return?bill_id=%d&payment_id=555003&status=approved", fx.bill.ID), nil)
	NewPluginHandlers(nil, nil).HandleMercadoPagoReturn(c)

	requireSettledReturnNotRefunded(t, fx, w)
}

func TestMercadoPagoReturn_SettledPaymentBreakdownMismatchNeverRefunded(t *testing.T) {
	fx := setupReturnFixture(t, "mercadopago", database.BillStatusOpen, "mp_tracker_settled_bd", "555004", "USD")
	require.NoError(t, database.GetDB().AutoMigrate(&database.OperationalAlert{}))
	fx.plugin.returnResponse.Amount = 2400 // tracker bound 2500: breakdown mismatch
	seedSettledReturnLedgerRow(t, fx, "555004")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/mercadopago/return?bill_id=%d&payment_id=555004&status=approved", fx.bill.ID), nil)
	NewPluginHandlers(nil, nil).HandleMercadoPagoReturn(c)

	requireSettledReturnNotRefunded(t, fx, w)
}
