package server

import "github.com/stdevmac/payverge/backend/internal/services"

var whatsAppManager *services.WhatsAppManager

// SetWhatsAppManager sets the WhatsApp manager for the server package
func SetWhatsAppManager(manager *services.WhatsAppManager) {
	whatsAppManager = manager
}

// GetWhatsAppManager returns the WhatsApp manager
func GetWhatsAppManager() *services.WhatsAppManager {
	return whatsAppManager
}
