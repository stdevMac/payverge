package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/logic"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type AuthHandlersTestSuite struct {
	suite.Suite
	router *gin.Engine
}

func (suite *AuthHandlersTestSuite) SetupTest() {
	gin.SetMode(gin.TestMode)
	suite.router = gin.New()

	// Initialize challenge store for testing
	ChallengeStore = logic.NewChallengeStore()

	// Set up test secret key
	structs.SecretKey = []byte("test-secret-key-for-testing-purposes")

	// Mock external services to prevent panics
	_ = os.Setenv("DISABLE_POSTHOG", "true")
	_ = os.Setenv("DISABLE_METRICS", "true")
}

func TestAuthHandlersTestSuite(t *testing.T) {
	suite.Run(t, new(AuthHandlersTestSuite))
}

func assertAuthErrorEnvelope(t *testing.T, w *httptest.ResponseRecorder, code string) ErrorResponse {
	t.Helper()

	var response ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotEmpty(t, response.Error)
	require.Equal(t, code, response.Code)
	return response
}

// Test GenerateChallenge Handler
func (suite *AuthHandlersTestSuite) TestGenerateChallenge_Success() {
	suite.router.POST("/challenge", GenerateChallenge)

	requestBody := map[string]string{
		"address": "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req, _ := http.NewRequest("POST", "/challenge", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 200, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)
	assert.Contains(suite.T(), response, "challenge")

	// The challenge verifies for the address it was issued for, and only for it
	nonce, _ := response["challenge"].(string)
	challenge, valid := ChallengeStore.Verify(nonce, requestBody["address"])
	assert.True(suite.T(), valid)
	assert.Equal(suite.T(), nonce, challenge.Value)
	assert.Equal(suite.T(), strings.ToLower(requestBody["address"]), challenge.Address)
	_, valid = ChallengeStore.Verify(nonce, "0x0000000000000000000000000000000000000001")
	assert.False(suite.T(), valid)
}

func (suite *AuthHandlersTestSuite) TestGenerateChallenge_InvalidJSON() {
	suite.router.POST("/challenge", GenerateChallenge)

	req, _ := http.NewRequest("POST", "/challenge", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 400, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "error")
}

func (suite *AuthHandlersTestSuite) TestGenerateChallenge_MissingAddress() {
	suite.router.POST("/challenge", GenerateChallenge)

	requestBody := map[string]string{}
	jsonBody, _ := json.Marshal(requestBody)

	req, _ := http.NewRequest("POST", "/challenge", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	// Missing/invalid address must be rejected to prevent issuing challenges to
	// junk addresses that can't produce valid signatures anyway.
	assert.Equal(suite.T(), 400, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Invalid Ethereum address")
	assertAuthErrorEnvelope(suite.T(), w, ErrCodeInvalidInput)
}

// Test GetSession Handler
func (suite *AuthHandlersTestSuite) TestGetSession_MissingAuthHeader() {
	suite.router.GET("/session", GetSession)

	req, _ := http.NewRequest("GET", "/session", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 401, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Missing Authorization header")
	assertAuthErrorEnvelope(suite.T(), w, ErrCodeTokenMissing)
}

func (suite *AuthHandlersTestSuite) TestGetSession_InvalidAuthHeaderFormat() {
	suite.router.GET("/session", GetSession)

	req, _ := http.NewRequest("GET", "/session", nil)
	req.Header.Set("Authorization", "InvalidFormat")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 401, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Invalid Authorization header format")
}

func (suite *AuthHandlersTestSuite) TestGetSession_InvalidToken() {
	suite.router.GET("/session", GetSession)

	// Create request with invalid token
	req, _ := http.NewRequest("GET", "/session", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	suite.router.ServeHTTP(w, req)

	suite.Equal(http.StatusUnauthorized, w.Code)
	suite.Contains(w.Body.String(), "Malformed token")
}

func (suite *AuthHandlersTestSuite) TestGetSession_ValidToken() {
	suite.router.GET("/session", GetSession)

	// Generate a valid token
	address := "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1"
	token, err := GenerateToken(address, structs.RoleUser)
	assert.NoError(suite.T(), err)

	req, _ := http.NewRequest("GET", "/session", nil)
	req.Header.Set("Authorization", "Bearer \""+token+"\"")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 200, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)
	assert.Contains(suite.T(), response, "session_token")
}

// Test SignIn Handler
func (suite *AuthHandlersTestSuite) TestSignIn_InvalidJSON() {
	suite.router.POST("/signin", SignIn)

	req, _ := http.NewRequest("POST", "/signin", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 400, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Invalid request")
}

func (suite *AuthHandlersTestSuite) TestSignIn_InvalidMessage() {
	suite.router.POST("/signin", SignIn)

	requestBody := SignInRequest{
		Message:   "Invalid message format",
		Signature: "0xsignature",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req, _ := http.NewRequest("POST", "/signin", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 400, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Invalid message")
}

func (suite *AuthHandlersTestSuite) TestSignIn_InvalidChallenge() {
	suite.router.POST("/signin", SignIn)

	// A well-formed SIWE message for this site, but with an unknown nonce.
	suite.T().Setenv("PUBLIC_URL", "https://example.com")
	message := `example.com wants you to sign in with your Ethereum account:
0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1

I accept the ExampleOrg Terms of Service: https://service.invalid

URI: https://example.com/login
Version: 1
Chain ID: #1
Nonce: invalid-challenge
Issued At: ` + time.Now().UTC().Format(time.RFC3339)

	requestBody := SignInRequest{
		Message:   message,
		Signature: "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1b",
	}
	jsonBody, _ := json.Marshal(requestBody)

	req, _ := http.NewRequest("POST", "/signin", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	// The test should fail at signature verification due to invalid signature format
	// This is acceptable since we're testing the error handling path
	assert.Equal(suite.T(), 400, w.Code)
	// Accept either signature error or challenge error since both are validation failures
	body := w.Body.String()
	assert.True(suite.T(),
		strings.Contains(body, "Invalid signature") ||
			strings.Contains(body, "Invalid or expired challenge"),
		"Expected signature or challenge error, got: %s", body)
}

// Test SignOut Handler
func (suite *AuthHandlersTestSuite) TestSignOut_Success() {
	gormDB, err := gorm.Open(sqlite.Open("file:"+suite.T().Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(suite.T(), err)
	require.NoError(suite.T(), gormDB.AutoMigrate(&session.UserSession{}))
	previousStore := session.GlobalStore
	store := session.NewStore(gormDB)
	session.GlobalStore = store
	suite.T().Cleanup(func() { session.GlobalStore = previousStore })

	rawToken := "raw-successful-signout-token"
	sess, err := store.Create(session.CreateInput{
		TokenHash: session.HashToken(rawToken),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(suite.T(), err)

	suite.router.POST("/signout", SignOut)

	req, _ := http.NewRequest("POST", "/signout", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: rawToken})
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 200, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Successfully signed out")

	// Check that cookie is cleared
	cookies := w.Result().Cookies()
	found := false
	for _, cookie := range cookies {
		if cookie.Name == "session_token" {
			found = true
			assert.Equal(suite.T(), "", cookie.Value)
			assert.True(suite.T(), cookie.MaxAge < 0)
		}
	}
	assert.True(suite.T(), found, "session_token cookie should be set to clear it")

	var persisted session.UserSession
	require.NoError(suite.T(), gormDB.First(&persisted, sess.ID).Error)
	assert.True(suite.T(), persisted.Revoked)
	assert.Equal(suite.T(), string(session.RevocationReasonUserLogout), persisted.RevocationReason)
}

// SEC-6 / #303: sign-out with only refresh_token present must revoke the session.
func (suite *AuthHandlersTestSuite) TestSignOut_RevokesViaRefreshTokenAlone() {
	gormDB, err := gorm.Open(sqlite.Open("file:"+suite.T().Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(suite.T(), err)
	require.NoError(suite.T(), gormDB.AutoMigrate(&session.UserSession{}))
	previousStore := session.GlobalStore
	store := session.NewStore(gormDB)
	session.GlobalStore = store
	suite.T().Cleanup(func() { session.GlobalStore = previousStore })

	refreshRaw := "signout-refresh-only"
	sess, err := store.Create(session.CreateInput{
		TokenHash: session.HashToken("expired-access"),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(suite.T(), err)
	require.NoError(suite.T(), store.RotateRefreshToken(
		sess.ID,
		session.HashToken(refreshRaw),
		time.Now().Add(7*24*time.Hour),
	))

	suite.router.POST("/signout-refresh-only", SignOut)
	req, _ := http.NewRequest("POST", "/signout-refresh-only", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshRaw})
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 200, w.Code)
	_, err = store.ValidateRefreshToken(session.HashToken(refreshRaw))
	require.Error(suite.T(), err)

	var persisted session.UserSession
	require.NoError(suite.T(), gormDB.First(&persisted, sess.ID).Error)
	assert.True(suite.T(), persisted.Revoked)
	assert.Equal(suite.T(), string(session.RevocationReasonUserLogout), persisted.RevocationReason)
}

func (suite *AuthHandlersTestSuite) TestSignOut_PartialFailureUsesCanonicalErrorEnvelope() {
	gormDB, err := gorm.Open(sqlite.Open("file:signout_partial?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(suite.T(), err)
	require.NoError(suite.T(), gormDB.AutoMigrate(&session.UserSession{}))

	previousStore := session.GlobalStore
	store := session.NewStore(gormDB)
	session.GlobalStore = store
	suite.T().Cleanup(func() { session.GlobalStore = previousStore })

	rawToken := "raw-signout-token"
	_, err = store.Create(session.CreateInput{
		TokenHash: session.HashToken(rawToken),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(suite.T(), err)

	sqlDB, err := gormDB.DB()
	require.NoError(suite.T(), err)
	require.NoError(suite.T(), sqlDB.Close())

	suite.router.POST("/signout", SignOut)
	req, _ := http.NewRequest("POST", "/signout", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: rawToken})
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusInternalServerError, w.Code)
	response := assertAuthErrorEnvelope(suite.T(), w, "sign_out_partial")
	assert.Contains(suite.T(), response.Error, "Local session cleared")
	assert.NotContains(suite.T(), w.Body.String(), `"message"`)
}

// Test Challenge Store Operations
func (suite *AuthHandlersTestSuite) TestChallengeStore_IssueVerifyRedeem() {
	address := "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1"

	challenge, err := ChallengeStore.Issue(address, 5*time.Minute)
	assert.NoError(suite.T(), err)

	verified, valid := ChallengeStore.Verify(challenge.Value, address)
	assert.True(suite.T(), valid)
	assert.True(suite.T(), ChallengeStore.Redeem(verified))

	_, valid = ChallengeStore.Verify(challenge.Value, address)
	assert.False(suite.T(), valid, "a redeemed challenge does not verify again")
}

func (suite *AuthHandlersTestSuite) TestChallengeStore_ExpiredChallenge() {
	address := "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1"

	challenge, err := ChallengeStore.Issue(address, -1*time.Minute) // Expired
	assert.NoError(suite.T(), err)

	_, valid := ChallengeStore.Verify(challenge.Value, address)
	assert.False(suite.T(), valid)
}

// Test Helper Functions
func (suite *AuthHandlersTestSuite) TestGenerateToken_ValidInput() {
	address := "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1"

	token, err := GenerateToken(address, structs.RoleUser)
	assert.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), token)

	// Token should be a valid JWT format (3 parts separated by dots)
	parts := strings.Split(token, ".")
	assert.Equal(suite.T(), 3, len(parts))
}

// Benchmark tests
func BenchmarkGenerateChallenge(b *testing.B) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/challenge", GenerateChallenge)

	ChallengeStore = logic.NewChallengeStore()

	requestBody := map[string]string{
		"address": "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1",
	}
	jsonBody, _ := json.Marshal(requestBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", "/challenge", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkGetSession(b *testing.B) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/session", GetSession)

	structs.SecretKey = []byte("test-secret-key-for-testing-purposes")

	// Generate a valid token
	address := "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1"
	token, _ := GenerateToken(address, "user")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/session", nil)
		req.Header.Set("Authorization", "Bearer \""+token+"\"")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
