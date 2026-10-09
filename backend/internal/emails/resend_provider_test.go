package emails

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// pointResendAt rewires a ResendProvider's underlying SDK client at the given
// test server so Send() never touches the real Resend API.
func pointResendAt(t *testing.T, p *ResendProvider, rawURL string) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test url: %v", err)
	}
	p.client.BaseURL = u
}

func TestResendProviderSendSuccess(t *testing.T) {
	var gotBody map[string]interface{}
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "email_abc123"})
	}))
	defer srv.Close()

	p := NewResendProvider("re_test_key")
	pointResendAt(t, p, srv.URL)

	result, err := p.SendWithReceipt(context.Background(), EmailMessage{
		From:        "noreply@payverge.io",
		To:          []string{"guest@example.com"},
		Subject:     "Order confirmed",
		HTMLBody:    "<p>Thanks</p>",
		TextBody:    "Thanks",
		ReplyTo:     "support@payverge.io",
		Tag:         "order_confirmation",
		MessageType: MessageTypeTransactional,
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if result.ProviderMessageID != "email_abc123" {
		t.Fatalf("ProviderMessageID = %q, want email_abc123", result.ProviderMessageID)
	}

	if gotAuth != "Bearer re_test_key" {
		t.Errorf("Authorization header = %q, want Bearer re_test_key", gotAuth)
	}
	if gotBody["from"] != "noreply@payverge.io" {
		t.Errorf("from = %v, want noreply@payverge.io", gotBody["from"])
	}
	if gotBody["subject"] != "Order confirmed" {
		t.Errorf("subject = %v, want Order confirmed", gotBody["subject"])
	}
	if gotBody["html"] != "<p>Thanks</p>" {
		t.Errorf("html = %v", gotBody["html"])
	}
	if gotBody["reply_to"] != "support@payverge.io" {
		t.Errorf("reply_to = %v", gotBody["reply_to"])
	}
	// "to" is serialized as a JSON array.
	to, ok := gotBody["to"].([]interface{})
	if !ok || len(to) != 1 || to[0] != "guest@example.com" {
		t.Errorf("to = %v, want [guest@example.com]", gotBody["to"])
	}
}

func TestResendProviderPropagatesIdempotencyKey(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	defer srv.Close()

	p := NewResendProvider("re_test_key")
	pointResendAt(t, p, srv.URL)
	require.NoError(t, p.Send(context.Background(), EmailMessage{
		From: "reports@example.test", To: []string{"owner@example.test"}, Subject: "Report",
		IdempotencyKey: "report:42:window",
	}))
	require.Equal(t, "report:42:window", got)
}

func TestResendProviderSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"statusCode": 422,
			"message":    "invalid from address",
			"name":       "validation_error",
		})
	}))
	defer srv.Close()

	p := NewResendProvider("re_test_key")
	pointResendAt(t, p, srv.URL)

	err := p.Send(context.Background(), EmailMessage{
		From:     "bad",
		To:       []string{"guest@example.com"},
		Subject:  "x",
		TextBody: "x",
	})
	if err == nil {
		t.Fatal("expected error on 422, got nil")
	}
}

func TestResendTagsForEncodesMessageTypeAndTag(t *testing.T) {
	tags := resendTagsFor("password reset!", MessageTypeBroadcast)

	want := map[string]string{
		"message_type": "broadcast",
		"tag":          "password_reset_",
	}
	got := make(map[string]string, len(tags))
	for _, tg := range tags {
		got[tg.Name] = tg.Value
	}
	for name, val := range want {
		if got[name] != val {
			t.Errorf("tag %q = %q, want %q", name, got[name], val)
		}
	}
}

func TestResendTagsForOmitsEmptyTag(t *testing.T) {
	tags := resendTagsFor("", MessageTypeTransactional)
	for _, tg := range tags {
		if tg.Name == "tag" {
			t.Errorf("expected no 'tag' entry for empty tag, got %q", tg.Value)
		}
	}
}

func TestSanitizeResendTagValue(t *testing.T) {
	cases := map[string]string{
		"order_confirmation": "order_confirmation",
		"password reset":     "password_reset",
		"payé!ment":          "pay__ment",
		"with-dash_9":        "with-dash_9",
		"":                   "",
	}
	for in, want := range cases {
		if got := sanitizeResendTagValue(in); got != want {
			t.Errorf("sanitizeResendTagValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewProviderResolvesResend(t *testing.T) {
	prov, err := NewProvider("resend", "re_test_key")
	if err != nil {
		t.Fatalf("NewProvider(resend) error: %v", err)
	}
	if _, ok := prov.(*ResendProvider); !ok {
		t.Fatalf("NewProvider(resend) = %T, want *ResendProvider", prov)
	}

	// Casing/whitespace tolerated, matching the postmark branch.
	if _, err := NewProvider("  Resend ", "re_test_key"); err != nil {
		t.Errorf("NewProvider with padded name errored: %v", err)
	}

	// Unknown provider still errors and now names resend in the message.
	_, err = NewProvider("sendgrid", "k")
	if err == nil || !strings.Contains(err.Error(), "resend") {
		t.Errorf("unknown provider error should list resend, got: %v", err)
	}
}
