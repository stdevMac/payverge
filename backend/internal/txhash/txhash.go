// Package txhash canonicalizes EVM transaction hashes used as ledger keys.
//
// go-ethereum's common.HexToHash is deliberately lenient: it accepts upper or
// lower case, a missing 0x, left zero-padding and even trailing non-hex
// garbage, and resolves all of them to the same 32-byte hash. Payverge uses the
// submitted hash string as the payment's dedupe key (payments.tx_hash UNIQUE),
// so any lenient spelling that reaches the chain lookup becomes a fresh ledger
// key for an already-claimed transfer. Every money path must therefore reduce a
// hash to exactly one spelling before it is verified or stored.
package txhash

import "strings"

// hashHexLen is the length of a 32-byte hash in hex digits.
const hashHexLen = 64

// Canonical returns the canonical spelling of an EVM transaction hash —
// "0x" followed by 64 lowercase hex digits — and true when s (after trimming
// surrounding whitespace) is exactly a 0x/0X prefix plus 64 hex digits.
// Anything else (missing prefix, wrong length, padding, non-hex) returns false.
func Canonical(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) != 2+hashHexLen || s[0] != '0' || (s[1] != 'x' && s[1] != 'X') {
		return "", false
	}
	body := s[2:]
	if !isHex(body) {
		return "", false
	}
	return "0x" + strings.ToLower(body), true
}

// NormalizeReference canonicalizes a stored payment reference when it is
// EVM-hash shaped (optional 0x/0X prefix + 64 hex digits) and otherwise returns
// it trimmed but untouched. Provider references (Stripe ids, plugin_/manual_/
// split_ synthetic keys) are case-sensitive identifiers and are never folded.
// This is the ledger-layer backstop: even a caller that skipped strict
// validation cannot store two spellings of one on-chain transfer.
func NormalizeReference(s string) string {
	s = strings.TrimSpace(s)
	body := s
	if len(body) >= 2 && body[0] == '0' && (body[1] == 'x' || body[1] == 'X') {
		body = body[2:]
	}
	if len(body) == hashHexLen && isHex(body) {
		return "0x" + strings.ToLower(body)
	}
	return s
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
