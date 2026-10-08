package main

import (
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"

	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestLogWhatsAppDisabled(t *testing.T) {
	hook := logtest.NewLocal(logger.Logger)
	defer hook.Reset()

	t.Setenv(services.WhatsAppEnabledEnv, "true")
	logWhatsAppDisabled()
	entry := hook.LastEntry()
	if entry == nil {
		t.Fatal("logWhatsAppDisabled logged nothing")
	}
	if services.WhatsAppBuilt {
		// Tagged build: main.go only calls this when WhatsApp is not available,
		// but the message must still be informational, never a false warning.
		if entry.Level != logrus.InfoLevel {
			t.Fatalf("tagged build logged %s: %q", entry.Level, entry.Message)
		}
		return
	}
	if entry.Level != logrus.WarnLevel {
		t.Fatalf("WHATSAPP_ENABLED=true without -tags whatsapp must warn; got %s: %q", entry.Level, entry.Message)
	}
	for _, want := range []string{"WHATSAPP_ENABLED=true is ignored", "GO_TAGS=whatsapp", "docs/self-hosting/whatsapp.md"} {
		if !strings.Contains(entry.Message, want) {
			t.Errorf("warning %q missing %q", entry.Message, want)
		}
	}

	hook.Reset()
	t.Setenv(services.WhatsAppEnabledEnv, "")
	logWhatsAppDisabled()
	if entry := hook.LastEntry(); entry == nil || entry.Level != logrus.InfoLevel {
		t.Fatalf("WHATSAPP_ENABLED unset must log at info, got %+v", entry)
	}
}
