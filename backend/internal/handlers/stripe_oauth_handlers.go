package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	appconfig "github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins/stripe"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// stripeOAuthState holds single-use OAuth start state (business + actor).
// Stripe Connect Standard OAuth does not use PKCE.
type stripeOAuthState struct {
	businessID uint
	userID     string
	expiresAt  time.Time
}

type stripeOAuthStateStore struct {
	mu     sync.Mutex
	states map[string]stripeOAuthState
}

func newStripeOAuthStateStore() *stripeOAuthStateStore {
	return &stripeOAuthStateStore{states: make(map[string]stripeOAuthState)}
}

func (s *stripeOAuthStateStore) Put(state string, entry stripeOAuthState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = entry
}

func (s *stripeOAuthStateStore) Consume(state string) (stripeOAuthState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.states[state]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(s.states, state)
		return stripeOAuthState{}, false
	}
	delete(s.states, state)
	return entry, true
}

// StripeOAuthHandlers serves OAuth start + callback for merchant connection.
type StripeOAuthHandlers struct {
	pluginService *services.PluginService
	oauthClient   *stripe.OAuthClient
	stateStore    *stripeOAuthStateStore
	// optional overrides for tests
	frontendBase func() string
	now          func() time.Time
}

// NewStripeOAuthHandlers constructs handlers. oauthClient may be nil when
// platform credentials are not configured (start returns 503).
func NewStripeOAuthHandlers(pluginService *services.PluginService, oauthClient *stripe.OAuthClient) *StripeOAuthHandlers {
	return &StripeOAuthHandlers{
		pluginService: pluginService,
		oauthClient:   oauthClient,
		stateStore:    newStripeOAuthStateStore(),
		now:           time.Now,
		frontendBase:  appconfig.FrontendBaseURL,
	}
}

// HandleOAuthStart POST /inside/businesses/:id/plugins/stripe/oauth/start
func (h *StripeOAuthHandlers) HandleOAuthStart(c *gin.Context) {
	if h == nil || h.oauthClient == nil {
		server.RespondWithError(c, http.StatusServiceUnavailable, server.ErrCodeInvalidInput, "Stripe OAuth is not configured")
		return
	}
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	userID := strings.TrimSpace(fmt.Sprint(c.GetString("user_id")))
	if userID == "" {
		if v, ok := c.Get("user_id"); ok {
			userID = strings.TrimSpace(fmt.Sprint(v))
		}
	}

	state := uuid.NewString()
	h.stateStore.Put(state, stripeOAuthState{
		businessID: businessID,
		userID:     userID,
		expiresAt:  h.now().Add(10 * time.Minute),
	})

	c.JSON(http.StatusOK, gin.H{
		"authorization_url": h.oauthClient.AuthorizationURL(state),
	})
}

// stripeOAuthConfigString reads a trimmed string out of a stored plugin config.
// fmt.Sprint on a missing key yields "<nil>", which would read as a real value.
func stripeOAuthConfigString(config map[string]interface{}, key string) string {
	raw, ok := config[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

// HandleOAuthCallback GET /api/v1/stripe/oauth/callback
func (h *StripeOAuthHandlers) HandleOAuthCallback(c *gin.Context) {
	// Land the operator back where the plugins UI actually lives. There is no
	// /business/plugins route: sending them there 404s, and the connect-result
	// toast — which renders inside the dashboard's PluginManager — never fires,
	// so a successful connection looks like a broken link. The business is only
	// known once the state is consumed, hence the captured variable.
	var redirectBusinessID uint
	redirect := func(status string, reason string) {
		path := "/"
		if redirectBusinessID > 0 {
			path = fmt.Sprintf("/business/%d/dashboard", redirectBusinessID)
		}
		base := strings.TrimRight(h.frontendBase(), "/")
		if base == "" {
			base = "http://localhost:3000"
		}
		u, err := url.Parse(base + path)
		if err != nil {
			c.Redirect(http.StatusFound, path+"?stripe_connect=error&reason=internal")
			return
		}
		q := u.Query()
		if redirectBusinessID > 0 {
			q.Set("tab", "plugins")
		}
		q.Set("stripe_connect", status)
		if reason != "" {
			q.Set("reason", reason)
		}
		u.RawQuery = q.Encode()
		c.Redirect(http.StatusFound, u.String())
	}

	if h == nil || h.oauthClient == nil {
		redirect("error", "not_configured")
		return
	}

	// User denied consent.
	if errParam := strings.TrimSpace(c.Query("error")); errParam != "" {
		redirect("error", "denied")
		return
	}

	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if code == "" || state == "" {
		redirect("error", "missing")
		return
	}
	entry, ok := h.stateStore.Consume(state)
	if !ok {
		redirect("error", "state")
		return
	}
	redirectBusinessID = entry.businessID

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	tokens, err := h.oauthClient.ExchangeCode(ctx, code)
	if err != nil {
		log.Printf("Stripe OAuth exchange failed for business %d: %v", entry.businessID, err)
		redirect("error", "exchange")
		return
	}

	// Production must not accept test/sandbox Connect (livemode=false).
	if !tokens.LiveMode {
		if appconfig.IsProductionMode(false) {
			log.Printf("Stripe OAuth: refusing test-mode (livemode=false) connect for business %d in production", entry.businessID)
			redirect("error", "sandbox_credentials")
			return
		}
	}

	// Merge into any existing business config (including disabled rows) so OAuth
	// reconnect does not wipe operator settings.
	cfg := map[string]interface{}{}
	existing, _, _, loadErr := database.GetBusinessPluginConfigState(entry.businessID, "stripe")
	if loadErr != nil {
		if !errors.Is(loadErr, database.ErrBusinessPluginNotEnabled) {
			log.Printf("Stripe OAuth: failed to load existing config for business %d: %v", entry.businessID, loadErr)
			redirect("error", "config_load")
			return
		}
	} else if existing != nil {
		for k, v := range existing {
			cfg[k] = v
		}
	}

	account := strings.TrimSpace(tokens.StripeUserID)
	previousMode := strings.ToLower(stripeOAuthConfigString(cfg, "connection_mode"))
	previousAccount := stripeOAuthConfigString(cfg, "stripe_user_id")
	previousStatus := strings.ToLower(stripeOAuthConfigString(cfg, "oauth_status"))
	wasOAuth := previousMode == "oauth" && previousAccount != ""

	// One acct_ id may only be claimed by one business. Connect delivers every
	// event for an account to the single platform endpoint, so two businesses
	// sharing an account is an unresolvable webhook ambiguity (409s at delivery
	// time) — refuse the grant here instead of half-connecting the merchant.
	if account != "" {
		others, err := database.FindBusinessIDsByStripeUserIDExcluding(account, entry.businessID)
		if err != nil {
			log.Printf("Stripe OAuth: account ownership check failed for business %d: %v", entry.businessID, err)
			redirect("error", "config_load")
			return
		}
		if len(others) > 0 {
			log.Printf("Stripe OAuth: refusing grant for business %d — account already connected to %v", entry.businessID, others)
			redirect("error", "account_in_use")
			return
		}
	}

	cfg["connection_mode"] = "oauth"
	cfg["stripe_user_id"] = account
	cfg["oauth_status"] = "connected"
	cfg["live_mode"] = tokens.LiveMode
	// Clear manual merchant credentials so we never mix modes. OAuth API calls
	// use the platform secret + Stripe-Account; Connect webhooks verify with
	// STRIPE_CONNECT_WEBHOOK_SECRET — a leftover per-account webhook_secret
	// from a prior manual setup would otherwise win verification and 401 all
	// Connect-signed events (manual→OAuth migration path).
	//
	// Only on that migration, though, and on a switch to a different account.
	// Reconnect/reauth of an already-connected merchant must keep whatever the
	// operator configured for this account — wiping webhook_secret on every
	// callback silently breaks any per-account Connect endpoint they added.
	if !wasOAuth || previousAccount != account {
		delete(cfg, "secret_key")
		delete(cfg, "webhook_secret")
		delete(cfg, "webhook_secret_previous")
	}
	// A deauthorization hides the card option from guests (config enabled=false)
	// while leaving the row enabled so the operator still sees the reconnect CTA.
	// The new grant is what undoes that; anything the operator turned off for
	// their own reasons stays off.
	if previousStatus == "reauth_required" || previousStatus == "disconnected" {
		cfg["enabled"] = true
	}

	if pk := strings.TrimSpace(tokens.StripePublishableKey); pk != "" {
		cfg["publishable_key"] = pk
	}
	// Optional: store OAuth access/refresh tokens encrypted-at-rest for
	// deauthorize/roll; primary API path uses platform key + Stripe-Account.
	if at := strings.TrimSpace(tokens.AccessToken); at != "" {
		cfg["access_token"] = at
	}
	if rt := strings.TrimSpace(tokens.RefreshToken); rt != "" {
		cfg["refresh_token"] = rt
	}

	plugin, err := database.GetPluginByName("stripe")
	if err != nil {
		log.Printf("Stripe OAuth: plugin catalog row missing: %v", err)
		redirect("error", "plugin")
		return
	}
	if err := database.EnableBusinessPlugin(entry.businessID, plugin.ID, cfg); err != nil {
		log.Printf("Stripe OAuth: failed to persist config for business %d: %v", entry.businessID, err)
		redirect("error", "persist")
		return
	}

	redirect("success", "")
}
