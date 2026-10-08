package stripe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// StripeAPIBaseURL is the Stripe API origin used by TestConnection. It is a
// package var (not a config field) so tests can point the read-only credential
// probe at a fake server without widening any request-controlled SSRF surface —
// operators never supply this value.
var StripeAPIBaseURL = "https://api.stripe.com"

// ErrStripeAuthFailed signals that Stripe rejected the stored secret key, so the
// caller can surface an "invalid credentials" message instead of a generic one.
var ErrStripeAuthFailed = errors.New("stripe authentication failed")

// TestConnection performs a cheap read-only GET /v1/account with the stored
// secret key to verify the credentials reach Stripe. It never echoes the key.
// A 401 maps to ErrStripeAuthFailed; other >=400 responses and transport errors
// return generic errors. The caller controls the timeout via ctx.
func TestConnection(ctx context.Context, config map[string]interface{}) error {
	secretKey, _ := config["secret_key"].(string)
	secretKey = strings.TrimSpace(secretKey)
	if secretKey == "" {
		return errors.New("stripe secret_key is not configured")
	}
	if !isValidStripeSecretKey(secretKey) {
		return ErrStripeAuthFailed
	}

	endpoint := strings.TrimRight(StripeAPIBaseURL, "/") + "/v1/account"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to build stripe request: %w", err)
	}
	req.SetBasicAuth(secretKey, "")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("stripe request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return ErrStripeAuthFailed
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("stripe API error %d", resp.StatusCode)
	}
	return nil
}
