package emails

import (
	"strings"
	"testing"
)

func clearEmailInstanceEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"PUBLIC_URL", "FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL",
		"PRODUCT_NAME", "COMPANY_NAME", "COMPANY_ADDRESS", "LOGO_URL",
		"SUPPORT_EMAIL", "EMAIL_REPLY_TO",
	} {
		t.Setenv(k, "")
	}
}

func TestEmailLogoURL(t *testing.T) {
	clearEmailInstanceEnv(t)
	t.Setenv("PUBLIC_URL", "https://eat.example.com")
	if got := emailLogoURL(); got != "https://eat.example.com/images/PayvergeLogo.png" {
		t.Fatalf("default logo = %q", got)
	}
	t.Setenv("LOGO_URL", "/media/brand/logo.png")
	if got := emailLogoURL(); got != "https://eat.example.com/media/brand/logo.png" {
		t.Fatalf("root-relative logo = %q", got)
	}
	t.Setenv("LOGO_URL", "https://cdn.example.net/logo.png")
	if got := emailLogoURL(); got != "https://cdn.example.net/logo.png" {
		t.Fatalf("absolute logo = %q", got)
	}
}

func TestReplyToFollowsSupportEmail(t *testing.T) {
	clearEmailInstanceEnv(t)
	if got := replyToAddress(); got != "" {
		t.Fatalf("reply-to without config = %q, want empty", got)
	}
	t.Setenv("SUPPORT_EMAIL", "help@eat.example.com")
	if got := replyToAddress(); got != "help@eat.example.com" {
		t.Fatalf("reply-to = %q", got)
	}
	t.Setenv("EMAIL_REPLY_TO", "replies@eat.example.com")
	if got := replyToAddress(); got != "replies@eat.example.com" {
		t.Fatalf("EMAIL_REPLY_TO must win, got %q", got)
	}
}

func TestEmailBaseURLsFollowInstance(t *testing.T) {
	clearEmailInstanceEnv(t)
	t.Setenv("PUBLIC_URL", "https://eat.example.com")
	if got := appBaseURL(); got != "https://eat.example.com" {
		t.Fatalf("appBaseURL = %q", got)
	}
	if got := apiPublicBaseURL(); got != "https://eat.example.com" {
		t.Fatalf("same-origin apiPublicBaseURL = %q", got)
	}
	t.Setenv("APP_BASE_URL", "https://api.eat.example.com")
	if got := apiPublicBaseURL(); got != "https://api.eat.example.com" {
		t.Fatalf("APP_BASE_URL apiPublicBaseURL = %q", got)
	}
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("BASE_URL", "https://retired-api.example.com/")
	if got := apiPublicBaseURL(); got != "https://eat.example.com" {
		t.Fatalf("BASE_URL is not an alias; apiPublicBaseURL = %q", got)
	}
}

func renderWithInstance(t *testing.T, lang, name, variant string, extra map[string]interface{}) string {
	t.Helper()
	tm, err := NewTemplateManager(resolveTemplatesRoot(t))
	if err != nil {
		t.Fatalf("new template manager: %v", err)
	}
	body := map[string]interface{}{
		"footer_variant":  variant,
		"owner_name":      "Sam",
		"business_name":   "Cafe Test",
		"unsubscribe_url": "https://eat.example.com/account",
	}
	for k, v := range extra {
		body[k] = v
	}
	addInstanceTemplateFields(body)
	out, err := tm.Render(lang, name, body)
	if err != nil {
		t.Fatalf("render %s/%s: %v", lang, name, err)
	}
	return out
}

func TestRenderedEmailsCarryInstanceIdentity(t *testing.T) {
	clearEmailInstanceEnv(t)
	t.Setenv("PUBLIC_URL", "https://eat.example.com")
	t.Setenv("PRODUCT_NAME", "Tavola")
	t.Setenv("COMPANY_NAME", "Tavola Co")

	for _, lang := range []string{"en", "es", "es-AR"} {
		out := renderWithInstance(t, lang, "getting_started", "operator", nil)
		for _, want := range []string{
			"Tavola",
			`src="https://eat.example.com/images/PayvergeLogo.png"`,
			`href="https://eat.example.com"`,
			">eat.example.com</a>",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s getting_started missing %q", lang, want)
			}
		}
		for _, banned := range []string{"payverge.io", "Marcos", "Payverge ·", "Tavola Co ·"} {
			if strings.Contains(out, banned) {
				t.Errorf("%s getting_started contains %q", lang, banned)
			}
		}
	}

	t.Setenv("COMPANY_ADDRESS", "1 Main St, Springfield")
	out := renderWithInstance(t, "en", "getting_started", "operator", nil)
	if !strings.Contains(out, "Tavola Co · 1 Main St, Springfield") {
		t.Error("operator footer should show company name and postal address when set")
	}
}
