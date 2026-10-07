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
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

// webhookFailMarkingFixture wires a signed Stripe webhook for business A whose
// plugin response names billID. It returns a function that sends the event.
func webhookFailMarkingFixture(t *testing.T, eventID string, billID func(a, b *database.Business) uint) func() *httptest.ResponseRecorder {
	t.Helper()
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))

	bizA := createTestBusinessWithName(t, "WebhookOwner")
	bizB := createTestBusinessWithName(t, "OtherBusiness")
	targetBillID := billID(bizA, bizB)

	stripeRow := &database.Plugin{Name: "stripe", DisplayName: "Stripe", Category: database.PluginCategoryPayment, IsActive: true}
	require.NoError(t, database.GetDB().Create(stripeRow).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: bizA.ID,
		PluginID:   stripeRow.ID,
		IsEnabled:  true,
		Config:     `{"webhook_secret":"whsec_fail_marking"}`,
	}).Error)

	testPlugin := &testPaymentPlugin{
		name:            "stripe",
		verifySignature: true,
		webhookResponse: &plugins.WebhookResponse{
			Success:   true,
			Status:    "completed",
			PaymentID: "cs_fail_marking",
			BillID:    targetBillID,
			Amount:    1000,
			Currency:  "usd",
		},
	}
	plugins.GlobalRegistry.RegisterPlugin(testPlugin)
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("stripe") })

	payload, err := json.Marshal(map[string]interface{}{
		"id":   eventID,
		"type": "checkout.session.completed",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":           "cs_fail_marking",
				"amount_total": float64(1000),
				"currency":     "usd",
				"metadata": map[string]interface{}{
					"bill_id":     fmt.Sprintf("%d", targetBillID),
					"business_id": fmt.Sprintf("%d", bizA.ID),
				},
			},
		},
	})
	require.NoError(t, err)

	return func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
		c.Request.Header.Set("Stripe-Signature", signStripePayload(t, payload, "whsec_fail_marking"))
		(&PluginHandlers{}).handlePaymentWebhook(c, "stripe")
		return w
	}
}

// OBS-001: a 403 after the dedup claim used to leave the row "processing", so
// the provider's redelivery inside five minutes was acknowledged as a
// duplicate and the event was lost. The row must end up failed.
func TestHandlePaymentWebhook_ForbiddenExitMarksEventFailed(t *testing.T) {
	const eventID = "evt_fail_marking_forbidden"
	send := webhookFailMarkingFixture(t, eventID, func(_, b *database.Business) uint {
		return createTestBillForBiz(t, b.ID).ID
	})

	w := send()
	require.Equal(t, http.StatusForbidden, w.Code)

	event, err := database.GetDBWrapper().GetWebhookEvent("plugin_stripe", eventID)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, "failed", event.Status)
	require.True(t, database.WebhookEventReprocessable(event, time.Now()))

	// The redelivery is reprocessed (and rejected again), not acked as a duplicate.
	w = send()
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandlePaymentWebhook_InFlightDuplicateGetsConflict(t *testing.T) {
	const eventID = "evt_fail_marking_in_flight"
	send := webhookFailMarkingFixture(t, eventID, func(a, _ *database.Business) uint {
		return createTestBillForBiz(t, a.ID).ID
	})

	duplicate, err := database.GetDBWrapper().CreateWebhookEventIfNotExists(&database.WebhookEvent{
		Provider:   "plugin_stripe",
		WebhookID:  eventID,
		EventType:  "checkout.session.completed",
		Status:     "processing",
		ReceivedAt: time.Now(),
	})
	require.NoError(t, err)
	require.False(t, duplicate)

	w := send()
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	event, err := database.GetDBWrapper().GetWebhookEvent("plugin_stripe", eventID)
	require.NoError(t, err)
	require.Equal(t, "processing", event.Status, "a request that does not own the row must not mark it")
}

func TestHandlePaymentWebhook_ProcessedDuplicateIsAcknowledged(t *testing.T) {
	const eventID = "evt_fail_marking_processed"
	send := webhookFailMarkingFixture(t, eventID, func(a, _ *database.Business) uint {
		return createTestBillForBiz(t, a.ID).ID
	})

	_, err := database.GetDBWrapper().CreateWebhookEventIfNotExists(&database.WebhookEvent{
		Provider:   "plugin_stripe",
		WebhookID:  eventID,
		EventType:  "checkout.session.completed",
		Status:     "processing",
		ReceivedAt: time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, database.GetDBWrapper().MarkWebhookEventProcessed("plugin_stripe", eventID))

	w := send()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "already processed")
}

// A panic inside webhook processing runs the deferred guard before gin's
// Recovery writes the 500, while the writer still reports 200. The owned row
// must still end up failed (not stuck "processing" for five minutes) and the
// panic must still reach Recovery.
func TestHandlePaymentWebhook_PanicMarksEventFailedAndRepanics(t *testing.T) {
	const eventID = "evt_fail_marking_panic"
	send := webhookFailMarkingFixture(t, eventID, func(a, _ *database.Business) uint {
		return createTestBillForBiz(t, a.ID).ID
	})
	registered, ok := plugins.GlobalRegistry.GetPlugin("stripe")
	require.True(t, ok)
	stub, ok := registered.(*testPaymentPlugin)
	require.True(t, ok)
	stub.webhookPanic = "provider SDK exploded"

	require.PanicsWithValue(t, "provider SDK exploded", func() { send() })

	event, err := database.GetDBWrapper().GetWebhookEvent("plugin_stripe", eventID)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, "failed", event.Status)
	require.True(t, database.WebhookEventReprocessable(event, time.Now()))
}
