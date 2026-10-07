package emails

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mattevans/postmark-go"
)

func TestPostmarkStreamFor(t *testing.T) {
	cases := map[MessageType]string{
		MessageTypeTransactional: "outbound",
		MessageTypeBroadcast:     "broadcast",
		MessageType(""):          "outbound",
	}
	for in, want := range cases {
		if got := postmarkStreamFor(in); got != want {
			t.Errorf("postmarkStreamFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPostmarkProviderReturnsErrorOnAPIError(t *testing.T) {
	// postmark-go's Do() runs CheckResponse() internally and wraps any non-2xx
	// status (here 422) into a non-nil error before returning. So this test
	// exercises the err-branch of PostmarkProvider.Send (lines ~40-46), not the
	// defensive resp.StatusCode != 200 branch below it. The assertion that the
	// error message contains "422" is still meaningful: it proves the API-error
	// path surfaces an actionable status code to callers.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{"Message": "invalid From"})
	}))
	defer srv.Close()

	client := postmark.NewClient(
		postmark.WithClient(&http.Client{Transport: &postmark.AuthTransport{Token: "test"}}),
		postmark.WithBackendURL(srv.URL),
	)
	p := &PostmarkProvider{client: client}

	err := p.Send(context.Background(), EmailMessage{
		From:        "test@payverge.io",
		To:          []string{"user@example.com"},
		Subject:     "x",
		HTMLBody:    "<p>x</p>",
		MessageType: MessageTypeTransactional,
	})
	if err == nil {
		t.Fatal("expected non-nil error on API error response")
	}
	if !strings.Contains(err.Error(), "422") {
		t.Errorf("expected status code in error message, got: %v", err)
	}
}

func TestPostmarkProviderPreservesIdempotencyKeyAsTraceHeader(t *testing.T) {
	var payload struct {
		Headers []struct {
			Name  string `json:"Name"`
			Value string `json:"Value"`
		} `json:"Headers"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ErrorCode": 0, "Message": "OK", "MessageID": "m1", "SubmittedAt": "2026-08-01T00:00:00Z", "To": "owner@example.test",
		})
	}))
	defer srv.Close()
	client := postmark.NewClient(
		postmark.WithClient(&http.Client{Transport: &postmark.AuthTransport{Token: "test"}}),
		postmark.WithBackendURL(srv.URL),
	)
	p := &PostmarkProvider{client: client}
	if err := p.Send(context.Background(), EmailMessage{
		From: "reports@example.test", To: []string{"owner@example.test"}, Subject: "Report",
		IdempotencyKey: "report:42:window",
	}); err != nil {
		t.Fatal(err)
	}
	if len(payload.Headers) != 1 || payload.Headers[0].Name != "X-Payverge-Idempotency-Key" || payload.Headers[0].Value != "report:42:window" {
		t.Fatalf("unexpected Postmark trace headers: %+v", payload.Headers)
	}
}

func TestNewProviderUnknownName(t *testing.T) {
	_, err := NewProvider("sendgrid", "key")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "sendgrid") {
		t.Errorf("expected provider name in error, got: %v", err)
	}
}

func TestNewProviderDefaultsToResend(t *testing.T) {
	provider, err := NewProvider("", "re_test_key")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := provider.(*ResendProvider); !ok {
		t.Fatalf("blank provider resolved to %T, want *ResendProvider", provider)
	}
}
