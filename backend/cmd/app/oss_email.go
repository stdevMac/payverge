package main

// Email wiring for the self-hosted (OSS) build: EMAIL_PROVIDER selection,
// sender defaults and provider-specific webhook routes. It lives outside
// main.go so the many branches that touch main.go only meet it at three call
// sites. The selection rule itself is config.EmailProvider(); see
// docs/self-hosting/email.md.

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// applyEmailFlagEnv mirrors the --email-provider and --email-api-key flags
// into the environment, so config.EmailProvider,
// config.EmailVerificationMode and every later reader (auth, the instance
// endpoint, preflight) share one resolution. A non-empty flag wins over the
// env var, which is the precedence main.go always had.
func applyEmailFlagEnv(provider, apiKey string) {
	for _, kv := range [][2]string{
		{config.EmailProviderEnv, provider},
		{config.EmailAPIKeyEnv, apiKey},
	} {
		if v := strings.TrimSpace(kv[1]); v != "" {
			_ = os.Setenv(kv[0], v)
		}
	}
}

// emailTransportPlan is what newEmailTransport decided, kept separate from
// logging so the decision is unit-testable without logger.Fatal.
type emailTransportPlan struct {
	provider  string
	transport emails.EmailProvider
	warnings  []string
}

// newEmailTransport resolves EMAIL_PROVIDER and builds the transport.
//
//   - An unknown EMAIL_PROVIDER or EMAIL_VERIFICATION value is an error.
//   - A provider chosen explicitly but missing its config (EMAIL_PROVIDER=resend
//     with no key, smtp with no SMTP_HOST) is an error in production. In
//     development it downgrades to the log provider with a warning, and
//     EMAIL_PROVIDER is rewritten to "log" so every reader agrees on the
//     transport; EMAIL_VERIFICATION=auto is then pinned to "required" (the
//     operator asked for a real transport, so the fallback must not open
//     unverified sign-up — the links are in the log with EMAIL_LOG_CONTENT=true).
//   - A delivery credential with no EMAIL_PROVIDER (EMAIL_API_KEY, SMTP_HOST,
//     SMTP_USERNAME, SMTP_PASSWORD; config.StrandedEmailCredentials) is an
//     error in production and a warning in development, where verification
//     stays "required" under auto.
//   - Nothing configured resolves to the log provider: no third-party call is
//     ever attempted.
func newEmailTransport(productionMode bool) (emailTransportPlan, error) {
	var plan emailTransportPlan
	if err := config.ValidateEmailEnv(); err != nil {
		return plan, err
	}

	provider := config.EmailProvider()
	if missing := config.EmailProviderMissingConfig(provider); missing != "" {
		if productionMode {
			return plan, fmt.Errorf("EMAIL_PROVIDER=%s needs %s; set it, or set EMAIL_PROVIDER=log to run without outbound email", provider, missing)
		}
		plan.warnings = append(plan.warnings, fmt.Sprintf(
			"EMAIL_PROVIDER=%s is missing %s; using EMAIL_PROVIDER=log for this development run (emails are not sent; only metadata is logged)",
			provider, missing))
		if setting, _ := config.ParseEmailVerificationMode(os.Getenv(config.EmailVerificationEnv)); setting == config.EmailVerificationModeAuto {
			_ = os.Setenv(config.EmailVerificationEnv, string(config.EmailVerificationModeRequired))
			plan.warnings = append(plan.warnings,
				"EMAIL_VERIFICATION=auto stays required for this fallback; verification links are written to the log only with EMAIL_LOG_CONTENT=true (set EMAIL_VERIFICATION=off to skip them)")
		}
		provider = config.EmailProviderLog
		_ = os.Setenv(config.EmailProviderEnv, provider)
	}

	if stranded := config.StrandedEmailCredentials(); len(stranded) > 0 {
		msg := fmt.Sprintf("%s set but EMAIL_PROVIDER is not; set EMAIL_PROVIDER=smtp (SMTP_*) or resend|postmark (EMAIL_API_KEY) to deliver, or EMAIL_PROVIDER=log to run without outbound email (only RESEND_API_KEY selects a provider on its own)",
			strings.Join(stranded, ", ")+pluralIsAre(len(stranded)))
		if productionMode {
			return plan, errors.New(msg)
		}
		plan.warnings = append(plan.warnings, msg+"; email verification stays required meanwhile")
	} else if provider == config.EmailProviderLog {
		for _, key := range []string{config.SMTPHostEnv, config.EmailAPIKeyEnv} {
			if strings.TrimSpace(os.Getenv(key)) != "" {
				plan.warnings = append(plan.warnings, fmt.Sprintf("%s is set but EMAIL_PROVIDER=log; it is ignored", key))
			}
		}
	}

	transport, err := emails.NewProvider(provider, config.EmailAPIKeyFor(provider))
	if err != nil {
		return plan, err
	}
	plan.provider = provider
	plan.transport = emails.NewObservedProvider(provider, transport)
	return plan, nil
}

func pluralIsAre(n int) string {
	if n == 1 {
		return " is"
	}
	return " are"
}

// defaultEmailSenders fills in sender addresses a self-host left empty:
// SMTP_FROM when set, else noreply@<public host>, else noreply@localhost.
// Hosted deployments always pass --from-email / FROM_EMAIL, so this only
// changes boxes that would otherwise have failed to start.
func defaultEmailSenders(fromTransactional, fromUpdates string) (string, string) {
	fromTransactional = strings.TrimSpace(fromTransactional)
	fromUpdates = strings.TrimSpace(fromUpdates)
	if fromTransactional == "" {
		if smtpFrom := strings.TrimSpace(os.Getenv(emails.SMTPFromEnv)); smtpFrom != "" {
			if _, err := mail.ParseAddress(smtpFrom); err == nil {
				fromTransactional = smtpFrom
			}
		}
	}
	if fromTransactional == "" {
		fromTransactional = "noreply@" + publicMailDomain()
	}
	if fromUpdates == "" {
		fromUpdates = fromTransactional
	}
	return fromTransactional, fromUpdates
}

// publicMailDomain is the host of the configured instance URL
// (config.PublicURL: PUBLIC_URL) when it is a real
// domain name, else "localhost".
func publicMailDomain() string {
	if !config.PublicURLConfigured() {
		return "localhost"
	}
	u, err := url.Parse(config.PublicURL())
	if err != nil {
		return "localhost"
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host == "" || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return "localhost"
	}
	return host
}

// buildEmailServer is main.go's single entry point for outbound email. It
// returns the server and the resolved provider name (for webhook routing).
func buildEmailServer(productionMode bool, fromTransactional, fromUpdates, templatesDir string) (*emails.EmailServer, string) {
	plan, err := newEmailTransport(productionMode)
	if err != nil {
		logger.Logger.Fatalf("Failed to create email provider: %v (see docs/self-hosting/email.md)", err)
	}
	for _, w := range plan.warnings {
		logger.Logger.Warn(w)
	}

	sender, updatesSender := defaultEmailSenders(fromTransactional, fromUpdates)
	server, err := emails.NewEmailServer(plan.transport, sender, updatesSender, templatesDir)
	if err != nil {
		logger.Logger.Fatalf("Failed to initialize email server: %v", err)
	}

	mode := config.EmailVerificationModeFor(plan.provider)
	if plan.provider == config.EmailProviderLog {
		logger.Logger.Warnf("Email provider: log (EMAIL_VERIFICATION=%s): emails are NOT delivered (the backend logs metadata only, or full content with EMAIL_LOG_CONTENT=true); set EMAIL_PROVIDER=smtp, resend or postmark to send mail", mode)
	} else {
		logger.Logger.Infof("Email provider: %s (EMAIL_VERIFICATION=%s)", plan.provider, mode)
	}
	return server, plan.provider
}

// registerEmailWebhookRoutes mounts provider delivery-event webhooks under
// /api/v1/webhooks. Only Resend has one (bounces/complaints feed the
// suppression list); Postmark has no webhook route, and smtp/log have no
// delivery events at all — the route is not registered for them, so the path
// 404s instead of accepting unauthenticated posts for a provider not in use.
func registerEmailWebhookRoutes(group gin.IRoutes, provider string) {
	registerEmailWebhookRoutesWith(group, provider, database.GetDB(), strings.TrimSpace(os.Getenv("RESEND_WEBHOOK_SECRET")))
}

func registerEmailWebhookRoutesWith(group gin.IRoutes, provider string, db *gorm.DB, resendSecret string) {
	if provider == config.EmailProviderResend {
		resendEmailWebhookHandler := emails.NewResendWebhookHandler(db, resendSecret)
		group.POST("/email/resend", resendEmailWebhookHandler.Handle)
	}
}
