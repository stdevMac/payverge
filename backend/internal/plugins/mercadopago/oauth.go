package mercadopago

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
	defaultMPAuthURL = "https://auth.mercadopago.com"
	defaultMPAPIURL  = "https://api.mercadopago.com"
	oauthHTTPTimeout = 15 * time.Second
)

// OAuthClient talks to Mercado Pago OAuth (authorize + token exchange/refresh).
type OAuthClient struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	BaseAuthURL  string // default https://auth.mercadopago.com
	BaseAPIURL   string // default https://api.mercadopago.com (tests override)
	HTTPClient   *http.Client
}

// OAuthTokens is the Mercado Pago token endpoint response.
type OAuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	PublicKey    string `json:"public_key"`
	ExpiresIn    int64  `json:"expires_in"`
	UserID       int64  `json:"user_id"`
	LiveMode     bool   `json:"live_mode"`
	Scope        string `json:"scope"`
}

// NewOAuthClientFromEnv builds an OAuthClient from MERCADOPAGO_CLIENT_ID/SECRET
// and the platform callback at APP_BASE_URL + /api/v1/mercadopago/oauth/callback.
func NewOAuthClientFromEnv() (*OAuthClient, error) {
	clientID := strings.TrimSpace(os.Getenv("MERCADOPAGO_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("MERCADOPAGO_CLIENT_SECRET"))
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("mercadopago oauth: MERCADOPAGO_CLIENT_ID and MERCADOPAGO_CLIENT_SECRET are required")
	}
	base := strings.TrimRight(strings.TrimSpace(appconfig.APIBaseURL()), "/")
	return &OAuthClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  base + "/api/v1/mercadopago/oauth/callback",
		BaseAuthURL:  defaultMPAuthURL,
		BaseAPIURL:   defaultMPAPIURL,
		HTTPClient:   &http.Client{Timeout: oauthHTTPTimeout},
	}, nil
}

// AuthorizationURL builds the Mercado Pago consent URL (PKCE S256).
func (c *OAuthClient) AuthorizationURL(state, codeChallenge string) string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseAuthURL), "/")
	if base == "" {
		base = defaultMPAuthURL
	}
	u, err := url.Parse(base + "/authorization")
	if err != nil {
		// Fall back to a simple concatenation if BaseAuthURL is somehow invalid.
		q := url.Values{}
		q.Set("client_id", c.ClientID)
		q.Set("response_type", "code")
		q.Set("platform_id", "mp")
		q.Set("state", state)
		q.Set("redirect_uri", c.RedirectURI)
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
		return base + "/authorization?" + q.Encode()
	}
	q := u.Query()
	q.Set("client_id", c.ClientID)
	q.Set("response_type", "code")
	q.Set("platform_id", "mp")
	q.Set("state", state)
	q.Set("redirect_uri", c.RedirectURI)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String()
}

// ExchangeCode swaps an authorization code (+ optional PKCE verifier) for tokens.
func (c *OAuthClient) ExchangeCode(ctx context.Context, code, codeVerifier string) (*OAuthTokens, error) {
	body := map[string]string{
		"client_id":     c.ClientID,
		"client_secret": c.ClientSecret,
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  c.RedirectURI,
	}
	if codeVerifier != "" {
		body["code_verifier"] = codeVerifier
	}
	return c.postToken(ctx, body)
}

// Refresh exchanges a refresh token for a new access/refresh pair.
func (c *OAuthClient) Refresh(ctx context.Context, refreshToken string) (*OAuthTokens, error) {
	body := map[string]string{
		"client_id":     c.ClientID,
		"client_secret": c.ClientSecret,
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}
	return c.postToken(ctx, body)
}

// GeneratePKCE returns a 64-char code_verifier and its S256 code_challenge
// (base64url, no padding).
func GeneratePKCE() (verifier, challenge string) {
	// 48 random bytes → base64url without padding = 64 characters.
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		// crypto/rand failure is effectively impossible on a healthy host;
		// panic rather than mint a weak PKCE pair.
		panic(fmt.Sprintf("mercadopago oauth: crypto/rand failed: %v", err))
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge
}

func (c *OAuthClient) postToken(ctx context.Context, body map[string]string) (*OAuthTokens, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseAPIURL), "/")
	if base == "" {
		base = defaultMPAPIURL
	}
	endpoint := base + "/oauth/token"

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("mercadopago oauth: marshal token request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("mercadopago oauth: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: oauthHTTPTimeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mercadopago oauth: token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("mercadopago oauth: read token response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, oauthTokenError(resp.StatusCode, respBody)
	}

	var tokens OAuthTokens
	if err := json.Unmarshal(respBody, &tokens); err != nil {
		return nil, fmt.Errorf("mercadopago oauth: parse token response: %w", err)
	}
	if strings.TrimSpace(tokens.AccessToken) == "" {
		return nil, fmt.Errorf("mercadopago oauth: token response missing access_token")
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
		Status           int    `json:"status"`
	}
	_ = json.Unmarshal(body, &parsed)

	parts := []string{fmt.Sprintf("mercadopago oauth: token endpoint returned %d", status)}
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
