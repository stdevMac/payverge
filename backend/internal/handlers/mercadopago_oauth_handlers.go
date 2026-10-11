package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	appconfig "github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// mpOAuthState holds single-use OAuth start state (business + PKCE verifier).
type mpOAuthState struct {
	businessID   uint
	userID       string
	codeVerifier string
	expiresAt    time.Time
}

type mpOAuthStateStore struct {
	mu     sync.Mutex
	states map[string]mpOAuthState
}

func newMPOAuthStateStore() *mpOAuthStateStore {
	return &mpOAuthStateStore{states: make(map[string]mpOAuthState)}
}

func (s *mpOAuthStateStore) Put(state string, entry mpOAuthState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = entry
}

func (s *mpOAuthStateStore) Consume(state string) (mpOAuthState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.states[state]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(s.states, state)
		return mpOAuthState{}, false
	}
	delete(s.states, state)
	return entry, true
}

// MercadoPagoOAuthHandlers serves OAuth start + callback for merchant connection.
type MercadoPagoOAuthHandlers struct {
	pluginService *services.PluginService
	oauthClient   *mercadopago.OAuthClient
	stateStore    *mpOAuthStateStore
	// optional overrides for tests
	usersMeFn    func(ctx context.Context, accessToken, apiBase string) (siteID string, userID int64, err error)
	frontendBase func() string
	now          func() time.Time
}

// NewMercadoPagoOAuthHandlers constructs handlers. oauthClient may be nil when
// platform credentials are not configured (start returns 503).
func NewMercadoPagoOAuthHandlers(pluginService *services.PluginService, oauthClient *mercadopago.OAuthClient) *MercadoPagoOAuthHandlers {
	return &MercadoPagoOAuthHandlers{
		pluginService: pluginService,
		oauthClient:   oauthClient,
		stateStore:    newMPOAuthStateStore(),
		now:           time.Now,
		frontendBase:  appconfig.FrontendBaseURL,
	}
}

// HandleOAuthStart POST /inside/businesses/:id/plugins/mercadopago/oauth/start
func (h *MercadoPagoOAuthHandlers) HandleOAuthStart(c *gin.Context) {
	if h == nil || h.oauthClient == nil {
		server.RespondWithError(c, http.StatusServiceUnavailable, server.ErrCodeInvalidInput, "Mercado Pago OAuth is not configured")
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

	verifier, challenge := mercadopago.GeneratePKCE()
	state := uuid.NewString()
	h.stateStore.Put(state, mpOAuthState{
		businessID:   businessID,
		userID:       userID,
		codeVerifier: verifier,
		expiresAt:    h.now().Add(10 * time.Minute),
	})

	c.JSON(http.StatusOK, gin.H{
		"authorization_url": h.oauthClient.AuthorizationURL(state, challenge),
	})
}

// HandleOAuthCallback GET /api/v1/mercadopago/oauth/callback
func (h *MercadoPagoOAuthHandlers) HandleOAuthCallback(c *gin.Context) {
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
			c.Redirect(http.StatusFound, path+"?mp_connect=error&reason=internal")
			return
		}
		q := u.Query()
		if redirectBusinessID > 0 {
			q.Set("tab", "plugins")
		}
		q.Set("mp_connect", status)
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

	tokens, err := h.oauthClient.ExchangeCode(ctx, code, entry.codeVerifier)
	if err != nil {
		log.Printf("MercadoPago OAuth exchange failed for business %d: %v", entry.businessID, err)
		redirect("error", "exchange")
		return
	}

	apiBase := strings.TrimRight(strings.TrimSpace(h.oauthClient.BaseAPIURL), "/")
	if apiBase == "" {
		apiBase = "https://api.mercadopago.com"
	}
	// Seed user id from the token response. Only adopt /users/me when it
	// succeeds with a positive id — fetch returns 0 on every error path, and
	// writing mp_user_id="0" poisons ensureStoreAndPOS (/users/0/stores) and
	// falsely triggers the account-change store/POS wipe on reconnect.
	siteID := ""
	userID := tokens.UserID
	var meSite string
	var meUserID int64
	if h.usersMeFn != nil {
		meSite, meUserID, err = h.usersMeFn(ctx, tokens.AccessToken, apiBase)
	} else {
		meSite, meUserID, err = fetchMercadoPagoUsersMe(ctx, tokens.AccessToken, apiBase)
	}
	if err != nil {
		log.Printf("MercadoPago OAuth /users/me failed for business %d: %v", entry.businessID, err)
		// Keep tokens.UserID; country can be filled later.
	} else {
		siteID = meSite
		if meUserID > 0 {
			userID = meUserID
		}
	}

	expiresAt := h.now().UTC().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	if tokens.ExpiresIn <= 0 {
		expiresAt = h.now().UTC().Add(180 * 24 * time.Hour)
	}

	// Merge into any existing business config (including disabled rows) so OAuth
	// reconnect does not wipe operator settings. GetBusinessPluginConfig filters
	// is_enabled=true and would miss a toggled-off plugin — use ConfigState.
	// Distinguish "no config row" (fresh connect) from load/decrypt/DB errors:
	// the latter must abort without persisting (never fall through to wipe).
	cfg := map[string]interface{}{}
	existing, _, _, loadErr := database.GetBusinessPluginConfigState(entry.businessID, "mercadopago")
	if loadErr != nil {
		if !errors.Is(loadErr, database.ErrBusinessPluginNotEnabled) {
			log.Printf("MercadoPago OAuth: failed to load existing config for business %d: %v", entry.businessID, loadErr)
			redirect("error", "config_load")
			return
		}
		// No prior row — proceed with a fresh config map.
	} else if existing != nil {
		for k, v := range existing {
			cfg[k] = v
		}
	}
	// Derive environment from the token response live_mode. Never hard-code
	// production — a sandbox OAuth app would otherwise settle real bills.
	if !tokens.LiveMode {
		if appconfig.IsProductionMode(false) {
			log.Printf("MercadoPago OAuth: refusing sandbox (live_mode=false) connect for business %d in production", entry.businessID)
			redirect("error", "sandbox_credentials")
			return
		}
		cfg["environment"] = "sandbox"
	} else {
		cfg["environment"] = "production"
	}
	cfg["connection_mode"] = "oauth"
	cfg["access_token"] = tokens.AccessToken
	cfg["refresh_token"] = tokens.RefreshToken
	cfg["token_expires_at"] = expiresAt.Format(time.RFC3339)
	// Only persist mp_user_id when we have a positive resolved id. Leaving it
	// unset (when both token and /users/me yield 0) lets a later re-resolve
	// fill it without ever writing the poison value "0".
	if userID > 0 {
		newMPUserID := strconv.FormatInt(userID, 10)
		// Account change: clear store/POS ids so EnsureInstore re-provisions against
		// the new MP user (instore short-circuits on mp_external_pos_id).
		if prev := strings.TrimSpace(fmt.Sprint(cfg["mp_user_id"])); prev != "" && prev != "<nil>" && prev != newMPUserID {
			delete(cfg, "mp_store_id")
			delete(cfg, "mp_external_pos_id")
			delete(cfg, "internal_pos_id")
		}
		cfg["mp_user_id"] = newMPUserID
	}
	cfg["live_mode"] = tokens.LiveMode
	cfg["oauth_status"] = "connected"
	// When the token omits public_key, clear any stored key so a previous MP
	// account's key cannot leak into this connect.
	if strings.TrimSpace(tokens.PublicKey) != "" {
		cfg["public_key"] = tokens.PublicKey
	} else {
		delete(cfg, "public_key")
	}
	// Only set country from /users/me when present; otherwise keep existing.
	if country := siteIDToCountry(siteID); country != "" {
		cfg["country"] = country
	}

	plugin, err := database.GetPluginByName("mercadopago")
	if err != nil {
		log.Printf("MercadoPago OAuth: plugin catalog row missing: %v", err)
		redirect("error", "plugin")
		return
	}
	// EnableBusinessPlugin encrypts secrets, upserts the row, and re-enables a
	// previously disabled plugin. (MergeBusinessPluginConfigFields cannot encrypt
	// credential fields; secrets must go through EnableBusinessPlugin.)
	if err := database.EnableBusinessPlugin(entry.businessID, plugin.ID, cfg); err != nil {
		log.Printf("MercadoPago OAuth: failed to persist config for business %d: %v", entry.businessID, err)
		redirect("error", "persist")
		return
	}

	redirect("success", "")
}

func siteIDToCountry(siteID string) string {
	switch strings.ToUpper(strings.TrimSpace(siteID)) {
	case "MLA":
		return "AR"
	case "MLB":
		return "BR"
	case "MLC":
		return "CL"
	case "MCO":
		return "CO"
	case "MLM":
		return "MX"
	case "MPE":
		return "PE"
	case "MLU":
		return "UY"
	default:
		return ""
	}
}

func fetchMercadoPagoUsersMe(ctx context.Context, accessToken, apiBase string) (siteID string, userID int64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiBase, "/")+"/users/me", nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("users/me status %d", resp.StatusCode)
	}
	var body struct {
		ID     int64  `json:"id"`
		SiteID string `json:"site_id"`
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return "", 0, err
	}
	return body.SiteID, body.ID, nil
}
