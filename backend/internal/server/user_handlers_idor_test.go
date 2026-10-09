package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupUserHandlerDB spins up an in-memory sqlite DB with the User model so the
// user_handlers IDOR tests can exercise GetUser against real database helpers.
func setupUserHandlerDB(t testing.TB) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.User{}))
	return gormDB
}

func seedUser(t testing.TB, gormDB *gorm.DB, address, email, role string) *database.User {
	t.Helper()
	u := &database.User{
		Address:          strings.ToLower(address),
		Email:            email,
		Role:             role,
		LanguageSelected: "en",
	}
	require.NoError(t, gormDB.Create(u).Error)
	return u
}

// TestGetUser_NonAdminCannotReadOtherUser proves a non-admin caller cannot read
// another user's record (403), can read their own (200), and an admin can read
// any user (200).
func TestGetUser_NonAdminCannotReadOtherUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupUserHandlerDB(t)

	addrA := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	addrB := "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	addrAdmin := "0xcccccccccccccccccccccccccccccccccccccccc"
	seedUser(t, gormDB, addrA, "a@example.com", "user")
	seedUser(t, gormDB, addrB, "b@example.com", "user")
	seedUser(t, gormDB, addrAdmin, "admin@example.com", "admin")

	newRouter := func(ctxAddress, ctxRole string) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("address", ctxAddress)
			if ctxRole != "" {
				c.Set("role", ctxRole)
			}
			c.Next()
		})
		r.GET("/get_user/:address", GetUser)
		return r
	}

	doGet := func(r *gin.Engine, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/get_user/"+target, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// Non-admin A requesting B -> 403.
	wForbidden := doGet(newRouter(addrA, ""), addrB)
	assert.Equal(t, http.StatusForbidden, wForbidden.Code,
		"non-admin caller must not read another user; body=%s", wForbidden.Body.String())

	// Non-admin A requesting self -> 200 (case-insensitive on the path param).
	wSelf := doGet(newRouter(addrA, ""), strings.ToUpper(addrA))
	require.Equal(t, http.StatusOK, wSelf.Code, "self read must succeed; body=%s", wSelf.Body.String())
	var selfResp map[string]any
	require.NoError(t, json.Unmarshal(wSelf.Body.Bytes(), &selfResp))
	assert.Equal(t, strings.ToLower(addrA), strings.ToLower(fmt.Sprint(selfResp["address"])))

	// Admin requesting B -> 200.
	wAdmin := doGet(newRouter(addrAdmin, "admin"), addrB)
	require.Equal(t, http.StatusOK, wAdmin.Code, "admin read must succeed; body=%s", wAdmin.Body.String())
}
