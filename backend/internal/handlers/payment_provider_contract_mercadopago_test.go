package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/paymentcontract"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func mercadoPagoContractAdapter() paymentcontract.Adapter {
	return providerContractAdapter{name: "mercadopago", verify: verifyMercadoPagoContractSignature, behavior: verifyMercadoPagoContractBehavior, rotation: verifyMercadoPagoContractRotation}
}

type mercadoPagoContractPayment struct {
	status, currency                     string
	businessID, billID                   uint
	amount, refunded, expectedBillAmount int64
}

func verifyMercadoPagoContractBehavior(t *testing.T, testCase paymentcontract.CaseSpec) error {
	t.Helper()
	return verifyProviderContractBehavior(t, testCase, newMercadoPagoContractWebhookDriver(t))
}

func newMercadoPagoContractWebhookDriver(t *testing.T) providerContractWebhookDriver {
	t.Helper()
	payments := map[string]mercadoPagoContractPayment{}
	refundedPaymentID := ""
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/payments/") && strings.HasSuffix(r.URL.Path, "/refunds") {
			refundedPaymentID = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/payments/"), "/refunds")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "mp-contract-compensation", "status": "approved"})
			return
		}
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/payments/") {
			http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
			return
		}
		paymentID := strings.TrimPrefix(r.URL.Path, "/v1/payments/")
		payment, ok := payments[paymentID]
		if !ok {
			http.Error(w, "payment not found", http.StatusNotFound)
			return
		}
		metadata := map[string]interface{}{}
		if payment.expectedBillAmount >= 0 {
			metadata["bill_amount_cents"] = payment.expectedBillAmount
			metadata["tip_amount_cents"] = 0
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": 1, "status": payment.status,
			"currency_id": payment.currency, "transaction_amount": float64(payment.amount) / 100,
			"transaction_amount_refunded": float64(payment.refunded) / 100,
			"external_reference":          fmt.Sprintf("bill_%d_business_%d", payment.billID, payment.businessID),
			"metadata":                    metadata,
		})
	}))
	t.Cleanup(api.Close)
	businessID := prepareProviderContractDatabase(t, "mercadopago", map[string]interface{}{
		"access_token":   "APP_USR-contract-access-token-1234567890",
		"public_key":     "APP_USR-contract-public-key-1234567890",
		"webhook_secret": "mp_contract_behavior", "environment": "sandbox",
		"base_url": "https://api.payverge.test", "api_base_url": api.URL,
	})
	plugins.GlobalRegistry.RegisterPlugin(mercadopago.NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper())))
	t.Cleanup(func() { plugins.GlobalRegistry.UnregisterPlugin("mercadopago") })
	return providerContractWebhookDriver{
		name: "mercadopago", businessID: businessID,
		refunded: func(paymentID string) error {
			if refundedPaymentID != paymentID {
				return fmt.Errorf("MercadoPago compensation refunded %q, want %q", refundedPaymentID, paymentID)
			}
			return nil
		},
		send: func(t *testing.T, kind, eventID, paymentID string, signingBusinessID, billID uint, amount int64, currency string, expectedBillAmount int64) *httptest.ResponseRecorder {
			status := "approved"
			switch kind {
			case "pending":
				status = "pending"
			case "cancellation", "expiration":
				status = "cancelled"
			case "refund":
				status = "refunded"
			case "reversal":
				status = "charged_back"
			case "dispute":
				status = "in_mediation"
			}
			// MercadoPago reports transaction_amount_refunded only once money
			// was actually refunded; an approved capture carries 0.
			refunded := int64(0)
			if status == "refunded" {
				refunded = amount
			}
			payments[paymentID] = mercadoPagoContractPayment{status: status, currency: currency, businessID: signingBusinessID, billID: billID, amount: amount, refunded: refunded, expectedBillAmount: expectedBillAmount}
			return sendMercadoPagoContractWebhook(t, kind, eventID, paymentID, signingBusinessID, billID, "mp_contract_behavior")
		},
		statusErr: func(t *testing.T) error {
			status, err := mercadopago.NewMercadoPagoPlugin(services.NewPluginService(database.GetDBWrapper())).GetPaymentStatus(businessID, "")
			if status != "" {
				return fmt.Errorf("MercadoPago empty payment id returned status %q", status)
			}
			return err
		},
		reconcile: func(t *testing.T, billID uint, paymentID string) error {
			payments[paymentID] = mercadoPagoContractPayment{
				status: "approved", currency: "USD", businessID: businessID,
				billID: billID, amount: 1000, expectedBillAmount: 1000,
			}
			NewPluginHandlers(nil, nil).ReconcilePendingPluginPayments(t.Context(), 10)
			return nil
		},
	}
}

func sendMercadoPagoContractWebhook(t *testing.T, kind, eventID, paymentID string, businessID, billID uint, secret string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(map[string]interface{}{
		"id": eventID, "action": "payment.updated", "type": "payment", "data": map[string]interface{}{"id": paymentID},
	})
	if err != nil {
		t.Fatal(err)
	}
	timestamp := time.Now().UnixMilli()
	requestID := "request-" + eventID
	manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%d;", strings.ToLower(paymentID), requestID, timestamp)
	if kind == "wrong_account" {
		secret = "mp_other_account_secret"
	}
	signature := fmt.Sprintf("ts=%d,v1=%s", timestamp, contractHMACHex(secret, []byte(manifest)))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	endpoint := fmt.Sprintf("/webhooks/mercadopago?data.id=%s&business_id=%d&bill_id=%d", url.QueryEscape(paymentID), businessID, billID)
	c.Request = httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	c.Request.Header.Set("x-signature", signature)
	c.Request.Header.Set("x-request-id", requestID)
	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")
	return w
}

func verifyMercadoPagoContractSignature(t *testing.T, testCase paymentcontract.SignatureCase) error {
	t.Helper()
	secret := "mp_contract_secret"
	dataID := "123456"
	requestID := "contract-request-id"
	timestamp := time.Now().UnixMilli()
	signature := ""
	if testCase != paymentcontract.SignatureMissing {
		manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%d;", dataID, requestID, timestamp)
		signature = fmt.Sprintf("ts=%d,v1=%s", timestamp, contractHMACHex(secret, []byte(manifest)))
	}
	if testCase == paymentcontract.SignatureInvalid {
		signature = fmt.Sprintf("ts=%d,v1=deadbeef", timestamp)
	}
	return mercadopago.NewMercadoPagoPlugin(nil).VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(`{"action":"payment.updated","data":{"id":"123456"}}`),
		Headers: map[string]string{
			"x-signature":  signature,
			"x-request-id": requestID,
		},
		Query:  url.Values{"data.id": []string{dataID}},
		Config: map[string]interface{}{"webhook_secret": secret},
	})
}

func verifyMercadoPagoContractRotation(t *testing.T) paymentcontract.SecretRotationObservation {
	t.Helper()
	previous := "mp_contract_previous"
	dataID := "rotation-payment"
	requestID := "rotation-request-id"
	timestamp := time.Now().UnixMilli()
	manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%d;", dataID, requestID, timestamp)
	request := plugins.WebhookVerificationRequest{
		Payload: []byte(`{"action":"payment.updated","data":{"id":"rotation-payment"}}`),
		Headers: map[string]string{
			"x-signature":  fmt.Sprintf("ts=%d,v1=%s", timestamp, contractHMACHex(previous, []byte(manifest))),
			"x-request-id": requestID,
		},
		Query: url.Values{"data.id": []string{dataID}},
		Config: map[string]interface{}{
			"webhook_secret":          "mp_contract_current",
			"webhook_secret_previous": previous,
		},
	}
	plugin := mercadopago.NewMercadoPagoPlugin(nil)
	configured := plugin.VerifyWebhookRequest(request)
	delete(request.Config, "webhook_secret_previous")
	return paymentcontract.SecretRotationObservation{
		PreviousConfigured: configured,
		PreviousRetired:    plugin.VerifyWebhookRequest(request),
	}
}
