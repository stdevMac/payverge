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
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// scheduleTestActor identifies the simulated caller a test request runs as.
type scheduleTestActor struct {
	role    string
	staffID uint
}

// newScheduleTestServer wires a ScheduleHandler against an isolated in-memory
// SQLite DB, initializes the global RBAC middleware (so the handler's
// row-scoping permission resolution works), and returns a do(actor,method,path,
// body) helper plus a seeded fixture and cleanup. A per-request middleware
// hydrates the staff gin-context (token_type/staff_role/staff_id) from the actor
// so ResolveContextPermissions classifies manager vs plain staff exactly as in
// production — routes themselves carry NO RBAC middleware (enforced in main.go).
func newScheduleTestServer(t *testing.T) (do func(a scheduleTestActor, method, url string, body interface{}) *httptest.ResponseRecorder, cleanup func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}, &database.StaffPermissionDeny{}, &database.Position{}, &database.Schedule{}, &database.Shift{}))
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
	r.GET("/b/:id/schedule", h.Get)
	r.POST("/b/:id/schedule", h.CreateDraft)
	r.POST("/b/:id/shifts", h.CreateShift)
	r.PATCH("/b/:id/shifts/:shiftId", h.UpdateShift)
	r.DELETE("/b/:id/shifts/:shiftId", h.DeleteShift)
	r.POST("/b/:id/schedule/:scheduleId/publish", h.Publish)
	r.POST("/b/:id/schedule/copy-week", h.CopyWeek)

	do = func(a scheduleTestActor, method, url string, body interface{}) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			b, err := json.Marshal(body)
			require.NoError(t, err)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, err := http.NewRequest(method, url, rdr)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Role", a.role)
		req.Header.Set("X-Test-Staff", strconv.FormatUint(uint64(a.staffID), 10))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	cleanup = func() { _ = sqlDB.Close() }
	return do, cleanup
}

type scheduleGetResp struct {
	Success bool `json:"success"`
	Data    struct {
		Schedule *database.Schedule `json:"schedule"`
		Shifts   []database.Shift   `json:"shifts"`
	} `json:"data"`
}

func TestScheduleGetRowScoping(t *testing.T) {
	do, cleanup := newScheduleTestServer(t)
	defer cleanup()
	d := database.GetDBWrapper()

	require.NoError(t, database.GetDB().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	pos := database.Position{BusinessID: 1, Name: "Server", IsActive: true}
	require.NoError(t, database.GetDB().Create(&pos).Error)
	mgr := database.Staff{BusinessID: 1, Email: "m@b1.test", Name: "Mgr", Role: "manager", IsActive: true}
	require.NoError(t, database.GetDB().Create(&mgr).Error)
	serverA := database.Staff{BusinessID: 1, Email: "a@b1.test", Name: "A", Role: "server", IsActive: true}
	require.NoError(t, database.GetDB().Create(&serverA).Error)
	serverB := database.Staff{BusinessID: 1, Email: "b@b1.test", Name: "B", Role: "server", IsActive: true}
	require.NoError(t, database.GetDB().Create(&serverB).Error)

	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, mgr.ID)
	require.NoError(t, err)
	aID, bID := serverA.ID, serverB.ID
	require.NoError(t, d.CreateShift(&database.Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: pos.ID, StaffID: &aID, CreatedByStaffID: mgr.ID, StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}))
	require.NoError(t, d.CreateShift(&database.Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: pos.ID, StaffID: &bID, CreatedByStaffID: mgr.ID, StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}))
	require.NoError(t, d.CreateShift(&database.Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: pos.ID, CreatedByStaffID: mgr.ID, StartsAt: weekStart.Add(18 * time.Hour), EndsAt: weekStart.Add(22 * time.Hour)})) // open

	week := "2026-06-29"

	// While DRAFT: manager sees the draft + all 3 shifts; a plain staff sees nothing.
	w := do(scheduleTestActor{role: "manager", staffID: mgr.ID}, http.MethodGet, "/b/1/schedule?week="+week, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var mgrResp scheduleGetResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &mgrResp))
	require.NotNil(t, mgrResp.Data.Schedule)
	require.Equal(t, database.ScheduleStatusDraft, mgrResp.Data.Schedule.Status)
	require.Len(t, mgrResp.Data.Shifts, 3, "manager sees all shifts incl. draft")

	w = do(scheduleTestActor{role: "server", staffID: serverA.ID}, http.MethodGet, "/b/1/schedule?week="+week, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var staffDraft scheduleGetResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &staffDraft))
	require.Nil(t, staffDraft.Data.Schedule, "staff must not see an unpublished schedule")
	require.Empty(t, staffDraft.Data.Shifts)

	// Publish, then re-check.
	_, _, err = d.PublishSchedule(1, sched.ID, mgr.ID)
	require.NoError(t, err)

	w = do(scheduleTestActor{role: "server", staffID: serverA.ID}, http.MethodGet, "/b/1/schedule?week="+week, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var staffResp scheduleGetResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &staffResp))
	require.NotNil(t, staffResp.Data.Schedule)
	require.Equal(t, database.ScheduleStatusPublished, staffResp.Data.Schedule.Status)
	require.Len(t, staffResp.Data.Shifts, 2, "serverA sees own + open, not serverB's")
	for _, s := range staffResp.Data.Shifts {
		if s.StaffID != nil {
			require.Equal(t, serverA.ID, *s.StaffID)
		}
	}

	// Manager still sees all 3 after publish.
	w = do(scheduleTestActor{role: "manager", staffID: mgr.ID}, http.MethodGet, "/b/1/schedule?week="+week, nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &mgrResp))
	require.Len(t, mgrResp.Data.Shifts, 3)

	// Missing week param -> 400.
	w = do(scheduleTestActor{role: "manager", staffID: mgr.ID}, http.MethodGet, "/b/1/schedule", nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// seedScheduleHandlerBiz inserts business 1 with a position + an active server,
// and returns the position id and the server's staff id.
func seedScheduleHandlerBiz(t *testing.T) (posID, staffID uint) {
	t.Helper()
	require.NoError(t, database.GetDB().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	pos := database.Position{BusinessID: 1, Name: "Server", IsActive: true}
	require.NoError(t, database.GetDB().Create(&pos).Error)
	st := database.Staff{BusinessID: 1, Email: "a@b1.test", Name: "A", Role: "server", IsActive: true}
	require.NoError(t, database.GetDB().Create(&st).Error)
	return pos.ID, st.ID
}

func TestScheduleCreateDraftHandler(t *testing.T) {
	do, cleanup := newScheduleTestServer(t)
	defer cleanup()
	seedScheduleHandlerBiz(t)
	mgr := scheduleTestActor{role: "manager", staffID: 1}

	// Create a draft for the week.
	w := do(mgr, http.MethodPost, "/b/1/schedule", map[string]interface{}{"week": "2026-06-29"})
	require.Equal(t, http.StatusCreated, w.Code)
	var resp struct {
		Data database.Schedule `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, database.ScheduleStatusDraft, resp.Data.Status)
	firstID := resp.Data.ID

	// Idempotent: same week returns the same draft, not a duplicate.
	w = do(mgr, http.MethodPost, "/b/1/schedule", map[string]interface{}{"week": "2026-06-29"})
	require.Equal(t, http.StatusCreated, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, firstID, resp.Data.ID)

	// Missing week -> 400.
	w = do(mgr, http.MethodPost, "/b/1/schedule", map[string]interface{}{})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScheduleCreateShiftHandler(t *testing.T) {
	do, cleanup := newScheduleTestServer(t)
	defer cleanup()
	posID, staffID := seedScheduleHandlerBiz(t)
	mgr := scheduleTestActor{role: "manager", staffID: 1}

	d := database.GetDBWrapper()
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, 1)
	require.NoError(t, err)

	start := weekStart.Add(9 * time.Hour)
	end := weekStart.Add(17 * time.Hour)

	// Happy path: assigned shift.
	w := do(mgr, http.MethodPost, "/b/1/shifts", map[string]interface{}{
		"schedule_id": sched.ID, "position_id": posID, "staff_id": staffID,
		"starts_at": start.Format(time.RFC3339), "ends_at": end.Format(time.RFC3339), "break_minutes": 30,
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var resp struct {
		Data database.Shift `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotZero(t, resp.Data.ID)
	require.NotNil(t, resp.Data.StaffID)
	require.Equal(t, staffID, *resp.Data.StaffID)
	require.Equal(t, 30, resp.Data.BreakMinutes)

	// Open shift (no staff_id).
	w = do(mgr, http.MethodPost, "/b/1/shifts", map[string]interface{}{
		"schedule_id": sched.ID, "position_id": posID,
		"starts_at": start.Format(time.RFC3339), "ends_at": end.Format(time.RFC3339),
	})
	require.Equal(t, http.StatusCreated, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Nil(t, resp.Data.StaffID)

	// Validation: end <= start -> 400.
	w = do(mgr, http.MethodPost, "/b/1/shifts", map[string]interface{}{
		"schedule_id": sched.ID, "position_id": posID,
		"starts_at": end.Format(time.RFC3339), "ends_at": start.Format(time.RFC3339),
	})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Validation: missing position_id -> 400.
	w = do(mgr, http.MethodPost, "/b/1/shifts", map[string]interface{}{
		"schedule_id": sched.ID,
		"starts_at":   start.Format(time.RFC3339), "ends_at": end.Format(time.RFC3339),
	})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Cross-tenant schedule -> 404 (schedule_id not in business 1).
	require.NoError(t, database.GetDB().Create(&database.Business{ID: 2, BusinessId: "biz-2"}).Error)
	other := database.Schedule{BusinessID: 2, WeekStart: weekStart, Status: database.ScheduleStatusDraft}
	require.NoError(t, database.GetDB().Create(&other).Error)
	w = do(mgr, http.MethodPost, "/b/1/shifts", map[string]interface{}{
		"schedule_id": other.ID, "position_id": posID,
		"starts_at": start.Format(time.RFC3339), "ends_at": end.Format(time.RFC3339),
	})
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestScheduleUpdateAndDeleteShiftHandler(t *testing.T) {
	do, cleanup := newScheduleTestServer(t)
	defer cleanup()
	posID, staffID := seedScheduleHandlerBiz(t)
	mgr := scheduleTestActor{role: "manager", staffID: 1}

	d := database.GetDBWrapper()
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, 1)
	require.NoError(t, err)
	sh := &database.Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, CreatedByStaffID: 1,
		StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}
	require.NoError(t, d.CreateShift(sh))

	// PATCH: assign staff + bump break.
	w := do(mgr, http.MethodPatch, fmt.Sprintf("/b/1/shifts/%d", sh.ID), map[string]interface{}{
		"staff_id": staffID, "break_minutes": 45,
	})
	require.Equal(t, http.StatusOK, w.Code)
	var reloaded database.Shift
	require.NoError(t, database.GetDB().First(&reloaded, sh.ID).Error)
	require.NotNil(t, reloaded.StaffID)
	require.Equal(t, staffID, *reloaded.StaffID)
	require.Equal(t, 45, reloaded.BreakMinutes)

	// PATCH: explicit unassign via staff_id=0 sentinel.
	w = do(mgr, http.MethodPatch, fmt.Sprintf("/b/1/shifts/%d", sh.ID), map[string]interface{}{"staff_id": 0})
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, database.GetDB().First(&reloaded, sh.ID).Error)
	require.Nil(t, reloaded.StaffID)

	// PATCH: invalid range (end before existing start) -> 400.
	w = do(mgr, http.MethodPatch, fmt.Sprintf("/b/1/shifts/%d", sh.ID), map[string]interface{}{
		"ends_at": weekStart.Add(1 * time.Hour).Format(time.RFC3339),
	})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// PATCH: empty body -> 400.
	w = do(mgr, http.MethodPatch, fmt.Sprintf("/b/1/shifts/%d", sh.ID), map[string]interface{}{})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// PATCH cross-tenant -> 404.
	w = do(mgr, http.MethodPatch, fmt.Sprintf("/b/2/shifts/%d", sh.ID), map[string]interface{}{"break_minutes": 5})
	require.Equal(t, http.StatusNotFound, w.Code)

	// DELETE happy path.
	w = do(mgr, http.MethodDelete, fmt.Sprintf("/b/1/shifts/%d", sh.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var count int64
	require.NoError(t, database.GetDB().Model(&database.Shift{}).Where("id = ?", sh.ID).Count(&count).Error)
	require.Equal(t, int64(0), count)

	// DELETE missing -> 404.
	w = do(mgr, http.MethodDelete, fmt.Sprintf("/b/1/shifts/%d", sh.ID), nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSchedulePublishFanOut(t *testing.T) {
	do, cleanup := newScheduleTestServer(t)
	defer cleanup()
	// The publish handler enqueues an operator roll-up notification (gated on a
	// connected Telegram config); migrate the outbox + plugin tables so the
	// enqueue persists rather than erroring on a missing table.
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.PluginNotificationDelivery{}, &database.Plugin{}, &database.BusinessPlugin{}))
	mgr := scheduleTestActor{role: "manager", staffID: 1}

	require.NoError(t, database.GetDB().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	// Connected Telegram with the opt-in schedule_published event enabled, so the
	// gated enqueue fires.
	tgPlugin := database.Plugin{Name: "telegram", DisplayName: "Telegram", IsActive: true}
	require.NoError(t, database.GetDB().Create(&tgPlugin).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: 1, PluginID: tgPlugin.ID, IsEnabled: true,
		Config: `{"is_connected":true,"chat_id":"55","notifications":{"schedule_published":true}}`,
	}).Error)
	services.ResetTelegramNotificationEligibilityCache()
	pos := database.Position{BusinessID: 1, Name: "Server", IsActive: true}
	require.NoError(t, database.GetDB().Create(&pos).Error)
	a := database.Staff{BusinessID: 1, Email: "a@b1.test", Name: "A", Role: "server", IsActive: true}
	require.NoError(t, database.GetDB().Create(&a).Error)
	b := database.Staff{BusinessID: 1, Email: "b@b1.test", Name: "B", Role: "server", IsActive: true}
	require.NoError(t, database.GetDB().Create(&b).Error)

	d := database.GetDBWrapper()
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, 1)
	require.NoError(t, err)
	aID, bID := a.ID, b.ID
	require.NoError(t, d.CreateShift(&database.Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: pos.ID, StaffID: &aID, CreatedByStaffID: 1, StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}))
	require.NoError(t, d.CreateShift(&database.Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: pos.ID, StaffID: &bID, CreatedByStaffID: 1, StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}))
	require.NoError(t, d.CreateShift(&database.Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: pos.ID, CreatedByStaffID: 1, StartsAt: weekStart.Add(18 * time.Hour), EndsAt: weekStart.Add(22 * time.Hour)})) // open

	// Subscribe to the hub BEFORE publishing so we capture the fan-out.
	ch, _, cancelSub := events.GetHub().SubscribeWithReplayTopic(1, 0, "schedule.published", "shift.assigned")
	defer cancelSub()

	w := do(mgr, http.MethodPost, fmt.Sprintf("/b/1/schedule/%d/publish", sched.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	// Drain the captured events.
	published, assigned := 0, 0
	deadline := time.After(500 * time.Millisecond)
	for done := false; !done; {
		select {
		case ev := <-ch:
			switch ev.Type {
			case "schedule.published":
				published++
			case "shift.assigned":
				assigned++
			}
		case <-deadline:
			done = true
		}
		if published >= 1 && assigned >= 2 {
			break
		}
	}
	require.Equal(t, 1, published, "exactly one schedule.published")
	require.Equal(t, 2, assigned, "one shift.assigned per assigned staff (open shift excluded)")

	// Exactly ONE operator roll-up notification is enqueued per publish (not one
	// per staffer — the assigned staff are reminded on their own channel).
	var deliveries []database.PluginNotificationDelivery
	require.NoError(t, database.GetDB().Where("event_type = ?", "schedule.published").Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	require.Equal(t, fmt.Sprintf("schedule:%d:published", sched.ID), deliveries[0].EventID)
	require.EqualValues(t, 2, deliveries[0].Payload["shift_count"])
	require.EqualValues(t, 2, deliveries[0].Payload["staff_count"])

	// Idempotent re-publish -> 409.
	w = do(mgr, http.MethodPost, fmt.Sprintf("/b/1/schedule/%d/publish", sched.ID), nil)
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestScheduleCreateShiftIntoPublishedScheduleVisibleAndSSE(t *testing.T) {
	do, cleanup := newScheduleTestServer(t)
	defer cleanup()
	posID, staffID := seedScheduleHandlerBiz(t)
	mgr := scheduleTestActor{role: "manager", staffID: 1}

	d := database.GetDBWrapper()
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, 1)
	require.NoError(t, err)
	// Publish the (empty) schedule so subsequent shifts land into a published week.
	_, _, err = d.PublishSchedule(1, sched.ID, 1)
	require.NoError(t, err)

	// Subscribe BEFORE the POST so we capture the live shift.assigned fan-out.
	ch, _, cancelSub := events.GetHub().SubscribeWithReplayTopic(1, 0, "shift.assigned")
	defer cancelSub()

	start := weekStart.Add(9 * time.Hour)
	end := weekStart.Add(17 * time.Hour)
	w := do(mgr, http.MethodPost, "/b/1/shifts", map[string]interface{}{
		"schedule_id": sched.ID, "position_id": posID, "staff_id": staffID,
		"starts_at": start.Format(time.RFC3339), "ends_at": end.Format(time.RFC3339),
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var resp struct {
		Data database.Shift `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Data.Published, "a shift added to a published week must be born published")
	require.Equal(t, database.ShiftStatusFilled, resp.Data.Status, "assigned -> filled")

	// SSE shift.assigned emitted for the affected staff.
	assigned := 0
	deadline := time.After(500 * time.Millisecond)
	for done := false; !done; {
		select {
		case ev := <-ch:
			if ev.Type == "shift.assigned" {
				assigned++
			}
		case <-deadline:
			done = true
		}
		if assigned >= 1 {
			break
		}
	}
	require.Equal(t, 1, assigned, "shift.assigned must fire for a live add to a published week")

	// And it is now staff-visible (GetPublishedScheduleForStaff filters published=true).
	w = do(scheduleTestActor{role: "server", staffID: staffID}, http.MethodGet, "/b/1/schedule?week=2026-06-29", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var got scheduleGetResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Data.Shifts, 1, "the live-added shift must be visible to the assigned staff")
}

func TestScheduleWeekNormalizationFloorsToUTCDate(t *testing.T) {
	do, cleanup := newScheduleTestServer(t)
	defer cleanup()
	seedScheduleHandlerBiz(t)
	mgr := scheduleTestActor{role: "manager", staffID: 1}

	// Date form creates the draft.
	w := do(mgr, http.MethodPost, "/b/1/schedule", map[string]interface{}{"week": "2026-06-29"})
	require.Equal(t, http.StatusCreated, w.Code)
	var first struct {
		Data database.Schedule `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &first))

	// A non-midnight RFC3339 on the SAME UTC date must resolve to the SAME row,
	// not key a near-duplicate (the unique index would otherwise be bypassed).
	w = do(mgr, http.MethodPost, "/b/1/schedule", map[string]interface{}{"week": "2026-06-29T15:30:00Z"})
	require.Equal(t, http.StatusCreated, w.Code)
	var second struct {
		Data database.Schedule `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &second))
	require.Equal(t, first.Data.ID, second.Data.ID, "non-midnight RFC3339 must floor to the same week row")

	// And a non-midnight GET finds the same schedule.
	w = do(mgr, http.MethodGet, "/b/1/schedule?week=2026-06-29T08:00:00Z", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var got scheduleGetResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.NotNil(t, got.Data.Schedule)
	require.Equal(t, first.Data.ID, got.Data.Schedule.ID)

	// Exactly one schedule row exists for the week (no near-duplicate).
	var count int64
	require.NoError(t, database.GetDB().Model(&database.Schedule{}).Where("business_id = ?", 1).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

// TestScheduleCopyWeekHandler drives the transactional copy-week endpoint: a
// manager copies a source week into a fresh destination week in ONE call and gets
// {created} back; the destination week then holds the copied shifts.
func TestScheduleCopyWeekHandler(t *testing.T) {
	do, cleanup := newScheduleTestServer(t)
	defer cleanup()
	posID, staffID := seedScheduleHandlerBiz(t)
	mgr := scheduleTestActor{role: "manager", staffID: staffID}
	d := database.GetDBWrapper()

	fromWeek := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	src, err := d.GetOrCreateDraftSchedule(1, fromWeek, staffID)
	require.NoError(t, err)
	require.NoError(t, d.CreateShift(&database.Shift{BusinessID: 1, ScheduleID: src.ID, PositionID: posID,
		StaffID: &staffID, CreatedByStaffID: staffID,
		StartsAt: fromWeek.Add(9 * time.Hour), EndsAt: fromWeek.Add(17 * time.Hour)}))

	w := do(mgr, http.MethodPost, "/b/1/schedule/copy-week", map[string]interface{}{
		"from_week": "2026-06-29", "to_week": "2026-07-06",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data database.CopyWeekResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Data.Created)

	// The destination week now has the copied shift.
	_, destShifts, err := d.GetScheduleForWeek(1, time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, destShifts, 1)

	// Same source==dest week is a 400.
	w = do(mgr, http.MethodPost, "/b/1/schedule/copy-week", map[string]interface{}{
		"from_week": "2026-06-29", "to_week": "2026-06-29",
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
}
