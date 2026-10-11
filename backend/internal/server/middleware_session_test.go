package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

func setupSessionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&session.UserSession{}, &database.Customer{}, &database.User{}, &operatorEmailAuthState{}))
	database.SetTestDB(db)
	return db
}

func TestValidateSession_RejectsTokenWithoutSessionID(t *testing.T) {
	db := setupSessionTestDB(t)
	session.GlobalStore = session.NewStore(db)
	structs.SecretKey = []byte("test-secret-key-for-session-tests")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/test", nil)

	result := validateSession(c, map[string]interface{}{
		"user_id": float64(1),
		"email":   "test@example.com",
	}, "any-token-string")

	assert.False(t, result, "Legacy token without session_id should be rejected")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestValidateSession_AcceptsTokenWithValidSessionID(t *testing.T) {
	db := setupSessionTestDB(t)
	session.GlobalStore = session.NewStore(db)
	structs.SecretKey = []byte("test-secret-key-for-session-tests")

	tokenString := "test-token-string"
	user := database.User{Email: "verified-session@example.com", Role: "user", AuthMethod: "email", EmailVerified: true}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&operatorEmailAuthState{
		UserID: user.ID, Provider: "email", ProviderUserID: user.Email, EmailVerified: true,
	}).Error)
	userID := user.ID

	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &userID,
		TokenHash: session.HashToken(tokenString),
		Provider:  "email",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/test", nil)

	result := validateSession(c, map[string]interface{}{
		"user_id":    float64(1),
		"session_id": float64(sess.ID),
	}, tokenString)

	assert.True(t, result, "Token with valid session_id should be accepted")
}

func TestCustomerAuthenticationMiddleware_RejectsInactiveCustomer(t *testing.T) {
	db := setupSessionTestDB(t)
	session.GlobalStore = session.NewStore(db)
	structs.SecretKey = []byte("test-secret-key-for-session-tests")

	customer := &database.Customer{
		Email:    "inactive@example.com",
		Name:     "Inactive Customer",
		IsActive: true,
	}
	require.NoError(t, db.Create(customer).Error)
	require.NoError(t, db.Model(&database.Customer{}).Where("id = ?", customer.ID).Update("is_active", false).Error)

	tokenString := "inactive-customer-token"
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &customer.ID,
		TokenHash: session.HashToken(tokenString),
		Provider:  "customer",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	token, err := GenerateCustomerToken(customer.ID, customer.Email, sess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CustomerAuthenticationMiddleware())
	router.GET("/customer/profile", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/customer/profile", nil)
	req.AddCookie(&http.Cookie{Name: "customer_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRefreshToken_RejectsInactiveCustomer(t *testing.T) {
	db := setupSessionTestDB(t)
	session.GlobalStore = session.NewStore(db)
	structs.SecretKey = []byte("test-secret-key-for-session-tests")

	customer := &database.Customer{
		Email:    "refresh-inactive@example.com",
		Name:     "Refresh Inactive",
		IsActive: true,
	}
	require.NoError(t, db.Create(customer).Error)
	require.NoError(t, db.Model(&database.Customer{}).Where("id = ?", customer.ID).Update("is_active", false).Error)

	refreshRaw := "refresh-token-raw"
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &customer.ID,
		TokenHash: session.HashToken("stale-access-token"),
		Provider:  "customer",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/auth/refresh", nil)
	c.Request.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshRaw})

	RefreshToken(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, string(session.RevocationReasonAccountDisabled), persisted.RevocationReason)
}

func TestRefreshToken_RejectsSessionsWithoutUserID(t *testing.T) {
	providers := []string{"email", "google", "register", "staff_code", "customer"}
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			db := setupSessionTestDB(t)
			session.GlobalStore = session.NewStore(db)
			structs.SecretKey = []byte("test-secret-key-for-session-tests")

			refreshRaw := fmt.Sprintf("refresh-%s-no-user", provider)
			sess, err := session.GlobalStore.Create(session.CreateInput{
				TokenHash: session.HashToken("stale-access-token"),
				Provider:  provider,
				ExpiresAt: time.Now().Add(time.Hour),
			})
			require.NoError(t, err)
			require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))

			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("POST", "/auth/refresh", nil)
			c.Request.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshRaw})

			RefreshToken(c)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Contains(t, w.Body.String(), ErrCodeTokenInvalid)
			var persisted session.UserSession
			require.NoError(t, db.First(&persisted, sess.ID).Error)
			assert.True(t, persisted.Revoked)
			assert.Equal(t, string(session.RevocationReasonInvalidSession), persisted.RevocationReason)
		})
	}
}

func TestRefreshToken_RejectsInactiveStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Refresh Staff Biz")
	staff := createStaffMember(t, business.ID, "refresh-staff@example.com", "Refresh Staff")
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("is_active", false).Error)

	refreshRaw := "refresh-staff-token"
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &staff.ID,
		TokenHash: session.HashToken("stale-staff-token"),
		Provider:  "staff_code",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/auth/refresh", nil)
	c.Request.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshRaw})

	RefreshToken(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var persisted session.UserSession
	require.NoError(t, database.GetDB().First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, string(session.RevocationReasonAccountDisabled), persisted.RevocationReason)
}

func TestRefreshToken_RevokesUnknownProviderAsInvalidSession(t *testing.T) {
	db := setupSessionTestDB(t)
	session.GlobalStore = session.NewStore(db)

	refreshRaw := "refresh-unknown-provider"
	sess, err := session.GlobalStore.Create(session.CreateInput{
		TokenHash: session.HashToken("unknown-provider-access"),
		Provider:  "mystery",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodPost, "/auth/refresh", nil)
	c.Request.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshRaw})

	RefreshToken(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, string(session.RevocationReasonInvalidSession), persisted.RevocationReason)
}

func TestRefreshToken_TransientStaffAndCustomerLookupDoesNotRevoke(t *testing.T) {
	tests := []struct {
		provider string
		table    string
		model    interface{}
	}{
		{provider: "staff_code", table: "staff", model: &database.Staff{}},
		{provider: "customer", table: "customers", model: &database.Customer{}},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			db := setupSessionTestDB(t)
			require.NoError(t, db.AutoMigrate(tt.model))
			session.GlobalStore = session.NewStore(db)
			structs.SecretKey = []byte("test-secret-key-for-session-tests")

			userID := uint(42)
			refreshRaw := "transient-" + tt.provider
			sess, err := session.GlobalStore.Create(session.CreateInput{
				UserID:    &userID,
				TokenHash: session.HashToken("access-" + tt.provider),
				Provider:  tt.provider,
				ExpiresAt: time.Now().Add(time.Hour),
			})
			require.NoError(t, err)
			require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))
			injectTransientQueryError(t, db, tt.table)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest(http.MethodPost, "/auth/refresh", nil)
			c.Request.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshRaw})
			RefreshToken(c)

			assert.Equal(t, http.StatusInternalServerError, w.Code)
			var persisted session.UserSession
			require.NoError(t, db.First(&persisted, sess.ID).Error)
			assert.False(t, persisted.Revoked)
			assert.Empty(t, persisted.RevocationReason)
		})
	}
}
