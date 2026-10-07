package paypal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// TestHandleWebhookOrderApprovedAlreadyCaptured locks P1: a CHECKOUT.ORDER.APPROVED
// webhook for an order already captured (return-capture path or webhook retry)
// must be a no-op success, not a permanent 500 retry loop. PayPal answers the
// duplicate capture with 422 ORDER_ALREADY_CAPTURED; the plugin resolves the
// existing completed order instead of erroring.
func TestHandleWebhookOrderApprovedAlreadyCaptured(t *testing.T) {
	var capturePosts, orderGets int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"access-token"}`))
		case r.URL.Path == "/v2/checkout/orders/ORDER-1/capture" && r.Method == http.MethodPost:
			capturePosts++
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"name":"UNPROCESSABLE_ENTITY","details":[{"issue":"ORDER_ALREADY_CAPTURED"}]}`))
		case r.URL.Path == "/v2/checkout/orders/ORDER-1" && r.Method == http.MethodGet:
			orderGets++
			_, _ = w.Write([]byte(`{"id":"ORDER-1","status":"COMPLETED","purchase_units":[{"custom_id":"bill_99_business_42","payments":{"captures":[{"id":"CAPTURE-1","status":"COMPLETED","amount":{"value":"28.00","currency_code":"USD"},"custom_id":"bill_99_business_42"}]}}]}`))
		default:
			t.Fatalf("unexpected PayPal API path: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	setupPayPalRefundTestDB(t, server.URL)
	resetPayPalTokenCache()
	plugin := NewPayPalPlugin(services.NewPluginService(database.GetDBWrapper()))

	resp, err := plugin.HandleWebhook(42, []byte(`{"event_type":"CHECKOUT.ORDER.APPROVED","resource":{"id":"ORDER-1"}}`), nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.Success)
	require.Equal(t, "completed", resp.Status)
	require.Equal(t, "CAPTURE-1", resp.PaymentID)
	require.EqualValues(t, 99, resp.BillID)
	require.Equal(t, 1, capturePosts, "capture should be attempted exactly once")
	require.Equal(t, 1, orderGets, "already-captured order should be resolved via GET")
}

// TestGetPayPalAccessTokenCachesToken locks P3: a token with a reported lifetime
// is cached per credential so back-to-back API calls do not re-fetch it.
func TestGetPayPalAccessTokenCachesToken(t *testing.T) {
	var tokenHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/oauth2/token", r.URL.Path)
		tokenHits++
		_, _ = w.Write([]byte(`{"access_token":"cached-token","expires_in":32400}`))
	}))
	defer server.Close()

	resetPayPalTokenCache()
	pp := &PayPalPlugin{}
	cfg := &PayPalConfig{
		ClientID:     testClientID,
		ClientSecret: testClientSecret,
		Environment:  "sandbox",
		APIBaseURL:   server.URL,
	}

	first, err := pp.getPayPalAccessToken(context.Background(), cfg)
	require.NoError(t, err)
	require.Equal(t, "cached-token", first)

	second, err := pp.getPayPalAccessToken(context.Background(), cfg)
	require.NoError(t, err)
	require.Equal(t, "cached-token", second)

	require.Equal(t, 1, tokenHits, "token should be fetched once and cached")
}

// TestRefundPaymentRejectsCancelledStatus locks P4: a refund PayPal reports as
// CANCELLED must surface an error, not be recorded as a completed refund.
func TestRefundPaymentRejectsCancelledStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"access-token"}`))
		case "/v2/payments/captures/CAP-CANCEL":
			require.Equal(t, http.MethodGet, r.Method)
			_, _ = w.Write([]byte(`{"id":"CAP-CANCEL","amount":{"value":"10.00","currency_code":"USD"}}`))
		case "/v2/payments/captures/CAP-CANCEL/refund":
			require.Equal(t, http.MethodPost, r.Method)
			_, _ = w.Write([]byte(`{"id":"REFUND-X","status":"CANCELLED"}`))
		default:
			t.Fatalf("unexpected PayPal API path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	setupPayPalRefundTestDB(t, server.URL)
	resetPayPalTokenCache()
	plugin := NewPayPalPlugin(services.NewPluginService(database.GetDBWrapper()))

	err := plugin.RefundPayment(42, "CAP-CANCEL", 1000)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "CANCELLED"), "error should name the cancelled status: %v", err)
}
