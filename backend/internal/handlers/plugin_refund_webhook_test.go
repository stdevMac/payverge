package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

// settleBillWithPluginPayment applies a confirmed plugin payment to a bill so
// that a subsequent refund webhook has something to (potentially) reverse.
// Returns the bill (refreshed) and the txHash of the recorded payment.
func settleBillWithPluginPayment(t *testing.T, bill *database.Bill, paymentID string, amountCents int64) (*database.Bill, string) {
	t.Helper()
	txHash := "plugin_" + paymentID
	updated, applied, err := database.ApplyConfirmedPayment(database.ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "test-payer",
		Amount:        amountCents,
		TxHash:        txHash,
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "plugin",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	return updated, txHash
}

// driveRefundWebhook invokes handlePaymentWebhook for a PayPal-shaped refund
// payload. refundedCents is what the provider reports as refunded (in
// WebhookResponse.Amount). When negative, the webhook response leaves Amount
// at 0 (unknown amount). billID is the local bill the refunded capture settled
// (the real PayPal refund webhook carries this via the resource custom_id; the
// test double sets it directly on the response).
func driveRefundWebhook(t *testing.T, billID uint, paymentID string, refundedCents int64) *httptest.ResponseRecorder {
	t.Helper()

	respAmount := refundedCents
	if refundedCents < 0 {
		respAmount = 0
	}
	testPlugin := &testPaymentPlugin{
		name:            "paypal",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "refunded",
			PaymentID: paymentID,
			BillID:    billID,
			Amount:    respAmount,
			Currency:  "USD",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(testPlugin)
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("paypal") })

	t.Setenv("PAYPAL_WEBHOOK_SECRET", "paypal_test_secret")

	payloadData := map[string]interface{}{
		"event_type": "PAYMENT.CAPTURE.REFUNDED",
		"id":         "WH-" + paymentID,
		"resource": map[string]interface{}{
			"id": "REFUND-" + paymentID,
		},
	}
	payloadBytes, err := json.Marshal(payloadData)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/paypal", bytes.NewReader(payloadBytes))
	c.Request.Header.Set("Paypal-Transmission-Sig", "stub-signature")

	ph := &PluginHandlers{}
	ph.handlePaymentWebhook(c, "paypal")
	return w
}

func countRefundReviewAlerts(t *testing.T, businessID uint) int64 {
	t.Helper()
	var count int64
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("business_id = ? AND alert_type = ?", businessID, database.OperationalAlertTypePaymentRefundReview).
		Count(&count).Error)
	return count
}

// TestHandlePaymentWebhook_PartialRefund_AppliesLedger asserts a strict partial
// provider refund decrements bill paid amount and shrinks the payment residual
// without full-reversing the capture. A review alert is still raised for fiscal
// follow-up. The webhook is acked 200 so the PSP does not retry.
func TestHandlePaymentWebhook_PartialRefund_AppliesLedger(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	biz := createTestBusiness(t)
	bill := createTestBillForBiz(t, biz.ID) // TotalAmount 1000
	settledBill, txHash := settleBillWithPluginPayment(t, bill, "PAYID_PARTIAL", 1000)
	require.Equal(t, database.BillStatusPaid, settledBill.Status)

	// Merchant refunds only $3 (300 cents) of the $10 (1000 cents) capture.
	w := driveRefundWebhook(t, bill.ID, "PAYID_PARTIAL", 300)

	assert.Equal(t, http.StatusOK, w.Code, "partial refund webhook must be acked 200, not retried")

	refreshed, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, database.BillStatusPartial, refreshed.Status, "partial refund must reopen residual balance")
	assert.Equal(t, int64(700), refreshed.PaidAmount, "partial refund must decrement paid amount by refunded cents")

	payment, err := database.GetPaymentByTxHash(txHash)
	require.NoError(t, err)
	require.NotNil(t, payment)
	assert.Equal(t, database.PaymentStatusConfirmed, payment.Status, "residual payment stays confirmed")
	assert.Equal(t, int64(700), payment.Amount, "payment residual must shrink by refunded cents")

	assert.Equal(t, int64(1), countRefundReviewAlerts(t, biz.ID),
		"partial refund must create a payment_refund_review operational alert")
}

// TestHandlePaymentWebhook_FullRefund_StillReverses asserts the existing full
// reversal still happens when the refunded amount covers the whole payment.
func TestHandlePaymentWebhook_FullRefund_StillReverses(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	biz := createTestBusiness(t)
	bill := createTestBillForBiz(t, biz.ID) // TotalAmount 1000
	settledBill, txHash := settleBillWithPluginPayment(t, bill, "PAYID_FULL", 1000)
	require.Equal(t, database.BillStatusPaid, settledBill.Status)

	// Merchant refunds the full $10 (1000 cents).
	w := driveRefundWebhook(t, bill.ID, "PAYID_FULL", 1000)

	assert.Equal(t, http.StatusOK, w.Code)

	refreshed, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, database.BillStatusOpen, refreshed.Status, "full refund must reopen the bill")
	assert.Equal(t, int64(0), refreshed.PaidAmount, "full refund must zero the paid amount")

	payment, err := database.GetPaymentByTxHash(txHash)
	require.NoError(t, err)
	require.NotNil(t, payment)
	assert.Equal(t, database.PaymentStatusReversed, payment.Status, "full refund must reverse the payment")

	assert.Equal(t, int64(0), countRefundReviewAlerts(t, biz.ID),
		"full refund must not create a refund-review alert")
}

// TestHandlePaymentWebhook_UnknownRefundAmount_FullyReverses asserts that when
// the provider does not report a refund amount (Amount == 0) we default to the
// existing safe full reversal rather than silently skipping.
func TestHandlePaymentWebhook_UnknownRefundAmount_FullyReverses(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	biz := createTestBusiness(t)
	bill := createTestBillForBiz(t, biz.ID)
	settledBill, txHash := settleBillWithPluginPayment(t, bill, "PAYID_UNKNOWN", 1000)
	require.Equal(t, database.BillStatusPaid, settledBill.Status)

	// refundedCents < 0 => response.Amount stays 0 (unknown).
	w := driveRefundWebhook(t, bill.ID, "PAYID_UNKNOWN", -1)

	assert.Equal(t, http.StatusOK, w.Code)

	refreshed, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, database.BillStatusOpen, refreshed.Status, "unknown refund amount must default to full reversal")
	assert.Equal(t, int64(0), refreshed.PaidAmount)

	payment, err := database.GetPaymentByTxHash(txHash)
	require.NoError(t, err)
	require.NotNil(t, payment)
	assert.Equal(t, database.PaymentStatusReversed, payment.Status)

	assert.Equal(t, int64(0), countRefundReviewAlerts(t, biz.ID))
}

// TestExtractPluginWebhookID_MercadoPagoUsesDataID asserts the dedup key for
// MercadoPago is action-scoped on the stable payment id (payload.data.id), not
// the per-delivery top-level notification id.
func TestExtractPluginWebhookID_MercadoPagoUsesDataID(t *testing.T) {
	payload := map[string]interface{}{
		"id":          "112233", // per-delivery notification id (varies on redelivery)
		"resource_id": "445566", // also delivery-scoped
		"action":      "payment.updated",
		"data": map[string]interface{}{
			"id": "PAYMENT-998877", // stable payment id
		},
	}

	got := extractPluginWebhookID("mercadopago", payload)
	assert.Equal(t, "payment.updated:PAYMENT-998877", got,
		"MercadoPago dedup key must be action-scoped on payload.data.id, not the per-delivery notification id")
}
