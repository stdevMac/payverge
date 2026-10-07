package guestsession

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "unit-test-secret")
	id := NewID()
	require.Len(t, id, 32)

	got, ok := Verify(id + "." + Sign(id))
	require.True(t, ok)
	require.Equal(t, id, got)
}

func TestVerifyRejectsTamperedOrMalformed(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "unit-test-secret")
	id := "abc123"
	sig := Sign(id)
	for _, raw := range []string{
		"",
		id,
		id + ".",
		"." + sig,
		"other." + sig,
		id + "." + sig + "x",
		id + "." + sig + ".extra",
	} {
		_, ok := Verify(raw)
		require.False(t, ok, "raw %q must not verify", raw)
	}

	t.Setenv("JWT_SECRET_KEY", "rotated-secret")
	_, ok := Verify(id + "." + sig)
	require.False(t, ok, "a cookie signed under another secret must not verify")
}

func TestFingerprint(t *testing.T) {
	fp := Fingerprint("abc123")
	require.Len(t, fp, 64)
	require.Equal(t, fp, Fingerprint(" abc123 "), "fingerprint ignores surrounding space")
	require.NotEqual(t, fp, Fingerprint("abc124"))
	require.NotContains(t, fp, "abc123", "raw id never appears in the stored form")
	require.Equal(t, "", Fingerprint("  "))
	require.Nil(t, FingerprintPtr(""))
	require.Equal(t, fp, *FingerprintPtr("abc123"))
	require.Nil(t, PayerFingerprint(nil))
}

func TestGetOrIssueIssuesThenReuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET_KEY", "unit-test-secret")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/guest", nil)
	issued := GetOrIssue(c)
	require.NotEmpty(t, issued)

	var cookie *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == CookieName {
			cookie = ck
		}
	}
	require.NotNil(t, cookie)
	require.True(t, cookie.HttpOnly)
	require.Equal(t, "/", cookie.Path)
	require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/guest", nil)
	c2.Request.AddCookie(cookie)
	require.Equal(t, issued, GetOrIssue(c2), "a valid cookie is reused")
	require.Empty(t, w2.Result().Cookies(), "no new cookie when the request carries a valid one")

	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Request = httptest.NewRequest(http.MethodPost, "/guest", nil)
	c3.Request.AddCookie(&http.Cookie{Name: CookieName, Value: issued + ".forged"})
	require.NotEqual(t, issued, GetOrIssue(c3), "a forged cookie gets a fresh session")
}
