package paypal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	testClientID     = "client-id-12345678901234567890"
	testClientSecret = "client-secret-12345678901234567890"
)

// tokenServer returns an httptest server that responds to the OAuth token
// endpoint with the given status and body.
func tokenServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestGetPayPalAccessToken_ClassifiesAuthFailure verifies that a 401 from the
// token endpoint is classified as ErrPayPalAuthFailed (a definitive bad-credential
// signal) while other failures are generic errors and 200 yields a token.
func TestGetPayPalAccessToken_ClassifiesAuthFailure(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantToken   bool
		wantAuthErr bool
		wantErr     bool
	}{
		{"valid credentials", http.StatusOK, `{"access_token":"A-123"}`, true, false, false},
		{"invalid credentials 401", http.StatusUnauthorized, `{"error":"invalid_client"}`, false, true, true},
		{"transient server error 500", http.StatusInternalServerError, `{"error":"server_error"}`, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := tokenServer(t, tc.status, tc.body)
			pp := &PayPalPlugin{}
			tok, err := pp.getPayPalAccessToken(context.Background(), &PayPalConfig{
				ClientID:     testClientID,
				ClientSecret: testClientSecret,
				Environment:  "sandbox",
				APIBaseURL:   srv.URL,
			})
			if tc.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantAuthErr && !errors.Is(err, ErrPayPalAuthFailed) {
				t.Fatalf("expected ErrPayPalAuthFailed, got %v", err)
			}
			if !tc.wantAuthErr && errors.Is(err, ErrPayPalAuthFailed) {
				t.Fatalf("did not expect ErrPayPalAuthFailed, got %v", err)
			}
			if tc.wantToken && tok == "" {
				t.Fatalf("expected a token, got empty string")
			}
		})
	}
}

// TestValidatePayPalCredentials_RejectsInvalidCreds verifies the enable-path
// contract: invalid credentials hard-fail (so a misconfigured key cannot enable
// the plugin — matching the Stripe plugin), while transient failures are tolerated.
func TestValidatePayPalCredentials_RejectsInvalidCreds(t *testing.T) {
	pp := &PayPalPlugin{}

	// 401 → reject (bad credentials must not enable the plugin).
	srv401 := tokenServer(t, http.StatusUnauthorized, `{"error":"invalid_client"}`)
	if err := pp.validatePayPalCredentials(&PayPalConfig{
		ClientID: testClientID, ClientSecret: testClientSecret, Environment: "sandbox", APIBaseURL: srv401.URL,
	}); err == nil {
		t.Fatal("expected invalid credentials to be rejected, got nil")
	}

	// 500 → tolerate (transient outage should not block enabling).
	srv500 := tokenServer(t, http.StatusInternalServerError, `{"error":"server_error"}`)
	if err := pp.validatePayPalCredentials(&PayPalConfig{
		ClientID: testClientID, ClientSecret: testClientSecret, Environment: "sandbox", APIBaseURL: srv500.URL,
	}); err != nil {
		t.Fatalf("transient failure should be tolerated, got %v", err)
	}

	// 200 → pass.
	srv200 := tokenServer(t, http.StatusOK, `{"access_token":"A-123"}`)
	if err := pp.validatePayPalCredentials(&PayPalConfig{
		ClientID: testClientID, ClientSecret: testClientSecret, Environment: "sandbox", APIBaseURL: srv200.URL,
	}); err != nil {
		t.Fatalf("valid credentials should pass, got %v", err)
	}

	// Malformed credentials → reject before any network call.
	if err := pp.validatePayPalCredentials(&PayPalConfig{
		ClientID: "short", ClientSecret: testClientSecret, Environment: "sandbox", APIBaseURL: srv200.URL,
	}); err == nil {
		t.Fatal("expected malformed client ID to be rejected, got nil")
	}
}
