package server

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

// UpdateNotificationPreferencesRequest represents the request body for updating notification preferences
type UpdateNotificationPreferencesRequest struct {
	Preferences structs.NotificationPreferences `json:"preferences" binding:"required"`
}

// UpdateNotificationPreferences updates a user's email notification preferences
func UpdateNotificationPreferences(c *gin.Context) {
	var request UpdateNotificationPreferencesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		RespondBindError(c, err)
		return
	}

	user, err := resolveNotificationUser(c)
	if err != nil {
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
	user, err := resolveNotificationUser(c)
	if err != nil {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"address":     user.Address,
		"preferences": user.NotificationPreferences,
	})
}

// resolveNotificationUser finds the authenticated user by user_id or address.
func resolveNotificationUser(c *gin.Context) (structs.User, error) {
	if userID, exists := c.Get("user_id"); exists {
		var uid uint
		switch v := userID.(type) {
		case uint:
			uid = v
		case float64:
			uid = uint(v)
		case int:
			uid = uint(v)
		case int64:
			uid = uint(v)
		}
		if uid > 0 {
			user, err := database.GetUserByID(uid)
			if err == nil {
				return user, nil
			}
			log.Printf("Error getting user by ID %d: %v", uid, err)
		}
	}

	if address, exists := c.Get("address"); exists {
		if addressStr, ok := address.(string); ok {
			lowercaseAddr := strings.ToLower(addressStr)
			user, err := database.GetUserByAddress(lowercaseAddr)
			if err == nil {
				return user, nil
			}
			log.Printf("Error getting user by address %s: %v", lowercaseAddr, err)
		}
	}

	c.JSON(http.StatusUnauthorized, gin.H{"error": "Could not identify user from token"})
	return structs.User{}, http.ErrAbortHandler
}
