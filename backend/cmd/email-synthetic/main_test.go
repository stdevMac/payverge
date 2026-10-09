package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setBaseSyntheticEnv(t *testing.T) {
	t.Helper()
	t.Setenv("EMAIL_PROVIDER", "resend")
	t.Setenv("EMAIL_API_KEY", "re_test_not_a_real_key")
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("EMAIL_VERIFICATION", "")
	t.Setenv("SMTP_HOST", "")
	t.Setenv("SMTP_PORT", "")
	t.Setenv("SMTP_USERNAME", "")
	t.Setenv("SMTP_PASSWORD", "")
	t.Setenv("SMTP_FROM", "")
	t.Setenv("SMTP_TLS", "")
	t.Setenv("FROM_EMAIL", "noreply@payverge.io")
	t.Setenv("EMAIL_SYNTHETIC_MODE", "provider")
	t.Setenv("EMAIL_SYNTHETIC_RECIPIENT", "seed@example.com")
	t.Setenv("EMAIL_SYNTHETIC_GMAIL_RECIPIENT", "")
	t.Setenv("EMAIL_SYNTHETIC_OUTLOOK_RECIPIENT", "")
}

func TestRunDryRunWithResendDoesNotSend(t *testing.T) {
	setBaseSyntheticEnv(t)
	require.NoError(t, run(true))
}

// The probe must resolve the provider exactly like the server: EMAIL_API_KEY
// alone no longer selects resend there, so the probe must not pretend it does.
func TestRunRefusesAStrandedAPIKeyThatResolvesToLog(t *testing.T) {
	setBaseSyntheticEnv(t)
	t.Setenv("EMAIL_PROVIDER", "")

	err := run(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EMAIL_API_KEY is set but EMAIL_PROVIDER is not")
	assert.Contains(t, err.Error(), "log")
}

func TestRunRefusesTheLogProvider(t *testing.T) {
	setBaseSyntheticEnv(t)
	t.Setenv("EMAIL_PROVIDER", "")
	t.Setenv("EMAIL_API_KEY", "")

	err := run(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolves to log")

	t.Setenv("EMAIL_PROVIDER", "log")
	err = run(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolves to log")
}

func TestRunRejectsAnUnknownProvider(t *testing.T) {
	setBaseSyntheticEnv(t)
	t.Setenv("EMAIL_PROVIDER", "sendgrid")

	err := run(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EMAIL_PROVIDER")
}

func TestRunResendAliasKeySelectsResendWithoutEmailProvider(t *testing.T) {
	setBaseSyntheticEnv(t)
	t.Setenv("EMAIL_PROVIDER", "")
	t.Setenv("EMAIL_API_KEY", "")
	t.Setenv("RESEND_API_KEY", "re_test_not_a_real_key")

	require.NoError(t, run(true))
}

func TestRunSMTPUsesSMTPHostAndSMTPFrom(t *testing.T) {
	setBaseSyntheticEnv(t)
	t.Setenv("EMAIL_PROVIDER", "smtp")
	t.Setenv("EMAIL_API_KEY", "")
	t.Setenv("FROM_EMAIL", "")

	err := run(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SMTP_HOST")

	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "mailer@example.com")
	require.NoError(t, run(true))
}

func TestRunDryRunRejectsInvalidSender(t *testing.T) {
	setBaseSyntheticEnv(t)
	t.Setenv("FROM_EMAIL", "not-an-address")

	err := run(true)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "from")
}

func TestRunInboxSeedRequiresBothMailboxFamilies(t *testing.T) {
	setBaseSyntheticEnv(t)
	t.Setenv("EMAIL_SYNTHETIC_MODE", "inbox-seed")
	t.Setenv("EMAIL_SYNTHETIC_GMAIL_RECIPIENT", "gmail-seed@example.com")

	err := run(true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EMAIL_SYNTHETIC_OUTLOOK_RECIPIENT")
}

func TestRunPostmarkRequiresEmailAPIKey(t *testing.T) {
	setBaseSyntheticEnv(t)
	t.Setenv("EMAIL_PROVIDER", "postmark")
	t.Setenv("EMAIL_API_KEY", "")
	require.Error(t, run(true))

	t.Setenv("EMAIL_API_KEY", "postmark-test-token")
	require.NoError(t, run(true))
}
