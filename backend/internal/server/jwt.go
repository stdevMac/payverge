package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/golang-jwt/jwt/v4"
)

type verifiedTokenCacheEntry struct {
	claims jwt.MapClaims
	exp    int64
}

var (
	verifiedTokenCacheMu sync.RWMutex
	verifiedTokenCache   = make(map[string]verifiedTokenCacheEntry)
)

// GenerateToken generates a JWT token with the user ID as part of the claims
// Deprecated: Use GenerateWeb3Token or GenerateUserToken instead for consistent claims
func GenerateToken(address string, role structs.Role, sessionID ...uint) (string, error) {
	sid := uint(0)
	if len(sessionID) > 0 {
		sid = sessionID[0]
	}
	return GenerateWeb3Token(address, role, sid)
}

// GenerateWeb3Token generates a JWT token for Web3 users
func GenerateWeb3Token(address string, role structs.Role, sessionID ...uint) (string, error) {
	sid := uint(0)
	if len(sessionID) > 0 {
		sid = sessionID[0]
	}
	claims := jwt.MapClaims{
		"address":    address,
		"role":       role,
		"type":       "web3", // Distinguish from staff tokens
		"session_id": sid,
		"exp":        time.Now().Add(15 * time.Minute).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(structs.GetSecretKey())
}

// GenerateUserToken generates a unified JWT token for any user (Email or Web3)
func GenerateUserToken(userID uint, email string, address string, role string, sessionID ...uint) (string, error) {
	sid := uint(0)
	if len(sessionID) > 0 {
		sid = sessionID[0]
	}
	claims := jwt.MapClaims{
		"user_id":    userID,
		"email":      email,
		"address":    address,
		"role":       role,
		"type":       "user",
		"session_id": sid,
		"exp":        time.Now().Add(15 * time.Minute).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(structs.GetSecretKey())
}

// GenerateOAuthUserToken generates a JWT token for OAuth users with additional profile info
func GenerateOAuthUserToken(userID uint, email string, name string, picture string, role string, sessionID ...uint) (string, error) {
	sid := uint(0)
	if len(sessionID) > 0 {
		sid = sessionID[0]
	}
	claims := jwt.MapClaims{
		"user_id":    userID,
		"email":      email,
		"name":       name,
		"picture":    picture,
		"role":       role,
		"type":       "user",
		"session_id": sid,
		"exp":        time.Now().Add(15 * time.Minute).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(structs.GetSecretKey())
}

// GenerateStaffToken generates a JWT token for staff members
func GenerateStaffToken(staff *database.Staff, sessionID ...uint) (string, error) {
	sid := uint(0)
	if len(sessionID) > 0 {
		sid = sessionID[0]
	}
	// AuthzVersion defaults to 1 when unset (pre-migration rows / zero value).
	authzVersion := staff.AuthzVersion
	if authzVersion <= 0 {
		authzVersion = 1
	}
	// Create the claims
	claims := jwt.MapClaims{
		"staff_id":      staff.ID,
		"email":         staff.Email,
		"name":          staff.Name,
		"role":          staff.Role,
		"business_id":   staff.BusinessID,
		"type":          "staff", // Distinguish from web3 tokens
		"session_id":    sid,
		"authz_version": authzVersion, // live check in hydrateLiveStaffContext
		"exp":           time.Now().Add(15 * time.Minute).Unix(),
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token
	return token.SignedString(structs.GetSecretKey())
}

// GenerateStaffMembershipSelectionToken proves that an email identity has
// completed its login-code challenge without yet granting access to any
// business. It is intentionally short-lived and contains no role/business
// claims; VerifyLoginCode exchanges it for one membership-scoped staff token.
func GenerateStaffMembershipSelectionToken(email string) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate selection token id: %w", err)
	}
	jti := hex.EncodeToString(raw[:])
	expiresAt := time.Now().Add(5 * time.Minute)
	claims := jwt.MapClaims{
		"email": strings.ToLower(strings.TrimSpace(email)),
		"type":  "staff_membership_selection",
		"jti":   jti,
		"exp":   expiresAt.Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(structs.GetSecretKey())
	if err != nil {
		return "", err
	}
	if err := database.RecordStaffMembershipSelectionToken(hashSelectionTokenJTI(jti), expiresAt); err != nil {
		return "", fmt.Errorf("record selection token: %w", err)
	}
	return signed, nil
}

func hashSelectionTokenJTI(jti string) string {
	sum := sha256.Sum256([]byte(jti))
	return hex.EncodeToString(sum[:])
}

// parseStaffMembershipSelectionToken verifies a selection token's signature
// and returns its email plus the hash of its jti. The caller must redeem the
// jti hash with database.ConsumeStaffMembershipSelectionToken before it
// issues a session, which makes each token single-use.
func parseStaffMembershipSelectionToken(raw string) (email string, jtiHash string, err error) {
	token, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return structs.GetSecretKey(), nil
	})
	if err != nil || token == nil || !token.Valid {
		return "", "", fmt.Errorf("invalid membership selection token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["type"] != "staff_membership_selection" {
		return "", "", fmt.Errorf("invalid membership selection token")
	}
	email, _ = claims["email"].(string)
	jti, _ := claims["jti"].(string)
	if strings.TrimSpace(email) == "" || strings.TrimSpace(jti) == "" {
		return "", "", fmt.Errorf("invalid membership selection token")
	}
	return strings.ToLower(strings.TrimSpace(email)), hashSelectionTokenJTI(jti), nil
}

// GenerateCustomerToken generates a JWT token for CRM customer accounts.
func GenerateCustomerToken(customerID uint, email string, sessionID ...uint) (string, error) {
	sid := uint(0)
	if len(sessionID) > 0 {
		sid = sessionID[0]
	}
	claims := jwt.MapClaims{
		"customer_id": customerID,
		"email":       email,
		"type":        "customer",
		"session_id":  sid,
		"exp":         time.Now().Add(15 * time.Minute).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(structs.GetSecretKey())
}

// IsTokenExpiredError checks if a JWT verification error is specifically an expiry error.
func IsTokenExpiredError(err error) bool {
	if ve, ok := err.(*jwt.ValidationError); ok {
		return ve.Errors&jwt.ValidationErrorExpired != 0
	}
	return false
}

// VerifyToken verifies a token JWT validate
func VerifyToken(tokenString string) (jwt.MapClaims, error) {
	if session.GlobalStore == nil {
		if claims, ok := cachedVerifiedToken(tokenString); ok {
			return claims, nil
		}
	}

	// Parse the token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return structs.GetSecretKey(), nil
	})

	// Check if there was an error parsing the token
	if err != nil {
		return nil, err
	}

	// Check if the token is valid
	if !token.Valid {
		fmt.Println("Token is invalid")
		return nil, jwt.ErrSignatureInvalid
	}

	claims := cloneMapClaims(token.Claims.(jwt.MapClaims))
	if session.GlobalStore == nil {
		cacheVerifiedToken(tokenString, claims)
	}

	// Return the claims
	return claims, nil
}

func cachedVerifiedToken(tokenString string) (jwt.MapClaims, bool) {
	claims, ok := cachedVerifiedTokenReadOnly(tokenString)
	if !ok {
		return nil, false
	}
	return cloneMapClaims(claims), true
}

func cachedVerifiedTokenReadOnly(tokenString string) (jwt.MapClaims, bool) {
	verifiedTokenCacheMu.RLock()
	entry, ok := verifiedTokenCache[tokenString]
	verifiedTokenCacheMu.RUnlock()
	if !ok {
		return nil, false
	}
	if entry.exp > 0 && time.Now().Unix() >= entry.exp {
		verifiedTokenCacheMu.Lock()
		delete(verifiedTokenCache, tokenString)
		verifiedTokenCacheMu.Unlock()
		return nil, false
	}
	return entry.claims, true
}

func cacheVerifiedToken(tokenString string, claims jwt.MapClaims) {
	exp, ok := claimExpirationUnix(claims)
	if !ok || exp == 0 {
		return
	}
	verifiedTokenCacheMu.Lock()
	verifiedTokenCache[tokenString] = verifiedTokenCacheEntry{claims: cloneMapClaims(claims), exp: exp}
	verifiedTokenCacheMu.Unlock()
}

func cloneMapClaims(claims jwt.MapClaims) jwt.MapClaims {
	if claims == nil {
		return nil
	}
	cloned := make(jwt.MapClaims, len(claims))
	for key, value := range claims {
		cloned[key] = value
	}
	return cloned
}

func claimExpirationUnix(claims jwt.MapClaims) (int64, bool) {
	raw, ok := claims["exp"]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	default:
		return 0, false
	}
}

// VerifyUserToken verifies the JWT and asserts the `type` claim is one of
// the user-facing kinds (user, web3). Staff and customer tokens are signed
// with the same secret; without a type check, either could satisfy a
// user-scoped middleware. Use this helper for AuthenticationMiddleware and
// friends so token-type confusion can't slip through.
func VerifyUserToken(tokenString string) (jwt.MapClaims, error) {
	claims, err := VerifyToken(tokenString)
	if err != nil {
		return nil, err
	}
	switch claims["type"] {
	case "user", "web3":
		return claims, nil
	default:
		return nil, fmt.Errorf("invalid token type")
	}
}

// VerifyStaffToken verifies a staff JWT token and returns staff claims
func VerifyStaffToken(tokenString string) (jwt.MapClaims, error) {
	claims, err := VerifyToken(tokenString)
	if err != nil {
		return nil, err
	}

	// Check if this is a staff token
	if tokenType, ok := claims["type"]; !ok || tokenType != "staff" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}

// VerifyCustomerToken verifies a customer JWT token and returns customer claims.
func VerifyCustomerToken(tokenString string) (jwt.MapClaims, error) {
	claims, err := VerifyToken(tokenString)
	if err != nil {
		return nil, err
	}

	if tokenType, ok := claims["type"]; !ok || tokenType != "customer" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}

// sweepVerifiedTokenCache deletes entries whose exp has passed as of now,
// so eviction does not depend on the same token being re-presented.
// The caller must not hold verifiedTokenCacheMu.
func sweepVerifiedTokenCache(now time.Time) {
	cutoff := now.Unix()
	verifiedTokenCacheMu.Lock()
	for tok, ent := range verifiedTokenCache {
		if ent.exp > 0 && cutoff >= ent.exp {
			delete(verifiedTokenCache, tok)
		}
	}
	verifiedTokenCacheMu.Unlock()
}

// StartVerifiedTokenCacheSweeper walks the cache on the given interval (default
// 1 minute) and evicts expired entries for the process lifetime. It returns a
// stop function; calling stop() signals the goroutine to exit and waits for it
// to finish, ensuring no goroutine leak on server shutdown.
//
// Wire this once in main.go near other background janitors; call stop() in the
// shutdown sequence.
func StartVerifiedTokenCacheSweeper(interval ...time.Duration) func() {
	iv := time.Minute
	if len(interval) > 0 && interval[0] > 0 {
		iv = interval[0]
	}

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	logger.SafeGoNamed("verified-token-cache-sweeper", func() {
		defer close(doneCh)
		ticker := time.NewTicker(iv)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case t := <-ticker.C:
				sweepVerifiedTokenCache(t)
			}
		}
	})

	return func() {
		close(stopCh)
		<-doneCh
	}
}
