package config

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
)

// FrontendBaseURL returns the public (guest/staff-facing) base URL. It is an
// alias of PublicURL: PUBLIC_URL, else http://localhost:3000 (warned once).
func FrontendBaseURL() string {
	return PublicURL()
}

// APIBaseURL returns the backend origin that external callbacks (OAuth,
// webhooks) and emailed API links use: APP_BASE_URL when the API lives on its
// own host, else PUBLIC_URL (same-origin deploys serve the API at /api/v1),
// else http://localhost:8080 in development (warned once).
func APIBaseURL() string {
	if raw := strings.TrimSpace(os.Getenv("APP_BASE_URL")); raw != "" {
		if normalized, err := NormalizePublicURL(raw); err == nil {
			return normalized
		}
		return strings.TrimRight(raw, "/")
	}
	if raw := strings.TrimSpace(os.Getenv("PUBLIC_URL")); raw != "" {
		if normalized, err := NormalizePublicURL(raw); err == nil {
			return normalized
		}
		return strings.TrimRight(raw, "/")
	}
	apiURLFallbackWarnOnce.Do(func() {
		log.Println("WARNING: APP_BASE_URL and PUBLIC_URL not set, falling back to " + DefaultDevAPIBaseURL)
	})
	return DefaultDevAPIBaseURL
}

type ValidationError struct {
	Field   string
	Message string
}

var productionModeOverride atomic.Bool

func SetProductionModeOverride(enabled bool) {
	productionModeOverride.Store(enabled)
}

func IsProductionMode(productionFlag bool) bool {
	if productionFlag {
		return true
	}
	if productionModeOverride.Load() {
		return true
	}

	for _, key := range []string{"ENV", "APP_ENV"} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
		case "production", "prod":
			return true
		}
	}

	return false
}

// isKnownUnsafeJWTSecret reports a JWT signing key that is a placeholder or
// a value published in this repository (see IsKnownUnsafeSecret).
func isKnownUnsafeJWTSecret(secret string) bool {
	if lower := strings.ToLower(strings.TrimSpace(secret)); strings.HasPrefix(lower, "replace_with") || strings.HasPrefix(lower, "changeme") {
		return true
	}
	return IsKnownUnsafeSecret(secret)
}

func isValidPluginSecretKey(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}

	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		decoded, err := encoding.DecodeString(raw)
		if err == nil && len(decoded) == 32 {
			return true
		}
	}

	if len([]byte(raw)) == 32 {
		return true
	}
	// 64 hex characters (openssl rand -hex 32), decision D-2. Checked last so
	// every key the formats above accept keeps its exact derivation; no
	// 64-character hex string is a valid 32-byte base64 or raw key.
	return isHexPluginKey(raw)
}

// isHexPluginKey reports whether raw is exactly 64 hex characters.
func isHexPluginKey(raw string) bool {
	if len(raw) != 64 {
		return false
	}
	_, err := hex.DecodeString(raw)
	return err == nil
}

func ValidateConfig(dbHost, dbUser, dbPassword, dbName string, productionFlag bool) []ValidationError {
	var errors []ValidationError

	production := IsProductionMode(productionFlag)
	jwtKey := strings.TrimSpace(os.Getenv("JWT_SECRET_KEY"))
	if jwtKey == "" {
		errors = append(errors, ValidationError{
			Field:   "JWT_SECRET_KEY",
			Message: "environment variable is required",
		})
	} else if len(jwtKey) < 32 {
		errors = append(errors, ValidationError{
			Field:   "JWT_SECRET_KEY",
			Message: "must be at least 32 characters for security",
		})
	} else if production && isKnownUnsafeJWTSecret(jwtKey) {
		errors = append(errors, ValidationError{
			Field:   "JWT_SECRET_KEY",
			Message: "must not use a placeholder or a value published in this repository in production",
		})
	}

	// PLUGIN_SECRET_KEY is unconditionally required in production: without it
	// EncryptConfigSecrets is a pass-through and every plugin/fiscal credential
	// (fields matched by security.IsSecretConfigKey: *_secret, *_token,
	// *_password, api_key, ...) is persisted PLAINTEXT at rest. Outside
	// production the key is optional: development generates a random
	// per-instance key on first boot (security.EnsureDevPluginSecretKey).
	if production {
		pluginSecretKey := strings.TrimSpace(os.Getenv("PLUGIN_SECRET_KEY"))
		if pluginSecretKey == "" {
			errors = append(errors, ValidationError{
				Field:   "PLUGIN_SECRET_KEY",
				Message: "environment variable is required in production; plugin and fiscal credentials would be stored plaintext without it (generate one with: openssl rand -base64 32)",
			})
		} else if IsKnownUnsafePluginKey(pluginSecretKey) {
			errors = append(errors, ValidationError{
				Field:   "PLUGIN_SECRET_KEY",
				Message: "must not use a placeholder or a value published in this repository in production",
			})
		} else if !isValidPluginSecretKey(pluginSecretKey) {
			errors = append(errors, ValidationError{
				Field:   "PLUGIN_SECRET_KEY",
				Message: "must be 32 bytes, base64-encoded 32 bytes or 64 hex characters (generate one with: openssl rand -hex 32)",
			})
		}
	}

	if strings.TrimSpace(dbHost) == "" {
		errors = append(errors, ValidationError{Field: "db-host", Message: "is required"})
	}
	if strings.TrimSpace(dbUser) == "" {
		errors = append(errors, ValidationError{Field: "db-user", Message: "is required"})
	}
	if strings.TrimSpace(dbPassword) == "" {
		errors = append(errors, ValidationError{Field: "db-password", Message: "is required"})
	} else if production && isKnownUnsafeDatabasePassword(dbPassword) {
		errors = append(errors, ValidationError{Field: "db-password", Message: "must not use a known development default or placeholder in production"})
	}
	if strings.TrimSpace(dbName) == "" {
		errors = append(errors, ValidationError{Field: "db-name", Message: "is required"})
	}

	return errors
}

func FormatErrors(errors []ValidationError) string {
	var lines []string
	for _, e := range errors {
		lines = append(lines, fmt.Sprintf("  - %s: %s", e.Field, e.Message))
	}
	return "Configuration validation failed:\n" + strings.Join(lines, "\n")
}

// ProductionWarnings returns non-fatal configuration problems that must be
// logged loudly at boot in production. Unlike ValidateConfig errors these do
// not block startup — they flag deploys that will silently degrade:
// dead transactional email (no provider API key, or the "log" provider that
// never delivers). PLUGIN_SECRET_KEY is a hard ValidateConfig requirement in
// production and is not warned here.
//
// Provider selection: --email-provider wins, otherwise EmailProvider() (the
// shared env rule: EMAIL_PROVIDER, else resend when RESEND_API_KEY is set,
// else log; EMAIL_API_KEY alone selects nothing). cmd/app mirrors
// --email-provider and --email-api-key into those env vars (applyEmailFlagEnv)
// before this runs, so flags and env resolve identically. The log and smtp providers take
// no API key, so they never warn here (ValidateProduction fails a missing
// SMTP_HOST and reports the log sink as a warning). The key is resolved by
// PreflightEmailAPIKey. The flag arguments stay in the signature: a direct CLI
// invocation is still supported and env-only inspection would miss it.
func ProductionWarnings(productionFlag bool, emailProviderFlag, emailAPIKeyFlag string) []ValidationError {
	var warnings []ValidationError
	if !IsProductionMode(productionFlag) {
		return warnings
	}

	provider := strings.ToLower(strings.TrimSpace(emailProviderFlag))
	if provider == "" {
		provider = EmailProvider()
	}
	if provider != EmailProviderResend && provider != EmailProviderPostmark {
		return warnings
	}

	apiKey := PreflightEmailAPIKey(provider, emailAPIKeyFlag)
	if apiKey == "" {
		warnings = append(warnings, ValidationError{
			Field:   "EMAIL_API_KEY",
			Message: fmt.Sprintf("no email API key configured for provider %q in production: verification, password-reset, and lifecycle emails will fail to send", provider),
		})
	} else if provider == EmailProviderResend && !strings.HasPrefix(apiKey, "re_") {
		warnings = append(warnings, ValidationError{
			Field:   "EMAIL_API_KEY",
			Message: "EMAIL_PROVIDER=resend but the configured API key does not look like a Resend key (expected 're_' prefix) — likely a leftover Postmark token from a provider switch",
		})
	}

	return warnings
}

// PreflightEmailAPIKey returns the API key preflight checks for an API-based
// email provider: --email-api-key first, then EMAIL_API_KEY, then
// RESEND_API_KEY for resend. Providers that take no API key
// (log, smtp) resolve to "". The env lookup is EmailAPIKeyFor, the same rule
// the email transport uses; only the CLI flags are layered on top here.
func PreflightEmailAPIKey(provider, emailAPIKeyFlag string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != EmailProviderResend && provider != EmailProviderPostmark {
		return ""
	}
	if key := strings.TrimSpace(emailAPIKeyFlag); key != "" {
		return key
	}
	return EmailAPIKeyFor(provider)
}
