package server

import (
	"fmt"
	"net"
	"strings"
	"sync/atomic"

	"github.com/stdevmac/payverge/backend/internal/config"

	"github.com/gin-gonic/gin"
)

func parseTrustedProxies(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	proxies := make([]string, 0, len(parts))
	for _, part := range parts {
		proxy := strings.TrimSpace(part)
		if proxy != "" {
			proxies = append(proxies, proxy)
		}
	}

	if len(proxies) == 0 {
		return nil
	}

	return proxies
}

// TrustedProxiesConfigured reports whether raw yields at least one proxy
// entry. main uses this to fail loud in production when empty, since an
// empty list makes c.ClientIP() the proxy peer and guts every IP-keyed
// rate limit.
func TrustedProxiesConfigured(raw string) bool {
	return len(parseTrustedProxies(raw)) > 0
}

// ConfigureTrustedProxies disables forwarded-header trust by default and only
// enables it when explicitly configured.
func ConfigureTrustedProxies(engine *gin.Engine, raw string) error {
	return engine.SetTrustedProxies(parseTrustedProxies(raw))
}

// ConfigureTrustedPlatform lets deployments behind a CDN use the CDN-authored
// client-IP header. Keep this opt-in: unlike TrustedProxies, Gin gives this
// header priority over proxy-chain parsing.
func ConfigureTrustedPlatform(engine *gin.Engine, raw string) error {
	platform := strings.TrimSpace(strings.ToLower(raw))
	switch platform {
	case "":
		engine.TrustedPlatform = ""
	case "cloudflare":
		engine.TrustedPlatform = gin.PlatformCloudflare
	default:
		return fmt.Errorf("unsupported trusted platform %q", raw)
	}

	return nil
}

// WarnOnUntrustedForwardingPeer returns middleware that logs one loud error
// the first time a request carrying X-Forwarded-For arrives from a private,
// non-loopback peer that TRUSTED_PROXIES does not cover. That is the shape
// of a reverse proxy on a container network running against the loopback-only
// default (decision D-1): every client then shares the proxy's IP and all
// per-IP rate limits collapse into one bucket. Only enable it when the
// operator left TRUSTED_PROXIES unset. logf receives the message once.
func WarnOnUntrustedForwardingPeer(logf func(format string, args ...any)) gin.HandlerFunc {
	var warned atomic.Bool
	return func(c *gin.Context) {
		if !warned.Load() && c.Request.Header.Get("X-Forwarded-For") != "" {
			if peer := untrustedPrivatePeer(c.Request.RemoteAddr); peer != "" && warned.CompareAndSwap(false, true) {
				logf("[SECURITY] request with X-Forwarded-For from private peer %s, but TRUSTED_PROXIES is unset (default %s, loopback only): every client behind this proxy shares its IP for rate limiting. Set TRUSTED_PROXIES to the proxy's network CIDR.", peer, config.DefaultTrustedProxies)
			}
		}
		c.Next()
	}
}

func untrustedPrivatePeer(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || !ip.IsPrivate() {
		return ""
	}
	return ip.String()
}
