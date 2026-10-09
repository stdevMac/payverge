package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/auth"
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// AdminUserHandler handles admin operations on user accounts
type AdminUserHandler struct {
	authService *auth.AuthService
}

// NewAdminUserHandler creates a new AdminUserHandler
func NewAdminUserHandler(authService *auth.AuthService) *AdminUserHandler {
	return &AdminUserHandler{
		authService: authService,
	}
}

// --- Response types ---

// AdminActionResponse represents an admin action in the API response.
type AdminActionResponse struct {
	ID         uint            `json:"id"`
	AdminEmail string          `json:"admin_email"`
	ActionType string          `json:"action_type"`
	Details    json.RawMessage `json:"details"`
	CreatedAt  time.Time       `json:"created_at"`
}

// --- Request types ---

// AdminCloseAccountRequest represents the request to close a business account.
type AdminCloseAccountRequest struct {
	Reason     string `json:"reason" binding:"required"`
	BusinessID *uint  `json:"business_id"`
}

// --- Handlers ---

// AdminGetUserDetail returns combined user + business + admin action history.
// GET /api/v1/admin/users/:id/detail
func (h *AdminUserHandler) AdminGetUserDetail(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid user ID")
		return
	}

	// Fetch user
	var user database.User
	if err := database.GetDB().First(&user, uint(userID)).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeUserNotFound, "User not found")
		return
	}

	// Fetch businesses (may be empty)
	var business *database.Business
	businessSummaries := []gin.H{}
	if businesses, err := findBusinessesForUser(user); err == nil {
		for _, b := range businesses {
			businessSummaries = append(businessSummaries, gin.H{
				"id":        b.ID,
				"name":      b.Name,
				"is_active": b.IsActive,
				"closed_at": b.ClosedAt,
			})
		}
		var businessIDParam *uint
		if raw := strings.TrimSpace(c.Query("business_id")); raw != "" {
			if id, parseErr := strconv.ParseUint(raw, 10, 64); parseErr == nil {
				parsed := uint(id)
				businessIDParam = &parsed
			}
		}
		if selected, err := resolveBusinessForUserDetail(user, businessIDParam); err == nil {
			business = selected
		}
	}

	// Fetch admin actions (scoped to selected business when available)
	var actions []database.AdminAction
	if business != nil {
		ownerUserID := uint(userID)
		actions, _ = database.GetAdminActionsForBusinessContext(business.ID, ownerUserID, 50)
	} else {
		actions, _ = database.GetAdminActionsForUser(uint(userID), 50)
	}
	actionResponses := mapAdminActionResponses(database.GetDB(), actions)

	c.JSON(http.StatusOK, gin.H{
		"user":          user,
		"business":      business,
		"businesses":    businessSummaries,
		"admin_actions": actionResponses,
	})
}

// AdminCloseAccount soft-closes a business account.
// POST /api/v1/admin/users/:id/close
func (h *AdminUserHandler) AdminCloseAccount(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid user ID")
		return
	}

	var req AdminCloseAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Reason is required")
		return
	}

	adminUserID, ok := requireAdminIdentity(c)
	if !ok {
		return
	}

	var user database.User
	if err := database.GetDB().First(&user, uint(userID)).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeUserNotFound, "User not found")
		return
	}

	businessPtr, err := resolveBusinessForAdminMutation(user, req.BusinessID)
	if err != nil {
		writeBusinessResolutionError(c, err)
		return
	}
	business := *businessPtr

	if business.ClosedAt != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Account is already closed")
		return
	}

	// Update business in DB — use WHERE closed_at IS NULL for concurrency safety
	now := time.Now()
	result := database.GetDB().Model(&database.Business{}).
		Where("id = ? AND closed_at IS NULL", business.ID).
		Updates(map[string]interface{}{
			"closed_at":     &now,
			"closed_reason": req.Reason,
		})
	if result.Error != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to close account")
		return
	}
	if result.RowsAffected == 0 {
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Account was already closed by another admin")
		return
	}

	logAdminActionBestEffort(adminUserID, uint(userID), "close_account", map[string]interface{}{
		"reason":      req.Reason,
		"business_id": business.ID,
	})

	// Send closure email
	if user.Email != "" && emails.EmailServerInstance != nil {
		lang := adminNormalizeLanguage(business.DefaultLanguage)
		ownerName := adminFirstNonEmpty(user.Name, user.Email)
		if err := emails.EmailServerInstance.SendAccountClosureEmail(
			[]string{user.Email},
			ownerName,
			business.Name,
			req.Reason,
			lang,
		); err != nil {
			log.Printf("[Admin Close] Failed to send closure email for user %d: %v", userID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"business_id":   business.ID,
		"business_name": business.Name,
	})
}

// AdminResetPassword triggers a password reset email for a user.
// POST /api/v1/admin/users/:id/reset-password
func (h *AdminUserHandler) AdminResetPassword(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid user ID")
		return
	}

	adminUserID, ok := requireAdminIdentity(c)
	if !ok {
		return
	}

	var user database.User
	if err := database.GetDB().First(&user, uint(userID)).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeUserNotFound, "User not found")
		return
	}

	if user.AuthMethod != "email" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "User does not use email/password authentication")
		return
	}

	if user.Email == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "User has no email address")
		return
	}

	// Admin-initiated reset bypasses the public per-email limiter.
	token, err := h.authService.RequestPasswordResetForAdmin(user.Email)
	if err != nil {
		log.Printf("[Admin ResetPassword] Failed for user %d: %v", userID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to generate reset token")
		return
	}

	// Send reset email — admin-initiated variant
	resetLink := adminGetFrontendBaseURL() + "/reset-password?token=" + url.QueryEscape(token)
	htmlBody, textBody := adminResetPasswordEmailBodies(resetLink)

	if emails.EmailServerInstance != nil {
		if err := emails.EmailServerInstance.SendCustomEmail([]string{user.Email}, "Reset Your Password", htmlBody, textBody); err != nil {
			log.Printf("[Admin ResetPassword] Failed to send reset email for user %d: %v", userID, err)
		}
	}

	logAdminActionBestEffort(adminUserID, uint(userID), "reset_password", map[string]interface{}{})

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// --- Helper functions ---

// findBusinessesForUser returns all businesses owned by a user (user_id or wallet).
func findBusinessesForUser(user database.User) ([]database.Business, error) {
	var businesses []database.Business
	if err := database.GetDB().Where("user_id = ?", user.ID).Order("created_at DESC").Find(&businesses).Error; err != nil {
		return nil, err
	}
	if len(businesses) == 0 && user.Address != "" {
		if err := database.GetDB().Where("LOWER(owner_address) = LOWER(?)", user.Address).Order("created_at DESC").Find(&businesses).Error; err != nil {
			return nil, err
		}
	}
	if len(businesses) == 0 {
		return nil, fmt.Errorf("no businesses found")
	}
	return businesses, nil
}

func resolveBusinessForUserDetail(user database.User, businessID *uint) (*database.Business, error) {
	businesses, err := findBusinessesForUser(user)
	if err != nil {
		return nil, err
	}
	if businessID != nil && *businessID > 0 {
		for i := range businesses {
			if businesses[i].ID == *businessID {
				return &businesses[i], nil
			}
		}
		return nil, fmt.Errorf("business not found")
	}
	return &businesses[0], nil
}

// ambiguousBusinessError signals that an admin mutation targeted a user who owns
// more than one business without specifying which one. The handler surfaces the
// list so the admin UI can re-submit with an explicit business_id.
type ambiguousBusinessError struct {
	businesses []database.Business
}

func (e *ambiguousBusinessError) Error() string {
	return "user owns multiple businesses; business_id required"
}

// resolveBusinessForAdminMutation resolves the business an admin mutation targets.
//   - explicit business_id: load it and verify it belongs to the user (else error).
//   - no business_id + exactly one business: use it.
//   - no business_id + multiple businesses: return *ambiguousBusinessError so the
//     caller can respond 422 with the choices (never silently pick the newest).
func resolveBusinessForAdminMutation(user database.User, businessID *uint) (*database.Business, error) {
	if businessID != nil && *businessID > 0 {
		var business database.Business
		if err := database.GetDB().First(&business, *businessID).Error; err != nil {
			return nil, fmt.Errorf("business not found")
		}
		if !businessBelongsToUser(business, user) {
			return nil, fmt.Errorf("business does not belong to user")
		}
		return &business, nil
	}

	businesses, err := findBusinessesForUser(user)
	if err != nil {
		return nil, err
	}
	if len(businesses) > 1 {
		return nil, &ambiguousBusinessError{businesses: businesses}
	}
	return &businesses[0], nil
}

// writeBusinessResolutionError maps a resolveBusinessForAdminMutation error to the
// appropriate HTTP response.
func writeBusinessResolutionError(c *gin.Context, err error) {
	var amb *ambiguousBusinessError
	if errors.As(err, &amb) {
		list := make([]gin.H, 0, len(amb.businesses))
		for _, b := range amb.businesses {
			list = append(list, gin.H{"id": b.ID, "name": b.Name})
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":      "This user owns multiple businesses; specify business_id",
			"code":       "business_id_required",
			"businesses": list,
		})
		return
	}
	if strings.Contains(err.Error(), "not found") {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
		return
	}
	if strings.Contains(err.Error(), "does not belong") {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Business does not belong to user")
		return
	}
	server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "User has no business")
}

// getAdminUserID extracts the admin user ID from the Gin context.
func getAdminUserID(c *gin.Context) (uint, error) {
	return server.ExtractAdminUserID(c)
}

// adminNormalizeLanguage normalizes the language code for email templates.
func adminNormalizeLanguage(lang string) string {
	if strings.EqualFold(strings.TrimSpace(lang), "es") {
		return "es"
	}
	return "en"
}

// adminFirstNonEmpty returns the first non-empty string from the provided values.
func adminFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// adminResetPasswordEmailBodies renders the admin-initiated reset email with
// the instance identity (PRODUCT_NAME, SUPPORT_EMAIL) instead of upstream
// contacts. Without SUPPORT_EMAIL the recipient is pointed at their admin.
func adminResetPasswordEmailBodies(resetLink string) (htmlBody, textBody string) {
	product := config.ProductName()
	contactHTML := "your administrator"
	contactText := contactHTML
	if support := config.SupportEmail(); support != "" {
		contactHTML = `<a href="mailto:` + html.EscapeString(support) + `">` + html.EscapeString(support) + `</a>`
		contactText = support
	}
	htmlBody = "<h2>Password Reset Request</h2>" +
		"<p>A " + html.EscapeString(product) + " administrator has initiated a password reset for your account.</p>" +
		"<p>Click the link below to reset your password. This link expires in 1 hour.</p>" +
		"<p><a href=\"" + html.EscapeString(resetLink) + "\">Reset Your Password</a></p>" +
		"<p>If you did not expect this, please contact " + contactHTML + ".</p>"
	textBody = "Password Reset Request\n\nA " + product + " administrator has initiated a password reset for your account.\n\n" +
		"Visit the following link to reset your password (expires in 1 hour):\n" + resetLink + "\n\n" +
		"If you did not expect this, please contact " + contactText + "."
	return htmlBody, textBody
}

// adminGetFrontendBaseURL returns the frontend base URL from environment.
// Prefixed with "admin" to avoid collision with getFrontendBaseURL in auth/handlers.go.
func adminGetFrontendBaseURL() string {
	return config.FrontendBaseURL()
}
