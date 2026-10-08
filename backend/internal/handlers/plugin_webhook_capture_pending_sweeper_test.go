package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/stripe"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// Review follow-ups to D3 (2026-10-07): the bound must be reachable when the
// provider stops redelivering, real dispute payloads (which carry no bill
// reference) must be deferred too, and an unsigned MercadoPago body timestamp
// must not shorten the bound.

func backdateWebhookRow(t *testing.T, provider, webhookID string, age time.Duration) {
	t.Helper()
	require.NoError(t, database.GetDB().Model(&database.WebhookEvent{}).
		Where("provider = ? AND webhook_id = ?", provider, webhookID).
		Update("created_at", time.Now().Add(-age)).Error)
}

func countWebhookEventReviewAlerts(t *testing.T, businessID uint) int64 {
	t.Helper()
	var count int64
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).
		Where("business_id = ? AND alert_type = ? AND resource_type = ?", businessID,
			database.OperationalAlertTypePaymentRefundReview, database.OperationalAlertResourceTypeWebhookEvent).
		Count(&count).Error)
	return count
}

// A row parked in retry_pending that the provider never redelivers is expired
// by the sweeper once it is past the bound: processed, alert raised, counter
// bumped. A fresher row is left alone, and a later redelivery of the expired
// event is a deduplicated no-op that does not raise a second alert.
func TestPluginCapturePendingSweeper_ExpiresAbandonedRowWithAlert(t *testing.T) {
	for _, provider := range []string{"stripe", "paypal", "mercadopago"} {
		t.Run(provider, func(t *testing.T) {
			fx := setupCapturePendingFixture(t, provider)
			fx.plugin.webhookResponse = &plugins.WebhookResponse{Success: true, Status: "refunded", PaymentID: "cap_abandoned_" + provider, BillID: fx.bill.ID, Amount: 10000, Currency: "USD"}

			abandoned := "evt_abandoned_" + provider
			fresh := "evt_fresh_" + provider
			require.Equal(t, http.StatusServiceUnavailable, fx.send(t, abandoned, fx.businessID, fx.bill.ID, time.Now()).Code)
			require.Equal(t, http.StatusServiceUnavailable, fx.send(t, fresh, fx.businessID, fx.bill.ID, time.Now()).Code)
			parked := requireWebhookStatus(t, fx.providerKey(), fx.webhookKey(abandoned), database.WebhookEventStatusRetryPending)
			pending, ok := parsePluginCapturePendingContext(parked.Error)
			require.True(t, ok, "parked row must carry its alert context: %q", parked.Error)
			require.Equal(t, pluginCapturePendingContext{BusinessID: fx.businessID, BillID: fx.bill.ID, PaymentID: "cap_abandoned_" + provider, Status: "refunded"}, pending)

			// The provider gives up; no further delivery arrives.
			backdateWebhookRow(t, fx.providerKey(), fx.webhookKey(abandoned), pluginCapturePendingMaxAge+time.Hour)
			backdateWebhookRow(t, fx.providerKey(), fx.webhookKey(fresh), pluginCapturePendingMaxAge-time.Hour)

			expiredCounter := metrics.PaymentWebhookUnsupportedActions.WithLabelValues(provider, "capture_pending.expired")
			before := testutil.ToFloat64(expiredCounter)
			n, err := SweepExpiredPluginCapturePendingWebhooks(context.Background(), time.Now())
			require.NoError(t, err)
			require.Equal(t, 1, n)
			require.Equal(t, before+1, testutil.ToFloat64(expiredCounter))

			row := requireWebhookStatus(t, fx.providerKey(), fx.webhookKey(abandoned), "processed")
			require.True(t, strings.HasPrefix(row.Error, pluginCapturePendingExpiredPrefix), row.Error)
			requireWebhookStatus(t, fx.providerKey(), fx.webhookKey(fresh), database.WebhookEventStatusRetryPending)
			require.EqualValues(t, 1, countRefundReviewAlerts(t, fx.businessID))
			requireBillPaid(t, fx.bill.ID, 0)

			n, err = SweepExpiredPluginCapturePendingWebhooks(context.Background(), time.Now())
			require.NoError(t, err)
			require.Zero(t, n, "an expired row is never swept twice")

			w := fx.send(t, abandoned, fx.businessID, fx.bill.ID, time.Now())
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.EqualValues(t, 1, countRefundReviewAlerts(t, fx.businessID))
		})
	}
}

// The sweeper never steals a row a provider redelivery has already claimed.
func TestExpireRetryPendingWebhookEvent_LosesToInFlightClaim(t *testing.T) {
	setupHandlerTestDB(t)
	db := database.GetDBWrapper()
	event := &database.WebhookEvent{Provider: "plugin_stripe", WebhookID: "evt_sweep_race", EventType: "charge.refunded", Status: "processing", ReceivedAt: time.Now()}
	_, err := db.CreateWebhookEventIfNotExists(event)
	require.NoError(t, err)
	require.NoError(t, db.MarkWebhookEventRetryPending("plugin_stripe", "evt_sweep_race", pluginCapturePendingReasonPrefix+`{"business_id":1,"provider_payment_id":"pi_x","provider_status":"refunded"}`))
	backdateWebhookRow(t, "plugin_stripe", "evt_sweep_race", pluginCapturePendingMaxAge+time.Hour)

	claimed, err := db.ClaimWebhookEventForProcessing(event.ID, time.Now())
	require.NoError(t, err)
	require.True(t, claimed)

	n, err := SweepExpiredPluginCapturePendingWebhooks(context.Background(), time.Now())
	require.NoError(t, err)
	require.Zero(t, n)
	requireWebhookStatus(t, "plugin_stripe", "evt_sweep_race", "processing")

	expired, err := db.ExpireRetryPendingWebhookEvent(event.ID, "x")
	require.NoError(t, err)
	require.False(t, expired)
}

// stripeConnectDisputeFixture runs the REAL Stripe plugin behind a Connect
// account, so payloads are parsed exactly as Stripe sends them.
type stripeConnectDisputeFixture struct {
	businessID uint
	bill       *database.Bill
	account    string
	secret     string
}

func setupStripeConnectDisputeFixture(t *testing.T) *stripeConnectDisputeFixture {
	t.Helper()
	fx := &stripeConnectDisputeFixture{account: "acct_dispute_owner", secret: "whsec_connect_dispute"}
	fx.businessID = prepareProviderContractDatabase(t, "stripe", map[string]interface{}{
		"connection_mode": "oauth",
		"stripe_user_id":  fx.account,
		"oauth_status":    "connected",
		"live_mode":       true,
	})
	fx.bill = newProviderContractBill(t, fx.businessID, 10000)
	t.Setenv("STRIPE_CONNECT_WEBHOOK_SECRET", fx.secret)
	plugins.GlobalRegistry.RegisterPlugin(stripe.NewStripePlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })
	return fx
}

func (fx *stripeConnectDisputeFixture) send(t *testing.T, event map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	event["account"] = fx.account
	event["object"] = "event"
	if _, ok := event["created"]; !ok {
		event["created"] = time.Now().Unix()
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, fx.secret))
	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "stripe")
	return w
}

// stripeDisputeEvent is a real charge.dispute.* event: data.object is the
// Dispute, whose metadata is its own and empty, so it names no bill.
func stripeDisputeEvent(eventID, eventType, paymentIntent string, amount int64, created time.Time) map[string]interface{} {
	return map[string]interface{}{
		"id": eventID, "type": eventType, "api_version": "2024-06-20", "livemode": true,
		"created": created.Unix(),
		"data": map[string]interface{}{"object": map[string]interface{}{
			"id": "dp_" + eventID, "object": "dispute", "amount": amount, "currency": "usd",
			"charge": "ch_" + paymentIntent, "payment_intent": paymentIntent,
			"reason": "fraudulent", "status": "needs_response", "metadata": map[string]interface{}{},
			"is_charge_refundable": false,
		}},
	}
}

func stripeCheckoutCompletedEvent(eventID, paymentIntent string, billID, businessID uint, amount int64) map[string]interface{} {
	metadata := map[string]interface{}{
		"bill_id": fmt.Sprint(billID), "business_id": fmt.Sprint(businessID),
		"bill_amount_cents": fmt.Sprint(amount), "tip_amount_cents": "0",
	}
	return map[string]interface{}{
		"id": eventID, "type": "checkout.session.completed",
		"data": map[string]interface{}{"object": map[string]interface{}{
			"id": "cs_" + paymentIntent, "object": "checkout.session", "payment_intent": paymentIntent,
			"amount_total": amount, "currency": "usd", "payment_status": "paid", "metadata": metadata,
		}},
	}
}

// A real Stripe dispute withdrawal that beats its capture is retried, not
// acknowledged and dropped, and is applied once the capture lands.
func TestStripeConnectDispute_RealPayloadBeforeCaptureRetries(t *testing.T) {
	fx := setupStripeConnectDisputeFixture(t)
	const pi = "pi_dispute_early"

	for _, eventType := range []string{"charge.dispute.created", "charge.dispute.funds_withdrawn"} {
		w := fx.send(t, stripeDisputeEvent("evt_"+eventType, eventType, pi, 4000, time.Now()))
		require.Equal(t, http.StatusServiceUnavailable, w.Code, "%s: %s", eventType, w.Body.String())
		requireWebhookStatus(t, "plugin_stripe", "evt_"+eventType, database.WebhookEventStatusRetryPending)
	}

	w := fx.send(t, stripeCheckoutCompletedEvent("evt_dispute_capture", pi, fx.bill.ID, fx.businessID, 10000))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requireBillPaid(t, fx.bill.ID, 10000)

	w = fx.send(t, stripeDisputeEvent("evt_charge.dispute.funds_withdrawn", "charge.dispute.funds_withdrawn", pi, 4000, time.Now()))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requireWebhookStatus(t, "plugin_stripe", "evt_charge.dispute.funds_withdrawn", "processed")
	requireBillPaid(t, fx.bill.ID, 6000)

	w = fx.send(t, stripeDisputeEvent("evt_charge.dispute.created", "charge.dispute.created", pi, 4000, time.Now()))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requireWebhookStatus(t, "plugin_stripe", "evt_charge.dispute.created", "processed")
	require.EqualValues(t, 1, countRefundReviewAlerts(t, fx.businessID), "the dispute alert is raised against the recorded payment")
}

// A bill-less dispute past the bound is acknowledged with a business-scoped
// alert on its webhook row; the sweeper does the same for an abandoned one.
func TestStripeConnectDispute_StaleBillLessDisputeRaisesBusinessAlert(t *testing.T) {
	fx := setupStripeConnectDisputeFixture(t)

	w := fx.send(t, stripeDisputeEvent("evt_stale_dispute", "charge.dispute.funds_withdrawn", "pi_never_recorded", 4000, time.Now().Add(-pluginCapturePendingMaxAge-time.Hour)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requireWebhookStatus(t, "plugin_stripe", "evt_stale_dispute", "processed")
	require.EqualValues(t, 1, countWebhookEventReviewAlerts(t, fx.businessID))

	w = fx.send(t, stripeDisputeEvent("evt_abandoned_dispute", "charge.dispute.created", "pi_never_recorded_2", 4000, time.Now()))
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	backdateWebhookRow(t, "plugin_stripe", "evt_abandoned_dispute", pluginCapturePendingMaxAge+time.Hour)
	n, err := SweepExpiredPluginCapturePendingWebhooks(context.Background(), time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.EqualValues(t, 2, countWebhookEventReviewAlerts(t, fx.businessID))
	requireBillPaid(t, fx.bill.ID, 0)
}

// A bill-less refund is a charge Payverge did not create (Payverge charges
// carry bill_id in their metadata): still acknowledged, never parked.
func TestStripeConnectRefund_ForeignChargeStillAcked(t *testing.T) {
	fx := setupStripeConnectDisputeFixture(t)
	w := fx.send(t, map[string]interface{}{
		"id": "evt_foreign_refund", "type": "charge.refunded",
		"data": map[string]interface{}{"object": map[string]interface{}{
			"id": "ch_foreign", "object": "charge", "payment_intent": "pi_foreign", "amount": 2500,
			"amount_refunded": 2500, "currency": "usd", "metadata": map[string]interface{}{},
		}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requireWebhookStatus(t, "plugin_stripe", "evt_foreign_refund", "processed")
}

// A real PayPal CUSTOMER.DISPUTE.CREATED has no top-level custom_id: the bill
// reference is on disputed_transactions[].custom. It used to be rejected as
// an unresolved business; it now resolves and is deferred until the capture.
func TestPayPalDispute_RealPayloadResolvesBillAndRetries(t *testing.T) {
	driver := newPayPalContractWebhookDriver(t)
	bill := newProviderContractBill(t, driver.businessID, 10000)
	const captureID = "CAPTURE-DISPUTE-EARLY"

	sendDispute := func() *httptest.ResponseRecorder {
		payload, err := json.Marshal(map[string]interface{}{
			"id": "WH-DISPUTE-EARLY", "event_type": "CUSTOMER.DISPUTE.CREATED", "resource_type": "dispute",
			"create_time": time.Now().UTC().Format(time.RFC3339),
			"resource": map[string]interface{}{
				"dispute_id": "PP-D-1", "reason": "MERCHANDISE_OR_SERVICE_NOT_RECEIVED", "status": "OPEN",
				"dispute_amount": map[string]interface{}{"currency_code": "USD", "value": "100.00"},
				"disputed_transactions": []interface{}{map[string]interface{}{
					"seller_transaction_id": captureID,
					"custom":                fmt.Sprintf("bill_%d_business_%d", bill.ID, driver.businessID),
					"gross_amount":          map[string]interface{}{"currency_code": "USD", "value": "100.00"},
				}},
			},
		})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/paypal", bytes.NewReader(payload))
		for key, value := range payPalContractHeaders("contract-behavior") {
			c.Request.Header.Set(key, value)
		}
		NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "paypal")
		return w
	}

	w := sendDispute()
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	requireWebhookStatus(t, "plugin_paypal", "WH-DISPUTE-EARLY", database.WebhookEventStatusRetryPending)

	w = driver.send(t, "completed", "WH-DISPUTE-CAPTURE", captureID, driver.businessID, bill.ID, 10000, "USD", 10000)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requireBillPaid(t, bill.ID, 10000)

	w = sendDispute()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requireWebhookStatus(t, "plugin_paypal", "WH-DISPUTE-EARLY", "processed")
	require.EqualValues(t, 1, countRefundReviewAlerts(t, driver.businessID))
	requireBillPaid(t, bill.ID, 10000)
}
