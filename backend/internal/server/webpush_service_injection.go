package server

import "github.com/stdevmac/payverge/backend/internal/services"

var webPushService *services.WebPushService

// SetWebPushService makes the WebPushService available to server-package
// handlers. Called once during application startup.
func SetWebPushService(svc *services.WebPushService) {
	webPushService = svc
}

// GetWebPushService returns the package-level WebPushService (may be nil if
// VAPID keys were not configured).
func GetWebPushService() *services.WebPushService {
	return webPushService
}
