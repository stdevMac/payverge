package mercadopago

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorizationURL(t *testing.T) {
	c := &OAuthClient{
		ClientID:    "test-client-id",
		RedirectURI: "https://api.example.com/api/v1/mercadopago/oauth/callback",
		BaseAuthURL: "https://auth.mercadopago.com",
	}

	state := "state-abc-123"
	challenge := "challenge-xyz"

	raw := c.AuthorizationURL(state, challenge)
	require.NotEmpty(t, raw)

	u, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "https", u.Scheme)
	require.Equal(t, "auth.mercadopago.com", u.Host)
	require.Equal(t, "/authorization", u.Path)

	q := u.Query()
	require.Equal(t, "test-client-id", q.Get("client_id"))
	require.Equal(t, "code", q.Get("response_type"))
	require.Equal(t, "mp", q.Get("platform_id"))
	require.Equal(t, state, q.Get("state"))
	require.Equal(t, c.RedirectURI, q.Get("redirect_uri"))
	require.Equal(t, challenge, q.Get("code_challenge"))
	require.Equal(t, "S256", q.Get("code_challenge_method"))

	// redirect_uri must appear url-escaped in the raw query string
	require.Contains(t, raw, "redirect_uri="+url.QueryEscape(c.RedirectURI))
	require.Contains(t, raw, "client_id=test-client-id")
	require.Contains(t, raw, "response_type=code")
	require.Contains(t, raw, "platform_id=mp")
	require.Contains(t, raw, "code_challenge_method=S256")
}

func TestExchangeCode(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotCT string
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")

		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(bodyBytes, &gotBody))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-access-token",
			"refresh_token": "TG-refresh-token",
			"public_key":    "APP_USR-public-key",
			"expires_in":    15552000,
			"user_id":       123456789,
			"live_mode":     true,
			"scope":         "offline_access read write",
		})
	}))
	defer srv.Close()

	c := &OAuthClient{
		ClientID:     "mp-client-id",
		ClientSecret: "mp-client-secret",
		RedirectURI:  "https://api.example.com/api/v1/mercadopago/oauth/callback",
		BaseAPIURL:   srv.URL,
		HTTPClient:   srv.Client(),
	}

	tokens, err := c.ExchangeCode(context.Background(), "auth-code-xyz", "verifier-abc")
	require.NoError(t, err)
	require.NotNil(t, tokens)

	require.Equal(t, http.MethodPost, gotMethod)
	require.Equal(t, "/oauth/token", gotPath)
	require.True(t, strings.HasPrefix(gotCT, "application/json"))
	require.Equal(t, "mp-client-id", gotBody["client_id"])
	require.Equal(t, "mp-client-secret", gotBody["client_secret"])
	require.Equal(t, "authorization_code", gotBody["grant_type"])
	require.Equal(t, "auth-code-xyz", gotBody["code"])
	require.Equal(t, c.RedirectURI, gotBody["redirect_uri"])
	require.Equal(t, "verifier-abc", gotBody["code_verifier"])

	require.Equal(t, "APP_USR-access-token", tokens.AccessToken)
	require.Equal(t, "TG-refresh-token", tokens.RefreshToken)
	require.Equal(t, "APP_USR-public-key", tokens.PublicKey)
	require.Equal(t, int64(15552000), tokens.ExpiresIn)
	require.Equal(t, int64(123456789), tokens.UserID)
	require.True(t, tokens.LiveMode)
	require.Equal(t, "offline_access read write", tokens.Scope)
}

func TestRefresh(t *testing.T) {
	var gotBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/oauth/token", r.URL.Path)

		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(bodyBytes, &gotBody))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "APP_USR-new-access",
			"refresh_token": "TG-new-refresh",
			"public_key":    "APP_USR-public",
			"expires_in":    15552000,
			"user_id":       999,
			"live_mode":     false,
			"scope":         "offline_access",
		})
	}))
	defer srv.Close()

	c := &OAuthClient{
		ClientID:     "mp-client-id",
		ClientSecret: "mp-client-secret",
		BaseAPIURL:   srv.URL,
		HTTPClient:   srv.Client(),
	}

	tokens, err := c.Refresh(context.Background(), "old-refresh-token")
	require.NoError(t, err)
	require.NotNil(t, tokens)

	require.Equal(t, "mp-client-id", gotBody["client_id"])
	require.Equal(t, "mp-client-secret", gotBody["client_secret"])
	require.Equal(t, "refresh_token", gotBody["grant_type"])
	require.Equal(t, "old-refresh-token", gotBody["refresh_token"])
	_, hasCode := gotBody["code"]
	require.False(t, hasCode)
	_, hasVerifier := gotBody["code_verifier"]
	require.False(t, hasVerifier)

	require.Equal(t, "APP_USR-new-access", tokens.AccessToken)
	require.Equal(t, "TG-new-refresh", tokens.RefreshToken)
	require.Equal(t, int64(999), tokens.UserID)
	require.False(t, tokens.LiveMode)
}

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge := GeneratePKCE()

	require.GreaterOrEqual(t, len(verifier), 43)
	// Plan: 64-char verifier
	require.Equal(t, 64, len(verifier))

	sum := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(sum[:])
	require.Equal(t, expected, challenge)

	// No padding in base64url
	require.NotContains(t, challenge, "=")
	require.NotContains(t, verifier, "=")
}
