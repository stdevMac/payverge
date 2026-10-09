package stripe

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBuildStripeCheckoutMetadata_IncludesPaymentBreakdown(t *testing.T) {
	metadata := buildStripeCheckoutMetadata(7, 42, map[string]interface{}{
		"tip_amount_cents":  int64(350),
		"bill_amount_cents": "3000",
		"return_url":        "https://example.com/ignored",
	})

	if got := metadata["bill_id"]; got != "42" {
		t.Fatalf("expected bill_id 42, got %v", got)
	}
	if got := metadata["business_id"]; got != "7" {
		t.Fatalf("expected business_id 7, got %v", got)
	}
	if got := metadata["tip_amount_cents"]; got != "350" {
		t.Fatalf("expected tip_amount_cents 350, got %v", got)
	}
	if got := metadata["bill_amount_cents"]; got != "3000" {
		t.Fatalf("expected bill_amount_cents 3000, got %v", got)
	}
	if _, exists := metadata["return_url"]; exists {
		t.Fatalf("expected return_url to be excluded from Stripe metadata")
	}
}

func TestStripeCheckoutRedirectURLsRequireExplicitMetadata(t *testing.T) {
	_, _, err := stripeCheckoutRedirectURLs(nil)
	if err == nil {
		t.Fatal("expected missing URL metadata to fail")
	}

	successURL, cancelURL, err := stripeCheckoutRedirectURLs(map[string]interface{}{
		"return_url": " https://app.example/success ",
		"cancel_url": "https://app.example/cancel",
	})
	if err != nil {
		t.Fatalf("expected explicit URLs to pass, got %v", err)
	}
	if successURL != "https://app.example/success" {
		t.Fatalf("unexpected success URL %q", successURL)
	}
	if cancelURL != "https://app.example/cancel" {
		t.Fatalf("unexpected cancel URL %q", cancelURL)
	}
}

func TestVerifyWebhookSignature_Valid(t *testing.T) {
	plugin := &StripePlugin{}
	secret := "whsec_test_secret"
	payload := []byte(`{"id":"evt_test","type":"checkout.session.completed"}`)
	timestamp := time.Now().Unix()
	signature := buildStripeSignature(t, payload, secret, timestamp)

	if ok := plugin.VerifyWebhookSignature(payload, signature, secret); !ok {
		t.Fatalf("expected signature verification to pass")
	}
}

func TestVerifyWebhookSignature_InvalidSignature(t *testing.T) {
	plugin := &StripePlugin{}
	secret := "whsec_test_secret"
	payload := []byte(`{"id":"evt_test","type":"checkout.session.completed"}`)
	timestamp := time.Now().Unix()
	signature := fmt.Sprintf("t=%d,v1=%s", timestamp, "deadbeef")

	if ok := plugin.VerifyWebhookSignature(payload, signature, secret); ok {
		t.Fatalf("expected signature verification to fail")
	}
}

func TestVerifyWebhookSignature_StaleTimestamp(t *testing.T) {
	plugin := &StripePlugin{}
	secret := "whsec_test_secret"
	payload := []byte(`{"id":"evt_test","type":"checkout.session.completed"}`)
	timestamp := time.Now().Add(-10 * time.Minute).Unix()
	signature := buildStripeSignature(t, payload, secret, timestamp)

	if ok := plugin.VerifyWebhookSignature(payload, signature, secret); ok {
		t.Fatalf("expected stale timestamp verification to fail")
	}
}

func TestHandleCheckoutCompleted_ExtractsPaymentBreakdownFromMetadata(t *testing.T) {
	plugin := &StripePlugin{}
	response, err := plugin.handleCheckoutCompleted(1, map[string]interface{}{
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":             "cs_test_123",
				"payment_intent": "pi_test_123",
				"amount_total":   float64(3350),
				"currency":       "usd",
				"payment_status": "paid",
				"metadata": map[string]interface{}{
					"bill_id":           "42",
					"tip_amount_cents":  "350",
					"bill_amount_cents": "3000",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !response.Success {
		t.Fatalf("expected successful response")
	}
	if response.BillID != 42 {
		t.Fatalf("expected bill id 42, got %d", response.BillID)
	}
	if response.PaymentID != "pi_test_123" {
		t.Fatalf("expected payment-intent settlement id, got %q", response.PaymentID)
	}
	if response.Metadata["provider_tracker_id"] != "cs_test_123" {
		t.Fatalf("expected checkout-session tracker id, got %#v", response.Metadata)
	}
	if response.Amount != 3350 {
		t.Fatalf("expected amount 3350, got %d", response.Amount)
	}
	if response.Metadata["tip_amount_cents"] != int64(350) {
		t.Fatalf("expected tip metadata 350, got %v", response.Metadata["tip_amount_cents"])
	}
	if response.Metadata["bill_amount_cents"] != int64(3000) {
		t.Fatalf("expected bill metadata 3000, got %v", response.Metadata["bill_amount_cents"])
	}
}

func TestStripeHandleWebhook_LifecycleMappingsUsePaymentIntentIdentity(t *testing.T) {
	tests := []struct {
		eventType string
		status    string
	}{
		{eventType: "charge.refunded", status: "refunded"},
		{eventType: "charge.dispute.created", status: "disputed"},
		{eventType: "charge.dispute.funds_withdrawn", status: "reversed"},
	}
	for _, test := range tests {
		t.Run(test.eventType, func(t *testing.T) {
			payload := fmt.Sprintf(`{
				"type":%q,
				"data":{"object":{
					"id":"evt_object_1","payment_intent":"pi_contract_1",
					"amount":1000,"amount_refunded":1000,"currency":"usd",
					"metadata":{"bill_id":"42"}
				}}
			}`, test.eventType)
			response, err := (&StripePlugin{}).HandleWebhook(7, []byte(payload), nil)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if !response.Success || response.Status != test.status {
				t.Fatalf("unexpected lifecycle response: %#v", response)
			}
			if response.PaymentID != "pi_contract_1" || response.BillID != 42 {
				t.Fatalf("lifecycle binding lost: %#v", response)
			}
		})
	}
}

func TestStripeHandleWebhook_ExpiredKeepsCheckoutTrackerIdentity(t *testing.T) {
	response, err := (&StripePlugin{}).HandleWebhook(7, []byte(`{
		"type":"checkout.session.expired",
		"data":{"object":{"id":"cs_expired_1","payment_intent":"pi_unsettled_1"}}
	}`), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !response.Success || response.Status != "expired" || response.PaymentID != "cs_expired_1" {
		t.Fatalf("unexpected expiration response: %#v", response)
	}
}

// TestHandleCheckoutCompleted_ConvertsInboundMinorUnitsToCents locks H1: Stripe
// reports amount_total in the currency's provider minor unit, which is NOT the
// platform's ×100 "cents" for zero-decimal (JPY/KRW) or three-decimal (BHD)
// currencies. Storing it raw makes settlement fail forever with a breakdown
// mismatch. The inbound amount must be converted with money.FromMinorUnits.
func TestHandleCheckoutCompleted_ConvertsInboundMinorUnitsToCents(t *testing.T) {
	cases := []struct {
		name        string
		amountTotal float64
		currency    string
		wantCents   int64
	}{
		{"USD unchanged", 3350, "usd", 3350},
		{"JPY whole yen -> ×100 cents", 1000, "jpy", 100000},
		{"KRW whole won -> ×100 cents", 2500, "krw", 250000},
		{"BHD mils -> ÷10 cents", 1500, "bhd", 150},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &StripePlugin{}
			response, err := plugin.handleCheckoutCompleted(1, map[string]interface{}{
				"data": map[string]interface{}{
					"object": map[string]interface{}{
						"id":             "cs_test_" + tc.currency,
						"amount_total":   tc.amountTotal,
						"currency":       tc.currency,
						"payment_status": "paid",
						"metadata": map[string]interface{}{
							"bill_id": "42",
						},
					},
				},
			})
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if response.Amount != tc.wantCents {
				t.Fatalf("expected amount %d cents, got %d", tc.wantCents, response.Amount)
			}
			if response.Currency != strings.ToUpper(tc.currency) {
				t.Fatalf("expected currency %s, got %s", strings.ToUpper(tc.currency), response.Currency)
			}
		})
	}
}

// The tip/bill breakdown carried in metadata is written by us in cents (see
// buildStripeCheckoutMetadata) and echoed back verbatim by Stripe, so it must
// NOT be re-converted even for a zero-decimal currency.
func TestHandleCheckoutCompleted_MetadataCentsNotDoubleConverted(t *testing.T) {
	plugin := &StripePlugin{}
	response, err := plugin.handleCheckoutCompleted(1, map[string]interface{}{
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":             "cs_test_jpy",
				"amount_total":   float64(1000),
				"currency":       "jpy",
				"payment_status": "paid",
				"metadata": map[string]interface{}{
					"bill_id":           "42",
					"tip_amount_cents":  "10000",
					"bill_amount_cents": "90000",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if response.Metadata["tip_amount_cents"] != int64(10000) {
		t.Fatalf("expected tip metadata 10000 (unchanged), got %v", response.Metadata["tip_amount_cents"])
	}
	if response.Metadata["bill_amount_cents"] != int64(90000) {
		t.Fatalf("expected bill metadata 90000 (unchanged), got %v", response.Metadata["bill_amount_cents"])
	}
}

func TestHandleWebhook_MarksPaymentIntentSucceededUnsupportedForCheckoutOnlyPlugin(t *testing.T) {
	plugin := &StripePlugin{}
	response, err := plugin.HandleWebhook(1, []byte(`{
		"type":"payment_intent.succeeded",
		"data":{
			"object":{
				"id":"pi_test_123",
				"amount":3350,
				"currency":"usd",
				"metadata":{
					"bill_id":"42",
					"tip_amount_cents":"350",
					"bill_amount_cents":"3000"
				}
			}
		}
	}`), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !response.Success {
		t.Fatalf("expected unsupported event to be acknowledged")
	}
	if response.Status != "unsupported" {
		t.Fatalf("expected unsupported status, got %q", response.Status)
	}
	if !strings.Contains(response.Message, "unsupported") {
		t.Fatalf("expected unsupported message, got %q", response.Message)
	}
	if response.BillID != 0 || response.PaymentID != "" {
		t.Fatalf("expected no settlement identifiers, got bill=%d payment=%q", response.BillID, response.PaymentID)
	}
}

func buildStripeSignature(t *testing.T, payload []byte, secret string, timestamp int64) string {
	t.Helper()
	signedPayload := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	_, err := mac.Write([]byte(signedPayload))
	if err != nil {
		t.Fatalf("failed to write payload: %v", err)
	}
	digest := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%d,v1=%s", timestamp, digest)
}
