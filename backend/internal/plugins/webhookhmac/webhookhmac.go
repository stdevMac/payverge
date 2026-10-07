// Package webhookhmac provides the shared timestamped-HMAC kernel used by
// payment-plugin webhook verifiers. Stripe and MercadoPago both build a
// provider-specific signed payload (Stripe: "<ts>.<body>"; MercadoPago:
// "id:..;request-id:..;ts:..;"), HMAC-SHA256 it under the webhook secret, hex
// encode the result, and constant-time compare against the v1 signatures from
// the signature header. That last step is the security-critical, copy-paste
// prone part; centralizing it keeps every plugin on one audited compare path.
//
// Provider-specific header parsing and timestamp/clock-skew checks intentionally
// stay in each plugin — they diverge (Stripe unix-seconds vs MercadoPago
// seconds-or-millis) and are not security-sensitive in the same way.
package webhookhmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

// ComputeHexSHA256 returns the lowercase hex HMAC-SHA256 of payload under secret.
func ComputeHexSHA256(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyHexSHA256 reports whether any candidate signature matches the
// HMAC-SHA256 of payload under secret. Candidates are trimmed and lowercased
// before a constant-time compare, matching the historical per-plugin behavior.
func VerifyHexSHA256(secret string, payload []byte, candidateSignatures []string) bool {
	expected := ComputeHexSHA256(secret, payload)
	for _, sig := range candidateSignatures {
		if subtle.ConstantTimeCompare([]byte(strings.ToLower(strings.TrimSpace(sig))), []byte(expected)) == 1 {
			return true
		}
	}
	return false
}
