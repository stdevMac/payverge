package server

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"payverge/internal/database"
	"payverge/internal/structs"
)

// UpdateNotificationPreferencesRequest represents the request body for updating notification preferences
type UpdateNotificationPreferencesRequest struct {
	Preferences structs.NotificationPreferences `json:"preferences" binding:"required"`
}

// UpdateNotificationPreferences updates a user's notification preferences for both email and telegram
func UpdateNotificationPreferences(c *gin.Context) {
	var request UpdateNotificationPreferencesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get address from middleware
	address, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Address not found in token"})
		return
	}

	// Convert address to string
	addressStr, ok := address.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid address type in token"})
		return
	}

	// Get user from database
	lowercaseAddr := strings.ToLower(addressStr)
	user, err := database.GetUserByAddress(lowercaseAddr)
	if err != nil {
		log.Printf("Error getting user: %v for address: %s", err, lowercaseAddr)
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found for address: " + addressStr})
		return
	}

	// Update user's notification preferences
	if err := database.UpdateNotificationPreferences(user, request.Preferences); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update notification preferences"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Notification preferences updated successfully",
		"address":     user.Address,
		"preferences": request.Preferences,
	})
}

// GetNotificationPreferences retrieves a user's notification preferences
func GetNotificationPreferences(c *gin.Context) {
	// Get address from middleware
	address, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Address not found in token"})
		return
	}

	// Convert address to string
	addressStr, ok := address.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid address type in token"})
		return
	}

	// Get user from database
	lowercaseAddr := strings.ToLower(addressStr)
	user, err := database.GetUserByAddress(lowercaseAddr)
	if err != nil {
		log.Printf("Error getting user: %v for address: %s", err, lowercaseAddr)
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found for address: " + addressStr})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"address":     user.Address,
		"preferences": user.NotificationPreferences,
	})
}
