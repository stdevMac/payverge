package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupOptoutTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gdb.AutoMigrate(&database.User{}))
	database.SetTestDB(gdb)
	return gdb
}

// P2-10: a valid one-click POST flips email_enabled off for EVERY
// case-colliding account, without auth, idempotently.
func TestUnsubscribeMarketingEmail_OneClickOptsOutAllCaseCollidingUsers(t *testing.T) {
	t.Setenv("UNSUBSCRIBE_TOKEN_SECRET", "optout-test-secret")
	db := setupOptoutTestDB(t)

	for _, email := range []string{"owner@example.com", "OWNER@example.com"} {
		u := database.User{Email: email}
		require.NoError(t, db.Create(&u).Error)
		require.NoError(t, db.Model(&database.User{}).Where("id = ?", u.ID).Update("email_enabled", true).Error)
	}

	token, ok := emails.SignUnsubscribeToken("owner@example.com")
	require.True(t, ok)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/email/unsubscribe", UnsubscribeMarketingEmail)

	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/email/unsubscribe?token=%s", token), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var stillEnabled int64
	require.NoError(t, db.Model(&database.User{}).
		Where("LOWER(email) = ? AND email_enabled = ?", "owner@example.com", true).
		Count(&stillEnabled).Error)
	require.Zero(t, stillEnabled, "every case-colliding account must be opted out")

	// Idempotent replay (mailbox providers may re-POST).
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/email/unsubscribe?token=%s", token), nil))
	require.Equal(t, http.StatusOK, rec2.Code)
}

func TestUnsubscribeMarketingEmail_RejectsBadToken(t *testing.T) {
	t.Setenv("UNSUBSCRIBE_TOKEN_SECRET", "optout-test-secret")
	setupOptoutTestDB(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/email/unsubscribe", UnsubscribeMarketingEmail)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/email/unsubscribe?token=garbage", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// GET is the FE page's status probe: valid token → masked email, no mutation.
func TestGetUnsubscribeStatus_MasksEmailWithoutMutating(t *testing.T) {
	t.Setenv("UNSUBSCRIBE_TOKEN_SECRET", "optout-test-secret")
	db := setupOptoutTestDB(t)

	u := database.User{Email: "owner@example.com"}
	require.NoError(t, db.Create(&u).Error)
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", u.ID).Update("email_enabled", true).Error)

	token, ok := emails.SignUnsubscribeToken("owner@example.com")
	require.True(t, ok)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/email/unsubscribe", GetUnsubscribeStatus)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/email/unsubscribe?token=%s", token), nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "o***@example.com")

	var stillEnabled int64
	require.NoError(t, db.Model(&database.User{}).Where("email_enabled = ?", true).Count(&stillEnabled).Error)
	require.Equal(t, int64(1), stillEnabled, "GET must not mutate")
}
