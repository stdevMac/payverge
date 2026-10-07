package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccessLoggerSkipsSuccessfulHealthProbes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	router := gin.New()
	router.Use(accessLogger(&output))
	for _, path := range []string{
		"/api/v1/health",
		"/api/v1/health/live",
		"/api/v1/health/ready",
	} {
		router.GET(path, func(c *gin.Context) {
			if c.Query("source") == "failure" {
				c.Status(http.StatusServiceUnavailable)
				return
			}
			c.Status(http.StatusOK)
		})
	}
	router.GET("/api/v1/inside/businesses", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, path := range []string{
		"/api/v1/health",
		"/api/v1/health/live",
		"/api/v1/health/ready?source=deploy",
		"/api/v1/inside/businesses",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, recorder.Code)
	}
	failingRecorder := httptest.NewRecorder()
	router.ServeHTTP(failingRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready?source=failure", nil))
	require.Equal(t, http.StatusServiceUnavailable, failingRecorder.Code)

	require.NotContains(t, output.String(), `"/api/v1/health"`)
	require.NotContains(t, output.String(), "/api/v1/health/live")
	require.Equal(t, 1, bytes.Count(output.Bytes(), []byte("/api/v1/health/ready")))
	require.Contains(t, output.String(), "/api/v1/inside/businesses")
}

func TestAccessLoggerRedactsQuerySecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	router := gin.New()
	router.Use(accessLogger(&output))
	router.GET("/api/v1/auth/google/callback", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/api/v1/payments/return", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, path := range []string{
		"/api/v1/auth/google/callback?code=SECRETCODE&state=SECRETSTATE&lang=en",
		"/api/v1/payments/return?token=TOKENSECRET&paymentId=PAYMENTSECRET&PayerID=PAYERSECRET&lang=en",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, recorder.Code)
	}

	logged := output.String()
	for _, secret := range []string{"SECRETCODE", "SECRETSTATE", "TOKENSECRET", "PAYMENTSECRET", "PAYERSECRET"} {
		require.NotContains(t, logged, secret)
	}
	require.Contains(t, logged, "lang=en")
	require.Contains(t, logged, "code=REDACTED")
	require.Contains(t, logged, "state=REDACTED")
	require.Contains(t, logged, "token=REDACTED")
	require.Contains(t, logged, "paymentId=REDACTED")
	require.Contains(t, logged, "PayerID=REDACTED")
}

func TestRedactAccessLogPath(t *testing.T) {
	const allSensitive = "/cb?token=a&code=b&state=c&paymentid=d&payerid=e&access_token=f&refresh_token=g&id_token=h&secret=i&signature=j&sig=k&key=l&api_key=m&apikey=n&password=o&otp=p&session_token=q&bill_token=r&public_token=s&invite_code=t&client_secret=u&lang=en"
	const allRedacted = "/cb?token=REDACTED&code=REDACTED&state=REDACTED&paymentid=REDACTED&payerid=REDACTED&access_token=REDACTED&refresh_token=REDACTED&id_token=REDACTED&secret=REDACTED&signature=REDACTED&sig=REDACTED&key=REDACTED&api_key=REDACTED&apikey=REDACTED&password=REDACTED&otp=REDACTED&session_token=REDACTED&bill_token=REDACTED&public_token=REDACTED&invite_code=REDACTED&client_secret=REDACTED&lang=en"

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no query", in: "/api/v1/health", want: "/api/v1/health"},
		{name: "oauth callback", in: "/api/v1/auth/google/callback?code=SECRETCODE&state=SECRETSTATE&lang=en", want: "/api/v1/auth/google/callback?code=REDACTED&state=REDACTED&lang=en"},
		{name: "payment return", in: "/pay?token=sekret&paymentId=abc&PayerID=def&ok=1", want: "/pay?token=REDACTED&paymentId=REDACTED&PayerID=REDACTED&ok=1"},
		{name: "every sensitive key", in: allSensitive, want: allRedacted},
		{name: "ai waiter session token", in: "/api/v1/ai-waiter/stream?session_token=CAP&lang=es", want: "/api/v1/ai-waiter/stream?session_token=REDACTED&lang=es"},
		{name: "mercadopago return bill token", in: "/api/v1/mp/return?bill_token=CAP&status=approved", want: "/api/v1/mp/return?bill_token=REDACTED&status=approved"},
		{name: "keeps unrelated encoding", in: "/pay?lang=hello%20world&code=abc", want: "/pay?lang=hello%20world&code=REDACTED"},
		{name: "does not redact a different key", in: "/pay?discount_code=SAVE&lang=en", want: "/pay?discount_code=SAVE&lang=en"},
		{name: "repeated keys", in: "/pay?token=a&token=b&x=1", want: "/pay?token=REDACTED&token=REDACTED&x=1"},
		{name: "case insensitive key", in: "/cb?Token=ABC&Lang=en", want: "/cb?Token=REDACTED&Lang=en"},
		{name: "encoded key name", in: "/cb?%74oken=SECRET&lang=en", want: "/cb?%74oken=REDACTED&lang=en"},
		{name: "empty query", in: "/pay?", want: "/pay?"},
		{name: "bad escape drops the query", in: "/pay?code=%ZZ&lang=en", want: "/pay?REDACTED"},
		{name: "semicolon drops the query", in: "/pay?a=1;b=2", want: "/pay?REDACTED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, redactAccessLogPath(tt.in))
		})
	}
}

func TestRedactAccessLogPathTokenSegments(t *testing.T) {
	cases := map[string]string{
		"/api/v1/guest/bill/abc123/split/state":          "/api/v1/guest/bill/REDACTED/split/state",
		"/api/v1/guest/bill/abc123":                      "/api/v1/guest/bill/REDACTED",
		"/api/v1/guest/table/T-9XK/menu?lang=es":         "/api/v1/guest/table/REDACTED/menu?lang=es",
		"/api/v1/guest/table/T-9XK/events?token=s3cr3t":  "/api/v1/guest/table/REDACTED/events?token=REDACTED",
		"/api/v1/space-scan/scantok/uploads":             "/api/v1/space-scan/REDACTED/uploads",
		"/api/v1/reservations/CONF123/cancel":            "/api/v1/reservations/REDACTED/cancel",
		"/api/v1/table/QR77/check-in":                    "/api/v1/table/REDACTED/check-in",
		"/api/v1/auth/reset/rawresettoken":               "/api/v1/auth/reset/REDACTED",
		"/api/v1/staff/invite/rawinvite":                 "/api/v1/staff/invite/REDACTED",
		"/api/v1/health/live":                            "/api/v1/health/live",
		"/api/v1/guest/bill":                             "/api/v1/guest/bill",
		"/api/v1/business/my-cafe/menu":                  "/api/v1/business/my-cafe/menu",
		"/api/v1/inside/businesses/42/orders?status=new": "/api/v1/inside/businesses/42/orders?status=new",
	}
	for in, want := range cases {
		require.Equal(t, want, redactAccessLogPath(in), in)
	}
}

func TestAccessLoggerRedactsPathTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	router := gin.New()
	router.Use(accessLogger(&output))
	router.GET("/api/v1/guest/bill/:bill_token/split/state", func(c *gin.Context) { c.Status(http.StatusOK) })
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/guest/bill/SUPERSECRETBILL/split/state", nil))
	require.NotContains(t, output.String(), "SUPERSECRETBILL")
	require.Contains(t, output.String(), "/api/v1/guest/bill/REDACTED/split/state")
}

func TestAccessLoggerRedactsTokensTableDriven(t *testing.T) {
	gin.SetMode(gin.TestMode)
	routes := []string{
		"/api/v1/guest/bill/:bill_token",
		"/api/v1/guest/bill/:bill_token/split/state",
		"/api/v1/guest/bill/:bill_token/fiscal-receipt/pdf",
		"/api/v1/guest/bill/:bill_token/email-receipt",
		"/api/v1/guest/bill/:bill_token/split/shares/:share_id/receipt",
		"/api/v1/guest/table/:code/menu",
		"/api/v1/space-scan/:token/uploads",
		"/api/v1/reservations/:confirmationCode/cancel",
		"/api/v1/table/:code/check-in",
		"/api/v1/staff/invite/:invite_token",
		"/api/v1/share/:share_token/view",
		"/api/v1/inside/businesses/:id/orders",
		"/api/v1/health/live",
	}
	cases := []struct {
		name, url, want string
		forbidden       []string
	}{
		{"bill", "/api/v1/guest/bill/BILLSECRET1", "/api/v1/guest/bill/REDACTED", []string{"BILLSECRET1"}},
		{"bill split", "/api/v1/guest/bill/BILLSECRET2/split/state", "/api/v1/guest/bill/REDACTED/split/state", []string{"BILLSECRET2"}},
		{"bill receipt pdf", "/api/v1/guest/bill/BILLSECRET3/fiscal-receipt/pdf", "/api/v1/guest/bill/REDACTED/fiscal-receipt/pdf", []string{"BILLSECRET3"}},
		{"email receipt", "/api/v1/guest/bill/BILLSECRET4/email-receipt", "/api/v1/guest/bill/REDACTED/email-receipt", []string{"BILLSECRET4"}},
		{"share receipt keeps share id", "/api/v1/guest/bill/BILLSECRET5/split/shares/77/receipt", "/api/v1/guest/bill/REDACTED/split/shares/77/receipt", []string{"BILLSECRET5"}},
		{"table code", "/api/v1/guest/table/TBLCODE9/menu?lang=es", "/api/v1/guest/table/REDACTED/menu?lang=es", []string{"TBLCODE9"}},
		{"space scan", "/api/v1/space-scan/SCANTOK/uploads", "/api/v1/space-scan/REDACTED/uploads", []string{"SCANTOK"}},
		{"reservation code", "/api/v1/reservations/CONFCODE/cancel", "/api/v1/reservations/REDACTED/cancel", []string{"CONFCODE"}},
		{"check-in", "/api/v1/table/QRSECRET/check-in", "/api/v1/table/REDACTED/check-in", []string{"QRSECRET"}},
		{"invite param", "/api/v1/staff/invite/INVITESECRET", "/api/v1/staff/invite/REDACTED", []string{"INVITESECRET"}},
		{"share param", "/api/v1/share/SHARESECRET/view", "/api/v1/share/REDACTED/view", []string{"SHARESECRET"}},
		{"non-token param kept", "/api/v1/inside/businesses/42/orders?status=new", "/api/v1/inside/businesses/42/orders?status=new", nil},
		{"query token", "/api/v1/inside/businesses/42/orders?token=QTOK&status=new", "/api/v1/inside/businesses/42/orders?token=REDACTED&status=new", []string{"QTOK"}},
		{"encoded query key", "/api/v1/inside/businesses/42/orders?%74oken=ENCTOK", "/api/v1/inside/businesses/42/orders?%74oken=REDACTED", []string{"ENCTOK"}},
		{"repeated query key", "/api/v1/inside/businesses/42/orders?code=A1&x=1&code=B2", "/api/v1/inside/businesses/42/orders?code=REDACTED&x=1&code=REDACTED", []string{"A1", "B2"}},
		{"unparseable query dropped", "/api/v1/inside/businesses/42/orders?token=S1;x=2", "/api/v1/inside/businesses/42/orders?REDACTED", []string{"S1"}},
		{"unmatched guest bill", "/api/v1/guest/bill/NOROUTETOK/unknown", "/api/v1/guest/bill/REDACTED/unknown", []string{"NOROUTETOK"}},
		{"unmatched guest table", "/api/v1/guest/table/NOROUTECODE/nope", "/api/v1/guest/table/REDACTED/nope", []string{"NOROUTECODE"}},
		{"unmatched reset", "/api/v1/auth/reset/RESETSECRET", "/api/v1/auth/reset/REDACTED", []string{"RESETSECRET"}},
		{"unmatched verify", "/api/v1/email/verify/VERIFYSECRET", "/api/v1/email/verify/REDACTED", []string{"VERIFYSECRET"}},
		{"unmatched unsubscribe", "/api/v1/email/unsubscribe/UNSUBSECRET", "/api/v1/email/unsubscribe/REDACTED", []string{"UNSUBSECRET"}},
		{"unmatched receipt", "/api/v1/receipt/RCPTSECRET", "/api/v1/receipt/REDACTED", []string{"RCPTSECRET"}},
		{"unmatched share", "/api/v1/share/SHRSECRET", "/api/v1/share/REDACTED", []string{"SHRSECRET"}},
		{"unmatched with query token", "/nope/x?token=NQ", "/nope/x?token=REDACTED", []string{"NQ"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			r := gin.New()
			r.Use(accessLogger(&out))
			for _, route := range routes {
				r.GET(route, func(c *gin.Context) { c.Status(http.StatusOK) })
			}
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			req.Header.Set("Referer", "https://x.test/?token=REFERERSECRET")
			req.Header.Set("Authorization", "Bearer AUTHSECRET")
			req.Header.Set("Cookie", "session=COOKIESECRET")
			r.ServeHTTP(httptest.NewRecorder(), req)

			line := out.String()
			require.Contains(t, line, tc.want)
			for _, f := range append(tc.forbidden, "REFERERSECRET", "AUTHSECRET", "COOKIESECRET") {
				require.NotContains(t, line, f)
			}
		})
	}
}

func TestSafeRequestPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct{ name, url, want string }{
		{"matched uses template", "/api/v1/guest/bill/BILLSECRET/split?token=Q", "/api/v1/guest/bill/:bill_token/split"},
		{"unmatched falls back to redacted raw path", "/api/v1/guest/bill/NOROUTE/x?token=Q", "/api/v1/guest/bill/REDACTED/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			var got string
			r.GET("/api/v1/guest/bill/:bill_token/split", func(c *gin.Context) {
				got = SafeRequestPath(c)
			})
			r.NoRoute(func(c *gin.Context) { got = SafeRequestPath(c) })
			r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tc.url, nil))
			require.Equal(t, tc.want, got)
		})
	}
	require.Equal(t, "", SafeRequestPath(nil))
}
