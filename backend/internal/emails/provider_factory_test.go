package emails

import (
	"strings"
	"testing"
)

func TestNewProviderSelectionTable(t *testing.T) {
	for _, key := range []string{SMTPHostEnv, SMTPPortEnv, SMTPUsernameEnv, SMTPPasswordEnv, SMTPFromEnv, SMTPTLSEnv} {
		t.Setenv(key, "")
	}
	t.Setenv(SMTPHostEnv, "mail.example.test")

	cases := []struct {
		name, provider, key string
		wantType            string
		wantErr             string
	}{
		{"blank without key logs instead of calling Resend", "", "", "*emails.LogProvider", ""},
		{"blank with key keeps legacy resend default", "", "re_x", "*emails.ResendProvider", ""},
		{"explicit log", "log", "", "*emails.LogProvider", ""},
		{"explicit log ignores a key", " LOG ", "re_x", "*emails.LogProvider", ""},
		{"smtp from env", "smtp", "", "*emails.SMTPProvider", ""},
		{"resend with key", "resend", "re_x", "*emails.ResendProvider", ""},
		{"resend without key refuses to build", "resend", "  ", "", "requires an API key"},
		{"postmark with key", "postmark", "pm", "*emails.PostmarkProvider", ""},
		{"postmark without key refuses to build", "postmark", "", "", "requires an API key"},
		{"unknown lists every provider", "sendgrid", "k", "", "log, smtp, resend, postmark"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewProvider(tc.provider, tc.key)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err=%v want containing %q", err, tc.wantErr)
				}
				if p != nil {
					t.Fatalf("provider must be nil on error, got %T", p)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := typeName(p); got != tc.wantType {
				t.Fatalf("got %s want %s", got, tc.wantType)
			}
		})
	}
}

func TestNewProviderSMTPWithoutHostFails(t *testing.T) {
	t.Setenv(SMTPHostEnv, "")
	p, err := NewProvider("smtp", "")
	if err == nil || !strings.Contains(err.Error(), SMTPHostEnv) {
		t.Fatalf("err=%v want SMTP_HOST error", err)
	}
	if p != nil {
		t.Fatalf("provider must be a true nil on error, got %T", p)
	}
}

func typeName(p EmailProvider) string {
	switch p.(type) {
	case *LogProvider:
		return "*emails.LogProvider"
	case *SMTPProvider:
		return "*emails.SMTPProvider"
	case *ResendProvider:
		return "*emails.ResendProvider"
	case *PostmarkProvider:
		return "*emails.PostmarkProvider"
	default:
		return "unknown"
	}
}
