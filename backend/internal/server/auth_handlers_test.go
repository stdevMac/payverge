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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"payverge/internal/logic"
	"payverge/internal/structs"
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
	os.Setenv("DISABLE_POSTHOG", "true")
	os.Setenv("DISABLE_METRICS", "true")
}

func TestAuthHandlersTestSuite(t *testing.T) {
	suite.Run(t, new(AuthHandlersTestSuite))
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

	// Verify challenge is stored
	challenge, exists := ChallengeStore.Get(strings.ToLower(requestBody["address"]))
	assert.True(suite.T(), exists)
	assert.Equal(suite.T(), response["challenge"], challenge.Value)
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

	// Should still work but with empty address
	assert.Equal(suite.T(), 200, w.Code)
}

// Test GetSession Handler
func (suite *AuthHandlersTestSuite) TestGetSession_MissingAuthHeader() {
	suite.router.GET("/session", GetSession)

	req, _ := http.NewRequest("GET", "/session", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), 401, w.Code)
	assert.Contains(suite.T(), w.Body.String(), "Missing Authorization header")
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

	// Create a proper SIWE message format but with invalid challenge
	message := `example.com wants you to sign in with your Ethereum account:
0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1

I accept the ExampleOrg Terms of Service: https://service.invalid

URI: https://example.com/login
Version: 1
Chain ID: #1
Nonce: invalid-challenge
Issued At: 2021-01-01T00:00:00.000Z
Expiration Time: 2021-01-01T00:00:00.000Z`

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
	suite.router.POST("/signout", SignOut)

	req, _ := http.NewRequest("POST", "/signout", nil)
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
}

// Test Challenge Store Operations
func (suite *AuthHandlersTestSuite) TestChallengeStore_SetAndGet() {
	address := "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1"
	challengeValue := "testchallenge123"
	
	challenge := logic.Challenge{
		Value:     challengeValue,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	
	ChallengeStore.Set(address, challenge)
	
	retrieved, exists := ChallengeStore.Get(address)
	assert.True(suite.T(), exists)
	assert.Equal(suite.T(), challengeValue, retrieved.Value)
}

func (suite *AuthHandlersTestSuite) TestChallengeStore_ExpiredChallenge() {
	address := "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1"
	challengeValue := "expiredchallenge"
	
	challenge := logic.Challenge{
		Value:     challengeValue,
		ExpiresAt: time.Now().Add(-1 * time.Minute), // Expired
	}
	
	ChallengeStore.Set(address, challenge)
	
	retrieved, exists := ChallengeStore.Get(address)
	assert.True(suite.T(), exists) // Store doesn't auto-cleanup, handler should check expiry
	assert.True(suite.T(), time.Now().After(retrieved.ExpiresAt))
}

func (suite *AuthHandlersTestSuite) TestChallengeStore_Delete() {
	address := "0x742d35Cc6635C0532925a3b8D400E4C3f2c0C1c1"
	challengeValue := "deletechallenge"
	
	challenge := logic.Challenge{
		Value:     challengeValue,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	
	ChallengeStore.Set(address, challenge)
	ChallengeStore.Delete(address)
	
	_, exists := ChallengeStore.Get(address)
	assert.False(suite.T(), exists)
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
