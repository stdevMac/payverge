package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCreateShiftAssignedNotifiesStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{}, &database.Staff{}, &database.User{},
		&database.Position{}, &database.Schedule{}, &database.Shift{},
		&database.StaffNotification{},
	))
	database.SetTestDB(gormDB)
	server.InitializeRBAC(database.GetDBWrapper())

	h := NewScheduleHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", c.GetHeader("X-Test-Role"))
		if v, err := strconv.ParseUint(c.GetHeader("X-Test-Staff"), 10, 64); err == nil {
			c.Set("staff_id", uint(v))
		}
		c.Next()
	})
	r.POST("/b/:id/shifts", h.CreateShift)

	// Seed: business, a published schedule, an active staffer with a matching User.
	require.NoError(t, gormDB.Create(&database.Business{ID: 1, BusinessId: "biz-1", DefaultLanguage: "en"}).Error)
	require.NoError(t, gormDB.Create(&database.Position{ID: 5, BusinessID: 1, Name: "Server", IsActive: true}).Error)
	weekStart := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, gormDB.Create(&database.Schedule{ID: 10, BusinessID: 1, WeekStart: weekStart, Status: "published"}).Error)
	staff := database.Staff{BusinessID: 1, Email: "s@b1.test", Name: "S", Role: "server", IsActive: true, InvitedBy: "o"}
	require.NoError(t, gormDB.Create(&staff).Error)
	require.NoError(t, gormDB.Create(&database.User{Email: "s@b1.test", LanguageSelected: "en"}).Error)

	// Act: POST a shift assigned to the staffer into the published schedule.
	startsAt := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	endsAt := time.Date(2026, 7, 3, 17, 0, 0, 0, time.UTC)
	body := map[string]interface{}{
		"schedule_id": 10,
		"position_id": 5,
		"staff_id":    staff.ID,
		"starts_at":   startsAt.Format(time.RFC3339),
		"ends_at":     endsAt.Format(time.RFC3339),
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, "/b/1/shifts", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Role", "manager")
	req.Header.Set("X-Test-Staff", strconv.FormatUint(uint64(staff.ID), 10))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// Assert: the assigned staffer got a "shift.assigned" inbox row.
	dbw := database.GetDBWrapper()
	rows, err := dbw.ListStaffNotifications(1, staff.ID, 50, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "shift.assigned", rows[0].Kind)
	require.Equal(t, "/staff/home?tab=schedule", rows[0].URL)
	require.NotContains(t, rows[0].Title+rows[0].Body, "$", "money-free")
	_ = sqlDB.Close()
}
