package crm

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

func assertNoDBLeak(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	assert.NotContains(t, lower, "database")
	assert.NotContains(t, lower, "sql")
	assert.NotContains(t, lower, "no such table")
}

func TestRegisterCustomer_DBErrorDoesNotLeak(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))
	require.NoError(t, db.Migrator().DropTable(&database.Customer{}))

	body := bytes.NewBufferString(`{"email":"ada@example.com","password":"password123","name":"Ada"}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.RegisterCustomer(c)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Could not register account")
	assert.Contains(t, w.Body.String(), server.ErrCodeInternal)
	assertNoDBLeak(t, w.Body.String())
}

func TestLoginCustomer_DBErrorDoesNotLeak(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))
	require.NoError(t, db.Migrator().DropTable(&database.Customer{}))

	body := bytes.NewBufferString(`{"email":"ada@example.com","password":"password123"}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.LoginCustomer(c)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Could not sign in")
	assert.Contains(t, w.Body.String(), server.ErrCodeInternal)
	assertNoDBLeak(t, w.Body.String())
}

func TestLoginCustomer_WrongPasswordIsGenericUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))
	_, err := handler.service.RegisterCustomer("ada@example.com", "password123", "Ada")
	require.NoError(t, err)

	body := bytes.NewBufferString(`{"email":"ada@example.com","password":"wrong-password"}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.LoginCustomer(c)

	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Invalid email or password")
	assert.Contains(t, w.Body.String(), server.ErrCodeTokenInvalid)
	assertNoDBLeak(t, w.Body.String())
}
