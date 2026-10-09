package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/auththrottle"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// throttleDBSeq ensures unique in-memory SQLite DSNs across parallel test runs.
var throttleDBSeq atomic.Int64

// newThrottleTestDB opens a fresh in-memory SQLite DB with tables for auth + throttle tests.
func newThrottleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	seq := throttleDBSeq.Add(1)
	dsn := fmt.Sprintf("file:login_throttle_%d?mode=memory&cache=shared", seq)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&database.User{},
		&UserAuth{},
		&database.AuthAttempt{},
	))
	return db
}

// newTestAuthHandler returns an AuthHandler backed by the provided DB.
func newTestAuthHandler(t *testing.T, db *gorm.DB) *AuthHandler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	es, err := emails.NewEmailServer(noopEmailProvider{}, "noreply@example.com", "updates@example.com", "../../email/templates")
	require.NoError(t, err)
	return NewAuthHandler(db, es)
}

// seedVerifiedUser creates a User row and a fully-verified email UserAuth record.
func seedVerifiedUser(t *testing.T, db *gorm.DB, email, password string) {
	t.Helper()
	user := &database.User{Email: email, Name: "Test Owner"}
	require.NoError(t, db.Create(user).Error)

	hash, err := HashPassword(password)
	require.NoError(t, err)

	authRecord := &UserAuth{
		UserID:         user.ID,
		Provider:       "email",
		ProviderUserID: email,
		PasswordHash:   hash,
		EmailVerified:  true,
	}
	require.NoError(t, db.Create(authRecord).Error)
}

func TestLogin_LocksAccountAfterRepeatedFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prevSecret := structs.SecretKey
	t.Cleanup(func() { structs.SecretKey = prevSecret })
	structs.SecretKey = []byte("test-secret-key-for-throttle-tests")

	db := newThrottleTestDB(t)
	h := newTestAuthHandler(t, db)
	h.SetLoginThrottle(auththrottle.New(db, auththrottle.Config{MaxAttempts: 3}))
	seedVerifiedUser(t, db, "owner@example.com", "Correct-Horse-1")

	post := func(pw string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/login",
			strings.NewReader(`{"email":"owner@example.com","password":"`+pw+`"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		h.Login(c)
		return w
	}

	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusUnauthorized, post("wrong").Code, "attempt %d should be 401", i+1)
	}
	assert.Equal(t, http.StatusTooManyRequests, post("Correct-Horse-1").Code,
		"account is locked — correct password must still return 429")
}

// TestLogin_FailsClosedWhenThrottleBackendErrors locks in the fail-closed fix:
// when the lockout backend errors (here: the auth_attempts table is missing), Login must DENY with 429
// rather than fall through to unthrottled password verification. The request
// supplies the CORRECT password, so a fail-open regression would return 200.
func TestLogin_FailsClosedWhenThrottleBackendErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prevSecret := structs.SecretKey
	t.Cleanup(func() { structs.SecretKey = prevSecret })
	structs.SecretKey = []byte("test-secret-key-for-throttle-tests")

	db := newThrottleTestDB(t)
	h := newTestAuthHandler(t, db)
	seedVerifiedUser(t, db, "owner@example.com", "Correct-Horse-1")

	// Throttle backed by a DB WITHOUT the auth_attempts table -> IsLocked errors.
	brokenSeq := throttleDBSeq.Add(1)
	brokenDB, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:broken_throttle_%d?mode=memory&cache=shared", brokenSeq)),
		&gorm.Config{})
	require.NoError(t, err)
	brokenSQL, err := brokenDB.DB()
	require.NoError(t, err)
	brokenSQL.SetMaxOpenConns(1)
	// Intentionally do NOT migrate AuthAttempt so IsLocked's SELECT errors.
	h.SetLoginThrottle(auththrottle.New(brokenDB, auththrottle.Config{MaxAttempts: 3}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(`{"email":"owner@example.com","password":"Correct-Horse-1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)

	require.Equal(t, http.StatusTooManyRequests, w.Code,
		"throttle backend error must fail CLOSED (429) even with the correct password; got %d body: %s",
		w.Code, w.Body.String())
}
