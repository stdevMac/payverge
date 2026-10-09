package server

import (
	"errors"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

// RevokeSessionsAfterRoleChange revokes every live operator-class session of
// a user whose users.role changed (for example a platform-admin demotion), so
// tokens minted with the old role cannot be used or refreshed. Callers that
// change users.role (admin tooling, CLI subcommands) should call it after the
// role update commits. It is a no-op when no session store is configured.
func RevokeSessionsAfterRoleChange(userID uint, address string) error {
	if session.GlobalStore == nil || userID == 0 {
		return nil
	}
	var addresses []string
	if address != "" {
		addresses = []string{address}
	}
	return session.GlobalStore.RevokeAllLinkedOperatorSessions(userID, addresses, session.RevocationReasonRoleChanged)
}

// liveUserIsPlatformAdmin reports whether users.role is currently admin.
// Lookup errors and unknown users report false (fail closed).
func liveUserIsPlatformAdmin(userID uint) bool {
	if userID == 0 || database.GetDB() == nil {
		return false
	}
	var live database.User
	if err := database.GetDB().Select("id", "role").First(&live, userID).Error; err != nil {
		return false
	}
	return live.Role == string(structs.RoleAdmin)
}

// liveHybridUserRole decides which role HybridAuthenticationMiddleware may
// expose for a user-type token (M-role).
//
// A non-admin role claim grants no platform power, so it is passed through
// without a database read (the common, hot path). An admin claim is
// re-checked against users.role on every request: if the user is no longer
// an admin, every session of that user is revoked (reason role_changed) and
// the request is rejected with 401 so the client re-authenticates and gets a
// token minted from the live role. A lookup failure fails closed with 503,
// matching AuthenticationAdminMiddleware.
//
// It returns the role value to store under the "role" context key and false
// when it already aborted the request.
func liveHybridUserRole(c *gin.Context, claims map[string]interface{}) (interface{}, bool) {
	claimRole, _ := claims["role"].(string)
	if claimRole != string(structs.RoleAdmin) {
		return claims["role"], true
	}

	userID := claimUint(claims, "user_id")
	if userID == 0 {
		// An admin claim without a user id cannot be confirmed; drop it.
		return string(structs.RoleUser), true
	}

	var live database.User
	err := database.GetDB().Select("id", "role", "address").First(&live, userID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("[Auth] Hybrid live-role lookup failed (user_id=%d path=%s): %v", userID, middleware.SafeRequestPath(c), err)
		RespondWithError(c, http.StatusServiceUnavailable, ErrCodeInternal, "Authorization state is temporarily unavailable")
		c.Abort()
		return nil, false
	}
	if err == nil && live.Role == string(structs.RoleAdmin) {
		return live.Role, true
	}

	address, _ := claims["address"].(string)
	if address == "" {
		address = live.Address
	}
	if revokeErr := RevokeSessionsAfterRoleChange(userID, address); revokeErr != nil {
		log.Printf("[Auth] Failed to revoke sessions after role change (user_id=%d): %v", userID, revokeErr)
	}
	log.Printf("[Auth] Rejected stale admin role claim (user_id=%d path=%s live_role=%q)", userID, middleware.SafeRequestPath(c), live.Role)
	RespondWithError(c, http.StatusUnauthorized, ErrCodeSessionUnknown, "Session has been revoked")
	c.Abort()
	return nil, false
}
