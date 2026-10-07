package middleware

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// accessLogRedactedQueryKeys are query names whose values must not reach the
// access log. Matching is case-insensitive on the decoded key.
var accessLogRedactedQueryKeys = map[string]struct{}{
	"token":         {},
	"code":          {},
	"state":         {},
	"paymentid":     {},
	"payerid":       {},
	"access_token":  {},
	"refresh_token": {},
	"id_token":      {},
	"secret":        {},
	"signature":     {},
	"sig":           {},
	"key":           {},
	"api_key":       {},
	"apikey":        {},
	"password":      {},
	"otp":           {},
	// Capability tokens read from the query string by guest/auth routes:
	// AI waiter session_token, MercadoPago return bill_token (a bill
	// public_token), staff invite_code.
	"session_token": {},
	"bill_token":    {},
	"public_token":  {},
	"invite_code":   {},
	"client_secret": {},
}

// accessLogFullPathKey is the gin context key under which the matched route
// template (c.FullPath()) is handed to the formatter. gin's
// LogFormatterParams does not carry it, so the Skip hook (which runs after the
// handler chain, before the formatter) copies it into c.Keys.
const accessLogFullPathKey = "_access_log_full_path"

// accessLogTokenParamNames are route-param names (lowercased, without ":" or
// "*") whose matched path segment is a capability token or secret. Enumerated
// from the router: :bill_token (guest bill links incl. receipts and splits),
// :code (guest table/QR codes, check-in), :token (space-scan sessions),
// :confirmationCode (public reservations). Any other param whose name contains
// "token" is also redacted. Invite, password-reset, email-verify and
// unsubscribe tokens are not path params in this router (they travel in
// bodies or the ?token= query, redacted below); the fallback rules cover a
// future /<invite|reset|verify|unsubscribe|share|receipt>/<token> shape.
var accessLogTokenParamNames = map[string]struct{}{
	"bill_token":       {},
	"billtoken":        {},
	"token":            {},
	"code":             {},
	"confirmationcode": {},
	"secret":           {},
	"signature":        {},
	"sig":              {},
	"invite_code":      {},
	"invitecode":       {},
}

func accessLogTokenParam(name string) bool {
	name = strings.ToLower(name)
	if _, ok := accessLogTokenParamNames[name]; ok {
		return true
	}
	return strings.Contains(name, "token")
}

// accessLogTokenPathPrefixes is the fallback for requests without a matched
// route template (404s, NoRoute, 405): the segment after the prefix is a
// capability token.
var accessLogTokenPathPrefixes = [][]string{
	{"api", "v1", "guest", "bill"},
	{"api", "v1", "guest", "table"},
	{"api", "v1", "space-scan"},
	{"api", "v1", "reservations"},
	{"api", "v1", "table"},
}

// accessLogTokenSegmentNames: in the fallback, the segment after one of these
// is treated as a token wherever it appears.
var accessLogTokenSegmentNames = map[string]struct{}{
	"token":       {},
	"tokens":      {},
	"invite":      {},
	"invitation":  {},
	"invitations": {},
	"reset":       {},
	"verify":      {},
	"unsubscribe": {},
	"share":       {},
	"receipt":     {},
	"receipts":    {},
	"bill":        {},
	"table":       {},
}

var successfulHealthAccessLogSkipPaths = []string{
	"/api/v1/health",
	"/api/v1/health/live",
	"/api/v1/health/ready",
}

// AccessLogger preserves Gin's normal request log while suppressing the
// successful health probes that otherwise consume most of the production log
// retention window. Non-successful health responses remain in the access log
// so a failed probe is still diagnosable even before centralized metrics land.
func AccessLogger() gin.HandlerFunc {
	return accessLogger(gin.DefaultWriter)
}

func accessLogger(output io.Writer) gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{
		Output:    output,
		Formatter: accessLogFormatter,
		Skip: func(c *gin.Context) bool {
			c.Set(accessLogFullPathKey, c.FullPath())
			if c.Writer.Status() >= 400 {
				return false
			}
			for _, path := range successfulHealthAccessLogSkipPaths {
				if c.Request.URL.Path == path {
					return true
				}
			}
			return false
		},
	})
}

// accessLogFormatter matches gin's defaultLogFormatter, with the request path
// passed through redactAccessLogPath so query secrets are not retained.
func accessLogFormatter(param gin.LogFormatterParams) string {
	var statusColor, methodColor, resetColor string
	if param.IsOutputColor() {
		statusColor = param.StatusCodeColor()
		methodColor = param.MethodColor()
		resetColor = param.ResetColor()
	}

	if param.Latency > time.Minute {
		param.Latency = param.Latency.Truncate(time.Second)
	}
	return fmt.Sprintf("[GIN] %v |%s %3d %s| %13v | %15s |%s %-7s %s %#v\n%s",
		param.TimeStamp.Format("2006/01/02 - 15:04:05"),
		statusColor, param.StatusCode, resetColor,
		param.Latency,
		param.ClientIP,
		methodColor, param.Method, resetColor,
		redactAccessLogPathForRoute(param.Path, fullPathFromKeys(param.Keys)),
		param.ErrorMessage,
	)
}

// redactAccessLogPath replaces values of sensitive query keys with REDACTED.
// Unrelated parameters keep their original encoding and order. A query that
// cannot be parsed is dropped entirely.
func redactAccessLogPath(rawPathWithQuery string) string {
	return redactAccessLogPathForRoute(rawPathWithQuery, "")
}

func fullPathFromKeys(keys map[string]any) string {
	if v, ok := keys[accessLogFullPathKey].(string); ok {
		return v
	}
	return ""
}

// redactAccessLogPathForRoute redacts token path segments using the matched
// route template when there is one, and the prefix heuristics otherwise.
func redactAccessLogPathForRoute(rawPathWithQuery, fullPath string) string {
	path, query, found := strings.Cut(rawPathWithQuery, "?")
	if fullPath != "" {
		path = redactAccessLogPathByTemplate(path, fullPath)
	} else {
		path = redactAccessLogTokenSegments(path)
	}
	if !found {
		return path
	}
	redacted, ok := redactAccessLogQuery(query)
	if !ok {
		return path + "?REDACTED"
	}
	return path + "?" + redacted
}

func redactAccessLogQuery(query string) (string, bool) {
	if _, err := url.ParseQuery(query); err != nil {
		return "", false
	}
	if query == "" {
		return "", true
	}
	parts := strings.Split(query, "&")
	for i, part := range parts {
		if part == "" {
			continue
		}
		keyPart, _, _ := strings.Cut(part, "=")
		key, err := url.QueryUnescape(keyPart)
		if err != nil {
			return "", false
		}
		if _, sensitive := accessLogRedactedQueryKeys[strings.ToLower(key)]; !sensitive {
			continue
		}
		parts[i] = keyPart + "=REDACTED"
	}
	return strings.Join(parts, "&"), true
}

// SafeRequestPath returns a path that is safe to hand to any log line, metric
// label, error-ingest payload or tracing attribute: the matched route template
// (c.FullPath(), e.g. /api/v1/guest/bill/:bill_token/split) when there is one,
// otherwise the raw path with capability-token segments redacted (404s,
// NoRoute). It never includes the query string.
func SafeRequestPath(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	if full := c.FullPath(); full != "" {
		return full
	}
	return redactAccessLogTokenSegments(c.Request.URL.Path)
}

// redactAccessLogTokenSegments replaces capability-token path segments with
// REDACTED, keyed by route pattern (see accessLogTokenPathPrefixes).
func redactAccessLogTokenSegments(path string) string {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, prefix := range accessLogTokenPathPrefixes {
		n := len(prefix)
		if len(segs) <= n || segs[n] == "" {
			continue
		}
		match := true
		for j, want := range prefix {
			if segs[j] != want {
				match = false
				break
			}
		}
		if match {
			segs[n] = "REDACTED"
		}
	}
	for i := 0; i+1 < len(segs); i++ {
		if _, ok := accessLogTokenSegmentNames[strings.ToLower(segs[i])]; ok && segs[i+1] != "" {
			segs[i+1] = "REDACTED"
		}
	}
	return "/" + strings.Join(segs, "/")
}

// redactAccessLogPathByTemplate replaces the raw path segments that line up
// with token-bearing params of the matched route template.
func redactAccessLogPathByTemplate(path, fullPath string) string {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	tmpl := strings.Split(strings.TrimPrefix(fullPath, "/"), "/")
	for i, t := range tmpl {
		if i >= len(segs) {
			break
		}
		if t == "" || (t[0] != ':' && t[0] != '*') {
			continue
		}
		if !accessLogTokenParam(t[1:]) {
			continue
		}
		if t[0] == '*' {
			for j := i; j < len(segs); j++ {
				segs[j] = "REDACTED"
			}
			break
		}
		segs[i] = "REDACTED"
	}
	return "/" + strings.Join(segs, "/")
}
