package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logic"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSignIn_ExpiresStaffCookieOnOwnerLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRealmSwitchOwnerDB(t)
	address, message, signature := signedSIWEChallenge(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, err := json.Marshal(SignInRequest{Message: message, Signature: signature})
	require.NoError(t, err)
	c.Request = httptest.NewRequest(http.MethodPost, "/signin", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.AddCookie(&http.Cookie{Name: "staff_token", Value: "live-staff-cookie"})
	c.Request.AddCookie(&http.Cookie{Name: "customer_token", Value: "live-customer-cookie"})

	SignIn(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.True(t, hasExpiredSetCookie(w, "staff_token"), "successful SIWE owner login must expire a live staff cookie")
	require.True(t, hasExpiredSetCookie(w, "customer_token"), "successful SIWE owner login must expire a live customer cookie")
	require.NotEmpty(t, liveSetCookieValue(w, "session_token"))
	require.Equal(t, strings.ToLower(address), jsonBodyString(t, w, "address"))
}

func TestSignIn_SameRealmLoginStillSetsOwnerSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRealmSwitchOwnerDB(t)
	_, message, signature := signedSIWEChallenge(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, err := json.Marshal(SignInRequest{Message: message, Signature: signature})
	require.NoError(t, err)
	c.Request = httptest.NewRequest(http.MethodPost, "/signin", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: "stale-owner-cookie"})

	SignIn(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotEmpty(t, liveSetCookieValue(w, "session_token"))
	require.NotEqual(t, "stale-owner-cookie", liveSetCookieValue(w, "session_token"))
}

func TestSignIn_FailedSignatureDoesNotExpireStaffCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupRealmSwitchOwnerDB(t)
	_, message, _ := signedSIWEChallenge(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body, err := json.Marshal(SignInRequest{Message: message, Signature: "0x" + strings.Repeat("ab", 65)})
	require.NoError(t, err)
	c.Request = httptest.NewRequest(http.MethodPost, "/signin", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.AddCookie(&http.Cookie{Name: "staff_token", Value: "live-staff-cookie"})

	SignIn(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.False(t, hasExpiredSetCookie(w, "staff_token"), "failed SIWE must not destroy an existing staff session")
	require.Empty(t, liveSetCookieValue(w, "session_token"))
}

func TestVerifyLoginCode_ExpiresOwnerCookiesOnStaffLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	business := createOwnedBusiness(t, "0xOwnerRealm", "Realm Switch Biz")
	staff := createStaffMember(t, business.ID, "realm-staff@example.com", "Realm Staff")
	require.NoError(t, database.GetDB().Create(&database.StaffLoginCode{
		StaffID: staff.ID, Code: "424242", ExpiresAt: time.Now().Add(10 * time.Minute), Used: false,
	}).Error)

	body, err := json.Marshal(map[string]string{"email": staff.Email, "code": "424242"})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: "live-owner-cookie"})
	c.Request.AddCookie(&http.Cookie{Name: "customer_token", Value: "live-customer-cookie"})

	VerifyLoginCode(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.True(t, hasExpiredSetCookie(w, "session_token"), "successful staff-code login must expire a live owner cookie")
	require.True(t, hasExpiredSetCookie(w, "customer_token"), "successful staff-code login must expire a live customer cookie")
	require.NotEmpty(t, liveSetCookieValue(w, "staff_token"))
}

func TestVerifyLoginCode_InvalidCodeDoesNotExpireOwnerCookies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	business := createOwnedBusiness(t, "0xOwnerRealmFail", "Realm Fail Biz")
	staff := createStaffMember(t, business.ID, "realm-fail@example.com", "Realm Fail")
	require.NoError(t, database.GetDB().Create(&database.StaffLoginCode{
		StaffID: staff.ID, Code: "111111", ExpiresAt: time.Now().Add(10 * time.Minute), Used: false,
	}).Error)

	body, err := json.Marshal(map[string]string{"email": staff.Email, "code": "000000"})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: "live-owner-cookie"})

	VerifyLoginCode(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.False(t, hasExpiredSetCookie(w, "session_token"), "failed staff-code login must not destroy an existing owner session")
	require.Empty(t, liveSetCookieValue(w, "staff_token"))
}

func TestAcceptInvitation_ExpiresOwnerCookiesOnStaffLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	business := createOwnedBusiness(t, "0xOwnerInvite", "Invite Realm Biz")
	invitation := createStaffInvitation(t, business.ID, "invite-realm@example.com", "Invitee", database.StaffRoleHost)

	body, err := json.Marshal(map[string]string{"token": invitation.Token, "name": "Accepted Staff"})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: "live-owner-cookie"})

	AcceptInvitation(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.True(t, hasExpiredSetCookie(w, "session_token"), "accepting a staff invite must expire a live owner cookie")
	require.NotEmpty(t, liveSetCookieValue(w, "staff_token"))
}

func setupRealmSwitchOwnerDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.User{}, &session.UserSession{}))
	database.SetTestDB(gormDB)
	structs.SecretKey = []byte("test-secret-key-for-testing-purposes")

	prevStore := session.GlobalStore
	session.GlobalStore = nil
	prevChallenges := ChallengeStore
	ChallengeStore = logic.NewChallengeStore()
	t.Cleanup(func() {
		session.GlobalStore = prevStore
		ChallengeStore = prevChallenges
	})
	return gormDB
}

func signedSIWEChallenge(t *testing.T) (address, message, signature string) {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	address = crypto.PubkeyToAddress(key.PublicKey).Hex()
	t.Setenv("PUBLIC_URL", "https://example.com")
	challenge, err := ChallengeStore.Issue(address, 5*time.Minute)
	require.NoError(t, err)
	nonce := challenge.Value
	require.NoError(t, database.GetDB().Create(&database.User{
		Address: strings.ToLower(address),
		Role:    "user",
	}).Error)
	message = fmt.Sprintf(`example.com wants you to sign in with your Ethereum account:
%s

Sign in to Payverge

URI: https://example.com/login
Version: 1
Chain ID: 1
Nonce: %s
Issued At: %s`, address, nonce, time.Now().UTC().Format(time.RFC3339))
	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)
	sig, err := crypto.Sign(crypto.Keccak256([]byte(prefixed)), key)
	require.NoError(t, err)
	sig[64] += 27
	return address, message, hexutil.Encode(sig)
}

func hasExpiredSetCookie(w *httptest.ResponseRecorder, name string) bool {
	for _, raw := range w.Header().Values("Set-Cookie") {
		if !strings.HasPrefix(raw, name+"=") {
			continue
		}
		if strings.Contains(raw, "Max-Age=0") || strings.HasPrefix(raw, name+"=;") {
			return true
		}
	}
	return false
}

func liveSetCookieValue(w *httptest.ResponseRecorder, name string) string {
	for _, raw := range w.Header().Values("Set-Cookie") {
		if !strings.HasPrefix(raw, name+"=") || strings.Contains(raw, "Max-Age=0") {
			continue
		}
		return strings.TrimPrefix(strings.Split(raw, ";")[0], name+"=")
	}
	return ""
}

func jsonBodyString(t *testing.T, w *httptest.ResponseRecorder, key string) string {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	value, _ := body[key].(string)
	return value
}
