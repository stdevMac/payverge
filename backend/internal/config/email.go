package config

import (
	"fmt"
	"os"
	"strings"
)

// Outbound email transport selection and the email-verification policy.
//
// This file is the single source of truth for "which email provider is this
// process using" and "must new email/password accounts verify their address".
// Startup wiring (cmd/app), production preflight, the auth handlers and the
// instance-info endpoint all read these accessors instead of re-deriving the
// rule from raw env, so a self-host box with zero email configuration resolves
// the same way everywhere: provider "log", verification "off".
//
// Accessors read the environment on every call (no cache), so tests and the
// dev fallback in cmd/app can change it. EmailVerificationRequired is on the
// per-request session boundary (verification.IsOperatorVerified); it costs a
// handful of os.Getenv reads, covered by BenchmarkIsOperatorVerified.

// EmailProviderEnv names the outbound transport explicitly.
const EmailProviderEnv = "EMAIL_PROVIDER"

// EmailVerificationEnv selects the email/password verification policy.
const EmailVerificationEnv = "EMAIL_VERIFICATION"

// Supported EMAIL_PROVIDER values.
const (
	// EmailProviderLog never sends: every message is written to the process
	// log (recipient, subject, truncated body and any links). Default when no
	// email credentials are configured.
	EmailProviderLog = "log"
	// EmailProviderSMTP sends through any SMTP relay (SMTP_HOST, SMTP_PORT,
	// SMTP_USERNAME, SMTP_PASSWORD, SMTP_FROM).
	EmailProviderSMTP = "smtp"
	// EmailProviderResend sends through the Resend HTTP API.
	EmailProviderResend = "resend"
	// EmailProviderPostmark sends through the Postmark HTTP API.
	EmailProviderPostmark = "postmark"
)

// Credential env vars consulted by the provider rule.
const (
	// EmailAPIKeyEnv is the provider-neutral API key (Resend or Postmark).
	EmailAPIKeyEnv = "EMAIL_API_KEY"
	// ResendAPIKeyEnv is the Resend-specific key; setting it alone selects
	// the resend provider.
	ResendAPIKeyEnv = "RESEND_API_KEY"
	// SMTPHostEnv is the only mandatory SMTP setting.
	SMTPHostEnv = "SMTP_HOST"
	// SMTPUsernameEnv / SMTPPasswordEnv are the SMTP relay credentials.
	SMTPUsernameEnv = "SMTP_USERNAME"
	SMTPPasswordEnv = "SMTP_PASSWORD"
)

// KnownEmailProviders lists every accepted EMAIL_PROVIDER value.
var KnownEmailProviders = []string{EmailProviderLog, EmailProviderSMTP, EmailProviderResend, EmailProviderPostmark}

// EmailProvider resolves the outbound email transport for this process.
//
//  1. EMAIL_PROVIDER, when set, wins (lower-cased). Unknown values are
//     returned as-is so emails.NewProvider / ValidateEmailEnv fail loudly
//     instead of silently downgrading to "log".
//  2. Otherwise "resend" when RESEND_API_KEY is set.
//  3. Otherwise "log": nothing is sent and no third-party call is made.
//
// The provider-neutral EMAIL_API_KEY never selects a provider on its own: it
// does not say which API it belongs to. It is the key for whichever of
// resend/postmark EMAIL_PROVIDER (or a provider-specific key) picked, so a
// deployment that uses EMAIL_API_KEY sets EMAIL_PROVIDER explicitly.
//
// cmd/app mirrors the --email-provider / --email-api-key flags into the same
// env vars before this is first read, so flags and env resolve identically.
func EmailProvider() string {
	// A public demo never sends mail: anyone can trigger it (DEMO_MODE).
	if DemoModeEnabled() {
		return EmailProviderLog
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv(EmailProviderEnv))); v != "" {
		return v
	}
	if envTrimmed(ResendAPIKeyEnv) != "" {
		return EmailProviderResend
	}
	return EmailProviderLog
}

// StrandedEmailCredentials lists the delivery credentials that are set while
// EMAIL_PROVIDER is unset and the rule above still resolved to "log": an
// EMAIL_API_KEY (which never selects a provider on its own) or SMTP settings
// without EMAIL_PROVIDER=smtp. Such a box was clearly meant to send mail, so
// startup refuses it in production and EMAIL_VERIFICATION=auto resolves to
// "required" instead of "off" (fail closed). SMTP_PORT and SMTP_FROM are not
// credentials (.env.example pre-fills the port) and are ignored. An explicit
// EMAIL_PROVIDER=log is a deliberate choice and strands nothing.
func StrandedEmailCredentials() []string {
	if envTrimmed(EmailProviderEnv) != "" || EmailProvider() != EmailProviderLog {
		return nil
	}
	var stranded []string
	for _, key := range []string{EmailAPIKeyEnv, SMTPHostEnv, SMTPUsernameEnv, SMTPPasswordEnv} {
		if envTrimmed(key) != "" {
			stranded = append(stranded, key)
		}
	}
	return stranded
}

// IsKnownEmailProvider reports whether name is a supported EMAIL_PROVIDER.
func IsKnownEmailProvider(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, known := range KnownEmailProviders {
		if name == known {
			return true
		}
	}
	return false
}

// EmailAPIKeyFor returns the API key for provider: EMAIL_API_KEY first, then
// RESEND_API_KEY for resend. The Resend key is never handed to Postmark.
func EmailAPIKeyFor(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case EmailProviderResend:
		return firstEnv(EmailAPIKeyEnv, ResendAPIKeyEnv)
	case EmailProviderPostmark:
		return envTrimmed(EmailAPIKeyEnv)
	default:
		return ""
	}
}

// EmailProviderMissingConfig names the env var(s) provider still needs before
// it can send, or "" when it is fully configured. "log" never needs anything;
// unknown providers report EMAIL_PROVIDER itself.
func EmailProviderMissingConfig(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case EmailProviderLog:
		return ""
	case EmailProviderSMTP:
		if envTrimmed(SMTPHostEnv) == "" {
			return SMTPHostEnv
		}
		return ""
	case EmailProviderResend:
		if EmailAPIKeyFor(EmailProviderResend) == "" {
			return EmailAPIKeyEnv + " (or " + ResendAPIKeyEnv + ")"
		}
		return ""
	case EmailProviderPostmark:
		if EmailAPIKeyFor(EmailProviderPostmark) == "" {
			return EmailAPIKeyEnv
		}
		return ""
	default:
		return EmailProviderEnv
	}
}

// EmailVerificationModeValue is an EMAIL_VERIFICATION setting.
type EmailVerificationModeValue string

const (
	// EmailVerificationModeAuto resolves to "off" with the log provider (no
	// mail can reach the user) and "required" with any real transport, or
	// when a delivery credential is set without EMAIL_PROVIDER
	// (StrandedEmailCredentials).
	EmailVerificationModeAuto EmailVerificationModeValue = "auto"
	// EmailVerificationModeRequired keeps the hosted contract: an email/password
	// account cannot sign in until it clicks the emailed verification link.
	EmailVerificationModeRequired EmailVerificationModeValue = "required"
	// EmailVerificationModeOff signs email/password accounts in without a
	// verification link. Nothing proves the address, so the stored
	// email_verified flags stay false: switching back to "required" gates
	// those accounts again, and Google sign-in does not silently merge into
	// one that was used (409).
	EmailVerificationModeOff EmailVerificationModeValue = "off"
)

// ParseEmailVerificationMode normalises a raw EMAIL_VERIFICATION value.
// Empty means "auto". Unknown values fail closed to "required" and return an
// error so startup can refuse to boot on a typo.
func ParseEmailVerificationMode(raw string) (EmailVerificationModeValue, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(EmailVerificationModeAuto):
		return EmailVerificationModeAuto, nil
	case string(EmailVerificationModeRequired):
		return EmailVerificationModeRequired, nil
	case string(EmailVerificationModeOff):
		return EmailVerificationModeOff, nil
	default:
		return EmailVerificationModeRequired, fmt.Errorf("invalid %s %q: want auto|required|off", EmailVerificationEnv, raw)
	}
}

// ResolveEmailVerificationMode collapses a parsed setting against a provider
// into the effective policy ("required" or "off").
func ResolveEmailVerificationMode(setting EmailVerificationModeValue, provider string) EmailVerificationModeValue {
	switch setting {
	case EmailVerificationModeOff:
		return EmailVerificationModeOff
	case EmailVerificationModeAuto:
		if strings.ToLower(strings.TrimSpace(provider)) == EmailProviderLog {
			return EmailVerificationModeOff
		}
		return EmailVerificationModeRequired
	default:
		return EmailVerificationModeRequired
	}
}

// EmailVerificationMode returns the effective verification policy for this
// process: "required" or "off" (never "auto").
func EmailVerificationMode() EmailVerificationModeValue {
	return EmailVerificationModeFor(EmailProvider())
}

// EmailVerificationModeFor resolves the configured EMAIL_VERIFICATION against
// an explicit provider (used where the provider came from a flag). An invalid
// setting resolves to "required", and "auto" with a stranded delivery
// credential resolves to "required" as well: a misconfiguration never turns
// verification off.
func EmailVerificationModeFor(provider string) EmailVerificationModeValue {
	setting, _ := ParseEmailVerificationMode(os.Getenv(EmailVerificationEnv))
	if setting == EmailVerificationModeAuto && len(StrandedEmailCredentials()) > 0 {
		return EmailVerificationModeRequired
	}
	return ResolveEmailVerificationMode(setting, provider)
}

// EmailVerificationRequired reports whether email/password accounts must
// verify their address before a session is issued.
func EmailVerificationRequired() bool {
	return EmailVerificationMode() != EmailVerificationModeOff
}

// ValidateEmailEnv rejects an unknown EMAIL_PROVIDER or EMAIL_VERIFICATION so
// startup fails loudly on a typo. Missing provider credentials are reported
// separately via EmailProviderMissingConfig (dev downgrades to "log";
// production refuses to boot).
func ValidateEmailEnv() error {
	provider := EmailProvider()
	if !IsKnownEmailProvider(provider) {
		return fmt.Errorf("invalid %s %q: want %s", EmailProviderEnv, provider, strings.Join(KnownEmailProviders, "|"))
	}
	if _, err := ParseEmailVerificationMode(os.Getenv(EmailVerificationEnv)); err != nil {
		return err
	}
	return nil
}

func envTrimmed(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := envTrimmed(key); v != "" {
			return v
		}
	}
	return ""
}
