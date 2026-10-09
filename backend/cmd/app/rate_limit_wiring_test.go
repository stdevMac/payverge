package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/middleware"
)

// TestRateLimitWiring_BehaviorOnSharedRouter exercises the same limiter
// constructors used in main.go against a minimal router. It establishes that
// authLimiter (10 rpm, burst 3) and publicFormLimiter (5 rpm, burst 2) do
// actually throttle bursts from a single client IP.
func TestRateLimitWiring_BehaviorOnSharedRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mirror the limiters used in main.go.
	authLimiter := middleware.AuthRateLimiter(10, 3)      // SIWE + CRM
	publicFormLimiter := middleware.AuthRateLimiter(5, 2) // marketing opt-out

	ok := func(c *gin.Context) { c.Status(http.StatusOK) }

	r := gin.New()

	siwe := r.Group("/api/v1/auth")
	siwe.Use(authLimiter)
	{
		siwe.POST("/challenge", ok)
		siwe.POST("/signin", ok)
	}

	crm := r.Group("/api/v1/crm")
	crm.Use(authLimiter)
	{
		crm.POST("/register", ok)
		crm.POST("/login", ok)
	}

	form := r.Group("/api/v1")
	form.Use(publicFormLimiter)
	{
		form.GET("/email/unsubscribe", ok)
		form.POST("/email/unsubscribe", ok)
	}

	cases := []struct {
		name     string
		method   string
		path     string
		burstCap int // max 200s expected before 429 starts
	}{
		{"auth/challenge", "POST", "/api/v1/auth/challenge", 3},
		{"auth/signin", "POST", "/api/v1/auth/signin", 3},
		{"crm/register", "POST", "/api/v1/crm/register", 3},
		{"crm/login", "POST", "/api/v1/crm/login", 3},
		{"email/unsubscribe GET", "GET", "/api/v1/email/unsubscribe", 2},
		{"email/unsubscribe POST", "POST", "/api/v1/email/unsubscribe", 2},
	}

	const totalSend = 15

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			sawLimited := false
			okCount := 0
			for i := 0; i < totalSend; i++ {
				req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
				req.Header.Set("Content-Type", "application/json")
				// Force a single client IP so per-IP limiter engages.
				req.RemoteAddr = "198.51.100.42:54321"

				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)

				switch w.Code {
				case http.StatusOK:
					okCount++
				case http.StatusTooManyRequests:
					sawLimited = true
				default:
					t.Fatalf("unexpected status %d on %s (iteration %d)", w.Code, tc.path, i)
				}
			}

			assert.Truef(
				t,
				sawLimited,
				"expected at least one 429 over %d rapid requests to %s (route is not behind a per-IP limiter)",
				totalSend, tc.path,
			)
			assert.LessOrEqualf(
				t,
				okCount, tc.burstCap,
				"expected at most %d OK responses for %s before rate limiting kicks in, got %d",
				tc.burstCap, tc.path, okCount,
			)
		})
	}
}

// TestRateLimitWiring_MainGoApplication asserts that main.go still wires
// authLimiter and publicFormLimiter onto the sensitive public POST endpoints
// called out in the Round 3 audit. This is a structural regression guard:
// if someone unregisters the middleware from one of these routes, the check
// below trips even if runtime behavior elsewhere looks fine.
func TestRateLimitWiring_MainGoApplication(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	mainPath := filepath.Join(wd, "main.go")
	src, err := os.ReadFile(mainPath)
	require.NoError(t, err, "expected to read main.go next to this test")
	body := string(src)

	// Sanity: the constructors we rely on must still be referenced, with
	// production-strict defaults even though CI can override them via env.
	require.Contains(t, body, `intEnv("GLOBAL_RATE_LIMIT_REQUESTS_PER_MINUTE", 300)`,
		"main.go must default the global production limiter to 300 rpm")
	require.Contains(t, body, `intEnv("GLOBAL_RATE_LIMIT_REQUESTS_PER_MINUTE", 1200)`,
		"main.go must default the global development limiter to 1200 rpm")
	require.Contains(t, body, `intEnv("AUTH_RATE_LIMIT_REQUESTS_PER_MINUTE", 10)`,
		"main.go must default the SIWE/CRM auth limiter to 10 rpm")
	require.Contains(t, body, `intEnv("AUTH_RATE_LIMIT_BURST", 3)`,
		"main.go must default the SIWE/CRM auth limiter burst to 3")
	require.Contains(t, body, `intEnv("PUBLIC_FORM_RATE_LIMIT_REQUESTS_PER_MINUTE", 5)`,
		"main.go must default the public-form limiter to 5 rpm for /email/unsubscribe")
	require.Contains(t, body, `intEnv("PUBLIC_FORM_RATE_LIMIT_BURST", 2)`,
		"main.go must default the public-form limiter burst to 2")

	// Each of these routes must be registered with a limiter handler wedged
	// between the path and the handler (either inline or via .Use on the
	// enclosing group — the regex tolerates both common wirings).
	groupGuarded := regexp.MustCompile(
		`siweGroup\s*:=\s*auth\.Group\(""\)[\s\S]*?siweGroup\.Use\(authLimiter\)`,
	)
	require.True(t, groupGuarded.MatchString(body),
		"expected a siweGroup using authLimiter inside the /auth block")

	mustContain := []string{
		// SIWE — declared on siweGroup (which is guarded above).
		`siweGroup.POST("/challenge"`,
		`siweGroup.POST("/signin"`,
		// CRM — guarded either inline or via a group.Use(authLimiter).
		`crmAuthRoutes := publicRoutes.Group("/crm")`,
		`crmAuthRoutes.POST("/register", authLimiter`,
		`crmAuthRoutes.POST("/login", authLimiter`,
		// Public form limiter wiring.
		`publicFormLimiter`,
	}
	for _, snippet := range mustContain {
		assert.Containsf(t, body, snippet,
			"main.go is missing expected wiring snippet %q", snippet)
	}

	// Verify each sensitive route appears on the same line (or adjacent lines)
	// as its limiter middleware. We take a coarse approach: for each route,
	// check that its line, or either neighbor, mentions the relevant limiter
	// name — or that the route sits inside an auth/crm/publicForm group that
	// applied .Use() earlier.
	type routeCheck struct {
		needle  string // fragment identifying the route registration
		limiter string // limiter name expected nearby or on the enclosing group
	}
	checks := []routeCheck{
		{`GET("/email/unsubscribe"`, "publicFormLimiter"},
		{`POST("/email/unsubscribe"`, "publicFormLimiter"},
		{`crmAuthRoutes.POST("/register"`, "authLimiter"},
		{`crmAuthRoutes.POST("/login"`, "authLimiter"},
	}
	for _, c := range checks {
		idx := strings.Index(body, c.needle)
		if idx < 0 {
			// Route may be registered on a subgroup whose path makes the
			// `/crm/` or `/tools/` prefix implicit. Look for the tail only.
			short := strings.TrimPrefix(c.needle, `POST("/crm`)
			short = strings.TrimPrefix(short, `POST("/tools`)
			short = strings.TrimPrefix(short, `POST("`)
			if short != c.needle {
				alt := `POST("` + strings.TrimPrefix(short, "/")
				idx = strings.Index(body, alt)
			}
		}
		require.GreaterOrEqualf(t, idx, 0, "route %q not found in main.go", c.needle)

		// Look within a 400-char window on either side of the route
		// registration — enough to catch either inline middleware or the
		// nearest enclosing group's .Use(limiter).
		start := idx - 1200
		if start < 0 {
			start = 0
		}
		end := idx + 400
		if end > len(body) {
			end = len(body)
		}
		window := body[start:end]
		assert.Containsf(t, window, c.limiter,
			"route %q does not appear to be guarded by %s in main.go", c.needle, c.limiter)
	}
}

func TestCustomerRoutesUseTrustedOrigin(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	mainPath := filepath.Join(wd, "main.go")
	src, err := os.ReadFile(mainPath)
	require.NoError(t, err, "expected to read main.go next to this test")
	body := string(src)

	sessionGroup := sourceWindow(t, body, "customerSessionRoutes := r.Group", "customerRoutes := r.Group")
	assert.Contains(t, sessionGroup, `customerSessionRoutes.Use(middleware.RequireTrustedOriginForMutations(allowedOrigins))`,
		"customer session routes should require trusted origins for mutations")
	assert.Contains(t, sessionGroup, `customerSessionRoutes.GET("/session-info"`,
		"customer session-info should remain on the trusted-origin guarded group; the middleware skips GET")
	assert.Contains(t, sessionGroup, `customerSessionRoutes.POST("/refresh", authLimiter`,
		"customer refresh should preserve authLimiter")
	assert.Contains(t, sessionGroup, `customerSessionRoutes.POST("/logout"`,
		"customer logout should be registered on the guarded customer session group")

	customerGroup := sourceWindow(t, body, "customerRoutes := r.Group", "protectedRoutes := r.Group")
	trustedIdx := strings.Index(customerGroup, "middleware.RequireTrustedOriginForMutations(allowedOrigins)")
	authIdx := strings.Index(customerGroup, "server.CustomerAuthenticationMiddleware()")
	require.GreaterOrEqual(t, trustedIdx, 0,
		"protected customer routes should use trusted-origin middleware")
	require.GreaterOrEqual(t, authIdx, 0,
		"protected customer routes should use customer authentication")
	assert.Less(t, trustedIdx, authIdx,
		"protected customer routes should run trusted-origin checks before customer authentication")
	assert.Contains(t, customerGroup, `customerRoutes.PUT("/profile"`,
		"profile mutation should remain on the protected customer group")
	assert.Contains(t, customerGroup, `customerRoutes.POST("/link-wallet"`,
		"wallet linking mutation should remain on the protected customer group")

	crmGroup := sourceWindow(t, body, "crmAuthRoutes := publicRoutes.Group(\"/crm\")", "// Public Reservation routes")
	assert.Contains(t, crmGroup, "crmAuthRoutes.Use(middleware.RequireTrustedOriginForMutations(allowedOrigins))",
		"public CRM customer auth routes should require trusted origins for mutations")
	assert.Contains(t, crmGroup, `crmAuthRoutes.POST("/register", authLimiter, crmHandler.RegisterCustomer)`,
		"CRM register should preserve authLimiter")
	assert.Contains(t, crmGroup, `crmAuthRoutes.POST("/login", authLimiter, crmHandler.LoginCustomer)`,
		"CRM login should preserve authLimiter")
	assert.NotContains(t, crmGroup, `crmAuthRoutes.POST("/logout"`,
		"customer logout has one route: /customer/logout on the guarded customer session group")
	assert.NotContains(t, body, `publicRoutes.POST("/crm/register"`,
		"CRM register should not bypass the guarded CRM auth group")
	assert.NotContains(t, body, `publicRoutes.POST("/crm/login"`,
		"CRM login should not bypass the guarded CRM auth group")
	assert.NotContains(t, body, `publicRoutes.POST("/crm/logout"`,
		"CRM logout should not bypass the guarded CRM auth group")
}

// TestRateLimitWiring_PublicGETRoutesHaveSiblingLimiters pins #530 / #572:
// previously-unlimited public GETs must sit behind the limiter already used
// by their sibling in the same group. String-presence matches the rest of
// this file and main_route_wiring_test.go.
func TestRateLimitWiring_PublicGETRoutesHaveSiblingLimiters(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	src, err := os.ReadFile(filepath.Join(wd, "main.go"))
	require.NoError(t, err, "expected to read main.go next to this test")
	body := string(src)

	required := []string{
		// Fiscal JSON is the sibling of the already-limited PDF stream.
		`publicRoutes.GET("/guest/bill/:bill_token/fiscal-receipt", guestOrderRateLimiter.RateLimit(), server.GetGuestFiscalReceipt)`,
		`publicRoutes.GET("/guest/bill/:bill_token/alternative-payments", guestOrderRateLimiter.RateLimit(), paymentHandler.GetBillAlternativePayments)`,
		// Invitation preview sits next to staffAuthLimiter login-code routes.
		`publicRoutes.GET("/staff/invitation-preview", staffAuthLimiter, server.GetStaffInvitationPreview)`,
		// Catalog / storefront GETs reuse the public table-read limiter
		// (middleware.GuestTableReadRequestsPerMinute per IP).
		`publicRoutes.GET("/currencies", guestTableReadLimiter.RateLimit(), currencyHandler.GetSupportedCurrencies)`,
		`publicRoutes.GET("/languages", guestTableReadLimiter.RateLimit(), currencyHandler.GetSupportedLanguages)`,
		`publicRoutes.GET("/business/:customUrl", guestTableReadLimiter.RateLimit(), server.GetBusinessByCustomURL)`,
		`publicRoutes.GET("/business/:customUrl/google/reviews", guestTableReadLimiter.RateLimit(), server.GetPublicBusinessGoogleReviews)`,
		`publicRoutes.GET("/business/:customUrl/google/details", guestTableReadLimiter.RateLimit(), server.GetPublicBusinessGoogleDetails)`,
		`publicRoutes.GET("/business/:customUrl/menu", guestTableReadLimiter.RateLimit(), server.GetMenuByBusinessCustomUrl)`,
		`publicRoutes.GET("/guest/bill/:bill_token/orders", guestTableReadLimiter.RateLimit(), handlers.GetGuestOrdersByBillNumber)`,
		`publicRoutes.GET("/exchange-rate", guestTableReadLimiter.RateLimit(), currencyHandler.GetExchangeRate)`,
		`publicRoutes.GET("/convert", guestTableReadLimiter.RateLimit(), currencyHandler.ConvertAmount)`,
		`publicRoutes.GET("/business/:customUrl/reservations/availability", guestTableReadLimiter.RateLimit(), server.GetReservationAvailability)`,
		// Unsubscribe status is the sibling of the opt-out POST.
		`publicRoutes.GET("/email/unsubscribe", publicFormLimiter, server.GetUnsubscribeStatus)`,
		// Public delivery track has its own per-client limiter (#530, M-track).
		`publicRoutes.GET("/delivery/:delivery_number/track", deliveryTrackRateLimit, deliveryHandler.TrackDelivery)`,
	}
	for _, snippet := range required {
		assert.Containsf(t, body, snippet,
			"public GET %q must be registered with its sibling limiter", snippet)
	}

	// Un-limited registrations of the same paths must not remain alongside
	// the guarded lines (a second bare GET would still be enumerable).
	bare := []string{
		`publicRoutes.GET("/guest/bill/:bill_token/fiscal-receipt", server.GetGuestFiscalReceipt)`,
		`publicRoutes.GET("/guest/bill/:bill_token/alternative-payments", paymentHandler.GetBillAlternativePayments)`,
		`publicRoutes.GET("/staff/invitation-preview", server.GetStaffInvitationPreview)`,
		`publicRoutes.GET("/currencies", currencyHandler.GetSupportedCurrencies)`,
		`publicRoutes.GET("/languages", currencyHandler.GetSupportedLanguages)`,
		`publicRoutes.GET("/business/:customUrl", server.GetBusinessByCustomURL)`,
		`publicRoutes.GET("/business/:customUrl/google/reviews", server.GetPublicBusinessGoogleReviews)`,
		`publicRoutes.GET("/business/:customUrl/google/details", server.GetPublicBusinessGoogleDetails)`,
		`publicRoutes.GET("/business/:customUrl/menu", server.GetMenuByBusinessCustomUrl)`,
		`publicRoutes.GET("/guest/bill/:bill_token/orders", handlers.GetGuestOrdersByBillNumber)`,
		`publicRoutes.GET("/exchange-rate", currencyHandler.GetExchangeRate)`,
		`publicRoutes.GET("/convert", currencyHandler.ConvertAmount)`,
		`publicRoutes.GET("/business/:customUrl/reservations/availability", server.GetReservationAvailability)`,
		`publicRoutes.GET("/email/unsubscribe", server.GetUnsubscribeStatus)`,
		`publicRoutes.GET("/delivery/:delivery_number/track", deliveryHandler.TrackDelivery)`,
	}
	for _, snippet := range bare {
		assert.NotContainsf(t, body, snippet,
			"unthrottled public GET registration must not remain: %q", snippet)
	}
}

// TestRateLimitWiring_ManagerCorrectionsHaveOwnBucket pins issue 909: the
// PIN-gated correction routes (void/refund, crypto-refund lifecycle) must not
// be wired to PaymentRateLimit. That limiter keys on business_id+bill_id and
// falls back to a single per-IP 1-per-10s bucket — and /bills/:bill_id/void has
// no business param, so every correction at a venue drew on one token and a
// 404 probe left the next real void with a 429 about "another payment".
func TestRateLimitWiring_ManagerCorrectionsHaveOwnBucket(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	src, err := os.ReadFile(filepath.Join(wd, "main.go"))
	require.NoError(t, err, "expected to read main.go next to this test")
	body := string(src)

	assert.Contains(t, body, `managerPinLimiter := middleware.ManagerActionRateLimit()`,
		"manager-PIN corrections must use their own limiter bucket")
	assert.NotContains(t, body, `managerPinLimiter := middleware.PaymentRateLimit()`,
		"void/refund must not share the payment limiter's shape or copy (issue 909)")

	// The routes themselves must still be limited — this is a rescope, not a
	// removal.
	for _, snippet := range []string{
		`protectedRoutes.POST("/bills/:bill_id/void",
			managerPinLimiter,`,
		`protectedRoutes.POST("/bills/:bill_id/refund",
			managerPinLimiter,`,
	} {
		assert.Containsf(t, body, snippet, "correction route must stay behind managerPinLimiter: %q", snippet)
	}
}

func sourceWindow(t *testing.T, body, startNeedle, endNeedle string) string {
	t.Helper()

	start := strings.Index(body, startNeedle)
	require.GreaterOrEqualf(t, start, 0, "source start marker %q not found", startNeedle)

	end := strings.Index(body[start:], endNeedle)
	require.GreaterOrEqualf(t, end, 0, "source end marker %q not found after %q", endNeedle, startNeedle)

	return body[start : start+end]
}
