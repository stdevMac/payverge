package utils

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSetSessionCookie_UsesLax(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	SetSessionCookie(c, "session_token", "v", 900)

	got := liveSetCookie(t, w, "session_token=v")
	if !strings.Contains(got, "SameSite=Lax") {
		t.Fatalf("expected SameSite=Lax, got %q", got)
	}
	if !strings.Contains(got, "HttpOnly") {
		t.Fatalf("expected HttpOnly, got %q", got)
	}
}

// SEC-5 / #302: empty COOKIE_DOMAIN must emit host-only cookies (no Domain=).
func TestSetSessionCookie_HostOnlyWhenCookieDomainEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("COOKIE_DOMAIN", "")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	SetSessionCookie(c, "session_token", "v", 900)
	got := liveSetCookie(t, w, "session_token=v")
	if strings.Contains(strings.ToLower(got), "domain=") {
		t.Fatalf("expected host-only cookie without Domain=, got %q", got)
	}
}

func TestSetSessionCookie_UsesConcreteAPIHostDomain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("COOKIE_DOMAIN", "api.payverge.io")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	SetSessionCookie(c, "session_token", "v", 900)
	headers := w.Header().Values("Set-Cookie")
	var live string
	for _, header := range headers {
		if strings.Contains(header, "session_token=v") {
			live = header
		}
	}
	if !strings.Contains(live, "Domain=api.payverge.io") {
		t.Fatalf("expected live Domain=api.payverge.io, got %v", headers)
	}
}

func TestSetSessionCookie_EvictsParentDomainAliases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("COOKIE_DOMAIN", "api.payverge.io")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "https://api.payverge.io/", nil)

	SetSessionCookie(c, "session_token", "v", 900)
	headers := w.Header().Values("Set-Cookie")

	var liveCount int
	var clearedParent, clearedDotParent, clearedHostOnly, clearedStrictTwin bool
	for _, header := range headers {
		if strings.Contains(header, "session_token=v") {
			liveCount++
			if strings.Contains(header, "Domain=.payverge.io") || strings.Contains(header, "Domain=payverge.io;") {
				t.Fatalf("must not issue a live parent-domain cookie: %s", header)
			}
			continue
		}
		if !strings.HasPrefix(header, "session_token=") || !strings.Contains(header, "Max-Age=0") {
			continue
		}
		switch {
		case strings.Contains(header, "Domain=.payverge.io"):
			clearedDotParent = true
		case strings.Contains(header, "Domain=payverge.io"):
			clearedParent = true
		case !strings.Contains(strings.ToLower(header), "domain="):
			clearedHostOnly = true
		}
		if strings.Contains(header, "SameSite=Strict") && strings.Contains(header, "Domain=api.payverge.io") {
			clearedStrictTwin = true
		}
	}
	if liveCount != 1 {
		t.Fatalf("expected exactly one live cookie, got %d in %v", liveCount, headers)
	}
	if !clearedParent || !clearedDotParent || !clearedHostOnly {
		t.Fatalf("expected parent/host-only evictions, got %v", headers)
	}
	if !clearedStrictTwin {
		t.Fatalf("expected SameSite=Strict twin eviction on api.payverge.io, got %v", headers)
	}
}

func TestSetSessionCookie_IPHostHasNoParentDomainEvictions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("COOKIE_DOMAIN", "")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "http://192.168.1.10:8080/", nil)

	SetSessionCookie(c, "session_token", "v", 900)
	for _, header := range w.Header().Values("Set-Cookie") {
		if strings.Contains(header, "Domain=1.10") || strings.Contains(header, "Domain=.1.10") {
			t.Fatalf("an IP host must not get parent-domain clearing cookies: %s", header)
		}
	}
	if got := parentRegistrableDomain("10.0.0.5"); got != "" {
		t.Fatalf("parentRegistrableDomain(10.0.0.5) = %q, want empty", got)
	}
	if got := parentRegistrableDomain("api.payverge.io"); got != "payverge.io" {
		t.Fatalf("parentRegistrableDomain(api.payverge.io) = %q", got)
	}
}

func TestSetStrictSessionCookie_UsesStrict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	SetStrictSessionCookie(c, "refresh_token", "v", 604800)

	got := liveSetCookie(t, w, "refresh_token=v")
	if !strings.Contains(got, "SameSite=Strict") {
		t.Fatalf("expected SameSite=Strict, got %q", got)
	}
	if !strings.Contains(got, "HttpOnly") {
		t.Fatalf("expected HttpOnly, got %q", got)
	}
	if strings.Contains(got, "SameSite=Lax") {
		t.Fatalf("refresh cookie should not fall back to Lax: %q", got)
	}
}

func TestClearStrictSessionCookie_MatchesStrict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	ClearStrictSessionCookie(c, "refresh_token")

	var got string
	for _, header := range w.Header().Values("Set-Cookie") {
		if strings.HasPrefix(header, "refresh_token=") && strings.Contains(header, "SameSite=Strict") && strings.Contains(header, "Max-Age=0") {
			got = header
			break
		}
	}
	// Must echo SameSite=Strict — Chrome/Firefox are picky: clearing a
	// Strict cookie with a Lax Set-Cookie can leave the old cookie intact.
	if got == "" {
		t.Fatalf("expected SameSite=Strict Max-Age=0 clear, got %v", w.Header().Values("Set-Cookie"))
	}
}

func TestSetSessionCookie_SecureInProduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ENV", "production")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	SetSessionCookie(c, "session_token", "v", 900)

	got := liveSetCookie(t, w, "session_token=v")
	if !strings.Contains(got, "Secure") {
		t.Fatalf("expected Secure in production, got %q", got)
	}
}

func liveSetCookie(t *testing.T, w *httptest.ResponseRecorder, needle string) string {
	t.Helper()
	for _, header := range w.Header().Values("Set-Cookie") {
		if strings.Contains(header, needle) {
			return header
		}
	}
	t.Fatalf("missing %q in Set-Cookie: %v", needle, w.Header().Values("Set-Cookie"))
	return ""
}

func TestLoginCookieHeaderBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	SetProductionOverride(true)
	t.Cleanup(func() { SetProductionOverride(false) })
	t.Setenv("COOKIE_DOMAIN", "api.payverge.io")
	t.Setenv("ENV", "production")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "https://api.payverge.io/api/v1/auth/login", nil)
	c.Request.Host = "api.payverge.io"

	ClearAllAuthCookies(c)
	SetSessionCookie(c, "session_token", "access-token", 900)
	SetSessionCookie(c, "refresh_token", "refresh-token", 7*24*3600)

	headers := w.Header().Values("Set-Cookie")
	if len(headers) > 24 {
		t.Fatalf("login Set-Cookie budget is 24, got %d: %v", len(headers), headers)
	}

	requireLiveCookie := func(name, value string) {
		t.Helper()
		count := 0
		for _, header := range headers {
			if strings.Contains(header, name+"="+value) {
				count++
				if strings.Contains(header, "Domain=.payverge.io") || strings.Contains(header, "Domain=payverge.io;") {
					t.Fatalf("must not issue a live parent-domain cookie: %s", header)
				}
			}
		}
		if count != 1 {
			t.Fatalf("expected exactly one live %s cookie, got %d in %v", name, count, headers)
		}
	}
	requireLiveCookie("session_token", "access-token")
	requireLiveCookie("refresh_token", "refresh-token")

	requireClearedAlias := func(name, domainNeedle string, hostOnly bool) {
		t.Helper()
		for _, header := range headers {
			if !strings.HasPrefix(header, name+"=") || !strings.Contains(header, "Max-Age=0") {
				continue
			}
			if hostOnly && !strings.Contains(strings.ToLower(header), "domain=") {
				return
			}
			if !hostOnly && strings.Contains(header, domainNeedle) {
				return
			}
		}
		t.Fatalf("expected %s leftover eviction %q, got %v", name, domainNeedle, headers)
	}

	for _, name := range []string{"session_token", "refresh_token", "staff_token", "customer_token", "customer_refresh_token"} {
		requireClearedAlias(name, "Domain=.payverge.io", false)
		requireClearedAlias(name, "Domain=payverge.io", false)
		requireClearedAlias(name, "host-only", true)
	}
}

func TestIsProductionChecksENVAndAPPENV(t *testing.T) {
	SetProductionOverride(false)
	t.Cleanup(func() { SetProductionOverride(false) })
	t.Setenv("ENV", "production")
	t.Setenv("APP_ENV", "")
	assert.True(t, IsProduction())

	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "production")
	assert.True(t, IsProduction())
}

func TestIsProductionNormalizesEnvAliases(t *testing.T) {
	tests := []struct {
		name   string
		env    string
		appEnv string
	}{
		{name: "trims ENV", env: " production "},
		{name: "uppercases ENV", env: "PRODUCTION"},
		{name: "mixed case ENV", env: "ProDucTion"},
		{name: "prod ENV alias", env: "prod"},
		{name: "trims APP_ENV", appEnv: " prod "},
		{name: "uppercases APP_ENV", appEnv: "PROD"},
		{name: "mixed case APP_ENV", appEnv: "PrOd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetProductionOverride(false)
			t.Cleanup(func() { SetProductionOverride(false) })
			t.Setenv("ENV", tt.env)
			t.Setenv("APP_ENV", tt.appEnv)

			assert.True(t, IsProduction())
		})
	}
}

func TestIsProductionHonorsOverride(t *testing.T) {
	SetProductionOverride(true)
	t.Cleanup(func() { SetProductionOverride(false) })
	t.Setenv("ENV", "development")
	t.Setenv("APP_ENV", "development")

	assert.True(t, IsProduction())
}

func TestSetSessionCookie_SecureFollowsPublicURLOutsideProduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	SetProductionOverride(false)
	t.Cleanup(func() { SetProductionOverride(false) })
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")

	t.Run("https", func(t *testing.T) {
		t.Setenv("PUBLIC_URL", "https://pos.example.com")
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		SetSessionCookie(c, "session_token", "v", 900)

		got := liveSetCookie(t, w, "session_token=v")
		if !strings.Contains(got, "; Secure") {
			t.Fatalf("expected ; Secure when PUBLIC_URL is https, got %q", got)
		}
	})

	t.Run("http", func(t *testing.T) {
		t.Setenv("PUBLIC_URL", "http://localhost:3000")
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		SetSessionCookie(c, "session_token", "v", 900)

		got := liveSetCookie(t, w, "session_token=v")
		if strings.Contains(got, "; Secure") {
			t.Fatalf("expected no ; Secure when PUBLIC_URL is http, got %q", got)
		}
	})
}
