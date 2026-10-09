package mercadopago

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func buildMPOfficialSignature(t *testing.T, secret string, timestampMillis int64, dataID, requestID string) string {
	t.Helper()
	manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%d;", dataID, requestID, timestampMillis)
	mac := hmac.New(sha256.New, []byte(secret))
	_, err := mac.Write([]byte(manifest))
	if err != nil {
		t.Fatalf("buildMPTestSignature: failed to write payload: %v", err)
	}
	digest := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("ts=%d,v1=%s", timestampMillis, digest)
}

func TestMPVerifyWebhookSignature_RejectsEmptySignature(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	payload := []byte(`{"action":"payment.created","id":"123"}`)
	if ok := plugin.VerifyWebhookSignature(payload, "", "mysecret"); ok {
		t.Fatal("expected empty signature to be rejected")
	}
}

func TestMPVerifyWebhookSignature_RejectsEmptySecret(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	payload := []byte(`{"action":"payment.created","id":"123"}`)
	timestamp := time.Now().UnixMilli()
	signature := buildMPOfficialSignature(t, "mysecret", timestamp, "123", "request-1")
	if ok := plugin.VerifyWebhookSignature(payload, signature, ""); ok {
		t.Fatal("expected empty secret to be rejected")
	}
}

func TestMPVerifyWebhookRequest_AcceptsOfficialManifestSignature(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	secret := "mp_webhook_secret"
	timestamp := time.Now().UnixMilli()
	requestID := "bb56a2f1-6aae-46ac-982e-9dcd3581d08e"
	signature := buildMPOfficialSignature(t, secret, timestamp, "123456", requestID)

	err := plugin.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(`{"action":"payment.updated","api_version":"v1","data":{"id":"123456"},"type":"payment"}`),
		Headers: map[string]string{
			"x-signature":  signature,
			"x-request-id": requestID,
		},
		Query: url.Values{"data.id": []string{"123456"}, "type": []string{"payment"}},
		Config: map[string]interface{}{
			"webhook_secret": secret,
		},
	})
	if err != nil {
		t.Fatalf("expected official MercadoPago signature to pass, got %v", err)
	}
}

func TestMPVerifyWebhookRequest_AcceptsPreviousSecretDuringRotation(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	previousSecret := "mp_previous_webhook_secret"
	timestamp := time.Now().UnixMilli()
	requestID := "rotation-request"
	signature := buildMPOfficialSignature(t, previousSecret, timestamp, "123456", requestID)

	err := plugin.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(`{"action":"payment.updated","data":{"id":"123456"}}`),
		Headers: map[string]string{"x-signature": signature, "x-request-id": requestID},
		Query:   url.Values{"data.id": []string{"123456"}},
		Config: map[string]interface{}{
			"webhook_secret":          "mp_current_webhook_secret",
			"webhook_secret_previous": previousSecret,
		},
	})
	require.NoError(t, err)
}

func TestMPVerifyWebhookRequest_LowercasesAlphanumericDataID(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	secret := "mp_webhook_secret"
	timestamp := time.Now().UnixMilli()
	requestID := "2066ca19-c6f1-498a-be75-1923005edd06"
	signature := buildMPOfficialSignature(t, secret, timestamp, "ord01jq4s4ky8hwq6na5pxb65b3d3", requestID)

	err := plugin.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(`{"action":"order.action_required","api_version":"v1","data":{"id":"ORD01JQ4S4KY8HWQ6NA5PXB65B3D3"},"type":"order"}`),
		Headers: map[string]string{
			"x-signature":  signature,
			"x-request-id": requestID,
		},
		Query: url.Values{"data.id": []string{"ORD01JQ4S4KY8HWQ6NA5PXB65B3D3"}, "type": []string{"order"}},
		Config: map[string]interface{}{
			"webhook_secret": secret,
		},
	})
	if err != nil {
		t.Fatalf("expected upper-case MercadoPago data.id to verify with lower-case manifest, got %v", err)
	}
}

func TestMPVerifyWebhookRequest_RejectsBodyHMACSignature(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	secret := "mp_webhook_secret"
	payload := []byte(`{"action":"payment.created","id":"123"}`)
	timestamp := time.Now().UnixMilli()
	bodyManifest := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(bodyManifest))
	signature := fmt.Sprintf("ts=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))

	err := plugin.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: payload,
		Headers: map[string]string{
			"x-signature":  signature,
			"x-request-id": "request-1",
		},
		Query:  url.Values{"data.id": []string{"123"}},
		Config: map[string]interface{}{"webhook_secret": secret},
	})
	if err == nil {
		t.Fatal("expected body HMAC signature to be rejected")
	}
}

func TestMPVerifyWebhookRequest_RejectsStaleTimestamp(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	secret := "mp_webhook_secret"
	staleTimestamp := time.Now().Add(-10 * time.Minute).UnixMilli()
	signature := buildMPOfficialSignature(t, secret, staleTimestamp, "123", "request-1")
	err := plugin.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(`{"action":"payment.created","id":"123"}`),
		Headers: map[string]string{
			"x-signature":  signature,
			"x-request-id": "request-1",
		},
		Query:  url.Values{"data.id": []string{"123"}},
		Config: map[string]interface{}{"webhook_secret": secret},
	})
	if err == nil {
		t.Fatal("expected stale timestamp to be rejected")
	}
}

// F1: OAuth-connected merchants persist config without webhook_secret; MP still
// signs with the platform app secret from MERCADOPAGO_WEBHOOK_SECRET.
func TestMPVerifyWebhookRequest_FallsBackToEnvSecretWhenConfigEmpty(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	envSecret := "platform_mp_webhook_secret"
	t.Setenv("MERCADOPAGO_WEBHOOK_SECRET", envSecret)
	t.Setenv("MERCADOPAGO_WEBHOOK_SECRET_PREVIOUS", "")

	timestamp := time.Now().UnixMilli()
	requestID := "oauth-env-fallback-request"
	signature := buildMPOfficialSignature(t, envSecret, timestamp, "789012", requestID)

	// OAuth-style config: tokens present, no per-business webhook_secret.
	err := plugin.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(`{"action":"payment.updated","data":{"id":"789012"},"type":"payment"}`),
		Headers: map[string]string{
			"x-signature":  signature,
			"x-request-id": requestID,
		},
		Query: url.Values{"data.id": []string{"789012"}, "type": []string{"payment"}},
		Config: map[string]interface{}{
			"connection_mode": "oauth",
			"access_token":    "APP_USR-oauth-token",
			"refresh_token":   "refresh-xyz",
			// webhook_secret intentionally absent
		},
	})
	require.NoError(t, err, "OAuth config without webhook_secret must verify via MERCADOPAGO_WEBHOOK_SECRET")
}

func TestMPVerifyWebhookRequest_PrefersBusinessSecretBeforeEnv(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	businessSecret := "business_webhook_secret"
	envSecret := "platform_mp_webhook_secret"
	t.Setenv("MERCADOPAGO_WEBHOOK_SECRET", envSecret)

	timestamp := time.Now().UnixMilli()
	requestID := "business-prefers-request"
	// Signed with business secret — must still pass even when env is set.
	signature := buildMPOfficialSignature(t, businessSecret, timestamp, "111222", requestID)

	err := plugin.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(`{"action":"payment.updated","data":{"id":"111222"}}`),
		Headers: map[string]string{"x-signature": signature, "x-request-id": requestID},
		Query:   url.Values{"data.id": []string{"111222"}},
		Config:  map[string]interface{}{"webhook_secret": businessSecret},
	})
	require.NoError(t, err)

	// Signed only with env secret while business secret is wrong — env is a
	// fallback candidate so verification still succeeds.
	envSig := buildMPOfficialSignature(t, envSecret, timestamp, "111222", requestID)
	err = plugin.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(`{"action":"payment.updated","data":{"id":"111222"}}`),
		Headers: map[string]string{"x-signature": envSig, "x-request-id": requestID},
		Query:   url.Values{"data.id": []string{"111222"}},
		Config:  map[string]interface{}{"webhook_secret": "wrong-business-secret"},
	})
	require.NoError(t, err, "env secret must be tried after business secret")
}

func TestMPHandlePaymentUpdated_FetchesPaymentDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payments/123456" {
			t.Fatalf("unexpected MercadoPago path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer APP_USR-1234567890-test-token" {
			t.Fatalf("unexpected authorization header: %s", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":                 123456,
			"status":             "approved",
			"currency_id":        "MXN",
			"transaction_amount": 33.50,
			"external_reference": "bill_42_business_7",
		})
	}))
	defer server.Close()

	plugin := &MercadoPagoPlugin{}
	response, err := plugin.handlePaymentUpdated(map[string]interface{}{
		"action": "payment.updated",
		"data": map[string]interface{}{
			"id": "123456",
		},
	}, &MercadoPagoConfig{
		AccessToken: "APP_USR-1234567890-test-token",
		PublicKey:   "APP_USR-1234567890-test-key",
		APIBaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !response.Success {
		t.Fatalf("expected success response: %#v", response)
	}
	if response.PaymentID != "123456" {
		t.Fatalf("expected payment id 123456, got %q", response.PaymentID)
	}
	if response.BillID != 42 {
		t.Fatalf("expected bill id 42, got %d", response.BillID)
	}
	if response.Amount != 3350 {
		t.Fatalf("expected amount 3350, got %d", response.Amount)
	}
	if response.Currency != "MXN" {
		t.Fatalf("expected currency MXN, got %q", response.Currency)
	}
}

// TestMPHandlePaymentUpdated_RefundedAmountCentsIsNil asserts that a
// status="refunded" event does NOT populate RefundedAmountCents. MP's
// webhook Amount/TransactionAmount is the original capture, not the refunded
// portion, and there is no status_detail to determine full vs partial. The
// field must be nil so isPartialPluginRefund falls back to the legacy heuristic.
func TestMPHandlePaymentUpdated_RefundedAmountCentsIsNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":                 999,
			"status":             "refunded",
			"currency_id":        "ARS",
			"transaction_amount": 100.00,
			"external_reference": "bill_1_business_2",
		})
	}))
	defer server.Close()

	plugin := &MercadoPagoPlugin{}
	response, err := plugin.handlePaymentUpdated(map[string]interface{}{
		"action": "payment.updated",
		"data":   map[string]interface{}{"id": "999"},
	}, &MercadoPagoConfig{
		AccessToken: "test-token",
		PublicKey:   "test-key",
		APIBaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response.RefundedAmountCents != nil {
		t.Fatalf("expected RefundedAmountCents to be nil for MP refund; got %d", *response.RefundedAmountCents)
	}
	// Amount should still carry the capture amount for the legacy heuristic.
	if response.Amount != 10000 {
		t.Fatalf("expected Amount=10000 (capture cents); got %d", response.Amount)
	}
}

func TestMPHandleWebhookReportsUnsupportedActions(t *testing.T) {
	plugin := &MercadoPagoPlugin{}

	response, err := plugin.HandleWebhook(7, []byte(`{"action":"payment.refunded","type":"payment","data":{"id":"123456"}}`), nil)
	if err != nil {
		t.Fatalf("expected unsupported action to be acknowledged without provider error: %v", err)
	}
	if !response.Success {
		t.Fatalf("expected unsupported action response to acknowledge delivery: %#v", response)
	}
	if response.Message != "Unsupported MercadoPago webhook action: payment.refunded" {
		t.Fatalf("unexpected response message: %q", response.Message)
	}
}

func TestMPHandleOrderUpdated_FetchesOrderDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/orders/ORD123" {
			t.Fatalf("unexpected MercadoPago path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer APP_USR-order-token" {
			t.Fatalf("unexpected authorization header: %s", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":                 "ORD123",
			"status":             "processed",
			"currency_id":        "ARS",
			"external_reference": "bill_42_business_7",
			"transactions": map[string]interface{}{
				"payments": []map[string]interface{}{
					{"amount": "33.50"},
				},
			},
		})
	}))
	defer server.Close()

	plugin := &MercadoPagoPlugin{}
	response, err := plugin.handleOrderUpdated(7, map[string]interface{}{
		"action": "order.processed",
		"data": map[string]interface{}{
			"id": "ORD123",
		},
	}, &MercadoPagoConfig{
		AccessToken: "APP_USR-order-token",
		PublicKey:   "APP_USR-order-key",
		APIBaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !response.Success {
		t.Fatalf("expected success response: %#v", response)
	}
	if response.PaymentID != "ORD123" {
		t.Fatalf("expected payment id ORD123, got %q", response.PaymentID)
	}
	if response.BillID != 42 {
		t.Fatalf("expected bill id 42, got %d", response.BillID)
	}
	if response.Status != "completed" {
		t.Fatalf("expected status completed, got %q", response.Status)
	}
	if response.Amount != 3350 {
		t.Fatalf("expected amount 3350, got %d", response.Amount)
	}
	if response.Currency != "ARS" {
		t.Fatalf("expected currency ARS, got %q", response.Currency)
	}
	if response.Metadata["payverge_tracker_id"] != "ORD123" {
		t.Fatalf("expected tracker metadata ORD123, got %#v", response.Metadata)
	}
}

func TestMPHandleWebhook_UnknownOrderActionIsNoOp(t *testing.T) {
	plugin := &MercadoPagoPlugin{}
	response, err := plugin.HandleWebhook(7, []byte(`{"action":"order.something_new","type":"order","data":{"id":"ORD999"}}`), nil)
	if err != nil {
		t.Fatalf("expected unknown order action to be acknowledged without provider error: %v", err)
	}
	if !response.Success {
		t.Fatalf("expected unsupported action response to acknowledge delivery: %#v", response)
	}
	if response.Message != "Unsupported MercadoPago webhook action: order.something_new" {
		t.Fatalf("unexpected response message: %q", response.Message)
	}
}

func TestMPHandleOrderUpdated_RejectsBusinessMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":                 "ORD-MISMATCH",
			"status":             "processed",
			"currency_id":        "USD",
			"external_reference": "bill_1_business_99",
			"transactions":       map[string]interface{}{"payments": []map[string]interface{}{{"amount": "10.00"}}},
		})
	}))
	defer server.Close()

	plugin := &MercadoPagoPlugin{}
	response, err := plugin.handleOrderUpdated(7, map[string]interface{}{
		"action": "order.processed",
		"data":   map[string]interface{}{"id": "ORD-MISMATCH"},
	}, &MercadoPagoConfig{
		AccessToken: "token",
		PublicKey:   "key",
		APIBaseURL:  server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response.Success {
		t.Fatalf("expected business mismatch to fail closed: %#v", response)
	}
}

func TestMPHandleWebhook_PaymentUpdatedStillWorks(t *testing.T) {
	// Regression: orders-topic support must not break payment.updated settlement.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payments/555" {
			t.Fatalf("unexpected path for payment.updated: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":                 555,
			"status":             "approved",
			"currency_id":        "USD",
			"transaction_amount": 12.00,
			"external_reference": "bill_9_business_42",
		})
	}))
	defer server.Close()

	setupMercadoPagoRefundTestDB(t, server.URL)
	plugin := NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper()))

	response, err := plugin.HandleWebhook(42, []byte(`{"action":"payment.updated","data":{"id":"555"}}`), nil)
	require.NoError(t, err)
	require.True(t, response.Success)
	require.Equal(t, "555", response.PaymentID)
	require.Equal(t, uint(9), response.BillID)
	require.Equal(t, int64(1200), response.Amount)
	require.Equal(t, "completed", response.Status)
}
