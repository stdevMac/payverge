// Command email-synthetic sends a uniquely tagged probe message through the
// same email provider stack the app uses (emails.NewProvider). Use it as an
// external synthetic monitor for deliverability — readiness probes must never
// send mail themselves.
//
// Required env:
//
//	EMAIL_PROVIDER              (smtp|resend|postmark; resolved exactly like the
//	                             server via config.EmailProvider(), so an unset
//	                             value picks resend when RESEND_API_KEY is set, and
//	                             otherwise log, which this probe refuses)
//	EMAIL_API_KEY               (or RESEND_API_KEY for resend; smtp
//	                             reads SMTP_HOST, SMTP_PORT, SMTP_USERNAME,
//	                             SMTP_PASSWORD instead)
//	FROM_EMAIL                  (SMTP_FROM is accepted for smtp)
//	EMAIL_SYNTHETIC_RECIPIENT   (provider-acceptance mode)
//	EMAIL_SYNTHETIC_MODE        (set inbox-seed for four critical templates)
//	EMAIL_SYNTHETIC_GMAIL_RECIPIENT / EMAIL_SYNTHETIC_OUTLOOK_RECIPIENT
//
// Flags:
//
//	--dry-run   validate configuration without sending
//
// Exit codes: 0 on success, nonzero on any configuration or provider error.
// Never logs or prints the API key.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/emails"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "validate configuration without sending")
	flag.Parse()

	if err := run(*dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "email-synthetic: %v\n", err)
		os.Exit(1)
	}
}

func run(dryRun bool) error {
	providerName, apiKey, err := resolveSyntheticProvider()
	if err != nil {
		return err
	}

	fromEmail := strings.TrimSpace(os.Getenv("FROM_EMAIL"))
	if fromEmail == "" && providerName == config.EmailProviderSMTP {
		fromEmail = strings.TrimSpace(os.Getenv(emails.SMTPFromEnv))
	}
	if fromEmail == "" {
		return fmt.Errorf("FROM_EMAIL (or SMTP_FROM for smtp) is required")
	}

	mode := strings.ToLower(strings.TrimSpace(os.Getenv("EMAIL_SYNTHETIC_MODE")))
	if mode == "" {
		mode = "provider"
	}
	var recipients []string
	if mode == "inbox-seed" {
		for _, key := range []string{"EMAIL_SYNTHETIC_GMAIL_RECIPIENT", "EMAIL_SYNTHETIC_OUTLOOK_RECIPIENT"} {
			value := strings.TrimSpace(os.Getenv(key))
			if value == "" {
				return fmt.Errorf("%s is required in inbox-seed mode", key)
			}
			recipients = append(recipients, value)
		}
	} else if mode == "provider" {
		recipient := strings.TrimSpace(os.Getenv("EMAIL_SYNTHETIC_RECIPIENT"))
		if recipient == "" {
			return fmt.Errorf("EMAIL_SYNTHETIC_RECIPIENT is required")
		}
		recipients = []string{recipient}
	} else {
		return fmt.Errorf("EMAIL_SYNTHETIC_MODE must be provider or inbox-seed")
	}

	// Same constructor the app uses in cmd/app/main.go — do not reimplement
	// provider transport here.
	provider, err := emails.NewProvider(providerName, apiKey)
	if err != nil {
		return fmt.Errorf("email provider: %w", err)
	}

	tag := fmt.Sprintf("synthetic-%s-%d", uuid.NewString(), time.Now().UTC().Unix())
	// Log only provider name, recipient, and tag — never the API key.
	fmt.Printf("email-synthetic: provider=%s mode=%s recipient_count=%d tag=%s dry_run=%v\n",
		providerName, mode, len(recipients), tag, dryRun)

	for _, recipient := range recipients {
		if _, err := emails.BuildSyntheticMessage(fromEmail, recipient); err != nil {
			return err
		}
	}

	if dryRun {
		fmt.Println("email-synthetic: dry-run ok (provider constructible, sender and recipients valid, provider credentials present)")
		return nil
	}
	if mode == "inbox-seed" {
		templatesDir := strings.TrimSpace(os.Getenv("EMAIL_TEMPLATES_DIR"))
		if templatesDir == "" {
			templatesDir = "email/templates"
		}
		server, err := emails.NewEmailServer(provider, fromEmail, fromEmail, templatesDir)
		if err != nil {
			return fmt.Errorf("inbox seed server: %w", err)
		}
		if err := emails.SendCriticalInboxSeed(server, recipients, tag); err != nil {
			return fmt.Errorf("inbox seed failed: %w", err)
		}
		fmt.Printf("email-synthetic: inbox seed accepted tag=%s messages=%d\n", tag, len(recipients)*4)
		return nil
	}

	msg, err := emails.BuildSyntheticMessage(fromEmail, recipients[0])
	if err != nil {
		return err
	}
	msg.Subject = fmt.Sprintf("[Payverge synthetic] %s", tag)
	msg.TextBody = fmt.Sprintf(
		"Payverge email synthetic probe.\n\nTag: %s\nTimestamp (UTC): %s\n",
		tag,
		time.Now().UTC().Format(time.RFC3339),
	)
	msg.HTMLBody = fmt.Sprintf(
		"<p>Payverge email synthetic probe.</p><p>Tag: <code>%s</code></p><p>Timestamp (UTC): %s</p>",
		tag,
		time.Now().UTC().Format(time.RFC3339),
	)

	msg.Tag = "synthetic"

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := provider.Send(ctx, msg); err != nil {
		return fmt.Errorf("provider send failed: %w", err)
	}

	fmt.Printf("email-synthetic: sent ok tag=%s\n", tag)
	return nil
}

// resolveSyntheticProvider picks the provider the same way the server and
// cmd/email-smoke do (config.EmailProvider), so the probe never tests a
// different transport than the one production mail goes through. It refuses
// "log": a probe that "succeeds" without delivering would hide an outage.
func resolveSyntheticProvider() (string, string, error) {
	if err := config.ValidateEmailEnv(); err != nil {
		return "", "", err
	}
	providerName := config.EmailProvider()
	if providerName == config.EmailProviderLog {
		if stranded := config.StrandedEmailCredentials(); len(stranded) > 0 {
			return "", "", fmt.Errorf("%s is set but EMAIL_PROVIDER is not, so the provider resolves to log, which never delivers; set EMAIL_PROVIDER=smtp, resend or postmark", strings.Join(stranded, ", "))
		}
		return "", "", fmt.Errorf("EMAIL_PROVIDER resolves to log, which never delivers; configure smtp, resend or postmark first")
	}
	if missing := config.EmailProviderMissingConfig(providerName); missing != "" {
		return "", "", fmt.Errorf("EMAIL_PROVIDER=%s needs %s", providerName, missing)
	}
	return providerName, config.EmailAPIKeyFor(providerName), nil
}
