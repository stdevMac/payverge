//go:build whatsapp

package services

import "testing"

func TestWhatsAppBuilt_TaggedBuild(t *testing.T) {
	if !WhatsAppBuilt {
		t.Fatal("-tags whatsapp build must report WhatsAppBuilt=true")
	}
	t.Setenv(WhatsAppEnabledEnv, "true")
	if !WhatsAppAvailable() {
		t.Fatal("tagged build with WHATSAPP_ENABLED=true must make WhatsApp available")
	}
	t.Setenv(WhatsAppEnabledEnv, "false")
	if WhatsAppAvailable() {
		t.Fatal("tagged build must stay dark unless WHATSAPP_ENABLED=true")
	}
}
