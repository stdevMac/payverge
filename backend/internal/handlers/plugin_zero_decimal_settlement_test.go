package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

// driveCreatePluginPayment posts a guest plugin-payment initiation for the given
// bill and returns the recorder plus the registered plugin double (so callers can
// assert whether the provider was ever called). It re-registers a fresh
// testPaymentPlugin under the checkout plugin name so the mock's lastBillID field
// reveals whether CreateBillPayment fired.
func driveCreatePluginPayment(t *testing.T, billNumber, pluginName string, amount, tip float64, metadata map[string]interface{}) (*httptest.ResponseRecorder, *testPaymentPlugin) {
	t.Helper()

	mock := &testPaymentPlugin{name: pluginName}
	plugins.GlobalRegistry.RegisterPlugin(mock)

	payload := map[string]interface{}{
		"plugin_id":  pluginName,
		"amount":     amount,
		"currency":   "USD", // ignored server-side; business.DefaultCurrency is authoritative
		"tip_amount": tip,
	}
	if metadata != nil {
		payload["metadata"] = metadata
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: billNumber}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	NewPluginHandlers(nil, nil).CreatePluginPayment(c)
	return w, mock
}

// setBusinessCurrency forces the business's authoritative DefaultCurrency and the
// bill's outstanding amount so the representability guard runs against a specific
// currency + cents amount.
func setBusinessCurrency(t *testing.T, businessID uint, currency string, billCents int64) {
	t.Helper()
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", businessID).
		Update("default_currency", currency).Error)
	require.NoError(t, database.GetDB().Model(&database.Bill{}).
		Where("business_id = ?", businessID).
		Updates(map[string]interface{}{"total_amount": billCents, "paid_amount": 0}).Error)
}

// TestCreatePluginPayment_ZeroDecimal_NonWholeBillRejected (FIX A): a JPY bill
// whose outstanding amount is not a whole major unit (¥333.33 = 33333 cents)
// cannot be charged exactly, so initiation is rejected 422 BEFORE any provider
// call — no money moves.
func TestCreatePluginPayment_ZeroDecimal_NonWholeBillRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, bill, pluginName := setupPluginCheckoutAccessDB(t, nil)
	setBusinessCurrency(t, business.ID, "JPY", 33333) // ¥333.33 — not representable

	w, mock := driveCreatePluginPayment(t, bill.PublicToken, pluginName, 333.33, 0, nil)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "amount_not_representable")
	assert.Equal(t, uint(0), mock.lastBillID, "provider CreateBillPayment must NOT be called for a non-representable amount")
}

// TestCreatePluginPayment_ZeroDecimal_NonWholeTipRejected (FIX A): a whole-yen
// bill with a fractional-yen tip (¥33.33 = 3333 cents) is also rejected — the tip
// alone is not representable.
func TestCreatePluginPayment_ZeroDecimal_NonWholeTipRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, bill, pluginName := setupPluginCheckoutAccessDB(t, nil)
	setBusinessCurrency(t, business.ID, "JPY", 100000) // ¥1000 — representable bill

	w, mock := driveCreatePluginPayment(t, bill.PublicToken, pluginName, 1000, 33.33, nil)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "amount_not_representable")
	assert.Equal(t, uint(0), mock.lastBillID, "provider must not be called when the tip is not representable")
}

// TestCreatePluginPayment_ZeroDecimal_WholeAmountAccepted (FIX A): a whole-yen
// bill and tip round-trip exactly and are accepted; the provider is called.
func TestCreatePluginPayment_ZeroDecimal_WholeAmountAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, bill, pluginName := setupPluginCheckoutAccessDB(t, nil)
	setBusinessCurrency(t, business.ID, "JPY", 100000) // ¥1000

	w, mock := driveCreatePluginPayment(t, bill.PublicToken, pluginName, 1000, 50, nil)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, bill.ID, mock.lastBillID, "whole-yen amount must reach the provider")
	assert.Equal(t, "JPY", mock.lastCurrency)
}

// TestCreatePluginPayment_TwoDecimal_FractionalAccepted (FIX A): USD (2-decimal)
// round-trips any cents amount exactly, so a fractional-dollar bill is accepted —
// the guard must not fire for currencies with a sub-unit.
func TestCreatePluginPayment_TwoDecimal_FractionalAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	business, bill, pluginName := setupPluginCheckoutAccessDB(t, nil)
	setBusinessCurrency(t, business.ID, "USD", 3333) // $33.33

	w, mock := driveCreatePluginPayment(t, bill.PublicToken, pluginName, 33.33, 1.11, nil)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, bill.ID, mock.lastBillID, "USD fractional amount must reach the provider")
}

// driveBreakdownMismatchWebhook drives a signature-verified "completed"
// MercadoPago webhook whose metadata bill/tip cents sum does NOT match the
// webhook amount_total — the shape a zero-decimal currency produces when the
// provider lossily rounds the charge (¥333.33 metadata vs ¥333 captured).
func driveBreakdownMismatchWebhook(t *testing.T, businessID, billID uint, paymentID string, capturedAmountCents, metadataBillCents int64, refundErr error) (*httptest.ResponseRecorder, *testPaymentPlugin) {
	t.Helper()
	mock := &testPaymentPlugin{
		name:            "mercadopago",
		verifySignature: true,
		refundErr:       refundErr,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			BillID:    billID,
			PaymentID: paymentID,
			Amount:    capturedAmountCents, // rounded, settle-able amount the PSP actually took
			Currency:  "JPY",
			Metadata: map[string]interface{}{
				// Unrounded cents kept at initiation — sums to a DIFFERENT total.
				"bill_amount_cents": fmt.Sprintf("%d", metadataBillCents),
				"tip_amount_cents":  "0",
			},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(mock)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/?bill_id=%d&business_id=%d", billID, businessID),
		bytes.NewBufferString(fmt.Sprintf(`{"id":"evt-%s","action":"payment.updated","data":{"id":"%s"}}`, paymentID, paymentID)),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")
	return w, mock
}

// TestHandlePaymentWebhook_BreakdownMismatch_AutoRefundsAndAcks (FIX B): a
// completed capture whose bill/tip breakdown cannot reconcile against the webhook
// amount (zero-decimal rounding) must be auto-refunded, alerted, and — on refund
// success — 200-acked so the PSP does not retry.
func TestHandlePaymentWebhook_BreakdownMismatch_AutoRefundsAndAcks(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-breakdown-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 33333,
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	// Captured ¥333 -> 33300 cents; metadata kept the unrounded 33333 -> mismatch.
	w, mock := driveBreakdownMismatchWebhook(t, business.ID, bill.ID, "mp-breakdown-1", 33300, 33333, nil)

	assert.Equal(t, http.StatusOK, w.Code, "successful auto-refund of a mismatched capture must ack 200")
	assert.Contains(t, w.Body.String(), "auto-refunded")
	require.Equal(t, 1, mock.refundCalls, "mismatched capture must be auto-refunded")
	assert.Equal(t, "mp-breakdown-1", mock.lastRefundPaymentID)

	// Bill must be untouched — the un-settleable capture never lands.
	refreshed, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), refreshed.PaidAmount)

	// No settlement Payment row for the refunded capture.
	_, lookupErr := database.GetPaymentByTxHash("plugin_mp-breakdown-1")
	assert.Error(t, lookupErr, "mismatched capture must not produce a settlement payment")

	assert.Equal(t, int64(1), countRefundReviewAlerts(t, business.ID))
}

// TestHandlePaymentWebhook_BreakdownMismatch_RefundFailureRetries (FIX B): when
// the auto-refund of a mismatched capture FAILS, keep the non-2xx response so the
// PSP retries, and raise the manual-action alert.
func TestHandlePaymentWebhook_BreakdownMismatch_RefundFailureRetries(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-breakdown-fail-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 33333,
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	w, mock := driveBreakdownMismatchWebhook(t, business.ID, bill.ID, "mp-breakdown-fail-1", 33300, 33333, fmt.Errorf("psp refund unavailable"))

	assert.NotEqual(t, http.StatusOK, w.Code, "failed auto-refund must keep a non-2xx so the PSP retries")
	require.Equal(t, 1, mock.refundCalls)
	assert.Equal(t, int64(1), countRefundReviewAlerts(t, business.ID))
}

// TestUnsettleablePluginCaptureError_IncludesBreakdownMismatch (FIX B) pins the
// classification directly.
func TestUnsettleablePluginCaptureError_IncludesBreakdownMismatch(t *testing.T) {
	assert.True(t, unsettleablePluginCaptureError(errPluginPaymentBreakdownMismatch),
		"breakdown mismatch must be treated as an unsettleable capture (auto-refund)")
}
