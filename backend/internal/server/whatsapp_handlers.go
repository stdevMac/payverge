package server

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/services"
)

// ConnectWhatsApp initiates the connection process and returns a QR code stream or string
func ConnectWhatsApp(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	manager := GetWhatsAppManager()
	if manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "WhatsApp service not initialized"})
		return
	}

	// This returns a channel that updates with the QR code
	qrChan, err := manager.ConnectBusiness(uint(businessID))
	if err != nil {
		fmt.Printf("Error connecting WhatsApp for business %d: %v\n", businessID, err)
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not process WhatsApp request")
		return
	}

	if qrChan == nil {
		// Already connected
		c.JSON(http.StatusOK, gin.H{"status": "connected", "message": "Already connected"})
		return
	}

	// The first pairing code comes back inline. whatsmeow then rotates the code
	// every ~20s; the dashboard picks up each new one from GET /whatsapp/qr.
	select {
	case code, ok := <-qrChan:
		if !ok || code == "" {
			// The QR channel closed before issuing a code (timeout, or the
			// pairing was superseded); the operator has to start over.
			RespondWithError(c, http.StatusBadGateway, ErrCodeInternal, "WhatsApp did not issue a pairing code; try again")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  "scanning",
			"qr_code": code,
		})
	case <-time.After(whatsAppFirstQRWait):
		RespondWithError(c, http.StatusGatewayTimeout, ErrCodeInternal, "Timed out waiting for a WhatsApp pairing code; try again")
	case <-c.Request.Context().Done():
		return
	}
}

// whatsAppFirstQRWait bounds how long POST /whatsapp/connect waits for
// whatsmeow's first pairing code before giving up. It stays well under the
// dashboard's 30s request timeout (frontend/src/api/tools/instance.ts) so the
// browser gets the code or this route's 504, not its own abort. The first
// code normally arrives within seconds of the socket handshake.
var whatsAppFirstQRWait = 20 * time.Second

// GetWhatsAppQR returns the pairing code the operator should scan right now.
// whatsmeow rotates it while the phone has not scanned yet, so the dashboard
// polls this route during pairing. The code is omitted once the device is
// paired or the attempt ended. Mounted with settings:write, not the
// settings:read of /status: whoever scans the code links their own WhatsApp
// account to the business.
func GetWhatsAppQR(c *gin.Context) {
	businessID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	manager := GetWhatsAppManager()
	if manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "WhatsApp service not initialized"})
		return
	}

	id := uint(businessID)
	out := gin.H{"status": manager.GetStatusDetail(id).Status}
	if code := manager.PendingQRCode(id); code != "" {
		out["qr_code"] = code
	}
	c.JSON(http.StatusOK, out)
}

// GetWhatsAppStatus returns the current connection status.
// When WhatsApp is dark (WHATSAPP_ENABLED != true / manager nil), return a
// healthy disconnected payload so the AI Waiter channel card soft-degrades
// instead of 404ing the status route. `built` says whether this binary has the
// whatsapp tag and `requested` whether WHATSAPP_ENABLED=true, so the card can
// tell "not turned on" (requested=false) from "turned on but failed to start"
// (built, requested, yet enabled=false).
func GetWhatsAppStatus(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	manager := GetWhatsAppManager()
	if manager == nil {
		c.JSON(http.StatusOK, gin.H{
			"status":    "disconnected",
			"enabled":   false,
			"built":     services.WhatsAppBuilt,
			"requested": services.WhatsAppRequested(),
		})
		return
	}

	detail := manager.GetStatusDetail(uint(businessID))
	// Keep legacy `status` and add optional lifecycle fields for the dashboard.
	out := gin.H{
		"status":    detail.Status,
		"enabled":   true,
		"built":     services.WhatsAppBuilt,
		"requested": services.WhatsAppRequested(),
	}
	if detail.LastErrorCode != "" {
		out["last_error_code"] = detail.LastErrorCode
	}
	if detail.LastConnectedAt != nil {
		out["last_connected_at"] = detail.LastConnectedAt.UTC().Format(time.RFC3339)
	}
	if detail.RetryAt != nil {
		out["retry_at"] = detail.RetryAt.UTC().Format(time.RFC3339)
	}
	c.JSON(http.StatusOK, out)
}

// DisconnectWhatsApp disconnects the session
func DisconnectWhatsApp(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	manager := GetWhatsAppManager()
	if manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "WhatsApp service not initialized"})
		return
	}

	if err := manager.DisconnectBusiness(uint(businessID)); err != nil {
		log.Printf("Failed to disconnect business %d: %v", businessID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disconnect"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "disconnected"})
}
