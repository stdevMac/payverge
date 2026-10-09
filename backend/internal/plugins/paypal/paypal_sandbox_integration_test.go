//go:build paypal_sandbox
// +build paypal_sandbox

package paypal

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestPayPalSandboxIntegration_CreateOrderAndValidateWebhookConfig(t *testing.T) {
	config := paypalSandboxConfigFromEnv(t)
	plugin := &PayPalPlugin{}

	orderResponse, err := plugin.makePayPalAPICall(context.Background(), http.MethodPost, "/v2/checkout/orders", map[string]interface{}{
		"intent": "CAPTURE",
		"purchase_units": []map[string]interface{}{
			{
				"reference_id": "bill_1",
				"description":  "Payverge sandbox bill payment readiness check",
				"amount": map[string]interface{}{
					"currency_code": "USD",
					"value":         "1.00",
				},
				"custom_id": "bill_1_business_1",
			},
		},
		"application_context": map[string]interface{}{
			"return_url":  "http://localhost:3000/payment/sandbox/success",
			"cancel_url":  "http://localhost:3000/payment/sandbox/cancel",
			"brand_name":  "Payverge Sandbox",
			"user_action": "PAY_NOW",
		},
	}, config)
	if err != nil {
		t.Fatalf("create sandbox PayPal order: %v", err)
	}

	orderID := strings.TrimSpace(stringFromPayPalAny(orderResponse["id"]))
	if orderID == "" {
		t.Fatalf("PayPal order response missing id: %#v", orderResponse)
	}
	if status := strings.ToUpper(stringFromPayPalAny(orderResponse["status"])); status != "CREATED" {
		t.Fatalf("expected created order status, got %q in %#v", status, orderResponse)
	}
	if !paypalResponseHasLink(orderResponse, "approve") {
		t.Fatalf("PayPal order response missing approve link: %#v", orderResponse)
	}

	webhookResponse, err := plugin.makePayPalAPICall(
		context.Background(),
		http.MethodGet,
		fmt.Sprintf("/v1/notifications/webhooks/%s", url.PathEscape(config.WebhookID)),
		nil,
		config,
	)
	if err != nil {
		t.Fatalf("get PayPal webhook configuration %s: %v", config.WebhookID, err)
	}
	if got := stringFromPayPalAny(webhookResponse["id"]); got != config.WebhookID {
		t.Fatalf("expected webhook id %q, got %q", config.WebhookID, got)
	}
	if webhookURL := strings.TrimSpace(stringFromPayPalAny(webhookResponse["url"])); webhookURL == "" {
		t.Fatalf("PayPal webhook configuration missing callback URL: %#v", webhookResponse)
	}

	configuredEvents := paypalWebhookEventNames(webhookResponse["event_types"])
	for _, requiredEvent := range []string{
		"CHECKOUT.ORDER.APPROVED",
		"PAYMENT.CAPTURE.COMPLETED",
		"PAYMENT.CAPTURE.DENIED",
		"PAYMENT.CAPTURE.REFUNDED",
	} {
		if !configuredEvents[requiredEvent] {
			t.Fatalf("PayPal webhook %s missing required event %s; configured=%v", config.WebhookID, requiredEvent, configuredEvents)
		}
	}
}

func paypalSandboxConfigFromEnv(t *testing.T) *PayPalConfig {
	t.Helper()
	clientID := strings.TrimSpace(os.Getenv("PAYPAL_SANDBOX_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("PAYPAL_SANDBOX_CLIENT_SECRET"))
	webhookID := strings.TrimSpace(os.Getenv("PAYPAL_SANDBOX_WEBHOOK_ID"))
	if clientID == "" || clientSecret == "" || webhookID == "" {
		t.Skip("set PAYPAL_SANDBOX_CLIENT_ID, PAYPAL_SANDBOX_CLIENT_SECRET, and PAYPAL_SANDBOX_WEBHOOK_ID to run PayPal sandbox integration tests")
	}
	return &PayPalConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Environment:  "sandbox",
		WebhookID:    webhookID,
	}
}

func paypalResponseHasLink(response map[string]interface{}, rel string) bool {
	links, _ := response["links"].([]interface{})
	for _, linkValue := range links {
		link, ok := linkValue.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.EqualFold(stringFromPayPalAny(link["rel"]), rel) && strings.TrimSpace(stringFromPayPalAny(link["href"])) != "" {
			return true
		}
	}
	return false
}

func paypalWebhookEventNames(value interface{}) map[string]bool {
	events := make(map[string]bool)
	eventTypes, _ := value.([]interface{})
	for _, eventValue := range eventTypes {
		switch event := eventValue.(type) {
		case string:
			events[strings.ToUpper(strings.TrimSpace(event))] = true
		case map[string]interface{}:
			name := strings.ToUpper(strings.TrimSpace(stringFromPayPalAny(event["name"])))
			if name != "" {
				events[name] = true
			}
		}
	}
	return events
}
