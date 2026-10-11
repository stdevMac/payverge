package config

// Instance identity for self-hosted deployments.
//
// This file is the single backend "brand-default" module: it is the only
// non-test Go file allowed to carry the upstream brand domain literal
// (scripts/check-hardcoded-domain.sh allowlists it). Everything else that
// needs a public URL, a product name, or a contact address reads it through
// the accessors below so an operator can rebrand an instance with env vars
// alone and never ship links to the upstream project's domain.
//
// Env contract (all optional outside production):
//
//	PUBLIC_URL         canonical same-origin URL (API at /api/v1, media at /media/*).
//	                   Required in production: https, or http on a loopback
//	                   host only. The CORS and redirect
//	                   allow-list defaults trust exactly the PUBLIC_URL origin,
//	                   never APP_BASE_URL and never an implicit www./apex twin.
//	PRODUCT_NAME       display name (default "Payverge").
//	COMPANY_NAME       operator company (default PRODUCT_NAME).
//	LEGAL_ENTITY       legal entity for footers/terms (default COMPANY_NAME when set, else empty).
//	SUPPORT_EMAIL      support contact (default empty: nothing is rendered).
//	SECURITY_EMAIL     security contact (default SUPPORT_EMAIL).
//	LOGO_URL           absolute http(s) URL or root-relative path (default empty).
//	BRAND_COLOR        #rgb / #rrggbb (default #1a6b6a).
//	LEGAL_TERMS_URL    absolute http(s) URL of the operator's own terms (default
//	                   empty: the frontend renders its generic template).
//	LEGAL_PRIVACY_URL  absolute http(s) URL of the operator's own privacy policy.

import (
	"fmt"
	"log"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
)

const (
	// DefaultProductName is the product name shown when PRODUCT_NAME is unset.
	DefaultProductName = "Payverge"
	// DefaultBrandColor is the upstream primary color (deep teal).
	DefaultBrandColor = "#1a6b6a"
	// DefaultDevPublicURL is the development fallback when no public URL env
	// var is set. Production refuses to boot without one (ValidateInstance).
	DefaultDevPublicURL = "http://localhost:3000"
	// DefaultDevAPIBaseURL is the development fallback for the API origin.
	DefaultDevAPIBaseURL = "http://localhost:8080"

	// UpstreamDomain is the upstream project's domain. It is NEVER a runtime
	// default for links, cookies, CORS, or redirects; tests use it to prove
	// the upstream host is rejected. Demo photography is served from the
	// instance's own storage (s3.PublicURL), not from an upstream bucket.
	UpstreamDomain = "payverge.io"
)

// publicURLEnvKeys names the instance public URL env var. PUBLIC_URL is the
// only name; there are no aliases.
var publicURLEnvKeys = []string{"PUBLIC_URL"}

var (
	publicURLFallbackWarnOnce sync.Once
	apiURLFallbackWarnOnce    sync.Once
	instanceWarnOnce          sync.Map // key -> struct{}
)

func warnInstanceOnce(key, format string, args ...any) {
	if _, loaded := instanceWarnOnce.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	log.Printf("WARNING: "+format, args...)
}

// configuredPublicURL returns the first non-empty public URL env value, the
// key it came from, and whether any was set. The value is normalized when it
// parses; a malformed value is returned trimmed so ValidateInstance can name it.
func configuredPublicURL() (value, key string, ok bool) {
	for _, k := range publicURLEnvKeys {
		raw := strings.TrimSpace(os.Getenv(k))
		if raw == "" {
			continue
		}
		if normalized, err := NormalizePublicURL(raw); err == nil {
			return normalized, k, true
		}
		return strings.TrimRight(raw, "/"), k, true
	}
	return "", "", false
}

// PublicURLSetting returns the raw (trimmed, un-normalized) value of the first
// non-empty public URL env var in the shared resolution order and the key it
// came from. Production preflight validates this raw value with its own
// stricter rules (https-only outside loopback, explicit scheme) so its error
// codes describe what the operator actually typed.
func PublicURLSetting() (raw, key string, ok bool) {
	return firstSetEnv(publicURLEnvKeys)
}

// PublicURL returns the instance's canonical public origin with no trailing
// slash, e.g. "https://pos.example.com". Falls back to http://localhost:3000
// (warning once) when nothing is configured.
func PublicURL() string {
	if v, _, ok := configuredPublicURL(); ok {
		return v
	}
	publicURLFallbackWarnOnce.Do(func() {
		log.Println("WARNING: PUBLIC_URL not set, falling back to " + DefaultDevPublicURL)
	})
	return DefaultDevPublicURL
}

// PublicURLConfigured reports whether an operator set PUBLIC_URL (as opposed
// to the development default).
func PublicURLConfigured() bool {
	_, _, ok := configuredPublicURL()
	return ok
}

// PublicHost returns the host (with port, if any) of PublicURL, lowercased,
// e.g. "pos.example.com".
func PublicHost() string {
	u, err := url.Parse(PublicURL())
	if err != nil || u.Host == "" {
		return "localhost:3000"
	}
	return strings.ToLower(u.Host)
}

// trustedOriginEnvKeys name the browser-facing origin that CORS and the
// redirect allow-list trust by default. APP_BASE_URL (the API origin) is
// deliberately excluded: it must never become a trusted credentialed browser
// origin by accident.
var trustedOriginEnvKeys = []string{"PUBLIC_URL"}

// configuredTrustedOrigin returns the normalized origin of the first non-empty
// trusted-origin env var. A malformed first value yields nothing (fail
// closed) instead of falling through to a later key; ValidateInstance names it.
func configuredTrustedOrigin() (*url.URL, bool) {
	for _, k := range trustedOriginEnvKeys {
		raw := strings.TrimSpace(os.Getenv(k))
		if raw == "" {
			continue
		}
		normalized, err := NormalizePublicURL(raw)
		if err != nil {
			return nil, false
		}
		u, err := url.Parse(normalized)
		if err != nil || u.Hostname() == "" {
			return nil, false
		}
		return u, true
	}
	return nil, false
}

// trustedPublicHostname returns the hostname (no port) of the configured
// trusted origin, or "" when only the development default applies.
func trustedPublicHostname() string {
	u, ok := configuredTrustedOrigin()
	if !ok {
		return ""
	}
	return normalizeRedirectHost(u.Hostname())
}

// PublicOrigins returns the CORS allow-list default: exactly the origin of
// PUBLIC_URL, nothing else. Empty when it is not set. The www./apex counterpart is NOT added: an operator who serves both
// lists them in ALLOWED_ORIGINS, so a dangling or taken-over www record never
// becomes a trusted origin the operator did not declare.
func PublicOrigins() []string {
	u, ok := configuredTrustedOrigin()
	if !ok {
		return nil
	}
	return []string{u.Scheme + "://" + u.Host}
}

// NormalizePublicURL validates and canonicalizes a public origin: absolute
// http(s), lowercase scheme and host, no userinfo/query/fragment, no path
// other than "/", no trailing slash. A bare host ("pos.example.com") is read
// as https.
func NormalizePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("is empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("is not a valid URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("must use http or https")
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("must include a host")
	}
	if u.User != nil {
		return "", fmt.Errorf("must not include credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return "", fmt.Errorf("must not include a query or fragment")
	}
	if p := strings.TrimRight(u.Path, "/"); p != "" {
		return "", fmt.Errorf("must be an origin without a path (the API is served at /api/v1 on the same origin)")
	}
	return scheme + "://" + strings.ToLower(u.Host), nil
}

// ProductName is the instance display name (PRODUCT_NAME, default "Payverge").
func ProductName() string {
	if v := cleanDisplayValue(os.Getenv("PRODUCT_NAME"), 80); v != "" {
		return v
	}
	return DefaultProductName
}

// CompanyName is the operating company (COMPANY_NAME, default ProductName).
func CompanyName() string {
	if v := cleanDisplayValue(os.Getenv("COMPANY_NAME"), 120); v != "" {
		return v
	}
	return ProductName()
}

// LegalEntity is the legal entity for footers and terms (LEGAL_ENTITY, else
// an explicitly set COMPANY_NAME, else empty so nothing is claimed).
func LegalEntity() string {
	if v := cleanDisplayValue(os.Getenv("LEGAL_ENTITY"), 160); v != "" {
		return v
	}
	return cleanDisplayValue(os.Getenv("COMPANY_NAME"), 120)
}

// SupportEmail is the support contact (SUPPORT_EMAIL). Empty when unset or
// invalid so templates omit the line rather than invent an address.
func SupportEmail() string {
	return validEmailEnv("SUPPORT_EMAIL")
}

// SecurityEmail is the security contact (SECURITY_EMAIL, default SupportEmail).
func SecurityEmail() string {
	if v := validEmailEnv("SECURITY_EMAIL"); v != "" {
		return v
	}
	return SupportEmail()
}

// LogoURL is an absolute http(s) URL or a root-relative path (LOGO_URL).
// Anything else (javascript:, data:, protocol-relative) is dropped.
func LogoURL() string {
	raw := strings.TrimSpace(os.Getenv("LOGO_URL"))
	if raw == "" {
		return ""
	}
	if ok := validLogoURL(raw); !ok {
		warnInstanceOnce("LOGO_URL", "LOGO_URL must be an absolute http(s) URL or a root-relative path; ignoring it")
		return ""
	}
	return raw
}

// LegalTermsURL is the operator's own terms-of-service page (LEGAL_TERMS_URL).
// Only an absolute http(s) URL is accepted: the frontend redirects /terms to
// it, so a relative value could loop back to /terms.
func LegalTermsURL() string { return legalURLEnv("LEGAL_TERMS_URL") }

// LegalPrivacyURL is the operator's own privacy policy (LEGAL_PRIVACY_URL).
func LegalPrivacyURL() string { return legalURLEnv("LEGAL_PRIVACY_URL") }

func legalURLEnv(key string) string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return ""
	}
	if !validAbsoluteHTTPURL(raw) {
		warnInstanceOnce(key, "%s must be an absolute http(s) URL; ignoring it", key)
		return ""
	}
	return raw
}

var brandColorPattern = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// BrandColor is the primary hex color (BRAND_COLOR, default #1a6b6a).
func BrandColor() string {
	raw := strings.TrimSpace(os.Getenv("BRAND_COLOR"))
	if raw == "" {
		return DefaultBrandColor
	}
	if !strings.HasPrefix(raw, "#") {
		raw = "#" + raw
	}
	if !brandColorPattern.MatchString(raw) {
		warnInstanceOnce("BRAND_COLOR", "BRAND_COLOR must be a #rgb or #rrggbb hex color; using %s", DefaultBrandColor)
		return DefaultBrandColor
	}
	return strings.ToLower(raw)
}

// ValidateInstance checks the instance identity env. errors block startup in
// production; warnings are logged. Production requires PUBLIC_URL. Outside
// production a malformed PUBLIC_URL is still an error (it
// would break every link we send) but a missing one is fine (development
// default).
func ValidateInstance(productionFlag bool) (errs []ValidationError, warnings []ValidationError) {
	production := IsProductionMode(productionFlag)

	if raw, key, ok := firstSetEnv(publicURLEnvKeys); ok {
		normalized, err := NormalizePublicURL(raw)
		switch {
		case err != nil:
			errs = append(errs, ValidationError{Field: key, Message: err.Error()})
		case production && !productionPublicScheme(normalized):
			errs = append(errs, ValidationError{Field: key, Message: "must use https in production (http is accepted only for localhost)"})
		}
	} else if production {
		errs = append(errs, ValidationError{Field: "PUBLIC_URL", Message: "is required in production (the https origin guests and staff use, e.g. https://pos.example.com)"})
	}

	if raw := strings.TrimSpace(os.Getenv("BRAND_COLOR")); raw != "" && !brandColorPattern.MatchString(ensureHash(raw)) {
		warnings = append(warnings, ValidationError{Field: "BRAND_COLOR", Message: "must be a #rgb or #rrggbb hex color; using the default"})
	}
	if raw := strings.TrimSpace(os.Getenv("LOGO_URL")); raw != "" && !validLogoURL(raw) {
		warnings = append(warnings, ValidationError{Field: "LOGO_URL", Message: "must be an absolute http(s) URL or a root-relative path; ignoring it"})
	}
	for _, key := range []string{"LEGAL_TERMS_URL", "LEGAL_PRIVACY_URL"} {
		if raw := strings.TrimSpace(os.Getenv(key)); raw != "" && !validAbsoluteHTTPURL(raw) {
			warnings = append(warnings, ValidationError{Field: key, Message: "must be an absolute http(s) URL; ignoring it"})
		}
	}
	for _, key := range []string{"SUPPORT_EMAIL", "SECURITY_EMAIL"} {
		if raw := strings.TrimSpace(os.Getenv(key)); raw != "" && validEmailEnv(key) == "" {
			warnings = append(warnings, ValidationError{Field: key, Message: "is not a valid email address; ignoring it"})
		}
	}
	if production && SupportEmail() == "" {
		warnings = append(warnings, ValidationError{Field: "SUPPORT_EMAIL", Message: "not set; emails and error pages will not show a support contact"})
	}
	return errs, warnings
}

// productionPublicScheme applies the production scheme rule to a normalized
// public origin: https, or plain http on a loopback host. It is the same
// exception parsePublicOrigin grants the production preflight, so the
// production-mode CI stack and laptop rehearsals boot on http://localhost.
func productionPublicScheme(normalized string) bool {
	if strings.HasPrefix(normalized, "https://") {
		return true
	}
	u, err := url.Parse(normalized)
	return err == nil && u.Scheme == "http" && isLoopbackHost(u.Hostname())
}

func firstSetEnv(keys []string) (value, key string, ok bool) {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v, k, true
		}
	}
	return "", "", false
}

func ensureHash(raw string) string {
	if strings.HasPrefix(raw, "#") {
		return raw
	}
	return "#" + raw
}

func validLogoURL(raw string) bool {
	if strings.HasPrefix(raw, "/") {
		return !strings.HasPrefix(raw, "//") && !strings.Contains(raw, `\`)
	}
	return validAbsoluteHTTPURL(raw)
}

func validAbsoluteHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return (scheme == "https" || scheme == "http") && u.Hostname() != "" && u.User == nil
}

func validEmailEnv(key string) string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return ""
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Name != "" || addr.Address != raw {
		warnInstanceOnce(key, "%s is not a valid bare email address; ignoring it", key)
		return ""
	}
	return addr.Address
}

// cleanDisplayValue trims, drops control characters (no header injection via
// CR/LF in a From name), and caps length.
func cleanDisplayValue(raw string, max int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if runes := []rune(out); len(runes) > max {
		out = strings.TrimSpace(string(runes[:max]))
	}
	return out
}
