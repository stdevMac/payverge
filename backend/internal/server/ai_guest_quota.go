package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/dailyquota"
	"github.com/stdevmac/payverge/backend/internal/signedid"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
)

// Guest AI per-network / per-device daily caps (SEC H-ai-cost). The AI waiter
// is anonymous: a session is free to mint (60/IP/hour) and carries a 60-message
// cap, so without a daily per-client ceiling one script could spend a venue's
// whole guest budget. These in-memory counters are the cheap first line; the
// durable per-business guest USD scope plus the instance-wide USD scope stay
// the hard backstop.
//
// The per-IP ceiling is deliberately generous because a busy venue's guest
// Wi-Fi puts many diners behind one address; the per-device ceiling (keyed by
// a server-signed HttpOnly cookie minted with the session) is the tighter one.
// A client that drops cookies is still bounded by the per-IP ceiling.
const (
	envAIWaiterDailyPerIP     = "AI_WAITER_DAILY_MESSAGES_PER_IP"
	envAIWaiterDailyPerDevice = "AI_WAITER_DAILY_MESSAGES_PER_DEVICE"

	defaultAIWaiterDailyPerIP     = 600
	defaultAIWaiterDailyPerDevice = 150

	aiDeviceCookieName   = "pv_ai_device"
	aiDeviceCookieMaxAge = 180 * 24 * 60 * 60
)

var (
	guestAIQuota   = dailyquota.New(200_000)
	aiDeviceSigner = signedid.New("ai-waiter-device", "dv_")
)

func positiveIntEnv(name string, def int64) int64 {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// aiDeviceID returns the caller's signed device id, or "" when the cookie is
// absent or not one this server minted.
func aiDeviceID(c *gin.Context) string {
	v, err := c.Cookie(aiDeviceCookieName)
	if err != nil {
		return ""
	}
	v = strings.TrimSpace(v)
	if !aiDeviceSigner.Valid(v) {
		return ""
	}
	return v
}

// ensureAIDeviceCookie mints the device cookie when the caller has no valid
// one. Same attributes as the session cookie (HttpOnly, SameSite=Lax,
// COOKIE_DOMAIN, Secure in production).
func ensureAIDeviceCookie(c *gin.Context) {
	if aiDeviceID(c) != "" {
		return
	}
	id, ok := aiDeviceSigner.Mint()
	if !ok {
		return
	}
	parts := []string{
		fmt.Sprintf("%s=%s", aiDeviceCookieName, id),
		"Path=/",
		fmt.Sprintf("Max-Age=%d", aiDeviceCookieMaxAge),
		"HttpOnly",
		"SameSite=Lax",
	}
	if d := strings.TrimSpace(os.Getenv("COOKIE_DOMAIN")); d != "" {
		parts = append(parts, "Domain="+d)
	}
	if utils.IsProduction() {
		parts = append(parts, "Secure")
	}
	c.Writer.Header().Add("Set-Cookie", strings.Join(parts, "; "))
}

// takeGuestAIQuota counts one guest AI turn against the caller's network and
// device. All-or-nothing; false means a daily ceiling is exhausted.
func takeGuestAIQuota(c *gin.Context) bool {
	limits := []dailyquota.Limit{{
		Key: "waiter-ip:" + dailyquota.NetworkKey(c.ClientIP()),
		Max: positiveIntEnv(envAIWaiterDailyPerIP, defaultAIWaiterDailyPerIP),
	}}
	if dev := aiDeviceID(c); dev != "" {
		limits = append(limits, dailyquota.Limit{
			Key: "waiter-dev:" + dev,
			Max: positiveIntEnv(envAIWaiterDailyPerDevice, defaultAIWaiterDailyPerDevice),
		})
	}
	ok, _ := guestAIQuota.Take(limits...)
	return ok
}
