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

// newAnnouncementLifecycleServer wires the announcement lifecycle + batch-summary
// routes exactly as main.go does (same paths, same param shapes) so this also
// smoke-checks that ack-summaries (static) alongside :announcementId (param) does
// not panic on registration.
func newAnnouncementLifecycleServer(t *testing.T, staffID uint, role string) (func(method, url string, body interface{}) *httptest.ResponseRecorder, *gorm.DB, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}, &database.Position{},
		&database.StaffPosition{}, &database.Announcement{}, &database.AnnouncementAck{}, &database.RBACAuditLog{}))
	database.SetTestDB(gormDB)
	h := NewChatHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if staffID != 0 {
			c.Set("staff_id", staffID)
			c.Set("staff_role", role)
		}
		c.Next()
	})
	r.POST("/b/:id/announcements", h.CreateAnnouncement)
	r.GET("/b/:id/announcements/ack-summaries", h.GetAnnouncementAckSummaries)
	r.GET("/b/:id/announcements/:announcementId/acks", h.GetAnnouncementAcks)
	r.POST("/b/:id/announcements/:announcementId/ack", h.AckAnnouncement)
	r.PUT("/b/:id/announcements/:announcementId", h.UpdateAnnouncement)
	r.DELETE("/b/:id/announcements/:announcementId", h.DeleteAnnouncement)
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
	return do, gormDB, func() { _ = sqlDB.Close() }
}

// TestAnnouncementAckSummariesHTTP drives the batch endpoint: ?ids=a,b returns a
// per-id {acked,total_eligible,first_acker_names} map in one call.
func TestAnnouncementAckSummariesHTTP(t *testing.T) {
	do, g, cleanup := newAnnouncementLifecycleServer(t, 0, "") // owner
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.Staff{ID: 1, BusinessID: 1, Email: "a@x.co", Name: "Ana", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	require.NoError(t, g.Create(&database.Staff{ID: 2, BusinessID: 1, Email: "b@x.co", Name: "Bo", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)

	// Create two require_ack announcements as the owner.
	w := do(http.MethodPost, "/b/1/announcements", map[string]interface{}{"title": "One", "require_ack": true, "audience_filter": "all"})
	require.Equal(t, http.StatusCreated, w.Code)
	var c1 struct {
		Data database.Announcement `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &c1))
	w = do(http.MethodPost, "/b/1/announcements", map[string]interface{}{"title": "Two", "require_ack": true, "audience_filter": "all"})
	require.Equal(t, http.StatusCreated, w.Code)
	var c2 struct {
		Data database.Announcement `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &c2))

	// Ana acks the first only.
	require.NoError(t, g.Create(&database.AnnouncementAck{AnnouncementID: c1.Data.ID, StaffID: 1, BusinessID: 1}).Error)

	url := fmt.Sprintf("/b/1/announcements/ack-summaries?ids=%d,%d", c1.Data.ID, c2.Data.ID)
	w = do(http.MethodGet, url, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data map[string]database.AnnouncementAckSummary `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2)
	first := resp.Data[fmt.Sprintf("%d", c1.Data.ID)]
	require.Equal(t, 1, first.Acked)
	require.Equal(t, 2, first.TotalEligible)
	require.Equal(t, []string{"Ana"}, first.FirstAckerNames)
	second := resp.Data[fmt.Sprintf("%d", c2.Data.ID)]
	require.Equal(t, 0, second.Acked)
	require.Equal(t, 2, second.TotalEligible)

	// Empty ids → empty map, still 200.
	w = do(http.MethodGet, "/b/1/announcements/ack-summaries", nil)
	require.Equal(t, http.StatusOK, w.Code)
}

// TestAnnouncementEditDeleteHTTP drives the lifecycle: edit updates in place; a
// delete removes it (subsequent read is not-found) and confirms the id is gone.
func TestAnnouncementEditDeleteHTTP(t *testing.T) {
	do, g, cleanup := newAnnouncementLifecycleServer(t, 0, "") // owner
	defer cleanup()
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.Staff{ID: 1, BusinessID: 1, Email: "a@x.co", Name: "Ana", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)

	w := do(http.MethodPost, "/b/1/announcements", map[string]interface{}{"title": "Old", "content": "old", "require_ack": false, "audience_filter": "all"})
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		Data database.Announcement `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	id := created.Data.ID

	// Edit in place.
	w = do(http.MethodPut, fmt.Sprintf("/b/1/announcements/%d", id), map[string]interface{}{"title": "New", "content": "new", "require_ack": true, "audience_filter": "role:server"})
	require.Equal(t, http.StatusOK, w.Code)
	var edited struct {
		Data database.Announcement `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &edited))
	require.Equal(t, id, edited.Data.ID)
	require.Equal(t, "New", edited.Data.Title)
	require.True(t, edited.Data.RequireAck)

	// Delete.
	w = do(http.MethodDelete, fmt.Sprintf("/b/1/announcements/%d", id), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var remaining int64
	require.NoError(t, g.Model(&database.Announcement{}).Where("id = ?", id).Count(&remaining).Error)
	require.Equal(t, int64(0), remaining)

	// A second delete is not-found.
	w = do(http.MethodDelete, fmt.Sprintf("/b/1/announcements/%d", id), nil)
	require.Equal(t, http.StatusNotFound, w.Code)

	// Invalid edit body (empty title) is a 400.
	w = do(http.MethodPost, "/b/1/announcements", map[string]interface{}{"title": "Keep", "audience_filter": "all"})
	require.Equal(t, http.StatusCreated, w.Code)
	var keep struct {
		Data database.Announcement `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &keep))
	w = do(http.MethodPut, fmt.Sprintf("/b/1/announcements/%d", keep.Data.ID), map[string]interface{}{"title": "  ", "audience_filter": "all"})
	require.Equal(t, http.StatusBadRequest, w.Code)
}
