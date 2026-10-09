package paypal

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/plugins"
)

func TestPayPalVerifyWebhookSignature_RejectsEmptySignature(t *testing.T) {
	plugin := &PayPalPlugin{}
	payload := []byte(`{"event_type":"PAYMENT.CAPTURE.COMPLETED"}`)
	if ok := plugin.VerifyWebhookSignature(payload, "", "test_secret"); ok {
		t.Fatal("expected empty signature to be rejected")
	}
}

func TestPayPalVerifyWebhookSignature_RejectsEmptySecret(t *testing.T) {
	plugin := &PayPalPlugin{}
	payload := []byte(`{"event_type":"PAYMENT.CAPTURE.COMPLETED"}`)
	if ok := plugin.VerifyWebhookSignature(payload, "transmission-signature", ""); ok {
		t.Fatal("expected empty secret to be rejected")
	}
}

func TestPayPalVerifyWebhookRequest_UsesOfficialVerificationAPI(t *testing.T) {
	var verifiedRequest map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			username, password, ok := r.BasicAuth()
			if !ok || username != "client-id-12345678901234567890" || password != "client-secret-12345678901234567890" {
				t.Fatalf("unexpected PayPal token basic auth")
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != "grant_type=client_credentials" {
				t.Fatalf("unexpected token body: %s", string(body))
			}
			_, _ = w.Write([]byte(`{"access_token":"access-token"}`))
		case "/v1/notifications/verify-webhook-signature":
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Fatalf("unexpected authorization header: %s", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&verifiedRequest); err != nil {
				t.Fatalf("failed to decode verification request: %v", err)
			}
			_, _ = w.Write([]byte(`{"verification_status":"SUCCESS"}`))
		default:
			t.Fatalf("unexpected PayPal API path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	plugin := &PayPalPlugin{}
	payload := []byte(`{"id":"WH-1","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAPTURE-1"}}`)
	err := plugin.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: payload,
		Headers: map[string]string{
			"PayPal-Transmission-Id":   "transmission-id",
			"PayPal-Transmission-Time": "2026-05-08T12:34:56Z",
			"PayPal-Cert-Url":          "https://api-m.sandbox.paypal.com/certs/test.pem",
			"PayPal-Auth-Algo":         "SHA256withRSA",
			"PayPal-Transmission-Sig":  "transmission-signature",
		},
		Config: map[string]interface{}{
			"client_id":     "client-id-12345678901234567890",
			"client_secret": "client-secret-12345678901234567890",
			"environment":   "sandbox",
			"webhook_id":    "WH-123456789",
			"api_base_url":  server.URL,
		},
	})
	if err != nil {
		t.Fatalf("expected PayPal verification to pass, got %v", err)
	}

	if verifiedRequest["transmission_id"] != "transmission-id" {
		t.Fatalf("expected transmission_id, got %#v", verifiedRequest["transmission_id"])
	}
	if verifiedRequest["webhook_id"] != "WH-123456789" {
		t.Fatalf("expected webhook_id, got %#v", verifiedRequest["webhook_id"])
	}
	if verifiedRequest["transmission_sig"] != "transmission-signature" {
		t.Fatalf("expected transmission_sig, got %#v", verifiedRequest["transmission_sig"])
	}
	if _, ok := verifiedRequest["webhook_event"].(map[string]interface{}); !ok {
		t.Fatalf("expected webhook_event object, got %#v", verifiedRequest["webhook_event"])
	}
}

func TestPayPalVerifyWebhookRequest_RejectsFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"access-token"}`))
		case "/v1/notifications/verify-webhook-signature":
			_, _ = w.Write([]byte(`{"verification_status":"FAILURE"}`))
		default:
			t.Fatalf("unexpected PayPal API path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	plugin := &PayPalPlugin{}
	err := plugin.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(`{"id":"WH-1","event_type":"PAYMENT.CAPTURE.COMPLETED"}`),
		Headers: map[string]string{
			"PayPal-Transmission-Id":   "transmission-id",
			"PayPal-Transmission-Time": "2026-05-08T12:34:56Z",
			"PayPal-Cert-Url":          "https://api-m.sandbox.paypal.com/certs/test.pem",
			"PayPal-Auth-Algo":         "SHA256withRSA",
			"PayPal-Transmission-Sig":  "transmission-signature",
		},
		Config: map[string]interface{}{
			"client_id":     "client-id-12345678901234567890",
			"client_secret": "client-secret-12345678901234567890",
			"environment":   "sandbox",
			"webhook_id":    "WH-123456789",
			"api_base_url":  server.URL,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("expected verification failure, got %v", err)
	}
}

func TestPayPalVerifyWebhookRequest_AcceptsPreviousWebhookIDDuringRotation(t *testing.T) {
	var attempted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"access-token"}`))
		case "/v1/notifications/verify-webhook-signature":
			var body map[string]interface{}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			webhookID, _ := body["webhook_id"].(string)
			attempted = append(attempted, webhookID)
			status := "FAILURE"
			if webhookID == "WH-PREVIOUS" {
				status = "SUCCESS"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"verification_status": status})
		default:
			t.Fatalf("unexpected PayPal API path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	err := (&PayPalPlugin{}).VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(`{"id":"WH-1","event_type":"PAYMENT.CAPTURE.COMPLETED"}`),
		Headers: map[string]string{
			"PayPal-Transmission-Id":   "transmission-id",
			"PayPal-Transmission-Time": "2026-05-08T12:34:56Z",
			"PayPal-Cert-Url":          "https://api-m.sandbox.paypal.com/certs/test.pem",
			"PayPal-Auth-Algo":         "SHA256withRSA",
			"PayPal-Transmission-Sig":  "transmission-signature",
		},
		Config: map[string]interface{}{
			"client_id":           "client-id-12345678901234567890",
			"client_secret":       "client-secret-12345678901234567890",
			"environment":         "sandbox",
			"webhook_id":          "WH-CURRENT",
			"webhook_id_previous": "WH-PREVIOUS",
			"api_base_url":        server.URL,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"WH-CURRENT", "WH-PREVIOUS"}, attempted)
}

func TestPayPalHandleWebhook_CaptureCompletedExtractsBillReference(t *testing.T) {
	plugin := &PayPalPlugin{}
	response, err := plugin.HandleWebhook(7, []byte(`{
		"id":"WH-1",
		"event_type":"PAYMENT.CAPTURE.COMPLETED",
		"resource":{
			"id":"CAPTURE-1",
			"status":"COMPLETED",
			"custom_id":"bill_42_business_7",
			"amount":{"value":"33.50","currency_code":"USD"}
		}
	}`), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !response.Success {
		t.Fatalf("expected success response: %#v", response)
	}
	if response.PaymentID != "CAPTURE-1" {
		t.Fatalf("expected capture payment id, got %q", response.PaymentID)
	}
	if response.BillID != 42 {
		t.Fatalf("expected bill id 42, got %d", response.BillID)
	}
	if response.Amount != 3350 {
		t.Fatalf("expected amount 3350, got %d", response.Amount)
	}
	if response.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", response.Currency)
	}
}

func TestPayPalHandleWebhook_CaptureRefundedUsesOriginalCaptureID(t *testing.T) {
	plugin := &PayPalPlugin{}
	response, err := plugin.HandleWebhook(7, []byte(`{
		"id":"WH-1",
		"event_type":"PAYMENT.CAPTURE.REFUNDED",
		"resource":{
			"id":"REFUND-1",
			"custom_id":"bill_42_business_7",
			"amount":{"value":"33.50","currency_code":"USD"},
			"supplementary_data":{"related_ids":{"capture_id":"CAPTURE-1"}}
		}
	}`), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !response.Success {
		t.Fatalf("expected success response: %#v", response)
	}
	if response.Status != "refunded" {
		t.Fatalf("expected refunded status, got %q", response.Status)
	}
	if response.PaymentID != "CAPTURE-1" {
		t.Fatalf("expected original capture id, got %q", response.PaymentID)
	}
	if response.TransactionID != "REFUND-1" {
		t.Fatalf("expected refund transaction id, got %q", response.TransactionID)
	}
}

func TestPayPalHandleWebhook_LifecycleMappings(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		resource  string
		status    string
		paymentID string
	}{
		{
			name: "reversal", eventType: "PAYMENT.CAPTURE.REVERSED", status: "reversed", paymentID: "CAPTURE-1",
			resource: `{"id":"CAPTURE-1","custom_id":"bill_42_business_7"}`,
		},
		{
			name: "expiration", eventType: "CHECKOUT.ORDER.VOIDED", status: "expired", paymentID: "ORDER-1",
			resource: `{"id":"ORDER-1","custom_id":"bill_42_business_7"}`,
		},
		{
			name: "dispute", eventType: "CUSTOMER.DISPUTE.CREATED", status: "disputed", paymentID: "CAPTURE-1",
			resource: `{"id":"DISPUTE-1","custom_id":"bill_42_business_7","disputed_transactions":[{"seller_transaction_id":"CAPTURE-1"}]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := []byte(`{"id":"WH-LIFECYCLE","event_type":"` + test.eventType + `","resource":` + test.resource + `}`)
			response, err := (&PayPalPlugin{}).HandleWebhook(7, payload, nil)
			require.NoError(t, err)
			require.True(t, response.Success)
			require.Equal(t, test.status, response.Status)
			require.Equal(t, test.paymentID, response.PaymentID)
			require.EqualValues(t, 42, response.BillID)
		})
	}
}

func TestPayPalHandleWebhook_DisputeRequiresOriginalCaptureBinding(t *testing.T) {
	response, err := (&PayPalPlugin{}).HandleWebhook(7, []byte(`{
		"id":"WH-DISPUTE","event_type":"CUSTOMER.DISPUTE.CREATED",
		"resource":{"id":"DISPUTE-ONLY","custom_id":"bill_42_business_7"}
	}`), nil)
	require.NoError(t, err)
	require.False(t, response.Success)
	require.Contains(t, response.Message, "capture ID")
}

func TestPayPalHandleWebhook_OrderApprovedCapturesOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Basic ") {
				t.Fatalf("expected basic auth, got %s", auth)
			}
			decoded, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
			if !strings.Contains(string(decoded), "client-id-12345678901234567890:client-secret-12345678901234567890") {
				t.Fatalf("unexpected token auth: %s", string(decoded))
			}
			_, _ = w.Write([]byte(`{"access_token":"access-token"}`))
		case "/v2/checkout/orders/ORDER-1/capture":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST capture, got %s", r.Method)
			}
			_, _ = w.Write([]byte(`{
				"id":"ORDER-1",
				"status":"COMPLETED",
				"purchase_units":[{
					"custom_id":"bill_42_business_7",
					"payments":{"captures":[{
						"id":"CAPTURE-1",
						"status":"COMPLETED",
						"amount":{"value":"33.50","currency_code":"USD"}
					}]}
				}]
			}`))
		default:
			t.Fatalf("unexpected PayPal API path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	plugin := &PayPalPlugin{}

	response, err := plugin.handleOrderApproved(map[string]interface{}{
		"resource": map[string]interface{}{
			"id": "ORDER-1",
			"purchase_units": []interface{}{
				map[string]interface{}{"custom_id": "bill_42_business_7"},
			},
		},
	}, &PayPalConfig{
		ClientID:     "client-id-12345678901234567890",
		ClientSecret: "client-secret-12345678901234567890",
		Environment:  "sandbox",
		APIBaseURL:   server.URL,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !response.Success {
		t.Fatalf("expected success response: %#v", response)
	}
	if response.PaymentID != "CAPTURE-1" {
		t.Fatalf("expected capture id, got %q", response.PaymentID)
	}
	if response.BillID != 42 {
		t.Fatalf("expected bill id 42, got %d", response.BillID)
	}
	if response.Amount != 3350 {
		t.Fatalf("expected amount 3350, got %d", response.Amount)
	}
}

func TestBuildPayPalCallbackURL_IncludesBillID(t *testing.T) {
	callbackURL, err := buildPayPalCallbackURL("https://api.example.com/", "/api/v1/webhooks/paypal/return", 42)
	if err != nil {
		t.Fatalf("expected callback URL to build: %v", err)
	}
	if callbackURL != "https://api.example.com/api/v1/webhooks/paypal/return?bill_id=42" {
		t.Fatalf("unexpected callback URL: %s", callbackURL)
	}
}

func TestBuildPayPalCallbackURL_RequiresBaseURL(t *testing.T) {
	_, err := buildPayPalCallbackURL("", "/api/v1/webhooks/paypal/return", 42)
	if err == nil {
		t.Fatal("expected missing base_url to fail")
	}
}

func TestPayPalCallbackBaseURL_PrefersRuntimeOrigin(t *testing.T) {
	prev := callbackBaseURL
	t.Cleanup(func() { callbackBaseURL = prev })

	// Runtime origin wins even when the per-business base_url points at prod.
	callbackBaseURL = func() string { return "https://api.staging.example" }
	got := paypalCallbackBaseURL(&PayPalConfig{BaseURL: "https://api.payverge.io"})
	require.Equal(t, "https://api.staging.example", got)

	// The per-business base_url is never used, even with no runtime origin:
	// a tenant must not be able to redirect its guests to another host.
	callbackBaseURL = func() string { return "" }
	got = paypalCallbackBaseURL(&PayPalConfig{BaseURL: "https://attacker.example"})
	require.Equal(t, "", got)
}

func TestPayPalHandleWebhook_CaptureCompletedCarriesOrderTrackerMetadata(t *testing.T) {
	plugin := &PayPalPlugin{}
	response, err := plugin.HandleWebhook(7, []byte(`{
		"id": "WH-2",
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": {
			"id": "CAPTURE-1",
			"status": "COMPLETED",
			"custom_id": "bill_42_business_7",
			"supplementary_data": {"related_ids": {"order_id": "ORDER-9"}},
			"amount": {"value": "33.50", "currency_code": "USD"}
		}
	}`), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if response.Metadata == nil || response.Metadata["provider_tracker_id"] != "ORDER-9" {
		t.Fatalf("expected Metadata.provider_tracker_id ORDER-9, got %#v", response.Metadata)
	}
}

// A CUSTOMER.DISPUTE.CREATED resource has no top-level custom_id: PayPal
// echoes the purchase unit's custom_id as "custom" on each disputed
// transaction. Without reading it, a real dispute resolves to no bill and no
// business, so it can never be deferred until its capture is recorded.
func TestPayPalBillBusinessFromResource_DisputedTransactionsCustom(t *testing.T) {
	resource := map[string]interface{}{
		"dispute_id": "PP-D-1",
		"disputed_transactions": []interface{}{
			map[string]interface{}{"seller_transaction_id": "CAPTURE-1"},
			map[string]interface{}{"seller_transaction_id": "CAPTURE-2", "custom": "bill_42_business_7"},
		},
	}
	billID, businessID := payPalBillBusinessFromResource(resource)
	if billID != 42 || businessID != 7 {
		t.Fatalf("payPalBillBusinessFromResource = (%d, %d), want (42, 7)", billID, businessID)
	}

	response, err := (&PayPalPlugin{}).handlePayPalDispute(map[string]interface{}{"resource": resource})
	if err != nil || response == nil || !response.Success {
		t.Fatalf("handlePayPalDispute = %+v, %v", response, err)
	}
	if response.BillID != 42 || response.PaymentID != "CAPTURE-1" || response.Status != "disputed" {
		t.Fatalf("dispute response = bill %d payment %q status %q", response.BillID, response.PaymentID, response.Status)
	}
}
