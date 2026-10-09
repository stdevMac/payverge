package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSentryStoreEndpoint(t *testing.T) {
	endpoint, publicKey, err := sentryStoreEndpoint("https://public-key@o123.ingest.sentry.io/456")
	if err != nil {
		t.Fatalf("sentryStoreEndpoint returned error: %v", err)
	}
	if endpoint != "https://o123.ingest.sentry.io/api/456/store/" {
		t.Fatalf("endpoint = %q", endpoint)
	}
	if publicKey != "public-key" {
		t.Fatalf("public key = %q", publicKey)
	}
}

func TestSendSentrySynthetic(t *testing.T) {
	fixedTime := time.Date(2026, 8, 11, 3, 30, 0, 0, time.UTC)
	fixedRandom := strings.NewReader("0123456789abcdef")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/api/42/store/" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		auth := r.Header.Get("X-Sentry-Auth")
		if !strings.Contains(auth, "sentry_version=7") || !strings.Contains(auth, "sentry_key=public-key") {
			t.Errorf("X-Sentry-Auth = %q", auth)
		}

		var event syntheticEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if event.EventID != "30313233343536373839616263646566" {
			t.Errorf("event_id = %q", event.EventID)
		}
		if event.Environment != "production" || event.Tags["synthetic"] != "true" {
			t.Errorf("unexpected event metadata: %#v", event)
		}
		if event.Message != "Payverge production synthetic event" {
			t.Errorf("message = %q", event.Message)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"30313233343536373839616263646566"}`)
	}))
	defer server.Close()

	dsnURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	dsnURL.User = url.User("public-key")
	dsnURL.Path = "/42"

	eventID, err := sendSentrySynthetic(context.Background(), server.Client(), dsnURL.String(), fixedTime, fixedRandom)
	if err != nil {
		t.Fatalf("sendSentrySynthetic returned error: %v", err)
	}
	if eventID != "30313233343536373839616263646566" {
		t.Fatalf("event ID = %q", eventID)
	}
}

func TestSendSentrySyntheticRejectsProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rejected", http.StatusUnauthorized)
	}))
	defer server.Close()

	dsnURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	dsnURL.User = url.User("public-key")
	dsnURL.Path = "/42"

	_, err = sendSentrySynthetic(context.Background(), server.Client(), dsnURL.String(), time.Now(), strings.NewReader("0123456789abcdef"))
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("expected provider status error, got %v", err)
	}
}

func TestSentryStoreEndpointRejectsMalformedDSN(t *testing.T) {
	for _, dsn := range []string{"", "https://o123.ingest.sentry.io/456", "ftp://key@example.com/1", "https://key@example.com/"} {
		if _, _, err := sentryStoreEndpoint(dsn); err == nil {
			t.Errorf("expected %q to be rejected", dsn)
		}
	}
}
