package emails

import (
	"strings"
	"testing"
)

// TestAddUnsubscribeURL_PointsAtAccount guards the operator-facing
// "Manage preferences or unsubscribe" link. It must resolve to the real
// operator account page (/account), NOT the never-built /profile route,
// which 404'd for every recipient. See base_*.html footer (operator +
// broadcast variants render this link; the guest footer omits it).
func TestAddUnsubscribeURL_PointsAtAccount(t *testing.T) {
	e := &EmailServer{}
	body := map[string]interface{}{}
	e.addUnsubscribeURL(body, []string{"owner@example.com"})

	got, _ := body["unsubscribe_url"].(string)
	if !strings.Contains(got, "/account") {
		t.Errorf("unsubscribe_url = %q, want it to target /account", got)
	}
	if !strings.Contains(got, "tab=notifications") {
		t.Errorf("unsubscribe_url = %q, want it to deep-link the notifications tab", got)
	}
	if strings.Contains(got, "/profile") {
		t.Errorf("unsubscribe_url = %q still points at the dead /profile route", got)
	}
	// EMAIL-4: unsubscribe_message was a hardcoded English string consumed by NO
	// template (the base_*.html footers render only unsubscribe_url). It is dead
	// and intentionally no longer injected.
	if _, ok := body["unsubscribe_message"]; ok {
		t.Error("unsubscribe_message is dead (no template consumes it) and must not be injected")
	}
}

// TestAddUnsubscribeURL_SkippedWithoutRecipients keeps the existing
// guard: no recipients => no unsubscribe fields injected.
func TestAddUnsubscribeURL_SkippedWithoutRecipients(t *testing.T) {
	e := &EmailServer{}
	body := map[string]interface{}{}
	e.addUnsubscribeURL(body, nil)
	if _, ok := body["unsubscribe_url"]; ok {
		t.Error("unsubscribe_url must not be set when there are no recipients")
	}
}

// isolateAdminInbox clears every env var the operator inbox resolves from and
// restores the package-level AdminsEmails afterwards. The restore is a saved
// slice, not initAdminEmails: cleanups run LIFO, so a re-init registered after
// t.Setenv would re-read this test's env and leak it into later tests.
func isolateAdminInbox(t *testing.T) {
	t.Helper()
	original := AdminsEmails
	t.Cleanup(func() { AdminsEmails = original })
	for _, key := range []string{"ADMIN_EMAILS", "SUPPORT_EMAIL", "ADMIN_EMAIL"} {
		t.Setenv(key, "")
	}
}

func TestAdminEmails_DefaultFallbackChain(t *testing.T) {
	// ADMIN_EMAILS unset: SUPPORT_EMAIL, else the bootstrap ADMIN_EMAIL, else
	// nobody — never an upstream mailbox.
	isolateAdminInbox(t)
	initAdminEmails()
	if len(AdminsEmails) != 0 {
		t.Fatalf("expected no admin inbox with nothing configured, got %v", AdminsEmails)
	}
	if HasAdminRecipients() {
		t.Fatal("HasAdminRecipients must be false with nothing configured")
	}
	if !WarnIfAdminEmailsUnset() {
		t.Fatal("WarnIfAdminEmailsUnset must report the missing inbox")
	}

	t.Setenv("ADMIN_EMAIL", " owner@resto.example.org ")
	initAdminEmails()
	if len(AdminsEmails) != 1 || AdminsEmails[0] != "owner@resto.example.org" {
		t.Fatalf("ADMIN_EMAIL fallback = %v", AdminsEmails)
	}

	t.Setenv("SUPPORT_EMAIL", "help@resto.example.org")
	initAdminEmails()
	if len(AdminsEmails) != 1 || AdminsEmails[0] != "help@resto.example.org" {
		t.Fatalf("SUPPORT_EMAIL must win over ADMIN_EMAIL, got %v", AdminsEmails)
	}

	t.Setenv("ADMIN_EMAILS", "ops@resto.example.org")
	initAdminEmails()
	if len(AdminsEmails) != 1 || AdminsEmails[0] != "ops@resto.example.org" {
		t.Fatalf("ADMIN_EMAILS must win over SUPPORT_EMAIL, got %v", AdminsEmails)
	}
	if !HasAdminRecipients() || WarnIfAdminEmailsUnset() {
		t.Fatal("a resolved inbox must count as configured")
	}
}

// Only a single bare address counts as a fallback inbox: a display-name form,
// a list or free text in SUPPORT_EMAIL/ADMIN_EMAIL is ignored, not guessed at.
func TestAdminEmails_FallbackIgnoresMalformedAddresses(t *testing.T) {
	isolateAdminInbox(t)
	for _, bad := range []string{"not an email", "Help <help@resto.example.org>", "a@resto.example.org,b@resto.example.org"} {
		t.Setenv("SUPPORT_EMAIL", bad)
		t.Setenv("ADMIN_EMAIL", bad)
		initAdminEmails()
		if len(AdminsEmails) != 0 {
			t.Fatalf("malformed fallback %q must be ignored, got %v", bad, AdminsEmails)
		}
	}

	t.Setenv("ADMIN_EMAIL", "owner@resto.example.org")
	initAdminEmails()
	if len(AdminsEmails) != 1 || AdminsEmails[0] != "owner@resto.example.org" {
		t.Fatalf("a malformed SUPPORT_EMAIL must fall through to ADMIN_EMAIL, got %v", AdminsEmails)
	}
}

func TestAdminEmails_BlankEntriesOnlyIsEmpty(t *testing.T) {
	isolateAdminInbox(t)
	t.Setenv("ADMIN_EMAILS", " , ,")
	initAdminEmails()
	if len(AdminsEmails) != 0 {
		t.Fatalf("expected blank-only ADMIN_EMAILS to yield no inbox, got %v", AdminsEmails)
	}
}

func TestAdminEmails_FromEnv(t *testing.T) {
	isolateAdminInbox(t)
	t.Setenv("ADMIN_EMAILS", "a@test.com,b@test.com")
	initAdminEmails()
	if !HasAdminRecipients() || WarnIfAdminEmailsUnset() {
		t.Fatal("a configured ADMIN_EMAILS must count as an admin inbox")
	}
	expected := []string{"a@test.com", "b@test.com"}
	if len(AdminsEmails) != len(expected) {
		t.Fatalf("expected %d admin emails, got %d", len(expected), len(AdminsEmails))
	}
	for i, email := range expected {
		if AdminsEmails[i] != email {
			t.Errorf("expected AdminsEmails[%d] = %q, got %q", i, email, AdminsEmails[i])
		}
	}
}

func TestAdminEmails_FromEnv_Trims(t *testing.T) {
	isolateAdminInbox(t)
	t.Setenv("ADMIN_EMAILS", " a@test.com , b@test.com ")
	initAdminEmails()
	expected := []string{"a@test.com", "b@test.com"}
	if len(AdminsEmails) != len(expected) {
		t.Fatalf("expected %d admin emails, got %d", len(expected), len(AdminsEmails))
	}
	for i, email := range expected {
		if AdminsEmails[i] != email {
			t.Errorf("expected AdminsEmails[%d] = %q, got %q", i, email, AdminsEmails[i])
		}
	}
}
