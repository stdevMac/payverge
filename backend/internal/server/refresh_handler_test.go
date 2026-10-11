package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

func requireRefreshOutcomeDelta(t *testing.T, realm metrics.RefreshRealm, expected metrics.RefreshOutcome, invoke func()) {
	t.Helper()
	before := make(map[metrics.RefreshOutcome]float64)
	for _, outcome := range metrics.RefreshOutcomeValues() {
		before[outcome] = metrics.CurrentRefreshOutcome(realm, outcome)
	}

	invoke()

	totalDelta := 0.0
	for _, outcome := range metrics.RefreshOutcomeValues() {
		after := metrics.CurrentRefreshOutcome(realm, outcome)
		delta := after - before[outcome]
		if outcome == expected {
			require.Equal(t, 1.0, delta, "expected one %s refresh outcome", outcome)
		} else {
			require.Equal(t, 0.0, delta, "unexpected %s refresh outcome", outcome)
		}
		totalDelta += delta
	}
	require.Equal(t, 1.0, totalDelta, "one request must record exactly one terminal outcome")
}

func injectTransientQueryError(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	callbackName := "payverge:test:transient_lookup:" + t.Name()
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == table {
			_ = tx.AddError(errors.New("transient identity lookup failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })
}

func setupRefreshHandlerTest(t testing.TB) (*gorm.DB, *database.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	prevDB := database.GetDB()
	prevStore := session.GlobalStore
	prevSecret := structs.SecretKey

	// Unique DSN per setup so -count=N benchmarks (and parallel tests) do not
	// collide on shared-cache in-memory SQLite or UNIQUE(email).
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		database.SetTestDB(prevDB)
		session.GlobalStore = prevStore
		structs.SecretKey = prevSecret
		require.NoError(t, sqlDB.Close())
	})
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&database.User{}, &session.UserSession{}, &operatorEmailAuthState{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE user_session_refresh_history (
			session_id integer NOT NULL,
			token_hash text NOT NULL,
			rotated_at datetime NOT NULL,
			expires_at datetime NOT NULL DEFAULT '2099-01-01 00:00:00',
			PRIMARY KEY (session_id, token_hash),
			UNIQUE (token_hash),
			FOREIGN KEY (session_id) REFERENCES user_sessions(id) ON DELETE CASCADE
		)
	`).Error)

	database.SetTestDB(db)
	session.GlobalStore = session.NewStore(db)
	structs.SecretKey = []byte("test-secret-key-for-refresh-handler-tests")

	user := &database.User{Email: "owner@example.com", Address: "0xowner", Role: "user", EmailVerified: true}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&operatorEmailAuthState{
		UserID: user.ID, Provider: "email", ProviderUserID: user.Email, EmailVerified: true,
	}).Error)
	return db, user
}

func seedRefreshSession(t testing.TB, userID uint, refreshRaw string) *session.UserSession {
	t.Helper()
	uid := userID
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &uid,
		TokenHash: session.HashToken("sess-" + refreshRaw),
		Provider:  "email",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))
	return sess
}

// callRefresh invokes RefreshToken with the given refresh_token cookie and
// returns the recorder plus the new refresh_token cookie value (empty if none).
func callRefresh(t *testing.T, refreshRaw string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshRaw})
	c.Request = req

	RefreshToken(c)

	newRefresh := ""
	for _, ck := range w.Result().Cookies() {
		if ck.Name == "refresh_token" && ck.Value != "" {
			newRefresh = ck.Value
		}
	}
	return w, newRefresh
}

// TestRefreshToken_RotationRecordsPrevious locks AUTH-1b: a successful refresh
// rotates the refresh token AND records the rotated-away hash as previous, so a
// later replay is detectable.
func TestRefreshToken_RotationRecordsPrevious(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	sess := seedRefreshSession(t, user.ID, "refresh-1")

	var w *httptest.ResponseRecorder
	var newRefresh string
	requireRefreshOutcomeDelta(t, metrics.RefreshRealmOperator, metrics.RefreshOutcomeSuccess, func() {
		w, newRefresh = callRefresh(t, "refresh-1")
	})
	require.Equal(t, http.StatusOK, w.Code, "valid refresh succeeds")
	require.NotEmpty(t, newRefresh, "a new refresh cookie is issued")
	require.NotEqual(t, "refresh-1", newRefresh, "the refresh token is rotated")

	var row session.UserSession
	require.NoError(t, db.First(&row, sess.ID).Error)
	require.Equal(t, session.HashToken(newRefresh), row.RefreshToken, "refresh hash advanced")
	require.Equal(t, session.HashToken("refresh-1"), row.PreviousRefreshToken, "old hash recorded as previous")
	require.False(t, row.Revoked)
}

// TestRefreshToken_PersistsAcrossRestart models a deployment restart: the
// process-owned Store, signing-key cache, and Gin router are rebuilt while the
// database and JWT_SECRET_KEY remain stable. The pre-restart cookies must keep
// working and rotate through the newly-created handler stack.
func TestRefreshToken_PersistsAcrossRestart(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	const jwtSecret = "stable-jwt-secret-across-process-recreation"
	t.Setenv("JWT_SECRET_KEY", jwtSecret)

	// Load the signing key as the first process would. JWT signing keys are not
	// session data: changing JWT_SECRET_KEY intentionally invalidates existing
	// access JWT signatures, and secret rotation is outside this regression.
	structs.SecretKey = nil
	require.Equal(t, []byte(jwtSecret), structs.GetSecretKey())

	uid := user.ID
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &uid,
		TokenHash: session.HashToken("access-placeholder"),
		Provider:  "email",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	require.NoError(t, err)

	accessBeforeRestart, err := GenerateUserToken(user.ID, user.Email, user.Address, user.Role, sess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(accessBeforeRestart)))
	const refreshBeforeRestart = "refresh-cookie-before-restart"
	require.NoError(t, session.GlobalStore.RotateRefreshToken(
		sess.ID,
		session.HashToken(refreshBeforeRestart),
		time.Now().Add(7*24*time.Hour),
	))

	// Simulate a replacement process reloading the same deployment secret and
	// constructing fresh process-local objects over the persistent database.
	session.GlobalStore = nil
	structs.SecretKey = nil
	require.Equal(t, []byte(jwtSecret), structs.GetSecretKey())
	session.GlobalStore = session.NewStore(db)

	claims, err := VerifyToken(accessBeforeRestart)
	require.NoError(t, err, "the stable secret verifies the pre-restart access JWT")
	require.Equal(t, float64(sess.ID), claims["session_id"])
	valid, err := session.GlobalStore.Validate(sess.ID, session.HashToken(accessBeforeRestart))
	require.NoError(t, err)
	require.True(t, valid, "the rebuilt store validates the pre-restart access hash")

	router := gin.New()
	router.POST("/auth/refresh", RefreshToken)
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshBeforeRestart})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "the pre-restart refresh cookie succeeds")
	var accessAfterRestart, refreshAfterRestart string
	for _, cookie := range w.Result().Cookies() {
		switch cookie.Name {
		case "session_token":
			accessAfterRestart = cookie.Value
		case "refresh_token":
			refreshAfterRestart = cookie.Value
		}
	}
	require.NotEmpty(t, accessAfterRestart, "the restarted router issues a new access cookie")
	require.NotEmpty(t, refreshAfterRestart, "the restarted router issues a new refresh cookie")
	require.NotEqual(t, refreshBeforeRestart, refreshAfterRestart)

	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	require.Equal(t, session.HashToken(accessAfterRestart), persisted.SessionToken)
	require.Equal(t, session.HashToken(refreshAfterRestart), persisted.RefreshToken)
	require.Equal(t, session.HashToken(refreshBeforeRestart), persisted.PreviousRefreshToken)
}

// TestRefreshToken_RecentReuseDoesNotRevoke locks the false-positive guard: a
// replay of a token rotated 60 seconds ago is a benign delayed browser retry and
// is rejected WITHOUT revoking the session.
func TestRefreshToken_RecentReuseDoesNotRevoke(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	sess := seedRefreshSession(t, user.ID, "refresh-1")

	w, refresh2 := callRefresh(t, "refresh-1")
	require.Equal(t, http.StatusOK, w.Code)
	require.NotEmpty(t, refresh2)

	// Replay refresh-1 after the observed one-minute browser retry delay.
	recent := time.Now().Add(-60 * time.Second)
	require.NoError(t, db.Table("user_session_refresh_history").
		Where("session_id = ? AND token_hash = ?", sess.ID, session.HashToken("refresh-1")).
		Update("rotated_at", recent).Error)

	var w2 *httptest.ResponseRecorder
	requireRefreshOutcomeDelta(t, metrics.RefreshRealmOperator, metrics.RefreshOutcomeRotated, func() {
		w2, _ = callRefresh(t, "refresh-1")
	})
	require.Equal(t, http.StatusUnauthorized, w2.Code, "the delayed retry is rejected")
	require.Contains(t, w2.Body.String(), ErrCodeRefreshRotated,
		"benign retry must emit AUTH_REFRESH_ROTATED so the FE does not clear the session hint")

	var row session.UserSession
	require.NoError(t, db.First(&row, sess.ID).Error)
	require.False(t, row.Revoked, "a benign delayed retry must NOT revoke the session")
}

// TestRefreshToken_StaleReuseRevokesSession locks the breach-detection: replaying
// a rotated-away token well after the rotation (outside the grace window) is a
// stolen-token signal that revokes the session family.
func TestRefreshToken_StaleReuseRevokesSession(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	sess := seedRefreshSession(t, user.ID, "refresh-1")

	w, refresh2 := callRefresh(t, "refresh-1")
	require.Equal(t, http.StatusOK, w.Code)
	require.NotEmpty(t, refresh2)

	// Age the rotation beyond 90 seconds so the next replay is treated as theft.
	stale := time.Now().Add(-91 * time.Second)
	require.NoError(t, db.Table("user_session_refresh_history").
		Where("session_id = ? AND token_hash = ?", sess.ID, session.HashToken("refresh-1")).
		Update("rotated_at", stale).Error)

	var w2 *httptest.ResponseRecorder
	requireRefreshOutcomeDelta(t, metrics.RefreshRealmOperator, metrics.RefreshOutcomeReuse, func() {
		w2, _ = callRefresh(t, "refresh-1")
	})
	require.Equal(t, http.StatusUnauthorized, w2.Code, "a stale replayed token is rejected")
	require.Contains(t, w2.Body.String(), ErrCodeSessionUnknown,
		"stale reuse must return the revoked/unknown-session response")

	var row session.UserSession
	require.NoError(t, db.First(&row, sess.ID).Error)
	require.True(t, row.Revoked, "stale reuse detection revokes the session family")
	require.Equal(t, string(session.RevocationReasonRefreshTokenReuse), row.RevocationReason)
}

func TestRefreshToken_OlderRecentGenerationReturnsRotatedWithoutRevoking(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	sess := seedRefreshSession(t, user.ID, "refresh-R1")

	w1, refreshR2 := callRefresh(t, "refresh-R1")
	require.Equal(t, http.StatusOK, w1.Code)
	require.NotEmpty(t, refreshR2)
	w2, refreshR3 := callRefresh(t, refreshR2)
	require.Equal(t, http.StatusOK, w2.Code)
	require.NotEmpty(t, refreshR3)

	replay, minted := callRefresh(t, "refresh-R1")
	require.Equal(t, http.StatusUnauthorized, replay.Code)
	require.Contains(t, replay.Body.String(), ErrCodeRefreshRotated)
	require.Empty(t, minted, "a historical token must never mint credentials")

	var row session.UserSession
	require.NoError(t, db.First(&row, sess.ID).Error)
	require.False(t, row.Revoked, "a recent older-generation replay is benign")
	require.Empty(t, row.RevocationReason)
}

func TestRefreshToken_OlderStaleGenerationRevokesProviderFamily(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	sess := seedRefreshSession(t, user.ID, "refresh-R1-stale")
	staffSession := seedRefreshSession(t, user.ID, "unrelated-provider")
	require.NoError(t, db.Model(&session.UserSession{}).Where("id = ?", staffSession.ID).
		Update("provider", "staff").Error)

	w1, refreshR2 := callRefresh(t, "refresh-R1-stale")
	require.Equal(t, http.StatusOK, w1.Code)
	w2, _ := callRefresh(t, refreshR2)
	require.Equal(t, http.StatusOK, w2.Code)
	require.NoError(t, db.Table("user_session_refresh_history").
		Where("session_id = ? AND token_hash = ?", sess.ID, session.HashToken("refresh-R1-stale")).
		Update("rotated_at", time.Now().Add(-session.RefreshReuseGracePeriod-time.Second)).Error)

	replay, minted := callRefresh(t, "refresh-R1-stale")
	require.Equal(t, http.StatusUnauthorized, replay.Code)
	require.Contains(t, replay.Body.String(), ErrCodeSessionUnknown)
	require.Empty(t, minted)

	var row, unrelated session.UserSession
	require.NoError(t, db.First(&row, sess.ID).Error)
	require.True(t, row.Revoked)
	require.Equal(t, string(session.RevocationReasonRefreshTokenReuse), row.RevocationReason)
	require.NoError(t, db.First(&unrelated, staffSession.ID).Error)
	require.False(t, unrelated.Revoked, "reuse revocation stays provider-scoped")
}

func TestRefreshToken_RevokeFailureReturns500WithoutClaimingRevoked(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	sess := seedRefreshSession(t, user.ID, "refresh-revoke-failure")
	w, _ := callRefresh(t, "refresh-revoke-failure")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, db.Table("user_session_refresh_history").Where("session_id = ?", sess.ID).
		Updates(map[string]interface{}{
			"rotated_at": time.Now().Add(-session.RefreshReuseGracePeriod - time.Second),
			"expires_at": time.Now().Add(time.Hour),
		}).Error)
	callbackName := "payverge:test:refresh-revoke-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "user_sessions" {
			_ = tx.AddError(errors.New("transient revocation write failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })
	replay, minted := callRefresh(t, "refresh-revoke-failure")
	require.Equal(t, http.StatusInternalServerError, replay.Code)
	require.NotContains(t, replay.Body.String(), ErrCodeSessionUnknown)
	require.Empty(t, minted)
	var row session.UserSession
	require.NoError(t, db.First(&row, sess.ID).Error)
	require.False(t, row.Revoked)
	require.Empty(t, row.RevocationReason)
}

func TestRefreshToken_SessionStoreOperationalErrorsReturn500WithoutRevocation(t *testing.T) {
	for _, failureQuery := range []int{1, 2} {
		name := "validation"
		if failureQuery == 2 {
			name = "history lookup"
		}
		t.Run(name, func(t *testing.T) {
			db, user := setupRefreshHandlerTest(t)
			sess := seedRefreshSession(t, user.ID, "operational-"+name)
			if failureQuery == 2 {
				require.NoError(t, db.Model(&session.UserSession{}).Where("id = ?", sess.ID).
					Update("refresh_token", session.HashToken("different-live-token")).Error)
			}

			callbackName := "payverge:test:session-store:" + t.Name()
			queryCount := 0
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Schema != nil &&
					(tx.Statement.Schema.Table == "user_sessions" || tx.Statement.Schema.Table == "user_session_refresh_history") {
					queryCount++
					if queryCount == failureQuery {
						_ = tx.AddError(errors.New("transient session store failure"))
					}
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

			var w *httptest.ResponseRecorder
			var minted string
			requireRefreshOutcomeDelta(t, metrics.RefreshRealmOperator, metrics.RefreshOutcomeStoreError, func() {
				w, minted = callRefresh(t, "operational-"+name)
			})
			require.Equal(t, http.StatusInternalServerError, w.Code)
			require.Empty(t, minted)

			var row session.UserSession
			require.NoError(t, db.First(&row, sess.ID).Error)
			require.False(t, row.Revoked)
			require.Empty(t, row.RevocationReason)
		})
	}
}

func TestRefreshToken_MissingCookieRecordsInvalidOutcome(t *testing.T) {
	setupRefreshHandlerTest(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)

	requireRefreshOutcomeDelta(t, metrics.RefreshRealmOperator, metrics.RefreshOutcomeInvalid, func() {
		RefreshToken(c)
	})

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRefreshToken_Web3ProvidersWithoutIdentityRecordInvalidSession(t *testing.T) {
	for _, provider := range []string{"web3", "dynamic", "dynamic_web3"} {
		t.Run(provider, func(t *testing.T) {
			db, _ := setupRefreshHandlerTest(t)
			refreshRaw := "malformed-" + provider
			sess, err := session.GlobalStore.Create(session.CreateInput{
				TokenHash: session.HashToken("access-" + provider),
				Provider:  provider,
				ExpiresAt: time.Now().Add(time.Hour),
			})
			require.NoError(t, err)
			require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))

			w, _ := callRefresh(t, refreshRaw)
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Contains(t, w.Body.String(), ErrCodeTokenInvalid)

			var persisted session.UserSession
			require.NoError(t, db.First(&persisted, sess.ID).Error)
			require.True(t, persisted.Revoked)
			require.Equal(t, string(session.RevocationReasonInvalidSession), persisted.RevocationReason)
		})
	}
}

func TestRefreshToken_Web3MissingOrDeletedIdentityRecordsAccountDisabled(t *testing.T) {
	missingUserID := uint(999999)
	tests := []struct {
		name     string
		provider string
		userID   *uint
		address  string
		deleted  bool
	}{
		{name: "missing user id", provider: "web3", userID: &missingUserID},
		{name: "missing address", provider: "dynamic", address: "0xmissing"},
		{name: "deleted user", provider: "dynamic_web3", deleted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, user := setupRefreshHandlerTest(t)
			userID := tt.userID
			address := tt.address
			if tt.deleted {
				deletedAt := time.Now().UTC()
				require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).Update("deleted_at", deletedAt).Error)
				userID = &user.ID
			}

			refreshRaw := "disabled-" + tt.provider
			sess, err := session.GlobalStore.Create(session.CreateInput{
				UserID:    userID,
				Address:   address,
				TokenHash: session.HashToken("access-" + tt.provider),
				Provider:  tt.provider,
				ExpiresAt: time.Now().Add(time.Hour),
			})
			require.NoError(t, err)
			require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))

			w, _ := callRefresh(t, refreshRaw)
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Contains(t, w.Body.String(), ErrCodeTokenInvalid)

			var persisted session.UserSession
			require.NoError(t, db.First(&persisted, sess.ID).Error)
			require.True(t, persisted.Revoked)
			require.Equal(t, string(session.RevocationReasonAccountDisabled), persisted.RevocationReason)
		})
	}
}

func TestRefreshToken_DeletedEmailProvidersRecordAccountDisabled(t *testing.T) {
	for _, provider := range []string{"email", "google", "register"} {
		t.Run(provider, func(t *testing.T) {
			db, user := setupRefreshHandlerTest(t)
			deletedAt := time.Now().UTC()
			require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).Update("deleted_at", deletedAt).Error)

			refreshRaw := "deleted-" + provider
			sess, err := session.GlobalStore.Create(session.CreateInput{
				UserID:    &user.ID,
				TokenHash: session.HashToken("access-" + provider),
				Provider:  provider,
				ExpiresAt: time.Now().Add(time.Hour),
			})
			require.NoError(t, err)
			require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))

			w, _ := callRefresh(t, refreshRaw)
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Contains(t, w.Body.String(), ErrCodeTokenInvalid)

			var persisted session.UserSession
			require.NoError(t, db.First(&persisted, sess.ID).Error)
			require.True(t, persisted.Revoked)
			require.Equal(t, string(session.RevocationReasonAccountDisabled), persisted.RevocationReason)
		})
	}
}

func TestRefreshToken_TransientUserLookupDoesNotRevoke(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		byAddress bool
	}{
		{name: "email user id", provider: "email"},
		{name: "web3 user id", provider: "web3"},
		{name: "web3 address", provider: "dynamic", byAddress: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, user := setupRefreshHandlerTest(t)
			var userID *uint
			address := ""
			if tt.byAddress {
				address = user.Address
			} else {
				userID = &user.ID
			}

			refreshRaw := "transient-" + tt.provider
			sess, err := session.GlobalStore.Create(session.CreateInput{
				UserID:    userID,
				Address:   address,
				TokenHash: session.HashToken("access-" + tt.provider),
				Provider:  tt.provider,
				ExpiresAt: time.Now().Add(time.Hour),
			})
			require.NoError(t, err)
			require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))
			injectTransientQueryError(t, db, "users")

			w, _ := callRefresh(t, refreshRaw)
			require.Equal(t, http.StatusInternalServerError, w.Code)

			var persisted session.UserSession
			require.NoError(t, db.First(&persisted, sess.ID).Error)
			require.False(t, persisted.Revoked)
			require.Empty(t, persisted.RevocationReason)
		})
	}
}

// TestRefreshToken_DeletedUserSessionRevoked locks P1-11 on the refresh path:
// a session belonging to a soft-deleted user must be revoked at the next
// refresh instead of minting fresh tokens (account deletion revokes sessions,
// but this closes the race with sessions created between mark and revoke, and
// any store failure during deletion).
func TestRefreshToken_DeletedUserSessionRevoked(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	sess := seedRefreshSession(t, user.ID, "refresh-1")

	deletedAt := time.Now().UTC()
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).
		Update("deleted_at", deletedAt).Error)

	w, _ := callRefresh(t, "refresh-1")
	require.Equal(t, http.StatusUnauthorized, w.Code)
	// Durable-session branch classifies soft-deleted identity as token-invalid
	// with account_disabled revocation audit (not session-unknown/reuse).
	require.Contains(t, w.Body.String(), ErrCodeTokenInvalid,
		"deleted-user refresh must reject as token-invalid / inactive identity")

	var row session.UserSession
	require.NoError(t, db.First(&row, sess.ID).Error)
	require.True(t, row.Revoked, "the deleted user's session must be revoked")
	require.Equal(t, string(session.RevocationReasonAccountDisabled), row.RevocationReason)
}

// TestRefreshToken_SurvivesStoreRecreation locks deployment-restart semantics
// at the handler level: a refresh cookie minted before the global store is
// rebuilt (same DB) still rotates successfully afterwards.
func TestRefreshToken_SurvivesStoreRecreation(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	seedRefreshSession(t, user.ID, "restart-refresh-1")

	w1, refresh2 := callRefresh(t, "restart-refresh-1")
	require.Equal(t, http.StatusOK, w1.Code)
	require.NotEmpty(t, refresh2)

	// Simulate a deploy: the global store object is rebuilt over the same DB.
	session.GlobalStore = session.NewStore(db)

	w2, refresh3 := callRefresh(t, refresh2)
	require.Equal(t, http.StatusOK, w2.Code, "pre-restart refresh cookie must still rotate")
	require.NotEmpty(t, refresh3)
	require.NotEqual(t, refresh2, refresh3)
}
