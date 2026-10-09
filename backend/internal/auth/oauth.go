package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	ErrOAuthStateMismatch = errors.New("OAuth state mismatch")
	ErrOAuthTokenExchange = errors.New("failed to exchange OAuth token")
	ErrOAuthUserInfo      = errors.New("failed to get user info from OAuth provider")
)

// OAuthConfig holds OAuth configuration for all providers
type OAuthConfig struct {
	Google *oauth2.Config
}

// NewOAuthConfig creates OAuth configurations from environment variables
func NewOAuthConfig() *OAuthConfig {
	baseURL := config.APIBaseURL()

	config := &OAuthConfig{}

	// Google OAuth
	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if googleClientID != "" && googleClientSecret != "" {
		config.Google = &oauth2.Config{
			ClientID:     googleClientID,
			ClientSecret: googleClientSecret,
			RedirectURL:  baseURL + "/api/v1/auth/google/callback",
			Scopes: []string{
				"https://www.googleapis.com/auth/userinfo.email",
				"https://www.googleapis.com/auth/userinfo.profile",
			},
			Endpoint: google.Endpoint,
		}
	}

	return config
}

// GetGoogleAuthURL returns the Google OAuth authorization URL
func (c *OAuthConfig) GetGoogleAuthURL(state string) string {
	if c.Google == nil {
		return ""
	}
	return c.Google.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// ExchangeGoogleCode exchanges a Google authorization code for tokens
func (c *OAuthConfig) ExchangeGoogleCode(ctx context.Context, code string) (*oauth2.Token, error) {
	if c.Google == nil {
		return nil, errors.New("google OAuth not configured")
	}
	token, err := c.Google.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOAuthTokenExchange, err)
	}
	return token, nil
}

// GetGoogleUserInfo fetches user info from Google
func GetGoogleUserInfo(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOAuthUserInfo, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d, body: %s", ErrOAuthUserInfo, resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseGoogleUserInfo(body)
}

// parseGoogleUserInfo decodes a Google v2 userinfo JSON payload. Kept separate
// from the HTTP fetch so the verified-email parsing contract can be tested
// without a live Google endpoint. The legacy v2 endpoint emits "verified_email"
// (matched by GoogleUserInfo's JSON tag) — if that tag ever drifts, the
// verified-email gate in the callback would silently fail closed.
func parseGoogleUserInfo(body []byte) (*GoogleUserInfo, error) {
	var userInfo GoogleUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, err
	}
	return &userInfo, nil
}

// GenerateOAuthState generates a random state for OAuth
func GenerateOAuthState() (string, error) {
	return GenerateToken(16)
}

// ValidateEmail performs basic email validation
func ValidateEmail(email string) bool {
	if email == "" {
		return false
	}
	// Basic validation - contains @ and has domain
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	if parts[0] == "" || parts[1] == "" {
		return false
	}
	if !strings.Contains(parts[1], ".") {
		return false
	}
	return true
}

// NormalizeEmail canonicalizes an email for identity purposes: lowercase +
// trim. Applied at every boundary (register, login, OAuth callback, resend,
// password reset, public checkout) so the same mailbox can never register
// twice and login is never case-exact (P2-2).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidatePassword validates password strength
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	// Add more validation as needed (uppercase, lowercase, numbers, special chars)
	return nil
}
