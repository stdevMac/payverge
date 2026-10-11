package crm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

func requireCustomerRefreshOutcomeDelta(t *testing.T, expected metrics.RefreshOutcome, invoke func()) {
	t.Helper()
	before := make(map[metrics.RefreshOutcome]float64)
	for _, outcome := range metrics.RefreshOutcomeValues() {
		before[outcome] = metrics.CurrentRefreshOutcome(metrics.RefreshRealmCustomer, outcome)
	}

	invoke()

	totalDelta := 0.0
	for _, outcome := range metrics.RefreshOutcomeValues() {
		after := metrics.CurrentRefreshOutcome(metrics.RefreshRealmCustomer, outcome)
		delta := after - before[outcome]
		if outcome == expected {
			require.Equal(t, 1.0, delta, "expected one %s customer refresh outcome", outcome)
		} else {
			require.Equal(t, 0.0, delta, "unexpected %s customer refresh outcome", outcome)
		}
		totalDelta += delta
	}
	require.Equal(t, 1.0, totalDelta, "one customer request must record exactly one terminal outcome")
}

func setupCRMHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := database.GetDB()
	previousSecretKey := structs.SecretKey
	t.Cleanup(func() {
		database.SetTestDB(previousDB)
		structs.SecretKey = previousSecretKey
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	database.SetTestDB(db)
	structs.SecretKey = []byte("test-secret-key-for-crm-handler-tests")

	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Customer{},
		&database.CustomerPreferences{},
		&database.CustomerBusiness{},
		&database.CustomerVisit{},
		&database.CustomerAddress{},
		&database.CustomerCommunication{},
		&database.Bill{},
		&database.DeliveryOrder{},
		&session.UserSession{},
	))
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

	return db
}

func createCRMTestBusiness(t *testing.T, db *gorm.DB, businessID, ownerAddress, name string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:     businessID,
		OwnerAddress:   ownerAddress,
		Name:           name,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)
	return business
}

func createCRMTestCustomer(t *testing.T, db *gorm.DB, email string) *database.Customer {
	t.Helper()

	customer := &database.Customer{
		Email:        email,
		PasswordHash: "hash",
		Name:         "Customer",
		IsActive:     true,
	}
	require.NoError(t, db.Create(customer).Error)
	return customer
}

func TestLoginCustomerSetsCustomerRefreshCookieOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	originalStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	defer func() {
		session.GlobalStore = originalStore
	}()

	_, err := handler.service.RegisterCustomer("login@example.com", "password123", "Login Customer")
	require.NoError(t, err)

	body, err := json.Marshal(map[string]string{
		"email":    "login@example.com",
		"password": "password123",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.LoginCustomer(c)

	assert.Equal(t, http.StatusOK, w.Code)

	setCookies := w.Header().Values("Set-Cookie")
	assert.True(t, hasCookiePrefix(setCookies, "customer_token="))
	assert.True(t, hasCookiePrefix(setCookies, "customer_refresh_token="))
	assert.False(t, hasCookiePrefix(setCookies, "refresh_token="))
}

func TestCustomerRefreshRotatesCustomerRefreshCookieOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	originalStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	defer func() {
		session.GlobalStore = originalStore
	}()

	customer := createCRMTestCustomer(t, db, "refresh@example.com")
	refreshRaw := "customer-refresh-token"
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &customer.ID,
		TokenHash: session.HashToken("stale-customer-token"),
		Provider:  "customer",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/refresh", nil)
	c.Request.AddCookie(&http.Cookie{Name: "customer_refresh_token", Value: refreshRaw})

	requireCustomerRefreshOutcomeDelta(t, metrics.RefreshOutcomeSuccess, func() {
		handler.RefreshCustomerToken(c)
	})

	assert.Equal(t, http.StatusOK, w.Code)

	setCookies := w.Header().Values("Set-Cookie")
	assert.True(t, hasCookiePrefix(setCookies, "customer_token="))
	assert.True(t, hasCookiePrefix(setCookies, "customer_refresh_token="))
	assert.False(t, hasCookiePrefix(setCookies, "session_token="))
	assert.False(t, hasCookiePrefix(setCookies, "staff_token="))
	assert.False(t, hasCookiePrefix(setCookies, "refresh_token="))

	_, err = session.GlobalStore.ValidateRefreshToken(session.HashToken(refreshRaw))
	assert.Error(t, err)
}

func TestCustomerRefreshIgnoresGenericRefreshCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	originalStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	defer func() {
		session.GlobalStore = originalStore
	}()

	customer := createCRMTestCustomer(t, db, "generic-refresh@example.com")
	refreshRaw := "generic-refresh-token"
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &customer.ID,
		TokenHash: session.HashToken("stale-customer-token"),
		Provider:  "customer",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/refresh", nil)
	c.Request.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshRaw})

	requireCustomerRefreshOutcomeDelta(t, metrics.RefreshOutcomeInvalid, func() {
		handler.RefreshCustomerToken(c)
	})

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	setCookies := w.Header().Values("Set-Cookie")
	assert.False(t, hasCookiePrefix(setCookies, "customer_token="))
	assert.False(t, hasCookiePrefix(setCookies, "customer_refresh_token="))
	assert.False(t, hasCookiePrefix(setCookies, "session_token="))
	assert.False(t, hasCookiePrefix(setCookies, "staff_token="))
	assert.False(t, hasCookiePrefix(setCookies, "refresh_token="))
}

// TestCustomerRefreshStaleReuseRevokesSession locks AUTH-1c: replaying a customer
// refresh token rotated away well before (outside the grace window) revokes the
// session, so a stolen customer refresh token cannot be used. An immediate replay
// (benign concurrent race) is instead rejected without revoking.
func TestCustomerRefreshStaleReuseRevokesSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	originalStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	defer func() { session.GlobalStore = originalStore }()

	customer := createCRMTestCustomer(t, db, "reuse@example.com")
	refresh1 := "customer-refresh-1"
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &customer.ID,
		TokenHash: session.HashToken("stale-customer-token"),
		Provider:  "customer",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refresh1), time.Now().Add(7*24*time.Hour)))

	// Legit rotation: refresh1 -> refresh2.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/refresh", nil)
	c.Request.AddCookie(&http.Cookie{Name: "customer_refresh_token", Value: refresh1})
	handler.RefreshCustomerToken(c)
	require.Equal(t, http.StatusOK, w.Code)

	// First an immediate replay: benign concurrent race — rejected, NOT revoked.
	wRace := httptest.NewRecorder()
	cRace, _ := gin.CreateTestContext(wRace)
	cRace.Request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/refresh", nil)
	cRace.Request.AddCookie(&http.Cookie{Name: "customer_refresh_token", Value: refresh1})
	requireCustomerRefreshOutcomeDelta(t, metrics.RefreshOutcomeRotated, func() {
		handler.RefreshCustomerToken(cRace)
	})
	require.Equal(t, http.StatusUnauthorized, wRace.Code)
	require.Contains(t, wRace.Body.String(), server.ErrCodeRefreshRotated)
	var raceRow session.UserSession
	require.NoError(t, db.First(&raceRow, sess.ID).Error)
	require.False(t, raceRow.Revoked, "an immediate replay (race) must not revoke")

	// Age the rotation past the grace window, then replay -> reuse detected.
	stale := time.Now().Add(-session.RefreshReuseGracePeriod - time.Minute)
	require.NoError(t, db.Table("user_session_refresh_history").
		Where("session_id = ? AND token_hash = ?", sess.ID, session.HashToken(refresh1)).
		Update("rotated_at", stale).Error)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/refresh", nil)
	c2.Request.AddCookie(&http.Cookie{Name: "customer_refresh_token", Value: refresh1})
	requireCustomerRefreshOutcomeDelta(t, metrics.RefreshOutcomeReuse, func() {
		handler.RefreshCustomerToken(c2)
	})
	require.Equal(t, http.StatusUnauthorized, w2.Code, "stale replayed token is rejected")

	var row session.UserSession
	require.NoError(t, db.First(&row, sess.ID).Error)
	require.True(t, row.Revoked, "stale reuse detection revokes the customer session")
	require.Equal(t, string(session.RevocationReasonRefreshTokenReuse), row.RevocationReason)
}

func TestCustomerRefreshOlderRecentGenerationReturnsRotatedWithoutRevoking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))
	previousStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = previousStore })

	customer := createCRMTestCustomer(t, db, "older-recent@example.com")
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID: &customer.ID, TokenHash: session.HashToken("customer-access-R1"),
		Provider: "customer", ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken("customer-R1"), time.Now().Add(7*24*time.Hour)))

	refresh := func(raw string) (*httptest.ResponseRecorder, string) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/refresh", nil)
		c.Request.AddCookie(&http.Cookie{Name: "customer_refresh_token", Value: raw})
		handler.RefreshCustomerToken(c)
		next := ""
		for _, cookie := range w.Result().Cookies() {
			if cookie.Name == "customer_refresh_token" {
				next = cookie.Value
			}
		}
		return w, next
	}

	w1, r2 := refresh("customer-R1")
	require.Equal(t, http.StatusOK, w1.Code)
	w2, r3 := refresh(r2)
	require.Equal(t, http.StatusOK, w2.Code)
	require.NotEmpty(t, r3)
	replay, minted := refresh("customer-R1")
	require.Equal(t, http.StatusUnauthorized, replay.Code)
	require.Contains(t, replay.Body.String(), server.ErrCodeRefreshRotated)
	require.Empty(t, minted)

	var row session.UserSession
	require.NoError(t, db.First(&row, sess.ID).Error)
	require.False(t, row.Revoked)
}

func TestCustomerRefreshRevokeFailureReturns500WithoutClaimingRevoked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))
	previousStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = previousStore })
	customer := createCRMTestCustomer(t, db, "revoke-failure@example.com")
	raw := "customer-revoke-failure"
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID: &customer.ID, TokenHash: session.HashToken("access-revoke-failure"),
		Provider: "customer", ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(raw), time.Now().Add(7*24*time.Hour)))
	rotate := httptest.NewRecorder()
	rotateCtx, _ := gin.CreateTestContext(rotate)
	rotateCtx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/refresh", nil)
	rotateCtx.Request.AddCookie(&http.Cookie{Name: "customer_refresh_token", Value: raw})
	handler.RefreshCustomerToken(rotateCtx)
	require.Equal(t, http.StatusOK, rotate.Code)
	require.NoError(t, db.Table("user_session_refresh_history").Where("session_id = ?", sess.ID).
		Updates(map[string]interface{}{
			"rotated_at": time.Now().Add(-session.RefreshReuseGracePeriod - time.Second),
			"expires_at": time.Now().Add(time.Hour),
		}).Error)
	callbackName := "payverge:test:customer-revoke-failure"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "user_sessions" {
			_ = tx.AddError(fmt.Errorf("transient customer revocation write failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/refresh", nil)
	c.Request.AddCookie(&http.Cookie{Name: "customer_refresh_token", Value: raw})
	handler.RefreshCustomerToken(c)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NotContains(t, w.Body.String(), server.ErrCodeSessionUnknown)
	var row session.UserSession
	require.NoError(t, db.First(&row, sess.ID).Error)
	require.False(t, row.Revoked)
	require.Empty(t, row.RevocationReason)
}

func TestCustomerRefreshSessionStoreOperationalErrorsReturn500WithoutRevocation(t *testing.T) {
	for _, failureQuery := range []int{1, 2} {
		name := "validation"
		if failureQuery == 2 {
			name = "history lookup"
		}
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			db := setupCRMHandlerTestDB(t)
			handler := NewHandler(NewService(db))
			previousStore := session.GlobalStore
			session.GlobalStore = session.NewStore(db)
			t.Cleanup(func() { session.GlobalStore = previousStore })

			customer := createCRMTestCustomer(t, db, "store-error-"+name+"@example.com")
			raw := "customer-store-error-" + name
			sess, err := session.GlobalStore.Create(session.CreateInput{
				UserID: &customer.ID, TokenHash: session.HashToken("access-" + name),
				Provider: "customer", ExpiresAt: time.Now().Add(time.Hour),
			})
			require.NoError(t, err)
			require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(raw), time.Now().Add(7*24*time.Hour)))
			if failureQuery == 2 {
				require.NoError(t, db.Model(&session.UserSession{}).Where("id = ?", sess.ID).
					Update("refresh_token", session.HashToken("different-customer-token")).Error)
			}

			callbackName := "payverge:test:crm-session-store:" + t.Name()
			queryCount := 0
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement != nil && tx.Statement.Schema != nil &&
					(tx.Statement.Schema.Table == "user_sessions" || tx.Statement.Schema.Table == "user_session_refresh_history") {
					queryCount++
					if queryCount == failureQuery {
						_ = tx.AddError(fmt.Errorf("transient customer session store failure"))
					}
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/refresh", nil)
			c.Request.AddCookie(&http.Cookie{Name: "customer_refresh_token", Value: raw})
			requireCustomerRefreshOutcomeDelta(t, metrics.RefreshOutcomeStoreError, func() {
				handler.RefreshCustomerToken(c)
			})
			require.Equal(t, http.StatusInternalServerError, w.Code)

			var row session.UserSession
			require.NoError(t, db.First(&row, sess.ID).Error)
			require.False(t, row.Revoked)
			require.Empty(t, row.RevocationReason)
		})
	}
}

func TestCustomerRefreshInvalidAndInactiveSessionsRecordReasons(t *testing.T) {
	tests := []struct {
		name       string
		withUserID bool
		inactive   bool
		reason     session.RevocationReason
	}{
		{name: "missing user id", reason: session.RevocationReasonInvalidSession},
		{name: "inactive customer", withUserID: true, inactive: true, reason: session.RevocationReasonAccountDisabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			db := setupCRMHandlerTestDB(t)
			handler := NewHandler(NewService(db))

			previousStore := session.GlobalStore
			session.GlobalStore = session.NewStore(db)
			t.Cleanup(func() { session.GlobalStore = previousStore })

			var userID *uint
			if tt.withUserID {
				customer := createCRMTestCustomer(t, db, "refresh-reason@example.com")
				if tt.inactive {
					require.NoError(t, db.Model(&database.Customer{}).Where("id = ?", customer.ID).Update("is_active", false).Error)
				}
				userID = &customer.ID
			}

			refreshRaw := "customer-refresh-reason"
			sess, err := session.GlobalStore.Create(session.CreateInput{
				UserID:    userID,
				TokenHash: session.HashToken("customer-access-reason"),
				Provider:  "customer",
				ExpiresAt: time.Now().Add(time.Hour),
			})
			require.NoError(t, err)
			require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/refresh", nil)
			c.Request.AddCookie(&http.Cookie{Name: "customer_refresh_token", Value: refreshRaw})
			handler.RefreshCustomerToken(c)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			var persisted session.UserSession
			require.NoError(t, db.First(&persisted, sess.ID).Error)
			assert.True(t, persisted.Revoked)
			assert.Equal(t, string(tt.reason), persisted.RevocationReason)
		})
	}
}

func TestCustomerRefreshTransientLookupDoesNotRevoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	previousStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = previousStore })

	customer := createCRMTestCustomer(t, db, "transient-refresh@example.com")
	refreshRaw := "customer-transient-refresh"
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &customer.ID,
		TokenHash: session.HashToken("customer-transient-access"),
		Provider:  "customer",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))

	callbackName := "payverge:test:transient_customer_lookup"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "customers" {
			_ = tx.AddError(fmt.Errorf("transient customer lookup failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/refresh", nil)
	c.Request.AddCookie(&http.Cookie{Name: "customer_refresh_token", Value: refreshRaw})
	handler.RefreshCustomerToken(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.False(t, persisted.Revoked)
	assert.Empty(t, persisted.RevocationReason)
}

func TestGetCustomerSessionInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	originalStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	defer func() {
		session.GlobalStore = originalStore
	}()

	router := gin.New()
	router.GET("/api/v1/customer/session-info", handler.GetCustomerSessionInfo)

	t.Run("no cookie returns unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/customer/session-info", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Authenticated bool `json:"authenticated"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Authenticated)
	})

	t.Run("valid customer cookie returns customer identity", func(t *testing.T) {
		customer := createCRMTestCustomer(t, db, "session-info@example.com")
		customer.Name = "Session Customer"
		require.NoError(t, db.Save(customer).Error)

		sess, err := session.GlobalStore.Create(session.CreateInput{
			UserID:    &customer.ID,
			TokenHash: "placeholder",
			Provider:  "customer",
			ExpiresAt: time.Now().Add(time.Hour),
		})
		require.NoError(t, err)

		token, err := server.GenerateCustomerToken(customer.ID, customer.Email, sess.ID)
		require.NoError(t, err)
		require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/customer/session-info", nil)
		req.AddCookie(&http.Cookie{Name: "customer_token", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Authenticated bool   `json:"authenticated"`
			Type          string `json:"type"`
			CustomerID    uint   `json:"customer_id"`
			Email         string `json:"email"`
			Name          string `json:"name"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.Authenticated)
		assert.Equal(t, "customer", resp.Type)
		assert.Equal(t, customer.ID, resp.CustomerID)
		assert.Equal(t, customer.Email, resp.Email)
		assert.Equal(t, "Session Customer", resp.Name)
	})

	t.Run("revoked session returns unauthenticated", func(t *testing.T) {
		customer := createCRMTestCustomer(t, db, "revoked-session-info@example.com")
		sess, err := session.GlobalStore.Create(session.CreateInput{
			UserID:    &customer.ID,
			TokenHash: "placeholder",
			Provider:  "customer",
			ExpiresAt: time.Now().Add(time.Hour),
		})
		require.NoError(t, err)

		token, err := server.GenerateCustomerToken(customer.ID, customer.Email, sess.ID)
		require.NoError(t, err)
		require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))
		require.NoError(t, session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonUnspecified))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/customer/session-info", nil)
		req.AddCookie(&http.Cookie{Name: "customer_token", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Authenticated bool `json:"authenticated"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Authenticated)
	})

	t.Run("inactive customer returns unauthenticated", func(t *testing.T) {
		customer := createCRMTestCustomer(t, db, "inactive-session-info@example.com")
		require.NoError(t, db.Model(&database.Customer{}).Where("id = ?", customer.ID).Update("is_active", false).Error)

		sess, err := session.GlobalStore.Create(session.CreateInput{
			UserID:    &customer.ID,
			TokenHash: "placeholder",
			Provider:  "customer",
			ExpiresAt: time.Now().Add(time.Hour),
		})
		require.NoError(t, err)

		token, err := server.GenerateCustomerToken(customer.ID, customer.Email, sess.ID)
		require.NoError(t, err)
		require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/customer/session-info", nil)
		req.AddCookie(&http.Cookie{Name: "customer_token", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Authenticated bool `json:"authenticated"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Authenticated)
	})
}

func TestLogoutCustomerClearsAllAuthCookies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	originalStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	defer func() {
		session.GlobalStore = originalStore
	}()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	handler.LogoutCustomer(c)

	assert.Equal(t, http.StatusOK, w.Code)

	setCookies := w.Header().Values("Set-Cookie")
	assert.True(t, hasCookiePrefix(setCookies, "session_token=;"))
	assert.True(t, hasCookiePrefix(setCookies, "staff_token=;"))
	assert.True(t, hasCookiePrefix(setCookies, "customer_token=;"))
	assert.True(t, hasCookiePrefix(setCookies, "refresh_token=;"))
	assert.True(t, hasCookiePrefix(setCookies, "customer_refresh_token=;"))
}

func TestLogoutCustomerRevokesCustomerCookieNotAuthorizationBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	originalStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	defer func() {
		session.GlobalStore = originalStore
	}()

	ownerID := uint(77)
	ownerToken := "generic-owner-token"
	ownerSession, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &ownerID,
		TokenHash: session.HashToken(ownerToken),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	customer := createCRMTestCustomer(t, db, "logout-cookie@example.com")
	customerSession, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &customer.ID,
		TokenHash: "placeholder",
		Provider:  "customer",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	customerToken, err := server.GenerateCustomerToken(customer.ID, customer.Email, customerSession.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(customerSession.ID, session.HashToken(customerToken)))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer "+ownerToken)
	c.Request.AddCookie(&http.Cookie{Name: "customer_token", Value: customerToken})

	handler.LogoutCustomer(c)

	assert.Equal(t, http.StatusOK, w.Code)

	ownerValid, err := session.GlobalStore.Validate(ownerSession.ID, session.HashToken(ownerToken))
	require.NoError(t, err)
	assert.True(t, ownerValid, "customer logout must not revoke Authorization bearer sessions")

	customerValid, err := session.GlobalStore.Validate(customerSession.ID, session.HashToken(customerToken))
	require.NoError(t, err)
	assert.False(t, customerValid, "customer logout should revoke the customer_token cookie session")
	var persistedCustomerSession session.UserSession
	require.NoError(t, db.First(&persistedCustomerSession, customerSession.ID).Error)
	assert.Equal(t, string(session.RevocationReasonUserLogout), persistedCustomerSession.RevocationReason)
}

func TestLogoutCustomerDoesNotRevokeNonCustomerTokenInCustomerCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	originalStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	defer func() {
		session.GlobalStore = originalStore
	}()

	ownerID := uint(88)
	ownerSession, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &ownerID,
		TokenHash: "placeholder",
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	ownerToken, err := server.GenerateUserToken(ownerID, "owner-cookie@example.com", "0xowner", "owner", ownerSession.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(ownerSession.ID, session.HashToken(ownerToken)))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.AddCookie(&http.Cookie{Name: "customer_token", Value: ownerToken})

	handler.LogoutCustomer(c)

	assert.Equal(t, http.StatusOK, w.Code)

	ownerValid, err := session.GlobalStore.Validate(ownerSession.ID, session.HashToken(ownerToken))
	require.NoError(t, err)
	assert.True(t, ownerValid, "customer logout must not revoke non-customer sessions from customer_token cookie")
}

func TestUpdateCustomerNotes_RejectsCrossBusinessAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	businessA := createCRMTestBusiness(t, db, "biz-a", "0xOwnerA", "Biz A")
	businessB := createCRMTestBusiness(t, db, "biz-b", "0xOwnerB", "Biz B")
	customer := createCRMTestCustomer(t, db, "customer@example.com")

	customerBusiness := &database.CustomerBusiness{
		CustomerID:     customer.ID,
		BusinessID:     businessB.ID,
		FirstVisitAt:   time.Now(),
		OptInEmail:     true,
		OptInMarketing: true,
		IsActive:       true,
		LoyaltyPoints:  5,
		TotalSpent:     20,
		VisitCount:     1,
	}
	require.NoError(t, db.Create(customerBusiness).Error)

	body, err := json.Marshal(map[string]string{"notes": "hijack"})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", businessA.ID)},
		{Key: "customerBusinessId", Value: fmt.Sprintf("%d", customerBusiness.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateCustomerNotes(c)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var persisted database.CustomerBusiness
	require.NoError(t, db.First(&persisted, customerBusiness.ID).Error)
	assert.Empty(t, persisted.Notes)
}

func TestUpdateCustomerTags_RejectsCrossBusinessAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	businessA := createCRMTestBusiness(t, db, "biz-a-tags", "0xOwnerA", "Biz A")
	businessB := createCRMTestBusiness(t, db, "biz-b-tags", "0xOwnerB", "Biz B")
	customer := createCRMTestCustomer(t, db, "customer-tags@example.com")

	customerBusiness := &database.CustomerBusiness{
		CustomerID:     customer.ID,
		BusinessID:     businessB.ID,
		FirstVisitAt:   time.Now(),
		OptInEmail:     true,
		OptInMarketing: true,
		IsActive:       true,
	}
	require.NoError(t, db.Create(customerBusiness).Error)

	body, err := json.Marshal(map[string]string{"tags": "vip"})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", businessA.ID)},
		{Key: "customerBusinessId", Value: fmt.Sprintf("%d", customerBusiness.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateCustomerTags(c)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var persisted database.CustomerBusiness
	require.NoError(t, db.First(&persisted, customerBusiness.ID).Error)
	assert.Empty(t, persisted.Tags)
}

func TestGetBusinessCustomers_NormalizesInvalidPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	business := createCRMTestBusiness(t, db, "biz-pagination", "0xOwner", "Biz")
	customer := createCRMTestCustomer(t, db, "pagination@example.com")

	customerBusiness := &database.CustomerBusiness{
		CustomerID:     customer.ID,
		BusinessID:     business.ID,
		FirstVisitAt:   time.Now(),
		OptInEmail:     true,
		OptInMarketing: true,
		IsActive:       true,
	}
	require.NoError(t, db.Create(customerBusiness).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/?page=0&page_size=0", nil)

	handler.GetBusinessCustomers(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Page       int `json:"page"`
		PageSize   int `json:"page_size"`
		TotalPages int `json:"total_pages"`
		Total      int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 1, response.Page)
	assert.Equal(t, 20, response.PageSize)
	assert.Equal(t, 1, response.TotalPages)
	assert.Equal(t, 1, response.Total)
}

func TestGetBusinessCustomers_AcceptsBusinessSlugIdentifier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	business := createCRMTestBusiness(t, db, "biz-slug-crm-customers", "0xOwner", "Biz")
	customer := createCRMTestCustomer(t, db, "slug-crm@example.com")

	customerBusiness := &database.CustomerBusiness{
		CustomerID:     customer.ID,
		BusinessID:     business.ID,
		FirstVisitAt:   time.Now(),
		OptInEmail:     true,
		OptInMarketing: true,
		IsActive:       true,
	}
	require.NoError(t, db.Create(customerBusiness).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler.GetBusinessCustomers(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 1, response.Total)
}

func TestExportCustomers_IncludesNotesAndTags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	business := createCRMTestBusiness(t, db, "biz-export-crm", "0xOwner", "Biz")
	customer := createCRMTestCustomer(t, db, "export-crm@example.com")

	customerBusiness := &database.CustomerBusiness{
		CustomerID:     customer.ID,
		BusinessID:     business.ID,
		FirstVisitAt:   time.Now(),
		OptInEmail:     true,
		OptInMarketing: true,
		IsActive:       true,
		Notes:          "prefers patio",
		Tags:           `["vip","birthday"]`,
	}
	require.NoError(t, db.Create(customerBusiness).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler.ExportCustomers(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Notes,Tags")
	assert.Contains(t, w.Body.String(), "prefers patio")
	assert.Contains(t, w.Body.String(), `[""vip"",""birthday""]`)
}

func TestConnectCustomerToBusiness_RejectsUnknownBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	customer := createCRMTestCustomer(t, db, "missing-biz@example.com")

	body, err := json.Marshal(map[string]interface{}{
		"business_id":      9999,
		"opt_in_marketing": true,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("customer_id", float64(customer.ID))

	handler.ConnectCustomerToBusiness(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateCustomerProfile_AcceptsDateOnlyBirthdayAndClearsBlank(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	customer := createCRMTestCustomer(t, db, "profile@example.com")

	body, err := json.Marshal(map[string]string{
		"name":     "Updated Customer",
		"birthday": "2026-04-01",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("customer_id", float64(customer.ID))

	handler.UpdateCustomerProfile(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var persisted database.Customer
	require.NoError(t, db.First(&persisted, customer.ID).Error)
	require.NotNil(t, persisted.Birthday)
	assert.Equal(t, "2026-04-01", persisted.Birthday.Format("2006-01-02"))
	assert.Equal(t, "Updated Customer", persisted.Name)

	clearBody, err := json.Marshal(map[string]string{
		"birthday": "",
	})
	require.NoError(t, err)

	clearWriter := httptest.NewRecorder()
	clearContext, _ := gin.CreateTestContext(clearWriter)
	clearContext.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(clearBody))
	clearContext.Request.Header.Set("Content-Type", "application/json")
	clearContext.Set("customer_id", float64(customer.ID))

	handler.UpdateCustomerProfile(clearContext)

	assert.Equal(t, http.StatusOK, clearWriter.Code)
	persisted = database.Customer{}
	require.NoError(t, db.First(&persisted, customer.ID).Error)
	assert.Nil(t, persisted.Birthday)
}

func TestDeleteCustomerAccount_RevokesOnlyCustomerSessionsAndClearsCookies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	originalStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	defer func() {
		session.GlobalStore = originalStore
	}()

	customer := createCRMTestCustomer(t, db, "delete@example.com")
	customerID := customer.ID

	customerSession, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &customerID,
		TokenHash: session.HashToken("customer-token"),
		Provider:  "customer",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	emailSession, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &customerID,
		TokenHash: session.HashToken("user-token"),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set("customer_id", float64(customer.ID))

	handler.DeleteCustomerAccount(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var persistedCustomer database.Customer
	require.NoError(t, db.First(&persistedCustomer, customer.ID).Error)
	assert.False(t, persistedCustomer.IsActive)

	var revokedCustomerSession session.UserSession
	require.NoError(t, db.First(&revokedCustomerSession, customerSession.ID).Error)
	assert.True(t, revokedCustomerSession.Revoked)
	assert.Equal(t, string(session.RevocationReasonAccountDisabled), revokedCustomerSession.RevocationReason)

	var persistedEmailSession session.UserSession
	require.NoError(t, db.First(&persistedEmailSession, emailSession.ID).Error)
	assert.False(t, persistedEmailSession.Revoked)

	setCookies := w.Header().Values("Set-Cookie")
	assert.True(t, hasCookiePrefix(setCookies, "session_token=;"))
	assert.True(t, hasCookiePrefix(setCookies, "staff_token=;"))
	assert.True(t, hasCookiePrefix(setCookies, "customer_token=;"))
	assert.True(t, hasCookiePrefix(setCookies, "refresh_token=;"))
	assert.True(t, hasCookiePrefix(setCookies, "customer_refresh_token=;"))
}

func hasCookiePrefix(headers []string, prefix string) bool {
	for _, header := range headers {
		if strings.HasPrefix(header, prefix) {
			return true
		}
	}
	return false
}
