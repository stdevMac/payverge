package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Staff invitation request
type InviteStaffRequest struct {
	Email string             `json:"email" binding:"required,email"`
	Name  string             `json:"name" binding:"required,max=120"`
	Role  database.StaffRole `json:"role" binding:"required"`
}

// Staff login request
type StaffLoginRequest struct {
	Email      string `json:"email" binding:"required,email"`
	BusinessID *uint  `json:"business_id,omitempty"`
}

// Staff login code verification request
type VerifyLoginCodeRequest struct {
	Email          string `json:"email,omitempty"`
	Code           string `json:"code,omitempty"`
	BusinessID     *uint  `json:"business_id,omitempty"`
	SelectionToken string `json:"selection_token,omitempty"`
}

// Accept invitation request
type AcceptInvitationRequest struct {
	Token string `json:"token" binding:"required"`
	Name  string `json:"name" binding:"required,max=120"`
}

func normalizeStaffEmailAddress(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// reservedStaffLoginTLDs are structurally undeliverable / special-use suffixes.
// Rejecting them with 400 is not an account oracle: the answer depends only on
// the address shape, never on whether a staff row exists.
var reservedStaffLoginTLDs = map[string]struct{}{
	"local":       {},
	"localhost":   {},
	"invalid":     {},
	"test":        {},
	"internal":    {},
	"lan":         {},
	"localdomain": {},
}

func staffLoginEmailUndeliverable(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 1 || at == len(email)-1 {
		return true
	}
	domain := strings.ToLower(email[at+1:])
	tld := domain
	if i := strings.LastIndex(domain, "."); i >= 0 {
		tld = domain[i+1:]
	}
	_, reserved := reservedStaffLoginTLDs[tld]
	return reserved
}

const loginCodeMaybeSentMessage = "If this address has an account, a login code was sent"

// staffRoleDisplayName localizes role tokens for invite/login emails by family.
func staffRoleDisplayName(role, language string) string {
	family := strings.ToLower(strings.TrimSpace(language))
	family = strings.ReplaceAll(family, "_", "-")
	if family == "es-ar" || strings.HasPrefix(family, "es-ar") {
		family = "es_ar"
	} else if family == "es" || strings.HasPrefix(family, "es") {
		family = "es"
	} else {
		family = "en"
	}
	names := map[string]map[string]string{
		"en": {
			"kitchen": "Kitchen",
			"server":  "Server",
			"host":    "Host",
			"manager": "Manager",
		},
		"es": {
			"kitchen": "Cocina",
			"server":  "Camarero",
			"host":    "Recepción",
			"manager": "Encargado",
		},
		"es_ar": {
			"kitchen": "Cocina",
			"server":  "Mozo",
			"host":    "Recepción",
			"manager": "Encargado",
		},
	}
	roleKey := strings.ToLower(strings.TrimSpace(role))
	if m, ok := names[family]; ok {
		if label, ok := m[roleKey]; ok {
			return label
		}
	}
	if m, ok := names["en"]; ok {
		if label, ok := m[roleKey]; ok {
			return label
		}
	}
	return role
}

// sendStaffInvitationEmailIfConfigured delivers the invite email when the
// email server is configured. Returns whether the email was actually handed
// to the provider; (false, nil) means "email not configured". Package-level
// var so handler tests can stub delivery outcomes (P2-21). The send is
// stamped with the inviting business so it counts against that tenant's
// outbound email budget (emails.ErrTenantMailBudgetExceeded when spent).
var sendStaffInvitationEmailIfConfigured = func(businessID uint, to []string, businessName, staffName, role, invitationURL, language string, expiresDays int) (bool, error) {
	if emails.EmailServerInstance == nil {
		logger.Logger.Infof("[StaffInvite] email server not configured; skipping invitation email for %s", logger.RedactEmails(to))
		return false, nil
	}
	if err := emails.EmailServerInstance.ForBusiness(businessID).SendStaffInvitationEmail(
		to,
		businessName,
		staffName,
		staffRoleDisplayName(role, language),
		invitationURL,
		language,
		expiresDays,
	); err != nil {
		return false, err
	}
	return true, nil
}

// sendStaffAddedEmailIfConfigured tells the venue's contact address that a
// staff invitation went out. business.Email is typed by the tenant and is not
// verified, so the notice is stamped with the business like the invite itself:
// it counts against the tenant's outbound email budget instead of riding along
// as uncounted system mail.
func sendStaffAddedEmailIfConfigured(businessID uint, to []string, ownerName, staffRole, dashboardURL, language string) error {
	if emails.EmailServerInstance == nil {
		logger.Logger.Infof("[StaffInvite] email server not configured; skipping owner notification for %s", logger.RedactEmails(to))
		return nil
	}

	return emails.EmailServerInstance.ForBusiness(businessID).SendStaffAddedEmail(to, ownerName, staffRoleDisplayName(staffRole, language), dashboardURL, language)
}

func getPendingInvitationByToken(token string) (*database.StaffInvitation, error) {
	db := database.GetDBWrapper()
	invitation, err := db.StaffInvitationService.GetByToken(strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}

	if time.Now().After(invitation.ExpiresAt) {
		if err := db.StaffInvitationService.UpdateStatus(invitation.ID, database.InvitationStatusExpired); err != nil {
			log.Printf("WARNING: failed to update invitation %d status to expired: %v", invitation.ID, err)
		}
		return nil, fmt.Errorf("invitation has expired")
	}

	return invitation, nil
}

var (
	staffLoginCodeGenerator = generateLoginCode
	sendStaffLoginCodeEmail = func(staff *database.Staff, business *database.Business, loginCode string) error {
		if emails.EmailServerInstance == nil {
			return fmt.Errorf("email server not configured")
		}

		businessName := "Your Business"
		language := emails.LanguageEnglish
		if business != nil {
			businessName = business.Name
			if business.DefaultLanguage != "" {
				language = business.DefaultLanguage
			}
		}

		return emails.EmailServerInstance.SendStaffLoginCodeEmail(
			[]string{staff.Email},
			businessName,
			staff.Name,
			loginCode,
			language,
			10,
		)
	}
	prepareStaffSession = func(c *gin.Context, staff *database.Staff, provider string) (*preparedStaffLogin, error) {
		prepared := &preparedStaffLogin{}

		if session.GlobalStore != nil {
			staffID := staff.ID
			sess, err := session.GlobalStore.Create(session.CreateInput{
				UserID:    &staffID,
				Provider:  provider,
				IPAddress: c.ClientIP(),
				UserAgent: c.GetHeader("User-Agent"),
				ExpiresAt: time.Now().Add(24 * time.Hour),
			})
			if err != nil {
				return nil, err
			}

			prepared.sessionID = &sess.ID

			token, err := GenerateStaffToken(staff, sess.ID)
			if err != nil {
				_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
				return nil, err
			}

			if err := session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)); err != nil {
				_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
				return nil, err
			}

			refreshToken, err := session.GenerateRefreshToken()
			if err != nil {
				_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
				return nil, err
			}

			if err := session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshToken), time.Now().Add(7*24*time.Hour)); err != nil {
				_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
				return nil, err
			}

			prepared.token = token
			prepared.refreshToken = refreshToken
			return prepared, nil
		}

		token, err := GenerateStaffToken(staff)
		if err != nil {
			return nil, err
		}

		prepared.token = token
		return prepared, nil
	}
	prepareStaffLoginSession = func(c *gin.Context, staff *database.Staff) (*preparedStaffLogin, error) {
		return prepareStaffSession(c, staff, "staff_code")
	}
	prepareAcceptedInvitationSession = func(c *gin.Context, staff *database.Staff) (*preparedStaffLogin, error) {
		return prepareStaffSession(c, staff, "staff_invite")
	}
	revokePreparedStaffLoginSession = func(prepared *preparedStaffLogin) {
		if prepared == nil || prepared.sessionID == nil || session.GlobalStore == nil {
			return
		}
		_ = session.GlobalStore.RevokeWithReason(*prepared.sessionID, session.RevocationReasonInvalidSession)
	}
)

type preparedStaffLogin struct {
	token        string
	refreshToken string
	sessionID    *uint
}

// deleteStaffPushSubscriptions removes a removed/deactivated staff member's
// Web Push subscriptions for ONE business. The user behind the staff row is
// resolved via the same staff.email → users.email seam notifyStaff uses,
// case-insensitively (staff emails are lowercased at write, user emails are
// stored verbatim). If no user account exists there is nothing to delete.
// Best-effort: the staff removal already succeeded, so failures are logged,
// never propagated.
func deleteStaffPushSubscriptions(businessID uint, staffEmail string) {
	email := strings.ToLower(strings.TrimSpace(staffEmail))
	if email == "" || businessID == 0 {
		return
	}
	res := database.GetDB().
		Where("business_id = ? AND user_id IN (SELECT id FROM users WHERE LOWER(email) = ?)", businessID, email).
		Delete(&database.PushSubscription{})
	if res.Error != nil {
		log.Printf("WARNING: failed to delete push subscriptions for offboarded staff (business_id=%d): %v", businessID, res.Error)
	}
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
func generateLoginCode() (string, error) {
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	n := int(bytes[0])<<16 | int(bytes[1])<<8 | int(bytes[2])
	return fmt.Sprintf("%06d", n%1000000), nil
}

// generateInvitationToken generates a secure random token for staff invitations
func generateInvitationToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return fmt.Sprintf("%x", bytes), nil
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

// InviteStaff handles staff invitation by business owners
func InviteStaff(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var req InviteStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	req.Email = normalizeStaffEmailAddress(req.Email)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
		return
	}

	if !IsValidStaffRole(req.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}

	// Hierarchy: a staff actor cannot invite a peer- or higher-ranked role
	// (otherwise a manager could mint a sock-puppet manager that bypasses
	// every existing hierarchy guard on existing staff). Owner Web3/OAuth
	// callers skip this check.
	if actorRoleVal, ok := c.Get("staff_role"); ok {
		actorRoleStr, _ := actorRoleVal.(string)
		if !CanManageRole(database.StaffRole(actorRoleStr), req.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot invite a staff member at or above your role"})
			return
		}
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Check this business's membership only. The same canonical identity may be
	// active in another business, while an inactive same-business row is eligible
	// for the rehire flow when the invitation is accepted.
	// silently skipping the guard could mint duplicate accounts/invitations.
	existingStaff, existingStaffErr := db.StaffService.GetByBusinessAndEmail(business.ID, req.Email)
	if existingStaffErr != nil && !errors.Is(existingStaffErr, gorm.ErrRecordNotFound) &&
		!strings.Contains(strings.ToLower(existingStaffErr.Error()), "record not found") {
		logger.Logger.Errorf("[StaffInvite] failed to check existing staff for %s: %v", logger.RedactEmail(req.Email), existingStaffErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate invitation", "code": "INVITE_VALIDATION_FAILED"})
		return
	}
	if existingStaff != nil && existingStaff.IsActive {
		c.JSON(http.StatusConflict, gin.H{"error": "Staff member with this email already exists", "code": "STAFF_EMAIL_EXISTS"})
		return
	}

	// Check for pending invitation — same fail-loud rule.
	pendingInvitations, pendingErr := db.StaffInvitationService.GetByBusinessID(business.ID)
	if pendingErr != nil {
		log.Printf("[StaffInvite] failed to list pending invitations for business %d: %v", business.ID, pendingErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate invitation", "code": "INVITE_VALIDATION_FAILED"})
		return
	}
	for _, invitation := range pendingInvitations {
		if normalizeStaffEmailAddress(invitation.Email) == req.Email && invitation.Status == database.InvitationStatusPending {
			c.JSON(http.StatusConflict, gin.H{"error": "Invitation already sent to this email", "code": "INVITE_ALREADY_PENDING"})
			return
		}
	}

	// Generate secure invitation token
	token, err := generateSecureToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate invitation token"})
		return
	}

	// Get inviter identifier (address for Web3, email for OAuth)
	var invitedBy string
	if userAddress, hasAddr := c.Get("address"); hasAddr {
		if s, ok := userAddress.(string); ok {
			invitedBy = s
		}
	} else if userEmail, hasEmail := c.Get("email"); hasEmail {
		if s, ok := userEmail.(string); ok {
			invitedBy = s
		}
	}

	// Create invitation
	invitation := &database.StaffInvitation{
		BusinessID: business.ID,
		Email:      strings.ToLower(req.Email),
		Name:       req.Name,
		Role:       req.Role,
		Token:      token,
		Status:     database.InvitationStatusPending,
		InvitedBy:  invitedBy,
		ExpiresAt:  time.Now().Add(7 * 24 * time.Hour), // 7 days
	}

	if err := db.StaffInvitationService.Create(invitation); err != nil {
		if isUniqueConstraintError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "Invitation already sent to this email", "code": "INVITE_ALREADY_PENDING"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invitation"})
		return
	}

	// Send invitation email
	baseURL := config.FrontendBaseURL()
	invitationURL := fmt.Sprintf("%s/staff/accept-invitation?token=%s", baseURL, token)

	// Get business language preference (default to English)
	language := emails.LanguageEnglish
	if business.DefaultLanguage != "" {
		language = business.DefaultLanguage
	}

	// Send invitation email to staff member
	emailSent, emailErr := sendStaffInvitationEmailIfConfigured(
		business.ID,
		[]string{req.Email},
		business.Name,
		req.Name,
		string(req.Role),
		invitationURL,
		language,
		7, // expires in 7 days
	)
	if emailErr != nil {
		logger.Logger.Errorf("[StaffInvite] failed to send invitation email to %s: %v", logger.RedactEmail(req.Email), emailErr)
	}

	// Send notification to business owner. Skipped when the invite itself
	// was refused for budget: the notice would say an invitation went out
	// when it did not, and it would spend budget the invite could not get.
	if business.Email != "" && !errors.Is(emailErr, emails.ErrTenantMailBudgetExceeded) {
		dashboardURL := fmt.Sprintf("%s/business/%d/dashboard", baseURL, business.ID)

		if err := sendStaffAddedEmailIfConfigured(
			business.ID,
			[]string{business.Email},
			business.Name, // owner name
			string(req.Role),
			dashboardURL,
			language,
		); err != nil {
			// Log error but don't fail the request
			log.Printf("[StaffInvite] failed to send staff added notification to owner: %v", err)
		}
	}

	c.JSON(http.StatusCreated, withStaffInviteEmailError(gin.H{
		"message":        "Staff invitation sent successfully",
		"invitation_id":  invitation.ID,
		"expires_at":     invitation.ExpiresAt,
		"email_sent":     emailSent,
		"invitation_url": invitationURL,
	}, emailErr))
}

// withStaffInviteEmailError tells the inviter why the invite email was not
// sent when the reason is actionable. The invitation itself is already
// created (a 429 here would make the retry hit "already invited"), so the
// response stays 2xx with email_sent=false, the copyable invitation_url, and
// email_error_code=email_budget_exceeded once the tenant's daily outbound
// email budget is spent.
func withStaffInviteEmailError(body gin.H, emailErr error) gin.H {
	if errors.Is(emailErr, emails.ErrTenantMailBudgetExceeded) {
		body["email_error_code"] = "email_budget_exceeded"
	}
	return body
}

// ResendInvitation resends an existing staff invitation
func ResendInvitation(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	invitationIDStr := c.Param("invitationId")
	invitationID, err := strconv.ParseUint(invitationIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invitation ID"})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Get the existing invitation
	invitation, err := db.StaffInvitationService.GetByID(uint(invitationID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}

	// Verify invitation belongs to this business
	if invitation.BusinessID != business.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invitation does not belong to this business"})
		return
	}

	// Hierarchy: staff actor cannot resend an invitation for a peer- or higher-
	// ranked role (otherwise a manager could keep alive an owner-created
	// manager-tier invitation indefinitely). Owner Web3/OAuth callers skip.
	if actorRoleVal, ok := c.Get("staff_role"); ok {
		actorRoleStr, _ := actorRoleVal.(string)
		if !CanManageRole(database.StaffRole(actorRoleStr), invitation.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot resend an invitation at or above your role"})
			return
		}
	}

	// Accepted or revoked invitations cannot be resent.
	if invitation.Status == database.InvitationStatusAccepted || invitation.Status == database.InvitationStatusRevoked {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot resend this invitation", "code": "INVITE_CANNOT_RESEND"})
		return
	}

	// Generate new token and extend expiry
	newToken, err := generateInvitationToken()
	if err != nil {
		log.Printf("[StaffInvite] Failed to generate invitation token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate invitation token"})
		return
	}
	newExpiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days from now

	// Update invitation with new token and expiry
	invitation.Token = newToken
	invitation.ExpiresAt = newExpiresAt
	invitation.Status = database.InvitationStatusPending

	if err := db.StaffInvitationService.Update(invitation); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update invitation"})
		return
	}

	// Send invitation email
	baseURL := config.FrontendBaseURL()
	invitationURL := fmt.Sprintf("%s/staff/accept-invitation?token=%s", baseURL, newToken)

	// Get business language preference (default to English)
	language := emails.LanguageEnglish
	if business.DefaultLanguage != "" {
		language = business.DefaultLanguage
	}

	emailSent, emailErr := sendStaffInvitationEmailIfConfigured(
		business.ID,
		[]string{invitation.Email},
		business.Name,
		invitation.Name,
		string(invitation.Role),
		invitationURL,
		language,
		7, // expires in 7 days
	)
	if emailErr != nil {
		logger.Logger.Errorf("[StaffInvite] failed to send invitation email to %s: %v", logger.RedactEmail(invitation.Email), emailErr)
	}

	c.JSON(http.StatusOK, withStaffInviteEmailError(gin.H{
		"message":        "Staff invitation resent successfully",
		"invitation_id":  invitation.ID,
		"expires_at":     invitation.ExpiresAt,
		"email_sent":     emailSent,
		"invitation_url": invitationURL,
	}, emailErr))
}

// RevokeInvitation invalidates a pending/expired invitation: it sets the status
// to revoked AND rotates the token to a fresh random value so the ORIGINAL
// invitation link (which the recipient may still hold) can never be accepted.
// Accepted invitations cannot be revoked (the staff member already exists — use
// staff removal instead). The same role-hierarchy guard as resend applies: a
// staff actor cannot revoke an invitation at or above their own role.
func RevokeInvitation(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	invitationIDStr := c.Param("invitationId")
	invitationID, err := strconv.ParseUint(invitationIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invitation ID"})
		return
	}

	db := database.GetDBWrapper()
	invitation, err := db.StaffInvitationService.GetByID(uint(invitationID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}
	if invitation.BusinessID != business.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invitation does not belong to this business"})
		return
	}

	// Hierarchy: a staff actor cannot revoke an invitation at or above their role.
	if actorRoleVal, ok := c.Get("staff_role"); ok {
		actorRoleStr, _ := actorRoleVal.(string)
		if !CanManageRole(database.StaffRole(actorRoleStr), invitation.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot revoke an invitation at or above your role"})
			return
		}
	}

	// An accepted invitation is already a real staff member — revoking the invite
	// would be misleading; the operator should remove the staff member instead.
	if invitation.Status == database.InvitationStatusAccepted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot revoke an accepted invitation", "code": "INVITE_ALREADY_ACCEPTED"})
		return
	}
	// Idempotent: revoking an already-revoked invitation is a no-op success.
	if invitation.Status == database.InvitationStatusRevoked {
		c.JSON(http.StatusOK, gin.H{"message": "Invitation already revoked", "invitation_id": invitation.ID})
		return
	}

	// Rotate the token so the ORIGINAL link is dead, and flip the status.
	deadToken, tErr := generateInvitationToken()
	if tErr != nil {
		log.Printf("[StaffInvite] Failed to generate invalidation token: %v", tErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke invitation"})
		return
	}
	invitation.Token = deadToken
	invitation.Status = database.InvitationStatusRevoked
	if err := db.StaffInvitationService.Update(invitation); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke invitation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Staff invitation revoked",
		"invitation_id": invitation.ID,
	})
}

// GetInvitationLink returns the accept URL for a pending/expired invitation.
// Gated by staff:invite (same as invite/resend/revoke). Invitation tokens must
// never appear on staff:read list responses — this is the only API path that
// re-materializes the secret for authorized inviters (copy-link / out-of-band).
func GetInvitationLink(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	invitationIDStr := c.Param("invitationId")
	invitationID, err := strconv.ParseUint(invitationIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invitation ID"})
		return
	}

	db := database.GetDBWrapper()
	invitation, err := db.StaffInvitationService.GetByID(uint(invitationID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invitation not found"})
		return
	}
	if invitation.BusinessID != business.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invitation does not belong to this business"})
		return
	}

	// Hierarchy: staff actor cannot read a link for a peer- or higher-ranked role.
	if actorRoleVal, ok := c.Get("staff_role"); ok {
		actorRoleStr, _ := actorRoleVal.(string)
		if !CanManageRole(database.StaffRole(actorRoleStr), invitation.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot access an invitation at or above your role"})
			return
		}
	}

	if invitation.Status != database.InvitationStatusPending && invitation.Status != database.InvitationStatusExpired {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invitation is no longer usable", "code": "INVITE_NOT_USABLE"})
		return
	}
	if invitation.Token == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invitation token missing"})
		return
	}

	baseURL := config.FrontendBaseURL()
	invitationURL := fmt.Sprintf("%s/staff/accept-invitation?token=%s", baseURL, invitation.Token)
	c.JSON(http.StatusOK, gin.H{
		"invitation_id":  invitation.ID,
		"invitation_url": invitationURL,
		"expires_at":     invitation.ExpiresAt,
	})
}

func GetStaffInvitationPreview(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invitation token is required"})
		return
	}

	invitation, err := getPendingInvitationByToken(token)
	if err != nil {
		if strings.EqualFold(err.Error(), "invitation has expired") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invitation has expired", "code": "INVITE_EXPIRED"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Invalid or expired invitation", "code": "INVITE_INVALID"})
		return
	}

	businessName := ""
	businessCustomURL := ""
	businessSlug := ""
	if business, businessErr := database.GetDBWrapper().BusinessService.GetByID(invitation.BusinessID); businessErr == nil && business != nil {
		businessName = business.Name
		businessCustomURL = business.CustomURL
		businessSlug = business.BusinessId
	}

	c.JSON(http.StatusOK, gin.H{
		"email":               invitation.Email,
		"name":                invitation.Name,
		"role":                invitation.Role,
		"business_id":         invitation.BusinessID,
		"business_name":       businessName,
		"business_custom_url": businessCustomURL,
		// business_slug is the operator-route identifier (businesses.business_id),
		// the same value staff sessions expose; custom_url is storefront-only.
		"business_slug": businessSlug,
		"expires_at":    invitation.ExpiresAt,
	})
}

// AcceptInvitation handles staff accepting invitations
func AcceptInvitation(c *gin.Context) {
	var req AcceptInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Find invitation by token
	invitation, err := getPendingInvitationByToken(req.Token)
	if err != nil {
		if strings.EqualFold(err.Error(), "invitation has expired") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invitation has expired", "code": "INVITE_EXPIRED"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Invalid or expired invitation", "code": "INVITE_INVALID"})
		return
	}

	accepted, err := db.StaffService.AcceptStaffInvitation(invitation.ID, req.Name)
	if err != nil {
		if errors.Is(err, database.ErrStaffMembershipActive) {
			c.JSON(http.StatusConflict, gin.H{"error": "A staff account already exists for this invitation email", "code": "STAFF_ACCOUNT_EXISTS"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create staff account"})
		return
	}
	staff := accepted.Staff

	businessName := ""
	var businessId string
	if business, businessErr := db.BusinessService.GetByID(invitation.BusinessID); businessErr == nil && business != nil {
		businessName = business.Name
		businessId = business.BusinessId
	}

	prepared, err := prepareAcceptedInvitationSession(c, staff)
	if err != nil {
		if restoreErr := db.StaffService.RestoreInvitationAcceptance(invitation.ID, accepted); restoreErr != nil {
			log.Printf("WARNING: failed to restore invitation %d after acceptance error: %v", invitation.ID, restoreErr)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	utils.ClearAllAuthCookies(c)
	if prepared.refreshToken != "" {
		utils.SetSessionCookie(c, "refresh_token", prepared.refreshToken, 7*24*3600)
	}

	utils.SetSessionCookie(c, "staff_token", prepared.token, 15*60)

	c.JSON(http.StatusCreated, gin.H{
		"message":       "Staff account created successfully",
		"token":         prepared.token,
		"role":          staff.Role,
		"business_name": businessName,
		"business_id":   businessId,
		"staff": gin.H{
			"id":          staff.ID,
			"name":        staff.Name,
			"email":       staff.Email,
			"role":        staff.Role,
			"business_id": staff.BusinessID,
		},
	})
}

// RequestLoginCode handles staff login code requests
func RequestLoginCode(c *gin.Context) {
	var req StaffLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	req.Email = normalizeStaffEmailAddress(req.Email)
	if staffLoginEmailUndeliverable(req.Email) {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Enter a deliverable email address")
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()
	successResponse := gin.H{
		"message":            loginCodeMaybeSentMessage,
		"expires_in_minutes": 10,
	}

	// Authenticate the canonical identity first. For a multi-business identity a
	// code is attached to one legacy row only as an expand-phase compatibility
	// detail; no business-scoped token is minted until verification + selection.
	memberships, err := db.StaffService.GetActiveByEmail(req.Email)
	if err != nil || len(memberships) == 0 {
		c.JSON(http.StatusOK, successResponse)
		return
	}
	if !staffLoginCodeSendGuard.reserve(normalizeStaffEmailAddress(req.Email)) {
		log.Printf("[StaffAuth] login code send limit reached")
		c.JSON(http.StatusOK, successResponse)
		return
	}
	staff := &memberships[0]
	if req.BusinessID != nil {
		for i := range memberships {
			if memberships[i].BusinessID == *req.BusinessID {
				staff = &memberships[i]
				break
			}
		}
	}

	if err := db.StaffLoginCodeService.DeleteExpiredCodes(); err != nil {
		log.Printf("[StaffAuth] Failed to delete expired login codes: %v", err)
	}

	var loginCode *database.StaffLoginCode
	for attempt := 0; attempt < 5; attempt++ {
		code, err := staffLoginCodeGenerator()
		if err != nil {
			log.Printf("[StaffAuth] Failed to generate login code: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate login code"})
			return
		}

		candidate := &database.StaffLoginCode{
			StaffID:   staff.ID,
			Code:      code,
			ExpiresAt: time.Now().Add(10 * time.Minute), // 10 minutes
			Used:      false,
		}

		if err := db.StaffLoginCodeService.Create(candidate); err != nil {
			if isUniqueConstraintError(err) {
				continue
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate login code"})
			return
		}

		loginCode = candidate
		break
	}
	if loginCode == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate login code"})
		return
	}

	// Get business info for email
	business, _ := db.BusinessService.GetByID(staff.BusinessID)

	// Send login code email
	if err := sendStaffLoginCodeEmail(staff, business, loginCode.Code); err != nil {
		_ = db.StaffLoginCodeService.Delete(loginCode.ID)
		fmt.Printf("Failed to send login code email: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send login code"})
		return
	}

	if err := db.StaffLoginCodeService.ActivateCodeForStaff(staff.ID, loginCode.ID); err != nil {
		_ = db.StaffLoginCodeService.Delete(loginCode.ID)
		log.Printf("[StaffAuth] Failed to activate login code for staff %d: %v", staff.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate login code"})
		return
	}

	c.JSON(http.StatusOK, successResponse)
}

// VerifyLoginCode handles staff login code verification
func VerifyLoginCode(c *gin.Context) {
	var req VerifyLoginCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	req.Email = normalizeStaffEmailAddress(req.Email)
	req.Code = strings.TrimSpace(req.Code)
	req.SelectionToken = strings.TrimSpace(req.SelectionToken)

	// Get database wrapper
	db := database.GetDBWrapper()
	if req.SelectionToken != "" {
		if req.BusinessID == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Business selection is required", "code": "MEMBERSHIP_SELECTION_REQUIRED"})
			return
		}
		email, jtiHash, err := parseStaffMembershipSelectionToken(req.SelectionToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired membership selection", "code": "MEMBERSHIP_SELECTION_INVALID"})
			return
		}
		staff, err := db.StaffService.GetByBusinessAndEmail(*req.BusinessID, email)
		if err != nil || staff == nil || !staff.IsActive {
			c.JSON(http.StatusForbidden, gin.H{"error": "Membership is not active", "code": "MEMBERSHIP_INACTIVE"})
			return
		}
		// Redeem the token atomically before minting a session: a replayed
		// (or concurrently raced) selection token finds no row and is refused.
		consumed, err := database.ConsumeStaffMembershipSelectionToken(jtiHash)
		if err != nil {
			log.Printf("[StaffAuth] Failed to redeem membership selection token: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete membership selection"})
			return
		}
		if !consumed {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired membership selection", "code": "MEMBERSHIP_SELECTION_INVALID"})
			return
		}
		issueStaffLoginResponse(c, staff)
		return
	}
	if req.Email == "" || req.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email and code are required"})
		return
	}

	// Per-(client, email) guard: one client's wrong guesses get it a 429 well
	// before they can trip the identity-wide lockout below, so a stranger on a
	// single network cannot lock a known staff email out. The attempt is claimed atomically before the code is compared, so
	// parallel requests cannot exceed the per-client budget.
	guardKey := staffLoginGuardKey(middleware.ClientRateLimitKey(c), req.Email)
	if !staffLoginGuard.reserve(guardKey) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many attempts. Try again later."})
		return
	}

	memberships, err := db.StaffService.GetActiveByEmail(req.Email)
	if err != nil || len(memberships) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired login code"})
		return
	}
	candidates := memberships
	if req.BusinessID != nil {
		candidates = nil
		for i := range memberships {
			if memberships[i].BusinessID == *req.BusinessID {
				candidates = append(candidates, memberships[i])
				break
			}
		}
		if len(candidates) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired login code"})
			return
		}
	}

	// Identity-wide ceiling (per email, across every client): claim the attempt
	// on every membership atomically BEFORE comparing the code, so neither
	// rotating source addresses nor parallel requests can exceed
	// StaffLoginMaxFailedAttempts. Every membership of the email is claimed,
	// not only the targeted one, so neither adding memberships nor cycling
	// business_id can multiply the brute-force budget.
	identityIDs := make([]uint, len(memberships))
	for i := range memberships {
		identityIDs[i] = memberships[i].ID
	}
	allowed, lockErr := db.StaffService.ReserveLoginAttempt(identityIDs)
	if lockErr != nil {
		// Fail closed: if we cannot claim the attempt, do not let it proceed —
		// otherwise a transient DB fault becomes a brute-force bypass window.
		log.Printf("[StaffAuth] Failed to reserve login attempt for %d staff rows: %v", len(identityIDs), lockErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify login status"})
		return
	}
	if !allowed {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many attempts. Try again later."})
		return
	}

	var staff *database.Staff
	for i := range candidates {
		if _, codeErr := db.StaffLoginCodeService.GetByStaffAndCode(candidates[i].ID, req.Code); codeErr == nil {
			staff = &candidates[i]
			break
		}
	}
	if staff == nil {
		// The attempt is already counted. If it reached the ceiling, burn the
		// outstanding codes so the lockout cannot be waited out and then
		// resumed against the same still-valid code.
		if err := db.StaffService.BurnCodesIfLoginLocked(identityIDs); err != nil {
			log.Printf("[StaffAuth] Failed to burn login codes after lockout: %v", err)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired login code"})
		return
	}

	selectionRequired := len(memberships) > 1 && req.BusinessID == nil
	var prepared *preparedStaffLogin
	if !selectionRequired {
		prepared, err = prepareStaffLoginSession(c, staff)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
			return
		}
	}

	// Consume all matching active rows atomically so duplicate or concurrent verifies cannot replay the code.
	consumed, err := db.StaffLoginCodeService.ConsumeActiveCodesByStaffAndCode(staff.ID, req.Code)
	if err != nil {
		revokePreparedStaffLoginSession(prepared)
		log.Printf("[StaffAuth] Failed to mark login code as used: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify login code"})
		return
	}
	if !consumed {
		revokePreparedStaffLoginSession(prepared)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired login code"})
		return
	}

	// Successful login — clear any failed-attempt count / lockout.
	staffLoginGuard.reset(guardKey)
	for _, id := range identityIDs {
		if err := db.StaffService.ResetLoginAttempts(id); err != nil {
			log.Printf("WARNING: failed to reset login attempts for staff %d: %v", id, err)
		}
	}

	if selectionRequired {
		selectionToken, tokenErr := GenerateStaffMembershipSelectionToken(req.Email)
		if tokenErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create membership selection"})
			return
		}
		choices := make([]gin.H, 0, len(memberships))
		for i := range memberships {
			businessName := ""
			if business, businessErr := db.BusinessService.GetByID(memberships[i].BusinessID); businessErr == nil && business != nil {
				businessName = business.Name
			}
			choices = append(choices, gin.H{
				"business_id":   memberships[i].BusinessID,
				"business_name": businessName,
				"role":          memberships[i].Role,
			})
		}
		c.JSON(http.StatusOK, gin.H{
			"message": "Select a business to continue", "membership_selection_required": true,
			"selection_token": selectionToken, "memberships": choices,
		})
		return
	}

	finishStaffLoginResponse(c, staff, prepared)
}

func issueStaffLoginResponse(c *gin.Context, staff *database.Staff) {
	prepared, err := prepareStaffLoginSession(c, staff)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}
	finishStaffLoginResponse(c, staff, prepared)
}

func finishStaffLoginResponse(c *gin.Context, staff *database.Staff, prepared *preparedStaffLogin) {
	db := database.GetDBWrapper()

	// Update last login time
	if err := db.StaffService.UpdateLastLogin(staff.ID); err != nil {
		log.Printf("WARNING: failed to update last login for staff %d: %v", staff.ID, err)
	}

	utils.ClearAllAuthCookies(c)
	if prepared.refreshToken != "" {
		utils.SetSessionCookie(c, "refresh_token", prepared.refreshToken, 7*24*3600)
	}

	// Set staff_token cookie (matching regular login pattern)
	utils.SetSessionCookie(c, "staff_token", prepared.token, 15*60)

	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"token":   prepared.token,
		"staff": gin.H{
			"id":          staff.ID,
			"name":        staff.Name,
			"email":       staff.Email,
			"role":        staff.Role,
			"business_id": staff.BusinessID,
		},
	})
}

// GetBusinessStaff returns all staff members for a business
func GetBusinessStaff(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Lifecycle management must include inactive memberships so an owner can
	// reach the existing reactivation action. Active-only operational callers
	// continue to use StaffService.GetByBusinessID.
	staff, err := db.StaffService.GetAllByBusinessID(business.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve staff"})
		return
	}

	// Get pending invitations (only pending status)
	allInvitations, err := db.StaffInvitationService.GetByBusinessID(business.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve invitations"})
		return
	}

	// Keep pending and expired invitations visible so managers can resend stale links.
	var pendingInvitations []database.StaffInvitation
	for _, invitation := range allInvitations {
		if invitation.Status == database.InvitationStatusPending || invitation.Status == database.InvitationStatusExpired {
			pendingInvitations = append(pendingInvitations, invitation)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"staff":               staff,
		"pending_invitations": pendingInvitations,
	})
}

// RemoveStaff removes a staff member
func RemoveStaff(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	staffIDStr := c.Param("staffId")
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid staff ID"})
		return
	}

	if actor := ExtractStaffIDFromContext(c); actor != nil && *actor == uint(staffID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot remove yourself"})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Get the staff member to verify they belong to this business
	staff, err := db.StaffService.GetByID(uint(staffID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Staff member not found"})
		return
	}

	if staff.BusinessID != business.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Staff member does not belong to this business"})
		return
	}

	// Hierarchy: a staff actor cannot soft-delete a peer or higher-ranked
	// staff member. Owner Web3/OAuth callers (no staff_role on context)
	// outrank every staff role and skip this check.
	if actorRoleVal, ok := c.Get("staff_role"); ok {
		actorRoleStr, _ := actorRoleVal.(string)
		if !CanManageRole(database.StaffRole(actorRoleStr), staff.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Cannot remove a staff member at or above your role"})
			return
		}
	}

	// Soft delete staff member
	if err := db.StaffService.SoftDelete(uint(staffID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove staff member"})
		return
	}

	// Centralized access revocation: bumps authz_version, revokes scoped
	// sessions, deletes principal-typed staff push subs, publishes SSE eject.
	// Handlers must not reimplement pieces of this — go through the chokepoint.
	changedBy := "unknown"
	if address, exists := c.Get("address"); exists {
		if s, ok := address.(string); ok && s != "" {
			changedBy = s
		}
	} else if email, exists := c.Get("staff_email"); exists {
		if s, ok := email.(string); ok && s != "" {
			changedBy = s
		}
	}
	if err := services.NewRBACService(db).RevokeStaffAccess(uint(staffID), "staff_removed", changedBy); err != nil {
		log.Printf("WARNING: RevokeStaffAccess failed for staff %d: %v", staffID, err)
	}

	// Legacy email→user push cleanup for rows registered before principal_type
	// existed (owner_user subs matched via staff email). Best-effort.
	deleteStaffPushSubscriptions(business.ID, staff.Email)

	// Send notification email to staff member
	language := emails.LanguageEnglish
	if business.DefaultLanguage != "" {
		language = business.DefaultLanguage
	}

	if emails.EmailServerInstance != nil {
		if err := emails.EmailServerInstance.ForBusiness(business.ID).SendAccessRemovedEmail(
			[]string{staff.Email},
			business.Name,
			staff.Name,
			language,
		); err != nil {
			// Log error but don't fail the request
			fmt.Printf("Failed to send access removed email: %v\n", err)
		}
	}

	// Send notification to owner
	if business.Email != "" && emails.EmailServerInstance != nil {
		baseURL := config.FrontendBaseURL()
		dashboardURL := fmt.Sprintf("%s/business/%d/dashboard", baseURL, business.ID)

		// Stamped with the business: business.Email is tenant-typed and
		// unverified, so this notice counts against the tenant budget.
		if err := emails.EmailServerInstance.ForBusiness(business.ID).SendStaffRemovedEmail(
			[]string{business.Email},
			business.Name, // owner name
			dashboardURL,
			language,
		); err != nil {
			fmt.Printf("Failed to send staff removed notification to owner: %v\n", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Staff member removed successfully"})
}

// GetStaffProfile returns the current staff member's profile information
func GetStaffProfile(c *gin.Context) {
	// Get staff ID from context (set by staff auth middleware)
	staffID, exists := c.Get("staff_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Staff ID not found"})
		return
	}

	// Get database wrapper
	db := database.GetDBWrapper()

	// Get staff member - handle type conversion from JWT claims
	var staffIDUint uint
	switch v := staffID.(type) {
	case float64:
		staffIDUint = uint(v)
	case uint:
		staffIDUint = v
	case int:
		staffIDUint = uint(v)
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid staff ID type"})
		return
	}

	staff, err := db.StaffService.GetByID(staffIDUint)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Staff member not found"})
		return
	}

	// Get business information
	business, err := db.BusinessService.GetByID(staff.BusinessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business information"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"staff": gin.H{
			"id":            staff.ID,
			"name":          staff.Name,
			"email":         staff.Email,
			"role":          staff.Role,
			"business_id":   staff.BusinessID,
			"is_active":     staff.IsActive,
			"last_login_at": staff.LastLoginAt,
		},
		"business": gin.H{
			"id":          business.ID,
			"name":        business.Name,
			"business_id": business.BusinessId,
		},
	})
}

// SetStaffPinRequest is the body for POST /inside/staff/:staff_id/pin.
type SetStaffPinRequest struct {
	PIN string `json:"pin" binding:"required"`
}

// SetStaffPin sets (or rotates) a manager PIN on a staff record. Owners can
// set any staff's PIN — useful for first-time enrollment when the staff
// member is standing at the terminal. Staff can rotate their own PIN once
// set. We deliberately omit a lost-PIN recovery flow in IMP-14; owners can
// re-POST to replace an existing hash if a staff loses theirs.
func SetStaffPin(c *gin.Context) {
	staffIDRaw := strings.TrimSpace(c.Param("staff_id"))
	if staffIDRaw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "staff_id is required"})
		return
	}
	parsed, err := strconv.ParseUint(staffIDRaw, 10, 64)
	if err != nil || parsed == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid staff_id"})
		return
	}
	targetStaffID := uint(parsed)

	var req SetStaffPinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	req.PIN = strings.TrimSpace(req.PIN)

	// Resolve the staff row to verify it exists and to authorize the caller
	// against the staff's business.
	staff, err := database.GetDBWrapper().StaffService.GetByID(targetStaffID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Staff not found"})
		return
	}

	business, err := database.GetBusinessByID(staff.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Authorization: a business owner may set any staff PIN. A staff member
	// may rotate THEIR OWN PIN only — comparing against the staff_id in the
	// caller's token guards against horizontal escalation.
	isOwner := CheckBusinessOwnership(c, business)
	isSelfRotation := false
	if callerStaffID := staffIDFromContext(c); callerStaffID != nil && *callerStaffID == targetStaffID {
		isSelfRotation = true
	}

	if !isOwner && !isSelfRotation {
		RespondWithError(c, http.StatusForbidden, ErrCodeForbidden, "Only the owner or the staff themselves can set this PIN")
		return
	}

	if err := database.SetStaffPin(targetStaffID, req.PIN); err != nil {
		switch {
		case errors.Is(err, database.ErrPinTooShort),
			errors.Is(err, database.ErrPinTooLong),
			errors.Is(err, database.ErrPinNotNumeric):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			log.Printf("[ManagerPIN] Failed to set PIN for staff %d: %v", targetStaffID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set PIN"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Manager PIN updated"})
}

// StaffLogout handles staff logout — revokes session and clears cookie
func StaffLogout(c *gin.Context) {
	_, exists := c.Get("staff_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Staff ID not found"})
		return
	}

	// Revoke the session so the token is immediately invalid — including when
	// only the refresh cookie remains (access staff JWT already expired).
	if session.GlobalStore != nil {
		tokenString := extractTokenFromRequest(c, "staff_token")
		if tokenString != "" {
			if err := session.GlobalStore.RevokeByTokenHashWithReason(session.HashToken(tokenString), session.RevocationReasonUserLogout); err != nil {
				log.Printf("WARNING: failed to revoke session token: %v", err)
			}
		}
		if refreshRaw, err := c.Cookie("refresh_token"); err == nil && refreshRaw != "" {
			if err := session.GlobalStore.RevokeByRefreshTokenHashWithReason(session.HashToken(refreshRaw), session.RevocationReasonUserLogout); err != nil {
				log.Printf("WARNING: failed to revoke refresh session token: %v", err)
			}
		}
	}

	utils.ClearAllAuthCookies(c)

	c.JSON(http.StatusOK, gin.H{
		"message": "Logout successful",
	})
}
