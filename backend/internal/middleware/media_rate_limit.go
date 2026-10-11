package middleware

import (
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// Default per-IP budget for GET/HEAD /media/*key (MEDIA_RATE_LIMIT_*).
//
// /media is exempt from the global API limiter (security.go) because one guest
// menu page fans out to dozens of photos and must not spend the API budget.
// That exemption left the route unthrottled, which matters most on the s3
// driver without S3_PUBLIC_BASE_URL: every request is a bucket GET streamed
// through the backend. This separate token bucket closes that gap without
// touching the API budget.
//
// The defaults are deliberately generous. A restaurant Wi-Fi NAT presents every
// guest phone as one IP. Browsers cache content-addressed keys as immutable,
// so a legitimate client rarely re-fetches; the limit is aimed at bulk
// scraping and egress amplification, not page loads.
const (
	MediaRateLimitRequestsPerMinute = 1800 // 30/s sustained per IP
	MediaRateLimitBurst             = 600  // a full menu for a NATed dining room
)

// MediaRateLimitInternalMultiplier scales both the rate and the burst for a
// private or loopback peer that sends no X-Forwarded-For / X-Real-IP, i.e. a
// server fetching on its own behalf rather than relaying a client. In the
// shipped topology that is the frontend container: next/image optimizer cache
// misses and server renders reach /media through its same-origin proxy
// unstamped, so every guest's miss lands in that one bucket. A cold optimizer
// cache on a large menu (dishes x widths) must not 429 every guest at once.
// Relayed client traffic always carries X-Forwarded-For and keeps the
// per-client budget.
const MediaRateLimitInternalMultiplier = 10

// MediaRateLimit is a per-client-IP token bucket for the public /media route,
// with a separate, MediaRateLimitInternalMultiplier-times larger bucket per
// unforwarded private peer. requestsPerMinute <= 0 disables it (a CDN in front
// already absorbs the traffic). burst <= 0 falls back to requestsPerMinute/3,
// at least 1. Rejections answer 429 with Retry-After.
func MediaRateLimit(requestsPerMinute, burst int) gin.HandlerFunc {
	if requestsPerMinute <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	if burst <= 0 {
		burst = max(requestsPerMinute/3, 1)
	}
	clients := NewRateLimiter(rate.Every(time.Minute/time.Duration(requestsPerMinute)), burst)
	internalRPM := saturatingMul(requestsPerMinute, MediaRateLimitInternalMultiplier)
	internal := NewRateLimiter(
		rate.Every(time.Minute/time.Duration(internalRPM)),
		saturatingMul(burst, MediaRateLimitInternalMultiplier),
	)
	retryAfter := strconv.Itoa(max(60/requestsPerMinute, 1))

	return func(c *gin.Context) {
		limiter, key := clients, c.ClientIP()
		if unforwardedPrivatePeer(c, key) {
			limiter = internal
		}
		if !limiter.GetLimiter(key).Allow() {
			c.Header("Retry-After", retryAfter)
			c.Header("Cache-Control", "no-store")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many media requests. Please try again later.",
			})
			return
		}
		c.Next()
	}
}

// unforwardedPrivatePeer is true when the TCP peer is a loopback or private
// address and the request names no other client: no X-Forwarded-For or
// X-Real-IP, and Gin resolved the client to the peer itself (a trusted
// platform header such as CF-Connecting-IP would make them differ). A private
// peer that is a trusted proxy can already steer its bucket through
// X-Forwarded-For, so the larger internal budget hands it nothing it could not
// take.
func unforwardedPrivatePeer(c *gin.Context, clientIP string) bool {
	// Canonical keys and a direct map read: http.Header.Get would allocate
	// to canonicalize "X-Real-IP" on every request.
	h := c.Request.Header
	if len(h["X-Forwarded-For"]) > 0 || len(h["X-Real-Ip"]) > 0 {
		return false
	}
	// netip parses without allocating. Gin normalizes ClientIP (an
	// IPv4-mapped peer comes back as plain IPv4), so compare addresses, not
	// strings.
	peer, err := netip.ParseAddr(c.RemoteIP())
	if err != nil {
		return false
	}
	client, err := netip.ParseAddr(clientIP)
	if err != nil || client.Unmap() != peer.Unmap() {
		return false
	}
	peer = peer.Unmap()
	return peer.IsLoopback() || peer.IsPrivate()
}

func saturatingMul(v, m int) int {
	if v > math.MaxInt/m {
		return math.MaxInt
	}
	return v * m
}
