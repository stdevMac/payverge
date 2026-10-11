package demomode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/config"
)

func init() { gin.SetMode(gin.TestMode) }

// concretePath turns a gin pattern into a URL that matches it.
func concretePath(pattern string) string {
	parts := strings.Split(pattern, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") || strings.HasPrefix(p, "*") {
			parts[i] = "x1"
		}
	}
	return strings.Join(parts, "/")
}

func methodsFor(r Rule) []string {
	if r.Method == anyWrite {
		return []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	}
	return []string{r.Method}
}

func routerFor(pattern string, methods ...string) *gin.Engine {
	r := gin.New()
	r.Use(Guard())
	ok := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }
	for _, m := range methods {
		r.Handle(m, pattern, ok)
	}
	return r
}

type refusal struct {
	Error  string `json:"error"`
	Code   string `json:"code"`
	Params struct {
		Reason string `json:"reason"`
		Kind   string `json:"kind"`
	} `json:"params"`
}

func TestGuardRefusesEveryRuleWith403AndClearMessage(t *testing.T) {
	config.SetDemoModeForTesting(t, true)
	t.Setenv(config.DemoWriteRateEnv, "100000")
	for _, rule := range Rules {
		pattern := rule.Path
		if rule.Prefix {
			pattern = strings.TrimSuffix(rule.Path, "/") + "/sub/:x"
		}
		for _, method := range methodsFor(rule) {
			r := routerFor(pattern, method)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(method, concretePath(pattern), strings.NewReader("{}")))
			if w.Code != http.StatusForbidden {
				t.Errorf("%s %s: status %d, want 403", method, pattern, w.Code)
				continue
			}
			var body refusal
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("%s %s: decode: %v", method, pattern, err)
			}
			if body.Code != ErrorCode || body.Params.Reason != "demo_mode" || body.Params.Kind != string(rule.Kind) {
				t.Errorf("%s %s: body %+v", method, pattern, body)
			}
			if !strings.Contains(body.Error, "public demo") || !strings.Contains(body.Error, "Install your own Payverge") {
				t.Errorf("%s %s: unclear message %q", method, pattern, body.Error)
			}
			if rule.Kind != KindUpload && !strings.Contains(body.Error, rule.Action) {
				t.Errorf("%s %s: message %q does not name the action %q", method, pattern, body.Error, rule.Action)
			}
		}
	}
}

func TestGuardNamedAccountActionsAreRefused(t *testing.T) {
	// The task's explicit list: email/password change, invites, staff access,
	// venue deletion, integration credentials, uploads.
	cases := []struct{ method, path, wantAction string }{
		{http.MethodPost, "/api/v1/auth/password/reset-request", "changing passwords"},
		{http.MethodPost, "/api/v1/auth/password/reset", "changing passwords"},
		{http.MethodPut, "/api/v1/inside/update_user", "editing the demo account profile"},
		{http.MethodPost, biz + "/staff/invite", "inviting users"},
		{http.MethodDelete, biz, "deleting the venue"},
		{http.MethodPost, biz + "/plugins/:pluginId/config", "credentials"},
		{http.MethodPost, biz + "/uploads", "Uploads are disabled"},
		{http.MethodPost, "/api/v1/guest/bill/:bill_token/plugin-payment", "online card payments"},
		{http.MethodPost, biz + "/printers/:printerId/test", "test prints"},
		{http.MethodPost, biz + "/fiscal/settings", "ARCA"},
		{http.MethodPost, "/api/v1/admin/users/:id/role", "platform users"},
		{http.MethodPost, "/api/v1/crm/register", "creating customer accounts"},
		{http.MethodPut, biz + "/gallery-images", "gallery images"},
		{http.MethodPost, biz + "/uploads/logo", "Uploads are disabled"},
		{http.MethodPut, biz + "/google", "Google Business"},
		{http.MethodDelete, biz + "/google", "Google Business"},
		{http.MethodPost, "/api/v1/inside/businesses", "creating venues"},
		{http.MethodPost, biz + "/ai/regenerate-image", "AI image generation"},
		{http.MethodPost, biz + "/ai/enhance-image", "AI image generation"},
		{http.MethodPost, biz + "/generate-menu-image", "AI image generation"},
		{http.MethodPost, biz + "/marketing/image", "AI image generation"},
		{http.MethodPost, biz + "/marketing/image/cleanup", "AI image generation"},
	}
	config.SetDemoModeForTesting(t, true)
	for _, tc := range cases {
		rule, ok := Match(tc.method, tc.path)
		if !ok {
			t.Errorf("%s %s is not refused", tc.method, tc.path)
			continue
		}
		if msg := Message(rule); !strings.Contains(msg, tc.wantAction) {
			t.Errorf("%s %s: message %q lacks %q", tc.method, tc.path, msg, tc.wantAction)
		}
	}
}

func TestGuardNeverRefusesReads(t *testing.T) {
	for _, rule := range Rules {
		for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
			if _, ok := Match(m, rule.Path); ok {
				t.Errorf("%s %s refused; reads must stay open", m, rule.Path)
			}
		}
	}
}

func TestGuardAllowsShowroomWorkflows(t *testing.T) {
	// Writes the demo exists to show must stay open.
	open := []struct{ method, path string }{
		{http.MethodPost, biz + "/menu/categories"},
		{http.MethodPut, biz + "/tables/:tableId"},
		{http.MethodPost, "/api/v1/guest/bill/:bill_token/request-alternative-payment"},
		{http.MethodPost, "/api/v1/auth/demo/login"},
		{http.MethodPost, "/api/v1/auth/login"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodPost, "/api/v1/auth/signout"},
		{http.MethodPost, "/api/v1/auth/refresh"},
		{http.MethodPut, biz},
		// Text AI stays on under its own budget; only image generation is off.
		{http.MethodPost, biz + "/marketing/caption"},
		{http.MethodPost, biz + "/ai/wizard/:sessionId/message"},
		{http.MethodPost, "/api/v1/ai-waiter/:businessId"},
		// Signing in an existing customer is not account creation.
		{http.MethodPost, "/api/v1/crm/login"},
	}
	for _, tc := range open {
		if rule, ok := Match(tc.method, tc.path); ok {
			t.Errorf("%s %s refused by %+v; it is a showroom workflow", tc.method, tc.path, rule)
		}
	}
}

// The auth and admin groups are default-deny: anything not in Exempt is
// refused, including routes added later.
func TestGuardDefaultDeniesAuthAndAdminGroups(t *testing.T) {
	refused := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/auth/challenge"},
		{http.MethodPost, "/api/v1/auth/signin"},
		{http.MethodPost, "/api/v1/auth/email/verify"},
		{http.MethodPost, "/api/v1/auth/some-future-route"},
		{http.MethodPost, "/api/v1/admin/demo/reset"},
		{http.MethodPatch, "/api/v1/admin/businesses/:id"},
		{http.MethodPut, "/api/v1/admin/runtime-controls/:key"},
		{http.MethodDelete, "/api/v1/admin/some-future-route"},
	}
	for _, tc := range refused {
		if _, ok := Match(tc.method, tc.path); !ok {
			t.Errorf("%s %s is not refused by the default-deny groups", tc.method, tc.path)
		}
	}
	for _, r := range Exempt {
		if _, ok := Match(r.Method, r.Path); ok {
			t.Errorf("exempt %s %s is refused", r.Method, r.Path)
		}
	}
}

// Every demo write body is bounded by MaxWriteBodyBytes, so JSON blobs with
// no field-level cap (menus, settings, layouts) are capped by total size.
func TestGuardCapsWriteBodySize(t *testing.T) {
	config.SetDemoModeForTesting(t, true)
	var readErr error
	r := gin.New()
	r.Use(Guard())
	r.PUT(biz+"/menu", func(c *gin.Context) {
		_, readErr = io.ReadAll(c.Request.Body)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	path := concretePath(biz + "/menu")

	big := strings.Repeat("a", int(MaxWriteBodyBytes)+1)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, path, strings.NewReader(big)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("oversized body: status %d, want 403", w.Code)
	}
	var body refusal
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Code != ErrorCode || body.Params.Kind != string(KindSize) {
		t.Fatalf("oversized body refusal = %+v (%v)", body, err)
	}

	// A chunked body (no Content-Length) is cut off at the same size.
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(big))
	req.ContentLength = -1
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if readErr == nil {
		t.Fatal("chunked oversized body was read in full; want a MaxBytesReader error")
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"name":"ok"}`)))
	if w.Code != http.StatusOK || readErr != nil {
		t.Fatalf("small body: status %d, read error %v", w.Code, readErr)
	}
}

func TestGuardIsNoOpWhenDemoModeOff(t *testing.T) {
	config.SetDemoModeForTesting(t, false)
	r := routerFor(biz+"/staff/invite", http.MethodPost)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, concretePath(biz+"/staff/invite"), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 with DEMO_MODE off", w.Code)
	}
	if got := w.Header().Get("X-Robots-Tag"); got != "" {
		t.Fatalf("X-Robots-Tag %q set with DEMO_MODE off", got)
	}
}

func TestGuardSetsNoindexInDemoMode(t *testing.T) {
	config.SetDemoModeForTesting(t, true)
	r := routerFor("/api/v1/health/live", http.MethodGet)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if got := w.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Fatalf("X-Robots-Tag = %q", got)
	}
}

func TestGuardRateLimitsWritesPerIP(t *testing.T) {
	config.SetDemoModeForTesting(t, true)
	t.Setenv(config.DemoWriteRateEnv, "3")
	r := routerFor("/api/v1/inside/things", http.MethodPost, http.MethodGet)
	do := func(method, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/inside/things", nil)
		req.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for i := 0; i < 3; i++ {
		if w := do(http.MethodPost, "203.0.113.7"); w.Code != http.StatusOK {
			t.Fatalf("write %d: status %d", i, w.Code)
		}
	}
	w := do(http.MethodPost, "203.0.113.7")
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "60" || !strings.Contains(w.Body.String(), RateLimitedCode) {
		t.Fatalf("4th write: status %d headers %v body %s", w.Code, w.Header(), w.Body.String())
	}
	if w := do(http.MethodGet, "203.0.113.7"); w.Code != http.StatusOK {
		t.Fatalf("reads must not be limited, got %d", w.Code)
	}
	if w := do(http.MethodPost, "198.51.100.9"); w.Code != http.StatusOK {
		t.Fatalf("another IP must have its own budget, got %d", w.Code)
	}
}

func TestWriteLimiterRefillsAndSweeps(t *testing.T) {
	t.Setenv(config.DemoWriteRateEnv, "2")
	clock := time.Unix(1_800_000_000, 0)
	l := newWriteLimiter()
	l.now = func() time.Time { return clock }
	if !l.allow("a") || !l.allow("a") || l.allow("a") {
		t.Fatal("burst of 2 not enforced")
	}
	clock = clock.Add(31 * time.Second)
	if !l.allow("a") {
		t.Fatal("bucket did not refill after 30s at 2/min")
	}
	clock = clock.Add(bucketIdle + time.Minute)
	l.allow("b")
	if _, ok := l.buckets["a"]; ok {
		t.Fatal("idle bucket was not swept")
	}
}

// TestWriteLimiterBoundsMemory locks review finding F5: rotating source
// addresses must not grow the bucket map without limit.
func TestWriteLimiterBoundsMemory(t *testing.T) {
	t.Setenv(config.DemoWriteRateEnv, "2")
	clock := time.Unix(1_800_000_000, 0)
	l := newWriteLimiter()
	l.now = func() time.Time { return clock }
	l.maxBuckets = 3

	// One IPv6 /64 is one client: rotating the interface ID buys nothing.
	if !l.allow("2001:db8:1:2::1") || !l.allow("2001:db8:1:2:ffff::9") || l.allow("2001:db8:1:2:abcd::5") {
		t.Fatal("addresses in one /64 must share a bucket")
	}
	if !l.allow("2001:db8:1:3::1") {
		t.Fatal("a different /64 has its own budget")
	}
	if !l.allow("203.0.113.1") {
		t.Fatal("IPv4 client admitted under the cap")
	}
	if len(l.buckets) != 3 {
		t.Fatalf("want 3 buckets, got %d", len(l.buckets))
	}
	if l.allow("203.0.113.2") {
		t.Fatal("a new client beyond the cap must be refused while every bucket is live")
	}
	if len(l.buckets) != 3 {
		t.Fatalf("cap exceeded: %d buckets", len(l.buckets))
	}
	if !l.allow("203.0.113.1") {
		t.Fatal("a known client keeps its budget at the cap")
	}

	// Once the old buckets have refilled they are swept and room frees up.
	clock = clock.Add(bucketIdle + time.Second)
	if !l.allow("203.0.113.2") {
		t.Fatal("new client refused after idle buckets could be swept")
	}
	if len(l.buckets) != 1 {
		t.Fatalf("idle buckets not swept: %d left", len(l.buckets))
	}
}

func TestLimiterKey(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.7":            "203.0.113.7",
		"::ffff:203.0.113.7":     "::ffff:203.0.113.7",
		"2001:db8:aa:bb:1:2:3:4": "2001:db8:aa:bb::/64",
		"not-an-ip":              "not-an-ip",
	} {
		if got := limiterKey(in); got != want {
			t.Errorf("limiterKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRefuseWritesTheStandardDemoRefusal(t *testing.T) {
	r := gin.New()
	r.PUT("/x", func(c *gin.Context) { Refuse(c, KindStorefront, "changing the venue name") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/x", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", w.Code)
	}
	var body refusal
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != ErrorCode || body.Params.Reason != "demo_mode" || body.Params.Kind != string(KindStorefront) {
		t.Fatalf("body %+v", body)
	}
	if !strings.Contains(body.Error, "public demo") || !strings.Contains(body.Error, "changing the venue name") {
		t.Fatalf("unclear message %q", body.Error)
	}
}
