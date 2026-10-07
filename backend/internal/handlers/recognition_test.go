package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newRecognitionTestServer(t *testing.T, staffID uint, role database.StaffRole) (
	func(method, url string, body interface{}) *httptest.ResponseRecorder, *database.DB, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}, &database.Shoutout{}))
	database.SetTestDB(gormDB)
	d := database.GetDBWrapper()
	h := NewRecognitionHandler(d)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if staffID != 0 {
			c.Set("staff_id", staffID)
			c.Set("staff_role", string(role))
		}
		c.Next()
	})
	r.GET("/b/:id/shoutouts", h.List)
	r.POST("/b/:id/shoutouts", h.Create)
	do := func(method, url string, body interface{}) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			bb, _ := json.Marshal(body)
			rdr = bytes.NewReader(bb)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, url, rdr)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	return do, d, func() { _ = sqlDB.Close() }
}

// TestShoutoutFromStaffIsContextNotBody: a forged from_staff_id in the body is
// ignored; the sender is always the authenticated caller.
func TestShoutoutFromStaffIsContextNotBody(t *testing.T) {
	do, d, cleanup := newRecognitionTestServer(t, 7, database.StaffRoleServer)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, d.GetGorm().Create(&database.Staff{ID: 3, BusinessID: 1, Email: "s3@x.co", Name: "Bo", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)

	w := do(http.MethodPost, "/b/1/shoutouts", map[string]any{
		"from_staff_id": 999, // forged — must be ignored
		"to_staff_id":   3,
		"message":       "great close",
		"emoji":         "🙌",
		"visibility":    "team",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var resp struct {
		Data struct {
			FromStaffID uint `json:"from_staff_id"`
			ToStaffID   uint `json:"to_staff_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, uint(7), resp.Data.FromStaffID, "sender must come from context")
	require.Equal(t, uint(3), resp.Data.ToStaffID)
}

// TestShoutoutCreateValidation: missing recipient / empty message → 400.
func TestShoutoutCreateValidation(t *testing.T) {
	do, d, cleanup := newRecognitionTestServer(t, 7, database.StaffRoleServer)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)

	w := do(http.MethodPost, "/b/1/shoutouts", map[string]any{"message": "hi", "visibility": "team"})
	require.Equal(t, http.StatusBadRequest, w.Code, "missing to_staff_id must 400")

	w = do(http.MethodPost, "/b/1/shoutouts", map[string]any{"to_staff_id": 3, "message": "   "})
	require.Equal(t, http.StatusBadRequest, w.Code, "empty message must 400")
}

// TestShoutoutRejectsSelf (MIN-4): shouting out yourself → 400.
func TestShoutoutRejectsSelf(t *testing.T) {
	do, d, cleanup := newRecognitionTestServer(t, 7, database.StaffRoleServer)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, d.GetGorm().Create(&database.Staff{ID: 7, BusinessID: 1, Email: "s7@x.co", Name: "Me", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)

	w := do(http.MethodPost, "/b/1/shoutouts", map[string]any{"to_staff_id": 7, "message": "I am great"})
	require.Equal(t, http.StatusBadRequest, w.Code, "self-shoutout must 400")
}

// TestShoutoutRejectsForeignRecipient (MIN-4): a recipient outside the business
// → 400.
func TestShoutoutRejectsForeignRecipient(t *testing.T) {
	do, d, cleanup := newRecognitionTestServer(t, 7, database.StaffRoleServer)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)

	w := do(http.MethodPost, "/b/1/shoutouts", map[string]any{"to_staff_id": 999, "message": "hi"})
	require.Equal(t, http.StatusBadRequest, w.Code, "recipient not in business must 400")
}
