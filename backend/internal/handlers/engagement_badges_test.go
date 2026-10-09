package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newEngagementBadgesTestServer(t *testing.T) (func(staffID uint, role string) *httptest.ResponseRecorder, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(
		&database.Business{}, &database.Staff{}, &database.Position{}, &database.StaffPosition{},
		&database.Announcement{}, &database.AnnouncementAck{}, &database.ChecklistRun{},
	))
	database.SetTestDB(g)
	t.Cleanup(func() { _ = sqlDB.Close() })
	h := NewEngagementBadgesHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		if v, err := strconv.ParseUint(c.GetHeader("X-Test-Staff"), 10, 64); err == nil && v != 0 {
			c.Set("staff_id", uint(v))
		}
		if role := c.GetHeader("X-Test-Role"); role != "" {
			c.Set("staff_role", role)
		}
		c.Next()
	})
	r.GET("/b/:id/me/engagement-badges", h.Badges)
	do := func(staffID uint, role string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodGet, "/b/1/me/engagement-badges", nil)
		req.Header.Set("X-Test-Staff", strconv.FormatUint(uint64(staffID), 10))
		req.Header.Set("X-Test-Role", role)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	return do, g
}

func ptrU(v uint) *uint { return &v }

func TestEngagementBadgesCountsUnackedAndPending(t *testing.T) {
	do, g := newEngagementBadgesTestServer(t)
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&database.Staff{ID: 5, BusinessID: 1, Email: "s5@x.co", Name: "Ana", Role: database.StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)

	// Two require_ack announcements targeted at a server; one non-ack notice.
	require.NoError(t, g.Create(&database.Announcement{BusinessID: 1, AuthorStaffID: 1, Title: "All", Content: "x", RequireAck: true, AudienceFilter: "all", CreatedAt: time.Now().UTC()}).Error)
	require.NoError(t, g.Create(&database.Announcement{BusinessID: 1, AuthorStaffID: 1, Title: "Servers", Content: "x", RequireAck: true, AudienceFilter: "role:server", CreatedAt: time.Now().UTC()}).Error)
	require.NoError(t, g.Create(&database.Announcement{BusinessID: 1, AuthorStaffID: 1, Title: "FYI", Content: "x", RequireAck: false, AudienceFilter: "all", CreatedAt: time.Now().UTC()}).Error)

	// One pending checklist run + one complete (excluded) for this staffer.
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, g.Create(&database.ChecklistRun{BusinessID: 1, TemplateID: 1, AssignedStaffID: ptrU(5), ForDate: day, Status: database.ChecklistRunPending}).Error)
	require.NoError(t, g.Create(&database.ChecklistRun{BusinessID: 1, TemplateID: 1, AssignedStaffID: ptrU(5), ForDate: day, Status: database.ChecklistRunComplete}).Error)

	w := do(5, "server")
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			UnackedAnnouncements int64 `json:"unacked_announcements"`
			PendingChecklists    int64 `json:"pending_checklists"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Equal(t, int64(2), resp.Data.UnackedAnnouncements, "all + role:server require_ack, none acked")
	require.Equal(t, int64(1), resp.Data.PendingChecklists, "one pending; complete excluded")
	// Money-free surface.
	require.NotContains(t, w.Body.String(), "$")
}

func TestEngagementBadgesRejectsOwnerWithNoStaffRow(t *testing.T) {
	do, g := newEngagementBadgesTestServer(t)
	require.NoError(t, g.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	w := do(0, "")
	require.Equal(t, http.StatusForbidden, w.Code)
}
