package server

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"payverge/internal/database"
	"payverge/internal/metrics"
	"payverge/internal/structs"
	"payverge/internal/utils"

	"math/big"
)

var ChainID big.Int

func SetChainId(chainID int64) {
	ChainID = *big.NewInt(chainID)
}

// GetUser fetch the information of a user
func GetUser(c *gin.Context) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.UserResponseTime.WithLabelValues("get").Observe(duration)
	}()

	address := c.Param("address")
	addressLower := strings.ToLower(address)
	user, err := database.GetUserByAddress(addressLower)
	if err != nil {
		metrics.UserOperations.WithLabelValues("get", "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error registering user"})
		return
	}

	userValsToReturn := struct {
		Username               string           `json:"username"`
		Address                string           `json:"address"`
		JoinedAt               time.Time        `json:"joined_at"`
		Email                  string           `json:"email"`
		Role                   structs.Role     `json:"role"`
		ReferralCode           string           `json:"referral_code"`
		Referrer               string           `json:"referrer"`
		Referees               map[string]int64 `json:"referees"`
		TGChatID               int64            `json:"telegram_chat_id"`
		NotificationPreference string           `json:"notification_preference"`
		Language               string           `json:"language_selected"`
	}{
		Username:               user.Username,
		Address:                user.Address,
		JoinedAt:               user.JoinedAt,
		Email:                  user.Email,
		Role:                   user.Role,
		ReferralCode:           user.ReferralCode,
		Referrer:               user.Referrer,
		Referees:               user.RefereePoints,
		TGChatID:               user.TGChatID,
		NotificationPreference: string(user.NotificationPreference),
		Language:               user.LanguageSelected,
	}

	c.JSON(http.StatusOK, userValsToReturn)

	// Track user profile view
	properties := map[string]interface{}{
		"user_address":   addressLower,
		"referral_count": len(user.RefereePoints),
		"timestamp":      time.Now(),
	}
	err = metrics.TrackGeneralEvent("User Profile Viewed", properties)
	if err != nil {
		log.Printf("Failed to track user profile view: %v", err)
	}
}

// UpdateUser user information
func UpdateUser(c *gin.Context) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.UserResponseTime.WithLabelValues("update").Observe(duration)
	}()

	var user structs.User

	if err := c.ShouldBindJSON(&user); err != nil {
		metrics.UserOperations.WithLabelValues("update", "error").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	// Get the user from the database
	userDB, err := database.GetUserByAddress(user.Address)
	if err != nil {
		metrics.UserOperations.WithLabelValues("update", "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting user"})
		return
	}

	userDB.Username = user.Username
	if userDB.Email != user.Email {
		userDB.Email = user.Email
	}

	err = database.UpdateUser(userDB)
	if err != nil {
		metrics.UserOperations.WithLabelValues("update", "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating user"})
		return
	}

	metrics.UserOperations.WithLabelValues("update", "success").Inc()
	c.JSON(http.StatusOK, gin.H{"message": "User updated successfully"})

	// Track user update
	properties := map[string]interface{}{
		"user_address":   user.Address,
		"updated_fields": []string{"email", "name", "phone"}, // Add the fields that were updated
		"timestamp":      time.Now(),
	}
	err = metrics.TrackGeneralEvent("User Profile Updated", properties)
	if err != nil {
		log.Printf("Failed to track user profile update: %v", err)
	}
}

// SetReferrer it's a function to set the referrer
func SetReferrer(c *gin.Context) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.UserResponseTime.WithLabelValues("set_referrer").Observe(duration)
	}()

	var params struct {
		Address  string `json:"address"`
		Referrer string `json:"referrer_code"`
	}

	if err := c.ShouldBindJSON(&params); err != nil {
		metrics.UserOperations.WithLabelValues("set_referrer", "error").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	address := strings.ToLower(params.Address)
	user, err := database.GetUserByAddress(address)
	if err != nil {
		// If the user is not found, we create a new user
		user = newUser(address)
		err = database.RegisterUser(user)
		if err != nil {
			metrics.UserOperations.WithLabelValues("set_referrer", "error").Inc()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating user"})
			return
		}
		metrics.ActiveUsers.Inc()
	}

	if user.Referrer != "" {
		metrics.UserOperations.WithLabelValues("set_referrer", "duplicate").Inc()
		metrics.UserReferrals.WithLabelValues("duplicate").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Referrer already set"})
		return
	}

	referrerCode := strings.ToLower(params.Referrer)

	// find the referrer, and update the referees
	referrer, err := database.GetUserByReferralCode(referrerCode)
	if err != nil {
		metrics.UserOperations.WithLabelValues("set_referrer", "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting referrer"})
		return
	}

	if user.Address == referrer.Address {
		metrics.UserOperations.WithLabelValues("set_referrer", "error").Inc()
		metrics.UserReferrals.WithLabelValues("self_referral").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "User cannot refer themselves"})
		return
	}

	referrer.AddReferee(address)
	user.Referrer = referrer.Address

	err = database.UpdateUser(user)
	if err != nil {
		metrics.UserOperations.WithLabelValues("set_referrer", "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating user"})
		return
	}

	err = database.UpdateUser(referrer)
	if err != nil {
		metrics.UserOperations.WithLabelValues("set_referrer", "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating referrer"})
		return
	}

	metrics.UserOperations.WithLabelValues("set_referrer", "success").Inc()
	metrics.UserReferrals.WithLabelValues("success").Inc()
	c.JSON(http.StatusOK, gin.H{"message": "Referrer set successfully"})

	// Track referral link
	properties := map[string]interface{}{
		"user_address":     user.Address,
		"referrer_address": referrer.Address,
		"timestamp":        time.Now(),
	}
	err = metrics.TrackGeneralEvent("Referral Link Created", properties)
	if err != nil {
		log.Printf("Failed to track referral link: %v", err)
	}
}

func UpdateNotificationPreference(c *gin.Context) {
	var params struct {
		Address                string `json:"address"`
		NotificationPreference string `json:"notification_preference"`
	}

	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	if params.NotificationPreference != "email" && params.NotificationPreference != "telegram" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid notification preference"})
		return
	}

	address := strings.ToLower(params.Address)
	user, err := database.GetUserByAddress(address)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting user"})
		return
	}

	if params.NotificationPreference == "telegram" {
		if user.TGChatID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "User has not linked their Telegram account"})
			return
		}
	}

	user.NotificationPreference = structs.NotificationPreference(params.NotificationPreference)

	err = database.UpdateUser(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification preference updated successfully"})

	// Track notification preference update
	properties := map[string]interface{}{
		"user_address":            user.Address,
		"notification_preference": user.NotificationPreference,
		"timestamp":               time.Now(),
	}
	err = metrics.TrackGeneralEvent("Notification Preference Updated", properties)
	if err != nil {
		log.Printf("Failed to track notification preference update: %v", err)
	}

}

func SetLanguage(c *gin.Context) {
	var params struct {
		Address  string `json:"address"`
		Language string `json:"language"`
	}

	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	address := strings.ToLower(params.Address)
	user, err := database.GetUserByAddress(address)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting user"})
		return
	}

	user.LanguageSelected = params.Language

	err = database.UpdateUser(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Language updated successfully"})
}

func newUser(address string) structs.User {
	return structs.User{
		Username:                "",
		JoinedAt:                time.Now(),
		Email:                   "",
		Address:                 address,
		Role:                    structs.RoleUser,
		NotificationPreference:  structs.EmailNotificationPreference,
		NotificationPreferences: structs.NewDefaultNotificationPreferences(),
		LanguageSelected:        "",
		ReferralCode:            utils.GenerateReferralCode(),
		Referrer:                "",
		RefereePoints:           make(map[string]int64),
		Notifications:           make([]structs.Notification, 0),
	}
}
