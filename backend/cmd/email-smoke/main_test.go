package main

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/emails"
)

func clearSmokeEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		config.EmailProviderEnv, config.EmailVerificationEnv, config.EmailAPIKeyEnv,
		config.ResendAPIKeyEnv,
		emails.SMTPHostEnv, emails.SMTPFromEnv, "FROM_EMAIL", "EMAIL_HEALTHCHECK_TO",
	} {
		t.Setenv(key, "")
	}
}

func TestResolveSmokeTarget(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		provider string
		wantErr  bool
	}{
		{name: "nothing configured is refused (log)", env: nil, wantErr: true},
		{name: "explicit log is refused", env: map[string]string{"EMAIL_PROVIDER": "log", "FROM_EMAIL": "a@x.test", "EMAIL_HEALTHCHECK_TO": "b@x.test"}, wantErr: true},
		{
			name:     "hosted resend contract still works",
			env:      map[string]string{"EMAIL_PROVIDER": "resend", "EMAIL_API_KEY": "re_x", "FROM_EMAIL": "a@x.test", "EMAIL_HEALTHCHECK_TO": "b@x.test"},
			provider: "resend",
		},
		{
			name:     "resend inferred from RESEND_API_KEY",
			env:      map[string]string{"RESEND_API_KEY": "re_x", "FROM_EMAIL": "a@x.test", "EMAIL_HEALTHCHECK_TO": "b@x.test"},
			provider: "resend",
		},
		{
			name:     "smtp needs no API key and can use SMTP_FROM",
			env:      map[string]string{"EMAIL_PROVIDER": "smtp", "SMTP_HOST": "mail.x.test", "SMTP_FROM": "a@x.test", "EMAIL_HEALTHCHECK_TO": "b@x.test"},
			provider: "smtp",
		},
		{name: "smtp without host", env: map[string]string{"EMAIL_PROVIDER": "smtp", "FROM_EMAIL": "a@x.test", "EMAIL_HEALTHCHECK_TO": "b@x.test"}, wantErr: true},
		{name: "missing recipient", env: map[string]string{"EMAIL_PROVIDER": "smtp", "SMTP_HOST": "mail.x.test", "FROM_EMAIL": "a@x.test"}, wantErr: true},
		{name: "unknown provider", env: map[string]string{"EMAIL_PROVIDER": "sendgrid"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearSmokeEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got, err := resolveSmokeTarget()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.provider != tc.provider {
				t.Fatalf("provider=%q want %q", got.provider, tc.provider)
			}
		})
	}
}
