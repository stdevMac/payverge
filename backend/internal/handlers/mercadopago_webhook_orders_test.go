package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// counterValueForLabels returns the current counter value for a metric family
// with exact label match, or 0 if the series is not yet present.
func counterValueForLabels(t *testing.T, name string, want map[string]string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.Metric {
			labels := map[string]string{}
			for _, l := range m.Label {
				labels[l.GetName()] = l.GetValue()
			}
			match := true
			for k, v := range want {
				if labels[k] != v {
					match = false
					break
				}
			}
			if match && m.Counter != nil {
				return m.Counter.GetValue()
			}
		}
	}
	_ = dto.Metric{} // keep import used when counter series missing
	return 0
}

// R4: invalid signature on orders topic must return 401 before the
// unresolved-tracker 500 path. A refactor that verifies after ACK would
// invert this ordering and accept forged order.* payloads.
func TestHandleMercadoPagoWebhook_OrdersTopicInvalidSignatureReturns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupHandlerTestDB(t)

	t.Setenv("MERCADOPAGO_WEBHOOK_SECRET", "mp-env-webhook-secret-for-orders-401")
	t.Setenv("MERCADOPAGO_WEBHOOK_SECRET_PREVIOUS", "")

	plugin := mercadopago.NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))
	original, hadOriginal := plugins.GlobalRegistry.GetPlugin("mercadopago")
	plugins.GlobalRegistry.RegisterPlugin(plugin)
	t.Cleanup(func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(original)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin("mercadopago")
		}
	})

	// order.* with no business_id and no local tracker — signature must still
	// be checked first (invalid → 401, not 500 tracker-not-ready).
	payload := []byte(`{"action":"order.created","data":{"id":"ORD01NOLOCALTRACKER"},"type":"order"}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/mercadopago?data.id=ORD01NOLOCALTRACKER", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=9999999999999,v1=deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	c.Request.Header.Set("x-request-id", "req-orders-401")

	NewPluginHandlers(services.NewPluginService(database.GetDBWrapper()), nil).HandleMercadoPagoWebhook(c)

	require.Equal(t, http.StatusUnauthorized, w.Code, "invalid orders signature must be 401 before tracker 500; body=%s", w.Body.String())
}

// P5: untracked order.created/updated ACK 200; untracked order.processed → 500.
func TestHandleMercadoPagoWebhook_UntrackedOrderCreatedACKs200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupHandlerTestDB(t)
	t.Setenv("MERCADOPAGO_WEBHOOK_SECRET", "mp-env-webhook-secret-for-orders-ack")
	t.Setenv("MERCADOPAGO_WEBHOOK_SECRET_PREVIOUS", "")

	original, hadOriginal := plugins.GlobalRegistry.GetPlugin("mercadopago")
	plugins.GlobalRegistry.RegisterPlugin(&testPaymentPlugin{name: "mercadopago", verifySignature: true})
	t.Cleanup(func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(original)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin("mercadopago")
		}
	})

	for _, action := range []string{"order.created", "order.updated"} {
		t.Run(action, func(t *testing.T) {
			payload := []byte(`{"action":"` + action + `","data":{"id":"ORD01UNTRACKEDINFO"},"type":"order"}`)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/?data.id=ORD01UNTRACKEDINFO", bytes.NewReader(payload))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("x-signature", "ts=1,v1=stub")
			NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")
			require.Equal(t, http.StatusOK, w.Code, "informational %s must ACK 200; body=%s", action, w.Body.String())
		})
	}
}

func TestHandleMercadoPagoWebhook_UntrackedOrderProcessedReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupHandlerTestDB(t)
	metrics.Init()
	t.Setenv("MERCADOPAGO_WEBHOOK_SECRET", "mp-env-webhook-secret-for-orders-500")

	original, hadOriginal := plugins.GlobalRegistry.GetPlugin("mercadopago")
	plugins.GlobalRegistry.RegisterPlugin(&testPaymentPlugin{name: "mercadopago", verifySignature: true})
	t.Cleanup(func() {
		if hadOriginal {
			plugins.GlobalRegistry.RegisterPlugin(original)
		} else {
			plugins.GlobalRegistry.UnregisterPlugin("mercadopago")
		}
	})

	before := counterValueForLabels(t, "payverge_payment_webhook_unsupported_actions_total",
		map[string]string{"plugin": "mercadopago", "action": "order.untracked_settle_retry"})

	payload := []byte(`{"action":"order.processed","data":{"id":"ORD01UNTRACKEDSETTLE"},"type":"order"}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/?data.id=ORD01UNTRACKEDSETTLE", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=stub")
	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")
	require.Equal(t, http.StatusInternalServerError, w.Code, "settle-class untracked must 500; body=%s", w.Body.String())

	after := counterValueForLabels(t, "payverge_payment_webhook_unsupported_actions_total",
		map[string]string{"plugin": "mercadopago", "action": "order.untracked_settle_retry"})
	require.GreaterOrEqual(t, after, before+1,
		"settle-class untracked must increment order.untracked_settle_retry (not generic error-rate)")
}

func TestMercadoPagoOrderActionNeedsTrackerRetry(t *testing.T) {
	require.False(t, mercadoPagoOrderActionNeedsTrackerRetry("order.created"))
	require.False(t, mercadoPagoOrderActionNeedsTrackerRetry("order.updated"))
	require.True(t, mercadoPagoOrderActionNeedsTrackerRetry("order.processed"))
	require.True(t, mercadoPagoOrderActionNeedsTrackerRetry("order.refunded"))
	require.True(t, mercadoPagoOrderActionNeedsTrackerRetry("order.canceled"))
}
