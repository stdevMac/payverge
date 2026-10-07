package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/session"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func verificationLatencySampleCount(t *testing.T) uint64 {
	t.Helper()
	metric, ok := metrics.EmailVerificationLatency.(prometheus.Metric)
	require.True(t, ok)
	var sample dto.Metric
	require.NoError(t, metric.Write(&sample))
	return sample.GetHistogram().GetSampleCount()
}

func withoutOperatorSessionStore(t *testing.T) {
	t.Helper()
	previous := session.GlobalStore
	session.GlobalStore = nil
	t.Cleanup(func() { session.GlobalStore = previous })
}

func TestRegisterReturnsNoOperatorCredentialUntilEmailVerification(t *testing.T) {
	withoutOperatorSessionStore(t)
	h, _, provider := newRegisterSessionHandler(t)

	w, c := postJSON(t, map[string]string{
		"email": "pending-boundary@example.com", "password": "password123", "name": "Pending",
	})
	h.Register(c)

	require.Equal(t, http.StatusCreated, w.Code)
	var body AuthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.True(t, body.VerificationRequired)
	assert.Empty(t, body.Token, "an unverified registration must not receive an operator JWT")
	for _, cookie := range w.Result().Cookies() {
		assert.NotEqual(t, "session_token", cookie.Name, "an unverified registration must not receive an operator session cookie")
		assert.NotEqual(t, "refresh_token", cookie.Name, "an unverified registration must not receive a refresh credential")
	}
	assert.GreaterOrEqual(t, provider.Count(), 1)
}

func TestVerifyEmailAtomicallyUpdatesCanonicalStateAndIssuesOneSession(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	require.NoError(t, db.AutoMigrate(&session.UserSession{}))
	previousStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = previousStore })

	user := database.User{Email: "atomic-verify@example.com", Name: "Atomic", Role: "user", AuthMethod: "email"}
	require.NoError(t, db.Create(&user).Error)
	authRecord, plaintext, err := h.authService.RegisterWithEmail(user.Email, "password123", user.Name)
	require.NoError(t, err)
	authRecord.UserID = user.ID
	require.NoError(t, db.Create(authRecord).Error)

	latencySamplesBefore := verificationLatencySampleCount(t)
	w, c := postJSON(t, map[string]string{"token": plaintext})
	h.VerifyEmail(c)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, latencySamplesBefore+1, verificationLatencySampleCount(t),
		"successful verification must emit exactly one latency observation")

	var body AuthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotEmpty(t, body.Token, "the successful verification boundary must issue the first operator JWT")
	assert.False(t, body.VerificationRequired)

	var reloadedUser database.User
	require.NoError(t, db.First(&reloadedUser, user.ID).Error)
	assert.True(t, reloadedUser.EmailVerified, "users and user_auths must commit verified state together")
	var reloadedAuth UserAuth
	require.NoError(t, db.First(&reloadedAuth, authRecord.ID).Error)
	assert.True(t, reloadedAuth.EmailVerified)
	assert.Empty(t, reloadedAuth.VerificationToken, "a successful verification token must be consumed")
	assert.Nil(t, reloadedAuth.VerificationExpiry)

	var sessionCount int64
	require.NoError(t, db.Model(&session.UserSession{}).Where("user_id = ?", user.ID).Count(&sessionCount).Error)
	assert.Equal(t, int64(1), sessionCount)

	// Replaying the same link must neither succeed nor mint another session.
	wReplay, cReplay := postJSON(t, map[string]string{"token": plaintext})
	h.VerifyEmail(cReplay)
	assert.Equal(t, http.StatusBadRequest, wReplay.Code)
	assert.Equal(t, latencySamplesBefore+1, verificationLatencySampleCount(t),
		"invalid/replayed verification must not emit latency")
	require.NoError(t, db.Model(&session.UserSession{}).Where("user_id = ?", user.ID).Count(&sessionCount).Error)
	assert.Equal(t, int64(1), sessionCount)
}

func TestConcurrentVerificationClaimsIssueExactlyOneOperatorSession(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	require.NoError(t, db.AutoMigrate(&session.UserSession{}))
	previousStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = previousStore })

	user := database.User{
		Email: fmt.Sprintf("concurrent-verify-%d@example.com", time.Now().UnixNano()),
		Name:  "Concurrent", Role: "user", AuthMethod: "email",
	}
	require.NoError(t, db.Create(&user).Error)
	authRecord, plaintext, err := h.authService.RegisterWithEmail(user.Email, "password123", user.Name)
	require.NoError(t, err)
	authRecord.UserID = user.ID
	require.NoError(t, db.Create(authRecord).Error)

	responses := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, c := postJSON(t, map[string]string{"token": plaintext})
			h.VerifyEmail(c)
			responses <- w.Code
		}()
	}
	wg.Wait()
	close(responses)

	statusCounts := map[int]int{}
	for status := range responses {
		statusCounts[status]++
	}
	assert.Equal(t, 1, statusCounts[http.StatusOK])
	assert.Equal(t, 1, statusCounts[http.StatusBadRequest])

	var sessionCount int64
	require.NoError(t, db.Model(&session.UserSession{}).Where("user_id = ? AND revoked = ?", user.ID, false).Count(&sessionCount).Error)
	assert.Equal(t, int64(1), sessionCount)
}

func TestVerificationLinksFailClosedWhenExpiredSupersededOrEmailChanged(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		h, db := newAuthEnvelopeHandler(t)
		user := database.User{Email: "expired-verify@example.com", Role: "user", AuthMethod: "email"}
		require.NoError(t, db.Create(&user).Error)
		authRecord, token, err := h.authService.RegisterWithEmail(user.Email, "password123", "Expired")
		require.NoError(t, err)
		authRecord.UserID = user.ID
		expired := time.Now().Add(-time.Minute)
		authRecord.VerificationExpiry = &expired
		require.NoError(t, db.Create(authRecord).Error)
		assert.ErrorIs(t, verifyEmailToken(h.authService, token), ErrTokenExpired)
	})

	t.Run("superseded", func(t *testing.T) {
		h, db := newAuthEnvelopeHandler(t)
		user := database.User{Email: "superseded-verify@example.com", Role: "user", AuthMethod: "email"}
		require.NoError(t, db.Create(&user).Error)
		authRecord, oldToken, err := h.authService.RegisterWithEmail(user.Email, "password123", "Superseded")
		require.NoError(t, err)
		authRecord.UserID = user.ID
		require.NoError(t, db.Create(authRecord).Error)
		newToken, err := h.authService.RotateVerificationToken(user.Email)
		require.NoError(t, err)
		assert.ErrorIs(t, verifyEmailToken(h.authService, oldToken), ErrTokenInvalid)
		require.NoError(t, verifyEmailToken(h.authService, newToken))
	})

	t.Run("canonical email changed", func(t *testing.T) {
		h, db := newAuthEnvelopeHandler(t)
		user := database.User{Email: "before-change@example.com", Role: "user", AuthMethod: "email"}
		require.NoError(t, db.Create(&user).Error)
		authRecord, token, err := h.authService.RegisterWithEmail(user.Email, "password123", "Changed")
		require.NoError(t, err)
		authRecord.UserID = user.ID
		require.NoError(t, db.Create(authRecord).Error)
		require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).Update("email", "after-change@example.com").Error)
		assert.ErrorIs(t, verifyEmailToken(h.authService, token), ErrTokenInvalid)
	})
}

func TestExpiredAbandonedRegistrationReleasesEmailReservation(t *testing.T) {
	withoutOperatorSessionStore(t)
	h, db, _ := newRegisterSessionHandler(t)

	old := time.Now().Add(-25 * time.Hour)
	user := database.User{
		Email: "abandoned@example.com", Name: "Abandoned", Role: "user", AuthMethod: "email",
		CreatedAt: old, UpdatedAt: old,
	}
	require.NoError(t, db.Create(&user).Error)
	authRecord, _, err := h.authService.RegisterWithEmail(user.Email, "password123", user.Name)
	require.NoError(t, err)
	authRecord.UserID = user.ID
	authRecord.CreatedAt = old
	authRecord.UpdatedAt = old
	expired := time.Now().Add(-time.Hour)
	authRecord.VerificationExpiry = &expired
	require.NoError(t, db.Create(authRecord).Error)

	w, c := postJSON(t, map[string]string{
		"email": user.Email, "password": "new-password123", "name": "Returned",
	})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var users []database.User
	require.NoError(t, db.Where("LOWER(email) = LOWER(?)", user.Email).Find(&users).Error)
	require.Len(t, users, 1)
	assert.NotEqual(t, user.ID, users[0].ID, "the expired placeholder must be replaced, not revived with old credentials")
}

func TestActiveUnverifiedRegistrationKeepsEmailReserved(t *testing.T) {
	withoutOperatorSessionStore(t)
	h, _, _ := newRegisterSessionHandler(t)

	wFirst, cFirst := postJSON(t, map[string]string{
		"email": "active-reservation@example.com", "password": "password123", "name": "First",
	})
	h.Register(cFirst)
	require.Equal(t, http.StatusCreated, wFirst.Code)

	wAgain, cAgain := postJSON(t, map[string]string{
		"email": "active-reservation@example.com", "password": "different123", "name": "Second",
	})
	h.Register(cAgain)
	assert.Equal(t, http.StatusConflict, wAgain.Code)
}

func TestGoogleOAuthRuleRejectsAnyUnverifiedEmailBeforeAccountCreation(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	info := &GoogleUserInfo{
		ID: "new-unverified-google", Email: "unverified-new@example.com", Name: "Unverified", VerifiedEmail: false,
	}
	authRecord := &UserAuth{Provider: "google", ProviderUserID: info.ID, EmailVerified: false}

	_, _, err := h.resolveGoogleCallbackUserWithInvite(context.Background(), info, authRecord, true, false, 0, "")
	assert.ErrorIs(t, err, ErrUnverifiedOAuthEmail)
	var users int64
	require.NoError(t, db.Model(&database.User{}).Where("LOWER(email) = LOWER(?)", info.Email).Count(&users).Error)
	assert.Zero(t, users)
}

func TestResendRateLimitKeepsLastIssuedLinkUsable(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	user := database.User{Email: "resend-limit@example.com", Role: "user", AuthMethod: "email"}
	require.NoError(t, db.Create(&user).Error)
	authRecord, _, err := h.authService.RegisterWithEmail(user.Email, "password123", "Resend")
	require.NoError(t, err)
	authRecord.UserID = user.ID
	require.NoError(t, db.Create(authRecord).Error)

	var last string
	for i := 0; i < 3; i++ {
		last, err = h.authService.ResendVerification(user.Email)
		require.NoError(t, err)
		require.NotEmpty(t, last)
	}
	rateLimited, err := h.authService.ResendVerification(user.Email)
	require.NoError(t, err)
	assert.Empty(t, rateLimited)
	require.NoError(t, verifyEmailToken(h.authService, last), "rate limiting must not rotate away the last link")
}

func TestVerificationURLCarriesOnlyOpaqueToken(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)
	link := h.buildVerificationLink("opaque-token")
	parsed, err := url.Parse(link)
	require.NoError(t, err)
	assert.Equal(t, "opaque-token", parsed.Query().Get("token"))
	assert.Empty(t, parsed.Query().Get("email"))
	assert.False(t, errors.Is(ErrTokenInvalid, ErrTokenExpired))
}
