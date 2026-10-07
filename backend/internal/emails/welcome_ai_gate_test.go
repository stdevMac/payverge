package emails

import (
	"strings"
	"testing"
)

func TestWelcomeTemplate_NoAIWithoutProvider(t *testing.T) {
	prev := aiProviderConfiguredForEmail
	t.Cleanup(func() { aiProviderConfiguredForEmail = prev })

	aiProviderConfiguredForEmail = func() bool { return false }
	if got := welcomeTemplate(); got != "welcome" {
		t.Fatalf("no LLM provider: got %q, want welcome", got)
	}
	aiProviderConfiguredForEmail = func() bool { return true }
	if got := welcomeTemplate(); got != "welcome_ai" {
		t.Fatalf("provider configured: got %q, want welcome_ai", got)
	}
}

// The welcome email must never name a plan: there are no plans.
func TestBusinessOnboardingEmailNamesNoPlan(t *testing.T) {
	prev := aiProviderConfiguredForEmail
	t.Cleanup(func() { aiProviderConfiguredForEmail = prev })

	for _, ai := range []bool{false, true} {
		for _, lang := range []string{"eng", "es", "es_ar"} {
			aiProviderConfiguredForEmail = func() bool { return ai }
			provider := &captureProvider{}
			server, err := NewEmailServer(provider, "notifications@example.com", "updates@example.com", resolveTemplatesRoot(t))
			if err != nil {
				t.Fatalf("email server: %v", err)
			}
			if err := server.SendBusinessOnboardingEmail([]string{"owner@example.com"}, "Ana", "https://pos.example.com/dash", lang); err != nil {
				t.Fatalf("ai=%v %s: send: %v", ai, lang, err)
			}
			msg := provider.sent[0]
			if lang == "eng" && msg.Subject != "Welcome to Payverge" {
				t.Errorf("ai=%v: subject %q, want %q", ai, msg.Subject, "Welcome to Payverge")
			}
			for _, bad := range []string{"Core", "AI Pro"} {
				if strings.Contains(msg.Subject, bad) || strings.Contains(msg.HTMLBody, bad) {
					t.Errorf("ai=%v %s: email names plan %q:\n%s\n%s", ai, lang, bad, msg.Subject, msg.HTMLBody)
				}
			}
		}
	}
}
