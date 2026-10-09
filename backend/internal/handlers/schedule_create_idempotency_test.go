package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestScheduleCreateShift_IdempotentReplay proves the L5-26 contract: with the
// shared Idempotency middleware in front of CreateShift, a retry under the same
// Idempotency-Key + body replays the cached 201 and does NOT insert a second
// shift row. A same-key / different-body retry returns 409.
func TestScheduleCreateShift_IdempotentReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.StaffPermissionDeny{},
		&database.Position{},
		&database.Schedule{},
		&database.Shift{},
		&middleware.IdempotencyKey{},
	))
	database.SetTestDB(gormDB)
	server.InitializeRBAC(database.GetDBWrapper())

	require.NoError(t, gormDB.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	pos := database.Position{BusinessID: 1, Name: "Server", IsActive: true}
	require.NoError(t, gormDB.Create(&pos).Error)
	st := database.Staff{BusinessID: 1, Email: "a@b1.test", Name: "A", Role: "server", IsActive: true}
	require.NoError(t, gormDB.Create(&st).Error)

	d := database.GetDBWrapper()
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, 1)
	require.NoError(t, err)

	h := NewScheduleHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", "manager")
		c.Set("staff_id", uint(1))
		c.Next()
	})
	// Same endpoint label production uses in main.go (decision #10).
	r.POST("/b/:id/shifts",
		middleware.Idempotency(gormDB, "POST /businesses/:id/shifts"),
		h.CreateShift,
	)

	body := map[string]interface{}{
		"schedule_id":   sched.ID,
		"position_id":   pos.ID,
		"staff_id":      st.ID,
		"starts_at":     weekStart.Add(9 * time.Hour).Format(time.RFC3339),
		"ends_at":       weekStart.Add(17 * time.Hour).Format(time.RFC3339),
		"break_minutes": 30,
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	post := func(key string, payload []byte) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodPost, "/b/1/shifts", bytes.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set(middleware.IdempotencyHeaderName, key)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	const key = "attempt-uuid:2026-06-29"
	w1 := post(key, raw)
	require.Equal(t, http.StatusCreated, w1.Code, w1.Body.String())
	var first struct {
		Data database.Shift `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &first))
	require.NotZero(t, first.Data.ID)

	// Lost-response retry: same key + same body → cached 201, no second row.
	w2 := post(key, raw)
	require.Equal(t, http.StatusCreated, w2.Code, w2.Body.String())
	var second struct {
		Data database.Shift `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &second))
	require.Equal(t, first.Data.ID, second.Data.ID, "replay must return the original shift id")

	var count int64
	require.NoError(t, gormDB.Model(&database.Shift{}).Count(&count).Error)
	require.Equal(t, int64(1), count, "retry must not double-create the shift")

	// Same key, different body → 409 Conflict (middleware contract).
	otherBody := map[string]interface{}{
		"schedule_id":   sched.ID,
		"position_id":   pos.ID,
		"staff_id":      st.ID,
		"starts_at":     weekStart.Add(10 * time.Hour).Format(time.RFC3339),
		"ends_at":       weekStart.Add(18 * time.Hour).Format(time.RFC3339),
		"break_minutes": 30,
	}
	otherRaw, err := json.Marshal(otherBody)
	require.NoError(t, err)
	w3 := post(key, otherRaw)
	require.Equal(t, http.StatusConflict, w3.Code, w3.Body.String())

	// Still only one row after the 409.
	require.NoError(t, gormDB.Model(&database.Shift{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
