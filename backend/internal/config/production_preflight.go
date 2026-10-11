package config

import (
	"encoding/base64"
	"net/mail"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// PreflightIssue is a single production configuration problem. Message must
// never contain a raw secret value — only codes and human-readable guidance.
type PreflightIssue struct {
	Code      string
	Component string
	Message   string
}

// ProductionInputs carries resolved configuration values for production
// preflight. Fields are plain resolved values passed in from main (flags +
// env), not re-read from the environment inside ValidateProduction.
//
// Only the core secrets and PUBLIC_URL are always required. Every optional
// integration (email provider, S3, Cloudflare edge, Telegram, ...)
// is validated only when it is selected, so a self-hosted production
// instance boots with PUBLIC_URL, DB_PASSWORD, JWT_SECRET_KEY,
// PLUGIN_SECRET_KEY, ADMIN_EMAIL and ADMIN_PASSWORD alone.
type ProductionInputs struct {
	Production bool
	DBPassword string

	JWTSecretKey    string
	PluginSecretKey string
	// RPCURL is the raw RPC_URL; empty means DefaultRPCURL. RPC problems are
	// warnings — an unreachable RPC only disables crypto settlement.
	RPCURL string

	// PublicURL is the canonical public origin (PUBLIC_URL).
	PublicURL string

	// AdminPassword is ADMIN_PASSWORD. Requiredness belongs to the admin
	// bootstrap; preflight only rejects placeholder and published values.
	AdminPassword string

	// RegistrationMode is the raw REGISTRATION_MODE. Empty means the default
	// (invite); any value ParseRegistrationMode rejects fails preflight,
	// because at runtime it silently resolves to "closed".
	RegistrationMode string

	// EmailProvider is the resolved provider (--email-provider flag, else
	// EmailProvider()). "" is treated as "log".
	EmailProvider           string
	EmailAPIKey             string
	FromEmail               string
	FromEmailUpdates        string
	EmailAllowedFromDomains string
	EmailWebhookSecret      string
	EmailSPFDomain          string
	EmailDKIMDomain         string
	EmailReturnPathDomain   string
	EmailDMARCPolicy        string
	// EmailLogContent is EMAIL_LOG_CONTENT. With the log provider it writes
	// recipients and token-bearing links. Fatal in production unless
	// EmailLogAllowProduction is also set.
	EmailLogContent bool
	// EmailLogAllowProduction is EMAIL_PROVIDER_LOG_ALLOW_PRODUCTION.
	// It turns email.log_content.production into a warning.
	EmailLogAllowProduction bool

	// SMTP settings, validated only when EmailProvider is "smtp".
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string

	// EmailTransportError is the error the email provider factory returned
	// for EmailProvider/EmailAPIKey (set by cmd/app in production). A non-empty
	// value fails preflight when no more specific email issue was reported, so
	// preflight never passes a provider the runtime cannot build.
	EmailTransportError string

	// StorageDriver is STORAGE_DRIVER: "" / "local" (default) or "s3". S3
	// credentials below are validated only for "s3".
	StorageDriver string

	// Public S3 / object storage
	S3Bucket        string
	AWSAccessKey    string
	AWSSecretKey    string
	S3Endpoint      string
	S3PublicBaseURL string

	// Protected S3 / object storage
	S3ProtectedBucket     string
	AWSProtectedAccessKey string
	AWSProtectedSecretKey string
	S3ProtectedEndpoint   string

	// AllowedOrigins is ALLOWED_ORIGINS; empty derives from PublicURL.
	AllowedOrigins string
	CookieDomain   string

	// Edge is EDGE: "" / "none" (default) or "cloudflare".
	Edge string

	// Reverse-proxy trust (TRUSTED_PLATFORM, TRUSTED_PROXIES). An empty
	// TrustedProxies means DefaultTrustedProxies (loopback only).
	TrustedPlatform string
	TrustedProxies  string

	// Raw budget strings. Empty uses the startup default; a set value must
	// parse as a positive decimal.
	AIDailyBudgetUSD string
	// Optional instance-wide caps; empty is derived, set must be valid.
	AIBudgetGlobalUSD    string
	AIBudgetGuestPoolUSD string

	// OpenRouterZDRMode is empty when startup will apply its production default
	// (enforce). An explicit audit value is never safe in production.
	OpenRouterAPIKey  string
	OpenRouterZDRMode string

	// PaymentSecrets contains optional provider/webhook credentials keyed only
	// by their configuration name. Non-empty values must not be placeholders.
	PaymentSecrets map[string]string

	// Optional fiscal endpoint overrides. Production normally leaves these
	// empty and uses provider-owned live endpoints; test/homologation overrides
	// must never reach production.
	FiscalWSAAURL string
	FiscalWSFEURL string

	TelegramWebhookEnabled bool
	TelegramWebhookSecret  string

	// Wave 4 fiscal delivery worker settings (required positive in production).
	// Zero means "use default" and is accepted; negative is rejected.
	FiscalDeliveryWorkerIntervalSeconds int
	FiscalDeliveryWorkerConcurrency     int
}

// Report is a non-secret typed preflight result.
type Report struct {
	issues   []PreflightIssue
	warnings []PreflightIssue
}

// OK reports whether production preflight found no issues.
// Warnings alone do not fail the report.
func (r Report) OK() bool {
	return len(r.issues) == 0
}

// Codes returns issue codes in deterministic order (a defensive copy).
func (r Report) Codes() []string {
	out := make([]string, len(r.issues))
	for i, issue := range r.issues {
		out[i] = issue.Code
	}
	return out
}

// Issues returns a defensive copy of preflight issues in deterministic order.
func (r Report) Issues() []PreflightIssue {
	out := make([]PreflightIssue, len(r.issues))
	copy(out, r.issues)
	return out
}

// Warnings returns a defensive copy of non-fatal preflight warnings.
func (r Report) Warnings() []PreflightIssue {
	out := make([]PreflightIssue, len(r.warnings))
	copy(out, r.warnings)
	return out
}

func (r *Report) add(code, component, message string) {
	r.issues = append(r.issues, PreflightIssue{
		Code:      code,
		Component: component,
		Message:   message,
	})
}

func (r *Report) addWarning(code, component, message string) {
	r.warnings = append(r.warnings, PreflightIssue{
		Code:      code,
		Component: component,
		Message:   message,
	})
}

// ValidateProduction validates configuration safety for production deploys.
// When in.Production is false the report is always OK. When true, the core
// secrets and PUBLIC_URL are always checked, and each optional integration is
// checked only when it is selected (per-component validators below). Issues
// are returned in a fixed component order so operators and tests see stable
// codes.
//
// Messages never include raw secret values.
func ValidateProduction(in ProductionInputs) Report {
	var r Report
	if !in.Production {
		return r
	}

	validateCoreSecrets(&r, in)
	validatePublicURL(&r, in)
	validateNetwork(&r, in)
	validateEmail(&r, in)
	validateStorage(&r, in)
	validateProxyTrust(&r, in)
	validateAIBudgets(&r, in)
	validateAIPrivacy(&r, in)
	validatePaymentSecrets(&r, in)
	validateWebhooks(&r, in)
	validateFiscal(&r, in)

	return r
}

// validateCoreSecrets covers the always-required secrets: database password,
// JWT signing key, plugin encryption key, plus a published-value check on the
// bootstrap admin password when one is configured.
func validateCoreSecrets(r *Report, in ProductionInputs) {
	// --- database ---
	if isKnownUnsafeDatabasePassword(in.DBPassword) {
		r.add("database.password.unsafe", "database", "database password must not use a known development default or placeholder in production")
	}

	// --- jwt ---
	jwt := strings.TrimSpace(in.JWTSecretKey)
	switch {
	case jwt == "":
		r.add("jwt.secret.missing", "jwt", "JWT_SECRET_KEY is required in production (generate one with: openssl rand -base64 48)")
	case len(jwt) < 32:
		r.add("jwt.secret.short", "jwt", "JWT_SECRET_KEY must be at least 32 characters")
	case isKnownUnsafeJWTSecret(jwt):
		r.add("jwt.secret.unsafe", "jwt", "JWT_SECRET_KEY must not use a placeholder or a value published in this repository (compose defaults, examples, CI fixtures)")
	}

	// --- plugin encryption key ---
	pluginKey := strings.TrimSpace(in.PluginSecretKey)
	switch {
	case pluginKey == "":
		r.add("plugin_secret.missing", "plugin", "PLUGIN_SECRET_KEY is required in production so plugin and fiscal credentials are encrypted at rest (generate one with: openssl rand -base64 32)")
	case isPlaceholderValue(pluginKey):
		r.add("plugin_secret.placeholder", "plugin", "PLUGIN_SECRET_KEY must not use a placeholder value in production")
	case IsKnownUnsafePluginKey(pluginKey):
		r.add("plugin_secret.unsafe", "plugin", "PLUGIN_SECRET_KEY must not use a value published in this repository (CI fixtures or the retired development fallback)")
	case !isValidPluginSecretKey(pluginKey):
		r.add("plugin_secret.invalid", "plugin", "PLUGIN_SECRET_KEY must be 32 bytes, base64-encoded 32 bytes or 64 hex characters (generate one with: openssl rand -hex 32)")
	}

	// --- bootstrap admin (requiredness owned by the admin bootstrap) ---
	if IsKnownUnsafeSecret(in.AdminPassword) {
		r.add("admin.password.unsafe", "admin", "ADMIN_PASSWORD must not use a placeholder or a password published in this repository")
	}

	// --- registration mode ---
	// The raw value is not echoed: it is operator input, and the message
	// only needs to name the accepted values.
	if _, err := ParseRegistrationMode(in.RegistrationMode); err != nil {
		r.add("registration_mode.invalid", "registration", "REGISTRATION_MODE must be invite, open or closed (or unset for the default, invite); an unknown value would silently close signup")
	}
}

// validatePublicURL requires PUBLIC_URL to be an https origin. It is the
// single source the CORS allow-list and absolute links derive from.
//
// The one deliberate exception is plain http on a loopback host
// (localhost, *.localhost, 127/8, ::1): it only warns. Production-mode CI
// stacks and laptop trials run there, browsers treat loopback as a secure
// context, and no remote site can be served from a loopback origin, so the
// derived CORS allow-list admits only pages on the same machine. Every other
// http origin is public_url.insecure.
func validatePublicURL(r *Report, in ProductionInputs) {
	origin, code, message := parsePublicOrigin(in.PublicURL)
	if code != "" {
		r.add(code, "proxy", message)
		return
	}
	if strings.HasPrefix(origin, "http://") {
		r.addWarning("public_url.loopback", "proxy", "PUBLIC_URL points at a loopback host over http; the instance is reachable only from this machine")
	}
}

// validateNetwork never fails startup: without a reachable RPC the instance
// still serves every non-crypto flow, so RPC problems are warnings.
func validateNetwork(r *Report, in ProductionInputs) {
	raw := strings.TrimSpace(in.RPCURL)
	if raw == "" {
		r.addWarning("rpc_url.default", "network", "RPC_URL is not set; USDC verification uses the public Base RPC "+DefaultRPCURL)
		return
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		r.addWarning("rpc_url.invalid", "network", "RPC_URL is not a valid URL; USDC settlement will be unavailable")
		return
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "ws", "wss":
	default:
		r.addWarning("rpc_url.invalid", "network", "RPC_URL must use http(s) or ws(s); USDC settlement will be unavailable")
	}
}

// validateEmail checks only the selected provider. "log" (the default when
// no provider key is configured) needs nothing and only warns: mail is not
// delivered, and only metadata is logged unless EMAIL_LOG_CONTENT is set.
func validateEmail(r *Report, in ProductionInputs) {
	provider := strings.ToLower(strings.TrimSpace(in.EmailProvider))
	issuesBefore := len(r.issues)
	defer func() {
		transportErr := strings.TrimSpace(in.EmailTransportError)
		if transportErr == "" || len(r.issues) != issuesBefore {
			return
		}
		name := provider
		if name == "" {
			name = "log"
		}
		r.add("email.transport.unavailable", "email", "EMAIL_PROVIDER="+name+" cannot be started by this build ("+transportErr+"); choose a provider this build supports and configure it")
	}()
	switch provider {
	case "", "log":
		r.addWarning("email.provider.log", "email", "EMAIL_PROVIDER=log: transactional email is not delivered and only metadata is logged; set EMAIL_PROVIDER=smtp, resend or postmark to send mail")
		switch {
		case in.Production && in.EmailLogContent && !in.EmailLogAllowProduction:
			r.add("email.log_content.production", "email", "EMAIL_LOG_CONTENT=true writes recipients and token-bearing links (password reset, verification, invitations) to the production log; configure a real EMAIL_PROVIDER, unset EMAIL_LOG_CONTENT, or set EMAIL_PROVIDER_LOG_ALLOW_PRODUCTION=true to accept that risk")
		case in.Production && in.EmailLogContent && in.EmailLogAllowProduction:
			r.addWarning("email.log_content.allowed", "email", "EMAIL_LOG_CONTENT=true writes recipients and token-bearing links to the production log; EMAIL_PROVIDER_LOG_ALLOW_PRODUCTION=true accepts that risk")
		}
	case "resend":
		validateAPIEmailProvider(r, in, "RESEND_API_KEY")
		validateResendAlignment(r, in)
	case "postmark":
		validateAPIEmailProvider(r, in, "EMAIL_API_KEY")
	case "smtp":
		validateSMTP(r, in)
	default:
		r.add("email.provider.invalid", "email", "EMAIL_PROVIDER must be one of log, smtp, resend, postmark")
	}
}

func validateAPIEmailProvider(r *Report, in ProductionInputs, keyEnv string) {
	provider := strings.ToLower(strings.TrimSpace(in.EmailProvider))
	apiKey := strings.TrimSpace(in.EmailAPIKey)
	switch {
	case apiKey == "":
		source := "EMAIL_API_KEY / --email-api-key"
		if keyEnv != "EMAIL_API_KEY" {
			source = keyEnv + " (or " + source + ")"
		}
		r.add("email.api_key.missing", "email", "EMAIL_PROVIDER="+provider+" requires "+source+", or set EMAIL_PROVIDER=log to run without outbound email")
	case isPlaceholderValue(apiKey):
		r.add("email.api_key.placeholder", "email", "the email provider API key must not use a placeholder value in production")
	case provider == "resend" && !strings.HasPrefix(apiKey, "re_"):
		r.add("email.api_key.provider_mismatch", "email", "the email API key must be a Resend API key (re_ prefix) when EMAIL_PROVIDER=resend")
	}
	if strings.TrimSpace(in.FromEmail) == "" || strings.TrimSpace(in.FromEmailUpdates) == "" {
		r.add("email.from.missing", "email", "FROM_EMAIL and FROM_EMAIL_UPDATES (or --from-email / --from-email-updates) are required when EMAIL_PROVIDER="+provider)
	} else if anyPlaceholder(in.FromEmail, in.FromEmailUpdates) {
		r.add("email.from.placeholder", "email", "production sender addresses must not use placeholder values")
	}
}

// validateResendAlignment is the Resend-only sender-domain, webhook, and
// SPF/DKIM/DMARC alignment block.
func validateResendAlignment(r *Report, in ProductionInputs) {
	approved := approvedEmailDomains(in.EmailAllowedFromDomains)
	if len(approved) == 0 {
		r.add("email.sender_domains.missing", "email", "EMAIL_ALLOWED_FROM_DOMAINS is required when EMAIL_PROVIDER=resend")
	} else {
		for _, from := range []string{in.FromEmail, in.FromEmailUpdates} {
			domain := senderDomain(from)
			_, ok := approved[domain]
			if domain == "" || !ok {
				r.add("email.from_domain.unapproved", "email", "FROM_EMAIL and FROM_EMAIL_UPDATES must use a domain listed in EMAIL_ALLOWED_FROM_DOMAINS")
				break
			}
		}
	}
	secret := strings.TrimSpace(in.EmailWebhookSecret)
	if secret == "" {
		r.add("email.webhook_secret.missing", "email", "RESEND_WEBHOOK_SECRET is required so delivery events are authenticated")
	} else if !validResendWebhookSecret(secret) {
		r.add("email.webhook_secret.invalid", "email", "RESEND_WEBHOOK_SECRET must be a valid whsec_ signing secret")
	}
	dnsDomains := []string{
		strings.TrimSpace(in.EmailSPFDomain),
		strings.TrimSpace(in.EmailDKIMDomain),
		strings.TrimSpace(in.EmailReturnPathDomain),
	}
	missingDNS := false
	invalidDNS := false
	for _, domain := range dnsDomains {
		if domain == "" {
			missingDNS = true
			continue
		}
		if !domainWithinApprovedEmailDomains(domain, approved) {
			invalidDNS = true
		}
	}
	if missingDNS {
		r.add("email.dns_alignment.missing", "email", "EMAIL_SPF_DOMAIN, EMAIL_DKIM_DOMAIN, and EMAIL_RETURN_PATH_DOMAIN are required for Resend alignment")
	} else if invalidDNS {
		r.add("email.dns_alignment.invalid", "email", "SPF, DKIM, and return-path domains must be the approved sender domain or its subdomain")
	}
	switch strings.ToLower(strings.TrimSpace(in.EmailDMARCPolicy)) {
	case "quarantine", "reject":
	default:
		r.add("email.dmarc_policy.invalid", "email", "EMAIL_DMARC_POLICY must be quarantine or reject in production")
	}
}

func validateSMTP(r *Report, in ProductionInputs) {
	if strings.TrimSpace(in.SMTPHost) == "" {
		r.add("email.smtp.host.missing", "email", "SMTP_HOST is required when EMAIL_PROVIDER=smtp")
	} else if isPlaceholderValue(in.SMTPHost) {
		r.add("email.smtp.host.placeholder", "email", "SMTP_HOST must not use a placeholder value in production")
	}
	if port := strings.TrimSpace(in.SMTPPort); port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			r.add("email.smtp.port.invalid", "email", "SMTP_PORT must be a TCP port between 1 and 65535 (default 587)")
		}
	}
	if IsKnownUnsafeSecret(in.SMTPPassword) {
		r.add("email.smtp.password.placeholder", "email", "SMTP_PASSWORD must not use a placeholder or published value in production")
	}
	from := strings.TrimSpace(in.SMTPFrom)
	if from == "" {
		from = strings.TrimSpace(in.FromEmail)
	}
	switch {
	case from == "":
		r.add("email.from.missing", "email", "SMTP_FROM (or FROM_EMAIL) is required when EMAIL_PROVIDER=smtp")
	case isPlaceholderValue(from):
		r.add("email.from.placeholder", "email", "production sender addresses must not use placeholder values")
	case senderDomain(from) == "":
		r.add("email.from.invalid", "email", "SMTP_FROM (or FROM_EMAIL) must be a valid email address")
	}
}

// validateStorage checks S3 credentials only when STORAGE_DRIVER=s3; the
// default local driver needs no credentials.
func validateStorage(r *Report, in ProductionInputs) {
	switch strings.ToLower(strings.TrimSpace(in.StorageDriver)) {
	case "", "local":
		if strings.TrimSpace(in.S3Bucket) != "" || strings.TrimSpace(in.S3ProtectedBucket) != "" {
			r.addWarning("storage.s3.ignored", "storage", "S3 buckets are configured but STORAGE_DRIVER is not s3; uploads use the local volume and S3 settings are not validated")
		}
		return
	case "s3":
	default:
		r.add("storage.driver.invalid", "storage", "STORAGE_DRIVER must be local or s3")
		return
	}

	if strings.TrimSpace(in.S3Bucket) == "" ||
		strings.TrimSpace(in.AWSAccessKey) == "" ||
		strings.TrimSpace(in.AWSSecretKey) == "" {
		r.add("s3.public.missing", "storage", "STORAGE_DRIVER=s3 requires S3_BUCKET, AWS_ACCESS_KEY, and AWS_SECRET_KEY for the public bucket")
	} else if anyPlaceholder(in.S3Bucket, in.AWSAccessKey, in.AWSSecretKey, in.S3Endpoint, in.S3PublicBaseURL) {
		r.add("s3.public.placeholder", "storage", "public S3 configuration must not use placeholder values in production")
	}
	// The protected bucket is optional (s3.Init): without S3_PROTECTED_BUCKET,
	// protected objects share S3_BUCKET under the "protected/" prefix, and
	// AWS_PROTECTED_* fall back to the public credentials. Startup probes an
	// anonymous read of a protected object and refuses to boot production if
	// it succeeds, so the security property is enforced there, not here.
	protectedBucket := strings.TrimSpace(in.S3ProtectedBucket)
	protectedKey := strings.TrimSpace(in.AWSProtectedAccessKey)
	protectedSecret := strings.TrimSpace(in.AWSProtectedSecretKey)
	if protectedBucket == "" {
		r.addWarning("s3.protected.shared", "storage", "S3_PROTECTED_BUCKET is empty: protected objects share S3_BUCKET under the protected/ prefix, so that bucket must not allow anonymous reads (startup verifies this)")
	}
	switch {
	case (protectedKey == "") != (protectedSecret == ""):
		// A lone protected key would be paired with the public secret (or
		// vice versa) and fail every protected read and write.
		r.add("s3.protected.missing", "storage", "set both AWS_PROTECTED_ACCESS_KEY and AWS_PROTECTED_SECRET_KEY, or neither to reuse the public bucket credentials")
	case anyPlaceholder(in.S3ProtectedBucket, in.AWSProtectedAccessKey, in.AWSProtectedSecretKey, in.S3ProtectedEndpoint):
		r.add("s3.protected.placeholder", "storage", "protected S3 configuration must not use placeholder values in production")
	}
}

// validateProxyTrust covers CORS origins, cookie scope, the network edge,
// and reverse-proxy trust.
func validateProxyTrust(r *Report, in ProductionInputs) {
	// ALLOWED_ORIGINS defaults to the PUBLIC_URL origin, so it is only
	// missing when neither is set (PUBLIC_URL problems are reported above).
	if strings.TrimSpace(in.AllowedOrigins) == "" && strings.TrimSpace(in.PublicURL) == "" {
		r.add("origins.missing", "proxy", "ALLOWED_ORIGINS is required in production when PUBLIC_URL is not set")
	}
	for _, bad := range invalidAllowedOrigins(in.AllowedOrigins) {
		r.add("origins.invalid", "proxy", "ALLOWED_ORIGINS entry "+strconv.Quote(bad)+" must be an https origin with no userinfo, path, query, or fragment")
	}
	// Empty COOKIE_DOMAIN is preferred (host-only cookies scoped to the API
	// host). A leading-dot / bare-parent domain would send session cookies to
	// every subdomain and expands the theft surface if any sibling is compromised.
	if code, msg := validateCookieDomain(in.CookieDomain); code != "" {
		r.add(code, "proxy", msg)
	}

	edge := strings.ToLower(strings.TrimSpace(in.Edge))
	switch edge {
	case "", "none":
		edge = "none"
	case "cloudflare":
	default:
		r.add("edge.invalid", "proxy", "EDGE must be none or cloudflare")
	}

	// TRUSTED_PLATFORM makes Gin read the client IP from a CDN header
	// (CF-Connecting-IP) sent by ANY peer. It is never safe for Gin itself:
	//
	//   - EDGE=cloudflare (Wave 2 topology): Caddy is the only component that
	//     talks to Cloudflare. It verifies CF-Connecting-IP against checked-in
	//     Cloudflare CIDRs and forwards the canonical client IP to Gin over the
	//     private compose network, so Gin trusts ONLY that network via
	//     TRUSTED_PROXIES and TRUSTED_PLATFORM must stay blank.
	//   - EDGE=none: there is no CDN at all, so the header is attacker-chosen
	//     and would let any client forge its IP past every rate limit.
	//
	// The Cloudflare rule (and its message) applies only behind
	// EDGE=cloudflare. Under EDGE=none the value is still fatal by design:
	// server.ConfigureTrustedPlatform accepts only "cloudflare", so any
	// TRUSTED_PLATFORM there trusts a forgeable header, and leaving it blank
	// costs a non-CDN install nothing.
	//
	// DEFERRED (deployment): live through-Cloudflare synthetic that records
	// two known source IPs and proves the backend sees distinct safe ClientIP
	// values without echoing them publicly.
	if strings.TrimSpace(in.TrustedPlatform) != "" {
		if edge == "cloudflare" {
			r.add("cloudflare.platform.unexpected", "proxy", "TRUSTED_PLATFORM must be empty in production; Caddy terminates Cloudflare identity and Gin must trust only the private Caddy network via TRUSTED_PROXIES")
		} else {
			r.add("proxy.platform.unexpected", "proxy", "TRUSTED_PLATFORM must be empty: without a CDN edge (EDGE=none) the client-IP header it trusts can be forged by anyone; trust the reverse proxy through TRUSTED_PROXIES instead")
		}
	}
	// Empty TRUSTED_PROXIES means DefaultTrustedProxies (loopback only). An
	// explicit value must still stay inside private/loopback space.
	if proxies := strings.TrimSpace(in.TrustedProxies); proxies != "" {
		switch validatePrivateTrustedProxies(proxies) {
		case "":
		case "invalid":
			r.add("cloudflare.proxies.invalid", "proxy", "TRUSTED_PROXIES contains an invalid IP address or CIDR")
		default:
			r.add("cloudflare.proxies.public", "proxy", "TRUSTED_PROXIES may contain only private or loopback CIDRs; public or unbounded proxy trust permits forged client IPs")
		}
	}
}

// invalidAllowedOrigins returns ALLOWED_ORIGINS entries that are not bare
// https origins (or http origins on a loopback host). Empty entries are skipped. A bare origin is https://host
// with no userinfo, path (including "/"), query, or fragment. The literals
// "*" and "null" (any case) are rejected.
func invalidAllowedOrigins(raw string) []string {
	var bad []string
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !allowedOriginOK(entry) {
			bad = append(bad, entry)
		}
	}
	return bad
}

func allowedOriginOK(entry string) bool {
	if entry == "*" || strings.EqualFold(entry, "null") {
		return false
	}
	parsed, err := url.Parse(entry)
	if err != nil {
		return false
	}
	if parsed.Host == "" || parsed.User != nil {
		return false
	}
	// Plain http is tolerated only for loopback hosts, mirroring
	// validatePublicURL: a localhost-bound production boot (CI e2e,
	// prod-local) has no TLS edge in front of it.
	switch parsed.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(parsed.Hostname()) {
			return false
		}
	default:
		return false
	}
	if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return true
}

// validateAIBudgets rejects explicit but unusable spend caps. Empty values
// fall back to the startup defaults, which are always positive.
func validateAIBudgets(r *Report, in ProductionInputs) {
	if value := strings.TrimSpace(in.AIDailyBudgetUSD); value != "" && !isPositiveDecimal(value) {
		r.add("ai_budget.daily.invalid", "ai_budget", "AI_DAILY_BUDGET_USD must be a positive decimal when set (unset uses the default cap)")
	}
	validateOptionalAIBudgetCaps(r, in)
}

func validateAIPrivacy(r *Report, in ProductionInputs) {
	if value := strings.TrimSpace(in.OpenRouterAPIKey); value != "" && isPlaceholderValue(value) {
		r.add("openrouter.api_key.placeholder", "ai_privacy", "OPENROUTER_API_KEY must not use a placeholder value in production")
	}
	if strings.EqualFold(strings.TrimSpace(in.OpenRouterZDRMode), "audit") {
		r.add("openrouter.zdr_mode.audit", "ai_privacy", "OPENROUTER_ZDR_MODE=audit is not permitted in production; use enforce")
	}
}

// validatePaymentSecrets checks optional provider/webhook credentials only
// when they are set.
func validatePaymentSecrets(r *Report, in ProductionInputs) {
	paymentNames := make([]string, 0, len(in.PaymentSecrets))
	for name := range in.PaymentSecrets {
		paymentNames = append(paymentNames, name)
	}
	sort.Strings(paymentNames)
	for _, name := range paymentNames {
		value := strings.TrimSpace(in.PaymentSecrets[name])
		if value != "" && isPlaceholderValue(value) {
			r.add("payment.secret.placeholder", "payment", "payment credentials and webhook signing secrets must not use placeholder values in production")
			break
		}
	}
}

// validateWebhooks checks inbound webhook secrets only when enabled.
func validateWebhooks(r *Report, in ProductionInputs) {
	if in.TelegramWebhookEnabled && strings.TrimSpace(in.TelegramWebhookSecret) == "" {
		r.add("webhook.telegram.secret.missing", "network", "TELEGRAM_WEBHOOK_SECRET is required when Telegram webhook is enabled")
	}
}

// validateFiscal checks the Wave 4 delivery worker and endpoint overrides.
// Zero worker values mean "default at startup"; negative is a
// misconfiguration that would silently disable or thrash the durable loop.
func validateFiscal(r *Report, in ProductionInputs) {
	if in.FiscalDeliveryWorkerIntervalSeconds < 0 {
		r.add("fiscal_delivery.interval.invalid", "fiscal", "FISCAL_DELIVERY_WORKER_INTERVAL_SECONDS must be >= 0 in production (0 uses the default)")
	}
	if in.FiscalDeliveryWorkerConcurrency < 0 {
		r.add("fiscal_delivery.concurrency.invalid", "fiscal", "FISCAL_DELIVERY_WORKER_CONCURRENCY must be >= 0 in production (0 uses the default)")
	}
	for _, endpoint := range []string{in.FiscalWSAAURL, in.FiscalWSFEURL} {
		if strings.TrimSpace(endpoint) != "" && !isSafeProductionFiscalURL(endpoint) {
			r.add("fiscal.endpoint.unsafe", "fiscal", "fiscal endpoint overrides in production must be valid HTTPS live-provider URLs, not local, test, or homologation endpoints")
			break
		}
	}
}

func isKnownUnsafeDatabasePassword(raw string) bool {
	// Empty is not "unsafe": requiredness remains owned by ValidateConfig.
	return IsKnownUnsafeSecret(raw)
}

func isPlaceholderValue(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return false
	}
	for _, marker := range []string{
		"replace_with", "replace-me", "replace_me", "changeme", "change-me",
		"placeholder", "your_api_key", "your-secret", "example_only",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

// validateCookieDomain returns a preflight (code, message) when COOKIE_DOMAIN
// is unsafe. Empty means host-only cookies (OK). Leading-dot parent domains
// like ".example.com" are rejected because SameSite does not isolate sibling
// subdomains. A concrete host (api.example.com) is allowed.
func validateCookieDomain(raw string) (code, message string) {
	domain := strings.TrimSpace(raw)
	if domain == "" {
		return "", ""
	}
	if strings.HasPrefix(domain, ".") {
		return "cookie_domain.parent_scope", "COOKIE_DOMAIN must not use a leading-dot parent domain (e.g. .example.com); leave it empty for host-only cookies or set the concrete API host (api.example.com)"
	}
	// Reject bare registrable parents that would still cover every subdomain
	// (browsers treat Domain=example.com like Domain=.example.com).
	if strings.Count(domain, ".") == 1 && !strings.Contains(domain, ":") {
		labels := strings.Split(domain, ".")
		if len(labels) == 2 && labels[0] != "" && labels[1] != "" {
			return "cookie_domain.parent_scope", "COOKIE_DOMAIN must not be a parent domain that covers all subdomains; leave it empty for host-only cookies or set the concrete API host (api.example.com)"
		}
	}
	return "", ""
}

func anyPlaceholder(values ...string) bool {
	for _, value := range values {
		if isPlaceholderValue(value) {
			return true
		}
	}
	return false
}

func isSafeProductionFiscalURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1" {
		return false
	}
	for _, marker := range []string{"homo", "sandbox", "staging", "test", "example"} {
		if strings.Contains(host, marker) {
			return false
		}
	}
	return true
}

func approvedEmailDomains(raw string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		if domain := strings.ToLower(strings.TrimSpace(item)); domain != "" {
			out[domain] = struct{}{}
		}
	}
	return out
}

func senderDomain(raw string) string {
	address, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	at := strings.LastIndex(address.Address, "@")
	if at < 0 || at == len(address.Address)-1 {
		return ""
	}
	return strings.ToLower(address.Address[at+1:])
}

func validResendWebhookSecret(secret string) bool {
	encoded, ok := strings.CutPrefix(strings.TrimSpace(secret), "whsec_")
	if !ok || encoded == "" {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	return err == nil && len(decoded) >= 16
}

func domainWithinApprovedEmailDomains(domain string, approved map[string]struct{}) bool {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	for parent := range approved {
		if domain == parent || strings.HasSuffix(domain, "."+parent) {
			return true
		}
	}
	return false
}

// validatePrivateTrustedProxies returns "invalid", "public", or an empty
// string. A prefix must fit wholly inside one RFC1918, IPv6 ULA, or loopback
// block; checking IsPrivate on only its first address would incorrectly accept
// broad prefixes such as 10.0.0.0/7 that also cover public space.
func validatePrivateTrustedProxies(raw string) string {
	allowed := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("fc00::/7"),
		netip.MustParsePrefix("::1/128"),
	}

	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return "invalid"
		}

		var prefix netip.Prefix
		if strings.Contains(item, "/") {
			parsed, err := netip.ParsePrefix(item)
			if err != nil {
				return "invalid"
			}
			prefix = parsed.Masked()
		} else {
			addr, err := netip.ParseAddr(item)
			if err != nil {
				return "invalid"
			}
			addr = addr.Unmap()
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}

		contained := false
		for _, block := range allowed {
			if prefix.Addr().BitLen() == block.Addr().BitLen() &&
				prefix.Bits() >= block.Bits() && block.Contains(prefix.Addr()) {
				contained = true
				break
			}
		}
		if !contained {
			return "public"
		}
	}
	return ""
}

func isPositiveDecimal(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return false
	}
	return f > 0
}
