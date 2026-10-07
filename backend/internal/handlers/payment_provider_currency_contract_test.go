package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

func TestHandlePaymentWebhookRejectsWrongAuthoritativeCurrency(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	enableTestMercadoPagoPlugin(t, business.ID)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-contract-currency-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		Subtotal:    2000,
		TotalAmount: 2000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	pluginName := "mercadopago"
	plugin := &testPaymentPlugin{
		name:            pluginName,
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success: true, Status: "completed", BillID: bill.ID,
			PaymentID: "wrong-currency", Amount: 2000, Currency: "EUR",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(plugin)
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin(pluginName) })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/?bill_id=%d&business_id=%d", bill.ID, business.ID), bytes.NewBufferString(
		`{"id":"currency-event","action":"payment.updated","data":{"id":"wrong-currency"}}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")
	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, pluginName)

	require.Equal(t, http.StatusOK, w.Code, "a successfully auto-refunded wrong-currency capture must be acknowledged")
	require.Equal(t, 1, plugin.refundCalls)
	require.Equal(t, "wrong-currency", plugin.lastRefundPaymentID)
	refreshed, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	require.Zero(t, refreshed.PaidAmount)
	_, err = database.GetPaymentByTxHash("plugin_wrong-currency")
	require.ErrorIs(t, err, database.ErrPaymentNotFound)
}

func TestHandlePaymentWebhookRetriesWrongCurrencyWhenAutoRefundFails(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "USD").Error)
	enableTestMercadoPagoPlugin(t, business.ID)
	bill := &database.Bill{
		BusinessID: business.ID, BillNumber: fmt.Sprintf("B-contract-currency-refund-failure-%d", time.Now().UnixNano()),
		Status: database.BillStatusOpen, Items: "[]", TotalAmount: 2000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	plugin := &testPaymentPlugin{
		name: "mercadopago", verifySignature: true, refundErr: fmt.Errorf("provider refund unavailable"),
		webhookResponse: &plugins.WebhookResponse{
			Success: true, Status: "completed", BillID: bill.ID,
			PaymentID: "wrong-currency-refund-failure", Amount: 2000, Currency: "EUR",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(plugin)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/?bill_id=%d&business_id=%d", bill.ID, business.ID), bytes.NewBufferString(
		`{"id":"currency-refund-failure-event","action":"payment.updated","data":{"id":"wrong-currency-refund-failure"}}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")
	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")

	require.Equal(t, http.StatusConflict, w.Code)
	require.Equal(t, 1, plugin.refundCalls)
	refreshed, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	require.Zero(t, refreshed.PaidAmount)
}

func TestPluginWebhookSettlementBreakdownRejectsLocalAmountMismatch(t *testing.T) {
	_, _, err := pluginWebhookSettlementBreakdownCents(&plugins.WebhookResponse{Amount: 900}, 1000, 0, true)
	require.ErrorIs(t, err, errPluginPaymentBreakdownMismatch)
	require.True(t, unsettleablePluginCaptureError(errPluginPaymentCurrencyMismatch))
}

func TestHandlePaymentWebhookRejectsBillReferenceDroppedCrossBusinessDispute(t *testing.T) {
	setupHandlerTestDB(t)
	attacker := createTestBusiness(t)
	victim := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, attacker.ID)
	victimBill := &database.Bill{
		BusinessID: victim.ID, BillNumber: fmt.Sprintf("B-dispute-victim-%d", time.Now().UnixNano()),
		Status: database.BillStatusPaid, Items: "[]", TotalAmount: 2000, PaidAmount: 2000,
	}
	require.NoError(t, database.GetDB().Create(victimBill).Error)
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID: victimBill.ID, PayerAddr: "plugin", Amount: 2000,
		TxHash: "plugin_cross-business-dispute", Status: database.PaymentStatusConfirmed, PaymentMethod: "mercadopago",
	}).Error)

	plugin := &testPaymentPlugin{
		name: "mercadopago", verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success: true, Status: "disputed", PaymentID: "cross-business-dispute",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(plugin)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/?business_id=%d&data.id=cross-business-dispute", attacker.ID), bytes.NewBufferString(
		`{"id":"cross-business-dispute-event","action":"payment.updated","data":{"id":"cross-business-dispute"}}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")
	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")

	require.Equal(t, http.StatusForbidden, w.Code)
	var alerts int64
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("business_id = ? AND alert_type = ?", victim.ID, database.OperationalAlertTypePaymentRefundReview).
		Count(&alerts).Error)
	require.Zero(t, alerts)
}
