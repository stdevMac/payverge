package services

// Build-tag-neutral WhatsApp helpers. Everything in this file is compiled into
// every binary: it must NOT import go.mau.fi/* (GPL-3.0). The whatsmeow-backed
// manager lives in whatsapp_manager.go / whatsapp_client.go behind the
// `whatsapp` build tag; whatsapp_stub.go provides the disabled default.

import (
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// WhatsAppStatus is the structured channel lifecycle returned to the dashboard.
// Status values match database.WhatsAppDeviceStatus* constants.
type WhatsAppStatus struct {
	Status          string     `json:"status"`
	LastErrorCode   string     `json:"last_error_code,omitempty"`
	LastConnectedAt *time.Time `json:"last_connected_at,omitempty"`
	RetryAt         *time.Time `json:"retry_at,omitempty"`
}

// buildWhatsAppDeliveryHint mirrors the AI Waiter three-state branching so
// guests messaging us through WhatsApp don't get told to use an in-house flow
// that doesn't exist for this merchant.
func buildWhatsAppDeliveryHint(settings *database.DeliverySettings) string {
	if settings == nil || !settings.DeliveryEnabled {
		return ""
	}
	if settings.InHouseDeliveryEnabled {
		return "Delivery is available — guests can use the 'Order Delivery' button on the business page."
	}
	if settings.ThirdPartyEnabled {
		return "Delivery is available through our delivery partners — guests can pick one from the delivery section on the business page."
	}
	return ""
}
