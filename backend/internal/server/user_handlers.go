package server

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
)

// GetUser fetch the information of a user
func GetUser(c *gin.Context) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.UserResponseTime.WithLabelValues("get").Observe(duration)
	}()

	address := c.Param("address")
	addressLower := strings.ToLower(address)

	// Authorization: a caller may only read their OWN record unless they are a
	// platform admin. The response includes Email, so an unrestricted read is
	// a latent PII disclosure.
	callerAddress := c.GetString("address")
	if !strings.EqualFold(callerAddress, addressLower) && !hasPlatformAdminRole(c) {
		RespondWithError(c, http.StatusForbidden, ErrCodeForbidden, "Access denied")
		return
	}

	user, err := database.GetUserByAddress(addressLower)
	if err != nil {
		metrics.UserOperations.WithLabelValues("get", "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error registering user"})
		return
	}

	userValsToReturn := struct {
		Username string       `json:"username"`
		Address  string       `json:"address"`
		JoinedAt time.Time    `json:"joined_at"`
		Email    string       `json:"email"`
		Role     structs.Role `json:"role"`
		Language string       `json:"language_selected"`
	}{
		Username: user.Username,
		Address:  user.Address,
		JoinedAt: user.JoinedAt,
		Email:    user.Email,
		Role:     user.Role,
		Language: user.LanguageSelected,
	}

	c.JSON(http.StatusOK, userValsToReturn)
}

// UpdateUser user information
func UpdateUser(c *gin.Context) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.UserResponseTime.WithLabelValues("update").Observe(duration)
	}()

	// Get authenticated user's address from JWT
	authAddress, exists := c.Get("address")
	if !exists {
		metrics.UserOperations.WithLabelValues("update", "error").Inc()
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}
	address, ok := authAddress.(string)
	if !ok || address == "" {
		metrics.UserOperations.WithLabelValues("update", "error").Inc()
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication"})
		return
	}

	var updates struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if err := c.ShouldBindJSON(&updates); err != nil {
		metrics.UserOperations.WithLabelValues("update", "error").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	userDB, err := database.GetUserByAddress(address)
	if err != nil {
		metrics.UserOperations.WithLabelValues("update", "error").Inc()
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if updates.Email != "" && !strings.EqualFold(strings.TrimSpace(updates.Email), userDB.Email) {
		metrics.UserOperations.WithLabelValues("update", "error").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email changes require verification"})
		return
	}
	if strings.TrimSpace(updates.Username) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No supported profile correction supplied"})
		return
	}

	if err := database.UpdateUserProfileByAddress(address, updates.Username); err != nil {
		metrics.UserOperations.WithLabelValues("update", "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	metrics.UserOperations.WithLabelValues("update", "success").Inc()
	userDB.Username = strings.TrimSpace(updates.Username)
	c.JSON(http.StatusOK, gin.H{"user": userDB})
}

func SetLanguage(c *gin.Context) {
	var params struct {
		Language string `json:"language"`
	}

	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	if !locales.IsOperatorLocale(params.Language) {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Unsupported language")
		return
	}

	// Get identity from context (secure)
	address := c.GetString("address") // from web3 token or linked wallet
	email := c.GetString("email")     // from oauth token

	if address == "" && email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User identity not found in context"})
		return
	}

	// Update language using available identifier
	err := database.UpdateUserLanguage(strings.ToLower(address), email, params.Language)
	if err != nil {
		log.Printf("Error updating user language: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating user preference"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Language updated successfully"})
}
