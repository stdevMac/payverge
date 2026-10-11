// Command email-smoke sends one synthetic message through the configured
// email provider and exits non-zero if the provider refuses it. Self-hosters
// can run it inside the backend container to check SMTP settings:
//
//	docker compose exec backend /app/email-smoke
//
// The provider is resolved exactly like the server does (config.EmailProvider),
// so EMAIL_PROVIDER may be left unset when an API key implies it.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/emails"
)

type smokeTarget struct {
	provider string
	apiKey   string
	from     string
	to       string
}

// resolveSmokeTarget reads the env the smoke probe needs. FROM_EMAIL may be
// omitted for smtp when SMTP_FROM is set. The log provider is refused: a
// probe that cannot deliver anything would report a false success.
func resolveSmokeTarget() (smokeTarget, error) {
	if err := config.ValidateEmailEnv(); err != nil {
		return smokeTarget{}, err
	}
	t := smokeTarget{
		provider: config.EmailProvider(),
		from:     strings.TrimSpace(os.Getenv("FROM_EMAIL")),
		to:       strings.TrimSpace(os.Getenv("EMAIL_HEALTHCHECK_TO")),
	}
	t.apiKey = config.EmailAPIKeyFor(t.provider)
	if t.provider == config.EmailProviderLog {
		return t, errors.New("EMAIL_PROVIDER resolves to log, which never delivers; configure smtp, resend or postmark first")
	}
	if missing := config.EmailProviderMissingConfig(t.provider); missing != "" {
		return t, fmt.Errorf("EMAIL_PROVIDER=%s needs %s", t.provider, missing)
	}
	if t.from == "" && t.provider == config.EmailProviderSMTP {
		t.from = strings.TrimSpace(os.Getenv(emails.SMTPFromEnv))
	}
	if t.from == "" || t.to == "" {
		return t, errors.New("FROM_EMAIL (or SMTP_FROM for smtp) and EMAIL_HEALTHCHECK_TO are required")
	}
	return t, nil
}

func main() {
	target, err := resolveSmokeTarget()
	if err != nil {
		log.Fatal(err)
	}
	provider, err := emails.NewProvider(target.provider, target.apiKey)
	if err != nil {
		log.Fatal(err)
	}
	message, err := emails.BuildSyntheticMessage(target.from, target.to)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if err := provider.Send(ctx, message); err != nil {
		log.Fatalf("email synthetic failed: %v", err)
	}
	log.Printf("email synthetic delivered via %s", target.provider)
}
