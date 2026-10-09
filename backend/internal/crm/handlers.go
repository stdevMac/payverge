package crm

import (
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/utils"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Handler handles CRM HTTP requests
type Handler struct {
	service *Service
}

// NewHandler creates a new CRM handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func getAuthenticatedCustomerID(c *gin.Context) (uint, bool) {
	customerIDRaw, exists := c.Get("customer_id")
	if !exists {
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeNotAuthenticated, "Not authenticated")
		return 0, false
	}

	switch v := customerIDRaw.(type) {
	case float64:
		return uint(v), true
	case uint:
		return v, true
	case int:
		return uint(v), true
	default:
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeNotAuthenticated, "Invalid customer identity")
		return 0, false
	}
}

// Customer Authentication Handlers

// RegisterCustomerRequest represents the registration request
type RegisterCustomerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Name     string `json:"name" binding:"required"`
}

// RegisterCustomer handles customer registration
func (h *Handler) RegisterCustomer(c *gin.Context) {
	var req RegisterCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	req.Email = normalizeCustomerEmail(req.Email)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Name is required")
		return
	}

	customer, err := h.service.RegisterCustomer(req.Email, req.Password, req.Name)
	if err != nil {
		if errors.Is(err, ErrCustomerEmailTaken) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, ErrCustomerEmailTaken.Error())
			return
		}
		if errors.Is(err, ErrCustomerNameRequired) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, ErrCustomerNameRequired.Error())
			return
		}
		log.Printf("[CRM] register customer failed: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not register account")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Customer registered successfully",
		"customer": customer,
	})
}

// LoginCustomerRequest represents the login request
type LoginCustomerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// LoginCustomer handles customer login
func (h *Handler) LoginCustomer(c *gin.Context) {
	var req LoginCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	req.Email = normalizeCustomerEmail(req.Email)

	customer, err := h.service.AuthenticateCustomer(req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCustomerCredentials) {
			server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeTokenInvalid, "Invalid email or password")
			return
		}
		log.Printf("[CRM] customer login failed: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not sign in")
		return
	}

	var token string
	if session.GlobalStore != nil {
		sess, sessErr := session.GlobalStore.Create(session.CreateInput{
			UserID:    &customer.ID,
			Provider:  "customer",
			IPAddress: c.ClientIP(),
			UserAgent: c.GetHeader("User-Agent"),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		})
		if sessErr != nil {
			log.Printf("[CRM] Failed to create session: %v", sessErr)
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create session")
			return
		}
		token, err = server.GenerateCustomerToken(customer.ID, customer.Email, sess.ID)
		if err != nil {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to generate customer token")
			return
		}
		if err := session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)); err != nil {
			log.Printf("[CRM] Failed to persist customer token hash for session %v: %v", sess.ID, err)
		}
		refreshToken, refreshErr := session.GenerateRefreshToken()
		if refreshErr != nil {
			log.Printf("[CRM] Failed to generate customer refresh token: %v", refreshErr)
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create session")
			return
		}
		if err := session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshToken), time.Now().Add(7*24*time.Hour)); err != nil {
			log.Printf("[CRM] Failed to store customer refresh token: %v", err)
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create session")
			return
		}
		utils.SetStrictSessionCookie(c, "customer_refresh_token", refreshToken, 7*24*3600)
	} else {
		token, err = server.GenerateCustomerToken(customer.ID, customer.Email)
		if err != nil {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to generate customer token")
			return
		}
	}

	utils.SetSessionCookie(c, "customer_token", token, 15*60)

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Login successful",
		"customer": customer,
		"token":    token,
	})
}

// LogoutCustomer clears the customer auth cookie.
func (h *Handler) LogoutCustomer(c *gin.Context) {
	if session.GlobalStore != nil {
		if tokenStr := extractCustomerCookieToken(c); tokenStr != "" {
			h.revokeCustomerCookieSession(tokenStr)
		}
		if refreshRaw, err := c.Cookie("customer_refresh_token"); err == nil && refreshRaw != "" {
			h.revokeCustomerRefreshSession(refreshRaw)
		}
	}
	utils.ClearAllAuthCookies(c)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Logged out successfully"})
}

func (h *Handler) revokeCustomerCookieSession(tokenStr string) {
	var sess session.UserSession
	if err := h.service.db.
		Select("id").
		Where("session_token = ? AND provider = ? AND revoked = false", session.HashToken(tokenStr), "customer").
		First(&sess).Error; err != nil {
		return
	}
	_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonUserLogout)
}

func (h *Handler) revokeCustomerRefreshSession(refreshRaw string) {
	var sess session.UserSession
	if err := h.service.db.
		Select("id").
		Where("refresh_token = ? AND provider = ? AND revoked = false", session.HashToken(refreshRaw), "customer").
		First(&sess).Error; err != nil {
		return
	}
	_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonUserLogout)
}

// GetCustomerSessionInfo returns the current customer authentication state.
func (h *Handler) GetCustomerSessionInfo(c *gin.Context) {
	customer, ok := server.OptionalCustomerFromRequest(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"authenticated": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
		"type":          "customer",
		"customer_id":   customer.ID,
		"email":         customer.Email,
		"name":          customer.Name,
	})
}

// RefreshCustomerToken rotates customer refresh credentials without touching owner/staff cookies.
func (h *Handler) RefreshCustomerToken(c *gin.Context) {
	outcome := metrics.RefreshOutcomeInvalid
	defer func() {
		metrics.RecordRefreshOutcome(metrics.RefreshRealmCustomer, outcome)
	}()

	refreshTokenRaw, err := c.Cookie("customer_refresh_token")
	if err != nil || strings.TrimSpace(refreshTokenRaw) == "" {
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeTokenMissing, "Refresh token missing")
		return
	}

	if session.GlobalStore == nil {
		outcome = metrics.RefreshOutcomeStoreError
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeTokenInvalid, "Failed to refresh session")
		return
	}

	refreshHash := session.HashToken(refreshTokenRaw)
	sess, err := session.GlobalStore.ValidateRefreshToken(refreshHash)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		outcome = metrics.RefreshOutcomeStoreError
		log.Printf("[CRM] Failed to validate customer refresh session: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeTokenInvalid, "Failed to refresh session")
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || sess == nil {
		// Reuse detection (see server.RefreshToken for rationale): a just-rotated
		// token replay is a benign concurrent race (reject, don't revoke); a later
		// replay is a stolen-token signal that revokes the session family.
		reused, ferr := session.GlobalStore.FindByPreviousRefreshToken(refreshHash)
		if ferr != nil {
			outcome = metrics.RefreshOutcomeStoreError
			log.Printf("[CRM] Failed to inspect customer refresh-token history: %v", ferr)
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeTokenInvalid, "Failed to refresh session")
			return
		}
		if reused != nil {
			if session.RecentlyRotated(reused, time.Now()) {
				outcome = metrics.RefreshOutcomeRotated
				server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeRefreshRotated, "Refresh token already rotated")
				return
			}
			if revokeErr := session.GlobalStore.RevokeFamilyWithReason(reused, session.RevocationReasonRefreshTokenReuse); revokeErr != nil {
				outcome = metrics.RefreshOutcomeStoreError
				log.Printf("[CRM] Failed to revoke customer refresh-token reuse family: %v", revokeErr)
				server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeTokenInvalid, "Failed to revoke compromised session")
				return
			}
			outcome = metrics.RefreshOutcomeReuse
			server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeSessionUnknown, "Session has been revoked")
			return
		}
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeTokenInvalid, "Invalid or expired refresh token")
		return
	}
	if sess.Provider != "customer" {
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeTokenInvalid, "Invalid customer session")
		return
	}
	if sess.UserID == nil || *sess.UserID == 0 {
		_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeTokenInvalid, "Invalid customer session")
		return
	}

	var customer database.Customer
	lookupErr := h.service.db.Select("id", "email", "is_active").First(&customer, *sess.UserID).Error
	if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		outcome = metrics.RefreshOutcomeStoreError
		log.Printf("[CRM] Failed to load customer %d during refresh: %v", *sess.UserID, lookupErr)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeTokenInvalid, "Failed to refresh session")
		return
	}
	if errors.Is(lookupErr, gorm.ErrRecordNotFound) || !customer.IsActive {
		_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonAccountDisabled)
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeTokenInvalid, "Customer account is inactive")
		return
	}

	newToken, err := server.GenerateCustomerToken(customer.ID, customer.Email, sess.ID)
	if err != nil {
		outcome = metrics.RefreshOutcomeStoreError
		log.Printf("[CRM] Failed to generate customer token: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeTokenInvalid, "Failed to generate token")
		return
	}

	newRefreshToken, err := session.GenerateRefreshToken()
	if err != nil {
		outcome = metrics.RefreshOutcomeStoreError
		log.Printf("[CRM] Failed to generate customer refresh token: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeTokenInvalid, "Failed to refresh session")
		return
	}
	// Atomic compare-and-swap rotation keyed on the old refresh hash (records the
	// old hash as previous for reuse detection). matched=false => a concurrent
	// rotation already advanced the token; this caller lost the race.
	matched, rerr := session.GlobalStore.RotateSession(
		sess.ID, refreshHash, session.HashToken(newRefreshToken), session.HashToken(newToken),
		time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	if rerr != nil {
		outcome = metrics.RefreshOutcomeStoreError
		log.Printf("[CRM] Failed to rotate customer session: %v", rerr)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeTokenInvalid, "Failed to refresh session")
		return
	}
	if !matched {
		outcome = metrics.RefreshOutcomeRotated
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeRefreshRotated, "Refresh token already rotated")
		return
	}

	utils.SetSessionCookie(c, "customer_token", newToken, 15*60)
	utils.SetStrictSessionCookie(c, "customer_refresh_token", newRefreshToken, 7*24*3600)

	outcome = metrics.RefreshOutcomeSuccess
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func extractCustomerCookieToken(c *gin.Context) string {
	if cookie, err := c.Cookie("customer_token"); err == nil && cookie != "" {
		return cookie
	}
	return ""
}

// ConnectToBusinessRequest represents the business connection request
type ConnectToBusinessRequest struct {
	BusinessID     uint `json:"business_id" binding:"required"`
	OptInMarketing bool `json:"opt_in_marketing"`
}

// ConnectCustomerToBusiness handles connecting a customer to a business
func (h *Handler) ConnectCustomerToBusiness(c *gin.Context) {
	customerID, ok := getAuthenticatedCustomerID(c)
	if !ok {
		return
	}

	var req ConnectToBusinessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	connection, err := h.service.ConnectCustomerToBusiness(customerID, req.BusinessID, req.OptInMarketing)
	if err != nil {
		if strings.Contains(err.Error(), "business not found") {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Business not found")
			return
		}
		if strings.Contains(err.Error(), "customer not found") || strings.Contains(err.Error(), "customer account is inactive") {
			server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeNotAuthenticated, "Customer account is inactive")
			return
		}
		if strings.Contains(err.Error(), "business is inactive") {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Business is inactive")
			return
		}
		// FIND-060: product copy only — never pass err.Error() on INTERNAL.
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not complete customer login")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Connected to business successfully",
		"connection": connection,
	})
}

// GetCustomerProfile retrieves customer profile
func (h *Handler) GetCustomerProfile(c *gin.Context) {
	customerID, ok := getAuthenticatedCustomerID(c)
	if !ok {
		return
	}

	customer, err := h.service.GetCustomerByID(customerID)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Customer not found")
		return
	}

	c.JSON(http.StatusOK, customer)
}

// GetCustomerBusinesses retrieves all businesses a customer is connected to
func (h *Handler) GetCustomerBusinesses(c *gin.Context) {
	customerID, ok := getAuthenticatedCustomerID(c)
	if !ok {
		return
	}

	connections, err := h.service.GetCustomerBusinessConnections(customerID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not load business connections")
		return
	}

	c.JSON(http.StatusOK, connections)
}

// UpdateCustomerProfileRequest represents the profile update request
type UpdateCustomerProfileRequest struct {
	Name            *string `json:"name"`
	Phone           *string `json:"phone"`
	Birthday        *string `json:"birthday"`
	ProfileImageURL *string `json:"profile_image_url"`
}

func parseOptionalCustomerBirthday(raw string) (*time.Time, bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, false, nil
	}

	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return &parsed, true, nil
	}

	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return &parsed, true, nil
	}

	return nil, false, fmt.Errorf("birthday must use YYYY-MM-DD format")
}

// UpdateCustomerProfile updates customer profile
func (h *Handler) UpdateCustomerProfile(c *gin.Context) {
	customerID, ok := getAuthenticatedCustomerID(c)
	if !ok {
		return
	}

	var req UpdateCustomerProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	updates := make(map[string]interface{})
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Name is required")
			return
		}
		updates["name"] = trimmed
	}
	if req.Phone != nil {
		updates["phone"] = strings.TrimSpace(*req.Phone)
	}
	if req.Birthday != nil {
		birthday, hasBirthday, err := parseOptionalCustomerBirthday(*req.Birthday)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
			return
		}
		if hasBirthday {
			updates["birthday"] = *birthday
		} else {
			updates["birthday"] = gorm.Expr("NULL")
		}
	}
	if req.ProfileImageURL != nil {
		updates["profile_image_url"] = strings.TrimSpace(*req.ProfileImageURL)
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "Profile updated successfully"})
		return
	}

	if err := h.service.UpdateCustomerProfile(customerID, updates); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not update profile")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated successfully"})
}

// UpdateCustomerPreferencesRequest represents the preferences update request
type UpdateCustomerPreferencesRequest struct {
	PreferredLanguage       *string `json:"preferred_language"`
	PreferredCurrency       *string `json:"preferred_currency"`
	ReceivePromotions       *bool   `json:"receive_promotions"`
	ReceiveNewsletters      *bool   `json:"receive_newsletters"`
	ReceiveBirthdayOffers   *bool   `json:"receive_birthday_offers"`
	ShareDataWithBusinesses *bool   `json:"share_data_with_businesses"`
}

// UpdateCustomerPreferences updates customer preferences
func (h *Handler) UpdateCustomerPreferences(c *gin.Context) {
	customerID, ok := getAuthenticatedCustomerID(c)
	if !ok {
		return
	}

	var req UpdateCustomerPreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	updates := make(map[string]interface{})
	if req.PreferredLanguage != nil {
		updates["preferred_language"] = *req.PreferredLanguage
	}
	if req.PreferredCurrency != nil {
		updates["preferred_currency"] = *req.PreferredCurrency
	}
	if req.ReceivePromotions != nil {
		updates["receive_promotions"] = *req.ReceivePromotions
	}
	if req.ReceiveNewsletters != nil {
		updates["receive_newsletters"] = *req.ReceiveNewsletters
	}
	if req.ReceiveBirthdayOffers != nil {
		updates["receive_birthday_offers"] = *req.ReceiveBirthdayOffers
	}
	if req.ShareDataWithBusinesses != nil {
		updates["share_data_with_businesses"] = *req.ShareDataWithBusinesses
	}

	if err := h.service.UpdateCustomerPreferences(customerID, updates); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not update preferences")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Preferences updated successfully"})
}

// LinkWalletRequest represents the wallet linking request
type LinkWalletRequest struct {
	WalletAddress string `json:"wallet_address" binding:"required"`
}

// LinkWallet links a wallet address to customer account
func (h *Handler) LinkWallet(c *gin.Context) {
	customerID, ok := getAuthenticatedCustomerID(c)
	if !ok {
		return
	}

	var req LinkWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	if err := h.service.LinkWalletToCustomer(customerID, req.WalletAddress); err != nil {
		if errors.Is(err, ErrWalletLinkedElsewhere) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, ErrWalletLinkedElsewhere.Error())
			return
		}
		log.Printf("[CRM] link wallet failed: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not link wallet")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Wallet linked successfully"})
}

// DeleteCustomerAccount deletes a customer account
func (h *Handler) DeleteCustomerAccount(c *gin.Context) {
	customerID, ok := getAuthenticatedCustomerID(c)
	if !ok {
		return
	}

	if err := h.service.DeleteCustomerAccount(customerID); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not delete account")
		return
	}

	if session.GlobalStore != nil {
		if err := session.GlobalStore.RevokeAllForScopedUserWithReason(customerID, session.RevocationReasonAccountDisabled, "customer"); err != nil {
			log.Printf("[CRM] Failed to revoke customer sessions for deleted account %d: %v", customerID, err)
		}
	}
	utils.ClearAllAuthCookies(c)

	c.JSON(http.StatusOK, gin.H{"message": "Account deleted successfully"})
}

// Business CRM Management Handlers

func businessFromRouteID(c *gin.Context) (*database.Business, bool) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Business not found")
		return nil, false
	}
	return business, true
}

// AddCustomerRequest is the body for operator-initiated customer creation.
type AddCustomerRequest struct {
	Email string `json:"email" binding:"required,email"`
	Name  string `json:"name" binding:"required"`
	Phone string `json:"phone"`
}

func isSQLiteLockError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "database is locked")
}

// AddCustomer lets an operator manually add a customer to their business CRM.
// If the customer email already exists, it connects the existing account to the
// business instead of creating a duplicate. New customers, their preferences
// (sharing opt-in = false), and the business connection are created in one
// transaction so any failure rolls back fully.
func (h *Handler) AddCustomer(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	var req AddCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	req.Email = normalizeCustomerEmail(req.Email)
	req.Name = strings.TrimSpace(req.Name)

	var customer database.Customer
	var connection *database.CustomerBusiness
	// created discriminates brand-new global customer vs link of existing email
	// so the operator toast does not claim "added" when we only linked (L5-4).
	var created bool
	// Short retry loop absorbs SQLite write-lock races and unique-email races
	// when two operators add the same new email concurrently.
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		customer = database.Customer{}
		connection = nil
		created = false
		err = h.service.db.Transaction(func(tx *gorm.DB) error {
			findErr := tx.Preload("Preferences", preloadCustomerConsent).
				Where("email = ?", req.Email).First(&customer).Error
			if findErr != nil {
				if !errors.Is(findErr, gorm.ErrRecordNotFound) {
					return findErr
				}
				// New customer path: create customer + default preferences (share=false).
				// On concurrent same-email races the unique email index rejects the
				// second insert — re-load the winner and continue to connection.
				newCustomer := database.Customer{
					Email:    req.Email,
					Name:     req.Name,
					Phone:    req.Phone,
					IsActive: true,
				}
				if createErr := tx.Create(&newCustomer).Error; createErr != nil {
					reload := database.Customer{}
					if reloadErr := tx.Preload("Preferences", preloadCustomerConsent).
						Where("email = ?", req.Email).First(&reload).Error; reloadErr != nil {
						if isSQLiteLockError(createErr) {
							return createErr
						}
						return fmt.Errorf("failed to create customer: %w", createErr)
					}
					customer = reload
					// Race loser: the other writer created the row — not us.
					created = false
				} else {
					customer = newCustomer
					created = true
					if prefErr := createDefaultCustomerPreferences(tx, customer.ID, business.DefaultCurrency); prefErr != nil {
						return fmt.Errorf("failed to create customer preferences: %w", prefErr)
					}
					// Reload preferences inside the transaction for the consent check.
					var prefs database.CustomerPreferences
					if loadErr := tx.Select("id", "customer_id", "share_data_with_businesses").
						Where("customer_id = ?", customer.ID).First(&prefs).Error; loadErr != nil {
						return fmt.Errorf("failed to load customer preferences: %w", loadErr)
					}
					customer.Preferences = &prefs
				}
			}

			conn, connErr := database.EnsureActiveCustomerBusinessConnection(tx, customer.ID, business.ID, false)
			if connErr != nil {
				return fmt.Errorf("failed to connect customer to business: %w", connErr)
			}
			connection = conn
			return nil
		})
		if err == nil {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 15 * time.Millisecond)
	}
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to add customer")
		return
	}

	// Consent chokepoint: the response embeds only a minimal customer view.
	// When the guest has not opted in to cross-business sharing, the operator
	// gets back exactly what they typed — never the existing global profile.
	if customerConsentsToShare(&customer) {
		connection.Customer = database.Customer{
			ID:    customer.ID,
			Email: customer.Email,
			Name:  customer.Name,
			Phone: customer.Phone,
		}
	} else {
		connection.Customer = database.Customer{
			ID:    customer.ID,
			Email: req.Email,
			Name:  req.Name,
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":    "Customer added successfully",
		"created":    created,
		"connection": connection,
	})
}

// GetBusinessCustomers retrieves all customers for a business
func (h *Handler) GetBusinessCustomers(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, pageSize = normalizeBusinessCustomersPagination(page, pageSize)
	search := c.Query("search")
	// "All" is the UI sentinel for no tier filter; treat it as empty.
	tier := c.Query("tier")
	if tier == "All" {
		tier = ""
	}

	// Optional server-side sort (fix 6) and segment filter (fix 7). All absent =
	// legacy behavior (BE-first). An unknown sort column is ignored by the
	// service's whitelist; a "regular"/unknown segment applies no predicate.
	sortBy := c.Query("sort_by")
	sortDir := c.Query("sort_dir")
	segment := c.Query("segment")

	var customers []database.CustomerBusiness
	var total int64
	var err error
	switch {
	case segment != "":
		// A segment drilldown scopes the whole base to one behavioral band; the
		// tier chip / free-text search do not combine with it in the UI.
		customers, total, err = h.service.GetBusinessCustomersSegment(business.ID, page, pageSize, segment)
	case sortBy != "":
		customers, total, err = h.service.GetBusinessCustomersSorted(business.ID, page, pageSize, search, tier, sortBy, sortDir)
	default:
		customers, total, err = h.service.GetBusinessCustomers(business.ID, page, pageSize, search, tier)
	}
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not list customers")
		return
	}

	// Aggregates for the dashboard cards (audit L6 #3 / L5-2): not the current
	// page, but the same search+tier scope as the list so stat cards and table
	// totals agree when the operator filters. topTier stays "" → "Gold".
	summary, err := h.service.GetBusinessCustomerSummary(business.ID, search, tier, "")
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not load customer summary")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"customers":   customers,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
		"summary":     summary,
	})
}

// GetCustomerDetails retrieves detailed information about a customer
func (h *Handler) GetCustomerDetails(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	customerBusinessID, err := strconv.ParseUint(c.Param("customerBusinessId"), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid customer business ID")
		return
	}

	customerBusiness, err := h.service.GetCustomerBusinessDetails(uint(customerBusinessID))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Customer not found")
		return
	}

	// Verify the customer belongs to this business
	if customerBusiness.BusinessID != business.ID {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied")
		return
	}

	// Get stats
	stats, _ := h.service.GetCustomerStats(uint(customerBusinessID))

	c.JSON(http.StatusOK, gin.H{
		"customer": customerBusiness,
		"stats":    stats,
	})
}

// UnlinkCustomer soft-unlinks a customer from this business (IsActive=false).
// Not a global hard delete — customer rows are cross-business (L5-9).
func (h *Handler) UnlinkCustomer(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	customerBusinessID, err := strconv.ParseUint(c.Param("customerBusinessId"), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid customer business ID")
		return
	}

	if err := h.service.UnlinkCustomerFromBusiness(business.ID, uint(customerBusinessID)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Customer link not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not unlink customer")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Customer removed from this business", "unlinked": true})
}

// UpdateCustomerNotesRequest represents the notes update request
type UpdateCustomerNotesRequest struct {
	Notes string `json:"notes"`
}

// UpdateCustomerNotes updates business-specific notes for a customer
func (h *Handler) UpdateCustomerNotes(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	customerBusinessID, err := strconv.ParseUint(c.Param("customerBusinessId"), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid customer business ID")
		return
	}

	var req UpdateCustomerNotesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	if _, err := h.service.GetCustomerBusinessDetailsForBusiness(business.ID, uint(customerBusinessID)); err != nil {
		if strings.Contains(err.Error(), "does not belong to this business") {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied")
			return
		}
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Customer not found")
		return
	}

	if err := h.service.UpdateCustomerBusinessNotes(uint(customerBusinessID), req.Notes); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not update customer notes")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notes updated successfully"})
}

// UpdateCustomerAllergiesRequest represents the allergies update request.
type UpdateCustomerAllergiesRequest struct {
	Allergies string `json:"allergies"`
}

// UpdateCustomerAllergies updates business-specific allergy notes for a customer.
func (h *Handler) UpdateCustomerAllergies(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	customerBusinessID, err := strconv.ParseUint(c.Param("customerBusinessId"), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid customer business ID")
		return
	}

	var req UpdateCustomerAllergiesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	if _, err := h.service.GetCustomerBusinessDetailsForBusiness(business.ID, uint(customerBusinessID)); err != nil {
		if strings.Contains(err.Error(), "does not belong to this business") {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied")
			return
		}
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Customer not found")
		return
	}

	if err := h.service.UpdateCustomerBusinessAllergies(uint(customerBusinessID), req.Allergies); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not update customer allergies")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Allergies updated successfully"})
}

// AdjustCustomerLoyaltyPointsRequest sets an absolute point balance.
type AdjustCustomerLoyaltyPointsRequest struct {
	LoyaltyPoints int    `json:"loyalty_points"`
	Reason        string `json:"reason"`
}

// AdjustCustomerLoyaltyPoints lets operators comp or correct a customer's
// point balance (service recovery). Balance is absolute and may not be negative.
func (h *Handler) AdjustCustomerLoyaltyPoints(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	customerBusinessID, err := strconv.ParseUint(c.Param("customerBusinessId"), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid customer business ID")
		return
	}

	var req AdjustCustomerLoyaltyPointsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if req.LoyaltyPoints < 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "loyalty_points cannot be negative")
		return
	}
	// Cap absurd values — same order of magnitude as earn-rate ceilings.
	if req.LoyaltyPoints > 10_000_000 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "loyalty_points is unreasonably large")
		return
	}

	if _, err := h.service.GetCustomerBusinessDetailsForBusiness(business.ID, uint(customerBusinessID)); err != nil {
		if strings.Contains(err.Error(), "does not belong to this business") {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied")
			return
		}
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Customer not found")
		return
	}

	if err := h.service.AdjustCustomerBusinessLoyaltyPoints(uint(customerBusinessID), req.LoyaltyPoints); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not adjust loyalty points")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Loyalty points updated",
		"loyalty_points": req.LoyaltyPoints,
		"reason":         req.Reason,
	})
}

// UpdateCustomerTagsRequest represents the tags update request
type UpdateCustomerTagsRequest struct {
	Tags string `json:"tags"`
}

// UpdateCustomerTags updates tags for a customer
func (h *Handler) UpdateCustomerTags(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	customerBusinessID, err := strconv.ParseUint(c.Param("customerBusinessId"), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid customer business ID")
		return
	}

	var req UpdateCustomerTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	if _, err := h.service.GetCustomerBusinessDetailsForBusiness(business.ID, uint(customerBusinessID)); err != nil {
		if strings.Contains(err.Error(), "does not belong to this business") {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied")
			return
		}
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Customer not found")
		return
	}

	if err := h.service.UpdateCustomerBusinessTags(uint(customerBusinessID), req.Tags); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not update customer tags")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tags updated successfully"})
}

// ExportCustomers exports customer data as CSV
func (h *Handler) ExportCustomers(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	// Set headers for CSV download before streaming rows. The csv writer
	// buffers, so nothing reaches the client until the buffer fills or the
	// explicit Flush below. A failure before any byte is committed (e.g. the
	// first page query) becomes a 500 instead of a 200 with a header-only file;
	// once bytes are committed, later failures can only be logged.
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=customers_%d_%s.csv", business.ID, time.Now().Format("20060102")))

	writer := csv.NewWriter(c.Writer)

	// Write header (localized via ?lang= for operator locale)
	header := crmCSVHeaders(c.Query("lang"))
	if err := writer.Write(header); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to write CSV header")
		return
	}

	// Every total_spent in this file is denominated in the venue's own currency
	// (CRM never converts), so it is one scalar for the whole export — resolved
	// from the already-loaded business row, no extra query (#926).
	currency := business.ResolvedCurrency()

	err := h.service.ExportBusinessCustomers(business.ID, func(customer CustomerExportRow) error {
		// Free-text fields (email/name/phone/notes/tags, operator-named tiers)
		// originate from customer/operator input and land in the operator's
		// spreadsheet on export, so they are sanitized against CSV formula
		// injection (CWE-1236). Code-formatted numerics/dates are left as-is.
		row := []string{
			utils.SanitizeCSVField(customer.Email),
			utils.SanitizeCSVField(customer.Name),
			utils.SanitizeCSVField(customer.Phone),
			fmt.Sprintf("%v", customer.LoyaltyPoints),
			utils.SanitizeCSVField(customer.LoyaltyTier),
			fmt.Sprintf("%.2f", customer.TotalSpent),
			currency,
			fmt.Sprintf("%v", customer.VisitCount),
			formatTime(customer.LastVisitAt),
			formatTime(customer.FirstVisitAt),
			fmt.Sprintf("%v", customer.OptInMarketing),
			fmt.Sprintf("%v", customer.OptInEmail),
			fmt.Sprintf("%v", customer.OptInSMS),
			utils.SanitizeCSVField(customer.Notes),
			utils.SanitizeCSVField(customer.Tags),
		}
		return writer.Write(row)
	})
	if err != nil {
		log.Printf("[CRM] customer export failed for business %d: %v", business.ID, err)
		if !c.Writer.Written() {
			// gin keeps an already-set Content-Type, so clear the CSV headers
			// before rendering JSON.
			c.Writer.Header().Del("Content-Type")
			c.Writer.Header().Del("Content-Disposition")
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not export customers")
		}
		return
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Printf("[CRM] customer export failed for business %d: %v", business.ID, err)
	}
}

// Helper function to format time
func formatTime(t interface{}) string {
	if t == nil {
		return ""
	}
	if timeVal, ok := t.(*time.Time); ok && timeVal != nil {
		return timeVal.Format("2006-01-02 15:04:05")
	}
	if timeVal, ok := t.(time.Time); ok {
		return timeVal.Format("2006-01-02 15:04:05")
	}
	return ""
}

// GetCRMStatus returns the current CRM enabled status for a business
func (h *Handler) GetCRMStatus(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled": business.CRMEnabled,
	})
}

// ToggleCRM enables or disables CRM for a business
func (h *Handler) ToggleCRM(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// A suspended or closed business cannot toggle CRM.
	if server.RespondIfBusinessLocked(c, business) {
		return
	}

	business.CRMEnabled = req.Enabled
	if err := h.service.db.Save(business).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to update CRM status")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "CRM status updated successfully",
		"enabled": req.Enabled,
	})
}

// GetSegments returns computed segment counts for the business's active
// customers. Segments are mutually exclusive with priority:
// lapsed > atRisk > vip > new; remaining customers are "regular" (not emitted).
func (h *Handler) GetSegments(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}

	// Compute the four mutually-exclusive segment counts in a single SQL
	// aggregate instead of hydrating every row and looping twice in Go
	// (UB-01). Day-boundary thresholds are converted to absolute timestamp
	// cutoffs in Go so the SQL stays portable (no DB-specific date math) and
	// matches the original integer-truncated `int(now.Sub(...)/day)` semantics
	// exactly: `daysSince > N` ⟺ the event is at least (N+1) days in the past.
	// UTC keeps the bound cutoffs comparable to the UTC timestamps the DB stores
	// (Postgres compares timestamptz semantically; SQLite compares the serialized
	// text, which only sorts correctly when both sides are the same UTC layout).
	now := time.Now().UTC()
	day := 24 * time.Hour
	// daysSinceVisit > 90  ⟺ last_visit_at <= now-91d
	visitLapsedCutoff := now.Add(-91 * day)
	// daysSinceVisit > 30  ⟺ last_visit_at <= now-31d
	visitAtRiskCutoff := now.Add(-31 * day)
	// daysSinceJoin > 90   ⟺ first_visit_at <= now-91d
	joinLapsedCutoff := now.Add(-91 * day)
	// daysSinceJoin <= 30  ⟺ first_visit_at > now-31d
	joinNewCutoff := now.Add(-31 * day)

	// avgSpend over the same (business, active) scope, used by the VIP band.
	// Inlined as a scalar subquery so the whole computation is one query
	// (COALESCE keeps it 0 for an empty base, matching the Go `avgSpend := 0`).
	const avgSubquery = "(SELECT COALESCE(AVG(total_spent), 0) FROM customer_businesses " +
		"WHERE business_id = ? AND is_active = ?)"

	// Nested CASE encodes the original priority lapsed > atRisk > vip > new in
	// portable SQL (CASE WHEN, no FILTER, no DB-specific date functions). A
	// customer is counted in the first band it matches; the rest are "regular"
	// and not emitted, exactly as the Go switch did. Parameter order below must
	// match the `?` placeholders here.
	segmentSums := "SUM(CASE " +
		// lapsed: visit older than 90d, OR no visit but joined over 90d ago.
		"WHEN (last_visit_at IS NOT NULL AND last_visit_at <= ?) " +
		"  OR (last_visit_at IS NULL AND first_visit_at <= ?) THEN 1 ELSE 0 END) AS lapsed, " +
		// atRisk: visit in the (30d, 90d] window (and not already lapsed).
		"SUM(CASE " +
		"WHEN (last_visit_at IS NOT NULL AND last_visit_at <= ?) " +
		"  OR (last_visit_at IS NULL AND first_visit_at <= ?) THEN 0 " +
		"WHEN last_visit_at IS NOT NULL AND last_visit_at <= ? THEN 1 ELSE 0 END) AS at_risk, " +
		// vip: >=5 visits and above-average lifetime spend (and not lapsed/atRisk).
		"SUM(CASE " +
		"WHEN (last_visit_at IS NOT NULL AND last_visit_at <= ?) " +
		"  OR (last_visit_at IS NULL AND first_visit_at <= ?) THEN 0 " +
		"WHEN last_visit_at IS NOT NULL AND last_visit_at <= ? THEN 0 " +
		"WHEN visit_count >= 5 AND total_spent > " + avgSubquery + " THEN 1 ELSE 0 END) AS vip, " +
		// new: joined within the last 30 days (and not lapsed/atRisk/vip).
		"SUM(CASE " +
		"WHEN (last_visit_at IS NOT NULL AND last_visit_at <= ?) " +
		"  OR (last_visit_at IS NULL AND first_visit_at <= ?) THEN 0 " +
		"WHEN last_visit_at IS NOT NULL AND last_visit_at <= ? THEN 0 " +
		"WHEN visit_count >= 5 AND total_spent > " + avgSubquery + " THEN 0 " +
		"WHEN first_visit_at > ? THEN 1 ELSE 0 END) AS new"

	args := []interface{}{
		// lapsed band
		visitLapsedCutoff, joinLapsedCutoff,
		// atRisk band: lapsed guard (2) then atRisk cutoff
		visitLapsedCutoff, joinLapsedCutoff, visitAtRiskCutoff,
		// vip band: lapsed guard (2), atRisk guard (1), then avg subquery (business, active)
		visitLapsedCutoff, joinLapsedCutoff, visitAtRiskCutoff, business.ID, true,
		// new band: lapsed guard (2), atRisk guard (1), vip guard avg subquery (business, active), then new cutoff
		visitLapsedCutoff, joinLapsedCutoff, visitAtRiskCutoff, business.ID, true, joinNewCutoff,
	}

	row := struct {
		Lapsed int
		AtRisk int
		Vip    int
		New    int
	}{}

	if err := h.service.db.Model(&database.CustomerBusiness{}).
		Select(segmentSums, args...).
		Where("business_id = ? AND is_active = ?", business.ID, true).
		Scan(&row).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not load customer segments")
		return
	}

	counts := map[string]int{
		"lapsed": row.Lapsed,
		"atRisk": row.AtRisk,
		"vip":    row.Vip,
		"new":    row.New,
	}

	c.JSON(http.StatusOK, counts)
}

// GetLoyalty returns the business's loyalty program plus its tiers. Missing
// programs return a disabled zero-point default so the dashboard renders an
// editor for opt-in.
func (h *Handler) GetLoyalty(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}
	program, err := h.service.loadLoyaltyProgram(nil, business.ID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not load loyalty program")
		return
	}
	response := gin.H{"program": program, "tiers": program.Tiers, "valid": true}
	if validationErr := validateTierLadder(program.Tiers); validationErr != nil {
		// Existing drift is surfaced without mutating or silently deleting rows.
		// The dashboard can show the ladder for repair, but must not preview/save it
		// as a valid program until the operator fixes the returned data.
		response["valid"] = false
		response["validation_error"] = validationErr.Error()
	}
	c.JSON(http.StatusOK, response)
}

type putLoyaltyRequest struct {
	Enabled                   bool                   `json:"enabled"`
	PointsPerDollar           float64                `json:"points_per_dollar"`
	RedemptionPointsPerDollar float64                `json:"redemption_points_per_dollar"`
	Tiers                     []database.LoyaltyTier `json:"tiers"`
}

func validatePointsPerDollar(rate float64) error {
	if rate < 0 || rate > 100 {
		return errors.New("points_per_dollar must be between 0 and 100")
	}
	return nil
}

func validateRedemptionPointsPerDollar(rate float64) error {
	// Redeem rates are typically much higher than earn rates (e.g. 100 pts = $1)
	// so the ceiling is higher than the earn-rate validator.
	if rate < 0 || rate > 10000 {
		return errors.New("redemption_points_per_dollar must be between 0 and 10000")
	}
	return nil
}

// PutLoyalty upserts the loyalty program and replaces its tier ladder in a
// single transaction. Tier IDs are zeroed before insert so callers can post
// either the existing list with edits or a freshly minted set.
func (h *Handler) PutLoyalty(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}
	var req putLoyaltyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if err := validatePointsPerDollar(req.PointsPerDollar); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	if err := validateRedemptionPointsPerDollar(req.RedemptionPointsPerDollar); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	// An enabled program must have positive earn AND redeem rates. Redemption
	// refuses to run without a configured redeem rate (no silent default), and
	// earn=redeem (e.g. 1.25/1.25) is ~100% cashback — operators configure both
	// so dinner venues can set a sane fraction (earn 1, redeem 100).
	if req.Enabled && req.PointsPerDollar <= 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Set a points-per-dollar earn rate before enabling loyalty")
		return
	}
	if req.Enabled && req.RedemptionPointsPerDollar <= 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Set a points-per-dollar redemption rate before enabling loyalty")
		return
	}
	canonicalTiers := canonicalizeTierSortOrders(req.Tiers)
	// Authoritative tier-ladder guard: reject duplicate names / overlapping
	// thresholds before touching the DB so the client can never persist an
	// ambiguous ladder (mirrors the FE TierEditor rule).
	if err := validateTierLadder(canonicalTiers); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	err := h.service.db.Transaction(func(tx *gorm.DB) error {
		// Settlement locks customer_businesses before the loyalty program. Take
		// the same locks in the same order here to serialize tier recomputation
		// without introducing a save/settlement deadlock.
		var rows []database.CustomerBusiness
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Model(&database.CustomerBusiness{}).
			Select("id", "total_spent", "loyalty_tier").
			Where("business_id = ? AND is_active = ?", business.ID, true).
			Order("id ASC").
			Find(&rows).Error; err != nil {
			return err
		}

		var program database.LoyaltyProgram
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Tiers", func(db *gorm.DB) *gorm.DB {
				return db.Order("sort_order ASC")
			}).Where("business_id = ?", business.ID).First(&program).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			program = database.LoyaltyProgram{BusinessID: business.ID}
		}
		program.Enabled = req.Enabled
		program.PointsPerDollar = req.PointsPerDollar
		program.RedemptionPointsPerDollar = req.RedemptionPointsPerDollar
		if err := tx.Omit(clause.Associations).Save(&program).Error; err != nil {
			return err
		}
		if err := tx.Where("loyalty_program_id = ?", program.ID).Delete(&database.LoyaltyTier{}).Error; err != nil {
			return err
		}
		for _, t := range canonicalTiers {
			t.ID = 0
			t.LoyaltyProgramID = program.ID
			if err := tx.Create(&t).Error; err != nil {
				return err
			}
		}

		// Recompute loyalty_tier for all active customers of this business.
		// Project only the columns the recompute needs (id, total_spent, current
		// tier) instead of hydrating full rows, then bucket the rows that change
		// by their target tier and issue ONE batched UPDATE per distinct tier
		// instead of a per-row UPDATE loop (UB-03 / N1-02). computeTier is a pure
		// function of total_spent, so each row lands in exactly one bucket
		// (disjoint), and the lowest/no-tier case ("") is preserved: a customer
		// below every threshold gets loyalty_tier reset to "", matching the
		// original loop. Only changed rows are touched, so updated_at stays
		// stable for unchanged customers exactly as before.
		program.Tiers = canonicalTiers
		idsByTier := make(map[string][]uint)
		for _, r := range rows {
			tier := computeTier(lifetimeSpentCents(r.TotalSpent), &program)
			if tier != r.LoyaltyTier {
				idsByTier[tier] = append(idsByTier[tier], r.ID)
			}
		}
		for tier, ids := range idsByTier {
			if err := tx.Model(&database.CustomerBusiness{}).
				Where("id IN ?", ids).
				Update("loyalty_tier", tier).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not save loyalty program")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type previewLoyaltyRequest struct {
	PointsPerDollar float64                `json:"points_per_dollar"`
	Tiers           []database.LoyaltyTier `json:"tiers"`
}

// PreviewLoyalty runs the proposed tier ladder over the business's existing
// active customers and returns the count per tier so operators can see the
// distribution before saving.
func (h *Handler) PreviewLoyalty(c *gin.Context) {
	business, ok := businessFromRouteID(c)
	if !ok {
		return
	}
	var req previewLoyaltyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if err := validatePointsPerDollar(req.PointsPerDollar); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	if err := validateTierLadder(req.Tiers); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	// Only total_spent is needed to bucket the preview distribution, so project
	// that single column instead of hydrating full CustomerBusiness rows (UB-02).
	var spends []float64
	if err := h.service.db.Model(&database.CustomerBusiness{}).
		Where("business_id = ? AND is_active = ?", business.ID, true).
		Pluck("total_spent", &spends).Error; err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not preview loyalty tiers")
		return
	}
	fakeProgram := &database.LoyaltyProgram{Enabled: true, PointsPerDollar: req.PointsPerDollar, Tiers: req.Tiers}
	dist := map[string]int{}
	for _, totalSpent := range spends {
		tier := computeTier(lifetimeSpentCents(totalSpent), fakeProgram)
		if tier == "" {
			tier = "(none)"
		}
		dist[tier]++
	}
	c.JSON(http.StatusOK, gin.H{"total_customers": len(spends), "tier_distribution": dist})
}

// crmCSVHeaders returns the localized column headers. "Currency"/"Moneda" sits
// immediately after the money column, the same placement the accounting and
// payment-history exports use — Total Spent alone left an ARS carta exporting
// bare numbers an operator had to read as dollars (#926).
func crmCSVHeaders(lang string) []string {
	if utils.NormalizeOperatorLang(lang) == "es" {
		return []string{
			"Correo", "Nombre", "Teléfono", "Puntos de lealtad", "Nivel de lealtad",
			"Total gastado", "Moneda", "Visitas", "Última visita", "Primera visita",
			"Opt-in marketing", "Opt-in email", "Opt-in SMS", "Notas", "Etiquetas",
		}
	}
	return []string{
		"Email", "Name", "Phone", "Loyalty Points", "Loyalty Tier", "Total Spent",
		"Currency", "Visit Count", "Last Visit", "First Visit", "Opt-In Marketing",
		"Opt-In Email", "Opt-In SMS", "Notes", "Tags",
	}
}
