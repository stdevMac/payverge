package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newRedirectContext(rawURL string) *gin.Context {
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, rawURL, nil)
	return ctx
}

func TestResolveSafeFrontendRedirect_AllowsApprovedRelativePath(t *testing.T) {
	tests := []struct {
		name     string
		redirect string
		expect   string
	}{
		{name: "dashboard", redirect: "/dashboard", expect: "https://app.payverge.test/dashboard"},
		{name: "business", redirect: "/business/12/dashboard", expect: "https://app.payverge.test/business/12/dashboard"},
		{name: "staff", redirect: "/staff/7/orders", expect: "https://app.payverge.test/staff/7/orders"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PUBLIC_URL", "https://app.payverge.test")
			ctx := newRedirectContext("/api/v1/auth/google/callback?redirect=" + tt.redirect)

			redirect := resolveSafeFrontendRedirect(ctx, "/dashboard")

			assert.Equal(t, tt.expect, redirect)
			assert.NotContains(t, redirect, "token=")
		})
	}
}

func TestResolveSafeFrontendRedirect_RejectsExternalOrigin(t *testing.T) {
	t.Setenv("PUBLIC_URL", "https://app.payverge.test")
	ctx := newRedirectContext("/api/v1/auth/google/callback?redirect=https://evil.tld/pwn")

	redirect := resolveSafeFrontendRedirect(ctx, "/dashboard")

	assert.Equal(t, "https://app.payverge.test/dashboard", redirect)
}

func TestGetFrontendBaseURL_ReadsPublicURLOnly(t *testing.T) {
	t.Setenv("PUBLIC_URL", "")
	t.Setenv("NEXT_PUBLIC_BASE_URL", "https://retired.payverge.test")
	assert.Equal(t, "http://localhost:3000", getFrontendBaseURL())

	t.Setenv("PUBLIC_URL", "https://app.payverge.test/")
	assert.Equal(t, "https://app.payverge.test", getFrontendBaseURL())
}

func TestIsAllowedRedirectPath_RejectsPathTraversal(t *testing.T) {
	tests := []struct {
		path   string
		expect bool
	}{
		{"/business/123/dashboard", true},
		{"/staff/login", true},
		{"/dashboard", true},
		{"/profile", false},
		{"/business/../../../evil.com", false},
		{"/business/%2e%2e/evil", false},
		{"/staff/../../etc/passwd", false},
		{"//evil.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expect, isAllowedRedirectPath(tt.path), "path: %s", tt.path)
		})
	}
}
