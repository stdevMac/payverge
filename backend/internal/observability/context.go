package observability

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/middleware"
)

type RequestContext struct {
	Tags    map[string]string
	UserID  string
	Context map[string]interface{}
}

func BuildRequestContext(c *gin.Context) RequestContext {
	requestContext := RequestContext{
		Tags: map[string]string{
			"service": "backend",
		},
		Context: map[string]interface{}{
			"service": "backend",
		},
	}
	if c == nil {
		return requestContext
	}

	if c.Request != nil {
		requestContext.Context["method"] = c.Request.Method
		if c.Request.URL != nil {
			// Route template when matched (never contains capability tokens);
			// otherwise the scrubbed raw path for 404s / unmatched routes.
			if path := middleware.SafeRequestPath(c); path != "" {
				if c.FullPath() == "" {
					path = sanitizeURL(path)
				}
				requestContext.Context["path"] = path
			}
		}
	}

	if route := strings.TrimSpace(c.FullPath()); route != "" {
		requestContext.Tags["route"] = route
		requestContext.Context["route"] = route
	}

	status := c.Writer.Status()
	requestContext.Tags["status"] = strconv.Itoa(status)
	requestContext.Context["status"] = status

	if requestID := firstContextID(c, middleware.RequestIDKey); requestID != "" {
		requestContext.Tags["request_id"] = requestID
		requestContext.Context["request_id"] = requestID
	}

	if authSource := firstContextID(c, "token_type"); authSource != "" {
		requestContext.Tags["auth_source"] = authSource
		requestContext.Context["auth_source"] = authSource
	}

	if businessID := businessIDFromContext(c); businessID != "" {
		requestContext.Tags["business_id"] = businessID
		requestContext.Context["business_id"] = businessID
	}

	staffID := firstContextID(c, "staff_id")
	if staffID != "" {
		requestContext.Context["staff_id"] = staffID
		requestContext.UserID = "staff:" + staffID
	}

	customerID := firstContextID(c, "customer_id")
	if customerID != "" {
		requestContext.Context["customer_id"] = customerID
		if requestContext.UserID == "" {
			requestContext.UserID = "customer:" + customerID
		}
	}

	userID := firstContextID(c, "user_id")
	if userID != "" {
		requestContext.Context["user_id"] = userID
		if requestContext.UserID == "" {
			requestContext.UserID = "user:" + userID
		}
	}

	if address := firstContextID(c, "address"); address != "" && requestContext.UserID == "" {
		web3ID := pseudonymousWeb3ID(address)
		requestContext.Context["web3_id"] = web3ID
		requestContext.UserID = web3ID
	}

	return requestContext
}

func ApplyRequestContext(scope *sentry.Scope, requestContext RequestContext) {
	if scope == nil {
		return
	}

	if len(requestContext.Tags) > 0 {
		scope.SetTags(requestContext.Tags)
	}
	if requestContext.UserID != "" {
		scope.SetUser(sentry.User{ID: requestContext.UserID})
	}
	if len(requestContext.Context) > 0 {
		scope.SetContext("payverge", sentry.Context(requestContext.Context))
	}
}

func GinContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		apply := func() {
			hub := sentrygin.GetHubFromContext(c)
			if hub == nil {
				return
			}
			requestContext := BuildRequestContext(c)
			hub.ConfigureScope(func(scope *sentry.Scope) {
				ApplyRequestContext(scope, requestContext)
			})
		}

		// Apply only after the handler chain runs: BuildRequestContext reads the
		// auth identity and response status, none of which exist before c.Next().
		// Calling apply() pre-Next just double-allocated an empty context per
		// request, so the deferred call is the only one we keep.
		defer apply()
		c.Next()
	}
}

func businessIDFromContext(c *gin.Context) string {
	if businessID := firstContextID(c, "staff_business_id", "business_id"); businessID != "" {
		return businessID
	}
	if businessID := stringifyID(c.Param("business_id")); businessID != "" {
		return businessID
	}
	if strings.Contains(c.FullPath(), "/businesses/:id") {
		return stringifyID(c.Param("id"))
	}

	return ""
}

func firstContextID(c *gin.Context, keys ...string) string {
	if c == nil {
		return ""
	}
	for _, key := range keys {
		value, ok := c.Get(key)
		if !ok {
			continue
		}
		if id := stringifyID(value); id != "" {
			return id
		}
	}

	return ""
}

func stringifyID(value interface{}) string {
	switch id := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(id)
	case uint:
		return strconv.FormatUint(uint64(id), 10)
	case uint64:
		return strconv.FormatUint(id, 10)
	case uint32:
		return strconv.FormatUint(uint64(id), 10)
	case int:
		return strconv.FormatInt(int64(id), 10)
	case int64:
		return strconv.FormatInt(id, 10)
	case int32:
		return strconv.FormatInt(int64(id), 10)
	case float64:
		return stringifyFloatID(id, 64)
	case float32:
		return stringifyFloatID(float64(id), 32)
	default:
		return ""
	}
}

func stringifyFloatID(value float64, bits int) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return ""
	}

	return strconv.FormatFloat(value, 'f', -1, bits)
}

func pseudonymousWeb3ID(address string) string {
	normalized := strings.ToLower(strings.TrimSpace(address))
	if normalized == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("web3:%x", sum[:8])
}
