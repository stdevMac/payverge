package utils

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

var productionOverride atomic.Bool

func SetProductionOverride(enabled bool) {
	productionOverride.Store(enabled)
}

// IsProduction returns true when startup explicitly enables the production
// override from --production, or when ENV/APP_ENV is production (or prod).
// Nothing else implies production.
//
// In particular we do NOT infer production from:
//   - APP_BASE_URL starting with https:// (devs commonly point local backends
//     at a staging URL — inferring "prod" from that silently broke local CORS).
//   - gin.Mode() == release. Startup is responsible for translating
//     --production into the explicit production override used here.
//
// Production deployments must opt in through --production or
// ENV/APP_ENV=production (or prod) to enable Secure cookies, the
// trusted-origin allowlist, and other hardening.
func IsProduction() bool {
	if productionOverride.Load() {
		return true
	}

	for _, key := range []string{"ENV", "APP_ENV"} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
		case "production", "prod":
			return true
		}
	}

	return false
}

// authCookieSecure reports whether an auth cookie must set Secure.
// True in production, or when PUBLIC_URL is https, so a dev-mode
// deployment served over TLS still marks the cookie Secure.
func authCookieSecure() bool {
	if IsProduction() {
		return true
	}
	publicURL := strings.TrimSpace(os.Getenv("PUBLIC_URL"))
	return strings.HasPrefix(strings.ToLower(publicURL), "https://")
}

// writeAuthCookie emits a Set-Cookie header with HttpOnly, Secure in prod or
// when PUBLIC_URL is https, Path=/, and the caller-supplied SameSite policy.
// Session/access cookies
// and the operator refresh_token use "Lax" so same-site XHR from
// example.com to api.example.com still carries them. Customer refresh
// stays "Strict" (diner traffic is same-origin). Cross-site POST still
// omits Lax cookies, so this is not a CSRF relaxation for /auth/refresh.
//
// COOKIE_DOMAIN: prefer empty (host-only) or the concrete API host
// (e.g. api.example.com). Parent-domain values like ".example.com" broaden
// session cookies to every subdomain and are rejected by production preflight.
//
// Every write also evicts leftover identities (parent-domain, host-only vs
// concrete API host, opposite SameSite) so a COOKIE_DOMAIN remap cannot
// leave two cookies with this name. Browsers send every matching cookie;
// Gin reads the first, which after the .example.com → api.example.com
// migration was often the revoked one.
//
// Evictions are request-scoped: aliases are emitted once per cookie name,
// and a later live write replaces an earlier Max-Age=0 for the same
// name+domain+SameSite. ClearAll + set session + set refresh therefore
// stays inside a small Set-Cookie budget instead of repeating the same
// ~8 headers per write (~72 on login success, which Caddy 502s).
func writeAuthCookie(c *gin.Context, name, value string, maxAge int, sameSite string) {
	secure := authCookieSecure()
	domain := strings.TrimSpace(os.Getenv("COOKIE_DOMAIN"))
	evictConflictingAuthCookieIdentities(c, name, domain, sameSite, secure)
	emitAuthCookie(c, name, value, maxAge, domain, sameSite, secure)
}

const authCookieJarKey = "utils.authCookieJar"

type authCookieIdentity struct {
	name     string
	domain   string
	sameSite string
}

type authCookieWrite struct {
	name     string
	value    string
	maxAge   int
	domain   string
	sameSite string
	secure   bool
}

type authCookieJar struct {
	order   []authCookieIdentity
	writes  map[authCookieIdentity]authCookieWrite
	evicted map[string]struct{}
}

func isLiveAuthCookie(value string, maxAge int) bool {
	return value != "" && maxAge > 0
}

func authCookieJarFrom(c *gin.Context) *authCookieJar {
	if c == nil {
		return &authCookieJar{
			writes:  map[authCookieIdentity]authCookieWrite{},
			evicted: map[string]struct{}{},
		}
	}
	if existing, ok := c.Get(authCookieJarKey); ok {
		if jar, ok := existing.(*authCookieJar); ok && jar != nil {
			return jar
		}
	}
	jar := &authCookieJar{
		writes:  make(map[authCookieIdentity]authCookieWrite),
		evicted: make(map[string]struct{}),
	}
	c.Set(authCookieJarKey, jar)
	return jar
}

func emitAuthCookie(c *gin.Context, name, value string, maxAge int, domain, sameSite string, secure bool) {
	if c == nil {
		return
	}
	jar := authCookieJarFrom(c)
	id := authCookieIdentity{name: name, domain: domain, sameSite: sameSite}
	incoming := authCookieWrite{
		name:     name,
		value:    value,
		maxAge:   maxAge,
		domain:   domain,
		sameSite: sameSite,
		secure:   secure,
	}
	if prev, ok := jar.writes[id]; ok {
		if isLiveAuthCookie(prev.value, prev.maxAge) && !isLiveAuthCookie(incoming.value, incoming.maxAge) {
			return
		}
	} else {
		jar.order = append(jar.order, id)
	}
	jar.writes[id] = incoming
	flushAuthCookies(c, jar)
}

func flushAuthCookies(c *gin.Context, jar *authCookieJar) {
	existing := c.Writer.Header().Values("Set-Cookie")
	managed := make(map[string]struct{}, len(jar.order))
	for _, id := range jar.order {
		managed[id.name] = struct{}{}
	}
	var kept []string
	for _, header := range existing {
		if _, ok := managed[setCookieName(header)]; ok {
			continue
		}
		kept = append(kept, header)
	}
	c.Writer.Header().Del("Set-Cookie")
	for _, header := range kept {
		c.Writer.Header().Add("Set-Cookie", header)
	}
	for _, id := range jar.order {
		w := jar.writes[id]
		c.Writer.Header().Add("Set-Cookie", formatAuthCookie(w.name, w.value, w.maxAge, w.domain, w.sameSite, w.secure))
	}
}

func setCookieName(header string) string {
	if i := strings.Index(header, "="); i > 0 {
		return header[:i]
	}
	return header
}

func evictKeepDomainTwin(name string) bool {
	switch name {
	case "session_token", "refresh_token":
		return true
	default:
		return false
	}
}

func evictLeftoverDomains(name string) bool {
	// oauth_state is a short-lived CSRF binder, not a principal cookie.
	// Skipping its parent-domain fan-out keeps the login header budget
	// under the Caddy 502 threshold; the keep-domain clear still runs.
	return name != "oauth_state"
}

func formatAuthCookie(name, value string, maxAge int, domain, sameSite string, secure bool) string {
	cookieValue := fmt.Sprintf("%s=%s; Path=/; Max-Age=%d", name, value, maxAge)
	if domain != "" {
		cookieValue += fmt.Sprintf("; Domain=%s", domain)
	}
	if secure {
		cookieValue += "; Secure"
	}
	cookieValue += "; HttpOnly"
	cookieValue += fmt.Sprintf("; SameSite=%s", sameSite)
	return cookieValue
}

func evictConflictingAuthCookieIdentities(c *gin.Context, name, keepDomain, sameSite string, secure bool) {
	jar := authCookieJarFrom(c)
	if _, done := jar.evicted[name]; done {
		return
	}
	jar.evicted[name] = struct{}{}

	twin := "Strict"
	if sameSite == "Strict" {
		twin = "Lax"
	}
	// Evict the opposite SameSite on the identity we are about to write so a
	// Strict→Lax (or Lax→Strict) policy change cannot leave two cookies.
	// session_token / refresh_token are the identities that actually remapped
	// SameSite; other names stay on one policy and skip the twin.
	if evictKeepDomainTwin(name) {
		emitAuthCookie(c, name, "", 0, keepDomain, twin, secure)
	}

	if !evictLeftoverDomains(name) {
		return
	}
	// One SameSite per leftover domain is enough to drop the .example.com
	// migration leftovers; repeating the twin on every alias blew the
	// login header budget.
	for _, alias := range conflictingAuthCookieDomains(keepDomain, requestCookieHost(c)) {
		emitAuthCookie(c, name, "", 0, alias, sameSite, secure)
	}
}

func requestCookieHost(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	host := strings.TrimSpace(c.Request.Host)
	if host == "" && c.Request.URL != nil {
		host = strings.TrimSpace(c.Request.URL.Host)
	}
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host, "]") {
		return host[:i]
	}
	return host
}

func conflictingAuthCookieDomains(keepDomain, requestHost string) []string {
	keepDomain = strings.TrimSpace(keepDomain)
	requestHost = strings.TrimSpace(requestHost)
	seen := map[string]struct{}{keepDomain: {}}
	var aliases []string
	add := func(domain string) {
		domain = strings.TrimSpace(domain)
		if _, ok := seen[domain]; ok {
			return
		}
		seen[domain] = struct{}{}
		aliases = append(aliases, domain)
	}

	if keepDomain != "" {
		add("") // leftover host-only cookie
	} else if requestHost != "" {
		add(requestHost) // leftover Domain=api.example.com when writing host-only
	}

	parent := parentRegistrableDomain(firstNonEmpty(keepDomain, requestHost))
	if parent != "" {
		add(parent)
		add("." + parent)
	}
	return aliases
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parentRegistrableDomain(host string) string {
	host = strings.TrimPrefix(strings.TrimSpace(host), ".")
	// An IP address has no parent domain: "192.168.1.10" would otherwise
	// yield the bogus Domain=1.10 / .1.10 clearing cookies.
	if host == "" || strings.Contains(host, ":") || net.ParseIP(host) != nil {
		return ""
	}
	labels := strings.Split(host, ".")
	if len(labels) < 3 {
		return ""
	}
	for _, label := range labels {
		if label == "" {
			return ""
		}
	}
	return strings.Join(labels[len(labels)-2:], ".")
}

// SetSessionCookie sets a session/access cookie with SameSite=Lax so
// top-level navigations (e.g. clicking an email confirmation link) retain
// the session. HttpOnly, Secure in prod or when PUBLIC_URL is https, Path=/.
func SetSessionCookie(c *gin.Context, name, value string, maxAge int) {
	writeAuthCookie(c, name, value, maxAge, "Lax")
}

// SetStrictSessionCookie sets a cookie with SameSite=Strict. Use for
// diner customer_refresh_token. Operator refresh_token uses Lax
// (SetSessionCookie) so example.com → api.example.com XHR still sends it.
func SetStrictSessionCookie(c *gin.Context, name, value string, maxAge int) {
	writeAuthCookie(c, name, value, maxAge, "Strict")
}

// ClearSessionCookie clears a SameSite=Lax session cookie.
func ClearSessionCookie(c *gin.Context, name string) {
	writeAuthCookie(c, name, "", 0, "Lax")
}

// ClearStrictSessionCookie clears a SameSite=Strict cookie. Must match the
// SameSite attribute used when the cookie was set, otherwise some browsers
// keep the old cookie around.
func ClearStrictSessionCookie(c *gin.Context, name string) {
	writeAuthCookie(c, name, "", 0, "Strict")
}
