package utils

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBusinessIdentifierFromParam_ReturnsRawValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		param    string
		expected string
	}{
		{"numeric id", "42", "42"},
		{"slug", "demo-core-business", "demo-core-business"},
		{"empty", "", ""},
		{"underscores", "my_test_business", "my_test_business"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/businesses/"+tc.param, nil)
			c.Params = gin.Params{{Key: "id", Value: tc.param}}

			got := BusinessIdentifierFromParam(c, "id")
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
