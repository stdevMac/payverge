package stripe

import (
	"context"
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
		ClientID:    "ca_test_client_id",
		RedirectURI: "https://api.example.com/api/v1/stripe/oauth/callback",
		BaseAuthURL: "https://connect.stripe.com",
	}

	state := "state-abc-123"
	raw := c.AuthorizationURL(state)
	require.NotEmpty(t, raw)

	u, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "https", u.Scheme)
	require.Equal(t, "connect.stripe.com", u.Host)
	require.Equal(t, "/oauth/authorize", u.Path)

	q := u.Query()
	require.Equal(t, "ca_test_client_id", q.Get("client_id"))
	require.Equal(t, "code", q.Get("response_type"))
	require.Equal(t, "read_write", q.Get("scope"))
	require.Equal(t, state, q.Get("state"))
	require.Equal(t, c.RedirectURI, q.Get("redirect_uri"))

	// No PKCE params for Stripe Connect OAuth.
	require.Empty(t, q.Get("code_challenge"))
	require.Empty(t, q.Get("code_challenge_method"))
	require.Empty(t, q.Get("code_verifier"))

	require.Contains(t, raw, "redirect_uri="+url.QueryEscape(c.RedirectURI))
	require.Contains(t, raw, "response_type=code")
	require.Contains(t, raw, "scope=read_write")
}

func TestExchangeCode(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotAuth string
	var gotCT string
	var gotBody url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")

		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		gotBody, err = url.ParseQuery(string(bodyBytes))
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token_type":             "bearer",
			"scope":                  "read_write",
			"livemode":               true,
			"stripe_user_id":         "acct_connected_123",
			"stripe_publishable_key": "pk_live_connected_pub",
			"access_token":           "sk_live_connected_access",
			"refresh_token":          "rt_connected_refresh",
		})
	}))
	defer srv.Close()

	c := &OAuthClient{
		ClientID:     "ca_platform",
		ClientSecret: "sk_live_platform_secret",
		RedirectURI:  "https://api.example.com/api/v1/stripe/oauth/callback",
		BaseTokenURL: srv.URL,
		HTTPClient:   srv.Client(),
	}

	tokens, err := c.ExchangeCode(context.Background(), "auth-code-xyz")
	require.NoError(t, err)
	require.NotNil(t, tokens)

	require.Equal(t, http.MethodPost, gotMethod)
	require.Equal(t, "/oauth/token", gotPath)
	require.True(t, strings.HasPrefix(gotCT, "application/x-www-form-urlencoded"))
	// Platform secret via HTTP Basic (password empty), per Stripe docs.
	require.True(t, strings.HasPrefix(gotAuth, "Basic "), "expected Basic auth with platform secret")
	require.Equal(t, "authorization_code", gotBody.Get("grant_type"))
	require.Equal(t, "auth-code-xyz", gotBody.Get("code"))
	// Stripe token exchange authenticates with the platform secret, not form client_secret.
	require.Empty(t, gotBody.Get("client_secret"))
	require.Empty(t, gotBody.Get("code_verifier"))

	require.Equal(t, "acct_connected_123", tokens.StripeUserID)
	require.True(t, tokens.LiveMode)
	require.Equal(t, "read_write", tokens.Scope)
	require.Equal(t, "pk_live_connected_pub", tokens.StripePublishableKey)
	require.Equal(t, "sk_live_connected_access", tokens.AccessToken)
	require.Equal(t, "rt_connected_refresh", tokens.RefreshToken)
}

func TestExchangeCode_ErrorMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":             "invalid_grant",
			"error_description": "Authorization code does not exist: ac_fake",
		})
	}))
	defer srv.Close()

	c := &OAuthClient{
		ClientID:     "ca_platform",
		ClientSecret: "sk_test_platform",
		BaseTokenURL: srv.URL,
		HTTPClient:   srv.Client(),
	}

	tokens, err := c.ExchangeCode(context.Background(), "bad-code")
	require.Nil(t, tokens)
	require.Error(t, err)
	msg := err.Error()
	require.Contains(t, msg, "400")
	require.Contains(t, msg, "invalid_grant")
	// Never leak platform secrets in errors.
	require.NotContains(t, msg, "sk_test_platform")
}

func TestExchangeCode_MissingStripeUserID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token_type": "bearer",
			"livemode":   true,
			// stripe_user_id omitted — fatal for Connect
		})
	}))
	defer srv.Close()

	c := &OAuthClient{
		ClientID:     "ca_platform",
		ClientSecret: "sk_test_platform",
		BaseTokenURL: srv.URL,
		HTTPClient:   srv.Client(),
	}

	tokens, err := c.ExchangeCode(context.Background(), "code")
	require.Nil(t, tokens)
	require.Error(t, err)
	require.Contains(t, err.Error(), "stripe_user_id")
}
