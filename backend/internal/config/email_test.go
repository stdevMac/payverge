package config

import "testing"

// emailEnvKeys are every env var the email accessors read. Tests blank them
// all first so the developer's shell cannot leak into the table.
var emailEnvKeys = []string{
	EmailProviderEnv, EmailVerificationEnv, EmailAPIKeyEnv, ResendAPIKeyEnv,
	SMTPHostEnv, SMTPUsernameEnv, SMTPPasswordEnv,
	"SMTP_PORT", "SMTP_FROM",
}

func clearEmailEnv(t *testing.T) {
	t.Helper()
	for _, key := range emailEnvKeys {
		t.Setenv(key, "")
	}
}

func TestEmailProviderSelection(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"nothing configured defaults to log", nil, EmailProviderLog},
		{"whitespace-only values count as unset", map[string]string{EmailProviderEnv: "  ", EmailAPIKeyEnv: " "}, EmailProviderLog},
		{"RESEND_API_KEY implies resend", map[string]string{ResendAPIKeyEnv: "re_x"}, EmailProviderResend},
		{"EMAIL_API_KEY alone selects nothing", map[string]string{EmailAPIKeyEnv: "re_x"}, EmailProviderLog},
		{"SMTP_HOST alone does not imply smtp", map[string]string{SMTPHostEnv: "mail.example.com"}, EmailProviderLog},
		{"explicit smtp", map[string]string{EmailProviderEnv: "smtp"}, EmailProviderSMTP},
		{"explicit value is case-insensitive", map[string]string{EmailProviderEnv: " SMTP "}, EmailProviderSMTP},
		{"explicit log wins over a key", map[string]string{EmailProviderEnv: "log", ResendAPIKeyEnv: "re_x"}, EmailProviderLog},
		{"explicit postmark wins over resend key", map[string]string{EmailProviderEnv: "postmark", EmailAPIKeyEnv: "re_x"}, EmailProviderPostmark},
		{"explicit resend without key stays resend", map[string]string{EmailProviderEnv: "resend"}, EmailProviderResend},
		{"unknown value is surfaced, not downgraded", map[string]string{EmailProviderEnv: "sendgrid"}, "sendgrid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearEmailEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := EmailProvider(); got != tc.want {
				t.Fatalf("EmailProvider()=%q want %q", got, tc.want)
			}
		})
	}
}

func TestEmailAPIKeyFor(t *testing.T) {
	clearEmailEnv(t)
	t.Setenv(ResendAPIKeyEnv, "re_alias")
	if got := EmailAPIKeyFor(EmailProviderResend); got != "re_alias" {
		t.Fatalf("resend key=%q want alias", got)
	}
	if got := EmailAPIKeyFor(EmailProviderPostmark); got != "" {
		t.Fatalf("postmark key=%q: the Resend key must not be used for postmark", got)
	}
	if got := EmailAPIKeyFor(EmailProviderSMTP); got != "" {
		t.Fatalf("smtp key=%q want empty", got)
	}
	if got := EmailAPIKeyFor(EmailProviderLog); got != "" {
		t.Fatalf("log key=%q want empty", got)
	}

	t.Setenv(EmailAPIKeyEnv, "shared")
	if got := EmailAPIKeyFor(EmailProviderResend); got != "shared" {
		t.Fatalf("EMAIL_API_KEY must win for resend, got %q", got)
	}
	if got := EmailAPIKeyFor(EmailProviderPostmark); got != "shared" {
		t.Fatalf("EMAIL_API_KEY must win for postmark, got %q", got)
	}
	if got := EmailAPIKeyFor(EmailProvider()); got != "shared" {
		t.Fatalf("key for inferred resend=%q", got)
	}
}

func TestEmailProviderMissingConfig(t *testing.T) {
	clearEmailEnv(t)
	for provider, wantMissing := range map[string]bool{
		EmailProviderLog:      false,
		EmailProviderSMTP:     true,
		EmailProviderResend:   true,
		EmailProviderPostmark: true,
		"sendgrid":            true,
	} {
		if got := EmailProviderMissingConfig(provider) != ""; got != wantMissing {
			t.Fatalf("missing(%q)=%v want %v", provider, got, wantMissing)
		}
	}
	t.Setenv(SMTPHostEnv, "mail.example.com")
	if got := EmailProviderMissingConfig(EmailProviderSMTP); got != "" {
		t.Fatalf("smtp with host reported missing %q", got)
	}
	if got := EmailProviderMissingConfig(EmailProviderPostmark); got == "" {
		t.Fatal("postmark without EMAIL_API_KEY must report missing config")
	}
	t.Setenv(EmailAPIKeyEnv, "pm")
	if got := EmailProviderMissingConfig(EmailProviderPostmark); got != "" {
		t.Fatalf("postmark with EMAIL_API_KEY reported missing %q", got)
	}
}

func TestParseEmailVerificationMode(t *testing.T) {
	cases := []struct {
		raw     string
		want    EmailVerificationModeValue
		wantErr bool
	}{
		{"", EmailVerificationModeAuto, false},
		{" auto ", EmailVerificationModeAuto, false},
		{"REQUIRED", EmailVerificationModeRequired, false},
		{"off", EmailVerificationModeOff, false},
		{"false", EmailVerificationModeRequired, true}, // typo fails closed
		{"disabled", EmailVerificationModeRequired, true},
	}
	for _, tc := range cases {
		got, err := ParseEmailVerificationMode(tc.raw)
		if (err != nil) != tc.wantErr {
			t.Fatalf("Parse(%q) err=%v wantErr=%v", tc.raw, err, tc.wantErr)
		}
		if got != tc.want {
			t.Fatalf("Parse(%q)=%q want %q", tc.raw, got, tc.want)
		}
	}
}

func TestEmailVerificationModeResolution(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want EmailVerificationModeValue
	}{
		{"auto + no email config => off (log provider)", nil, EmailVerificationModeOff},
		{"auto + explicit log => off", map[string]string{EmailProviderEnv: "log"}, EmailVerificationModeOff},
		{"auto + smtp => required", map[string]string{EmailProviderEnv: "smtp", SMTPHostEnv: "h"}, EmailVerificationModeRequired},
		{"auto + inferred resend => required", map[string]string{ResendAPIKeyEnv: "re_x"}, EmailVerificationModeRequired},
		// EMAIL-2: a delivery credential without EMAIL_PROVIDER means the box
		// was meant to send mail; the log fallback must not turn verification off.
		{"auto + EMAIL_API_KEY alone => required (stranded credential)", map[string]string{EmailAPIKeyEnv: "re_x"}, EmailVerificationModeRequired},
		{"auto + SMTP_HOST alone => required (stranded credential)", map[string]string{SMTPHostEnv: "smtp.example.test"}, EmailVerificationModeRequired},
		{"auto + SMTP_USERNAME alone => required (stranded credential)", map[string]string{SMTPUsernameEnv: "u"}, EmailVerificationModeRequired},
		{"auto + only SMTP_PORT/SMTP_FROM => off (not credentials)", map[string]string{"SMTP_PORT": "587", "SMTP_FROM": "a@b.test"}, EmailVerificationModeOff},
		{"auto + explicit log + EMAIL_API_KEY => off (deliberate)", map[string]string{EmailProviderEnv: "log", EmailAPIKeyEnv: "re_x"}, EmailVerificationModeOff},
		{"off + stranded EMAIL_API_KEY => off (explicit setting wins)", map[string]string{EmailVerificationEnv: "off", EmailAPIKeyEnv: "re_x"}, EmailVerificationModeOff},
		{"auto + postmark => required", map[string]string{EmailProviderEnv: "postmark", EmailAPIKeyEnv: "pm"}, EmailVerificationModeRequired},
		{"required + log => required", map[string]string{EmailVerificationEnv: "required"}, EmailVerificationModeRequired},
		{"off + resend => off", map[string]string{EmailVerificationEnv: "off", ResendAPIKeyEnv: "re_x"}, EmailVerificationModeOff},
		{"invalid fails closed to required even with log", map[string]string{EmailVerificationEnv: "nope"}, EmailVerificationModeRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearEmailEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := EmailVerificationMode(); got != tc.want {
				t.Fatalf("EmailVerificationMode()=%q want %q", got, tc.want)
			}
			if got, want := EmailVerificationRequired(), tc.want == EmailVerificationModeRequired; got != want {
				t.Fatalf("EmailVerificationRequired()=%v want %v", got, want)
			}
		})
	}
}

func TestValidateEmailEnv(t *testing.T) {
	clearEmailEnv(t)
	if err := ValidateEmailEnv(); err != nil {
		t.Fatalf("empty env must validate (log provider), got %v", err)
	}
	t.Setenv(EmailProviderEnv, "sendgrid")
	if err := ValidateEmailEnv(); err == nil {
		t.Fatal("unknown provider must fail validation")
	}
	t.Setenv(EmailProviderEnv, "smtp")
	t.Setenv(EmailVerificationEnv, "maybe")
	if err := ValidateEmailEnv(); err == nil {
		t.Fatal("unknown EMAIL_VERIFICATION must fail validation")
	}
	t.Setenv(EmailVerificationEnv, "off")
	if err := ValidateEmailEnv(); err != nil {
		t.Fatalf("smtp + off should validate (missing SMTP_HOST is reported separately), got %v", err)
	}
}

func TestStrandedEmailCredentials(t *testing.T) {
	clearEmailEnv(t)
	if got := StrandedEmailCredentials(); len(got) != 0 {
		t.Fatalf("empty env strands nothing, got %v", got)
	}
	t.Setenv(EmailAPIKeyEnv, "re_x")
	t.Setenv(SMTPHostEnv, "smtp.example.test")
	if got := StrandedEmailCredentials(); len(got) != 2 || got[0] != EmailAPIKeyEnv || got[1] != SMTPHostEnv {
		t.Fatalf("got %v, want [EMAIL_API_KEY SMTP_HOST]", got)
	}
	t.Setenv(ResendAPIKeyEnv, "re_y") // provider now inferred: nothing stranded
	if got := StrandedEmailCredentials(); len(got) != 0 {
		t.Fatalf("inferred resend strands nothing, got %v", got)
	}
	t.Setenv(ResendAPIKeyEnv, "")
	t.Setenv(EmailProviderEnv, "log") // explicit choice
	if got := StrandedEmailCredentials(); len(got) != 0 {
		t.Fatalf("explicit EMAIL_PROVIDER=log strands nothing, got %v", got)
	}
}
