package server

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/session"

	"github.com/gin-gonic/gin"
)

// IMP-20 — Account / data export + delete (GDPR / UAE PDPL self-service).
//
// Two endpoints live here:
//
//   POST /api/v1/inside/account/export — assembles a JSON snapshot of the
//     authenticated user's profile + owned businesses (basic fields).
//     Inlined in the response (small enough
//     for a one-shot download). A signed-URL flow + 24h TTL is the planned
//     follow-up if exports start exceeding ~5MB.
//
//   POST /api/v1/inside/account/delete — soft-delete. Sets `deleted_at` +
//     `deletion_scheduled_at = now() + 30d`. Final deletion requires reviewed
//     handling because tax, fiscal, billing, fraud, security, or legal records
//     may need to remain.

// accountExportBusiness is the minimal business shape we expose in an export.
type accountExportBusiness struct {
	ID              uint   `json:"id"`
	BusinessID      string `json:"business_id"`
	Name            string `json:"name"`
	DefaultCurrency string `json:"default_currency"`
	DisplayCurrency string `json:"display_currency"`
}

// accountExportPayload is the full JSON shape returned by /account/export.
type accountExportPayload struct {
	GeneratedAt time.Time `json:"generated_at"`
	// 24h "freshness" / TTL marker — the snapshot is a moment-in-time view;
	// re-request after this point to get an updated copy. We embed this so
	// downstream tooling (and the operator) can tell when the file was made.
	ExportTTLAt time.Time `json:"export_ttl_at"`
	Profile     struct {
		ID                  uint   `json:"id"`
		Email               string `json:"email"`
		Name                string `json:"name"`
		Username            string `json:"username"`
		WalletAddress       string `json:"wallet_address"`
		Role                string `json:"role"`
		AuthMethod          string `json:"auth_method"`
		EmailVerified       bool   `json:"email_verified"`
		LanguageSelected    string `json:"language_selected"`
		CreatedAt           string `json:"created_at"`
		DeletedAt           string `json:"deleted_at,omitempty"`
		DeletionScheduledAt string `json:"deletion_scheduled_at,omitempty"`
	} `json:"profile"`
	Businesses []accountExportBusiness `json:"businesses"`
}

// resolveAccountUser pulls the authenticated user's DB row using whichever
// auth surface (web3 address, OAuth user_id, OAuth email) the request
// carries. Returns the loaded row or writes the 401/500 and reports false.
func resolveAccountUser(c *gin.Context) (*database.User, bool) {
	db := database.GetDB()

	// 1. OAuth path: user_id is the most reliable identifier.
	if uidVal, ok := c.Get("user_id"); ok {
		if uid, parsed := extractContextUint(uidVal); parsed && uid > 0 {
			var u database.User
			if err := db.First(&u, uid).Error; err == nil {
				return &u, true
			} else {
				log.Printf("[account] resolve by user_id=%d failed: %v", uid, err)
			}
		}
	}

	// 2. OAuth path fallback: email.
	if emailVal, ok := c.Get("email"); ok {
		if email, _ := emailVal.(string); email != "" {
			var u database.User
			if err := db.Where("email = ?", email).First(&u).Error; err == nil {
				return &u, true
			}
		}
	}

	// 3. Web3 path: wallet address.
	if addrVal, ok := c.Get("address"); ok {
		if addr, _ := addrVal.(string); addr != "" {
			var u database.User
			if err := db.Where("address = ?", strings.ToLower(addr)).First(&u).Error; err == nil {
				return &u, true
			}
		}
	}

	c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
	return nil, false
}

// loadOwnedBusinessesForUser returns the businesses owned by this user,
// matching either by user_id or by lower-cased wallet address.
func loadOwnedBusinessesForUser(user *database.User) ([]database.Business, error) {
	db := database.GetDB()

	var out []database.Business
	q := db.Model(&database.Business{})

	addrLower := strings.ToLower(strings.TrimSpace(user.Address))
	switch {
	case user.ID != 0 && addrLower != "":
		q = q.Where("user_id = ? OR LOWER(owner_address) = ?", user.ID, addrLower)
	case user.ID != 0:
		q = q.Where("user_id = ?", user.ID)
	case addrLower != "":
		q = q.Where("LOWER(owner_address) = ?", addrLower)
	default:
		return []database.Business{}, nil
	}

	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// ExportAccountData implements POST /api/v1/inside/account/export.
//
// Returns the JSON snapshot inline. Logged + tracked so the audit trail
// captures every export request (GDPR Art. 15 traceability).
func ExportAccountData(c *gin.Context) {
	user, ok := resolveAccountUser(c)
	if !ok {
		return
	}

	businesses, err := loadOwnedBusinessesForUser(user)
	if err != nil {
		log.Printf("[account] export: load businesses for user %d failed: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assemble export"})
		return
	}

	exportBizs := make([]accountExportBusiness, 0, len(businesses))
	for _, b := range businesses {
		exportBizs = append(exportBizs, accountExportBusiness{
			ID:              b.ID,
			BusinessID:      b.BusinessId,
			Name:            b.Name,
			DefaultCurrency: b.DefaultCurrency,
			DisplayCurrency: b.DisplayCurrency,
		})
	}

	now := time.Now().UTC()
	payload := accountExportPayload{
		GeneratedAt: now,
		ExportTTLAt: now.Add(24 * time.Hour),
		Businesses:  exportBizs,
	}
	payload.Profile.ID = user.ID
	payload.Profile.Email = user.Email
	payload.Profile.Name = user.Name
	payload.Profile.Username = user.Username
	payload.Profile.WalletAddress = user.Address
	payload.Profile.Role = user.Role
	payload.Profile.AuthMethod = user.AuthMethod
	payload.Profile.EmailVerified = user.EmailVerified
	payload.Profile.LanguageSelected = user.LanguageSelected
	payload.Profile.CreatedAt = user.CreatedAt.UTC().Format(time.RFC3339)
	if user.DeletedAt != nil {
		payload.Profile.DeletedAt = user.DeletedAt.UTC().Format(time.RFC3339)
	}
	if user.DeletionScheduledAt != nil {
		payload.Profile.DeletionScheduledAt = user.DeletionScheduledAt.UTC().Format(time.RFC3339)
	}

	log.Printf("[account] export issued for user_id=%d email=%s businesses=%d",
		user.ID, logger.RedactEmail(user.Email), len(exportBizs))

	// Inline response — small enough to download client-side. If a future
	// export crosses ~5MB we'll move to a signed S3 URL (see spec doc).
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition",
		"attachment; filename=\"payverge-account-export-"+now.Format("20060102-150405")+".json\"")
	c.JSON(http.StatusOK, payload)
}

// accountDeleteRequest is the JSON body for the delete endpoint. The email
// is a typed-confirmation guard — the frontend collects the user's own
// email in a modal and we verify it matches before flipping the soft-delete
// flag. Cheap defence against "fat-fingered delete" support tickets.
type accountDeleteRequest struct {
	ConfirmEmail string `json:"confirm_email"`
	Reason       string `json:"reason"`
}

var accountDeletionRetentionExceptions = []string{
	"tax", "fiscal", "billing", "fraud", "security", "legal",
}

// RequestAccountDeletion implements POST /api/v1/inside/account/delete.
//
// Soft-deletes the user (DeletedAt + DeletionScheduledAt + 30d window). When
// the window ends, services.RunAccountErasure anonymizes the account's
// identifiers and login secrets; rows (and owned business records, kept for
// the retention exceptions) are never hard-deleted.
func RequestAccountDeletion(c *gin.Context) {
	user, ok := resolveAccountUser(c)
	if !ok {
		return
	}

	var req accountDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Typed-confirmation: must match the user's email (case-insensitive).
	// Accounts without an email (pure-Web3) fall back to confirming the
	// truncated wallet address — the frontend mirrors that.
	confirm := strings.TrimSpace(strings.ToLower(req.ConfirmEmail))
	want := strings.TrimSpace(strings.ToLower(user.Email))
	if want == "" {
		want = strings.TrimSpace(strings.ToLower(user.Address))
	}
	if confirm == "" || confirm != want {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Confirmation does not match account email"})
		return
	}

	if user.DeletedAt != nil {
		// Idempotent: same response shape as a successful first call so the
		// UI can treat retries the same way.
		c.JSON(http.StatusOK, gin.H{
			"success":               true,
			"already_pending":       true,
			"deleted_at":            user.DeletedAt,
			"deletion_scheduled_at": user.DeletionScheduledAt,
			"deletion_mode":         "anonymize_after_grace",
			"hard_delete_automated": false,
			"retention_exceptions":  accountDeletionRetentionExceptions,
		})
		return
	}

	now := time.Now().UTC()
	scheduled := now.Add(30 * 24 * time.Hour)
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 1024 {
		reason = reason[:1024]
	}

	updates := map[string]any{
		"deleted_at":            now,
		"deletion_scheduled_at": scheduled,
		"deletion_reason":       reason,
	}
	if err := database.GetDB().Model(&database.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		log.Printf("[account] delete: failed to mark user %d: %v", user.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to schedule account deletion"})
		return
	}

	// Revoke all active sessions so the user is immediately logged out
	// after requesting account deletion. Best-effort: log but don't fail
	// the request if session revocation errors (the account is already
	// marked for deletion).
	if session.GlobalStore != nil {
		if err := session.GlobalStore.RevokeAllLinkedOperatorSessions(
			user.ID,
			[]string{user.Address},
			session.RevocationReasonAccountDisabled,
		); err != nil {
			log.Printf("[account] delete: failed to revoke sessions for user %d: %v", user.ID, err)
		}
	}

	log.Printf("[account] deletion scheduled user_id=%d email=%s scheduled_at=%s reason=%q",
		user.ID, logger.RedactEmail(user.Email), scheduled.Format(time.RFC3339), reason)

	c.JSON(http.StatusOK, gin.H{
		"success":               true,
		"deleted_at":            now,
		"deletion_scheduled_at": scheduled,
		"grace_period_days":     30,
		"deletion_mode":         "anonymize_after_grace",
		"hard_delete_automated": false,
		"retention_exceptions":  accountDeletionRetentionExceptions,
	})
}
