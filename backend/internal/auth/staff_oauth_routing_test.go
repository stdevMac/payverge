package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestGoogleCallback_RoutesStaffStateToStaffHandler verifies the fix for staff
// Google sign-in. The staff auth URL is built with the regular
// /api/v1/auth/google/callback redirect_uri (GoogleStaffAuthURL reuses
// GetGoogleAuthURL), so Google returns *here* carrying a state that lives only
// in the staff state store. Before the fix, GoogleCallback validated that state
// against the user store, missed it, and returned 400 "Invalid or expired
// state" — so every staff Google login failed and GoogleStaffCallback was dead
// code. GoogleCallback must instead hand staff states to GoogleStaffCallback.
func TestGoogleCallback_RoutesStaffStateToStaffHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &AuthHandler{
		staffStateStore: map[string]StaffOAuthState{
			"staff-state": {ExpiresAt: time.Now().Add(time.Hour), Redirect: "https://payverge.io/staff/login"},
		},
		// Google is nil, so the staff handler fails at the *token exchange* step —
		// which only happens if we successfully routed past the user-state check.
		oauthConfig: &OAuthConfig{},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=x&state=staff-state", nil)
	// Staff callback requires the browser-bound oauth_state cookie (SEC-1).
	c.Request.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "staff-state"})

	h.GoogleCallback(c)

	if strings.Contains(w.Body.String(), "Invalid or expired state") {
		t.Fatalf("staff state was rejected instead of routed to the staff handler: %s", w.Body.String())
	}

	// GoogleStaffCallback must have atomically consumed the single-use state.
	h.staffStateMu.RLock()
	_, present := h.staffStateStore["staff-state"]
	h.staffStateMu.RUnlock()
	if present {
		t.Fatal("staff state should have been consumed by GoogleStaffCallback")
	}
}

// TestIsStaffState reports membership without consuming, so the routing peek in
// GoogleCallback never burns a state that GoogleStaffCallback then needs.
func TestIsStaffState(t *testing.T) {
	h := &AuthHandler{
		staffStateStore: map[string]StaffOAuthState{
			"present": {ExpiresAt: time.Now().Add(time.Hour), Redirect: "x"},
		},
	}

	if !h.isStaffState("present") {
		t.Fatal("isStaffState should report a known staff state as present")
	}
	if h.isStaffState("absent") {
		t.Fatal("isStaffState should report an unknown state as absent")
	}
	if h.isStaffState("") {
		t.Fatal("isStaffState should treat an empty state as absent")
	}
	// The peek must not have removed the state.
	h.staffStateMu.RLock()
	_, present := h.staffStateStore["present"]
	h.staffStateMu.RUnlock()
	if !present {
		t.Fatal("isStaffState must not consume the state")
	}
}
