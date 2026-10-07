package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
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

// D3 (2026-10-07): refund, dispute and reversal webhooks that arrive before
// their capture is recorded must be retried, not acknowledged and dropped.

const capturePendingSecret = "whsec_capture_pending"

type capturePendingFixture struct {
	provider   string
	businessID uint
	bill       *database.Bill
	plugin     *testPaymentPlugin
}

func setupCapturePendingFixture(t *testing.T, provider string) *capturePendingFixture {
	t.Helper()
	businessID := prepareProviderContractDatabase(t, provider, map[string]interface{}{"webhook_secret": capturePendingSecret})
	bill := newProviderContractBill(t, businessID, 10000)
	plugin := &testPaymentPlugin{name: provider, verifySignature: true}
	original, hadOriginal := plugins.GlobalRegistry.GetPlugin(provider)
	plugins.GlobalRegistry.RegisterPlugin(plugin)
	t.Cleanup(func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(original)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin(provider)
		}
	})
	return &capturePendingFixture{provider: provider, businessID: businessID, bill: bill, plugin: plugin}
}

// send delivers a provider-shaped, signed webhook naming businessID/billID.
// created is the provider's own event timestamp; zero omits it.
func (fx *capturePendingFixture) send(t *testing.T, eventID string, businessID, billID uint, created time.Time) *httptest.ResponseRecorder {
	t.Helper()
	var body map[string]interface{}
	target := "/webhooks/" + fx.provider
	switch fx.provider {
	case "stripe":
		body = map[string]interface{}{
			"id": eventID, "type": "charge.refunded",
			"data": map[string]interface{}{"object": map[string]interface{}{
				"id":       "ch_" + eventID,
				"metadata": map[string]interface{}{"bill_id": fmt.Sprint(billID), "business_id": fmt.Sprint(businessID)},
			}},
		}
		if !created.IsZero() {
			body["created"] = created.Unix()
		}
	case "paypal":
		body = map[string]interface{}{
			"id": eventID, "event_type": "PAYMENT.CAPTURE.REFUNDED",
			"resource": map[string]interface{}{
				"id":        "REFUND-" + eventID,
				"custom_id": fmt.Sprintf("bill_%d_business_%d", billID, businessID),
			},
		}
		if !created.IsZero() {
			body["create_time"] = created.UTC().Format(time.RFC3339)
		}
	case "mercadopago":
		body = map[string]interface{}{
			"id": 1, "action": "payment.updated", "type": "payment",
			"data": map[string]interface{}{"id": eventID},
		}
		if !created.IsZero() {
			body["date_created"] = created.Format("2006-01-02T15:04:05.000-07:00")
		}
		target = fmt.Sprintf("%s?business_id=%d&bill_id=%d&data.id=%s", target, businessID, billID, eventID)
	default:
		t.Fatalf("unknown provider %s", fx.provider)
	}
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, target, bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Stripe-Signature", "t=1,v1=stub")
	c.Request.Header.Set("Paypal-Transmission-Sig", "stub")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")
	c.Request.Header.Set("x-request-id", "req-"+eventID)
	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, fx.provider)
	return w
}

func (fx *capturePendingFixture) providerKey() string { return "plugin_" + fx.provider }

// webhookKey is the dedup id the handler derives for send(eventID).
func (fx *capturePendingFixture) webhookKey(eventID string) string {
	if fx.provider == "mercadopago" {
		return "payment.updated:" + eventID
	}
	return eventID
}

func (fx *capturePendingFixture) captureResponse(paymentID string) *plugins.WebhookResponse {
	return &plugins.WebhookResponse{
		Success: true, Status: "completed", PaymentID: paymentID, BillID: fx.bill.ID,
		Amount: 10000, Currency: "USD",
		Metadata: map[string]interface{}{"bill_amount_cents": int64(10000), "tip_amount_cents": int64(0)},
	}
}

func requireWebhookStatus(t *testing.T, provider, webhookID, status string) *database.WebhookEvent {
	t.Helper()
	event, err := database.GetDBWrapper().GetWebhookEvent(provider, webhookID)
	require.NoError(t, err)
	require.NotNil(t, event, "webhook row %s/%s must exist", provider, webhookID)
	require.Equal(t, status, event.Status)
	return event
}

type earlyReversalCase struct {
	name     string
	response func(paymentID string, billID uint) *plugins.WebhookResponse
	paidPost int64 // bill paid amount after the capture lands and the event is applied
}

var earlyReversalCases = []earlyReversalCase{
	{
		name: "full_refund",
		response: func(paymentID string, billID uint) *plugins.WebhookResponse {
			return &plugins.WebhookResponse{Success: true, Status: "refunded", PaymentID: paymentID, BillID: billID, Amount: 10000, Currency: "USD"}
		},
		paidPost: 0,
	},
	{
		name: "dispute_withdrawal",
		response: func(paymentID string, billID uint) *plugins.WebhookResponse {
			return &plugins.WebhookResponse{Success: true, Status: "reversed", PaymentID: paymentID, BillID: billID, Amount: 4000, Currency: "USD", DisputedCents: int64Ptr(4000)}
		},
		paidPost: 6000,
	},
	{
		name: "partial_reversal",
		response: func(paymentID string, billID uint) *plugins.WebhookResponse {
			return &plugins.WebhookResponse{Success: true, Status: "refunded", PaymentID: paymentID, BillID: billID, Amount: 3000, Currency: "USD", RefundedCumulativeCents: int64Ptr(3000)}
		},
		paidPost: 7000,
	},
	{
		name: "dispute_opened",
		response: func(paymentID string, billID uint) *plugins.WebhookResponse {
			return &plugins.WebhookResponse{Success: true, Status: "disputed", PaymentID: paymentID, BillID: billID, Amount: 10000, Currency: "USD"}
		},
		paidPost: 10000,
	},
}

// An early refund / dispute / partial reversal answers a retryable 503 and
// parks the row in retry_pending; once the capture lands, the redelivery is
// applied exactly once, and later replays are no-ops. Runs every provider
// that shares handlePaymentWebhook.
func TestPluginWebhook_EarlyReversalRetriesUntilCaptureLands(t *testing.T) {
	for _, provider := range []string{"stripe", "paypal", "mercadopago"} {
		for _, tc := range earlyReversalCases {
			t.Run(provider+"/"+tc.name, func(t *testing.T) {
				fx := setupCapturePendingFixture(t, provider)
				paymentID := "cap_" + provider + "_" + tc.name
				eventID := "evt_early_" + provider + "_" + tc.name

				fx.plugin.webhookResponse = tc.response(paymentID, fx.bill.ID)
				w := fx.send(t, eventID, fx.businessID, fx.bill.ID, time.Now())
				require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
				require.Equal(t, "300", w.Header().Get("Retry-After"))
				requireWebhookStatus(t, fx.providerKey(), fx.webhookKey(eventID), database.WebhookEventStatusRetryPending)
				requireBillPaid(t, fx.bill.ID, 0)

				// A redelivery before the capture lands is still retryable, not deduplicated.
				w = fx.send(t, eventID, fx.businessID, fx.bill.ID, time.Now())
				require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
				requireWebhookStatus(t, fx.providerKey(), fx.webhookKey(eventID), database.WebhookEventStatusRetryPending)

				// The capture lands.
				fx.plugin.webhookResponse = fx.captureResponse(paymentID)
				w = fx.send(t, "evt_capture_"+provider+"_"+tc.name, fx.businessID, fx.bill.ID, time.Now())
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				requireBillPaid(t, fx.bill.ID, 10000)

				// The provider's redelivery now applies, exactly once.
				fx.plugin.webhookResponse = tc.response(paymentID, fx.bill.ID)
				w = fx.send(t, eventID, fx.businessID, fx.bill.ID, time.Now())
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				requireWebhookStatus(t, fx.providerKey(), fx.webhookKey(eventID), "processed")
				requireBillPaid(t, fx.bill.ID, tc.paidPost)

				w = fx.send(t, eventID, fx.businessID, fx.bill.ID, time.Now())
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				requireBillPaid(t, fx.bill.ID, tc.paidPost)
				if tc.name == "full_refund" {
					requirePaymentAmount(t, paymentID, 10000, database.PaymentStatusReversed)
				}
			})
		}
	}
}

// An event older than the bound (by the provider's signed created timestamp)
// whose capture never landed is acknowledged with an operator alert instead
// of retrying. The bound sits inside every provider's redelivery window, so
// the last automatic redeliveries reach this branch.
func TestPluginWebhook_EarlyReversalStaleEventAckedWithAlert(t *testing.T) {
	require.Less(t, pluginCapturePendingMaxAge, 72*time.Hour, "the bound must sit inside the providers' ~72h redelivery window")
	for _, provider := range []string{"stripe", "paypal"} {
		t.Run(provider, func(t *testing.T) {
			fx := setupCapturePendingFixture(t, provider)
			paymentID := "cap_stale_" + provider
			eventID := "evt_stale_" + provider
			fx.plugin.webhookResponse = &plugins.WebhookResponse{Success: true, Status: "refunded", PaymentID: paymentID, BillID: fx.bill.ID, Amount: 10000, Currency: "USD"}

			// Just inside the bound the event still retries.
			w := fx.send(t, eventID+"_fresh", fx.businessID, fx.bill.ID, time.Now().Add(-pluginCapturePendingMaxAge+time.Hour))
			require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())

			require.Zero(t, countRefundReviewAlerts(t, fx.businessID))
			w = fx.send(t, eventID, fx.businessID, fx.bill.ID, time.Now().Add(-pluginCapturePendingMaxAge-time.Hour))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			requireWebhookStatus(t, fx.providerKey(), fx.webhookKey(eventID), "processed")
			require.EqualValues(t, 1, countRefundReviewAlerts(t, fx.businessID))
			requireBillPaid(t, fx.bill.ID, 0)
		})
	}
}

// MercadoPago's x-signature does not cover the body, so an old body
// date_created must not shorten the bound: the event keeps retrying until
// Payverge's own first-seen created_at is past it.
func TestPluginWebhook_MercadoPagoUnsignedDateCreatedDoesNotExpireEvent(t *testing.T) {
	fx := setupCapturePendingFixture(t, "mercadopago")
	eventID := "evt_mp_unsigned_date"
	fx.plugin.webhookResponse = &plugins.WebhookResponse{Success: true, Status: "refunded", PaymentID: "cap_mp_unsigned_date", BillID: fx.bill.ID, Amount: 10000, Currency: "USD"}

	w := fx.send(t, eventID, fx.businessID, fx.bill.ID, time.Now().Add(-30*24*time.Hour))
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	requireWebhookStatus(t, fx.providerKey(), fx.webhookKey(eventID), database.WebhookEventStatusRetryPending)
	require.Zero(t, countRefundReviewAlerts(t, fx.businessID))

	require.NoError(t, database.GetDB().Model(&database.WebhookEvent{}).
		Where("provider = ? AND webhook_id = ?", fx.providerKey(), fx.webhookKey(eventID)).
		Update("created_at", time.Now().Add(-pluginCapturePendingMaxAge-time.Hour)).Error)
	w = fx.send(t, eventID, fx.businessID, fx.bill.ID, time.Now().Add(-30*24*time.Hour))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requireWebhookStatus(t, fx.providerKey(), fx.webhookKey(eventID), "processed")
	require.EqualValues(t, 1, countRefundReviewAlerts(t, fx.businessID))
}

// Without a provider timestamp, the bound falls back to when Payverge first
// stored the event (created_at survives retry claims).
func TestPluginWebhook_EarlyReversalBoundUsesFirstSeenWhenUndated(t *testing.T) {
	fx := setupCapturePendingFixture(t, "stripe")
	fx.plugin.webhookResponse = &plugins.WebhookResponse{Success: true, Status: "refunded", PaymentID: "cap_undated", BillID: fx.bill.ID, Amount: 10000, Currency: "USD"}

	require.Equal(t, http.StatusServiceUnavailable, fx.send(t, "evt_undated", fx.businessID, fx.bill.ID, time.Time{}).Code)
	require.NoError(t, database.GetDB().Model(&database.WebhookEvent{}).
		Where("provider = ? AND webhook_id = ?", "plugin_stripe", "evt_undated").
		Update("created_at", time.Now().Add(-pluginCapturePendingMaxAge-time.Hour)).Error)

	w := fx.send(t, "evt_undated", fx.businessID, fx.bill.ID, time.Time{})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requireWebhookStatus(t, "plugin_stripe", "evt_undated", "processed")
	require.EqualValues(t, 1, countRefundReviewAlerts(t, fx.businessID))
}

// A capture Payverge refunded itself (unsettleable) never reaches the ledger;
// the provider's follow-up refund webhook is a no-op ack, not three days of
// retries.
func TestPluginWebhook_RefundOfAutoRefundedCaptureIsAcked(t *testing.T) {
	fx := setupCapturePendingFixture(t, "stripe")
	capture := fx.captureResponse("cap_autorefunded")
	require.NoError(t, refundUnsettleablePluginCapture(fx.plugin, "stripe", fx.businessID, fx.bill.ID, capture, "plugin_webhook", errors.New("bill already paid")))
	refunded, err := pluginCaptureWasAutoRefunded("stripe", "cap_autorefunded")
	require.NoError(t, err)
	require.True(t, refunded)

	fx.plugin.webhookResponse = &plugins.WebhookResponse{Success: true, Status: "refunded", PaymentID: "cap_autorefunded", BillID: fx.bill.ID, Amount: 10000, Currency: "USD"}
	w := fx.send(t, "evt_refund_of_autorefund", fx.businessID, fx.bill.ID, time.Now())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	requireWebhookStatus(t, "plugin_stripe", "evt_refund_of_autorefund", "processed")
	requireBillPaid(t, fx.bill.ID, 0)
}

// The retry gate never widens tenant reach: business A naming B's bill is
// still refused before the gate, and A's early event naming its own bill with
// B's payment id is refused once B's capture lands, leaving B's ledger intact.
func TestPluginWebhook_EarlyReversalNoCrossTenantRegression(t *testing.T) {
	fx := setupCapturePendingFixture(t, "stripe")
	var plugin database.Plugin
	require.NoError(t, database.GetDB().Where("name = ?", "stripe").First(&plugin).Error)
	victim := createTestBusinessWithName(t, "CapturePendingVictim")
	require.NoError(t, database.GetDB().Model(victim).Update("default_currency", "USD").Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: victim.ID, PluginID: plugin.ID, IsEnabled: true, Config: `{"webhook_secret":"` + capturePendingSecret + `"}`,
	}).Error)
	victimBill := newProviderContractBill(t, victim.ID, 10000)

	// A names the victim's bill: rejected by bill ownership, never parked.
	fx.plugin.webhookResponse = &plugins.WebhookResponse{Success: true, Status: "refunded", PaymentID: "cap_victim", BillID: victimBill.ID, Amount: 10000, Currency: "USD"}
	w := fx.send(t, "evt_cross_bill", fx.businessID, victimBill.ID, time.Now())
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	// A names its own bill but the victim's (not yet recorded) payment id.
	fx.plugin.webhookResponse = &plugins.WebhookResponse{Success: true, Status: "refunded", PaymentID: "cap_victim", BillID: fx.bill.ID, Amount: 10000, Currency: "USD"}
	w = fx.send(t, "evt_cross_payment", fx.businessID, fx.bill.ID, time.Now())
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())

	// The victim's capture lands on the victim's bill.
	fx.plugin.webhookResponse = &plugins.WebhookResponse{
		Success: true, Status: "completed", PaymentID: "cap_victim", BillID: victimBill.ID, Amount: 10000, Currency: "USD",
		Metadata: map[string]interface{}{"bill_amount_cents": int64(10000), "tip_amount_cents": int64(0)},
	}
	require.Equal(t, http.StatusOK, fx.send(t, "evt_victim_capture", victim.ID, victimBill.ID, time.Now()).Code)
	requireBillPaid(t, victimBill.ID, 10000)

	// A's redelivery is refused against the recorded payment's bill.
	fx.plugin.webhookResponse = &plugins.WebhookResponse{Success: true, Status: "refunded", PaymentID: "cap_victim", BillID: fx.bill.ID, Amount: 10000, Currency: "USD"}
	w = fx.send(t, "evt_cross_payment", fx.businessID, fx.bill.ID, time.Now())
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	requireBillPaid(t, victimBill.ID, 10000)
	requirePaymentAmount(t, "cap_victim", 10000, database.PaymentStatusConfirmed)
}

// retry_pending keeps the dedup table's guarantees: reprocessable, claimable
// by exactly one delivery, and a processed row is never reprocessed.
func TestWebhookEventRetryPending_ClaimIsExclusive(t *testing.T) {
	setupHandlerTestDB(t)
	db := database.GetDBWrapper()
	event := &database.WebhookEvent{Provider: "plugin_stripe", WebhookID: "evt_claim_once", EventType: "charge.refunded", Status: "processing", ReceivedAt: time.Now()}
	duplicate, err := db.CreateWebhookEventIfNotExists(event)
	require.NoError(t, err)
	require.False(t, duplicate)
	require.NoError(t, db.MarkWebhookEventRetryPending("plugin_stripe", "evt_claim_once", "capture_pending"))

	stored := requireWebhookStatus(t, "plugin_stripe", "evt_claim_once", database.WebhookEventStatusRetryPending)
	require.True(t, database.WebhookEventReprocessable(stored, time.Now()))

	claimed, err := db.ClaimWebhookEventForProcessing(stored.ID, time.Now())
	require.NoError(t, err)
	require.True(t, claimed)
	claimed, err = db.ClaimWebhookEventForProcessing(stored.ID, time.Now())
	require.NoError(t, err)
	require.False(t, claimed, "a second concurrent delivery must not also claim the row")

	require.NoError(t, db.MarkWebhookEventProcessed("plugin_stripe", "evt_claim_once"))
	stored = requireWebhookStatus(t, "plugin_stripe", "evt_claim_once", "processed")
	require.False(t, database.WebhookEventReprocessable(stored, time.Now()))
	duplicate, err = db.CreateWebhookEventIfNotExists(&database.WebhookEvent{Provider: "plugin_stripe", WebhookID: "evt_claim_once", EventType: "charge.refunded", Status: "processing"})
	require.NoError(t, err)
	require.True(t, duplicate)
}

func TestExtractPluginWebhookCreatedAt(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	got, ok := extractPluginWebhookCreatedAt("stripe", map[string]interface{}{"created": float64(at.Unix())})
	require.True(t, ok)
	require.True(t, got.Equal(at))
	got, ok = extractPluginWebhookCreatedAt("paypal", map[string]interface{}{"create_time": "2026-10-01T12:00:00Z"})
	require.True(t, ok)
	require.True(t, got.Equal(at))
	// MercadoPago's body is not covered by its signature: never trusted.
	_, ok = extractPluginWebhookCreatedAt("mercadopago", map[string]interface{}{"date_created": "2026-10-01T09:00:00.000-03:00"})
	require.False(t, ok)
	_, ok = extractPluginWebhookCreatedAt("stripe", map[string]interface{}{})
	require.False(t, ok)
	_, ok = extractPluginWebhookCreatedAt("paypal", map[string]interface{}{"create_time": "yesterday"})
	require.False(t, ok)
}
