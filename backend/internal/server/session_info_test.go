package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupSessionInfoTestDB seeds an in-memory DB with a Business + Staff row
// and returns the staff JWT plus the seeded business name.
func setupSessionInfoTestDB(t *testing.T) (token string, businessName string) {
	t.Helper()
	t.Setenv("JWT_SECRET_KEY", "test-session-info-secret-key-1234")

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := gormDB.AutoMigrate(&database.Business{}, &database.Staff{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.SetTestDB(gormDB)

	biz := database.Business{Name: "Real Business Name", Email: "biz@example.test"}
	biz.ID = 7
	if err := gormDB.Create(&biz).Error; err != nil {
		t.Fatalf("create business: %v", err)
	}
	staff := database.Staff{
		BusinessID: biz.ID,
		Name:       "Test Manager",
		Email:      "manager@example.test",
		Role:       "manager",
		IsActive:   true,
	}
	if err := gormDB.Create(&staff).Error; err != nil {
		t.Fatalf("create staff: %v", err)
	}

	tok, err := GenerateStaffToken(&staff)
	if err != nil {
		t.Fatalf("generate staff token: %v", err)
	}
	return tok, biz.Name
}

// testUserAuth is a minimal local mapping to the user_auths table so this test
// can seed verification rows without importing the auth package (which imports
// server, so a direct import would be a cycle).
type testUserAuth struct {
	ID             uint `gorm:"primaryKey"`
	UserID         uint
	Provider       string
	ProviderUserID string
	EmailVerified  bool
}

func (testUserAuth) TableName() string { return "user_auths" }

func setupUserSessionInfoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("JWT_SECRET_KEY", "test-session-info-secret-key-1234")
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := gormDB.AutoMigrate(&database.User{}, &testUserAuth{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.SetTestDB(gormDB)
	return gormDB
}

func fetchUserSessionInfo(t *testing.T, token string) SessionInfoResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/session-info", GetSessionInfo)
	req := httptest.NewRequest(http.MethodGet, "/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: token, Expires: time.Now().Add(time.Hour)})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var resp SessionInfoResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp
}

func TestGetSessionInfo_UserReportsEmailVerified(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION", "required")
	gormDB := setupUserSessionInfoTestDB(t)

	// Unverified email/password user.
	unverified := database.User{Email: "unverified@example.test", Name: "Uv", Role: "user", AuthMethod: "email"}
	if err := gormDB.Create(&unverified).Error; err != nil {
		t.Fatalf("create unverified user: %v", err)
	}
	if err := gormDB.Create(&testUserAuth{UserID: unverified.ID, Provider: "email", ProviderUserID: unverified.Email, EmailVerified: false}).Error; err != nil {
		t.Fatalf("create unverified auth: %v", err)
	}

	// Verified email/password user.
	verified := database.User{Email: "verified@example.test", Name: "V", Role: "user", AuthMethod: "email"}
	if err := gormDB.Create(&verified).Error; err != nil {
		t.Fatalf("create verified user: %v", err)
	}
	if err := gormDB.Create(&testUserAuth{UserID: verified.ID, Provider: "email", ProviderUserID: verified.Email, EmailVerified: true}).Error; err != nil {
		t.Fatalf("create verified auth: %v", err)
	}

	// OAuth-only user: no email/password auth row — email is provider-verified.
	oauth := database.User{Email: "oauth@example.test", Name: "O", Role: "user", AuthMethod: "google"}
	if err := gormDB.Create(&oauth).Error; err != nil {
		t.Fatalf("create oauth user: %v", err)
	}

	cases := []struct {
		name string
		user database.User
		want bool
	}{
		{"unverified email user", unverified, false},
		{"verified email user", verified, true},
		{"oauth user has no email/password row", oauth, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := GenerateUserToken(tc.user.ID, tc.user.Email, "", tc.user.Role)
			if err != nil {
				t.Fatalf("generate token: %v", err)
			}
			resp := fetchUserSessionInfo(t, token)
			if !resp.Authenticated || resp.Type != "user" {
				t.Fatalf("expected authenticated user session, got %+v", resp)
			}
			if resp.EmailVerified != tc.want {
				t.Errorf("email_verified: want %v, got %v", tc.want, resp.EmailVerified)
			}
		})
	}
}

// EMAIL_VERIFICATION=off: the stored flag stays false, but no verification is
// pending, so the client must not show the verify-email banner.
func TestGetSessionInfo_VerificationOffReportsNothingPending(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION", "off")
	gormDB := setupUserSessionInfoTestDB(t)
	user := database.User{Email: "off-mode@example.test", Name: "Off", Role: "user", AuthMethod: "email"}
	if err := gormDB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := gormDB.Create(&testUserAuth{UserID: user.ID, Provider: "email", ProviderUserID: user.Email, EmailVerified: false}).Error; err != nil {
		t.Fatalf("create auth: %v", err)
	}
	token, err := GenerateUserToken(user.ID, user.Email, "", user.Role)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	resp := fetchUserSessionInfo(t, token)
	if !resp.Authenticated || !resp.EmailVerified {
		t.Fatalf("verification off: want authenticated with email_verified=true, got %+v", resp)
	}
}

func TestGetSessionInfo_StaffReturnsBusinessName(t *testing.T) {
	token, businessName := setupSessionInfoTestDB(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/session-info", GetSessionInfo)

	req := httptest.NewRequest(http.MethodGet, "/session-info", nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: token, Expires: time.Now().Add(time.Hour)})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	var resp SessionInfoResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.Authenticated {
		t.Fatal("expected authenticated=true")
	}
	if resp.Type != "staff" {
		t.Errorf("type: want staff, got %q", resp.Type)
	}
	if resp.BusinessName != businessName {
		t.Errorf("business_name: want %q, got %q", businessName, resp.BusinessName)
	}
	// Sanity: existing fields still populate.
	if resp.BusinessID != 7 {
		t.Errorf("business_id: want 7, got %d", resp.BusinessID)
	}
}
