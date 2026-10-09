package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// mintLinkedSessionToken installs a session store on db, persists a live email
// session for user, and returns a user JWT whose hash matches that session.
func mintLinkedSessionToken(t *testing.T, db *gorm.DB, user *database.User) (string, *session.UserSession) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&session.UserSession{}))
	previous := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = previous })

	uid := user.ID
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &uid,
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	token, err := server.GenerateUserToken(user.ID, user.Email, "", user.Role, sess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))
	return token, sess
}

func sessionUserIDForCookie(token string) uint {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if token != "" {
		c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	}
	return sessionUserIDFromCookie(c)
}

func TestGoogleCallbackLinkIntent_RejectsAmbientSessionAlone(t *testing.T) {
	// SEC-1: attacker-started (anonymous) state + victim session must not link.
	linkingUserID, isLinking := googleCallbackLinkIntent(0, 42)
	assert.False(t, isLinking)
	assert.Zero(t, linkingUserID)

	// Mismatched session vs encoded link user — refuse.
	linkingUserID, isLinking = googleCallbackLinkIntent(7, 42)
	assert.False(t, isLinking)
	assert.Zero(t, linkingUserID)

	// Encoded link user but no session on callback — refuse.
	linkingUserID, isLinking = googleCallbackLinkIntent(42, 0)
	assert.False(t, isLinking)
	assert.Zero(t, linkingUserID)

	// Explicit link-mode: state + matching session.
	linkingUserID, isLinking = googleCallbackLinkIntent(42, 42)
	assert.True(t, isLinking)
	assert.Equal(t, uint(42), linkingUserID)
}

func TestGoogleAuthURL_SetsHttpOnlyStateCookie(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)
	h.oauthConfig = &OAuthConfig{
		Google: &oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/api/v1/auth/google/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: "https://accounts.google.test/o/oauth2/auth"},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google", nil)

	h.GoogleAuthURL(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NotEmpty(t, body["state"])

	cookie := findSetCookie(w, oauthStateCookieName)
	require.NotNil(t, cookie, "GoogleAuthURL must Set-Cookie oauth_state")
	assert.Equal(t, body["state"], cookie.Value)
	assert.True(t, cookie.HttpOnly, "oauth_state must be HttpOnly")
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
}

func TestGoogleAuthURL_EncodesLinkIntentOnlyWhenAuthenticatedAndRequested(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	h.oauthConfig = &OAuthConfig{
		Google: &oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/api/v1/auth/google/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: "https://accounts.google.test/o/oauth2/auth"},
		},
	}

	user := &database.User{Email: "owner@example.com", Name: "Owner", AuthMethod: "email", Role: "user"}
	require.NoError(t, db.Create(user).Error)
	token, _ := mintLinkedSessionToken(t, db, user)

	// Authenticated start WITHOUT link=1 — must not encode linkUserID.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google", nil)
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	h.GoogleAuthURL(c)
	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	entry, ok := h.stateStore.Consume(body["state"])
	require.True(t, ok)
	assert.Zero(t, entry.linkUserID, "ambient session without link=1 must not encode link intent")

	// Authenticated start WITH link=1 — encodes linkUserID.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google?link=1", nil)
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	h.GoogleAuthURL(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	entry, ok = h.stateStore.Consume(body["state"])
	require.True(t, ok)
	assert.Equal(t, user.ID, entry.linkUserID)
}

// TestGoogleCallback_SEC1_AttackerStartedOAuth_DoesNotLinkVictimAccount is the
// core account-linking CSRF regression. Attacker mints OAuth state anonymously;
// victim is logged in and hits the callback with the attacker state. Even when
// the victim somehow also presents the oauth_state cookie (worst-case cookie
// binding bypass), the Google identity must NOT attach to the victim.
func TestGoogleCallback_SEC1_AttackerStartedOAuth_DoesNotLinkVictimAccount(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)

	victim := &database.User{Email: "victim@example.com", Name: "Victim", AuthMethod: "email", Role: "user"}
	require.NoError(t, db.Create(victim).Error)
	require.NoError(t, db.Create(&UserAuth{
		UserID: victim.ID, Provider: "email", ProviderUserID: "victim@example.com", EmailVerified: true,
	}).Error)
	victimToken, err := server.GenerateUserToken(victim.ID, victim.Email, "", victim.Role)
	require.NoError(t, err)

	// Attacker starts OAuth anonymously (no session, no link=1).
	h.oauthConfig = &OAuthConfig{
		Google: &oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/api/v1/auth/google/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: "https://accounts.google.test/o/oauth2/auth"},
		},
	}
	startW := httptest.NewRecorder()
	startC, _ := gin.CreateTestContext(startW)
	startC.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google", nil)
	h.GoogleAuthURL(startC)
	require.Equal(t, http.StatusOK, startW.Code)
	var startBody map[string]string
	require.NoError(t, json.Unmarshal(startW.Body.Bytes(), &startBody))
	attackerState := startBody["state"]
	require.NotEmpty(t, attackerState)

	h.exchangeGoogleCodeFn = func(ctx context.Context, code string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "attacker-access", RefreshToken: "attacker-refresh", Expiry: time.Now().Add(time.Hour)}, nil
	}
	h.getGoogleUserInfoFn = func(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
		return &GoogleUserInfo{
			ID:            "attacker-google-id",
			Email:         "attacker@evil.example",
			VerifiedEmail: true,
			Name:          "Attacker",
			Picture:       "",
		}, nil
	}

	// Case A: victim visits callback WITHOUT attacker's oauth_state cookie → reject.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=authcode&state="+attackerState, nil)
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: victimToken})
	h.GoogleCallback(c)
	assert.Equal(t, http.StatusBadRequest, w.Code, "missing oauth_state cookie must reject callback")
	assertNoGoogleAuthRow(t, db, "attacker-google-id")
	assertOnlyEmailAuth(t, db, victim.ID)

	// Case B: worst case — victim also has the oauth_state cookie (e.g. shared
	// browser). State was started anonymously so linkUserID=0; ambient victim
	// session must still NOT attach the attacker's Google identity.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=authcode&state="+attackerState, nil)
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: victimToken})
	c.Request.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: attackerState})
	h.GoogleCallback(c)

	// Callback may succeed as a *new* attacker account (login), but must not
	// attach Google to the victim.
	assertOnlyEmailAuth(t, db, victim.ID)

	var googleAuth UserAuth
	err = db.Where("provider = ? AND provider_user_id = ?", "google", "attacker-google-id").First(&googleAuth).Error
	require.NoError(t, err, "attacker Google identity may create its own auth row")
	assert.NotEqual(t, victim.ID, googleAuth.UserID, "attacker Google must not be linked to victim")
}

// TestGoogleCallback_ExplicitLinkIntent_AttachesGoogleToAuthenticatedUser is
// the positive control for the SEC-1 fix: link=1 at start + cookie-bound state
// + matching session still links a new Google identity to that user.
func TestGoogleCallback_ExplicitLinkIntent_AttachesGoogleToAuthenticatedUser(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	h.oauthConfig = &OAuthConfig{
		Google: &oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/api/v1/auth/google/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: "https://accounts.google.test/o/oauth2/auth"},
		},
	}

	owner := &database.User{Email: "owner@example.com", Name: "Owner", AuthMethod: "email", Role: "user"}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(&UserAuth{
		UserID: owner.ID, Provider: "email", ProviderUserID: "owner@example.com", EmailVerified: true,
	}).Error)
	token, _ := mintLinkedSessionToken(t, db, owner)

	startW := httptest.NewRecorder()
	startC, _ := gin.CreateTestContext(startW)
	startC.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google?link=1", nil)
	startC.Request.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	h.GoogleAuthURL(startC)
	require.Equal(t, http.StatusOK, startW.Code)
	var startBody map[string]string
	require.NoError(t, json.Unmarshal(startW.Body.Bytes(), &startBody))
	state := startBody["state"]

	h.exchangeGoogleCodeFn = func(ctx context.Context, code string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "owner-access", Expiry: time.Now().Add(time.Hour)}, nil
	}
	h.getGoogleUserInfoFn = func(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
		return &GoogleUserInfo{
			ID: "owner-google-id", Email: "other@example.com", VerifiedEmail: true, Name: "Owner G",
		}, nil
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=authcode&state="+state, nil)
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	c.Request.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: state})
	h.GoogleCallback(c)

	var googleAuth UserAuth
	require.NoError(t, db.Where("provider = ? AND provider_user_id = ?", "google", "owner-google-id").First(&googleAuth).Error)
	assert.Equal(t, owner.ID, googleAuth.UserID)

	var auths []UserAuth
	require.NoError(t, db.Where("user_id = ?", owner.ID).Find(&auths).Error)
	assert.Len(t, auths, 2)
}

// TestGoogleCallback_RevokedSessionDoesNotLink proves a link=1 state minted
// from a live session cannot attach Google after that session is revoked.
func TestGoogleCallback_RevokedSessionDoesNotLink(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	h.oauthConfig = &OAuthConfig{
		Google: &oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/api/v1/auth/google/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: "https://accounts.google.test/o/oauth2/auth"},
		},
	}

	owner := &database.User{Email: "revoked-owner@example.com", Name: "Owner", AuthMethod: "email", Role: "user"}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(&UserAuth{
		UserID: owner.ID, Provider: "email", ProviderUserID: "revoked-owner@example.com", EmailVerified: true,
	}).Error)
	token, sess := mintLinkedSessionToken(t, db, owner)
	require.Equal(t, owner.ID, sessionUserIDForCookie(token))

	startW := httptest.NewRecorder()
	startC, _ := gin.CreateTestContext(startW)
	startC.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google?link=1", nil)
	startC.Request.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	h.GoogleAuthURL(startC)
	require.Equal(t, http.StatusOK, startW.Code)
	var startBody map[string]string
	require.NoError(t, json.Unmarshal(startW.Body.Bytes(), &startBody))
	state := startBody["state"]
	require.NotEmpty(t, state)

	require.NoError(t, session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonUserLogout))
	assert.Zero(t, sessionUserIDForCookie(token))

	h.exchangeGoogleCodeFn = func(ctx context.Context, code string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "revoked-access", Expiry: time.Now().Add(time.Hour)}, nil
	}
	h.getGoogleUserInfoFn = func(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
		return &GoogleUserInfo{
			ID: "revoked-google-id", Email: "other-revoked@example.com", VerifiedEmail: true, Name: "Other",
		}, nil
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=authcode&state="+state, nil)
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	c.Request.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: state})
	h.GoogleCallback(c)

	var linked []UserAuth
	require.NoError(t, db.Where("provider = ? AND user_id = ?", "google", owner.ID).Find(&linked).Error)
	assert.Empty(t, linked, "revoked session must not link Google to the owner")
	assertOnlyEmailAuth(t, db, owner.ID)
}

func TestSessionUserIDFromCookie(t *testing.T) {
	_, db := newAuthEnvelopeHandler(t)
	user := &database.User{Email: "cookie-sess@example.com", Name: "Cookie", AuthMethod: "email", Role: "user"}
	require.NoError(t, db.Create(user).Error)

	// (a) user token with no session_id claim.
	bareClaims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"role":    user.Role,
		"type":    "user",
		"exp":     time.Now().Add(time.Hour).Unix(),
	}
	bareToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, bareClaims).SignedString(structs.GetSecretKey())
	require.NoError(t, err)
	assert.Zero(t, sessionUserIDForCookie(bareToken))

	// Valid session returns the user id.
	token, sess := mintLinkedSessionToken(t, db, user)
	assert.Equal(t, user.ID, sessionUserIDForCookie(token))

	// (b) the same token is rejected after the session is revoked.
	require.NoError(t, session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonUserLogout))
	assert.Zero(t, sessionUserIDForCookie(token))

	// (c) a staff-type token is not an operator session.
	staffToken, err := server.GenerateStaffToken(&database.Staff{
		ID: 4, Email: "staff-cookie@example.com", Name: "Staff", Role: database.StaffRoleServer, BusinessID: 1,
	})
	require.NoError(t, err)
	assert.Zero(t, sessionUserIDForCookie(staffToken))
}

func TestGoogleCallback_ExpiresStaffCookieOnOwnerLogin(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	h.oauthConfig = &OAuthConfig{
		Google: &oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/api/v1/auth/google/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: "https://accounts.google.test/o/oauth2/auth"},
		},
	}

	owner := &database.User{Email: "google-owner@example.com", Name: "Owner", AuthMethod: "google", Role: "user"}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(&UserAuth{
		UserID: owner.ID, Provider: "google", ProviderUserID: "google-owner-id", EmailVerified: true,
	}).Error)

	startW := httptest.NewRecorder()
	startC, _ := gin.CreateTestContext(startW)
	startC.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google", nil)
	h.GoogleAuthURL(startC)
	require.Equal(t, http.StatusOK, startW.Code)
	var startBody map[string]string
	require.NoError(t, json.Unmarshal(startW.Body.Bytes(), &startBody))
	state := startBody["state"]

	h.exchangeGoogleCodeFn = func(ctx context.Context, code string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "owner-access", Expiry: time.Now().Add(time.Hour)}, nil
	}
	h.getGoogleUserInfoFn = func(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
		return &GoogleUserInfo{
			ID: "google-owner-id", Email: owner.Email, VerifiedEmail: true, Name: "Owner",
		}, nil
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=authcode&state="+state, nil)
	c.Request.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: state})
	c.Request.AddCookie(&http.Cookie{Name: "staff_token", Value: "live-staff-cookie"})
	h.GoogleCallback(c)

	require.True(t, w.Code == http.StatusTemporaryRedirect || w.Code == http.StatusFound, "owner Google callback must complete: %d", w.Code)
	require.True(t, hasExpiredSetCookie(w, "staff_token"), "successful owner Google login must expire a live staff cookie")
	sessionCookie := findSetCookie(w, "session_token")
	require.NotNil(t, sessionCookie)
	require.NotEmpty(t, sessionCookie.Value)
}

func hasExpiredSetCookie(w *httptest.ResponseRecorder, name string) bool {
	for _, raw := range w.Header().Values("Set-Cookie") {
		if !strings.HasPrefix(raw, name+"=") {
			continue
		}
		if strings.Contains(raw, "Max-Age=0") || strings.HasPrefix(raw, name+"=;") {
			return true
		}
	}
	return false
}

func TestGoogleCallback_RejectsStateCookieMismatch(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)
	h.stateStore.Set("real-state", time.Now().Add(time.Minute), "", 0)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=x&state=real-state", nil)
	c.Request.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: "other-state"})
	c.Request.AddCookie(&http.Cookie{Name: "staff_token", Value: "live-staff-cookie"})

	h.GoogleCallback(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, hasExpiredSetCookie(w, "staff_token"), "failed Google owner callback must not destroy an existing staff session")
	// Mismatched cookie must not consume the server-side state.
	_, stillPresent := h.stateStore.Consume("real-state")
	assert.True(t, stillPresent, "state must remain until a cookie-matching callback consumes it")
}

func TestGoogleStaffCallback_ExpiresOwnerCookiesOnStaffLogin(t *testing.T) {
	h, staff := setupStaffOAuthCookieHandler(t)
	h.exchangeGoogleCodeFn = func(ctx context.Context, code string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "staff-access", Expiry: time.Now().Add(time.Hour)}, nil
	}
	h.getGoogleUserInfoFn = func(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
		return &GoogleUserInfo{ID: "staff-google-id", Email: staff.Email, VerifiedEmail: true, Name: staff.Name}, nil
	}

	const state = "staff-oauth-state"
	h.staffStateStore[state] = StaffOAuthState{ExpiresAt: time.Now().Add(time.Minute), Redirect: "https://payverge.io/staff/login"}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=authcode&state="+state, nil)
	c.Request.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: state})
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: "live-owner-cookie"})
	c.Request.AddCookie(&http.Cookie{Name: "customer_token", Value: "live-customer-cookie"})
	h.GoogleStaffCallback(c)

	require.True(t, w.Code == http.StatusTemporaryRedirect || w.Code == http.StatusFound, "staff Google callback must complete: %d %s", w.Code, w.Body.String())
	require.True(t, hasExpiredSetCookie(w, "session_token"), "successful staff OAuth must expire a live owner cookie")
	require.True(t, hasExpiredSetCookie(w, "customer_token"), "successful staff OAuth must expire a live customer cookie")
	require.NotNil(t, findSetCookie(w, "staff_token"))
	require.NotEmpty(t, findSetCookie(w, "staff_token").Value)
}

func TestGoogleStaffCallback_FailedIssuanceDoesNotExpireOwnerCookies(t *testing.T) {
	h, staff := setupStaffOAuthCookieHandler(t)
	h.exchangeGoogleCodeFn = func(ctx context.Context, code string) (*oauth2.Token, error) {
		return &oauth2.Token{AccessToken: "staff-access", Expiry: time.Now().Add(time.Hour)}, nil
	}
	h.getGoogleUserInfoFn = func(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
		return &GoogleUserInfo{ID: "staff-google-id", Email: staff.Email, VerifiedEmail: false, Name: staff.Name}, nil
	}

	const state = "staff-oauth-fail-state"
	h.staffStateStore[state] = StaffOAuthState{ExpiresAt: time.Now().Add(time.Minute), Redirect: "https://payverge.io/staff/login"}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=authcode&state="+state, nil)
	c.Request.AddCookie(&http.Cookie{Name: oauthStateCookieName, Value: state})
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: "live-owner-cookie"})
	h.GoogleStaffCallback(c)

	require.True(t, w.Code == http.StatusTemporaryRedirect || w.Code == http.StatusFound, "unverified staff Google login should redirect: %d", w.Code)
	assert.Contains(t, w.Header().Get("Location"), "staff_error=unverified_email")
	assert.False(t, hasExpiredSetCookie(w, "session_token"), "failed staff OAuth must not destroy an existing owner session")
	assert.Nil(t, findSetCookie(w, "staff_token"))
}

func TestGoogleStaffAuthURL_SetsHttpOnlyStateCookie(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)
	h.oauthConfig = &OAuthConfig{
		Google: &oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/api/v1/auth/google/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: "https://accounts.google.test/o/oauth2/auth"},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/staff", nil)
	h.GoogleStaffAuthURL(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	cookie := findSetCookie(w, oauthStateCookieName)
	require.NotNil(t, cookie)
	assert.Equal(t, body["state"], cookie.Value)
	assert.True(t, cookie.HttpOnly)
}

func setupStaffOAuthCookieHandler(t *testing.T) (*AuthHandler, *database.Staff) {
	t.Helper()
	h, db := newAuthEnvelopeHandler(t)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.StaffIdentity{},
		&database.StaffMembership{},
		&session.UserSession{},
	))
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_staff_business_email_lower ON staff (business_id, LOWER(TRIM(email)))`).Error)

	prevDB := database.GetDB()
	database.SetTestDB(db)
	prevStore := session.GlobalStore
	session.GlobalStore = nil
	t.Cleanup(func() {
		database.SetTestDB(prevDB)
		session.GlobalStore = prevStore
	})

	business := &database.Business{
		BusinessId:     "biz-staff-oauth-cookies",
		Name:           "Staff OAuth Biz",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(business).Error)
	staff := &database.Staff{
		BusinessID: business.ID,
		Email:      "staff-oauth@example.com",
		Name:       "Staff OAuth",
		Role:       database.StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, database.GetDBWrapper().StaffService.Create(staff))

	h.oauthConfig = &OAuthConfig{
		Google: &oauth2.Config{
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			RedirectURL:  "http://localhost/api/v1/auth/google/callback",
			Endpoint:     oauth2.Endpoint{AuthURL: "https://accounts.google.test/o/oauth2/auth"},
		},
	}
	return h, staff
}

func findSetCookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	// utils.writeAuthCookie uses Header().Add("Set-Cookie", ...) which
	// Response.Cookies() may not always parse in tests; read the raw header.
	// Prefer the live value — writeAuthCookie also emits Max-Age=0 evictions
	// for leftover Domain/SameSite identities of the same name.
	for _, raw := range w.Header().Values("Set-Cookie") {
		if strings.HasPrefix(raw, name+"=") && !strings.Contains(raw, "Max-Age=0") {
			parts := strings.Split(raw, ";")
			val := strings.TrimPrefix(parts[0], name+"=")
			if val == "" {
				continue
			}
			ck := &http.Cookie{Name: name, Value: val}
			for _, p := range parts[1:] {
				p = strings.TrimSpace(p)
				switch {
				case strings.EqualFold(p, "HttpOnly"):
					ck.HttpOnly = true
				case strings.EqualFold(p, "SameSite=Lax"):
					ck.SameSite = http.SameSiteLaxMode
				}
			}
			return ck
		}
	}
	return nil
}

func assertNoGoogleAuthRow(t *testing.T, db *gorm.DB, providerUserID string) {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&UserAuth{}).
		Where("provider = ? AND provider_user_id = ?", "google", providerUserID).
		Count(&count).Error)
	assert.Zero(t, count)
}

func assertOnlyEmailAuth(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	var auths []UserAuth
	require.NoError(t, db.Where("user_id = ?", userID).Find(&auths).Error)
	require.Len(t, auths, 1)
	assert.Equal(t, "email", auths[0].Provider)
}
