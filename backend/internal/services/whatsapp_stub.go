//go:build !whatsapp

package services

// Default (non-`whatsapp`) build: the whatsmeow integration is compiled out so
// the binary carries no go.mau.fi/* (GPL-3.0) code. Build with `-tags whatsapp`
// to get the real manager from whatsapp_manager.go / whatsapp_client.go.

import (
	"errors"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guardrails"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// WhatsAppBuilt reports whether this binary was compiled with the whatsmeow
// integration. False in default builds.
const WhatsAppBuilt = false

// ErrWhatsAppNotBuilt is returned by every entry point of the stub manager.
var ErrWhatsAppNotBuilt = errors.New("whatsapp: integration not compiled in (rebuild with -tags whatsapp)")

// WhatsAppManager is an inert placeholder so callers compile unchanged.
// NewWhatsAppManager never returns a usable instance in this build.
type WhatsAppManager struct{}

// NewWhatsAppManager always fails in default builds.
func NewWhatsAppManager(_ *database.DB, _ *AIService) (*WhatsAppManager, error) {
	return nil, ErrWhatsAppNotBuilt
}

func (wm *WhatsAppManager) WithClassifier(guardrails.InputClassifier) *WhatsAppManager { return wm }
func (wm *WhatsAppManager) WithCostGate(*llm.AICostGate) *WhatsAppManager              { return wm }
func (wm *WhatsAppManager) RestoreAllSessions() error                                  { return ErrWhatsAppNotBuilt }
func (wm *WhatsAppManager) Stop()                                                      {}
func (wm *WhatsAppManager) ConnectBusiness(uint) (<-chan string, error) {
	return nil, ErrWhatsAppNotBuilt
}
func (wm *WhatsAppManager) DisconnectBusiness(uint) error { return ErrWhatsAppNotBuilt }
func (wm *WhatsAppManager) PendingQRCode(uint) string     { return "" }
func (wm *WhatsAppManager) GetStatusDetail(uint) WhatsAppStatus {
	return WhatsAppStatus{Status: database.WhatsAppDeviceStatusDisconnected}
}
func (wm *WhatsAppManager) SendManualToJID(uint, string, string) error { return ErrWhatsAppNotBuilt }
