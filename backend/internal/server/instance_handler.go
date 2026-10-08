package server

// GET /api/v1/instance — public, unauthenticated instance identity + feature
// probe for self-hosted installs. The frontend reads it (SSR and client) to
// brand the UI and hide surfaces whose integrations are not configured, so
// the payload must stay small, cacheable and secret-free: it only ever
// carries display values and booleans, never keys, tokens, DSNs or hosts of
// third-party services.

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// instanceInfoCacheControl lets browsers/CDNs reuse the payload for a minute.
const instanceInfoCacheControl = "public, max-age=60"

// instanceInfoServerTTL bounds how long the encoded payload is reused
// in-process. Values come from env and boot-time wiring, so this only
// avoids re-encoding JSON on every request of a hot public route.
const instanceInfoServerTTL = 30 * time.Second

// InstanceInfo is the GET /api/v1/instance response.
type InstanceInfo struct {
	ProductName       string           `json:"product_name"`
	CompanyName       string           `json:"company_name"`
	LegalEntity       string           `json:"legal_entity"`
	LogoURL           string           `json:"logo_url"`
	BrandColor        string           `json:"brand_color"`
	PublicURL         string           `json:"public_url"`
	SupportEmail      string           `json:"support_email"`
	SecurityEmail     string           `json:"security_email"`
	LegalTermsURL     string           `json:"legal_terms_url"`
	LegalPrivacyURL   string           `json:"legal_privacy_url"`
	RegistrationMode  string           `json:"registration_mode"`
	EmailVerification string           `json:"email_verification"`
	Features          InstanceFeatures `json:"features"`
	Demo              InstanceDemo     `json:"demo"`
}

// InstanceFeatures reports which optional integrations this install has
// configured. Every flag is a boolean; no configuration value is exposed.
// docs/api/instance.md documents what each flag means.
//
// FiscalAR is a capability, not an activation: it is always true because the
// ARCA (AFIP) e-invoicing module has no build tag or instance switch, so every
// install can offer it. A venue invoices only after it opts in (fiscal
// settings plus its own ARCA certificate), and the UI shows ARCA surfaces only
// to businesses whose country is AR. It never means "this instance is in
// Argentina".
type InstanceFeatures struct {
	AI          bool `json:"ai"`
	WhatsApp    bool `json:"whatsapp"`
	Telegram    bool `json:"telegram"`
	Email       bool `json:"email"`
	GoogleOAuth bool `json:"google_oauth"`
	Crypto      bool `json:"crypto"` // false under DEMO_MODE (crypto routes are refused there)
	FiscalAR    bool `json:"fiscal_ar"`
}

// InstanceDemo reports whether the install was seeded with demo data.
type InstanceDemo struct {
	Enabled bool `json:"enabled"`
	// Mode is DEMO_MODE: a public demo anyone can enter (one-click demo
	// sign-in, outbound side effects off, nightly reset). The frontend shows
	// the demo banner and the demo sign-in buttons from it.
	Mode bool `json:"mode"`
	// ResetUTC is the displayed nightly reset time (HH:MM UTC), "" unless Mode.
	ResetUTC string `json:"reset_utc"`
}

// InstanceRuntime carries boot-time facts that are resolved from CLI flags
// as well as env (compose passes RPC_URL as --rpc-url), so the handler
// cannot recover them from os.Getenv alone. main.go sets it once after the
// integrations are wired; tests leave it unset and get the env fallback.
type InstanceRuntime struct {
	// EmailProvider is the resolved provider name (log|smtp|resend|postmark).
	EmailProvider string
	// CryptoEnabled is true when an RPC endpoint is configured.
	CryptoEnabled bool
	// TelegramEnabled is true when a Telegram bot token is configured.
	TelegramEnabled bool
}

var (
	instanceRuntime   atomic.Pointer[InstanceRuntime]
	instanceInfoCache atomic.Pointer[instanceInfoCacheEntry]
	instanceInfoNow   = time.Now
)

type instanceInfoCacheEntry struct {
	body    []byte
	expires time.Time
}

// SetInstanceRuntime records the boot-time integration facts and drops the
// cached payload.
func SetInstanceRuntime(rt InstanceRuntime) {
	rt.EmailProvider = strings.ToLower(strings.TrimSpace(rt.EmailProvider))
	instanceRuntime.Store(&rt)
	instanceInfoCache.Store(nil)
}

// resetInstanceInfoForTest clears runtime facts and the payload cache.
func resetInstanceInfoForTest() {
	instanceRuntime.Store(nil)
	instanceInfoCache.Store(nil)
}

// GetInstanceInfo is the public GET /api/v1/instance handler.
func GetInstanceInfo(c *gin.Context) {
	c.Header("Cache-Control", instanceInfoCacheControl)
	c.Data(http.StatusOK, "application/json; charset=utf-8", instanceInfoJSON())
}

func instanceInfoJSON() []byte {
	now := instanceInfoNow()
	if e := instanceInfoCache.Load(); e != nil && now.Before(e.expires) {
		return e.body
	}
	body, err := json.Marshal(BuildInstanceInfo())
	if err != nil {
		// Only strings and bools: unreachable, but never serve a 500 from a
		// public probe the whole UI depends on.
		return []byte(`{}`)
	}
	instanceInfoCache.Store(&instanceInfoCacheEntry{body: body, expires: now.Add(instanceInfoServerTTL)})
	return body
}

// BuildInstanceInfo assembles the instance payload from config + runtime.
func BuildInstanceInfo() InstanceInfo {
	rt := instanceRuntime.Load()

	emailProvider := config.EmailProvider()
	crypto := strings.TrimSpace(os.Getenv("RPC_URL")) != ""
	telegram := strings.TrimSpace(os.Getenv("TELEGRAM_TOKEN")) != "" ||
		strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")) != ""
	if rt != nil {
		if rt.EmailProvider != "" {
			emailProvider = rt.EmailProvider
		}
		crypto = rt.CryptoEnabled
		telegram = rt.TelegramEnabled
	}
	// The public demo refuses every crypto payment/quote route
	// (internal/demomode), so advertising the rail would only lead guests to a
	// 403. Report it off so the guest UI hides the crypto tender.
	if config.DemoModeEnabled() {
		crypto = false
	}

	return InstanceInfo{
		ProductName:       config.ProductName(),
		CompanyName:       config.CompanyName(),
		LegalEntity:       config.LegalEntity(),
		LogoURL:           config.LogoURL(),
		BrandColor:        config.BrandColor(),
		PublicURL:         config.PublicURL(),
		SupportEmail:      config.SupportEmail(),
		SecurityEmail:     config.SecurityEmail(),
		LegalTermsURL:     config.LegalTermsURL(),
		LegalPrivacyURL:   config.LegalPrivacyURL(),
		RegistrationMode:  string(config.RegistrationMode()),
		EmailVerification: string(config.EmailVerificationModeFor(emailProvider)),
		Features: InstanceFeatures{
			AI:          aiProviderConfigured(),
			WhatsApp:    services.WhatsAppAvailable(),
			Telegram:    telegram,
			Email:       emailProvider != config.EmailProviderLog,
			GoogleOAuth: strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")) != "" && strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")) != "",
			Crypto:      crypto,
			FiscalAR:    true, // capability, not activation (see InstanceFeatures)
		},
		Demo: instanceDemo(),
	}
}

func instanceDemo() InstanceDemo {
	d := InstanceDemo{Enabled: config.DemoDataEnabled(), Mode: config.DemoModeEnabled()}
	if d.Mode {
		d.ResetUTC = config.DemoResetUTC()
	}
	return d
}
