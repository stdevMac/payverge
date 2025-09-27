package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"payverge/internal/database"
	"payverge/internal/emails"

	"github.com/gin-gonic/gin"
)

// Staff invitation request
type InviteStaffRequest struct {
	Email string             `json:"email" binding:"required,email"`
	Name  string             `json:"name" binding:"required"`
	Role  database.StaffRole `json:"role" binding:"required"`
}

// Staff login request
type StaffLoginRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// Staff login code verification request
type VerifyLoginCodeRequest struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required"`
}

// Accept invitation request
type AcceptInvitationRequest struct {
	Token string `json:"token" binding:"required"`
	Name  string `json:"name" binding:"required"`
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// generateLoginCode generates a 6-digit login code
func generateLoginCode() string {
	bytes := make([]byte, 3)
	rand.Read(bytes)
	code := fmt.Sprintf("%06d", int(bytes[0])<<16|int(bytes[1])<<8|int(bytes[2]))
	return code[:6]
}

// generateInvitationToken generates a secure random token for staff invitations
func generateInvitationToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return fmt.Sprintf("%x", bytes)
}

// InviteStaff handles staff invitation by business owners
func InviteStaff(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Get owner address from context (set by auth middleware)
	ownerAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Owner address not found"})
		return
	}

	var req InviteStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Verify business ownership
	business, err := db.BusinessService.GetByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != ownerAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to manage this business"})
		return
	}

	// Check if staff member already exists
	existingStaff, _ := db.StaffService.GetByEmail(req.Email)
	if existingStaff != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Staff member with this email already exists"})
		return
	}

	// Check for pending invitation
	pendingInvitations, _ := db.StaffInvitationService.GetByBusinessID(uint(businessID))
	for _, invitation := range pendingInvitations {
		if invitation.Email == req.Email && invitation.Status == database.InvitationStatusPending {
			c.JSON(http.StatusConflict, gin.H{"error": "Invitation already sent to this email"})
			return
		}
	}

	// Generate secure invitation token
	token, err := generateSecureToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate invitation token"})
		return
	}

	// Create invitation
	invitation := &database.StaffInvitation{
		BusinessID: uint(businessID),
		Email:      strings.ToLower(req.Email),
		Name:       req.Name,
		Role:       req.Role,
		Token:      token,
		Status:     database.InvitationStatusPending,
		InvitedBy:  ownerAddress.(string),
		ExpiresAt:  time.Now().Add(7 * 24 * time.Hour), // 7 days
	}

	if err := db.StaffInvitationService.Create(invitation); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invitation"})
		return
	}

	// Send invitation email
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	invitationURL := fmt.Sprintf("%s/staff/accept-invitation?token=%s", baseURL, token)

	templateData := map[string]interface{}{
		"business_name":  business.Name,
		"staff_name":     req.Name,
		"role":           string(req.Role),
		"invitation_url": invitationURL,
		"owner_address":  ownerAddress.(string),
		"expires_days":   "7",
	}

	if err := emails.EmailServerInstance.SendTransactionalEmail(
		[]string{req.Email},
		emails.TemplateStaffInvitation,
		templateData,
	); err != nil {
		// Log error but don't fail the request
		fmt.Printf("Failed to send invitation email: %v\n", err)
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":       "Staff invitation sent successfully",
		"invitation_id": invitation.ID,
		"expires_at":    invitation.ExpiresAt,
	})
}

// ResendInvitation resends an existing staff invitation
func ResendInvitation(c *gin.Context) {
	businessIDStr := c.Param("id")
	invitationIDStr := c.Param("invitationId")
	
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}
	
	invitationID, err := strconv.ParseUint(invitationIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invitation ID"})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Get owner address from context (set by auth middleware)
	ownerAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Owner address not found"})
		return
	}

	// Verify business ownership
	business, err := db.BusinessService.GetByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != ownerAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to manage this business"})
		return
	}

	// Get the existing invitation
	invitation, err := db.StaffInvitationService.GetByID(uint(invitationID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}

	// Verify invitation belongs to this business
	if invitation.BusinessID != uint(businessID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invitation does not belong to this business"})
		return
	}

	// Check if invitation is still pending (not expired or already accepted)
	if time.Now().After(invitation.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot resend expired invitation"})
		return
	}

	// Generate new token and extend expiry
	newToken := generateInvitationToken()
	newExpiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days from now

	// Update invitation with new token and expiry
	invitation.Token = newToken
	invitation.ExpiresAt = newExpiresAt
	
	if err := db.StaffInvitationService.Update(invitation); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update invitation"})
		return
	}

	// Send invitation email
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	invitationURL := fmt.Sprintf("%s/staff/accept-invitation?token=%s", baseURL, newToken)

	templateData := map[string]interface{}{
		"business_name":  business.Name,
		"staff_name":     invitation.Name,
		"role":           string(invitation.Role),
		"invitation_url": invitationURL,
		"owner_address":  ownerAddress.(string),
		"expires_days":   "7",
	}

	if err := emails.EmailServerInstance.SendTransactionalEmail(
		[]string{invitation.Email},
		emails.TemplateStaffInvitation,
		templateData,
	); err != nil {
		// Log error but don't fail the request
		fmt.Printf("Failed to send invitation email: %v\n", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Staff invitation resent successfully",
		"invitation_id": invitation.ID,
		"expires_at":    invitation.ExpiresAt,
	})
}

// AcceptInvitation handles staff accepting invitations
func AcceptInvitation(c *gin.Context) {
	var req AcceptInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Find invitation by token
	invitation, err := db.StaffInvitationService.GetByToken(req.Token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invalid or expired invitation"})
		return
	}

	// Check if invitation is expired
	if time.Now().After(invitation.ExpiresAt) {
		db.StaffInvitationService.UpdateStatus(invitation.ID, database.InvitationStatusExpired)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invitation has expired"})
		return
	}

	// Create staff member
	staff := &database.Staff{
		BusinessID: invitation.BusinessID,
		Email:      invitation.Email,
		Name:       req.Name,
		Role:       invitation.Role,
		IsActive:   true,
		InvitedBy:  invitation.InvitedBy,
	}

	if err := db.StaffService.Create(staff); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create staff account"})
		return
	}

	// Mark invitation as accepted
	db.StaffInvitationService.UpdateStatus(invitation.ID, database.InvitationStatusAccepted)

	c.JSON(http.StatusCreated, gin.H{
		"message": "Staff account created successfully",
		"staff_id": staff.ID,
		"business_id": staff.BusinessID,
		"role": staff.Role,
	})
}

// RequestLoginCode handles staff login code requests
func RequestLoginCode(c *gin.Context) {
	var req StaffLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Find staff member by email
	staff, err := db.StaffService.GetByEmail(strings.ToLower(req.Email))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Staff member not found"})
		return
	}

	if !staff.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "Staff account is inactive"})
		return
	}

	// Generate login code
	code := generateLoginCode()

	// Create login code record
	loginCode := &database.StaffLoginCode{
		StaffID:   staff.ID,
		Code:      code,
		ExpiresAt: time.Now().Add(10 * time.Minute), // 10 minutes
		Used:      false,
	}

	if err := db.StaffLoginCodeService.Create(loginCode); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate login code"})
		return
	}

	// Get business info for email
	business, _ := db.BusinessService.GetByID(staff.BusinessID)
	businessName := "Your Business"
	if business != nil {
		businessName = business.Name
	}

	// Send login code email
	templateData := map[string]interface{}{
		"staff_name":      staff.Name,
		"business_name":   businessName,
		"login_code":      code,
		"expires_minutes": "10",
	}

	if err := emails.EmailServerInstance.SendTransactionalEmail(
		[]string{staff.Email},
		emails.TemplateStaffLoginCode,
		templateData,
	); err != nil {
		fmt.Printf("Failed to send login code email: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send login code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Login code sent to your email",
		"expires_in_minutes": 10,
	})
}

// VerifyLoginCode handles staff login code verification
func VerifyLoginCode(c *gin.Context) {
	var req VerifyLoginCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Find staff member by email
	staff, err := db.StaffService.GetByEmail(strings.ToLower(req.Email))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Staff member not found"})
		return
	}

	// Find and verify login code
	loginCode, err := db.StaffLoginCodeService.GetByCode(req.Code)
	if err != nil || loginCode.StaffID != staff.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired login code"})
		return
	}

	// Mark code as used
	db.StaffLoginCodeService.MarkAsUsed(loginCode.ID)

	// Update last login time
	db.StaffService.UpdateLastLogin(staff.ID)

	// Generate JWT token for staff (placeholder for now)
	token := "staff_jwt_token_placeholder"

	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"token": token,
		"staff": gin.H{
			"id": staff.ID,
			"name": staff.Name,
			"email": staff.Email,
			"role": staff.Role,
			"business_id": staff.BusinessID,
		},
	})
}

// GetBusinessStaff returns all staff members for a business
func GetBusinessStaff(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Get owner address from context
	ownerAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Owner address not found"})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Verify business ownership
	business, err := db.BusinessService.GetByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != ownerAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to view this business staff"})
		return
	}

	// Get staff members
	staff, err := db.StaffService.GetByBusinessID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve staff"})
		return
	}

	// Get pending invitations (only pending status)
	allInvitations, err := db.StaffInvitationService.GetByBusinessID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve invitations"})
		return
	}
	
	// Filter only pending invitations
	var pendingInvitations []database.StaffInvitation
	for _, invitation := range allInvitations {
		if invitation.Status == database.InvitationStatusPending {
			pendingInvitations = append(pendingInvitations, invitation)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"staff": staff,
		"pending_invitations": pendingInvitations,
	})
}

// RemoveStaff removes a staff member
func RemoveStaff(c *gin.Context) {
	businessIDStr := c.Param("id")
	staffIDStr := c.Param("staffId")
	
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}
	
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid staff ID"})
		return
	}

	// Get owner address from context
	ownerAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Owner address not found"})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Verify business ownership
	business, err := db.BusinessService.GetByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != ownerAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to manage this business"})
		return
	}

	// Verify staff belongs to business
	staff, err := db.StaffService.GetByID(uint(staffID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Staff member not found"})
		return
	}

	if staff.BusinessID != uint(businessID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Staff member does not belong to this business"})
		return
	}

	// Soft delete staff member
	if err := db.StaffService.SoftDelete(uint(staffID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove staff member"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Staff member removed successfully"})
}

// UpdateStaffRole updates a staff member's role
func UpdateStaffRole(c *gin.Context) {
	businessIDStr := c.Param("id")
	staffIDStr := c.Param("staffId")
	
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}
	
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid staff ID"})
		return
	}

	var req struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate role
	validRoles := []string{"manager", "server", "host", "kitchen"}
	roleValid := false
	for _, validRole := range validRoles {
		if req.Role == validRole {
			roleValid = true
			break
		}
	}
	if !roleValid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}

	// Get owner address from context
	ownerAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Owner address not found"})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Verify business ownership
	business, err := db.BusinessService.GetByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != ownerAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to manage this business"})
		return
	}

	// Get staff member
	staff, err := db.StaffService.GetByID(uint(staffID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Staff member not found"})
		return
	}

	// Verify staff belongs to this business
	if staff.BusinessID != uint(businessID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Staff member does not belong to this business"})
		return
	}

	// Update staff role
	staff.Role = database.StaffRole(req.Role)
	if err := db.StaffService.Update(staff); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update staff role"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Staff role updated successfully",
		"staff": staff,
	})
}
