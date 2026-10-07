package config

import (
	"strings"
	"testing"
)

func clearInstanceEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"PUBLIC_URL", "FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL",
		"PRODUCT_NAME", "COMPANY_NAME", "LEGAL_ENTITY", "SUPPORT_EMAIL", "SECURITY_EMAIL",
		"LOGO_URL", "BRAND_COLOR", "ENV", "APP_ENV",
		"ALLOWED_REDIRECT_DOMAINS", "APP_DOMAIN",
	} {
		t.Setenv(k, "")
	}
}

func TestInstanceDefaults(t *testing.T) {
	clearInstanceEnv(t)
	if got := PublicURL(); got != DefaultDevPublicURL {
		t.Fatalf("PublicURL = %q", got)
	}
	if PublicURLConfigured() {
		t.Fatal("PublicURLConfigured must be false with no env")
	}
	if got := PublicHost(); got != "localhost:3000" {
		t.Fatalf("PublicHost = %q", got)
	}
	if ProductName() != "Payverge" || CompanyName() != "Payverge" {
		t.Fatalf("names = %q %q", ProductName(), CompanyName())
	}
	if LegalEntity() != "" || SupportEmail() != "" || SecurityEmail() != "" || LogoURL() != "" {
		t.Fatal("contact/legal/logo must default to empty, never an upstream value")
	}
	if BrandColor() != "#1a6b6a" {
		t.Fatalf("BrandColor = %q", BrandColor())
	}
	if got := APIBaseURL(); got != DefaultDevAPIBaseURL {
		t.Fatalf("APIBaseURL = %q", got)
	}
}

func TestInstanceConfigured(t *testing.T) {
	clearInstanceEnv(t)
	t.Setenv("PUBLIC_URL", " HTTPS://POS.Example.com/ ")
	t.Setenv("PRODUCT_NAME", "Bistro OS")
	t.Setenv("COMPANY_NAME", "Bistro Labs")
	t.Setenv("LEGAL_ENTITY", "Bistro Labs S.R.L.")
	t.Setenv("SUPPORT_EMAIL", "help@example.com")
	t.Setenv("SECURITY_EMAIL", "security@example.com")
	t.Setenv("LOGO_URL", "/brand/logo.svg")
	t.Setenv("BRAND_COLOR", "AA3355")

	if got := PublicURL(); got != "https://pos.example.com" {
		t.Fatalf("PublicURL = %q", got)
	}
	if got := PublicHost(); got != "pos.example.com" {
		t.Fatalf("PublicHost = %q", got)
	}
	if got := FrontendBaseURL(); got != "https://pos.example.com" {
		t.Fatalf("FrontendBaseURL = %q", got)
	}
	if got := APIBaseURL(); got != "https://pos.example.com" {
		t.Fatalf("APIBaseURL must default to the same origin, got %q", got)
	}
	t.Setenv("APP_BASE_URL", "https://api.example.com/")
	if got := APIBaseURL(); got != "https://api.example.com" {
		t.Fatalf("APP_BASE_URL must win for split deploys, got %q", got)
	}
	if ProductName() != "Bistro OS" || CompanyName() != "Bistro Labs" || LegalEntity() != "Bistro Labs S.R.L." {
		t.Fatal("names not applied")
	}
	if SupportEmail() != "help@example.com" || SecurityEmail() != "security@example.com" {
		t.Fatal("emails not applied")
	}
	if LogoURL() != "/brand/logo.svg" || BrandColor() != "#aa3355" {
		t.Fatalf("logo/color = %q %q", LogoURL(), BrandColor())
	}
}

func TestInstanceFallbacksChain(t *testing.T) {
	clearInstanceEnv(t)
	// PUBLIC_URL is the only name: the retired aliases are ignored.
	for _, k := range []string{"FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL"} {
		t.Setenv(k, "https://alias.example.com")
		if got := PublicURL(); got != DefaultDevPublicURL {
			t.Fatalf("%s must not set the public URL, got %q", k, got)
		}
		t.Setenv(k, "")
	}
	t.Setenv("PUBLIC_URL", "https://public.example.com")
	if got := PublicURL(); got != "https://public.example.com" {
		t.Fatalf("PUBLIC_URL = %q", got)
	}
	t.Setenv("SECURITY_EMAIL", "")
	t.Setenv("SUPPORT_EMAIL", "help@example.com")
	if SecurityEmail() != "help@example.com" {
		t.Fatal("SECURITY_EMAIL must default to SUPPORT_EMAIL")
	}
	t.Setenv("COMPANY_NAME", "Acme")
	if LegalEntity() != "Acme" {
		t.Fatal("LEGAL_ENTITY must default to an explicit COMPANY_NAME")
	}
}

func TestInstanceRejectsUnsafeValues(t *testing.T) {
	clearInstanceEnv(t)
	for _, bad := range []string{"javascript:alert(1)", "//evil.example/x.png", "data:image/png;base64,AA", `/\evil`, "https://user:pw@cdn.example/x.png"} {
		t.Setenv("LOGO_URL", bad)
		if got := LogoURL(); got != "" {
			t.Errorf("LogoURL(%q) = %q, want empty", bad, got)
		}
	}
	for _, bad := range []string{"red", "#12345", "#gggggg", "rgb(0,0,0)"} {
		t.Setenv("BRAND_COLOR", bad)
		if got := BrandColor(); got != DefaultBrandColor {
			t.Errorf("BrandColor(%q) = %q", bad, got)
		}
	}
	for _, bad := range []string{"not-an-email", "Help <help@example.com>", "a@b.com\r\nBcc: x@y.z"} {
		t.Setenv("SUPPORT_EMAIL", bad)
		if got := SupportEmail(); got != "" {
			t.Errorf("SupportEmail(%q) = %q", bad, got)
		}
	}
	t.Setenv("PRODUCT_NAME", "Evil\r\nBcc: x@y.z")
	if got := ProductName(); strings.ContainsAny(got, "\r\n") {
		t.Fatalf("ProductName kept control chars: %q", got)
	}
	t.Setenv("PRODUCT_NAME", strings.Repeat("x", 500))
	if got := ProductName(); len([]rune(got)) > 80 {
		t.Fatalf("ProductName not capped: %d", len(got))
	}
}

func TestNormalizePublicURL(t *testing.T) {
	ok := map[string]string{
		"https://pos.example.com":       "https://pos.example.com",
		"https://pos.example.com/":      "https://pos.example.com",
		"pos.example.com":               "https://pos.example.com",
		"HTTP://LOCALHOST:3000":         "http://localhost:3000",
		"https://pos.example.com:8443/": "https://pos.example.com:8443",
	}
	for in, want := range ok {
		got, err := NormalizePublicURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizePublicURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x.example", "https://", "https://u:p@x.example", "https://x.example/app", "https://x.example/?a=1", "https://x.example/#f"} {
		if _, err := NormalizePublicURL(bad); err == nil {
			t.Errorf("NormalizePublicURL(%q) accepted", bad)
		}
	}
}

func TestValidateInstance(t *testing.T) {
	clearInstanceEnv(t)
	errs, _ := ValidateInstance(false)
	if len(errs) != 0 {
		t.Fatalf("dev without PUBLIC_URL must be fine: %v", errs)
	}
	errs, _ = ValidateInstance(true)
	if len(errs) != 1 || errs[0].Field != "PUBLIC_URL" {
		t.Fatalf("production without PUBLIC_URL: %v", errs)
	}

	t.Setenv("PUBLIC_URL", "http://pos.example.com")
	if errs, _ := ValidateInstance(false); len(errs) != 0 {
		t.Fatalf("dev http must be allowed: %v", errs)
	}
	if errs, _ := ValidateInstance(true); len(errs) != 1 || !strings.Contains(errs[0].Message, "https") {
		t.Fatalf("production http must fail: %v", errs)
	}

	t.Setenv("PUBLIC_URL", "https://pos.example.com/subpath")
	if errs, _ := ValidateInstance(false); len(errs) != 1 {
		t.Fatalf("path must be rejected: %v", errs)
	}

	t.Setenv("PUBLIC_URL", "")
	t.Setenv("FRONTEND_URL", "https://retired.example.com")
	// A retired alias never satisfies production.
	errs, _ = ValidateInstance(true)
	if !containsField(errs, "PUBLIC_URL") {
		t.Fatalf("FRONTEND_URL must not satisfy production: %v", errs)
	}
	t.Setenv("FRONTEND_URL", "")

	t.Setenv("BRAND_COLOR", "teal")
	t.Setenv("LOGO_URL", "javascript:x")
	t.Setenv("SUPPORT_EMAIL", "nope")
	_, warns := ValidateInstance(true)
	for _, f := range []string{"BRAND_COLOR", "LOGO_URL", "SUPPORT_EMAIL"} {
		if !containsField(warns, f) {
			t.Errorf("missing warning for %s: %v", f, warns)
		}
	}
}

func containsField(list []ValidationError, field string) bool {
	for _, e := range list {
		if e.Field == field {
			return true
		}
	}
	return false
}

func TestRedirectDefaultsFollowPublicURL(t *testing.T) {
	clearInstanceEnv(t)
	t.Setenv("PUBLIC_URL", "https://pos.example.com")
	if _, err := ValidateRedirectURL("https://pos.example.com/payment/success", true); err != nil {
		t.Errorf("PUBLIC_URL host must be a redirect target: %v", err)
	}
	// Exactly the declared host: no implicit www./apex twin.
	if _, err := ValidateRedirectURL("https://www.pos.example.com/x", true); err == nil {
		t.Error("an undeclared www twin must not be an implicit redirect target")
	}
	// API-origin keys never feed the default allow-list.
	t.Setenv("PUBLIC_URL", "")
	t.Setenv("BASE_URL", "https://api.pos.example.com")
	t.Setenv("APP_BASE_URL", "https://api.pos.example.com")
	t.Setenv("NEXT_PUBLIC_BASE_URL", "https://api.pos.example.com")
	if _, err := ValidateRedirectURL("https://api.pos.example.com/x", true); err == nil {
		t.Error("BASE_URL/APP_BASE_URL/NEXT_PUBLIC_BASE_URL must not seed redirect defaults")
	}
	t.Setenv("BASE_URL", "")
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("NEXT_PUBLIC_BASE_URL", "")
	// An explicit list (or APP_DOMAIN) opts the twin back in.
	t.Setenv("PUBLIC_URL", "https://pos.example.com")
	t.Setenv("APP_DOMAIN", "www.pos.example.com")
	if _, err := ValidateRedirectURL("https://www.pos.example.com/x", true); err != nil {
		t.Errorf("APP_DOMAIN must extend the default: %v", err)
	}
	t.Setenv("APP_DOMAIN", "")
	upstream := "https://" + UpstreamDomain + "/payment/success"
	if _, err := ValidateRedirectURL(upstream, true); err == nil {
		t.Fatal("the upstream domain must not be an implicit redirect target")
	}
	t.Setenv("PUBLIC_URL", "")
	if _, err := ValidateRedirectURL(upstream, true); err == nil {
		t.Fatal("no default redirect domains without a configured public URL")
	}
}

func TestPublicOrigins(t *testing.T) {
	clearInstanceEnv(t)
	if got := PublicOrigins(); len(got) != 0 {
		t.Fatalf("no configured URL → no origins, got %v", got)
	}
	// Exactly the declared origin: no www./apex twin is ever added.
	cases := map[string][]string{
		"https://Pos.Example.com/":    {"https://pos.example.com"},
		"https://www.pos.example.com": {"https://www.pos.example.com"},
		"http://pos.example.com:8443": {"http://pos.example.com:8443"},
		"http://localhost:3000":       {"http://localhost:3000"},
		"http://192.168.1.10:3000":    {"http://192.168.1.10:3000"},
		"pos.example.com":             {"https://pos.example.com"},
		"ftp://pos.example.com":       nil,
		"https://pos.example.com/app": nil,
	}
	for raw, want := range cases {
		t.Setenv("PUBLIC_URL", raw)
		got := PublicOrigins()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("PublicOrigins(%q) = %v, want %v", raw, got, want)
		}
	}
	// A malformed PUBLIC_URL fails closed.
	t.Setenv("PUBLIC_URL", "ftp://pos.example.com")
	if got := PublicOrigins(); len(got) != 0 {
		t.Fatalf("malformed PUBLIC_URL must not fall through, got %v", got)
	}
	// No other key (the API origin APP_BASE_URL, or a retired alias) ever
	// becomes a trusted browser origin.
	t.Setenv("PUBLIC_URL", "")
	for _, k := range []string{"FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL"} {
		t.Setenv(k, "https://api.pos.example.com")
		if got := PublicOrigins(); len(got) != 0 {
			t.Fatalf("%s must not seed trusted origins, got %v", k, got)
		}
		t.Setenv(k, "")
	}
}
