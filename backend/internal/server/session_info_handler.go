package server

import (
	"net/http"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"

	"github.com/gin-gonic/gin"
)

// SessionInfoResponse describes the current user's authentication state.
type SessionInfoResponse struct {
	Authenticated bool   `json:"authenticated"`
	Type          string `json:"type,omitempty"`
	UserID        uint   `json:"user_id,omitempty"`
	Email         string `json:"email,omitempty"`
	Address       string `json:"address,omitempty"`
	Role          string `json:"role,omitempty"`
	Picture       string `json:"picture,omitempty"`
	StaffID       uint   `json:"staff_id,omitempty"`
	BusinessID    uint   `json:"business_id,omitempty"`
	BusinessName  string `json:"business_name,omitempty"`
	BusinessSlug  string `json:"business_slug,omitempty"`
	StaffName     string `json:"staff_name,omitempty"`
	CustomerID    uint   `json:"customer_id,omitempty"`
	// EmailVerified reflects whether the user's email/password auth is verified.
	// It drives the dashboard "verify your email" banner. OAuth/web3 users have
	// no email/password row, so their email is treated as provider-verified.
	// No omitempty: the frontend must distinguish an explicit false (show the
	// banner) from a field that simply doesn't apply.
	EmailVerified bool `json:"email_verified"`
}

// GetSessionInfo reads httpOnly cookies server-side and returns the caller's
// auth state so the frontend never needs to decode JWTs in the browser.
func GetSessionInfo(c *gin.Context) {
	if bearer := extractBearerToken(c); bearer != "" {
		if resp, ok := sessionInfoFromToken(bearer); ok {
			c.JSON(http.StatusOK, resp)
			return
		}
		c.JSON(http.StatusOK, SessionInfoResponse{Authenticated: false})
		return
	}

	// Try cookies independently. A leftover revoked staff_token must not
	// hide a live session_token — that combination 401'd /auth/me after
	// email login and bounced Reservations/Tables to the sign-in gate.
	for _, name := range []string{"staff_token", "session_token", "customer_token"} {
		token := extractCookieTokenFromRequest(c, name)
		if token == "" {
			continue
		}
		if resp, ok := sessionInfoFromToken(token); ok {
			c.JSON(http.StatusOK, resp)
			return
		}
	}
	c.JSON(http.StatusOK, SessionInfoResponse{Authenticated: false})
}

func sessionInfoFromToken(tokenString string) (SessionInfoResponse, bool) {
	claims, err := VerifyToken(tokenString)
	if err != nil {
		return SessionInfoResponse{}, false
	}
	if session.GlobalStore != nil {
		if sidFloat, ok := claims["session_id"].(float64); ok && sidFloat != 0 {
			valid, _ := session.GlobalStore.Validate(uint(sidFloat), session.HashToken(tokenString))
			if !valid {
				return SessionInfoResponse{}, false
			}
		}
	}

	tokenType, _ := claims["type"].(string)
	switch tokenType {
	case "staff":
		staffID := claimUint(claims, "staff_id")
		if staffID == 0 {
			return SessionInfoResponse{}, false
		}

		staff, err := database.GetDBWrapper().StaffService.GetByID(staffID)
		if err != nil || !staff.IsActive {
			return SessionInfoResponse{}, false
		}

		resp := SessionInfoResponse{
			Authenticated: true,
			Type:          "staff",
			Email:         staff.Email,
			StaffName:     staff.Name,
			Role:          string(staff.Role),
			StaffID:       staff.ID,
			BusinessID:    staff.BusinessID,
		}
		// Best-effort lookup of the business name. If the row is missing or
		// the DB errors, the response stays valid — the frontend already has
		// a "Business N" fallback for that case.
		if business, err := database.GetBusinessByID(staff.BusinessID); err == nil && business != nil {
			resp.BusinessName = business.Name
			resp.BusinessSlug = business.BusinessId
		}
		return resp, true

	case "customer":
		customerID := claimUint(claims, "customer_id")
		if customerID == 0 {
			return SessionInfoResponse{}, false
		}

		var customer database.Customer
		if err := database.GetDB().Select("id", "email", "is_active").First(&customer, customerID).Error; err != nil || !customer.IsActive {
			return SessionInfoResponse{}, false
		}

		return SessionInfoResponse{
			Authenticated: true,
			Type:          "customer",
			CustomerID:    customer.ID,
			Email:         customer.Email,
		}, true

	case "user":
		userID := claimUint(claims, "user_id")
		resp := SessionInfoResponse{
			Authenticated: true,
			Type:          "user",
			UserID:        userID,
			Email:         claimString(claims, "email"),
			Address:       claimString(claims, "address"),
			Role:          claimString(claims, "role"),
			Picture:       claimString(claims, "picture"),
			// Default: OAuth/web3 emails are provider-verified. Overridden below
			// only when an email/password auth row exists and is unverified.
			EmailVerified: true,
		}
		// With EMAIL_VERIFICATION=off nothing is pending: the stored flag stays
		// false (no proof of ownership was made) but no verification can be
		// completed or is required, so the client must not prompt for one.
		if userID != 0 && config.EmailVerificationRequired() {
			// One narrow, indexed (user_id) lookup on the email/password auth row;
			// verification happens async so we read live state, not a JWT claim.
			var rows []struct{ EmailVerified bool }
			if err := database.GetDB().Table("user_auths").
				Where("user_id = ? AND provider = ?", userID, "email").
				Select("email_verified").
				Limit(1).
				Scan(&rows).Error; err == nil && len(rows) > 0 {
				resp.EmailVerified = rows[0].EmailVerified
			}
		}
		return resp, true

	case "web3":
		return SessionInfoResponse{
			Authenticated: true,
			Type:          "web3",
			Address:       claimString(claims, "address"),
			Role:          claimString(claims, "role"),
		}, true

	default:
		return SessionInfoResponse{}, false
	}
}

// claimString safely extracts a string value from JWT MapClaims.
func claimString(claims map[string]interface{}, key string) string {
	v, _ := claims[key].(string)
	return v
}

// claimUint safely extracts a uint value from JWT MapClaims (stored as float64).
func claimUint(claims map[string]interface{}, key string) uint {
	f, _ := claims[key].(float64)
	return uint(f)
}
