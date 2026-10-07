package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/emails"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ossEmailEnvKeys = []string{
	config.EmailProviderEnv, config.EmailVerificationEnv, config.EmailAPIKeyEnv,
	config.ResendAPIKeyEnv,
	emails.SMTPHostEnv, emails.SMTPPortEnv, emails.SMTPUsernameEnv, emails.SMTPPasswordEnv,
	emails.SMTPFromEnv, emails.SMTPTLSEnv,
	"PUBLIC_URL", "FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL",
}

// clearOSSEmailEnv blanks every email-related variable for one test (t.Setenv
// restores the originals afterwards).
func clearOSSEmailEnv(t *testing.T) {
	t.Helper()
	for _, key := range ossEmailEnvKeys {
		t.Setenv(key, "")
	}
}

func innerTransport(t *testing.T, p emails.EmailProvider) string {
	t.Helper()
	observed, ok := p.(*emails.ObservedProvider)
	require.True(t, ok, "transport must be wrapped for metrics, got %T", p)
	return observed.ProviderName()
}

func TestNewEmailTransportFreshBootUsesTheLogProvider(t *testing.T) {
	clearOSSEmailEnv(t)
	for _, production := range []bool{false, true} {
		plan, err := newEmailTransport(production)
		require.NoError(t, err)
		assert.Equal(t, "log", plan.provider)
		assert.Equal(t, "log", innerTransport(t, plan.transport))
		assert.Empty(t, plan.warnings)
	}
	assert.Equal(t, config.EmailVerificationModeOff, config.EmailVerificationMode(),
		"a box with no email config must not require verification it cannot deliver")
}

func TestNewEmailTransportSelection(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "resend key infers resend", env: map[string]string{config.ResendAPIKeyEnv: "re_test"}, want: "resend"},
		{name: "EMAIL_API_KEY with explicit resend", env: map[string]string{config.EmailProviderEnv: "resend", config.EmailAPIKeyEnv: "re_test"}, want: "resend"},
		{name: "explicit smtp", env: map[string]string{config.EmailProviderEnv: "smtp", emails.SMTPHostEnv: "mail.example.test"}, want: "smtp"},
		{name: "explicit log beats a key", env: map[string]string{config.EmailProviderEnv: "log", config.ResendAPIKeyEnv: "re_test"}, want: "log"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearOSSEmailEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			plan, err := newEmailTransport(true)
			require.NoError(t, err)
			assert.Equal(t, tc.want, plan.provider)
			assert.Equal(t, tc.want, innerTransport(t, plan.transport))
		})
	}
}

func TestNewEmailTransportMissingConfig(t *testing.T) {
	for _, tc := range []struct {
		provider string
		missing  string
	}{
		{provider: "resend", missing: "EMAIL_API_KEY"},
		{provider: "postmark", missing: "EMAIL_API_KEY"},
		{provider: "smtp", missing: "SMTP_HOST"},
	} {
		t.Run(tc.provider+"/production fails", func(t *testing.T) {
			clearOSSEmailEnv(t)
			t.Setenv(config.EmailProviderEnv, tc.provider)
			_, err := newEmailTransport(true)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.missing)
		})
		t.Run(tc.provider+"/development downgrades to log", func(t *testing.T) {
			clearOSSEmailEnv(t)
			t.Setenv(config.EmailProviderEnv, tc.provider)
			plan, err := newEmailTransport(false)
			require.NoError(t, err)
			assert.Equal(t, "log", plan.provider)
			require.Len(t, plan.warnings, 2)
			assert.Contains(t, plan.warnings[0], tc.missing)
			assert.Contains(t, plan.warnings[1], "required")
			assert.Equal(t, "log", os.Getenv(config.EmailProviderEnv), "env must agree with the transport in use")
			// EMAIL-5: the operator asked for a real transport; the dev
			// fallback must not open unverified sign-up.
			assert.Equal(t, config.EmailVerificationModeRequired, config.EmailVerificationMode())
		})
		t.Run(tc.provider+"/development fallback keeps an explicit off", func(t *testing.T) {
			clearOSSEmailEnv(t)
			t.Setenv(config.EmailProviderEnv, tc.provider)
			t.Setenv(config.EmailVerificationEnv, "off")
			plan, err := newEmailTransport(false)
			require.NoError(t, err)
			require.Len(t, plan.warnings, 1)
			assert.Equal(t, config.EmailVerificationModeOff, config.EmailVerificationMode())
		})
	}
}

func TestNewEmailTransportRejectsInvalidValues(t *testing.T) {
	clearOSSEmailEnv(t)
	t.Setenv(config.EmailProviderEnv, "sendgrid")
	_, err := newEmailTransport(false)
	require.Error(t, err)

	clearOSSEmailEnv(t)
	t.Setenv(config.EmailVerificationEnv, "sometimes")
	_, err = newEmailTransport(false)
	require.Error(t, err)

	clearOSSEmailEnv(t)
	t.Setenv(config.EmailProviderEnv, "smtp")
	t.Setenv(emails.SMTPHostEnv, "mail.example.test")
	t.Setenv(emails.SMTPPortEnv, "not-a-port")
	_, err = newEmailTransport(false)
	require.Error(t, err, "an invalid SMTP value is a misconfiguration, not a missing one")
}

// EMAIL-2: a delivery credential without EMAIL_PROVIDER is a box that was
// meant to send mail. Production refuses to boot; development boots with the
// log provider but keeps verification required (fail closed).
func TestNewEmailTransportStrandedCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "EMAIL_API_KEY alone", env: map[string]string{config.EmailAPIKeyEnv: "re_secret_value"}, want: "EMAIL_API_KEY is set"},
		{name: "SMTP_HOST alone", env: map[string]string{emails.SMTPHostEnv: "mail.example.test"}, want: "SMTP_HOST is set"},
		{name: "SMTP credentials without host", env: map[string]string{emails.SMTPUsernameEnv: "u", emails.SMTPPasswordEnv: "pw_secret_value"}, want: "SMTP_USERNAME, SMTP_PASSWORD are set"},
	} {
		t.Run(tc.name+"/production fails", func(t *testing.T) {
			clearOSSEmailEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, err := newEmailTransport(true)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Contains(t, err.Error(), "EMAIL_PROVIDER=log")
			assert.NotContains(t, err.Error(), "secret_value", "credential values are never logged")
		})
		t.Run(tc.name+"/development warns and keeps verification required", func(t *testing.T) {
			clearOSSEmailEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			plan, err := newEmailTransport(false)
			require.NoError(t, err)
			assert.Equal(t, "log", plan.provider)
			require.Len(t, plan.warnings, 1)
			assert.Contains(t, plan.warnings[0], tc.want)
			assert.NotContains(t, plan.warnings[0], "secret_value")
			assert.Equal(t, config.EmailVerificationModeRequired, config.EmailVerificationMode())
		})
	}
}

func TestNewEmailTransportExplicitLogIgnoresCredentials(t *testing.T) {
	clearOSSEmailEnv(t)
	t.Setenv(config.EmailProviderEnv, "log")
	t.Setenv(config.EmailAPIKeyEnv, "re_test")
	plan, err := newEmailTransport(true)
	require.NoError(t, err, "EMAIL_PROVIDER=log is a deliberate choice")
	assert.Equal(t, "log", plan.provider)
	require.Len(t, plan.warnings, 1)
	assert.Contains(t, plan.warnings[0], "EMAIL_API_KEY is set but EMAIL_PROVIDER=log")
	assert.Equal(t, config.EmailVerificationModeOff, config.EmailVerificationMode())
}

func TestApplyEmailFlagEnvFlagsWinOverEnv(t *testing.T) {
	clearOSSEmailEnv(t)
	t.Setenv(config.EmailProviderEnv, "log")
	applyEmailFlagEnv(" smtp ", "")
	assert.Equal(t, "smtp", os.Getenv(config.EmailProviderEnv))
	assert.Equal(t, "", os.Getenv(config.EmailAPIKeyEnv), "an empty flag must not clobber the env")
}

func TestDefaultEmailSenders(t *testing.T) {
	clearOSSEmailEnv(t)
	from, updates := defaultEmailSenders("", "")
	assert.Equal(t, "noreply@localhost", from)
	assert.Equal(t, from, updates)

	t.Setenv("PUBLIC_URL", "https://www.Bistro.example/")
	from, updates = defaultEmailSenders("", "")
	assert.Equal(t, "noreply@bistro.example", from)
	assert.Equal(t, from, updates)

	t.Setenv("PUBLIC_URL", "http://192.168.1.20:3000")
	from, _ = defaultEmailSenders("", "")
	assert.Equal(t, "noreply@localhost", from, "an IP host is not a mail domain")

	t.Setenv(emails.SMTPFromEnv, "Bistro <hello@bistro.example>")
	from, _ = defaultEmailSenders("", "")
	assert.Equal(t, "Bistro <hello@bistro.example>", from)

	from, updates = defaultEmailSenders("noreply@hosted.example", "updates@hosted.example")
	assert.Equal(t, "noreply@hosted.example", from, "explicit senders are never replaced")
	assert.Equal(t, "updates@hosted.example", updates)
}

func TestRegisterEmailWebhookRoutesOnlyForResend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, provider := range []string{"resend", "postmark", "smtp", "log"} {
		t.Run(provider, func(t *testing.T) {
			r := gin.New()
			registerEmailWebhookRoutesWith(r.Group("/api/v1/webhooks"), provider, nil, "")

			var mounted bool
			for _, route := range r.Routes() {
				if route.Method == http.MethodPost && route.Path == "/api/v1/webhooks/email/resend" {
					mounted = true
				}
			}
			assert.Equal(t, provider == "resend", mounted)

			if provider != "resend" {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/email/resend", strings.NewReader(`{"type":"email.bounced"}`))
				r.ServeHTTP(w, req)
				assert.Equal(t, http.StatusNotFound, w.Code)
			}
		})
	}
}
