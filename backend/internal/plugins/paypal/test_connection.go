package paypal

import (
	"context"
	"strings"
)

// PayPalAPIBaseURLOverride, when non-empty, replaces the environment-derived
// PayPal API origin used by TestConnection. It is a package var (not a config
// field) so tests can point the read-only credential probe at a fake server
// without widening any request-controlled SSRF surface — operators never supply
// this value.
var PayPalAPIBaseURLOverride = ""

// TestConnection verifies the stored PayPal client credentials by fetching an
// OAuth access token (grant_type=client_credentials), a cheap read-only call.
// It never echoes the credentials. Rejected credentials surface as
// ErrPayPalAuthFailed (already distinguished by getPayPalAccessToken); other
// failures return generic errors. The caller controls the timeout via ctx.
func TestConnection(ctx context.Context, config map[string]interface{}) error {
	cfg, err := payPalConfigFromMap(config)
	if err != nil {
		return err
	}
	if override := strings.TrimSpace(PayPalAPIBaseURLOverride); override != "" {
		cfg.APIBaseURL = strings.TrimRight(override, "/")
	}
	pp := &PayPalPlugin{}
	if _, err := pp.getPayPalAccessToken(ctx, cfg); err != nil {
		return err
	}
	return nil
}
