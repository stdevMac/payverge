package utils

import "github.com/gin-gonic/gin"

// ClearAllAuthCookies clears every auth cookie this app sets, regardless of
// which login flow created them. The SameSite attribute on clear must match the
// one used at set time, otherwise browsers may keep the original cookie.
func ClearAllAuthCookies(c *gin.Context) {
	ClearSessionCookie(c, "session_token")
	ClearSessionCookie(c, "staff_token")
	ClearSessionCookie(c, "customer_token")
	// oauth_state binds Google OAuth CSRF state to the browser (SEC-1); clear on logout.
	ClearSessionCookie(c, "oauth_state")
	// Operator refresh is SameSite=Lax (must travel on example.com → api
	// XHR). Also emit the Strict twin clear so a pre-fix cookie is evicted.
	ClearSessionCookie(c, "refresh_token")
	ClearStrictSessionCookie(c, "refresh_token")
	ClearStrictSessionCookie(c, "customer_refresh_token")
}
