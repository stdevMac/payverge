package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// extractBusinessID tries multiple param names for business ID (defense-in-depth).
func extractBusinessID(c *gin.Context) string {
	if id := c.Param("business_id"); id != "" {
		return id
	}
	if id := c.Param("businessId"); id != "" {
		return id
	}
	if id := c.Param("id"); id != "" && routeIDParamIsBusinessID(c) {
		return id
	}
	return ""
}

func routeIDParamIsBusinessID(c *gin.Context) bool {
	return strings.Contains(c.FullPath(), "/businesses/:id")
}

func hydrateLiveStaffContext(c *gin.Context, claims map[string]interface{}) (*database.Staff, bool) {
	staffID := claimUint(claims, "staff_id")
	if staffID == 0 {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid staff token")
		c.Abort()
		return nil, false
	}

	staff, err := database.GetDBWrapper().StaffService.GetByID(staffID)
	// Inactive staff OR missing row: treat as session-revoked so clients stop
	// retrying with a dead credential (same machine code as version mismatch).
	if err != nil || !staff.IsActive {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeSessionUnknown, "Session has been revoked")
		c.Abort()
		return nil, false
	}

	// Cross-replica access revocation: JWT carries authz_version at issuance;
	// any access change bumps the live column, so the next request on any
	// replica rejects the token without relying on process-local state.
	tokenVersion := claimUint(claims, "authz_version")
	liveVersion := uint(staff.AuthzVersion)
	if liveVersion == 0 {
		liveVersion = 1
	}
	if tokenVersion != liveVersion {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeSessionUnknown, "Session has been revoked")
		c.Abort()
		return nil, false
	}

	// Hydrate context from the live staff record so deactivation and role changes
	// take effect immediately instead of waiting for token expiry.
	c.Set("staff_id", staff.ID)
	c.Set("staff_email", staff.Email)
	c.Set("staff_name", staff.Name)
	c.Set("staff_role", string(staff.Role))
	c.Set("staff_business_id", staff.BusinessID)
	c.Set("staff_authz_version", int(liveVersion))
	c.Set("token_type", "staff")
	// Stash the raw custom_permissions JSON (already loaded on this staff row) so
	// RBAC permission resolution reuses it instead of issuing a second per-request
	// staff SELECT (MW-01). Pure stash — parsing stays in rbac.go.
	c.Set("staff_custom_permissions", staff.CustomPermissions)

	// Load this staff's explicit denies in the SAME hydration step (ONE indexed
	// query by staff_id; denies are typically empty). Stash as []string so
	// checkStaffPermissions / getPermissionDenies issue zero extra queries on
	// the hot path. Always set the key (empty slice = authoritative "no denies").
	denyPerms, err := loadStaffPermissionDenies(staff.ID)
	if err != nil {
		// Explicit denies are a subtraction from otherwise broad role/custom
		// grants. Treating a read failure as an empty set would restore access
		// precisely while the authorization store is unhealthy.
		requestID, _ := c.Get("request_id")
		log.Printf("[Auth] RBAC deny lookup failed closed (staff_id=%d request_id=%v): %v", staff.ID, requestID, err)
		RespondWithError(c, http.StatusServiceUnavailable, ErrCodeInternal, "Authorization state is temporarily unavailable")
		c.Abort()
		return nil, false
	}
	c.Set("staff_permission_denies", denyPerms)

	return staff, true
}

// loadStaffPermissionDenies returns the permission strings denied for staffID.
// Single Pluck by staff_id — never N+1, never joined into the staff row SELECT.
func loadStaffPermissionDenies(staffID uint) ([]string, error) {
	var denies []string
	if err := database.GetDB().Model(&database.StaffPermissionDeny{}).
		Where("staff_id = ?", staffID).
		Pluck("permission", &denies).Error; err != nil {
		return nil, err
	}
	if denies == nil {
		denies = []string{}
	}
	return denies, nil
}

func hydrateLiveCustomerContext(c *gin.Context, claims map[string]interface{}) (*database.Customer, bool) {
	customerID := claimUint(claims, "customer_id")
	if customerID == 0 {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid customer token")
		c.Abort()
		return nil, false
	}

	var customer database.Customer
	if err := database.GetDB().Select("id", "email", "is_active").First(&customer, customerID).Error; err != nil || !customer.IsActive {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Customer account is inactive")
		c.Abort()
		return nil, false
	}

	c.Set("customer_id", customer.ID)
	c.Set("customer_email", customer.Email)
	c.Set("token_type", "customer")

	return &customer, true
}

// validateSession checks the session_id claim in the JWT against the session
// store. Returns true if validation passes. When it returns false it has
// already written an error response.
//
// Production fail-closed: a missing GlobalStore rejects auth so logout and
// family revoke cannot silently no-op. Non-production keeps the historical
// skip so unit tests without a store still work.
func validateSession(c *gin.Context, claims map[string]interface{}, tokenString string) bool {
	if session.GlobalStore == nil {
		if utils.IsProduction() {
			log.Printf("[Auth] Rejected request: session store not configured (path: %s)", middleware.SafeRequestPath(c))
			RespondWithError(c, http.StatusServiceUnavailable, ErrCodeInternal, "Session service unavailable")
			c.Abort()
			return false
		}
		return true
	}
	sessionIDFloat, ok := claims["session_id"].(float64)
	if !ok || sessionIDFloat == 0 {
		log.Printf("[Auth] Rejected legacy token without session_id (path: %s)", middleware.SafeRequestPath(c))
		RespondWithError(c, http.StatusUnauthorized, ErrCodeSessionUnknown, "Invalid session")
		c.Abort()
		return false
	}
	valid, err := session.GlobalStore.Validate(uint(sessionIDFloat), session.HashToken(tokenString))
	if err != nil {
		log.Printf("[Auth] Session validate error for path %s: %v", middleware.SafeRequestPath(c), err)
		RespondWithError(c, http.StatusServiceUnavailable, ErrCodeInternal, "Session service unavailable")
		c.Abort()
		return false
	}
	if !valid {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeSessionUnknown, "Session has been revoked")
		c.Abort()
		return false
	}
	persisted, err := session.GlobalStore.Get(uint(sessionIDFloat))
	if err != nil {
		log.Printf("[Auth] Session lookup error for path %s: %v", middleware.SafeRequestPath(c), err)
		RespondWithError(c, http.StatusServiceUnavailable, ErrCodeInternal, "Session service unavailable")
		c.Abort()
		return false
	}
	verified, err := verifyPersistedOperatorSession(persisted)
	if err != nil {
		log.Printf("[Auth] Verification-state lookup error for path %s: %v", middleware.SafeRequestPath(c), err)
		RespondWithError(c, http.StatusServiceUnavailable, ErrCodeInternal, "Session service unavailable")
		c.Abort()
		return false
	}
	if !verified {
		if err := revokeUnverifiedOperatorSession(persisted); err != nil {
			log.Printf("[Auth] Failed to revoke unverified session %d: %v", persisted.ID, err)
			RespondWithError(c, http.StatusServiceUnavailable, ErrCodeInternal, "Session service unavailable")
			c.Abort()
			return false
		}
		metrics.AuthOperations.WithLabelValues("unverified_operator_access_denied").Inc()
		RespondWithError(c, http.StatusForbidden, ErrCodeForbidden, "Email verification required")
		c.Abort()
		return false
	}
	return true
}

func normalizeTokenString(tokenString string) string {
	if len(tokenString) > 2 && tokenString[0] == '"' && tokenString[len(tokenString)-1] == '"' {
		return tokenString[1 : len(tokenString)-1]
	}
	return tokenString
}

func extractBearerToken(c *gin.Context) string {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if header == "" {
		return ""
	}
	parts, token, ok := strings.Cut(header, " ")
	if !ok || parts != "Bearer" {
		return ""
	}
	return normalizeTokenString(token)
}

func extractTokenFromRequest(c *gin.Context, cookieNames ...string) string {
	if bearer := extractBearerToken(c); bearer != "" {
		return bearer
	}

	for _, cookieName := range cookieNames {
		if cookieName == "" {
			continue
		}
		if value, err := c.Cookie(cookieName); err == nil && strings.TrimSpace(value) != "" {
			return normalizeTokenString(value)
		}
	}

	return ""
}

func extractCookieTokenFromRequest(c *gin.Context, cookieName string) string {
	if value, err := c.Cookie(cookieName); err == nil && strings.TrimSpace(value) != "" {
		return normalizeTokenString(value)
	}
	return ""
}

func adminMCPTokenConfigured() bool {
	return strings.TrimSpace(os.Getenv("PAYVERGE_ADMIN_MCP_TOKEN")) != "" ||
		strings.TrimSpace(os.Getenv("PAYVERGE_ADMIN_MCP_TOKEN_SHA256")) != ""
}

func adminMCPTokenMatches(tokenString string) bool {
	presented := strings.TrimSpace(tokenString)
	if presented == "" || !adminMCPTokenConfigured() {
		return false
	}

	presentedHash := sha256.Sum256([]byte(presented))

	if expectedHashHex := strings.TrimSpace(os.Getenv("PAYVERGE_ADMIN_MCP_TOKEN_SHA256")); expectedHashHex != "" {
		expectedHash, err := hex.DecodeString(expectedHashHex)
		if err == nil && len(expectedHash) == sha256.Size && subtle.ConstantTimeCompare(presentedHash[:], expectedHash) == 1 {
			return true
		}
	}

	if expectedRaw := strings.TrimSpace(os.Getenv("PAYVERGE_ADMIN_MCP_TOKEN")); expectedRaw != "" {
		expectedHash := sha256.Sum256([]byte(expectedRaw))
		return subtle.ConstantTimeCompare(presentedHash[:], expectedHash[:]) == 1
	}

	return false
}

// adminMCPClientIPAllowed checks the client IP the backend resolved
// (c.ClientIP(), which honours X-Forwarded-For only from TRUSTED_PROXIES)
// against PAYVERGE_ADMIN_MCP_ALLOWED_IPS. A loopback client address is only
// believed for a direct local connection: when it arrived through a proxy, or
// with forwarding headers, it is a proxy-local or spoofed address and never
// matches, so a misconfigured proxy cannot turn the default loopback-only
// allowlist into "everyone".
func adminMCPClientIPAllowed(c *gin.Context) bool {
	clientIP := net.ParseIP(c.ClientIP())
	if clientIP == nil {
		return false
	}
	if clientIP.IsLoopback() && !adminMCPDirectLoopback(c) {
		return false
	}

	allowed := strings.TrimSpace(os.Getenv("PAYVERGE_ADMIN_MCP_ALLOWED_IPS"))
	if allowed == "" {
		allowed = "127.0.0.1/32,::1/128"
	}

	for _, entry := range strings.Split(allowed, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		if strings.Contains(entry, "/") {
			if _, ipNet, err := net.ParseCIDR(entry); err == nil && ipNet.Contains(clientIP) {
				return true
			}
			continue
		}

		if allowedIP := net.ParseIP(entry); allowedIP != nil && allowedIP.Equal(clientIP) {
			return true
		}
	}

	return false
}

func adminMCPDirectLoopback(c *gin.Context) bool {
	remote := net.ParseIP(c.RemoteIP())
	if remote == nil || !remote.IsLoopback() {
		return false
	}
	for _, h := range []string{"X-Forwarded-For", "X-Real-IP", "Forwarded", "CF-Connecting-IP", "True-Client-IP"} {
		if strings.TrimSpace(c.GetHeader(h)) != "" {
			return false
		}
	}
	return true
}

func hydrateAdminMCPContext(c *gin.Context) {
	c.Set("user_id", uint(0))
	c.Set("address", "mcp-admin")
	c.Set("role", "admin")
	c.Set("token_type", "admin_mcp")
	c.Set("admin_auth_method", "mcp_token")
}

// adminMCPRouteAllowlist is the routes tools/payverge-admin-mcp calls.
// Keys are c.Request.Method + " " + c.FullPath(). The MCP admin token is a
// synthetic identity (user_id 0) and is refused on every other admin route.
var adminMCPRouteAllowlist = map[string]struct{}{
	"GET /api/v1/admin/system/health":             {},
	"GET /api/v1/admin/fiscal/summary":            {},
	"GET /api/v1/admin/fiscal/jobs":               {},
	"POST /api/v1/admin/fiscal/jobs/:id/requeue":  {},
	"GET /api/v1/admin/errors":                    {},
	"GET /api/v1/admin/webhooks/failed":           {},
	"POST /api/v1/admin/webhooks/:id/acknowledge": {},
	"GET /api/v1/admin/businesses/:id/detail":     {},
}

func adminMCPRouteAllowed(c *gin.Context) bool {
	_, ok := adminMCPRouteAllowlist[c.Request.Method+" "+c.FullPath()]
	return ok
}

// respondTokenError sends the appropriate error code for token verification failures,
// distinguishing expired tokens from invalid ones.
func respondTokenError(c *gin.Context, err error) {
	if IsTokenExpiredError(err) {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenExpired, "Authentication token has expired")
	} else {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid authentication token")
	}
	c.Abort()
}

type authenticationTokenClaims struct {
	claims     map[string]interface{}
	userID     interface{}
	address    interface{}
	picture    interface{}
	hasPicture bool
}

func verifyAuthenticationToken(tokenString string) (authenticationTokenClaims, error) {
	if session.GlobalStore == nil {
		if claims, ok := cachedVerifiedTokenReadOnly(tokenString); ok {
			if claims["type"] != "user" && claims["type"] != "web3" {
				return authenticationTokenClaims{}, fmt.Errorf("invalid token type")
			}
			return authenticationClaimsFromMap(claims), nil
		}
	}

	claims, err := VerifyUserToken(tokenString)
	if err != nil {
		return authenticationTokenClaims{}, err
	}
	return authenticationClaimsFromMap(claims), nil
}

func authenticationClaimsFromMap(claims map[string]interface{}) authenticationTokenClaims {
	result := authenticationTokenClaims{
		claims:  claims,
		userID:  claims["user_id"],
		address: claims["address"],
	}
	if picture, ok := claims["picture"]; ok {
		result.picture = picture
		result.hasPicture = true
	}
	return result
}

// AuthenticationMiddleware checks if the user has a valid JWT token
func AuthenticationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store") // sensitive authenticated data — see HybridAuthenticationMiddleware
		tokenString := extractTokenFromRequest(c, "session_token")
		if tokenString == "" {
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenMissing, AuthTokenMissingMessage(c))
			c.Abort()
			return
		}
		// Reject staff/customer tokens at the edge — they're signed with the
		// same secret and would otherwise be treated as user sessions
		// downstream (claims["user_id"] would be nil, which some handlers
		// may silently tolerate). See jwt.VerifyUserToken.
		claims, err := verifyAuthenticationToken(tokenString)
		if err != nil {
			respondTokenError(c, err)
			return
		}

		if session.GlobalStore != nil && !validateSession(c, claims.claims, tokenString) {
			return
		}

		c.Set("user_id", claims.userID)
		c.Set("address", claims.address)
		if claims.hasPicture {
			c.Set("picture", claims.picture)
		}
		c.Next()
	}
}

// AuthenticationAdminMiddleware checks if the user has a valid JWT token and if is an admin
func AuthenticationAdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store") // sensitive authenticated data — see HybridAuthenticationMiddleware
		tokenString := extractTokenFromRequest(c, "session_token")
		if tokenString == "" {
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenMissing, AuthTokenMissingMessage(c))
			c.Abort()
			return
		}

		if adminMCPTokenMatches(tokenString) {
			if !adminMCPClientIPAllowed(c) {
				RespondWithError(c, http.StatusForbidden, ErrCodeNotAdmin, "MCP admin token is not allowed from this IP")
				c.Abort()
				return
			}
			if !adminMCPRouteAllowed(c) {
				RespondWithError(c, http.StatusForbidden, ErrCodeNotAdmin, "MCP admin token is not allowed on this route")
				c.Abort()
				return
			}

			hydrateAdminMCPContext(c)
			c.Next()
			return
		}

		// Admin endpoints only accept user-type tokens. A staff token whose
		// associated staff row happened to carry role=admin must not be able
		// to pivot here — staff flows go through AuthenticationStaffMiddleware.
		claims, err := VerifyUserToken(tokenString)
		if err != nil {
			respondTokenError(c, err)
			return
		}

		if !validateSession(c, claims, tokenString) {
			return
		}

		// Live DB revalidation: JWT role claims can lag demotion until access
		// token expiry. Staff hydration already reloads authz from DB; admins
		// must do the same against users.role.
		userID := claimUint(claims, "user_id")
		if userID == 0 {
			RespondWithError(c, http.StatusForbidden, ErrCodeNotAdmin, "forbidden")
			c.Abort()
			return
		}
		var liveUser database.User
		if err := database.GetDB().Select("id", "role", "address").First(&liveUser, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				RespondWithError(c, http.StatusForbidden, ErrCodeNotAdmin, "forbidden")
				c.Abort()
				return
			}
			log.Printf("[Auth] Admin live-role lookup failed (user_id=%d path=%s): %v", userID, middleware.SafeRequestPath(c), err)
			RespondWithError(c, http.StatusServiceUnavailable, ErrCodeInternal, "Authorization state is temporarily unavailable")
			c.Abort()
			return
		}
		if liveUser.Role != "admin" {
			RespondWithError(c, http.StatusForbidden, ErrCodeNotAdmin, "forbidden")
			c.Abort()
			return
		}

		c.Set("user_id", liveUser.ID)
		c.Set("address", liveUser.Address)
		c.Set("role", liveUser.Role)
		c.Set("token_type", "user")
		c.Next()
	}
}

// StaffAuthenticationMiddleware checks if the user has a valid staff JWT token
func StaffAuthenticationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store") // sensitive authenticated data — see HybridAuthenticationMiddleware
		tokenString := extractTokenFromRequest(c, "staff_token")
		if tokenString == "" {
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenMissing, AuthTokenMissingMessage(c))
			c.Abort()
			return
		}
		claims, err := VerifyStaffToken(tokenString)
		if err != nil {
			respondTokenError(c, err)
			return
		}

		if !validateSession(c, claims, tokenString) {
			return
		}

		if _, ok := hydrateLiveStaffContext(c, claims); !ok {
			return
		}

		c.Next()
	}
}

// CustomerAuthenticationMiddleware verifies customer JWT auth for CRM self-service routes.
func CustomerAuthenticationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := extractTokenFromRequest(c, "customer_token")
		if tokenString == "" {
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenMissing, "Missing customer authentication token")
			c.Abort()
			return
		}

		claims, err := VerifyCustomerToken(tokenString)
		if err != nil {
			respondTokenError(c, err)
			return
		}

		if !validateSession(c, claims, tokenString) {
			return
		}

		if _, ok := hydrateLiveCustomerContext(c, claims); !ok {
			return
		}
		c.Next()
	}
}

func OptionalCustomerFromRequest(c *gin.Context) (*database.Customer, bool) {
	tokenString := extractCookieTokenFromRequest(c, "customer_token")
	if strings.TrimSpace(tokenString) == "" {
		return nil, false
	}

	claims, err := VerifyCustomerToken(tokenString)
	if err != nil {
		return nil, false
	}

	customerID := claimUint(claims, "customer_id")
	if customerID == 0 {
		return nil, false
	}

	if session.GlobalStore != nil {
		sessionID := claimUint(claims, "session_id")
		if sessionID == 0 {
			return nil, false
		}
		valid, _ := session.GlobalStore.Validate(sessionID, session.HashToken(tokenString))
		if !valid {
			return nil, false
		}
	}

	var customer database.Customer
	if err := database.GetDB().
		Select("id", "email", "name", "phone", "wallet_address", "is_active").
		Where("id = ? AND is_active = ?", customerID, true).
		First(&customer).Error; err != nil {
		return nil, false
	}

	return &customer, true
}

// HybridAuthenticationMiddleware checks for both Web3 and Staff authentication
func sessionTokenCurrentlyValid(claims map[string]interface{}, tokenString string) bool {
	if session.GlobalStore == nil {
		return !utils.IsProduction()
	}
	sessionIDFloat, ok := claims["session_id"].(float64)
	if !ok || sessionIDFloat == 0 {
		return false
	}
	valid, err := session.GlobalStore.Validate(uint(sessionIDFloat), session.HashToken(tokenString))
	return err == nil && valid
}

func applyHybridStaffContext(c *gin.Context, claims map[string]interface{}) bool {
	staff, ok := hydrateLiveStaffContext(c, claims)
	if !ok {
		return true
	}
	if businessIDStr := extractBusinessID(c); businessIDStr != "" {
		business, err := database.GetBusinessAuthScopeByIdOrBusinessId(businessIDStr)
		if err != nil {
			RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Business not found")
			c.Abort()
			return true
		}
		c.Set("_business", business)
		if staff.BusinessID != business.ID {
			RespondWithError(c, http.StatusForbidden, ErrCodeStaffNoAccess, "Access denied: staff member does not belong to this business")
			c.Abort()
			return true
		}
	}
	c.Next()
	return true
}

func applyHybridUserOrWeb3Context(c *gin.Context, claims map[string]interface{}) bool {
	tokenType, _ := claims["type"].(string)
	if tokenType == "web3" {
		c.Set("address", claims["address"])
		c.Set("token_type", "web3")
		if businessIDStr := extractBusinessID(c); businessIDStr != "" {
			business, err := database.GetBusinessAuthScopeByIdOrBusinessId(businessIDStr)
			if err != nil {
				RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Business not found")
				c.Abort()
				return true
			}
			c.Set("_business", business)
			if _, ok := claims["address"].(string); !ok {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid address in token"})
				c.Abort()
				return true
			}
			if !denyUnlessListedBusinessAccess(c, business) {
				return true
			}
		}
		c.Next()
		return true
	}
	if tokenType == "user" {
		role, ok := liveHybridUserRole(c, claims)
		if !ok {
			return true
		}
		c.Set("user_id", claims["user_id"])
		c.Set("email", claims["email"])
		c.Set("role", role)
		c.Set("token_type", "user")
		if address, ok := claims["address"].(string); ok && address != "" {
			c.Set("address", address)
			c.Set("wallet_address", address)
		}
		if businessIDStr := extractBusinessID(c); businessIDStr != "" {
			business, err := database.GetBusinessAuthScopeByIdOrBusinessId(businessIDStr)
			if err != nil {
				RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Business not found")
				c.Abort()
				return true
			}
			c.Set("_business", business)
			if _, ok := extractContextUint(claims["user_id"]); !ok {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user_id in token"})
				c.Abort()
				return true
			}
			if !denyUnlessListedBusinessAccess(c, business) {
				return true
			}
		}
		c.Next()
		return true
	}
	return false
}

func tryHybridStaffCookie(c *gin.Context) bool {
	token := extractCookieTokenFromRequest(c, "staff_token")
	if token == "" {
		return false
	}
	claims, err := VerifyStaffToken(token)
	if err != nil || !sessionTokenCurrentlyValid(claims, token) {
		return false
	}
	staffID := claimUint(claims, "staff_id")
	if staffID == 0 {
		return false
	}
	staff, err := database.GetDBWrapper().StaffService.GetByID(staffID)
	if err != nil || staff == nil || !staff.IsActive {
		return false
	}
	tokenVersion := claimUint(claims, "authz_version")
	liveVersion := uint(staff.AuthzVersion)
	if liveVersion == 0 {
		liveVersion = 1
	}
	if tokenVersion != liveVersion {
		return false
	}
	return applyHybridStaffContext(c, claims)
}

func tryHybridSessionCookie(c *gin.Context) bool {
	token := extractCookieTokenFromRequest(c, "session_token")
	if token == "" {
		return false
	}
	claims, err := VerifyToken(token)
	if err != nil {
		return false
	}
	if !validateSession(c, claims, token) {
		return true
	}
	return applyHybridUserOrWeb3Context(c, claims)
}

func HybridAuthenticationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Authenticated responses carry per-user/business sensitive data (bills,
		// payments, profile, wallets). no-store keeps it out of the browser disk
		// cache + shared/intermediary caches so it isn't retrievable post-logout
		// on a shared device. Defense-in-depth: if Caddy also sets Cache-Control,
		// no-store is the safe most-restrictive value. SSE handlers override to
		// no-cache as needed (c.Header replaces). The FE caches in React Query
		// (in-memory), not the HTTP cache, so this changes no app behavior.
		c.Header("Cache-Control", "no-store")
		// Cookie paths must try staff and session independently. A leftover
		// revoked staff_token (common after COOKIE_DOMAIN remaps / mixed
		// staff+email logins) used to 401 AUTH_SESSION_UNKNOWN and never
		// consult a live session_token — Reservations/Tables then bounced
		// the operator to Authentication Required.
		if extractBearerToken(c) == "" {
			if handled := tryHybridStaffCookie(c); handled {
				return
			}
			if handled := tryHybridSessionCookie(c); handled {
				return
			}
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenMissing, AuthTokenMissingMessage(c))
			c.Abort()
			return
		}

		tokenString := extractTokenFromRequest(c, "staff_token", "session_token")
		if tokenString == "" {
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenMissing, AuthTokenMissingMessage(c))
			c.Abort()
			return
		}

		// Try to verify as staff token first
		if claims, err := VerifyStaffToken(tokenString); err == nil {
			if !validateSession(c, claims, tokenString) {
				return
			}
			staff, ok := hydrateLiveStaffContext(c, claims)
			if !ok {
				return
			}

			// For staff users, validate business access if business ID is in the URL
			if businessIDStr := extractBusinessID(c); businessIDStr != "" {
				// Get business by ID or business_id string
				business, err := database.GetBusinessAuthScopeByIdOrBusinessId(businessIDStr)
				if err != nil {
					RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Business not found")
					c.Abort()
					return
				}
				c.Set("_business", business)

				// Check if staff member belongs to the requested business
				if staff.BusinessID != business.ID {
					RespondWithError(c, http.StatusForbidden, ErrCodeStaffNoAccess, "Access denied: staff member does not belong to this business")
					c.Abort()
					return
				}
			}

			c.Next()
			return
		}

		// Try to verify as Web3 token or OAuth user token
		if claims, err := VerifyToken(tokenString); err == nil {
			if !validateSession(c, claims, tokenString) {
				return
			}
			tokenType, _ := claims["type"].(string)

			// Handle Web3 tokens
			if tokenType == "web3" {
				c.Set("address", claims["address"])
				c.Set("token_type", "web3")

				// For Web3 users, validate business ownership if business ID is in the URL
				if businessIDStr := extractBusinessID(c); businessIDStr != "" {
					// Get business by ID or business_id string
					business, err := database.GetBusinessAuthScopeByIdOrBusinessId(businessIDStr)
					if err != nil {
						RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Business not found")
						c.Abort()
						return
					}
					c.Set("_business", business)

					if _, ok := claims["address"].(string); !ok {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid address in token"})
						c.Abort()
						return
					}
					if !denyUnlessListedBusinessAccess(c, business) {
						return
					}
				}

				c.Next()
				return
			}

			// Handle OAuth user tokens (email/password, Google)
			if tokenType == "user" {
				role, ok := liveHybridUserRole(c, claims)
				if !ok {
					return
				}
				c.Set("user_id", claims["user_id"])
				c.Set("email", claims["email"])
				c.Set("role", role)
				c.Set("token_type", "user")
				if address, ok := claims["address"].(string); ok && address != "" {
					c.Set("address", address)
					c.Set("wallet_address", address)
				}

				// For OAuth users, validate business ownership if business ID is in the URL
				if businessIDStr := extractBusinessID(c); businessIDStr != "" {
					// Get business by ID or business_id string
					business, err := database.GetBusinessAuthScopeByIdOrBusinessId(businessIDStr)
					if err != nil {
						RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Business not found")
						c.Abort()
						return
					}
					c.Set("_business", business)

					if _, ok := extractContextUint(claims["user_id"]); !ok {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user_id in token"})
						c.Abort()
						return
					}
					if !denyUnlessListedBusinessAccess(c, business) {
						return
					}
				}

				c.Next()
				return
			}
		}

		// If we get here, neither token type worked
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid authentication token")
		c.Abort()
	}
}

// RoleBasedAccessMiddleware gates a route on every listed permission (see
// RequirePermissions in rbac.go).
func RoleBasedAccessMiddleware(requiredPermissions ...string) gin.HandlerFunc {
	// Use the comprehensive RBAC system from rbac.go
	return RequirePermissions(requiredPermissions...)
}

// RoleBasedAnyAccessMiddleware protects mixed-resource endpoints where any one
// of the candidate permissions permits entry. The handler must still filter
// individual resources using ContextAuthorizedAnyPermissions.
func RoleBasedAnyAccessMiddleware(candidatePermissions ...string) gin.HandlerFunc {
	return RequireAnyPermissions(candidatePermissions...)
}

func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		status := fmt.Sprintf("%d", c.Writer.Status())
		metrics.TotalRequests.WithLabelValues(c.FullPath(), c.Request.Method, status).Inc()
	}
}
