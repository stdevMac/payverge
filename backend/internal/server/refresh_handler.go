package server

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RefreshToken handles token refresh using the refresh_token cookie.
func RefreshToken(c *gin.Context) {
	outcome := metrics.RefreshOutcomeInvalid
	defer func() {
		metrics.RecordRefreshOutcome(metrics.RefreshRealmOperator, outcome)
	}()

	refreshTokenRaw, err := c.Cookie("refresh_token")
	if err != nil || refreshTokenRaw == "" {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenMissing, "Refresh token missing")
		return
	}

	refreshHash := session.HashToken(refreshTokenRaw)
	sess, err := session.GlobalStore.ValidateRefreshToken(refreshHash)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		outcome = metrics.RefreshOutcomeStoreError
		log.Printf("[RefreshToken] Failed to validate refresh session: %v", err)
		RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || sess == nil {
		// Reuse detection: a token that no longer validates but matches a
		// previously-rotated hash is a replay of an already-rotated refresh token.
		// If that rotation happened just now it is a benign concurrent
		// double-refresh (this request lost the race) — reject without revoking.
		// Only a replay well after the rotation is treated as a stolen-token
		// signal, revoking the whole session family.
		reused, ferr := session.GlobalStore.FindByPreviousRefreshToken(refreshHash)
		if ferr != nil {
			outcome = metrics.RefreshOutcomeStoreError
			log.Printf("[RefreshToken] Failed to inspect refresh-token history: %v", ferr)
			RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
			return
		}
		if reused != nil {
			if session.RecentlyRotated(reused, time.Now()) {
				outcome = metrics.RefreshOutcomeRotated
				RespondWithError(c, http.StatusUnauthorized, ErrCodeRefreshRotated, "Refresh token already rotated")
				return
			}
			revokeReused := session.GlobalStore.RevokeFamilyWithReason
			if demomode.IsSharedSessionProvider(reused.Provider) {
				// Every public-demo visitor shares one identity, so a family
				// revoke would let any visitor sign all the others out by
				// replaying their own stale token. Revoke only this session.
				revokeReused = func(s *session.UserSession, reason session.RevocationReason) error {
					return session.GlobalStore.RevokeWithReason(s.ID, reason)
				}
			}
			if revokeErr := revokeReused(reused, session.RevocationReasonRefreshTokenReuse); revokeErr != nil {
				outcome = metrics.RefreshOutcomeStoreError
				log.Printf("[RefreshToken] Failed to revoke refresh-token reuse family: %v", revokeErr)
				RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to revoke compromised session")
				return
			}
			outcome = metrics.RefreshOutcomeReuse
			RespondWithError(c, http.StatusUnauthorized, ErrCodeSessionUnknown, "Session has been revoked")
			return
		}
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid or expired refresh token")
		return
	}

	// Read the session's Provider field to determine which Generate function to call
	var newToken string
	var tokenErr error

	switch sess.Provider {
	case "email", "google", "register", "demo":
		// "demo" is the public-demo owner (DEMO_MODE only; see
		// verifyPersistedOperatorSession).
		// "register" is a legacy email-registration session provider; it
		// still refreshes through the normal user-token path when a user id is
		// present.
		if sess.UserID != nil && *sess.UserID > 0 {
			var user database.User
			lookupErr := database.GetDB().First(&user, *sess.UserID).Error
			if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				outcome = metrics.RefreshOutcomeStoreError
				log.Printf("[RefreshToken] Failed to load user %d: %v", *sess.UserID, lookupErr)
				RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
				return
			}
			if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonAccountDisabled)
				RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "User not found")
				return
			}
			// P1-11: sessions of a soft-deleted account die at the next
			// refresh even if they were minted before the deletion request.
			if user.DeletedAt != nil {
				_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonAccountDisabled)
				RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "User account is inactive")
				return
			}
			verified, verificationErr := verifyPersistedOperatorSession(sess)
			if verificationErr != nil {
				outcome = metrics.RefreshOutcomeStoreError
				log.Printf("[RefreshToken] Failed to load operator verification state: %v", verificationErr)
				RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
				return
			}
			if !verified {
				if revokeErr := revokeUnverifiedOperatorSession(sess); revokeErr != nil {
					outcome = metrics.RefreshOutcomeStoreError
					log.Printf("[RefreshToken] Failed to revoke unverified operator session: %v", revokeErr)
					RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
					return
				}
				metrics.AuthOperations.WithLabelValues("unverified_operator_refresh_denied").Inc()
				RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Email verification required")
				return
			}
			if user.Picture != "" {
				newToken, tokenErr = GenerateOAuthUserToken(user.ID, user.Email, user.Name, user.Picture, user.Role, sess.ID)
			} else {
				newToken, tokenErr = GenerateUserToken(user.ID, user.Email, user.Address, user.Role, sess.ID)
			}
		} else {
			_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid user session")
			return
		}
	case "web3", "dynamic", "dynamic_web3":
		if sess.UserID != nil && *sess.UserID > 0 {
			var user database.User
			lookupErr := database.GetDB().First(&user, *sess.UserID).Error
			if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				outcome = metrics.RefreshOutcomeStoreError
				log.Printf("[RefreshToken] Failed to load Web3 user %d: %v", *sess.UserID, lookupErr)
				RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
				return
			}
			if errors.Is(lookupErr, gorm.ErrRecordNotFound) || user.DeletedAt != nil {
				_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonAccountDisabled)
				RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "User account is inactive")
				return
			}
			newToken, tokenErr = GenerateWeb3Token(user.Address, structs.Role(user.Role), sess.ID)
		} else if sess.Address != "" {
			// Web3 sessions may not have a UserID, use Address directly
			var user database.User
			lookupErr := database.GetDB().Where("address = ?", sess.Address).First(&user).Error
			if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				outcome = metrics.RefreshOutcomeStoreError
				log.Printf("[RefreshToken] Failed to load Web3 user by address: %v", lookupErr)
				RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
				return
			}
			if errors.Is(lookupErr, gorm.ErrRecordNotFound) || user.DeletedAt != nil {
				_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonAccountDisabled)
				RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "User account is inactive")
				return
			}
			newToken, tokenErr = GenerateWeb3Token(user.Address, structs.Role(user.Role), sess.ID)
		} else {
			_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid user session")
			return
		}
	case "google_staff", "staff_code", "staff_invite", demomode.StaffSessionProvider:
		if sess.UserID != nil && *sess.UserID > 0 {
			var staff database.Staff
			lookupErr := database.GetDB().First(&staff, *sess.UserID).Error
			if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				outcome = metrics.RefreshOutcomeStoreError
				log.Printf("[RefreshToken] Failed to load staff %d: %v", *sess.UserID, lookupErr)
				RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
				return
			}
			if errors.Is(lookupErr, gorm.ErrRecordNotFound) || !staff.IsActive {
				_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonAccountDisabled)
				RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Staff account is inactive")
				return
			}
			verified, verificationErr := verifyPersistedOperatorSession(sess)
			if verificationErr != nil {
				outcome = metrics.RefreshOutcomeStoreError
				log.Printf("[RefreshToken] Failed to load staff session verification state: %v", verificationErr)
				RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
				return
			}
			if !verified {
				// A demo staff session outside DEMO_MODE.
				_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
				RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid staff session")
				return
			}
			newToken, tokenErr = GenerateStaffToken(&staff, sess.ID)
		} else {
			_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid staff session")
			return
		}
	case "customer":
		if sess.UserID != nil && *sess.UserID > 0 {
			var customer database.Customer
			lookupErr := database.GetDB().Select("id", "email", "is_active").First(&customer, *sess.UserID).Error
			if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				outcome = metrics.RefreshOutcomeStoreError
				log.Printf("[RefreshToken] Failed to load customer %d: %v", *sess.UserID, lookupErr)
				RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
				return
			}
			if errors.Is(lookupErr, gorm.ErrRecordNotFound) || !customer.IsActive {
				_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonAccountDisabled)
				RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Customer account is inactive")
				return
			}
			newToken, tokenErr = GenerateCustomerToken(customer.ID, customer.Email, sess.ID)
		} else {
			_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid customer session")
			return
		}
	default:
		_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Unknown session type")
		return
	}

	if tokenErr != nil || newToken == "" {
		outcome = metrics.RefreshOutcomeStoreError
		RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to generate token")
		return
	}

	// Rotate refresh token
	newRefreshToken, err := session.GenerateRefreshToken()
	if err != nil {
		outcome = metrics.RefreshOutcomeStoreError
		log.Printf("[RefreshToken] Failed to generate refresh token: %v", err)
		RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
		return
	}
	newRefreshHash := session.HashToken(newRefreshToken)
	// Atomic compare-and-swap rotation: advances the refresh hash + session-token
	// hash + expiries in ONE transaction keyed on the old refresh hash, and records
	// the old hash as previous_refresh_token for reuse detection. matched=false
	// means a concurrent refresh already rotated this token — this caller lost the
	// race and must not mint a parallel valid token (so we 401 rather than issue
	// cookies pointing at a hash the store no longer recognizes).
	matched, rerr := session.GlobalStore.RotateSession(
		sess.ID, refreshHash, newRefreshHash, session.HashToken(newToken),
		time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	if rerr != nil {
		outcome = metrics.RefreshOutcomeStoreError
		log.Printf("[RefreshToken] Failed to rotate session: %v", rerr)
		RespondWithError(c, http.StatusInternalServerError, ErrCodeTokenInvalid, "Failed to refresh session")
		return
	}
	if !matched {
		outcome = metrics.RefreshOutcomeRotated
		RespondWithError(c, http.StatusUnauthorized, ErrCodeRefreshRotated, "Refresh token already rotated")
		return
	}

	// Set new cookies
	utils.SetSessionCookie(c, "session_token", newToken, 15*60)
	utils.SetSessionCookie(c, "refresh_token", newRefreshToken, 7*24*3600)

	// Set provider-specific cookie
	switch sess.Provider {
	case "google_staff", "staff_code", "staff_invite", demomode.StaffSessionProvider:
		utils.SetSessionCookie(c, "staff_token", newToken, 15*60)
	case "customer":
		utils.SetSessionCookie(c, "customer_token", newToken, 15*60)
	}

	outcome = metrics.RefreshOutcomeSuccess
	c.JSON(http.StatusOK, gin.H{"success": true})
}
