package stripe

import (
	"encoding/json"
	"testing"
)

func sessionWebhook(eventType, paymentStatus string) map[string]interface{} {
	return map[string]interface{}{
		"type": eventType,
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":             "cs_test_123",
				"amount_total":   float64(1000),
				"currency":       "usd",
				"payment_status": paymentStatus,
				"metadata":       map[string]interface{}{"bill_id": "42"},
			},
		},
	}
}

func TestHandleWebhook_PaidSessionCompletes(t *testing.T) {
	s := &StripePlugin{}
	resp, err := s.HandleWebhook(1, mustJSON(t, sessionWebhook("checkout.session.completed", "paid")), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Status != "completed" {
		t.Fatalf("paid session: got status %q, want completed", resp.Status)
	}
}

func TestHandleWebhook_UnpaidSessionDoesNotComplete(t *testing.T) {
	s := &StripePlugin{}
	resp, err := s.HandleWebhook(1, mustJSON(t, sessionWebhook("checkout.session.completed", "unpaid")), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Status == "completed" {
		t.Fatalf("unpaid async session must NOT settle: got status %q", resp.Status)
	}
	if !resp.Success {
		t.Fatalf("unpaid session must be acked (Success:true) so Stripe does not retry-loop")
	}
}

func TestHandleWebhook_AsyncSucceededCompletes(t *testing.T) {
	s := &StripePlugin{}
	resp, _ := s.HandleWebhook(1, mustJSON(t, sessionWebhook("checkout.session.async_payment_succeeded", "paid")), nil)
	if resp.Status != "completed" {
		t.Fatalf("async_payment_succeeded: got %q, want completed", resp.Status)
	}
}

func TestHandleWebhook_AsyncFailedMarksFailed(t *testing.T) {
	s := &StripePlugin{}
	resp, _ := s.HandleWebhook(1, mustJSON(t, sessionWebhook("checkout.session.async_payment_failed", "unpaid")), nil)
	if resp.Status != "failed" {
		t.Fatalf("async_payment_failed: got %q, want failed", resp.Status)
	}
}

func mustJSON(t *testing.T, v map[string]interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
