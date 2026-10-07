package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// clearOSSPreflightEnv isolates a test from the developer/CI shell. t.Setenv
// also restores whatever applyOSSPreflightInputs writes back with os.Setenv.
func clearOSSPreflightEnv(t *testing.T) {
	t.Helper()
	if logger.Logger == nil {
		logger.InitLogger()
	}
	for _, key := range []string{
		"PUBLIC_URL", "FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL",
		"ADMIN_PASSWORD", "STORAGE_DRIVER", "EDGE",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM",
		"EMAIL_PROVIDER", "EMAIL_API_KEY", "RESEND_API_KEY",
		"ALLOWED_ORIGINS", "TRUSTED_PROXIES",
	} {
		t.Setenv(key, "")
	}
}

// minimalProductionInputs mirrors what main.go resolves from flags for the
// six-variable self-host deploy (the rest comes from applyOSSPreflightInputs).
func minimalProductionInputs() config.ProductionInputs {
	return config.ProductionInputs{
		Production:      true,
		DBPassword:      "cmdapp-db-password-5e81c2a9",
		JWTSecretKey:    "cmdapp-jwt-secret-0123456789abcdefghijklmnopqrstuvwxyz",
		PluginSecretKey: base64.StdEncoding.EncodeToString([]byte("cmdapp-plugin-key-32-bytes-long!")),
	}
}

// assertPreflightPassesOrTransportGap asserts the report is clean, except
// that a provider this build's email factory cannot construct must fail
// closed with exactly email.transport.unavailable. While
// emails.NewProvider knows only resend/postmark, the log
// and smtp self-host paths must refuse production instead of booting with a
// transport that cannot send.
//
// The allowance tightens itself: it asks the factory, so once NewProvider
// builds log and smtp, every caller must produce a clean
// report, and the minimal six-variable config is held to "passes outright".
// The gap branch can then be deleted.
func assertPreflightPassesOrTransportGap(t *testing.T, in config.ProductionInputs) {
	t.Helper()
	report := config.ValidateProduction(in)
	if _, err := emails.NewProvider(in.EmailProvider, in.EmailAPIKey); err != nil {
		if codes := report.Codes(); len(codes) != 1 || codes[0] != "email.transport.unavailable" {
			t.Fatalf("factory cannot build %q (%v): codes = %v, want only email.transport.unavailable", in.EmailProvider, err, codes)
		}
		return
	}
	if !report.OK() {
		t.Fatalf("config failed preflight: %v", report.Codes())
	}
}

func TestApplyOSSPreflightInputs_MinimalSelfHostPasses(t *testing.T) {
	clearOSSPreflightEnv(t)
	t.Setenv("PUBLIC_URL", "https://Bistro.Example.com/")
	t.Setenv("ADMIN_PASSWORD", "cmdapp-admin-password-9c1e")

	in := minimalProductionInputs()
	applyOSSPreflightInputs(&in, "", "")
	assertPreflightPassesOrTransportGap(t, in)
	if in.EmailProvider != "log" || in.EmailAPIKey != "" {
		t.Fatalf("email provider = %q (key set: %v), want log without key", in.EmailProvider, in.EmailAPIKey != "")
	}
	if got := os.Getenv("ALLOWED_ORIGINS"); got != "https://bistro.example.com" {
		t.Fatalf("ALLOWED_ORIGINS = %q, want the PUBLIC_URL origin", got)
	}
	if got := os.Getenv("TRUSTED_PROXIES"); got != config.DefaultTrustedProxies {
		t.Fatalf("TRUSTED_PROXIES = %q, want the private-range default", got)
	}
	if in.AllowedOrigins != "https://bistro.example.com" || in.TrustedProxies != config.DefaultTrustedProxies {
		t.Fatalf("derived defaults not reflected in preflight inputs: origins=%q proxies=%q", in.AllowedOrigins, in.TrustedProxies)
	}
}

// PUBLIC_URL is the only instance URL name: APP_BASE_URL (the API origin) and
// the retired aliases neither resolve it nor become a trusted CORS origin.
func TestApplyOSSPreflightInputs_APIOriginKeysNeverTrusted(t *testing.T) {
	for _, key := range []string{"FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL"} {
		t.Run(key, func(t *testing.T) {
			clearOSSPreflightEnv(t)
			t.Setenv(key, "https://api.bistro.example.com")

			in := minimalProductionInputs()
			applyOSSPreflightInputs(&in, "", "")

			if in.PublicURL != "" {
				t.Fatalf("PublicURL = %q, want empty (%s is not an alias)", in.PublicURL, key)
			}
			if got := os.Getenv("ALLOWED_ORIGINS"); got != "" || in.AllowedOrigins != "" {
				t.Fatalf("ALLOWED_ORIGINS derived from API-origin key %s: env=%q input=%q", key, got, in.AllowedOrigins)
			}
		})
	}
}

func TestApplyOSSPreflightInputs_KeepsExplicitOriginsAndProxies(t *testing.T) {
	clearOSSPreflightEnv(t)
	t.Setenv("PUBLIC_URL", "https://bistro.example.com")
	t.Setenv("ALLOWED_ORIGINS", "https://a.example.com,https://b.example.com")
	t.Setenv("TRUSTED_PROXIES", "172.16.0.0/12")

	in := minimalProductionInputs()
	in.AllowedOrigins = os.Getenv("ALLOWED_ORIGINS")
	in.TrustedProxies = os.Getenv("TRUSTED_PROXIES")
	applyOSSPreflightInputs(&in, "", "")

	if got := os.Getenv("ALLOWED_ORIGINS"); got != "https://a.example.com,https://b.example.com" {
		t.Fatalf("explicit ALLOWED_ORIGINS overwritten: %q", got)
	}
	if got := os.Getenv("TRUSTED_PROXIES"); got != "172.16.0.0/12" {
		t.Fatalf("explicit TRUSTED_PROXIES overwritten: %q", got)
	}
	if in.TrustedProxies != "172.16.0.0/12" {
		t.Fatalf("TrustedProxies input = %q", in.TrustedProxies)
	}
}

func TestApplyOSSPreflightInputs_InvalidPublicURLDerivesNothing(t *testing.T) {
	clearOSSPreflightEnv(t)
	t.Setenv("PUBLIC_URL", "http://bistro.invalid")

	in := minimalProductionInputs()
	applyOSSPreflightInputs(&in, "", "")

	if got := os.Getenv("ALLOWED_ORIGINS"); got != "" {
		t.Fatalf("ALLOWED_ORIGINS derived from an insecure PUBLIC_URL: %q", got)
	}
	codes := config.ValidateProduction(in).Codes()
	if !containsCode(codes, "public_url.insecure") {
		t.Fatalf("codes = %v, want public_url.insecure", codes)
	}
}

func TestApplyOSSPreflightInputs_EmailProviderSelection(t *testing.T) {
	t.Run("RESEND_API_KEY selects resend and supplies the key", func(t *testing.T) {
		clearOSSPreflightEnv(t)
		t.Setenv("RESEND_API_KEY", "re_cmdapp_test")
		in := minimalProductionInputs()
		applyOSSPreflightInputs(&in, "", "")
		if in.EmailProvider != "resend" || in.EmailAPIKey != "re_cmdapp_test" {
			t.Fatalf("provider=%q key-from-env=%v", in.EmailProvider, in.EmailAPIKey == "re_cmdapp_test")
		}
	})
	t.Run("explicit resend without keys fails preflight", func(t *testing.T) {
		clearOSSPreflightEnv(t)
		t.Setenv("PUBLIC_URL", "https://bistro.example.com")
		t.Setenv("EMAIL_PROVIDER", "resend")
		in := minimalProductionInputs()
		applyOSSPreflightInputs(&in, "", "")
		codes := config.ValidateProduction(in).Codes()
		if !containsCode(codes, "email.api_key.missing") {
			t.Fatalf("codes = %v, want email.api_key.missing", codes)
		}
	})
	t.Run("flag wins over env and the email-api-key flag is used for postmark", func(t *testing.T) {
		clearOSSPreflightEnv(t)
		t.Setenv("RESEND_API_KEY", "re_cmdapp_test")
		in := minimalProductionInputs()
		applyOSSPreflightInputs(&in, "Postmark", "pm-flag-token")
		if in.EmailProvider != "postmark" || in.EmailAPIKey != "pm-flag-token" {
			t.Fatalf("provider=%q key-is-flag=%v", in.EmailProvider, in.EmailAPIKey == "pm-flag-token")
		}
	})
	t.Run("smtp inputs flow through", func(t *testing.T) {
		clearOSSPreflightEnv(t)
		t.Setenv("PUBLIC_URL", "https://bistro.example.com")
		t.Setenv("EMAIL_PROVIDER", "smtp")
		t.Setenv("SMTP_HOST", "smtp.bistro.example.com")
		t.Setenv("SMTP_PORT", "587")
		t.Setenv("SMTP_FROM", "Bistro <hello@bistro.example.com>")
		in := minimalProductionInputs()
		applyOSSPreflightInputs(&in, "", "")
		if in.SMTPHost != "smtp.bistro.example.com" || in.SMTPPort != "587" {
			t.Fatalf("SMTP inputs not resolved: host=%q port=%q", in.SMTPHost, in.SMTPPort)
		}
		assertPreflightPassesOrTransportGap(t, in)
	})
	t.Run("unknown provider is rejected by name", func(t *testing.T) {
		clearOSSPreflightEnv(t)
		t.Setenv("PUBLIC_URL", "https://bistro.example.com")
		t.Setenv("EMAIL_PROVIDER", "sendgrid")
		in := minimalProductionInputs()
		applyOSSPreflightInputs(&in, "", "")
		codes := config.ValidateProduction(in).Codes()
		if len(codes) != 1 || codes[0] != "email.provider.invalid" {
			t.Fatalf("codes = %v, want only email.provider.invalid (the transport probe must not double-report)", codes)
		}
	})
}

// TestApplyOSSPreflightInputs_RuntimeEmailMatchesPreflight pins the M1
// invariant: the provider and key the runtime reads first (--email-provider,
// then EMAIL_PROVIDER; --email-api-key, then EMAIL_API_KEY) are exactly the
// ones preflight validated. Before the fix a RESEND_API_KEY-only deploy
// passed preflight and then built a Resend client with an empty key.
func TestApplyOSSPreflightInputs_RuntimeEmailMatchesPreflight(t *testing.T) {
	firstNonEmpty := func(values ...string) string {
		for _, v := range values {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
		return ""
	}
	cases := []struct {
		name                  string
		env                   map[string]string
		flagProvider, flagKey string
		wantProvider, wantKey string
	}{
		{name: "RESEND_API_KEY only", env: map[string]string{"RESEND_API_KEY": "re_only_resend"}, wantProvider: "resend", wantKey: "re_only_resend"},
		{name: "resend flag with RESEND_API_KEY", env: map[string]string{"RESEND_API_KEY": "re_flagged"}, flagProvider: "resend", wantProvider: "resend", wantKey: "re_flagged"},
		{name: "postmark with EMAIL_API_KEY", env: map[string]string{"EMAIL_PROVIDER": "postmark", "EMAIL_API_KEY": "pm-key"}, wantProvider: "postmark", wantKey: "pm-key"},
		{name: "explicit EMAIL_API_KEY wins and is kept", env: map[string]string{"EMAIL_PROVIDER": "resend", "EMAIL_API_KEY": "re_explicit", "RESEND_API_KEY": "re_other"}, wantProvider: "resend", wantKey: "re_explicit"},
		{name: "flag key wins and env stays empty", env: map[string]string{"RESEND_API_KEY": "re_env"}, flagKey: "re_flag", wantProvider: "resend", wantKey: "re_flag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearOSSPreflightEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			in := minimalProductionInputs()
			applyOSSPreflightInputs(&in, tc.flagProvider, tc.flagKey)
			if in.EmailProvider != tc.wantProvider || in.EmailAPIKey != tc.wantKey {
				t.Fatalf("preflight resolved provider=%q key-match=%v, want %q", in.EmailProvider, in.EmailAPIKey == tc.wantKey, tc.wantProvider)
			}
			runtimeProvider := strings.ToLower(firstNonEmpty(tc.flagProvider, os.Getenv("EMAIL_PROVIDER")))
			runtimeKey := firstNonEmpty(tc.flagKey, os.Getenv("EMAIL_API_KEY"))
			if runtimeProvider != in.EmailProvider || runtimeKey != in.EmailAPIKey {
				t.Fatalf("runtime would build provider=%q key-match=%v; preflight validated %q", runtimeProvider, runtimeKey == in.EmailAPIKey, in.EmailProvider)
			}
			if tc.env["EMAIL_API_KEY"] == "" && tc.flagKey != "" && os.Getenv("EMAIL_API_KEY") != "" {
				t.Fatalf("EMAIL_API_KEY mirrored although --email-api-key already supplies the key")
			}
			if in.EmailTransportError != "" {
				t.Fatalf("resend/postmark with a key must build: %s", in.EmailTransportError)
			}
		})
	}

	t.Run("log and smtp mirror nothing", func(t *testing.T) {
		for _, provider := range []string{"", "smtp"} {
			clearOSSPreflightEnv(t)
			t.Setenv("EMAIL_PROVIDER", provider)
			t.Setenv("EMAIL_API_KEY", "")
			in := minimalProductionInputs()
			applyOSSPreflightInputs(&in, "", "")
			if got := os.Getenv("EMAIL_PROVIDER"); got != provider {
				t.Fatalf("EMAIL_PROVIDER rewritten to %q for provider %q", got, in.EmailProvider)
			}
			if got := os.Getenv("EMAIL_API_KEY"); got != "" {
				t.Fatalf("EMAIL_API_KEY mirrored for keyless provider %q", in.EmailProvider)
			}
		}
	})

	t.Run("development skips the transport probe", func(t *testing.T) {
		clearOSSPreflightEnv(t)
		t.Setenv("EMAIL_PROVIDER", "smtp")
		in := minimalProductionInputs()
		in.Production = false
		applyOSSPreflightInputs(&in, "", "")
		if in.EmailTransportError != "" {
			t.Fatalf("development must not probe the email factory: %s", in.EmailTransportError)
		}
	})
}

func TestApplyOSSPreflightInputs_S3WithoutBucketFails(t *testing.T) {
	clearOSSPreflightEnv(t)
	t.Setenv("PUBLIC_URL", "https://bistro.example.com")
	t.Setenv("STORAGE_DRIVER", "s3")
	in := minimalProductionInputs()
	applyOSSPreflightInputs(&in, "", "")
	codes := config.ValidateProduction(in).Codes()
	if !containsCode(codes, "s3.public.missing") {
		t.Fatalf("codes = %v, want s3.public.missing", codes)
	}
}

func TestProductionSettlementChainError(t *testing.T) {
	tests := []struct {
		name       string
		production bool
		chainID    int64
		wantErr    bool
	}{
		{"production on Base mainnet", true, 8453, false},
		{"production RPC unreachable", true, 0, false},
		{"production on Base Sepolia", true, 84532, true},
		{"production on Ethereum mainnet", true, 1, true},
		{"development on Base Sepolia", false, 84532, false},
		{"development on Base mainnet", false, 8453, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := productionSettlementChainError(tt.production, tt.chainID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "8453") {
				t.Fatalf("error should name the required chain: %v", err)
			}
		})
	}
	// Nil-safe: an unreachable RPC leaves the service nil and must not fatal.
	enforceProductionSettlementChain(true, nil)
}

func TestResolveSettlementRPCURL(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	if got := resolveSettlementRPCURL(""); got != config.DefaultRPCURL {
		t.Fatalf("empty RPC_URL = %q, want %q", got, config.DefaultRPCURL)
	}
	if got := resolveSettlementRPCURL(" https://base-rpc.example "); got != "https://base-rpc.example" {
		t.Fatalf("explicit RPC_URL = %q", got)
	}
}

func TestEnsureInstancePluginKey(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}

	t.Run("production without a key is fatal", func(t *testing.T) {
		t.Setenv("PLUGIN_SECRET_KEY", "")
		if err := ensureInstancePluginKey(true, t.TempDir()); err == nil {
			t.Fatal("expected an error for production without PLUGIN_SECRET_KEY")
		}
	})

	t.Run("explicit key needs no file", func(t *testing.T) {
		t.Setenv("PLUGIN_SECRET_KEY", base64.StdEncoding.EncodeToString([]byte("cmdapp-plugin-key-32-bytes-long!")))
		dir := t.TempDir()
		if err := ensureInstancePluginKey(true, dir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "secrets")); !os.IsNotExist(err) {
			t.Fatalf("no key file should be written when PLUGIN_SECRET_KEY is set (stat err %v)", err)
		}
	})

	t.Run("development generates then reuses a 0600 key", func(t *testing.T) {
		t.Setenv("PLUGIN_SECRET_KEY", "")
		t.Setenv("STORAGE_DIR", "")
		dir := t.TempDir()
		if err := ensureInstancePluginKey(false, dir); err != nil {
			t.Fatalf("first boot: %v", err)
		}
		path := filepath.Join(dir, "secrets", "plugin_secret_key")
		first, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("key file not persisted: %v", err)
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if perm := info.Mode().Perm(); perm != 0o600 {
				t.Fatalf("key file mode = %o, want 0600", perm)
			}
		}
		if err := ensureInstancePluginKey(false, dir); err != nil {
			t.Fatalf("second boot: %v", err)
		}
		second, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(first) != string(second) {
			t.Fatal("second boot replaced the persisted key")
		}
	})

	t.Run("development refuses a data dir inside the served STORAGE_DIR", func(t *testing.T) {
		t.Setenv("PLUGIN_SECRET_KEY", "")
		storage := t.TempDir()
		t.Setenv("STORAGE_DIR", storage)
		for _, dataDir := range []string{storage, filepath.Join(storage, "state")} {
			if err := ensureInstancePluginKey(false, dataDir); err == nil {
				t.Fatalf("DATA_DIR %q inside STORAGE_DIR must be refused", dataDir)
			}
			if _, err := os.Stat(filepath.Join(dataDir, "secrets")); !os.IsNotExist(err) {
				t.Fatalf("no key may be written under STORAGE_DIR (stat err %v)", err)
			}
		}
	})

	t.Run("development default storage dir is guarded too", func(t *testing.T) {
		t.Setenv("PLUGIN_SECRET_KEY", "")
		t.Setenv("STORAGE_DIR", "")
		if err := ensureInstancePluginKey(false, filepath.Join("data", "storage")); err == nil {
			t.Fatal("DATA_DIR=data/storage collides with the default development STORAGE_DIR and must be refused")
		}
	})
}

func containsCode(codes []string, want string) bool {
	for _, code := range codes {
		if code == want {
			return true
		}
	}
	return false
}
