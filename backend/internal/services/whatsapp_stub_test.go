//go:build !whatsapp

package services

import (
	"errors"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// The default-build stub must keep every caller compiling while never
// pretending the channel works.
func TestWhatsAppStub_DefaultBuildIsInert(t *testing.T) {
	if WhatsAppBuilt {
		t.Fatal("default build must report WhatsAppBuilt=false")
	}
	t.Setenv(WhatsAppEnabledEnv, "true")
	if WhatsAppAvailable() {
		t.Fatal("WHATSAPP_ENABLED=true must not make WhatsApp available without -tags whatsapp")
	}

	wm, err := NewWhatsAppManager(nil, nil)
	if wm != nil || !errors.Is(err, ErrWhatsAppNotBuilt) {
		t.Fatalf("NewWhatsAppManager() = (%v, %v); want (nil, ErrWhatsAppNotBuilt)", wm, err)
	}

	stub := &WhatsAppManager{}
	if got := stub.WithClassifier(nil).WithCostGate(nil); got != stub {
		t.Fatal("stub builder methods must return the receiver")
	}
	if err := stub.RestoreAllSessions(); !errors.Is(err, ErrWhatsAppNotBuilt) {
		t.Fatalf("RestoreAllSessions() = %v", err)
	}
	if ch, err := stub.ConnectBusiness(1); ch != nil || !errors.Is(err, ErrWhatsAppNotBuilt) {
		t.Fatalf("ConnectBusiness() = (%v, %v)", ch, err)
	}
	if err := stub.DisconnectBusiness(1); !errors.Is(err, ErrWhatsAppNotBuilt) {
		t.Fatalf("DisconnectBusiness() = %v", err)
	}
	if err := stub.SendManualToJID(1, "15550001111@s.whatsapp.net", "hi"); !errors.Is(err, ErrWhatsAppNotBuilt) {
		t.Fatalf("SendManualToJID() = %v", err)
	}
	if got := stub.PendingQRCode(1); got != "" {
		t.Fatalf("PendingQRCode() = %q", got)
	}
	if got := stub.GetStatusDetail(1); got.Status != database.WhatsAppDeviceStatusDisconnected {
		t.Fatalf("GetStatusDetail() = %+v", got)
	}
	stub.Stop()
}
