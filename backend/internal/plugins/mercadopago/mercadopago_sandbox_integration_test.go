//go:build mercadopago_sandbox
// +build mercadopago_sandbox

package mercadopago

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/plugins"
)

func TestMercadoPagoSandboxIntegration_CreatePreferenceAndVerifyWebhookSecret(t *testing.T) {
	config := mercadoPagoSandboxConfigFromEnv(t)
	plugin := &MercadoPagoPlugin{}

	userResponse, err := plugin.makeMercadoPagoAPICall(context.Background(), http.MethodGet, "/users/me", nil, config)
	if err != nil {
		t.Fatalf("get MercadoPago sandbox account: %v", err)
	}
	if userID := stringFromMercadoPagoAny(userResponse["id"]); userID == "" {
		t.Fatalf("MercadoPago users/me response missing id: %#v", userResponse)
	}

	notificationURL, err := mercadoPagoWebhookURL(config, 1)
	if err != nil {
		t.Fatalf("build MercadoPago webhook URL: %v", err)
	}
	preferenceResponse, err := plugin.makeMercadoPagoAPICall(context.Background(), http.MethodPost, "/checkout/preferences", PaymentPreferenceRequest{
		Items: []PreferenceItem{
			{
				Title:      "Payverge sandbox bill payment readiness check",
				Quantity:   1,
				UnitPrice:  1.00,
				CurrencyID: mercadoPagoSandboxCurrency(),
			},
		},
		BackURLs: &BackURLs{
			Success: "http://localhost:3000/payment/sandbox/success",
			Failure: "http://localhost:3000/payment/sandbox/failure",
			Pending: "http://localhost:3000/payment/sandbox/pending",
		},
		AutoReturn:        "approved",
		ExternalReference: "bill_1_business_1",
		NotificationURL:   notificationURL,
	}, config)
	if err != nil {
		t.Fatalf("create MercadoPago sandbox preference: %v", err)
	}
	if preferenceID := strings.TrimSpace(stringFromMercadoPagoAny(preferenceResponse["id"])); preferenceID == "" {
		t.Fatalf("MercadoPago preference response missing id: %#v", preferenceResponse)
	}
	if checkoutURL := firstMercadoPagoCheckoutURL(preferenceResponse); checkoutURL == "" {
		t.Fatalf("MercadoPago preference response missing checkout URL: %#v", preferenceResponse)
	}

	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	requestID := "sandbox-readiness-request"
	dataID := "123456789"
	signature := mercadoPagoSandboxSignature(config.WebhookSecret, timestamp, dataID, requestID)
	err = plugin.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
		Payload: []byte(fmt.Sprintf(`{"action":"payment.updated","api_version":"v1","data":{"id":"%s"},"type":"payment"}`, dataID)),
		Headers: map[string]string{
			"x-signature":  signature,
			"x-request-id": requestID,
		},
		Query: url.Values{"data.id": []string{dataID}},
		Config: map[string]interface{}{
			"webhook_secret": config.WebhookSecret,
		},
	})
	if err != nil {
		t.Fatalf("verify MercadoPago webhook signature with sandbox secret: %v", err)
	}
}

func mercadoPagoSandboxConfigFromEnv(t *testing.T) *MercadoPagoConfig {
	t.Helper()
	accessToken := strings.TrimSpace(os.Getenv("MERCADOPAGO_SANDBOX_ACCESS_TOKEN"))
	publicKey := strings.TrimSpace(os.Getenv("MERCADOPAGO_SANDBOX_PUBLIC_KEY"))
	webhookSecret := strings.TrimSpace(os.Getenv("MERCADOPAGO_SANDBOX_WEBHOOK_SECRET"))
	if webhookSecret == "" {
		webhookSecret = strings.TrimSpace(os.Getenv("MERCADOPAGO_WEBHOOK_SECRET"))
	}
	if accessToken == "" || publicKey == "" || webhookSecret == "" {
		t.Skip("set MERCADOPAGO_SANDBOX_ACCESS_TOKEN, MERCADOPAGO_SANDBOX_PUBLIC_KEY, and MERCADOPAGO_SANDBOX_WEBHOOK_SECRET to run MercadoPago sandbox integration tests")
	}
	baseURL := strings.TrimSpace(os.Getenv("PAYVERGE_PUBLIC_WEBHOOK_BASE_URL"))
	if baseURL == "" {
		t.Skip("set PAYVERGE_PUBLIC_WEBHOOK_BASE_URL to run MercadoPago sandbox integration tests")
	}
	return &MercadoPagoConfig{
		AccessToken:   accessToken,
		PublicKey:     publicKey,
		Environment:   "sandbox",
		BaseURL:       baseURL,
		WebhookSecret: webhookSecret,
	}
}

func mercadoPagoSandboxCurrency() string {
	currency := strings.ToUpper(strings.TrimSpace(os.Getenv("MERCADOPAGO_SANDBOX_CURRENCY")))
	if currency == "" {
		return "ARS"
	}
	return currency
}

func firstMercadoPagoCheckoutURL(response map[string]interface{}) string {
	for _, key := range []string{"sandbox_init_point", "init_point"} {
		if value := strings.TrimSpace(stringFromMercadoPagoAny(response[key])); value != "" {
			return value
		}
	}
	return ""
}

func mercadoPagoSandboxSignature(secret, timestamp, dataID, requestID string) string {
	manifest := fmt.Sprintf("id:%s;request-id:%s;ts:%s;", dataID, requestID, timestamp)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(manifest))
	return fmt.Sprintf("ts=%s,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
}
