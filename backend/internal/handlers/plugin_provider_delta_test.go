package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func int64Ptr(v int64) *int64 { return &v }

type providerDeltaFixture struct {
	bill   *database.Bill
	plugin *testPaymentPlugin
	send   func(eventID string) *httptest.ResponseRecorder
}

// setupStripeDeltaFixture settles a $100 Stripe capture on a fresh bill
// through the webhook handler, then lets the test swap the plugin response.
func setupStripeDeltaFixture(t *testing.T, paymentID string) providerDeltaFixture {
	t.Helper()
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.AlternativePayment{}))
	biz := createTestBusinessWithName(t, "DeltaOwner")
	require.NoError(t, database.GetDB().Model(biz).Update("default_currency", "USD").Error)
	bill := newProviderContractBill(t, biz.ID, 10000)

	row := &database.Plugin{Name: "stripe", DisplayName: "Stripe", Category: database.PluginCategoryPayment, IsActive: true}
	require.NoError(t, database.GetDB().Create(row).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: biz.ID, PluginID: row.ID, IsEnabled: true, Config: `{"webhook_secret":"whsec_delta"}`,
	}).Error)

	plugin := &testPaymentPlugin{
		name:            "stripe",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success: true, Status: "completed", PaymentID: paymentID, BillID: bill.ID,
			Amount: 10000, Currency: "USD",
			Metadata: map[string]interface{}{"bill_amount_cents": int64(10000), "tip_amount_cents": int64(0)},
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(plugin)
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	send := func(eventID string) *httptest.ResponseRecorder {
		payload, err := json.Marshal(map[string]interface{}{
			"id": eventID, "type": "charge.refunded",
			"data": map[string]interface{}{"object": map[string]interface{}{
				"id": "ch_" + paymentID, "payment_intent": paymentID,
				"metadata": map[string]interface{}{"bill_id": fmt.Sprint(bill.ID), "business_id": fmt.Sprint(biz.ID)},
			}},
		})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
		c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_delta"))
		NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")
		return w
	}

	w := send("evt_delta_settle_" + paymentID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requireBillPaid(t, bill.ID, 10000)
	return providerDeltaFixture{bill: bill, plugin: plugin, send: send}
}

func requireBillPaid(t *testing.T, billID uint, paid int64) {
	t.Helper()
	var bill database.Bill
	require.NoError(t, database.GetDB().First(&bill, billID).Error)
	require.EqualValues(t, paid, bill.PaidAmount)
}

func requirePaymentAmount(t *testing.T, paymentID string, amount int64, status database.PaymentStatus) {
	t.Helper()
	var payment database.Payment
	require.NoError(t, database.GetDB().Where("tx_hash = ?", "plugin_"+paymentID).First(&payment).Error)
	require.EqualValues(t, amount, payment.Amount)
	require.Equal(t, status, payment.Status)
}

// stripe-cumulative-refund through the webhook handler: 30 then cumulative 50
// reverses 30 then 20; a fresh event replaying cumulative 50 is a no-op.
func TestPluginWebhook_StripeCumulativeRefundAppliesDeltas(t *testing.T) {
	fx := setupStripeDeltaFixture(t, "pi_delta_refund")
	refund := func(cumulative int64) *plugins.WebhookResponse {
		return &plugins.WebhookResponse{
			Success: true, Status: "refunded", PaymentID: "pi_delta_refund", BillID: fx.bill.ID,
			Amount: cumulative, Currency: "USD", RefundedCumulativeCents: int64Ptr(cumulative),
		}
	}

	fx.plugin.webhookResponse = refund(3000)
	require.Equal(t, http.StatusOK, fx.send("evt_delta_refund_30").Code)
	requireBillPaid(t, fx.bill.ID, 7000)

	fx.plugin.webhookResponse = refund(5000)
	require.Equal(t, http.StatusOK, fx.send("evt_delta_refund_50").Code)
	requireBillPaid(t, fx.bill.ID, 5000)
	requirePaymentAmount(t, "pi_delta_refund", 5000, database.PaymentStatusConfirmed)

	require.Equal(t, http.StatusOK, fx.send("evt_delta_refund_50_again").Code)
	requireBillPaid(t, fx.bill.ID, 5000)
	requirePaymentAmount(t, "pi_delta_refund", 5000, database.PaymentStatusConfirmed)
}

// stripe-dispute-reversal through the handler: a 40-of-100 withdrawal
// reverses 40 (not the whole payment) and funds_reinstated restores it.
func TestPluginWebhook_StripePartialDisputeReversesAndReinstates(t *testing.T) {
	fx := setupStripeDeltaFixture(t, "pi_delta_dispute")

	fx.plugin.webhookResponse = &plugins.WebhookResponse{
		Success: true, Status: "reversed", PaymentID: "pi_delta_dispute", BillID: fx.bill.ID,
		Amount: 4000, Currency: "USD", DisputedCents: int64Ptr(4000),
	}
	require.Equal(t, http.StatusOK, fx.send("evt_delta_dispute_withdrawn").Code)
	requireBillPaid(t, fx.bill.ID, 6000)
	requirePaymentAmount(t, "pi_delta_dispute", 6000, database.PaymentStatusConfirmed)

	// charge.dispute.closed(lost) repeats the same withdrawal: no-op.
	require.Equal(t, http.StatusOK, fx.send("evt_delta_dispute_lost_replay").Code)
	requireBillPaid(t, fx.bill.ID, 6000)

	fx.plugin.webhookResponse = &plugins.WebhookResponse{
		Success: true, Status: "dispute_reinstated", PaymentID: "pi_delta_dispute", BillID: fx.bill.ID,
		Currency: "USD", DisputedCents: int64Ptr(0),
	}
	require.Equal(t, http.StatusOK, fx.send("evt_delta_dispute_reinstated").Code)
	requireBillPaid(t, fx.bill.ID, 10000)
	requirePaymentAmount(t, "pi_delta_dispute", 10000, database.PaymentStatusConfirmed)
}

// mp-partial-refund-dropped: MercadoPago keeps a partially refunded payment
// "approved". 30-of-100 must reach the ledger, exactly once across repeated
// notifications.
func TestPluginWebhook_MercadoPagoPartialRefundReversedOnce(t *testing.T) {
	refundedCents := int64(0)
	var businessID uint
	var billID uint
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/payments/") {
			http.Error(w, "unexpected", http.StatusServiceUnavailable)
			return
		}
		detail := "accredited"
		if refundedCents > 0 {
			detail = "partially_refunded"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": 1, "status": "approved", "status_detail": detail, "currency_id": "USD",
			"transaction_amount":          100.0,
			"transaction_amount_refunded": float64(refundedCents) / 100,
			"external_reference":          fmt.Sprintf("bill_%d_business_%d", billID, businessID),
			"metadata":                    map[string]interface{}{"bill_amount_cents": 10000, "tip_amount_cents": 0},
		})
	}))
	t.Cleanup(api.Close)
	businessID = prepareProviderContractDatabase(t, "mercadopago", map[string]interface{}{
		"access_token":   "APP_USR-delta-access-token-1234567890",
		"public_key":     "APP_USR-delta-public-key-1234567890",
		"webhook_secret": "mp_delta", "environment": "sandbox",
		"base_url": "https://api.payverge.test", "api_base_url": api.URL,
	})
	billID = newProviderContractBill(t, businessID, 10000).ID
	plugins.GlobalRegistry.RegisterPlugin(mercadopago.NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("mercadopago") })

	send := func(eventID string) {
		t.Helper()
		w := sendMercadoPagoContractWebhook(t, "completed", eventID, "777001", businessID, billID, "mp_delta")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}

	send("mp_delta_approved")
	requireBillPaid(t, billID, 10000)

	refundedCents = 3000
	send("mp_delta_partial_refund")
	requireBillPaid(t, billID, 7000)
	requirePaymentAmount(t, "777001", 7000, database.PaymentStatusConfirmed)

	send("mp_delta_partial_refund_followup")
	requireBillPaid(t, billID, 7000)
	requirePaymentAmount(t, "777001", 7000, database.PaymentStatusConfirmed)
}

// Business A signs reversal events with its own secret and its own bill_id but
// names business B's provider payment. The ledger is keyed on the payment id, so
// without the recorded-bill check A could reverse B's money.
func TestPluginWebhook_CrossTenantReversalRejected(t *testing.T) {
	fx := setupStripeDeltaFixture(t, "pi_victim_b")

	attacker := createTestBusinessWithName(t, "AttackerA")
	require.NoError(t, database.GetDB().Model(attacker).Update("default_currency", "USD").Error)
	attackerBill := newProviderContractBill(t, attacker.ID, 10000)
	var row database.Plugin
	require.NoError(t, database.GetDB().Where("name = ?", "stripe").First(&row).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: attacker.ID, PluginID: row.ID, IsEnabled: true, Config: `{"webhook_secret":"whsec_attacker"}`,
	}).Error)

	sendAsAttacker := func(eventID string) *httptest.ResponseRecorder {
		payload, err := json.Marshal(map[string]interface{}{
			"id": eventID, "type": "charge.refunded",
			"data": map[string]interface{}{"object": map[string]interface{}{
				"id": "ch_pi_victim_b", "payment_intent": "pi_victim_b",
				"metadata": map[string]interface{}{"bill_id": fmt.Sprint(attackerBill.ID), "business_id": fmt.Sprint(attacker.ID)},
			}},
		})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
		c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_attacker"))
		NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")
		return w
	}

	responses := map[string]*plugins.WebhookResponse{
		"full refund": {Success: true, Status: "refunded", PaymentID: "pi_victim_b", BillID: attackerBill.ID, Amount: 10000, Currency: "USD"},
		"cumulative":  {Success: true, Status: "refunded", PaymentID: "pi_victim_b", BillID: attackerBill.ID, Amount: 5000, Currency: "USD", RefundedCumulativeCents: int64Ptr(5000)},
		"dispute":     {Success: true, Status: "reversed", PaymentID: "pi_victim_b", BillID: attackerBill.ID, Amount: 4000, Currency: "USD", DisputedCents: int64Ptr(4000)},
	}
	i := 0
	for name, resp := range responses {
		i++
		fx.plugin.webhookResponse = resp
		w := sendAsAttacker(fmt.Sprintf("evt_cross_tenant_%d", i))
		require.Equal(t, http.StatusForbidden, w.Code, "%s: %s", name, w.Body.String())
		requireBillPaid(t, fx.bill.ID, 10000)
		requirePaymentAmount(t, "pi_victim_b", 10000, database.PaymentStatusConfirmed)
		requireBillPaid(t, attackerBill.ID, 0)
	}
}

// A provider-signed reversal may only touch a payment that provider recorded:
// a Stripe-signed event naming a PayPal-tendered payment on the same bill is
// rejected, and a failed recorded-payment lookup fails closed instead of
// falling through to a tx_hash-keyed reversal that was never ownership-checked.
func TestPluginWebhook_ReversalBindsProviderAndFailsClosedOnLookupError(t *testing.T) {
	fx := setupStripeDeltaFixture(t, "pi_bind_provider")
	reversal := &plugins.WebhookResponse{Success: true, Status: "refunded", PaymentID: "pi_bind_provider", BillID: fx.bill.ID, Amount: 10000, Currency: "USD"}

	require.NoError(t, database.GetDB().Model(&database.Payment{}).
		Where("tx_hash = ?", "plugin_pi_bind_provider").Update("payment_method", "paypal").Error)
	fx.plugin.webhookResponse = reversal
	w := fx.send("evt_bind_provider")
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	requireBillPaid(t, fx.bill.ID, 10000)
	requirePaymentAmount(t, "pi_bind_provider", 10000, database.PaymentStatusConfirmed)

	require.NoError(t, database.GetDB().Model(&database.Payment{}).
		Where("tx_hash = ?", "plugin_pi_bind_provider").Update("payment_method", "stripe").Error)
	cbName := "test:fail_payment_lookup"
	require.NoError(t, database.GetDB().Callback().Query().Before("gorm:query").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement.Table == "payments" && strings.Contains(fmt.Sprint(tx.Statement.Clauses["WHERE"].Expression), "tx_hash") &&
			tx.Statement.Clauses["FOR"].Expression == nil {
			_ = tx.AddError(errors.New("simulated lookup failure"))
		}
	}))
	w = fx.send("evt_lookup_error")
	require.NoError(t, database.GetDB().Callback().Query().Remove(cbName))
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	requireBillPaid(t, fx.bill.ID, 10000)
	requirePaymentAmount(t, "pi_bind_provider", 10000, database.PaymentStatusConfirmed)
}
