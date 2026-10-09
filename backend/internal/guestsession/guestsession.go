// Package guestsession owns the anonymous guest browser session cookie
// (pv_guest_session). The cookie value is "<id>.<hmac>"; the id is random and
// the HMAC binds it to the server secret so a guest cannot mint another
// guest's session. Split shares store the raw id; payment rows and the guest
// fiscal identity store only Fingerprint(id).
package guestsession

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/utils"
)

// CookieName is the guest session cookie shared by bill splitting, guest
// payments and the guest fiscal identity.
const CookieName = "pv_guest_session"

// cookieTTL matches the guest split hold horizon; a dining session is shorter.
const cookieTTL = 8 * time.Hour

// fingerprintDomain separates fingerprints from any other SHA-256 of the id.
const fingerprintDomain = "pv-guest-session:"

func secret() []byte {
	if s := strings.TrimSpace(os.Getenv("JWT_SECRET_KEY")); s != "" {
		return []byte(s)
	}
	return []byte("payverge-dev-guest-split-session")
}

// Sign returns the hex HMAC-SHA256 of a session id.
func Sign(sessionID string) string {
	mac := hmac.New(sha256.New, secret())
	mac.Write([]byte(sessionID))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a raw cookie value and returns its session id.
func Verify(raw string) (string, bool) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	expected := Sign(parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return "", false
	}
	return parts[0], true
}

// NewID returns a fresh random session id.
func NewID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return hex.EncodeToString([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
	}
	return hex.EncodeToString(buf[:])
}

// FromRequest returns the verified session id carried by the request, if any.
func FromRequest(c *gin.Context) (string, bool) {
	raw, err := c.Cookie(CookieName)
	if err != nil {
		return "", false
	}
	return Verify(raw)
}

// GetOrIssue returns the request's verified session id, issuing a new signed
// cookie when the request carries none (or an invalid one).
func GetOrIssue(c *gin.Context) string {
	if sessionID, ok := FromRequest(c); ok {
		return sessionID
	}
	sessionID := NewID()
	signed := sessionID + "." + Sign(sessionID)
	// Caddy terminates TLS in front of the backend, so c.Request.TLS is nil in
	// production — key the Secure flag off IsProduction() like every other
	// session cookie (utils.SetSessionCookie / ai_waiter_session_handler), and
	// still honor a direct TLS conn in non-prod.
	secure := utils.IsProduction() || (c.Request != nil && c.Request.TLS != nil)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(CookieName, signed, int(cookieTTL.Seconds()), "/", "", secure, true)
	return sessionID
}

// Fingerprint is the stored form of a session id: hex SHA-256 over a domain
// prefix, 64 characters. Empty ids have no fingerprint.
func Fingerprint(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(fingerprintDomain + sessionID))
	return hex.EncodeToString(sum[:])
}

// FingerprintPtr is Fingerprint as a nullable column value.
func FingerprintPtr(sessionID string) *string {
	fp := Fingerprint(sessionID)
	if fp == "" {
		return nil
	}
	return &fp
}

// PayerFingerprint returns the fingerprint of the caller's guest session,
// issuing the cookie when it is missing, for stamping a guest-initiated
// payment row.
func PayerFingerprint(c *gin.Context) *string {
	if c == nil {
		return nil
	}
	return FingerprintPtr(GetOrIssue(c))
}
