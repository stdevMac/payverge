package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	appconfig "github.com/stdevmac/payverge/backend/internal/config"
)

const (
	defaultStripeConnectAuthURL  = "https://connect.stripe.com"
	defaultStripeConnectTokenURL = "https://connect.stripe.com"
	oauthHTTPTimeout             = 15 * time.Second
)

// OAuthClient talks to Stripe Connect OAuth (authorize + token exchange).
// Standard Connect OAuth does not use PKCE; CSRF protection is the single-use
// state stored by the handlers.
type OAuthClient struct {
	ClientID     string // Connect client_id (ca_…)
	ClientSecret string // Platform secret key (sk_…) used as Basic auth for token
	RedirectURI  string
	BaseAuthURL  string // default https://connect.stripe.com
	BaseTokenURL string // default https://connect.stripe.com (tests override)
	HTTPClient   *http.Client
}

// OAuthTokens is the Stripe Connect token endpoint response.
// Primary field for the Stripe-Account auth model is StripeUserID (acct_…).
type OAuthTokens struct {
	AccessToken          string `json:"access_token"`
	RefreshToken         string `json:"refresh_token"`
	TokenType            string `json:"token_type"`
	Scope                string `json:"scope"`
	LiveMode             bool   `json:"livemode"`
	StripeUserID         string `json:"stripe_user_id"`
	StripePublishableKey string `json:"stripe_publishable_key"`
}

// NewOAuthClientFromEnv builds an OAuthClient from STRIPE_CONNECT_CLIENT_ID and
// STRIPE_SECRET_KEY. Redirect is APP_BASE_URL + /api/v1/stripe/oauth/callback.
func NewOAuthClientFromEnv() (*OAuthClient, error) {
	clientID := strings.TrimSpace(os.Getenv("STRIPE_CONNECT_CLIENT_ID"))
	// Platform secret used for token exchange and Stripe-Account API calls.
	clientSecret := strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY"))
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("stripe oauth: STRIPE_CONNECT_CLIENT_ID and STRIPE_SECRET_KEY are required")
	}
	base := strings.TrimRight(strings.TrimSpace(appconfig.APIBaseURL()), "/")
	return &OAuthClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  base + "/api/v1/stripe/oauth/callback",
		BaseAuthURL:  defaultStripeConnectAuthURL,
		BaseTokenURL: defaultStripeConnectTokenURL,
		HTTPClient:   &http.Client{Timeout: oauthHTTPTimeout},
	}, nil
}

// AuthorizationURL builds the Stripe Connect consent URL (no PKCE).
func (c *OAuthClient) AuthorizationURL(state string) string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseAuthURL), "/")
	if base == "" {
		base = defaultStripeConnectAuthURL
	}
	u, err := url.Parse(base + "/oauth/authorize")
	if err != nil {
		q := url.Values{}
		q.Set("response_type", "code")
		q.Set("client_id", c.ClientID)
		q.Set("scope", "read_write")
		q.Set("state", state)
		q.Set("redirect_uri", c.RedirectURI)
		return base + "/oauth/authorize?" + q.Encode()
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", c.ClientID)
	q.Set("scope", "read_write")
	q.Set("state", state)
	q.Set("redirect_uri", c.RedirectURI)
	u.RawQuery = q.Encode()
	return u.String()
}

// ExchangeCode swaps an authorization code for a Connect account link.
// Authenticates with the platform secret via HTTP Basic (Stripe docs).
func (c *OAuthClient) ExchangeCode(ctx context.Context, code string) (*OAuthTokens, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseTokenURL), "/")
	if base == "" {
		base = defaultStripeConnectTokenURL
	}
	endpoint := base + "/oauth/token"

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("stripe oauth: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	// Stripe: -u sk_platform: (empty password)
	req.SetBasicAuth(c.ClientSecret, "")

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: oauthHTTPTimeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe oauth: token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("stripe oauth: read token response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, oauthTokenError(resp.StatusCode, respBody)
	}

	var tokens OAuthTokens
	if err := json.Unmarshal(respBody, &tokens); err != nil {
		return nil, fmt.Errorf("stripe oauth: parse token response: %w", err)
	}
	if strings.TrimSpace(tokens.StripeUserID) == "" {
		return nil, fmt.Errorf("stripe oauth: token response missing stripe_user_id")
	}
	return &tokens, nil
}

// oauthTokenError builds a safe error from a non-2xx token response.
// Never includes secrets or token values — only status and public error fields.
func oauthTokenError(status int, body []byte) error {
	var parsed struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Message          string `json:"message"`
	}
	_ = json.Unmarshal(body, &parsed)

	parts := []string{fmt.Sprintf("stripe oauth: token endpoint returned %d", status)}
	if parsed.Error != "" {
		parts = append(parts, "error="+parsed.Error)
	}
	if parsed.Message != "" {
		parts = append(parts, "message="+parsed.Message)
	}
	if parsed.ErrorDescription != "" {
		parts = append(parts, "description="+parsed.ErrorDescription)
	}
	return fmt.Errorf("%s", strings.Join(parts, ": "))
}
