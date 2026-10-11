package mercadopago

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// MercadoPagoAPIBaseURL is the MercadoPago API origin used by TestConnection.
// It is a package var (not a config field) so tests can point the read-only
// credential probe at a fake server without widening any request-controlled
// SSRF surface — operators never supply this value.
var MercadoPagoAPIBaseURL = "https://api.mercadopago.com"

// ErrMercadoPagoAuthFailed signals that MercadoPago rejected the stored access
// token, so the caller can surface an "invalid credentials" message.
var ErrMercadoPagoAuthFailed = errors.New("mercadopago authentication failed")

// TestConnection performs a cheap read-only GET /users/me with the stored
// access token to verify the credentials reach MercadoPago. It never echoes the
// token. 401/403 map to ErrMercadoPagoAuthFailed; other >=400 responses and
// transport errors return generic errors. The caller controls the timeout via ctx.
func TestConnection(ctx context.Context, config map[string]interface{}) error {
	accessToken, _ := config["access_token"].(string)
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return errors.New("mercadopago access_token is not configured")
	}

	// Prefer the configured api_base_url override (SSRF-validated at config save
	// time) so the probe hits the same host real API calls use; fall back to the
	// package default otherwise.
	baseURL := MercadoPagoAPIBaseURL
	if apiBaseURL, ok := config["api_base_url"].(string); ok && strings.TrimSpace(apiBaseURL) != "" {
		baseURL = strings.TrimSpace(apiBaseURL)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/users/me"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to build mercadopago request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("mercadopago request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrMercadoPagoAuthFailed
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("mercadopago API error %d", resp.StatusCode)
	}
	return nil
}
