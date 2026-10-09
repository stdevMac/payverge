package emails

import (
	"errors"
	"testing"
)

func newCaptureServer(t *testing.T) (*EmailServer, *captureProvider) {
	t.Helper()
	provider := &captureProvider{}
	server, err := NewEmailServer(provider, "notifications@example.com", "updates@example.com", resolveTemplatesRoot(t))
	if err != nil {
		t.Fatalf("failed to build email server: %v", err)
	}
	return server, provider
}

func withAdminEmails(t *testing.T, inbox []string) {
	t.Helper()
	original := AdminsEmails
	AdminsEmails = inbox
	t.Cleanup(func() { AdminsEmails = original })
}

// With ADMIN_EMAILS unset every internal admin alert is skipped (nil error, no
// provider call) so the signup flow that triggered it never fails and
// nothing is mailed to a hard-coded upstream inbox.
func TestAdminAlertsSkipWhenAdminEmailsUnset(t *testing.T) {
	server, provider := newCaptureServer(t)
	withAdminEmails(t, nil)

	if err := server.SendAdminNewSignupEmail(AdminNewSignupInfo{BusinessName: "Test Taqueria"}); err != nil {
		t.Fatalf("SendAdminNewSignupEmail: %v", err)
	}
	if len(provider.sent) != 0 {
		t.Fatalf("expected no admin alert to be sent without ADMIN_EMAILS, got %d: %+v", len(provider.sent), provider.sent)
	}
}

func TestAdminAlertsGoOnlyToConfiguredInbox(t *testing.T) {
	server, provider := newCaptureServer(t)
	withAdminEmails(t, []string{"ops@example.org"})

	if err := server.SendAdminNewSignupEmail(AdminNewSignupInfo{BusinessName: "Test Taqueria"}); err != nil {
		t.Fatalf("SendAdminNewSignupEmail: %v", err)
	}
	if len(provider.sent) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(provider.sent))
	}
	if got := provider.sent[0].To; len(got) != 1 || got[0] != "ops@example.org" {
		t.Fatalf("alert recipients = %v, want [ops@example.org]", got)
	}
}

// Ad-hoc callers (escalation, concierge, ops tools) pass AdminsEmails straight
// to SendCustomEmail; an empty inbox must surface ErrNoRecipients rather than
// a provider call or a dead outbox row.
func TestSendCustomEmailRejectsEmptyRecipients(t *testing.T) {
	server, provider := newCaptureServer(t)

	for _, to := range [][]string{nil, {}, {"", "  "}} {
		err := server.SendCustomEmail(to, "subject", "<p>body</p>", "body")
		if !errors.Is(err, ErrNoRecipients) {
			t.Fatalf("SendCustomEmail(%q) error = %v, want ErrNoRecipients", to, err)
		}
	}
	if len(provider.sent) != 0 {
		t.Fatalf("expected no provider call for empty recipients, got %d", len(provider.sent))
	}
}

func TestParseAdminEmails(t *testing.T) {
	cases := map[string][]string{
		"":                       nil,
		"   ":                    nil,
		",,":                     nil,
		"a@example.com":          {"a@example.com"},
		" a@example.com ,b@x.io": {"a@example.com", "b@x.io"},
	}
	for raw, want := range cases {
		got := parseAdminEmails(raw)
		if len(got) != len(want) {
			t.Fatalf("parseAdminEmails(%q) = %v, want %v", raw, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("parseAdminEmails(%q) = %v, want %v", raw, got, want)
			}
		}
	}
}

func TestReplyToAddressHasNoUpstreamDefault(t *testing.T) {
	t.Setenv("EMAIL_REPLY_TO", "")
	t.Setenv("SUPPORT_EMAIL", "")
	if got := replyToAddress(); got != "" {
		t.Fatalf("replyToAddress() = %q with nothing configured, want empty (no Reply-To header)", got)
	}

	t.Setenv("SUPPORT_EMAIL", " help@resto.example.org ")
	if got := replyToAddress(); got != "help@resto.example.org" {
		t.Fatalf("replyToAddress() = %q, want SUPPORT_EMAIL", got)
	}

	t.Setenv("EMAIL_REPLY_TO", "replies@resto.example.org")
	if got := replyToAddress(); got != "replies@resto.example.org" {
		t.Fatalf("replyToAddress() = %q, want EMAIL_REPLY_TO to win", got)
	}
}
