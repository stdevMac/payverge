package middleware

import (
	"net/netip"
	"strings"

	"github.com/gin-gonic/gin"
)

// ClientRateLimitKey returns the per-client bucket key that IP-keyed limiters
// should use for this request. See RateLimitKeyForIP.
func ClientRateLimitKey(c *gin.Context) string {
	return RateLimitKeyForIP(c.ClientIP())
}

// RateLimitKeyForIP maps a client IP to its rate-limit bucket key.
//
// IPv4 (and IPv4-mapped IPv6) addresses key on the full address. IPv6
// addresses key on their /64 prefix: a single subscriber is routinely
// delegated a whole /64 and can rotate through 2^64 source addresses, so a
// per-address bucket is no limit at all for an IPv6 attacker. Unparseable
// values are used verbatim and an empty value becomes "unknown" so a caller
// is always bucketed, never exempted.
func RateLimitKeyForIP(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "unknown"
	}
	if strings.IndexByte(raw, ':') < 0 {
		// IPv4 (gin's ClientIP has already validated it) or an opaque value:
		// either way the verbatim string is the key. No parse, no allocation
		// on the common path.
		return raw
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return raw
	}
	addr = addr.WithZone("").Unmap()
	if addr.Is4() {
		return addr.String()
	}
	prefix, err := addr.Prefix(64)
	if err != nil {
		return addr.String()
	}
	return prefix.String()
}
