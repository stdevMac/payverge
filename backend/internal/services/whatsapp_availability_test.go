package services

import (
	"strings"
	"testing"
)

func TestWhatsAppEnabledValue(t *testing.T) {
	cases := []struct {
		env  string
		want bool
	}{
		{"", false},
		{"false", false},
		{"0", false},
		{"1", false},
		{"yes", false},
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"  TRUE ", true},
		{"  true  ", true},
	}
	for _, tc := range cases {
		if got := whatsAppEnabledValue(tc.env); got != tc.want {
			t.Errorf("whatsAppEnabledValue(%q) = %v; want %v", tc.env, got, tc.want)
		}
	}
}

func TestWhatsAppAvailableRequiresTagAndEnv(t *testing.T) {
	t.Setenv(WhatsAppEnabledEnv, "")
	if WhatsAppRequested() || WhatsAppAvailable() {
		t.Fatal("WHATSAPP_ENABLED unset must keep WhatsApp off")
	}

	t.Setenv(WhatsAppEnabledEnv, "true")
	if !WhatsAppRequested() {
		t.Fatal("WHATSAPP_ENABLED=true must register as requested")
	}
	// Default builds compile whatsmeow out, so the env var alone never turns
	// the channel on; tagged builds honour it.
	if got := WhatsAppAvailable(); got != WhatsAppBuilt {
		t.Fatalf("WhatsAppAvailable() = %v with WHATSAPP_ENABLED=true; want WhatsAppBuilt (%v)", got, WhatsAppBuilt)
	}
}

func TestWhatsAppStartupMessage(t *testing.T) {
	cases := []struct {
		name      string
		requested bool
		built     bool
		wantWarn  bool
		wantParts []string
	}{
		{"requested but compiled out warns with the fix", true, false, true,
			[]string{"WHATSAPP_ENABLED=true is ignored", "GO_TAGS=whatsapp", "GPL-3.0", "docs/self-hosting/whatsapp.md"}},
		{"tagged build, requested", true, true, false, []string{"enabled"}},
		{"default build, not requested", false, false, false, []string{"not compiled in"}},
		{"tagged build, not requested", false, true, false, []string{"WHATSAPP_ENABLED != true"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, warn := whatsAppStartupMessage(tc.requested, tc.built)
			if warn != tc.wantWarn {
				t.Fatalf("warn = %v; want %v (msg=%q)", warn, tc.wantWarn, msg)
			}
			for _, part := range tc.wantParts {
				if !strings.Contains(msg, part) {
					t.Errorf("message %q missing %q", msg, part)
				}
			}
		})
	}

	t.Setenv(WhatsAppEnabledEnv, "TRUE")
	_, warn := WhatsAppStartupMessage()
	if warn != !WhatsAppBuilt {
		t.Fatalf("WhatsAppStartupMessage() warn = %v with WHATSAPP_ENABLED=TRUE; want %v in this build", warn, !WhatsAppBuilt)
	}
}
