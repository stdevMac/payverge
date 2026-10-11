package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// instanceEnvKeys is every env var the /instance payload can read, cleared
// so the host environment never leaks into these assertions.
var instanceEnvKeys = []string{
	"PUBLIC_URL", "FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL",
	"PRODUCT_NAME", "COMPANY_NAME", "LEGAL_ENTITY", "SUPPORT_EMAIL", "SECURITY_EMAIL",
	"LOGO_URL", "BRAND_COLOR", "LEGAL_TERMS_URL", "LEGAL_PRIVACY_URL",
	"REGISTRATION_MODE", "EMAIL_PROVIDER", "EMAIL_VERIFICATION", "EMAIL_API_KEY",
	"RESEND_API_KEY", "SMTP_HOST", "SMTP_USERNAME",
	"SMTP_PASSWORD", "RPC_URL", "TELEGRAM_TOKEN",
	"TELEGRAM_BOT_TOKEN", "GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET",
	"WHATSAPP_ENABLED", "DEMO_DATA",
	"OPENROUTER_API_KEY", "LLM_API_KEY", "LLM_BASE_URL",
}

func clearInstanceTestEnv(t testing.TB) {
	t.Helper()
	for _, k := range instanceEnvKeys {
		t.Setenv(k, "")
	}
	resetInstanceInfoForTest()
	t.Cleanup(resetInstanceInfoForTest)
}

func getInstance(t testing.TB) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/instance", GetInstanceInfo)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil))
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	return w, body
}

func TestInstanceInfoDefaults(t *testing.T) {
	clearInstanceTestEnv(t)
	// RegistrationMode is cached process-wide; pin the default explicitly.
	config.SetRegistrationModeForTesting(t, config.DefaultRegistrationMode)
	SetAIService(nil)

	w, body := getInstance(t)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=60" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q", ct)
	}

	want := map[string]any{
		"product_name":       "Payverge",
		"company_name":       "Payverge",
		"legal_entity":       "",
		"logo_url":           "",
		"brand_color":        "#1a6b6a",
		"public_url":         "http://localhost:3000",
		"support_email":      "",
		"security_email":     "",
		"legal_terms_url":    "",
		"legal_privacy_url":  "",
		"registration_mode":  "invite",
		"email_verification": "off",
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %#v, want %#v", k, body[k], v)
		}
	}
	if _, ok := body["billing_mode"]; ok {
		t.Errorf("billing_mode must not be served: %#v", body["billing_mode"])
	}
	features, ok := body["features"].(map[string]any)
	if !ok {
		t.Fatalf("features missing: %#v", body)
	}
	wantFeatures := map[string]bool{
		"ai": false, "whatsapp": false, "telegram": false, "email": false,
		"google_oauth": false, "crypto": false, "fiscal_ar": true,
	}
	if len(features) != len(wantFeatures) {
		t.Errorf("features has %d keys, want %d: %#v", len(features), len(wantFeatures), features)
	}
	for k, v := range wantFeatures {
		if features[k] != v {
			t.Errorf("features.%s = %#v, want %v", k, features[k], v)
		}
	}
	demo, _ := body["demo"].(map[string]any)
	if demo == nil || demo["enabled"] != false {
		t.Errorf("demo = %#v, want enabled=false", body["demo"])
	}
}

func TestInstanceInfoConfigured(t *testing.T) {
	clearInstanceTestEnv(t)
	t.Setenv("PUBLIC_URL", "https://Eat.Example.com/")
	t.Setenv("PRODUCT_NAME", "Tavola")
	t.Setenv("COMPANY_NAME", "Tavola Co")
	t.Setenv("LEGAL_ENTITY", "Tavola Co S.R.L.")
	t.Setenv("SUPPORT_EMAIL", "help@eat.example.com")
	t.Setenv("SECURITY_EMAIL", "sec@eat.example.com")
	t.Setenv("LOGO_URL", "/media/logo.svg")
	t.Setenv("BRAND_COLOR", "AA3300")
	t.Setenv("LEGAL_TERMS_URL", "https://eat.example.com/legal/terms")
	// Relative values are dropped: the frontend redirects /privacy to this URL.
	t.Setenv("LEGAL_PRIVACY_URL", "/privacy")
	config.SetRegistrationModeForTesting(t, config.RegistrationModeOpen)
	t.Setenv("EMAIL_PROVIDER", "smtp")
	t.Setenv("GOOGLE_CLIENT_ID", "gid")
	t.Setenv("GOOGLE_CLIENT_SECRET", "gsecret")
	t.Setenv("WHATSAPP_ENABLED", "true")
	t.Setenv("DEMO_DATA", "true")
	SetInstanceRuntime(InstanceRuntime{EmailProvider: "SMTP", CryptoEnabled: true, TelegramEnabled: true})

	_, body := getInstance(t)
	want := map[string]any{
		"product_name":       "Tavola",
		"company_name":       "Tavola Co",
		"legal_entity":       "Tavola Co S.R.L.",
		"logo_url":           "/media/logo.svg",
		"brand_color":        "#aa3300",
		"public_url":         "https://eat.example.com",
		"support_email":      "help@eat.example.com",
		"security_email":     "sec@eat.example.com",
		"legal_terms_url":    "https://eat.example.com/legal/terms",
		"legal_privacy_url":  "",
		"registration_mode":  "open",
		"email_verification": "required",
	}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %#v, want %#v", k, body[k], v)
		}
	}
	features := body["features"].(map[string]any)
	wantFeatures := map[string]bool{
		"telegram": true, "email": true, "google_oauth": true, "crypto": true,
		"fiscal_ar": true,
		// WhatsApp needs the whatsmeow build tag as well as the env switch.
		"whatsapp": services.WhatsAppBuilt,
	}
	for k, v := range wantFeatures {
		if features[k] != v {
			t.Errorf("features.%s = %#v, want %v", k, features[k], v)
		}
	}
	if demo := body["demo"].(map[string]any); demo["enabled"] != true {
		t.Errorf("demo.enabled = %#v, want true", demo["enabled"])
	}
}

func TestInstanceInfoEnvFallbacksWithoutRuntime(t *testing.T) {
	clearInstanceTestEnv(t)
	t.Setenv("RPC_URL", "https://rpc.example")
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("RESEND_API_KEY", "re_x")

	info := BuildInstanceInfo()
	if !info.Features.Crypto || !info.Features.Telegram || !info.Features.Email {
		t.Fatalf("env fallbacks not honoured: %+v", info.Features)
	}
	if info.EmailVerification != "required" {
		t.Fatalf("email_verification = %q, want required with a real provider", info.EmailVerification)
	}

	t.Setenv("EMAIL_VERIFICATION", "off")
	if got := BuildInstanceInfo().EmailVerification; got != "off" {
		t.Fatalf("EMAIL_VERIFICATION=off ignored: %q", got)
	}
	t.Setenv("EMAIL_VERIFICATION", "required")
	t.Setenv("RESEND_API_KEY", "")
	if got := BuildInstanceInfo().EmailVerification; got != "required" {
		t.Fatalf("EMAIL_VERIFICATION=required ignored with log provider: %q", got)
	}
	// An unknown REGISTRATION_MODE fails closed (config.ParseRegistrationMode);
	// the payload reports whatever the shared accessor resolved.
	config.SetRegistrationModeForTesting(t, config.RegistrationModeClosed)
	if got := BuildInstanceInfo().RegistrationMode; got != "closed" {
		t.Fatalf("registration_mode = %q, want closed", got)
	}
	t.Setenv("EMAIL_PROVIDER", "postmark")
	t.Setenv("EMAIL_API_KEY", "pm")
	t.Setenv("EMAIL_VERIFICATION", "")
	info = BuildInstanceInfo()
	if !info.Features.Email || info.EmailVerification != "required" {
		t.Fatalf("postmark with EMAIL_API_KEY should enable email + required verification: %+v", info)
	}
}

func TestInstanceInfoNeverExposesSecrets(t *testing.T) {
	clearInstanceTestEnv(t)
	secrets := map[string]string{
		"OPENROUTER_API_KEY":   "sk-or-v1-SECRETopenrouter",
		"LLM_API_KEY":          "sk-llm-SECRETkey",
		"LLM_BASE_URL":         "http://ollama-SECRET-host:11434/v1",
		"RESEND_API_KEY":       "re_SECRETresend",
		"EMAIL_API_KEY":        "SECRETemailkey",
		"GOOGLE_CLIENT_ID":     "SECRETgoogleid.apps.googleusercontent.com",
		"GOOGLE_CLIENT_SECRET": "SECRETgooglesecret",
		"TELEGRAM_TOKEN":       "123456:SECRETtelegram",
		"RPC_URL":              "https://base-SECRET.example/rpc/key-SECRET",
		"JWT_SECRET_KEY":       "SECRETjwt",
		"PLUGIN_SECRET_KEY":    "SECRETplugin-key-000000000000000",
		"STRIPE_SECRET_KEY":    "sk_live_SECRETstripe",
		"DATABASE_URL":         "postgres://u:SECRETpw@db:5432/payverge",
		"DB_PASSWORD":          "SECRETdbpw",
		"METRICS_TOKEN":        "SECRETmetrics",
	}
	for k, v := range secrets {
		t.Setenv(k, v)
	}

	w, _ := getInstance(t)
	raw := w.Body.String()
	if strings.Contains(raw, "SECRET") {
		t.Fatalf("instance payload leaks a secret: %s", raw)
	}
	for k, v := range secrets {
		if strings.Contains(raw, v) {
			t.Errorf("payload contains %s value", k)
		}
	}
	// Every features value must be a bool — no config value can slip in.
	var body struct {
		Features map[string]any `json:"features"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for k, v := range body.Features {
		if _, ok := v.(bool); !ok {
			t.Errorf("features.%s is %T, want bool", k, v)
		}
	}
}

func TestInstanceInfoServerCacheExpires(t *testing.T) {
	clearInstanceTestEnv(t)
	now := time.Unix(1_800_000_000, 0)
	prevNow := instanceInfoNow
	instanceInfoNow = func() time.Time { return now }
	t.Cleanup(func() { instanceInfoNow = prevNow })

	t.Setenv("PRODUCT_NAME", "First")
	if _, body := getInstance(t); body["product_name"] != "First" {
		t.Fatalf("product_name = %#v", body["product_name"])
	}
	t.Setenv("PRODUCT_NAME", "Second")
	if _, body := getInstance(t); body["product_name"] != "First" {
		t.Fatalf("payload should be served from cache within TTL, got %#v", body["product_name"])
	}
	now = now.Add(instanceInfoServerTTL + time.Second)
	if _, body := getInstance(t); body["product_name"] != "Second" {
		t.Fatalf("payload should refresh after TTL, got %#v", body["product_name"])
	}
	// SetInstanceRuntime invalidates immediately.
	t.Setenv("PRODUCT_NAME", "Third")
	SetInstanceRuntime(InstanceRuntime{EmailProvider: "log"})
	if _, body := getInstance(t); body["product_name"] != "Third" {
		t.Fatalf("SetInstanceRuntime should drop the cache, got %#v", body["product_name"])
	}
}

// BenchmarkInstanceInfoHandler measures the hot public route (served from the
// in-process cache) and the uncached build path.
func BenchmarkInstanceInfoHandler(b *testing.B) {
	clearInstanceTestEnv(b)
	b.Setenv("PUBLIC_URL", "https://eat.example.com")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/instance", GetInstanceInfo)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance", nil)

	b.Run("cached", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})
	b.Run("uncached", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			instanceInfoCache.Store(nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}
	})
}

// TestInstanceInfoDemoModeHidesCrypto: DEMO_MODE refuses the guest crypto
// payment and quote routes, so /instance must not advertise the rail even when
// an RPC endpoint is configured.
func TestInstanceInfoDemoModeHidesCrypto(t *testing.T) {
	clearInstanceTestEnv(t)
	SetInstanceRuntime(InstanceRuntime{CryptoEnabled: true})

	config.SetDemoModeForTesting(t, false)
	if !BuildInstanceInfo().Features.Crypto {
		t.Fatal("features.crypto = false outside demo mode, want true with an RPC endpoint")
	}

	config.SetDemoModeForTesting(t, true)
	info := BuildInstanceInfo()
	if info.Features.Crypto {
		t.Fatal("features.crypto = true under DEMO_MODE, want false (crypto routes are refused)")
	}
	if !info.Demo.Mode {
		t.Fatal("demo.mode = false, want true")
	}
}

// TestInstanceInfoDemoModeHidesFiscalAR: the demo guard refuses every fiscal
// (ARCA) write, so /instance reports fiscal_ar=false there and the dashboard
// hides the fiscal tab instead of offering settings that answer 403.
func TestInstanceInfoDemoModeHidesFiscalAR(t *testing.T) {
	clearInstanceTestEnv(t)

	config.SetDemoModeForTesting(t, false)
	if !BuildInstanceInfo().Features.FiscalAR {
		t.Fatal("features.fiscal_ar = false outside demo mode, want true (capability)")
	}

	config.SetDemoModeForTesting(t, true)
	if BuildInstanceInfo().Features.FiscalAR {
		t.Fatal("features.fiscal_ar = true under DEMO_MODE, want false (fiscal writes are refused)")
	}
}
