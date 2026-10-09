package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/verification"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setEmailMode sets the email env that decides EMAIL_VERIFICATION for one test.
func setEmailMode(t *testing.T, provider, mode string) {
	t.Helper()
	for _, key := range []string{config.ResendAPIKeyEnv, config.EmailAPIKeyEnv} {
		t.Setenv(key, "")
	}
	t.Setenv(config.EmailProviderEnv, provider)
	t.Setenv(config.EmailVerificationEnv, mode)
}

func registerForModeTest(t *testing.T, h *AuthHandler, email string) (*http.Response, AuthResponse) {
	t.Helper()
	w, c := postJSON(t, map[string]string{"email": email, "password": "password123", "name": "Owner"})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var body AuthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w.Result(), body
}

func hasSessionCookie(resp *http.Response) bool {
	for _, ck := range resp.Cookies() {
		if ck.Name == "session_token" && ck.Value != "" {
			return true
		}
	}
	return false
}

// assertSignedInUnproven checks the off-mode contract: the account can hold a
// session (the middleware's live check accepts it) while both flags stay a
// truthful "nobody proved this address" (false), with no token lingering.
func assertSignedInUnproven(t *testing.T, h *AuthHandler, email string) {
	t.Helper()
	var authRec UserAuth
	require.NoError(t, h.db.Where("provider = ? AND provider_user_id = ?", "email", email).First(&authRec).Error)
	assert.False(t, authRec.EmailVerified, "off mode must not write user_auths.email_verified")
	assert.Empty(t, authRec.VerificationToken, "off mode mints no verification token")
	assert.Nil(t, authRec.VerificationExpiry)

	var user database.User
	require.NoError(t, h.db.First(&user, authRec.UserID).Error)
	assert.False(t, user.EmailVerified, "off mode must not write users.email_verified")

	ok, err := verification.NewService(h.db).IsOperatorVerified(user.ID, "email")
	require.NoError(t, err)
	assert.True(t, ok, "session middleware must accept the off-mode identity")
}

func TestRegisterWithVerificationOffSignsInWithoutMarkingVerified(t *testing.T) {
	setEmailMode(t, "smtp", "off")
	h, _, provider := newRegisterSessionHandler(t)

	resp, body := registerForModeTest(t, h, "owner@example.com")

	assert.True(t, body.Success)
	assert.NotEmpty(t, body.Token, "off mode signs the new operator straight in")
	assert.False(t, body.VerificationRequired)
	assert.Empty(t, body.VerificationURL)
	assert.True(t, hasSessionCookie(resp), "off mode sets the normal session cookie")
	assert.Equal(t, 0, provider.Count(), "no verification email in off mode")
	assertSignedInUnproven(t, h, "owner@example.com")
}

func TestRegisterWithVerificationAutoFollowsTheProvider(t *testing.T) {
	cases := []struct {
		provider     string
		wantSignedIn bool
	}{
		{provider: "log", wantSignedIn: true},
		{provider: "", wantSignedIn: true}, // nothing configured → log
		{provider: "smtp", wantSignedIn: false},
		{provider: "resend", wantSignedIn: false},
	}
	for _, tc := range cases {
		t.Run("provider="+tc.provider, func(t *testing.T) {
			setEmailMode(t, tc.provider, "auto")
			h, _, provider := newRegisterSessionHandler(t)

			resp, body := registerForModeTest(t, h, "auto@example.com")

			if tc.wantSignedIn {
				assert.NotEmpty(t, body.Token)
				assert.False(t, body.VerificationRequired)
				assert.True(t, hasSessionCookie(resp))
				assert.Equal(t, 0, provider.Count())
				assertSignedInUnproven(t, h, "auto@example.com")
				return
			}
			assert.Empty(t, body.Token)
			assert.True(t, body.VerificationRequired)
			assert.False(t, hasSessionCookie(resp))
			assert.GreaterOrEqual(t, provider.Count(), 1, "verification email must be sent")
		})
	}
}

// TestRegisterWithVerificationRequiredOverridesTheLogProvider: an operator may
// keep the hosted contract on a log-only box (links are read from the logs).
func TestRegisterWithVerificationRequiredOverridesTheLogProvider(t *testing.T) {
	setEmailMode(t, "log", "required")
	h, _, provider := newRegisterSessionHandler(t)

	resp, body := registerForModeTest(t, h, "strict@example.com")

	assert.Empty(t, body.Token)
	assert.True(t, body.VerificationRequired)
	assert.False(t, hasSessionCookie(resp))
	assert.GreaterOrEqual(t, provider.Count(), 1)

	w, c := postJSON(t, map[string]string{"email": "strict@example.com", "password": "password123"})
	h.Login(c)
	assert.Equal(t, http.StatusForbidden, w.Code, "required mode keeps the unverified-login 403")
}

// TestLoginWithVerificationOffSignsInAPendingAccount covers the upgrade path:
// an account registered while verification was required can sign in once the
// operator switches EMAIL_VERIFICATION=off. Nothing is converged: its flags
// stay false (and its pending link stays usable) because nobody proved the
// address, and the middleware's live check accepts it on the binding alone.
func TestLoginWithVerificationOffSignsInAPendingAccount(t *testing.T) {
	setEmailMode(t, "smtp", "required")
	h, _, _ := newRegisterSessionHandler(t)
	_, body := registerForModeTest(t, h, "pending@example.com")
	require.True(t, body.VerificationRequired)

	t.Setenv(config.EmailVerificationEnv, "off")
	w, c := postJSON(t, map[string]string{"email": "pending@example.com", "password": "password123"})
	h.Login(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var login AuthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &login))
	assert.NotEmpty(t, login.Token)
	assert.True(t, hasSessionCookie(w.Result()))

	var authRec UserAuth
	require.NoError(t, h.db.Where("provider = ? AND provider_user_id = ?", "email", "pending@example.com").First(&authRec).Error)
	assert.False(t, authRec.EmailVerified, "off-mode login must not verify the pending identity")
	assert.NotEmpty(t, authRec.VerificationToken, "the emailed link stays valid")
	var user database.User
	require.NoError(t, h.db.First(&user, authRec.UserID).Error)
	assert.False(t, user.EmailVerified)
}

// TestSwitchingVerificationBackToRequiredGatesOffModeAccounts: accounts
// created while verification was off were never marked verified, so turning
// it on again makes them verify like any pending account — their sessions
// stop passing the live check and the next login mails a verification link.
func TestSwitchingVerificationBackToRequiredGatesOffModeAccounts(t *testing.T) {
	setEmailMode(t, "smtp", "off")
	h, _, provider := newRegisterSessionHandler(t)
	_, body := registerForModeTest(t, h, "squat@example.com")
	require.NotEmpty(t, body.Token)
	require.Equal(t, 0, provider.Count())

	t.Setenv(config.EmailVerificationEnv, "required")
	ok, err := verification.NewService(h.db).IsOperatorVerified(body.UserID, "email")
	require.NoError(t, err)
	assert.False(t, ok, "an off-mode session must stop passing the live check under required")

	w, c := postJSON(t, map[string]string{"email": "squat@example.com", "password": "password123"})
	h.Login(c)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "requires_email_verification")
	assert.False(t, hasSessionCookie(w.Result()))
	assert.GreaterOrEqual(t, provider.Count(), 1, "the login mails a fresh verification link")
}

// TestLoginWithVerificationOffRefusesAStaleIdentity covers an unverified
// email identity left behind by an email change: off mode must not vouch for
// an address the account no longer holds, and must answer 403, not 500.
func TestLoginWithVerificationOffRefusesAStaleIdentity(t *testing.T) {
	setEmailMode(t, "smtp", "required")
	h, _, _ := newRegisterSessionHandler(t)
	registerForModeTest(t, h, "old@example.com")
	var authRec UserAuth
	require.NoError(t, h.db.Where("provider = ? AND provider_user_id = ?", "email", "old@example.com").First(&authRec).Error)
	require.NoError(t, h.db.Model(&database.User{}).Where("id = ?", authRec.UserID).Update("email", "new@example.com").Error)

	t.Setenv(config.EmailVerificationEnv, "off")
	w, c := postJSON(t, map[string]string{"email": "old@example.com", "password": "password123"})
	h.Login(c)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.False(t, hasSessionCookie(w.Result()))
	var user database.User
	require.NoError(t, h.db.First(&user, authRec.UserID).Error)
	assert.False(t, user.EmailVerified, "the stale identity must not verify the account")

	// The account's current address has no email credential either, so a
	// login with it is plain bad credentials.
	w, c = postJSON(t, map[string]string{"email": "new@example.com", "password": "password123"})
	h.Login(c)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

// TestLoginRefusesAVerifiedCredentialOnAnUnverifiedAccountWith403 (EMAIL-7):
// an inconsistent pair — verified email credential, unverified users row —
// is refused like any unverified account (403), not reported as a 500.
func TestLoginRefusesAVerifiedCredentialOnAnUnverifiedAccountWith403(t *testing.T) {
	setEmailMode(t, "smtp", "required")
	h, _, _ := newRegisterSessionHandler(t)
	registerForModeTest(t, h, "half@example.com")
	require.NoError(t, h.db.Model(&UserAuth{}).Where("provider = ? AND provider_user_id = ?", "email", "half@example.com").
		Updates(map[string]interface{}{"email_verified": true, "verification_token": "", "verification_expiry": nil}).Error)

	w, c := postJSON(t, map[string]string{"email": "half@example.com", "password": "password123"})
	h.Login(c)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.False(t, hasSessionCookie(w.Result()))
}

func TestLoginWithVerificationOffStillRejectsAWrongPassword(t *testing.T) {
	setEmailMode(t, "log", "off")
	h, _, _ := newRegisterSessionHandler(t)
	registerForModeTest(t, h, "owner@example.com")

	w, c := postJSON(t, map[string]string{"email": "owner@example.com", "password": "wrong-password"})
	h.Login(c)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthServiceRegisterWithEmailHonoursVerificationMode(t *testing.T) {
	h, _, _ := newRegisterSessionHandler(t)

	setEmailMode(t, "log", "off")
	authOff, tokenOff, err := h.authService.RegisterWithEmail("off@example.com", "password123", "Off")
	require.NoError(t, err)
	assert.False(t, authOff.EmailVerified, "off mode never records unproven ownership as verified")
	assert.Empty(t, tokenOff)
	assert.Empty(t, authOff.VerificationToken)
	assert.Nil(t, authOff.VerificationExpiry)
	assert.True(t, CheckPassword("password123", authOff.PasswordHash))

	setEmailMode(t, "log", "required")
	authReq, tokenReq, err := h.authService.RegisterWithEmail("req@example.com", "password123", "Req")
	require.NoError(t, err)
	assert.False(t, authReq.EmailVerified)
	assert.NotEmpty(t, tokenReq)
	assert.Equal(t, HashToken(tokenReq), authReq.VerificationToken, "only the hash is stored")
	require.NotNil(t, authReq.VerificationExpiry)
}

func TestAuthServiceLoginWithEmailHonoursVerificationMode(t *testing.T) {
	setEmailMode(t, "smtp", "required")
	h, _, _ := newRegisterSessionHandler(t)
	registerForModeTest(t, h, "login@example.com")

	_, err := h.authService.LoginWithEmail("login@example.com", "password123")
	assert.ErrorIs(t, err, ErrEmailNotVerified)

	t.Setenv(config.EmailVerificationEnv, "off")
	rec, err := h.authService.LoginWithEmail("login@example.com", "password123")
	require.NoError(t, err)
	assert.False(t, rec.EmailVerified, "the service only lets it through; nothing is converged")
}

// googleIdentityFor builds the callback inputs for a Google account whose
// address Google verified.
func googleIdentityFor(email, googleID string) (*GoogleUserInfo, *UserAuth) {
	info := &GoogleUserInfo{ID: googleID, Email: email, VerifiedEmail: true, Name: "Google Owner"}
	return info, &UserAuth{Provider: "google", ProviderUserID: googleID, EmailVerified: true}
}

func createUserSessionsTable(t *testing.T, h *AuthHandler) {
	t.Helper()
	require.NoError(t, h.db.Exec("CREATE TABLE IF NOT EXISTS user_sessions (id INTEGER PRIMARY KEY, user_id INTEGER)").Error)
}

// TestGoogleMergeRefusesAnOffModeSquat is the EMAIL-1 regression: with
// verification off anyone can register someone else's address and is signed
// straight in. When the real owner later arrives through Google (which did
// verify the address), the Google identity must not be merged into the
// squatter's account — the squatter's password would keep working on it.
func TestGoogleMergeRefusesAnOffModeSquat(t *testing.T) {
	setEmailMode(t, "log", "auto") // zero-config: log provider, verification off
	h, _, _ := newRegisterSessionHandler(t)
	createUserSessionsTable(t, h)
	_, body := registerForModeTest(t, h, "victim@example.com")
	require.NotEmpty(t, body.Token, "the squatter is signed in")
	// The real session store records the squatter's session.
	require.NoError(t, h.db.Exec("INSERT INTO user_sessions (user_id) VALUES (?)", body.UserID).Error)

	info, googleAuth := googleIdentityFor("victim@example.com", "victim-google-id")
	_, _, err := h.resolveGoogleCallbackUserWithInvite(context.Background(), info, googleAuth, true, false, 0, "")
	require.ErrorIs(t, err, ErrUnprovenEmailAccount)

	var googleRows int64
	require.NoError(t, h.db.Model(&UserAuth{}).Where("provider = ?", "google").Count(&googleRows).Error)
	assert.Zero(t, googleRows, "no Google identity may be attached to the squatted account")
	var emailRows int64
	require.NoError(t, h.db.Model(&UserAuth{}).Where("user_id = ? AND provider = ?", body.UserID, "email").Count(&emailRows).Error)
	assert.Equal(t, int64(1), emailRows, "a refused merge leaves the account as it was")
}

// TestGoogleMergeIntoAnUnusedPendingAccountDropsTheUnprovenPassword keeps the
// hosted (required mode) flow working: an account still waiting for its link
// and never signed in to is merged, but the password nobody proved is
// deleted first, so whoever chose it cannot sign in to the merged account.
func TestGoogleMergeIntoAnUnusedPendingAccountDropsTheUnprovenPassword(t *testing.T) {
	setEmailMode(t, "smtp", "required")
	h, _, _ := newRegisterSessionHandler(t)
	createUserSessionsTable(t, h)
	_, body := registerForModeTest(t, h, "pending@example.com")
	require.True(t, body.VerificationRequired)

	info, googleAuth := googleIdentityFor("pending@example.com", "pending-google-id")
	user, isNew, err := h.resolveGoogleCallbackUserWithInvite(context.Background(), info, googleAuth, true, false, 0, "")
	require.NoError(t, err)
	assert.False(t, isNew)
	assert.Equal(t, body.UserID, user.ID, "the Google identity joins the existing account")

	var emailRows int64
	require.NoError(t, h.db.Model(&UserAuth{}).Where("user_id = ? AND provider = ?", user.ID, "email").Count(&emailRows).Error)
	assert.Zero(t, emailRows, "the unproven password credential is gone")

	// Even after an operator switches verification off, that password no
	// longer signs in to the merged account.
	t.Setenv(config.EmailVerificationEnv, "off")
	w, c := postJSON(t, map[string]string{"email": "pending@example.com", "password": "password123"})
	h.Login(c)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

// TestGoogleLinkFromTheSameAccountKeepsItsPassword: explicit link mode,
// started from a session on the account itself, already proved control of it,
// so the off-mode guard neither refuses nor drops anything.
func TestGoogleLinkFromTheSameAccountKeepsItsPassword(t *testing.T) {
	setEmailMode(t, "log", "off")
	h, _, _ := newRegisterSessionHandler(t)
	createUserSessionsTable(t, h)
	_, body := registerForModeTest(t, h, "linker@example.com")
	require.NoError(t, h.db.Exec("INSERT INTO user_sessions (user_id) VALUES (?)", body.UserID).Error)

	info, googleAuth := googleIdentityFor("linker@example.com", "linker-google-id")
	user, _, err := h.resolveGoogleCallbackUserWithInvite(context.Background(), info, googleAuth, true, true, body.UserID, "")
	require.NoError(t, err)
	assert.Equal(t, body.UserID, user.ID)

	var rows int64
	require.NoError(t, h.db.Model(&UserAuth{}).Where("user_id = ?", user.ID).Count(&rows).Error)
	assert.Equal(t, int64(2), rows, "email credential kept, Google identity linked")
}

func TestUnprovenEmailAccountMessageFollowsTheVerificationMode(t *testing.T) {
	setEmailMode(t, "smtp", "required")
	required := unprovenEmailAccountMessage()
	assert.Contains(t, required, "verification link")
	assert.NotContains(t, required, "Sign in with its password and link Google")

	t.Setenv(config.EmailVerificationEnv, "off")
	off := unprovenEmailAccountMessage()
	assert.Contains(t, off, "Sign in with its password and link Google")
}
