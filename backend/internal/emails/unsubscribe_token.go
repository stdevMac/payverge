package emails

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

// unsubscribeTokenTTL bounds how long a one-click link in an old email keeps
// working. Long by design (RFC-8058 mailbox-provider clicks can arrive months
// after send); the token only ever grants "turn marketing email OFF".
const unsubscribeTokenTTL = 365 * 24 * time.Hour

// unsubscribeClaims is the signed payload: the normalized recipient email and
// an expiry. Mirrors the crypto-quote-token idiom
// (internal/handlers/crypto_quote_token.go): base64url(JSON) + "." + hex(HMAC).
type unsubscribeClaims struct {
	Email string `json:"email"`
	Exp   int64  `json:"exp"`
}

var errUnsubscribeTokenInvalid = errors.New("unsubscribe token invalid")

// unsubscribeSecret returns the HMAC key: UNSUBSCRIBE_TOKEN_SECRET, else a
// domain-separated derivation of JWT_SECRET_KEY (fatal-at-startup in prod, so
// always present there). Empty result means "cannot mint" — senders then fall
// back to the login-gated account link instead of a broken URL.
func unsubscribeSecret() []byte {
	if s := strings.TrimSpace(os.Getenv("UNSUBSCRIBE_TOKEN_SECRET")); s != "" {
		return []byte(s)
	}
	if s := strings.TrimSpace(os.Getenv("JWT_SECRET_KEY")); s != "" {
		sum := sha256.Sum256([]byte("payverge-unsubscribe-v1|" + s))
		return sum[:]
	}
	return nil
}

// SignUnsubscribeToken mints a no-auth marketing opt-out token for an email
// address. ok=false when no secret material is configured.
func SignUnsubscribeToken(email string) (string, bool) {
	secret := unsubscribeSecret()
	normalized := strings.ToLower(strings.TrimSpace(email))
	if len(secret) == 0 || normalized == "" {
		return "", false
	}
	payload, err := json.Marshal(unsubscribeClaims{
		Email: normalized,
		Exp:   time.Now().Add(unsubscribeTokenTTL).Unix(),
	})
	if err != nil {
		return "", false
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encoded))
	return encoded + "." + hex.EncodeToString(mac.Sum(nil)), true
}

// ParseUnsubscribeToken verifies signature first (constant-time), then expiry,
// and returns the normalized email the token was minted for.
func ParseUnsubscribeToken(token string, now time.Time) (string, error) {
	secret := unsubscribeSecret()
	if len(secret) == 0 {
		return "", errUnsubscribeTokenInvalid
	}
	parts := strings.SplitN(strings.TrimSpace(token), ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", errUnsubscribeTokenInvalid
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0]))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return "", errUnsubscribeTokenInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", errUnsubscribeTokenInvalid
	}
	var claims unsubscribeClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", errUnsubscribeTokenInvalid
	}
	if claims.Email == "" || now.Unix() >= claims.Exp {
		return "", errUnsubscribeTokenInvalid
	}
	return claims.Email, nil
}
