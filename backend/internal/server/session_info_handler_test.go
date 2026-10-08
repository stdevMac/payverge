package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type SessionInfoTestSuite struct {
	suite.Suite
	router *gin.Engine
}

func (s *SessionInfoTestSuite) SetupTest() {
	gin.SetMode(gin.TestMode)
	s.router = gin.New()
	s.router.GET("/auth/session-info", GetSessionInfo)

	// Ensure a deterministic secret for token generation in tests.
	structs.SecretKey = []byte("test-secret-key-for-testing-purposes")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", s.T().Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	s.Require().NoError(err)
	sqlDB, err := db.DB()
	s.Require().NoError(err)
	sqlDB.SetMaxOpenConns(1)
	s.Require().NoError(db.AutoMigrate(&session.UserSession{}, &database.Staff{}, &database.Customer{}))
	database.SetTestDB(db)
	session.GlobalStore = session.NewStore(db)
}

func TestSessionInfoTestSuite(t *testing.T) {
	suite.Run(t, new(SessionInfoTestSuite))
}

// TestNoToken verifies that a request without any token returns authenticated: false.
func (s *SessionInfoTestSuite) TestNoToken() {
	req, _ := http.NewRequest("GET", "/auth/session-info", nil)
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp SessionInfoResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.False(s.T(), resp.Authenticated)
	assert.Empty(s.T(), resp.Type)
}

// TestWeb3Token verifies that a valid web3 token returns the correct session info.
func (s *SessionInfoTestSuite) TestWeb3Token() {
	token, err := GenerateWeb3Token("0xABCD", "admin", 0)
	assert.NoError(s.T(), err)

	req, _ := http.NewRequest("GET", "/auth/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp SessionInfoResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), resp.Authenticated)
	assert.Equal(s.T(), "web3", resp.Type)
	assert.Equal(s.T(), "0xABCD", resp.Address)
	assert.Equal(s.T(), "admin", resp.Role)
}

// TestUserToken verifies that a valid user token returns the correct session info.
func (s *SessionInfoTestSuite) TestUserToken() {
	token, err := GenerateUserToken(42, "user@example.com", "0x1234", "owner", 0)
	assert.NoError(s.T(), err)

	req, _ := http.NewRequest("GET", "/auth/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp SessionInfoResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), resp.Authenticated)
	assert.Equal(s.T(), "user", resp.Type)
	assert.Equal(s.T(), uint(42), resp.UserID)
	assert.Equal(s.T(), "user@example.com", resp.Email)
	assert.Equal(s.T(), "0x1234", resp.Address)
	assert.Equal(s.T(), "owner", resp.Role)
}

// TestStaffToken verifies that a valid staff token (via staff_token cookie) returns staff info.
func (s *SessionInfoTestSuite) TestStaffToken() {
	staff := &database.Staff{
		BusinessID: 10,
		Email:      "staff@example.com",
		Name:       "Jane",
		Role:       database.StaffRoleManager,
		IsActive:   true,
	}
	s.Require().NoError(database.GetDB().Create(staff).Error)

	token, err := generateTestStaffToken(staff.ID, staff.Email, staff.Name, string(staff.Role), staff.BusinessID)
	assert.NoError(s.T(), err)

	req, _ := http.NewRequest("GET", "/auth/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: token})
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp SessionInfoResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), resp.Authenticated)
	assert.Equal(s.T(), "staff", resp.Type)
	assert.Equal(s.T(), uint(1), resp.StaffID)
	assert.Equal(s.T(), "staff@example.com", resp.Email)
	assert.Equal(s.T(), "Jane", resp.StaffName)
	assert.Equal(s.T(), "manager", resp.Role)
	assert.Equal(s.T(), uint(10), resp.BusinessID)
}

// TestCustomerToken verifies that a valid customer token returns customer info.
func (s *SessionInfoTestSuite) TestCustomerToken() {
	customer := &database.Customer{
		Email:    "cust@example.com",
		Name:     "Customer",
		IsActive: true,
	}
	s.Require().NoError(database.GetDB().Create(customer).Error)

	token, err := GenerateCustomerToken(customer.ID, customer.Email, 0)
	assert.NoError(s.T(), err)

	req, _ := http.NewRequest("GET", "/auth/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "customer_token", Value: token})
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp SessionInfoResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), resp.Authenticated)
	assert.Equal(s.T(), "customer", resp.Type)
	assert.Equal(s.T(), customer.ID, resp.CustomerID)
	assert.Equal(s.T(), "cust@example.com", resp.Email)
}

// TestInvalidToken verifies that a garbage token returns authenticated: false.
func (s *SessionInfoTestSuite) TestInvalidToken() {
	req, _ := http.NewRequest("GET", "/auth/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "not-a-real-jwt"})
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp SessionInfoResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.False(s.T(), resp.Authenticated)
}

// generateTestStaffToken creates a staff JWT without needing a database.Staff model.
func generateTestStaffToken(staffID uint, email, name, role string, businessID uint) (string, error) {
	claims := jwt.MapClaims{
		"staff_id":    staffID,
		"email":       email,
		"name":        name,
		"role":        role,
		"business_id": businessID,
		"type":        "staff",
		"session_id":  uint(0),
		"exp":         time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(structs.SecretKey)
}

func TestGetSessionInfo_FallsThroughRevokedStaffCookieToUserSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.User{}))

	business := createOwnedBusiness(t, "0xOwnerA", "Shadow Cookie Biz")
	staff := createStaffMember(t, business.ID, "stale-staff@example.com", "Stale Staff")
	staffToken := generateStaffTokenWithSession(t, staff)
	require.NoError(t, session.GlobalStore.RevokeByTokenHashWithReason(session.HashToken(staffToken), session.RevocationReasonUserLogout))

	user := &database.User{Email: "owner@example.com", Role: "user", EmailVerified: true}
	require.NoError(t, database.GetDB().Create(user).Error)
	userSess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &user.ID,
		TokenHash: "placeholder",
		Provider:  "web3",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	userToken, err := GenerateUserToken(user.ID, user.Email, "", user.Role, userSess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(userSess.ID, session.HashToken(userToken)))

	router := gin.New()
	router.GET("/auth/session-info", GetSessionInfo)
	req, _ := http.NewRequest("GET", "/auth/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: staffToken})
	req.AddCookie(&http.Cookie{Name: "session_token", Value: userToken})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp SessionInfoResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Authenticated)
	assert.Equal(t, "user", resp.Type)
	assert.Equal(t, user.ID, resp.UserID)
}

func TestGetSessionInfo_RejectsInactiveStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	router := gin.New()
	router.GET("/auth/session-info", GetSessionInfo)

	business := createOwnedBusiness(t, "0xOwnerA", "Session Info Biz")
	staff := createStaffMember(t, business.ID, "session-info@example.com", "Session Info Staff")
	token := generateStaffTokenWithSession(t, staff)
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("is_active", false).Error)

	req, _ := http.NewRequest("GET", "/auth/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp SessionInfoResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Authenticated)
}

func TestGetSessionInfo_UsesLiveStaffRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	router := gin.New()
	router.GET("/auth/session-info", GetSessionInfo)

	business := createOwnedBusiness(t, "0xOwnerA", "Session Role Biz")
	staff := createStaffMember(t, business.ID, "session-role@example.com", "Session Role Staff")
	token := generateStaffTokenWithSession(t, staff)
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("role", database.StaffRoleKitchen).Error)

	req, _ := http.NewRequest("GET", "/auth/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp SessionInfoResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Authenticated)
	assert.Equal(t, "staff", resp.Type)
	assert.Equal(t, string(database.StaffRoleKitchen), resp.Role)
	assert.Equal(t, staff.Name, resp.StaffName)
	assert.Equal(t, business.ID, resp.BusinessID)
}

func TestGetSessionInfo_RejectsInactiveCustomer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSessionTestDB(t)
	session.GlobalStore = session.NewStore(db)
	structs.SecretKey = []byte("test-secret-key-for-session-tests")

	router := gin.New()
	router.GET("/auth/session-info", GetSessionInfo)

	customer := &database.Customer{
		Email:    "session-customer@example.com",
		Name:     "Session Customer",
		IsActive: true,
	}
	require.NoError(t, db.Create(customer).Error)
	require.NoError(t, db.Model(&database.Customer{}).Where("id = ?", customer.ID).Update("is_active", false).Error)

	tokenString := "inactive-customer-session-info-token"
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

	req, _ := http.NewRequest("GET", "/auth/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "customer_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp SessionInfoResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Authenticated)
}

func TestGetSessionInfo_RejectsRevokedSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSessionTestDB(t)
	session.GlobalStore = session.NewStore(db)
	structs.SecretKey = []byte("test-secret-key-for-session-tests")

	router := gin.New()
	router.GET("/auth/session-info", GetSessionInfo)

	token, err := GenerateUserToken(42, "revoked@example.com", "0x1234", "owner", 0)
	require.NoError(t, err)

	userID := uint(42)
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &userID,
		TokenHash: session.HashToken(token),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	token, err = GenerateUserToken(42, "revoked@example.com", "0x1234", "owner", sess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))
	require.NoError(t, session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonUnspecified))

	req, _ := http.NewRequest("GET", "/auth/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp SessionInfoResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Authenticated)
}
