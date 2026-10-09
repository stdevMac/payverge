package utils

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestClearAllAuthCookies_ClearsEveryAuthCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/logout", nil)

	ClearAllAuthCookies(c)

	headers := w.Header().Values("Set-Cookie")
	if len(headers) < 6 {
		t.Fatalf("expected 6 Set-Cookie headers (one per auth cookie), got %d: %v", len(headers), headers)
	}

	expected := []string{
		"session_token=",
		"staff_token=",
		"customer_token=",
		"oauth_state=",
		"refresh_token=",
		"customer_refresh_token=",
	}
	for _, name := range expected {
		found := false
		for _, h := range headers {
			if strings.HasPrefix(h, name) && strings.Contains(h, "Max-Age=0") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected cleared cookie for %s with Max-Age=0, got headers: %v", name, headers)
		}
	}

	var operatorLax, operatorStrict, customerStrict bool
	for _, h := range headers {
		if strings.HasPrefix(h, "refresh_token=") && strings.Contains(h, "Max-Age=0") {
			if strings.Contains(h, "SameSite=Lax") {
				operatorLax = true
			}
			if strings.Contains(h, "SameSite=Strict") {
				operatorStrict = true
			}
		}
		if strings.HasPrefix(h, "customer_refresh_token=") && strings.Contains(h, "SameSite=Strict") {
			customerStrict = true
		}
	}
	if !operatorLax || !operatorStrict {
		t.Errorf("operator refresh clear must emit both Lax and Strict, got: %v", headers)
	}
	if !customerStrict {
		t.Errorf("customer refresh cookie clear must use SameSite=Strict, got: %v", headers)
	}
}
