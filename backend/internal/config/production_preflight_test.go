package config

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unitPluginKeyRaw is a 32-byte plugin key used only by these tests. The
// sequential-hex key other packages' tests use is a published value and is
// refused by production preflight.
const unitPluginKeyRaw = "unit-preflight-plugin-key-32byte"

// validProductionInputs returns a fully-populated production configuration that
// should pass preflight. Tests clone and mutate one field at a time.
func validProductionInputs() ProductionInputs {
	pluginKey := base64.StdEncoding.EncodeToString([]byte(unitPluginKeyRaw))
	return ProductionInputs{
		Production: true,
		DBPassword: "unit-db-password-8c4f1d907e2a",
		PublicURL:  "https://restaurant.example.com",

		JWTSecretKey:    "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6A7B8C9",
		PluginSecretKey: pluginKey,
		RPCURL:          "https://mainnet.base.org",

		EmailProvider:           "resend",
		EmailAPIKey:             "re_test_server_token_not_a_real_secret",
		FromEmail:               "noreply@payverge.io",
		FromEmailUpdates:        "updates@payverge.io",
		EmailAllowedFromDomains: "payverge.io",
		EmailWebhookSecret:      "whsec_" + base64.StdEncoding.EncodeToString([]byte("unit-resend-webhook-secret")),
		EmailSPFDomain:          "send.payverge.io",
		EmailDKIMDomain:         "payverge.io",
		EmailReturnPathDomain:   "send.payverge.io",
		EmailDMARCPolicy:        "quarantine",

		StorageDriver:   "s3",
		S3Bucket:        "payverge-public",
		AWSAccessKey:    "AKIAEXAMPLE",
		AWSSecretKey:    "secret-example-not-real",
		S3Endpoint:      "https://s3.example.com",
		S3PublicBaseURL: "https://images.example.com",

		S3ProtectedBucket:     "payverge-protected",
		AWSProtectedAccessKey: "AKIAEXAMPLEPROT",
		AWSProtectedSecretKey: "secret-protected-not-real",
		S3ProtectedEndpoint:   "https://s3-protected.example.com",

		AllowedOrigins: "https://payverge.io,https://www.payverge.io",
		CookieDomain:   "api.payverge.io",

		// Wave 2: Caddy terminates Cloudflare; Gin trusts only private Caddy CIDR.
		Edge:            "cloudflare",
		TrustedPlatform: "",
		TrustedProxies:  "172.16.0.0/12,127.0.0.1",

		AIDailyBudgetUSD: "25",

		OpenRouterAPIKey:  "unit-openrouter-key-402b",
		OpenRouterZDRMode: "enforce",
		PaymentSecrets: map[string]string{
			"STRIPE_WEBHOOK_SECRET": "unit-payment-signing-value-7d39",
		},
		FiscalWSAAURL: "https://wsaa.afip.gov.ar/ws/services/LoginCms",
		FiscalWSFEURL: "https://servicios1.afip.gov.ar/wsfev1/service.asmx",

		TelegramWebhookEnabled: false,
		TelegramWebhookSecret:  "",
	}
}

func TestProductionPreflight_CanonicalEmptyProductionInput(t *testing.T) {
	report := ValidateProduction(ProductionInputs{Production: true})
	if report.OK() {
		t.Fatal("expected ValidateProduction(Production:true) with empty inputs to fail")
	}
	codes := report.Codes()
	// Only the always-required core is fatal on an empty deploy (CORS origins
	// derive from PUBLIC_URL, so they are reported missing alongside it).
	assert.Equal(t, []string{"jwt.secret.missing", "plugin_secret.missing", "public_url.missing", "origins.missing"}, codes)
	// Optional integrations default to safe fallbacks and only warn.
	assert.NotContains(t, codes, "email.api_key.missing")
	assert.NotContains(t, codes, "s3.public.missing")
	assert.Subset(t, warningCodes(report), []string{"rpc_url.default", "email.provider.log"})
}

func warningCodes(r Report) []string {
	warnings := r.Warnings()
	out := make([]string, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, w.Code)
	}
	return out
}

func TestProductionPreflight_FullyValidProductionOK(t *testing.T) {
	report := ValidateProduction(validProductionInputs())
	assert.True(t, report.OK(), "codes=%v", report.Codes())
	assert.Empty(t, report.Codes())
	assert.Empty(t, report.Issues())
}

func TestProductionPreflight_DevAcceptsOmissions(t *testing.T) {
	report := ValidateProduction(ProductionInputs{Production: false})
	assert.True(t, report.OK(), "dev mode must accept provider omissions; codes=%v", report.Codes())
}

func TestProductionPreflight_PostmarkRollbackDoesNotRequireResendDNSOrWebhook(t *testing.T) {
	in := validProductionInputs()
	in.EmailProvider = "postmark"
	in.EmailAPIKey = "postmark-rollback-token"
	in.EmailWebhookSecret = ""
	in.EmailSPFDomain = ""
	in.EmailDKIMDomain = ""
	in.EmailReturnPathDomain = ""
	in.EmailDMARCPolicy = ""

	report := ValidateProduction(in)
	assert.True(t, report.OK(), "rollback codes=%v", report.Codes())
}

func TestProductionPreflight_OneFieldAtATime(t *testing.T) {
	pluginKey := base64.StdEncoding.EncodeToString([]byte(unitPluginKeyRaw))

	tests := []struct {
		name         string
		mutate       func(*ProductionInputs)
		wantCode     string
		wantOK       bool
		extraAsserts func(t *testing.T, r Report)
	}{
		{
			name: "database password rejects repository default",
			mutate: func(in *ProductionInputs) {
				in.DBPassword = "payverge_password"
			},
			wantCode: "database.password.unsafe",
		},
		{
			name: "jwt secret missing",
			mutate: func(in *ProductionInputs) {
				in.JWTSecretKey = ""
			},
			wantCode: "jwt.secret.missing",
		},
		{
			name: "plugin secret rejects valid-length placeholder",
			mutate: func(in *ProductionInputs) {
				in.PluginSecretKey = "replace_with_plugin_secret_12345"
			},
			wantCode: "plugin_secret.placeholder",
		},
		{
			name: "jwt secret whitespace only",
			mutate: func(in *ProductionInputs) {
				in.JWTSecretKey = "   "
			},
			wantCode: "jwt.secret.missing",
		},
		{
			name: "email api key rejects placeholder",
			mutate: func(in *ProductionInputs) {
				in.EmailAPIKey = "replace_with_email_api_key"
			},
			wantCode: "email.api_key.placeholder",
		},
		{
			name: "resend rejects a provider-mismatched api key",
			mutate: func(in *ProductionInputs) {
				in.EmailAPIKey = "pm-server-token"
			},
			wantCode: "email.api_key.provider_mismatch",
		},
		{
			name: "jwt secret short",
			mutate: func(in *ProductionInputs) {
				in.JWTSecretKey = "too-short-for-production"
			},
			wantCode: "jwt.secret.short",
		},
		{
			name: "s3 public access key rejects placeholder",
			mutate: func(in *ProductionInputs) {
				in.AWSAccessKey = "replace_with_aws_access_key"
			},
			wantCode: "s3.public.placeholder",
		},
		{
			name: "s3 protected secret key rejects placeholder",
			mutate: func(in *ProductionInputs) {
				in.AWSProtectedSecretKey = "changeme"
			},
			wantCode: "s3.protected.placeholder",
		},
		{
			name: "jwt secret unsafe local default",
			mutate: func(in *ProductionInputs) {
				in.JWTSecretKey = "payverge_local_dev_jwt_secret_2026"
			},
			wantCode: "jwt.secret.unsafe",
		},
		{
			name: "jwt secret unsafe replace_with placeholder",
			mutate: func(in *ProductionInputs) {
				in.JWTSecretKey = "replace_with_at_least_32_random_characters_1234567890"
			},
			wantCode: "jwt.secret.unsafe",
		},
		{
			name: "plugin secret missing",
			mutate: func(in *ProductionInputs) {
				in.PluginSecretKey = ""
			},
			wantCode: "plugin_secret.missing",
		},
		{
			name: "plugin secret invalid",
			mutate: func(in *ProductionInputs) {
				in.PluginSecretKey = "too-short"
			},
			wantCode: "plugin_secret.invalid",
		},
		{
			name: "plugin secret valid raw 32 bytes",
			mutate: func(in *ProductionInputs) {
				in.PluginSecretKey = unitPluginKeyRaw
			},
			wantOK: true,
		},
		{
			name: "plugin secret valid base64",
			mutate: func(in *ProductionInputs) {
				in.PluginSecretKey = pluginKey
			},
			wantOK: true,
		},
		{
			name: "rpc url unset uses the public Base RPC with a warning",
			mutate: func(in *ProductionInputs) {
				in.RPCURL = ""
			},
			wantOK: true,
			extraAsserts: func(t *testing.T, r Report) {
				assert.Contains(t, warningCodes(r), "rpc_url.default")
			},
		},
		{
			name: "rpc url malformed only warns",
			mutate: func(in *ProductionInputs) {
				in.RPCURL = "ftp://rpc.example.com"
			},
			wantOK: true,
			extraAsserts: func(t *testing.T, r Report) {
				assert.Contains(t, warningCodes(r), "rpc_url.invalid")
			},
		},
		{
			name: "email provider unset falls back to the log sink",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = ""
			},
			wantOK: true,
			extraAsserts: func(t *testing.T, r Report) {
				assert.Contains(t, warningCodes(r), "email.provider.log")
			},
		},
		{
			name: "email provider log needs no keys or senders",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "log"
				in.EmailAPIKey = ""
				in.FromEmail = ""
				in.FromEmailUpdates = ""
				in.EmailAllowedFromDomains = ""
				in.EmailWebhookSecret = ""
				in.EmailDMARCPolicy = ""
			},
			wantOK: true,
		},
		{
			name: "email provider unknown",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "sendgrid"
			},
			wantCode: "email.provider.invalid",
		},
		{
			name: "smtp requires a host",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "smtp"
				in.SMTPFrom = "noreply@restaurant.example"
			},
			wantCode: "email.smtp.host.missing",
		},
		{
			name: "smtp rejects an invalid port",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "smtp"
				in.SMTPHost = "smtp.restaurant.example"
				in.SMTPPort = "70000"
			},
			wantCode: "email.smtp.port.invalid",
		},
		{
			name: "smtp rejects a placeholder password",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "smtp"
				in.SMTPHost = "smtp.restaurant.example"
				in.SMTPPassword = "changeme"
			},
			wantCode: "email.smtp.password.placeholder",
		},
		{
			name: "smtp requires a sender",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "smtp"
				in.SMTPHost = "smtp.restaurant.example"
				in.FromEmail = ""
				in.SMTPFrom = ""
			},
			wantCode: "email.from.missing",
		},
		{
			name: "smtp rejects a malformed sender",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "smtp"
				in.SMTPHost = "smtp.restaurant.example"
				in.SMTPFrom = "not-an-address"
			},
			wantCode: "email.from.invalid",
		},
		{
			name: "smtp skips the Resend DNS and webhook block",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "SMTP"
				in.EmailAPIKey = ""
				in.SMTPHost = "smtp.restaurant.example"
				in.SMTPPort = "465"
				in.SMTPUsername = "mailer"
				in.SMTPPassword = "unit-smtp-password-5c1e"
				in.SMTPFrom = "Restaurant <noreply@restaurant.example>"
				in.FromEmail = ""
				in.FromEmailUpdates = ""
				in.EmailAllowedFromDomains = ""
				in.EmailWebhookSecret = ""
				in.EmailSPFDomain = ""
				in.EmailDMARCPolicy = ""
			},
			wantOK: true,
		},
		{
			name: "email api key missing",
			mutate: func(in *ProductionInputs) {
				in.EmailAPIKey = ""
			},
			wantCode: "email.api_key.missing",
		},
		{
			name: "email from missing",
			mutate: func(in *ProductionInputs) {
				in.FromEmail = ""
			},
			wantCode: "email.from.missing",
		},
		{
			name: "email from updates missing",
			mutate: func(in *ProductionInputs) {
				in.FromEmailUpdates = ""
			},
			wantCode: "email.from.missing",
		},
		{
			name: "resend requires approved sender domains",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "resend"
				in.EmailAPIKey = "re_valid_shape"
				in.EmailAllowedFromDomains = ""
			},
			wantCode: "email.sender_domains.missing",
		},
		{
			name: "resend requires an authenticated webhook secret",
			mutate: func(in *ProductionInputs) {
				in.EmailWebhookSecret = ""
			},
			wantCode: "email.webhook_secret.missing",
		},
		{
			name: "resend rejects a malformed webhook secret",
			mutate: func(in *ProductionInputs) {
				in.EmailWebhookSecret = "plain-text-secret"
			},
			wantCode: "email.webhook_secret.invalid",
		},
		{
			name: "resend requires declared SPF DKIM and return path alignment",
			mutate: func(in *ProductionInputs) {
				in.EmailReturnPathDomain = ""
			},
			wantCode: "email.dns_alignment.missing",
		},
		{
			name: "resend rejects an unrelated return path domain",
			mutate: func(in *ProductionInputs) {
				in.EmailReturnPathDomain = "bounce.attacker.example"
			},
			wantCode: "email.dns_alignment.invalid",
		},
		{
			name: "resend rejects monitoring-only DMARC",
			mutate: func(in *ProductionInputs) {
				in.EmailDMARCPolicy = "none"
			},
			wantCode: "email.dmarc_policy.invalid",
		},
		{
			name: "resend rejects a from domain outside the approved list",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "resend"
				in.EmailAPIKey = "re_valid_shape"
				in.EmailAllowedFromDomains = "mail.payverge.io"
				in.FromEmail = "noreply@payverge.io"
				in.FromEmailUpdates = "updates@payverge.io"
			},
			wantCode: "email.from_domain.unapproved",
		},
		{
			name: "resend accepts both approved senders",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "resend"
				in.EmailAPIKey = "re_valid_shape"
				in.EmailAllowedFromDomains = "payverge.io"
				in.FromEmail = "Payverge <noreply@payverge.io>"
				in.FromEmailUpdates = "updates@payverge.io"
			},
			wantOK: true,
		},
		{
			name: "s3 public bucket missing",
			mutate: func(in *ProductionInputs) {
				in.S3Bucket = ""
			},
			wantCode: "s3.public.missing",
		},
		{
			name: "s3 public access key missing",
			mutate: func(in *ProductionInputs) {
				in.AWSAccessKey = ""
			},
			wantCode: "s3.public.missing",
		},
		{
			name: "s3 public secret key missing",
			mutate: func(in *ProductionInputs) {
				in.AWSSecretKey = ""
			},
			wantCode: "s3.public.missing",
		},
		{
			// Shared-bucket mode (s3.Init): protected objects live in
			// S3_BUCKET under protected/; the startup probe guards exposure.
			name: "s3 protected bucket empty shares the public bucket",
			mutate: func(in *ProductionInputs) {
				in.S3ProtectedBucket = ""
				in.AWSProtectedAccessKey = ""
				in.AWSProtectedSecretKey = ""
			},
			wantOK: true,
			extraAsserts: func(t *testing.T, r Report) {
				assert.Contains(t, warningCodes(r), "s3.protected.shared")
			},
		},
		{
			name: "s3 protected bucket reuses the public credentials",
			mutate: func(in *ProductionInputs) {
				in.AWSProtectedAccessKey = ""
				in.AWSProtectedSecretKey = ""
			},
			wantOK: true,
			extraAsserts: func(t *testing.T, r Report) {
				assert.NotContains(t, warningCodes(r), "s3.protected.shared")
			},
		},
		{
			name: "s3 protected access key missing",
			mutate: func(in *ProductionInputs) {
				in.AWSProtectedAccessKey = ""
			},
			wantCode: "s3.protected.missing",
		},
		{
			name: "s3 protected secret key missing",
			mutate: func(in *ProductionInputs) {
				in.AWSProtectedSecretKey = ""
			},
			wantCode: "s3.protected.missing",
		},
		{
			name: "origins unset derive from PUBLIC_URL",
			mutate: func(in *ProductionInputs) {
				in.AllowedOrigins = ""
			},
			wantOK: true,
		},
		{
			name: "origins missing when PUBLIC_URL is also unset",
			mutate: func(in *ProductionInputs) {
				in.AllowedOrigins = ""
				in.PublicURL = ""
			},
			wantCode: "origins.missing",
		},
		{
			name: "public url missing",
			mutate: func(in *ProductionInputs) {
				in.PublicURL = " "
			},
			wantCode: "public_url.missing",
		},
		{
			name: "public url must be https",
			mutate: func(in *ProductionInputs) {
				in.PublicURL = "http://restaurant.example.com"
			},
			wantCode: "public_url.insecure",
		},
		{
			name: "public url must not carry a path",
			mutate: func(in *ProductionInputs) {
				in.PublicURL = "https://restaurant.example.com/api/v1"
			},
			wantCode: "public_url.invalid",
		},
		{
			name: "public url must be absolute",
			mutate: func(in *ProductionInputs) {
				in.PublicURL = "restaurant.example.com"
			},
			wantCode: "public_url.invalid",
		},
		{
			name: "public url trailing slash accepted",
			mutate: func(in *ProductionInputs) {
				in.PublicURL = "https://restaurant.example.com/"
			},
			wantOK: true,
		},
		{
			name: "public url loopback http only warns",
			mutate: func(in *ProductionInputs) {
				in.PublicURL = "http://localhost:3000"
			},
			wantOK: true,
			extraAsserts: func(t *testing.T, r Report) {
				assert.Contains(t, warningCodes(r), "public_url.loopback")
			},
		},
		{
			name: "storage local skips S3 credentials",
			mutate: func(in *ProductionInputs) {
				in.StorageDriver = ""
				in.S3Bucket = ""
				in.AWSAccessKey = ""
				in.AWSSecretKey = ""
				in.S3ProtectedBucket = ""
				in.AWSProtectedAccessKey = ""
				in.AWSProtectedSecretKey = ""
			},
			wantOK: true,
		},
		{
			name: "storage local warns that configured S3 buckets are ignored",
			mutate: func(in *ProductionInputs) {
				in.StorageDriver = "local"
				in.AWSAccessKey = "replace_with_aws_access_key"
			},
			wantOK: true,
			extraAsserts: func(t *testing.T, r Report) {
				assert.Contains(t, warningCodes(r), "storage.s3.ignored")
			},
		},
		{
			name: "storage driver unknown",
			mutate: func(in *ProductionInputs) {
				in.StorageDriver = "gcs"
			},
			wantCode: "storage.driver.invalid",
		},
		{
			name: "edge unknown",
			mutate: func(in *ProductionInputs) {
				in.Edge = "fastly"
			},
			wantCode: "edge.invalid",
		},
		{
			name: "edge none rejects a trusted platform header",
			mutate: func(in *ProductionInputs) {
				in.Edge = ""
				in.TrustedPlatform = "cloudflare"
			},
			wantCode: "proxy.platform.unexpected",
		},
		{
			name: "admin password rejects a published default",
			mutate: func(in *ProductionInputs) {
				in.AdminPassword = "password"
			},
			wantCode: "admin.password.unsafe",
		},
		{
			name: "admin password accepts an operator value",
			mutate: func(in *ProductionInputs) {
				in.AdminPassword = "unit-admin-password-91be"
			},
			wantOK: true,
		},
		{
			name: "plugin secret rejects the retired development fallback seed",
			mutate: func(in *ProductionInputs) {
				in.PluginSecretKey = "payverge-local-plugin-secret-key"
			},
			wantCode: "plugin_secret.unsafe",
		},
		{
			name: "jwt secret rejects the compose min32 default",
			mutate: func(in *ProductionInputs) {
				in.JWTSecretKey = "payverge_local_dev_jwt_secret_2026_min32"
			},
			wantCode: "jwt.secret.unsafe",
		},
		{
			name: "cookie domain empty host-only OK",
			mutate: func(in *ProductionInputs) {
				in.CookieDomain = ""
			},
			wantOK: true,
		},
		{
			name: "cookie domain leading-dot parent rejected",
			mutate: func(in *ProductionInputs) {
				in.CookieDomain = ".payverge.io"
			},
			wantCode: "cookie_domain.parent_scope",
		},
		{
			name: "cookie domain bare parent rejected",
			mutate: func(in *ProductionInputs) {
				in.CookieDomain = "payverge.io"
			},
			wantCode: "cookie_domain.parent_scope",
		},
		{
			// Blank platform is the Wave 2 production contract (Caddy terminates CF).
			name: "cloudflare platform blank OK",
			mutate: func(in *ProductionInputs) {
				in.TrustedPlatform = ""
			},
			wantOK: true,
		},
		{
			name: "cloudflare platform whitespace only OK",
			mutate: func(in *ProductionInputs) {
				in.TrustedPlatform = "   "
			},
			wantOK: true,
		},
		{
			// A set platform means Gin would trust CF headers on direct peers.
			name: "cloudflare platform unexpected when set",
			mutate: func(in *ProductionInputs) {
				in.TrustedPlatform = "cloudflare"
			},
			wantCode: "cloudflare.platform.unexpected",
		},
		{
			name: "trusted proxies unset default to private ranges",
			mutate: func(in *ProductionInputs) {
				in.TrustedProxies = ""
			},
			wantOK: true,
		},
		{
			name: "trusted proxies whitespace only default to private ranges",
			mutate: func(in *ProductionInputs) {
				in.TrustedProxies = "   "
			},
			wantOK: true,
		},
		{
			name: "default trusted proxies pass the private-range check",
			mutate: func(in *ProductionInputs) {
				in.TrustedProxies = DefaultTrustedProxies
			},
			wantOK: true,
		},
		{
			name: "cloudflare proxies reject wildcard",
			mutate: func(in *ProductionInputs) {
				in.TrustedProxies = "0.0.0.0/0"
			},
			wantCode: "cloudflare.proxies.public",
		},
		{
			name: "cloudflare proxies reject public address",
			mutate: func(in *ProductionInputs) {
				in.TrustedProxies = "8.8.8.8"
			},
			wantCode: "cloudflare.proxies.public",
		},
		{
			name: "cloudflare proxies reject malformed entry",
			mutate: func(in *ProductionInputs) {
				in.TrustedProxies = "172.16.0.0/12,not-a-network"
			},
			wantCode: "cloudflare.proxies.invalid",
		},
		{
			name: "cloudflare proxies reject CIDR spanning private and public space",
			mutate: func(in *ProductionInputs) {
				in.TrustedProxies = "10.0.0.0/7"
			},
			wantCode: "cloudflare.proxies.public",
		},
		{
			name: "cloudflare proxies private and loopback OK",
			mutate: func(in *ProductionInputs) {
				in.TrustedProxies = "10.10.0.0/16,172.20.0.0/16,192.168.1.4,127.0.0.1,fc00::/7,::1"
			},
			wantOK: true,
		},
		{
			name: "ai daily budget unset uses the default cap",
			mutate: func(in *ProductionInputs) {
				in.AIDailyBudgetUSD = ""
			},
			wantOK: true,
		},
		{
			name: "ai daily budget zero",
			mutate: func(in *ProductionInputs) {
				in.AIDailyBudgetUSD = "0"
			},
			wantCode: "ai_budget.daily.invalid",
		},
		{
			name: "ai daily budget negative",
			mutate: func(in *ProductionInputs) {
				in.AIDailyBudgetUSD = "-1"
			},
			wantCode: "ai_budget.daily.invalid",
		},
		{
			name: "ai daily budget unparseable",
			mutate: func(in *ProductionInputs) {
				in.AIDailyBudgetUSD = "not-a-number"
			},
			wantCode: "ai_budget.daily.invalid",
		},
		{
			name: "payment signing secret rejects placeholder",
			mutate: func(in *ProductionInputs) {
				in.PaymentSecrets = map[string]string{"PAYPAL_WEBHOOK_SECRET": "replace_with_paypal_secret"}
			},
			wantCode: "payment.secret.placeholder",
		},
		{
			name: "OpenRouter API key rejects placeholder",
			mutate: func(in *ProductionInputs) {
				in.OpenRouterAPIKey = "replace_with_openrouter_api_key"
			},
			wantCode: "openrouter.api_key.placeholder",
		},
		{
			name: "OpenRouter audit mode is forbidden in production",
			mutate: func(in *ProductionInputs) {
				in.OpenRouterZDRMode = "audit"
			},
			wantCode: "openrouter.zdr_mode.audit",
		},
		{
			name: "fiscal production endpoint rejects homologation URL",
			mutate: func(in *ProductionInputs) {
				in.FiscalWSAAURL = "https://wsaahomo.afip.gov.ar/ws/services/LoginCms"
			},
			wantCode: "fiscal.endpoint.unsafe",
		},
		{
			name: "telegram webhook enabled without secret",
			mutate: func(in *ProductionInputs) {
				in.TelegramWebhookEnabled = true
				in.TelegramWebhookSecret = ""
			},
			wantCode: "webhook.telegram.secret.missing",
		},
		{
			name: "telegram webhook enabled with secret OK",
			mutate: func(in *ProductionInputs) {
				in.TelegramWebhookEnabled = true
				in.TelegramWebhookSecret = "telegram-webhook-secret-value"
			},
			wantOK: true,
		},
		{
			name: "telegram webhook disabled without secret OK",
			mutate: func(in *ProductionInputs) {
				in.TelegramWebhookEnabled = false
				in.TelegramWebhookSecret = ""
			},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validProductionInputs()
			tt.mutate(&in)
			report := ValidateProduction(in)

			if tt.wantOK {
				assert.True(t, report.OK(), "expected OK; codes=%v", report.Codes())
				if tt.extraAsserts != nil {
					tt.extraAsserts(t, report)
				}
				return
			}

			require.False(t, report.OK(), "expected failure for %s", tt.wantCode)
			assert.Contains(t, report.Codes(), tt.wantCode)

			// Messages must never echo raw secret material (skip empty fields —
			// every string "contains" the empty substring).
			for _, issue := range report.Issues() {
				for _, secret := range []string{
					in.JWTSecretKey,
					in.PluginSecretKey,
					in.EmailAPIKey,
					in.AWSSecretKey,
					in.AWSProtectedSecretKey,
					in.TelegramWebhookSecret,
				} {
					if secret != "" {
						assert.NotContains(t, issue.Message, secret)
					}
				}
				assert.NotEmpty(t, issue.Code)
				assert.NotEmpty(t, issue.Component)
				assert.NotEmpty(t, issue.Message)
			}

			if tt.extraAsserts != nil {
				tt.extraAsserts(t, report)
			}
		})
	}
}

func TestProductionPreflight_DeterministicOrder(t *testing.T) {
	// A production input with every selected component broken yields many
	// issues; order must be stable and follow the component order.
	broken := ProductionInputs{
		Production:       true,
		EmailProvider:    "resend",
		StorageDriver:    "s3",
		Edge:             "cloudflare",
		TrustedPlatform:  "cloudflare",
		TrustedProxies:   "0.0.0.0/0",
		AIDailyBudgetUSD: "0",
	}
	r1 := ValidateProduction(broken)
	r2 := ValidateProduction(broken)
	assert.Equal(t, r1.Codes(), r2.Codes())
	assert.False(t, r1.OK())

	codes := r1.Codes()
	require.NotEmpty(t, codes)
	assert.Equal(t, "jwt.secret.missing", codes[0])

	joined := strings.Join(codes, ",")
	for _, code := range []string{
		"jwt.secret.missing",
		"plugin_secret.missing",
		"public_url.missing",
		"email.api_key.missing",
		"email.from.missing",
		"s3.public.missing",
		"origins.missing",
		"cloudflare.platform.unexpected",
		"cloudflare.proxies.public",
		"ai_budget.daily.invalid",
	} {
		assert.Contains(t, joined, code)
	}
	// Empty COOKIE_DOMAIN is host-only and intentionally allowed.
	assert.NotContains(t, joined, "cookie_domain.missing")
	assert.NotContains(t, joined, "cookie_domain.parent_scope")
	// RPC problems never fail startup.
	assert.NotContains(t, joined, "rpc_url")

	// Relative ordering of component groups.
	idx := func(code string) int {
		for i, c := range codes {
			if c == code {
				return i
			}
		}
		return -1
	}
	assert.Less(t, idx("jwt.secret.missing"), idx("plugin_secret.missing"))
	assert.Less(t, idx("plugin_secret.missing"), idx("public_url.missing"))
	assert.Less(t, idx("public_url.missing"), idx("email.api_key.missing"))
	assert.Less(t, idx("email.api_key.missing"), idx("email.from.missing"))
	assert.Less(t, idx("email.from.missing"), idx("s3.public.missing"))
	assert.Less(t, idx("s3.public.missing"), idx("origins.missing"))
	// An empty protected bucket is the shared-prefix mode, not an error.
	assert.NotContains(t, joined, "s3.protected.missing")
	assert.Less(t, idx("origins.missing"), idx("cloudflare.platform.unexpected"))
	assert.Less(t, idx("cloudflare.platform.unexpected"), idx("cloudflare.proxies.public"))
	assert.Less(t, idx("cloudflare.proxies.public"), idx("ai_budget.daily.invalid"))
}

func TestProductionPreflight_IssuesNeverContainSecrets(t *testing.T) {
	secret := "super-secret-value-that-must-not-leak-12345"
	in := validProductionInputs()
	in.JWTSecretKey = secret // long enough, but we force short+unsafe path via another field
	in.PluginSecretKey = secret
	in.EmailAPIKey = secret
	in.AWSSecretKey = secret
	// Make plugin invalid (not 32 bytes) and leave other issues
	in.PluginSecretKey = "short"
	in.EmailAPIKey = ""
	report := ValidateProduction(in)
	require.False(t, report.OK())
	for _, issue := range report.Issues() {
		assert.NotContains(t, issue.Message, secret)
		assert.NotContains(t, issue.Code, secret)
	}
}

func TestProductionPreflight_ReportCodesAndIssuesCopy(t *testing.T) {
	report := ValidateProduction(ProductionInputs{Production: true})
	codes := report.Codes()
	issues := report.Issues()
	require.NotEmpty(t, codes)
	require.NotEmpty(t, issues)

	// Mutating returned slices must not affect subsequent calls.
	codes[0] = "mutated"
	issues[0].Code = "mutated"
	assert.NotEqual(t, "mutated", report.Codes()[0])
	assert.NotEqual(t, "mutated", report.Issues()[0].Code)
}

// TestProductionPreflight_EmailTransportError covers the probe cmd/app runs
// against the email provider factory: a provider the build cannot construct
// fails closed, but never on top of a more specific email issue.
func TestProductionPreflight_EmailLogContent(t *testing.T) {
	t.Run("content in production without allow is fatal", func(t *testing.T) {
		in := validProductionInputs()
		in.EmailProvider = "log"
		in.EmailLogContent = true
		report := ValidateProduction(in)
		assert.Contains(t, report.Codes(), "email.log_content.production")
		var issue PreflightIssue
		for _, candidate := range report.Issues() {
			if candidate.Code == "email.log_content.production" {
				issue = candidate
			}
		}
		assert.Equal(t, "email.log_content.production", issue.Code)
		assert.Equal(t, "email", issue.Component)
		assert.Contains(t, issue.Message, "EMAIL_PROVIDER_LOG_ALLOW_PRODUCTION=true")
		assert.Contains(t, warningCodes(report), "email.provider.log")
	})
	t.Run("content in production with allow warns", func(t *testing.T) {
		in := validProductionInputs()
		in.EmailProvider = "log"
		in.EmailLogContent = true
		in.EmailLogAllowProduction = true
		report := ValidateProduction(in)
		assert.True(t, report.OK(), "codes=%v", report.Codes())
		assert.NotContains(t, report.Codes(), "email.log_content.production")
		assert.Contains(t, warningCodes(report), "email.log_content.allowed")
		assert.Contains(t, warningCodes(report), "email.provider.log")
	})
	t.Run("content off has no content issue", func(t *testing.T) {
		in := validProductionInputs()
		in.EmailProvider = "log"
		in.EmailLogContent = false
		report := ValidateProduction(in)
		assert.True(t, report.OK(), "codes=%v", report.Codes())
		assert.NotContains(t, report.Codes(), "email.log_content.production")
		assert.NotContains(t, warningCodes(report), "email.log_content.allowed")
		assert.Contains(t, warningCodes(report), "email.provider.log")
	})
}

func TestProductionPreflight_AllowedOrigins(t *testing.T) {
	tests := []struct {
		name    string
		origins string
		wantBad bool
	}{
		{name: "http scheme", origins: "http://payverge.io", wantBad: true},
		{name: "null literal", origins: "null", wantBad: true},
		{name: "wildcard", origins: "*", wantBad: true},
		{name: "path", origins: "https://payverge.io/path", wantBad: true},
		{name: "trailing slash", origins: "https://payverge.io/", wantBad: true},
		{name: "userinfo", origins: "https://user@payverge.io", wantBad: true},
		{name: "https origins", origins: "https://payverge.io,https://www.payverge.io", wantBad: false},
		{name: "http non-loopback", origins: "http://example.com", wantBad: true},
		{name: "ftp loopback", origins: "ftp://localhost:3000", wantBad: true},
		// CI e2e (.github/e2e/ci.env.fixture) boots production mode on
		// localhost without TLS; loopback http mirrors validatePublicURL.
		{name: "http localhost", origins: "http://localhost:3000", wantBad: false},
		{name: "http 127.0.0.1", origins: "http://127.0.0.1:3000", wantBad: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validProductionInputs()
			in.AllowedOrigins = tt.origins
			report := ValidateProduction(in)
			if !tt.wantBad {
				assert.True(t, report.OK(), "codes=%v", report.Codes())
				assert.NotContains(t, report.Codes(), "origins.invalid")
				return
			}
			require.False(t, report.OK())
			assert.Contains(t, report.Codes(), "origins.invalid")
			var named bool
			for _, issue := range report.Issues() {
				if issue.Code != "origins.invalid" {
					continue
				}
				assert.Equal(t, "proxy", issue.Component)
				if strings.Contains(issue.Message, tt.origins) {
					named = true
				}
			}
			assert.True(t, named, "origins.invalid message must name %q; issues=%v", tt.origins, report.Issues())
		})
	}
}

func TestProductionPreflight_EmailTransportError(t *testing.T) {
	const factoryErr = `unknown email provider "log" (supported: postmark, resend)`

	t.Run("log provider the factory cannot build fails", func(t *testing.T) {
		in := validProductionInputs()
		in.EmailProvider = "log"
		in.EmailTransportError = factoryErr
		report := ValidateProduction(in)
		require.Equal(t, []string{"email.transport.unavailable"}, report.Codes())
		assert.Equal(t, "email", report.Issues()[0].Component)
		assert.Contains(t, report.Issues()[0].Message, "EMAIL_PROVIDER=log")
	})
	t.Run("missing key is reported instead of the transport error", func(t *testing.T) {
		in := validProductionInputs()
		in.EmailAPIKey = ""
		in.EmailTransportError = "email provider resend requires an API key"
		assert.Equal(t, []string{"email.api_key.missing"}, ValidateProduction(in).Codes())
	})
	t.Run("unknown provider is reported once", func(t *testing.T) {
		in := validProductionInputs()
		in.EmailProvider = "sendgrid"
		in.EmailTransportError = `unknown email provider "sendgrid"`
		assert.Equal(t, []string{"email.provider.invalid"}, ValidateProduction(in).Codes())
	})
	t.Run("no transport error keeps log a warning", func(t *testing.T) {
		in := validProductionInputs()
		in.EmailProvider = "log"
		report := ValidateProduction(in)
		assert.True(t, report.OK(), "codes: %v", report.Codes())
		assert.Contains(t, warningCodes(report), "email.provider.log")
	})
	t.Run("missing-key message points at the log provider", func(t *testing.T) {
		in := validProductionInputs()
		in.EmailAPIKey = ""
		msg := ValidateProduction(in).Issues()[0].Message
		assert.Contains(t, msg, "set EMAIL_PROVIDER=log")
		assert.NotContains(t, msg, "unset EMAIL_PROVIDER")
	})
}
