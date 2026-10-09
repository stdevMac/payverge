package main

import (
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// logWhatsAppDisabled records why the WhatsApp manager did not start. The
// decision itself is services.WhatsAppAvailable (build tag + WHATSAPP_ENABLED).
// WHATSAPP_ENABLED=true on a binary built without `-tags whatsapp` logs at warn
// level with the rebuild instructions: the operator asked for a channel this
// image cannot provide.
func logWhatsAppDisabled() {
	msg, warn := services.WhatsAppStartupMessage()
	if warn {
		logger.Logger.Warn(msg)
		return
	}
	logger.Logger.Info(msg)
}
