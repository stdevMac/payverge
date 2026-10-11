package emails

import (
	"context"
	"strings"
	"testing"
)

// captureProvider is a minimal EmailProvider that records every message it
// is asked to send, so tests can assert on subject/body/recipients without
// hitting a real email API.
type captureProvider struct {
	sent []EmailMessage
}

func (p *captureProvider) Send(ctx context.Context, msg EmailMessage) error {
	p.sent = append(p.sent, msg)
	return nil
}

func TestSendAdminNewSignupEmail(t *testing.T) {
	templatesRoot := resolveTemplatesRoot(t)
	provider := &captureProvider{}

	server, err := NewEmailServer(provider, "notifications@payverge.io", "updates@payverge.io", templatesRoot)
	if err != nil {
		t.Fatalf("failed to build email server: %v", err)
	}

	originalAdmins := AdminsEmails
	AdminsEmails = []string{"admin1@payverge.io", "admin2@payverge.io"}
	defer func() { AdminsEmails = originalAdmins }()

	err = server.SendAdminNewSignupEmail(AdminNewSignupInfo{
		BusinessName: "Test Taqueria",
		OwnerName:    "Jane Owner",
		OwnerEmail:   "jane@example.com",
		Country:      "US",
		BusinessType: "restaurant",
		DashboardURL: "https://payverge.io/business/1/dashboard",
	})
	if err != nil {
		t.Fatalf("SendAdminNewSignupEmail returned error: %v", err)
	}

	if len(provider.sent) != 1 {
		t.Fatalf("expected exactly 1 email sent, got %d", len(provider.sent))
	}

	msg := provider.sent[0]
	if len(msg.To) != 2 || msg.To[0] != "admin1@payverge.io" || msg.To[1] != "admin2@payverge.io" {
		t.Errorf("expected admin recipients, got %v", msg.To)
	}
	if !strings.Contains(msg.Subject, "Test Taqueria") {
		t.Errorf("expected subject to contain business name, got %q", msg.Subject)
	}
	for _, want := range []string{"Test Taqueria", "Jane Owner", "jane@example.com", "US", "restaurant", "https://payverge.io/business/1/dashboard"} {
		if !strings.Contains(msg.HTMLBody, want) {
			t.Errorf("expected email body to contain %q, got:\n%s", want, msg.HTMLBody)
		}
	}
}

// Nobody buys a plan, so the admin alert must not claim one.
func TestSendAdminNewSignupEmailOmitsPlan(t *testing.T) {
	templatesRoot := resolveTemplatesRoot(t)
	provider := &captureProvider{}
	server, err := NewEmailServer(provider, "notifications@example.com", "updates@example.com", templatesRoot)
	if err != nil {
		t.Fatalf("failed to build email server: %v", err)
	}
	originalAdmins := AdminsEmails
	AdminsEmails = []string{"admin@example.com"}
	defer func() { AdminsEmails = originalAdmins }()

	if err := server.SendAdminNewSignupEmail(AdminNewSignupInfo{
		BusinessName: "Test Taqueria",
		OwnerEmail:   "jane@example.com",
	}); err != nil {
		t.Fatalf("SendAdminNewSignupEmail returned error: %v", err)
	}
	if len(provider.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(provider.sent))
	}
	body := provider.sent[0].HTMLBody
	for _, unwanted := range []string{"ai_pro", "monthly", "July 19, 2026", ">Plan<", "Billing cycle", "Trial ends"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("body must not contain %q:\n%s", unwanted, body)
		}
	}
	if !strings.Contains(body, "Test Taqueria") {
		t.Errorf("body lost the business name:\n%s", body)
	}
}
