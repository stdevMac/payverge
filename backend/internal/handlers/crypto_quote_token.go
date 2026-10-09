package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// cryptoQuoteClaims is the signed payload of a crypto payment quote. It locks
// the USD amount (in USDC micro-units) the guest must transfer on-chain for a
// specific bill, so settlement verification is immune to FX drift between the
// quote and the on-chain confirmation. All money fields are integers.
type cryptoQuoteClaims struct {
	BusinessID uint `json:"business_id,omitempty"` // tenant the quote was minted for; binds the quote to a business
	// QuoteID is the crypto_payment_quotes row that reserved
	// USDMicrounits as this quote's exact amount. Settlement loads and consumes
	// that row; a token without it predates payer binding and is refused.
	QuoteID    uint  `json:"quote_id,omitempty"`
	BillID     uint  `json:"bill_id"`
	LocalCents int64 `json:"local_cents"` // combined amount+tip in the bill's currency
	// USDMicrounits is the EXACT on-chain amount (USDC, 6 decimals): the
	// converted amount plus the quote's unique sub-cent offset. A transfer of
	// any other value does not settle this quote.
	USDMicrounits  int64  `json:"usd_microunits"`
	SettlementAddr string `json:"settlement_address,omitempty"`
	ChainID        int64  `json:"chain_id,omitempty"`
	Token          string `json:"token,omitempty"`
	PaymentMethod  string `json:"payment_method,omitempty"`
	// Iat is when the quote was minted (unix seconds). A transfer mined before
	// the quote existed cannot be the payment for it, so settlement rejects a
	// receipt whose block time is earlier than Iat minus cryptoQuoteClockSkew.
	// Settlement rejects a token without it.
	Iat int64 `json:"iat,omitempty"`
	Exp int64 `json:"exp"` // expiry, unix seconds
}

// cryptoQuoteClockSkew tolerates drift between this server's clock and the
// chain's block timestamps (validators may set block time slightly off wall
// time) when comparing a transfer's block time against the quote's Iat.
const cryptoQuoteClockSkew = 120 * time.Second

// transferPredatesQuote reports whether a mined transfer is older than the
// quote it is being claimed against. blockTimestamp is unix seconds.
func transferPredatesQuote(claims cryptoQuoteClaims, blockTimestamp uint64) bool {
	if claims.Iat <= 0 {
		return false
	}
	floor := claims.Iat - int64(cryptoQuoteClockSkew/time.Second)
	return int64(blockTimestamp) < floor
}

var (
	errQuoteInvalid = errors.New("quote token invalid")
	errQuoteExpired = errors.New("quote token expired")
)

// signCryptoQuote serializes the claims and appends an HMAC-SHA256 signature
// over the base64url-encoded payload: "<base64url(payload)>.<hex(hmac)>".
func signCryptoQuote(claims cryptoQuoteClaims, secret []byte) string {
	payload, _ := json.Marshal(claims)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encoded))
	return encoded + "." + hex.EncodeToString(mac.Sum(nil))
}

// parseCryptoQuote verifies the signature first (constant-time), then expiry.
// Returns errQuoteInvalid for any structural/signature problem and
// errQuoteExpired when the signature is valid but exp has passed.
func parseCryptoQuote(token string, secret []byte, now time.Time) (cryptoQuoteClaims, error) {
	var claims cryptoQuoteClaims
	parts := strings.SplitN(strings.TrimSpace(token), ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return claims, errQuoteInvalid
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0]))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return claims, errQuoteInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, errQuoteInvalid
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, errQuoteInvalid
	}
	if now.Unix() >= claims.Exp {
		return claims, errQuoteExpired
	}
	return claims, nil
}
