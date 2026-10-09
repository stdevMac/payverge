package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newNotificationsTestServer(t *testing.T) (func(staffID uint, method, url string, body interface{}) *httptest.ResponseRecorder, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&database.Business{}, &database.Staff{}, &database.StaffNotification{}))
	database.SetTestDB(g)
	h := NewNotificationsHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		if v, err := strconv.ParseUint(c.GetHeader("X-Test-Staff"), 10, 64); err == nil {
			c.Set("staff_id", uint(v))
		}
		c.Next()
	})
	r.GET("/b/:id/me/notifications", h.ListMine)
	r.GET("/b/:id/me/notifications/unread-count", h.UnreadCount)
	r.POST("/b/:id/me/notifications/read", h.MarkRead)
	do := func(staffID uint, method, url string, body interface{}) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			b, err := json.Marshal(body)
			require.NoError(t, err)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, url, rdr)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Staff", strconv.FormatUint(uint64(staffID), 10))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	return do, func() { _ = sqlDB.Close() }
}

func TestNotificationsSelfRoutesRejectZeroStaff(t *testing.T) {
	do, cleanup := newNotificationsTestServer(t)
	defer cleanup()
	require.NoError(t, database.GetDB().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	for _, tc := range []struct{ method, url string }{
		{http.MethodGet, "/b/1/me/notifications"},
		{http.MethodGet, "/b/1/me/notifications/unread-count"},
		{http.MethodPost, "/b/1/me/notifications/read"},
	} {
		w := do(0, tc.method, tc.url, map[string]any{"all": true})
		require.Equal(t, http.StatusForbidden, w.Code, "%s %s must 403 without a staff identity", tc.method, tc.url)
	}
}

func TestNotificationsListAndRead(t *testing.T) {
	do, cleanup := newNotificationsTestServer(t)
	defer cleanup()
	require.NoError(t, database.GetDB().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, database.GetDBWrapper().CreateStaffNotifications([]database.StaffNotification{
		{BusinessID: 1, StaffID: 7, Kind: "shift.assigned", Title: "A", Body: "b"},
		{BusinessID: 1, StaffID: 7, Kind: "chat.announcement", Title: "B", Body: "b"},
		{BusinessID: 1, StaffID: 8, Kind: "shift.assigned", Title: "other", Body: "b"},
	}))

	// List self only (staff 7 → 2 rows; never staff 8's).
	w := do(7, http.MethodGet, "/b/1/me/notifications", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Data []database.StaffNotification `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Len(t, listResp.Data, 2)
	require.NotContains(t, w.Body.String(), "$") // money-free

	// Unread count.
	w = do(7, http.MethodGet, "/b/1/me/notifications/unread-count", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var cnt struct {
		Data struct {
			Count int `json:"count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cnt))
	require.Equal(t, 2, cnt.Data.Count)

	// Mark all read → count 0.
	w = do(7, http.MethodPost, "/b/1/me/notifications/read", map[string]any{"all": true})
	require.Equal(t, http.StatusOK, w.Code)
	w = do(7, http.MethodGet, "/b/1/me/notifications/unread-count", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cnt))
	require.Equal(t, 0, cnt.Data.Count)
}
