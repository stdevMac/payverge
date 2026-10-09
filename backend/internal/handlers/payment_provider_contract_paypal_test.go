package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/paymentcontract"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/paypal"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func payPalContractAdapter() paymentcontract.Adapter {
	return providerContractAdapter{name: "paypal", verify: verifyPayPalContractSignature, behavior: verifyPayPalContractBehavior, rotation: verifyPayPalContractRotation}
}

func verifyPayPalContractBehavior(t *testing.T, testCase paymentcontract.CaseSpec) error {
	t.Helper()
	return verifyProviderContractBehavior(t, testCase, newPayPalContractWebhookDriver(t))
}

func newPayPalContractWebhookDriver(t *testing.T) providerContractWebhookDriver {
	t.Helper()
	var refundedPaymentID string
	verificationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"contract-access-token"}`))
		case "/v1/notifications/verify-webhook-signature":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read failed", http.StatusInternalServerError)
				return
			}
			var request map[string]interface{}
			if json.Unmarshal(body, &request) != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			status := "SUCCESS"
			if strings.Contains(fmt.Sprint(request["transmission_sig"]), "wrong-account") {
				status = "FAILURE"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"verification_status": status})
		default:
			const refundPrefix = "/v2/payments/captures/"
			const refundSuffix = "/refund"
			if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, refundPrefix) && strings.HasSuffix(r.URL.Path, refundSuffix) {
				refundedPaymentID = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, refundPrefix), refundSuffix)
				_ = json.NewEncoder(w).Encode(map[string]string{"id": "REFUND-CONTRACT-COMPENSATION", "status": "COMPLETED"})
				return
			}
			http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(verificationServer.Close)
	businessID := prepareProviderContractDatabase(t, "paypal", map[string]interface{}{
		"client_id": "client-id-contract-12345678901234567890", "client_secret": "client-secret-contract-12345678901234567890",
		"environment": "sandbox", "webhook_id": "WHCONTRACT", "base_url": "https://api.payverge.test", "api_base_url": verificationServer.URL,
	})
	plugins.GlobalRegistry.RegisterPlugin(paypal.NewPayPalPlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("paypal") })
	return providerContractWebhookDriver{
		name: "paypal", businessID: businessID,
		refunded: func(paymentID string) error {
			if refundedPaymentID != paymentID {
				return fmt.Errorf("PayPal compensation refunded %q, want %q", refundedPaymentID, paymentID)
			}
			return nil
		},
		send: func(t *testing.T, kind, eventID, paymentID string, signingBusinessID, billID uint, amount int64, currency string, expectedBillAmount int64) *httptest.ResponseRecorder {
			return sendPayPalContractWebhook(t, verificationServer.URL, kind, eventID, paymentID, signingBusinessID, billID, amount, currency, expectedBillAmount)
		},
		statusErr: func(t *testing.T) error {
			originalTransport := http.DefaultTransport
			http.DefaultTransport = providerContractRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("simulated PayPal provider timeout at %s", request.URL.Host)
			})
			defer func() { http.DefaultTransport = originalTransport }()
			status, err := paypal.NewPayPalPlugin(services.NewPluginService(database.GetDBWrapper())).GetPaymentStatus(businessID, "pay-timeout")
			if status != "" {
				return fmt.Errorf("PayPal empty payment id returned status %q", status)
			}
			return err
		},
		reconcile: func(t *testing.T, billID uint, paymentID string) error {
			originalTransport := http.DefaultTransport
			http.DefaultTransport = providerContractRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				switch request.URL.Path {
				case "/v1/oauth2/token":
					return providerContractHTTPResponse(http.StatusOK, `{"access_token":"contract-reconciliation-token"}`), nil
				case "/v2/checkout/orders/" + paymentID:
					return providerContractHTTPResponse(http.StatusOK, `{"status":"COMPLETED"}`), nil
				default:
					return nil, fmt.Errorf("unexpected PayPal reconciliation request %s", request.URL.String())
				}
			})
			t.Cleanup(func() { http.DefaultTransport = originalTransport })
			plugins.GlobalRegistry.RegisterPlugin(paypal.NewPayPalPlugin(services.NewPluginService(database.GetDBWrapper())))
			NewPluginHandlers(nil, nil).ReconcilePendingPluginPayments(t.Context(), 10)
			return nil
		},
	}
}

func sendPayPalContractWebhook(t *testing.T, verificationURL, kind, eventID, paymentID string, signingBusinessID, billID uint, amount int64, currency string, expectedBillAmount int64) *httptest.ResponseRecorder {
	t.Helper()
	eventType := "PAYMENT.CAPTURE.COMPLETED"
	switch kind {
	case "pending":
		eventType = "CHECKOUT.ORDER.CREATED"
	case "failed", "cancellation":
		eventType = "PAYMENT.CAPTURE.DENIED"
	case "reversal":
		eventType = "PAYMENT.CAPTURE.REVERSED"
	case "refund":
		eventType = "PAYMENT.CAPTURE.REFUNDED"
	case "dispute":
		eventType = "CUSTOMER.DISPUTE.CREATED"
	case "expiration":
		eventType = "CHECKOUT.ORDER.VOIDED"
	}
	resourceID := paymentID
	resource := map[string]interface{}{
		"id":        resourceID,
		"custom_id": fmt.Sprintf("bill_%d_business_%d", billID, signingBusinessID),
		"amount":    map[string]interface{}{"value": fmt.Sprintf("%.2f", float64(amount)/100), "currency_code": strings.ToUpper(currency)},
	}
	if kind == "refund" {
		resource["id"] = "refund-" + paymentID
		resource["supplementary_data"] = map[string]interface{}{"related_ids": map[string]interface{}{"capture_id": paymentID}}
	}
	if kind == "dispute" {
		resource["id"] = "dispute-" + paymentID
		resource["disputed_transactions"] = []interface{}{
			map[string]interface{}{"seller_transaction_id": paymentID},
		}
	}
	payload, err := json.Marshal(map[string]interface{}{"id": eventID, "event_type": eventType, "resource": resource})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/webhooks/paypal", bytes.NewReader(payload))
	prefix := "contract-behavior"
	if kind == "wrong_account" {
		prefix = "wrong-account"
	}
	for key, value := range payPalContractHeaders(prefix) {
		c.Request.Header.Set(key, value)
	}
	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "paypal")
	return w
}

func verifyPayPalContractSignature(t *testing.T, testCase paymentcontract.SignatureCase) error {
	t.Helper()
	verificationStatus := "SUCCESS"
	if testCase == paymentcontract.SignatureInvalid {
		verificationStatus = "FAILURE"
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"contract-access-token"}`))
		case "/v1/notifications/verify-webhook-signature":
			_ = json.NewEncoder(w).Encode(map[string]string{"verification_status": verificationStatus})
		default:
			t.Errorf("unexpected PayPal verification path %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	headers := map[string]string{}
	if testCase != paymentcontract.SignatureMissing {
		headers = payPalContractHeaders("contract")
	}
	return paypal.NewPayPalPlugin(nil).VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(`{"id":"WH-CONTRACT","event_type":"PAYMENT.CAPTURE.COMPLETED"}`),
		Headers: headers,
		Config:  payPalContractConfig(server.URL),
	})
}

func verifyPayPalContractRotation(t *testing.T) paymentcontract.SecretRotationObservation {
	t.Helper()
	previousID := "WH-PREVIOUS"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			_, _ = w.Write([]byte(`{"access_token":"contract-access-token"}`))
		case "/v1/notifications/verify-webhook-signature":
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var request map[string]interface{}
			require.NoError(t, json.Unmarshal(body, &request))
			status := "FAILURE"
			if request["webhook_id"] == previousID {
				status = "SUCCESS"
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"verification_status": status})
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	verify := func(includePrevious bool) error {
		config := payPalContractConfig(server.URL)
		config["webhook_id"] = "WH-CURRENT"
		if includePrevious {
			config["webhook_id_previous"] = previousID
		}
		return paypal.NewPayPalPlugin(nil).VerifyWebhookRequest(plugins.WebhookVerificationRequest{
			Payload: []byte(`{"id":"WH-ROTATION","event_type":"PAYMENT.CAPTURE.COMPLETED"}`),
			Headers: payPalContractHeaders("rotation"),
			Config:  config,
		})
	}
	return paymentcontract.SecretRotationObservation{
		PreviousConfigured: verify(true),
		PreviousRetired:    verify(false),
	}
}

func payPalContractHeaders(prefix string) map[string]string {
	return map[string]string{
		"PayPal-Transmission-Id":   prefix + "-transmission-id",
		"PayPal-Transmission-Time": "2026-08-02T12:00:00Z",
		"PayPal-Cert-Url":          "https://api-m.sandbox.paypal.com/certs/contract.pem",
		"PayPal-Auth-Algo":         "SHA256withRSA",
		"PayPal-Transmission-Sig":  prefix + "-signature",
	}
}

func payPalContractConfig(apiBaseURL string) map[string]interface{} {
	return map[string]interface{}{
		"client_id":     "client-id-contract-12345678901234567890",
		"client_secret": "client-secret-contract-12345678901234567890",
		"environment":   "sandbox",
		"webhook_id":    "WH-CONTRACT",
		"api_base_url":  apiBaseURL,
	}
}
