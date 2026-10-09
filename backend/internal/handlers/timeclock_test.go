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

// tcTestActor identifies the simulated caller a request runs as.
type tcTestActor struct {
	role    string
	staffID uint
}

// newTimeclockTestServer wires a TimeclockHandler against an isolated in-memory
// SQLite DB and returns a do() helper. Mirrors newAvailabilityTestServer: routes
// carry NO RBAC middleware (enforced in main.go); a per-request middleware
// hydrates the staff gin-context from the actor.
func newTimeclockTestServer(t *testing.T) (do func(a tcTestActor, method, url string, body interface{}) *httptest.ResponseRecorder, cleanup func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}, &database.StaffPosition{}, &database.TimeEntry{}, &database.RBACAuditLog{}))
	database.SetTestDB(gormDB)
	server.InitializeRBAC(database.GetDBWrapper())

	h := NewTimeclockHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", c.GetHeader("X-Test-Role"))
		if v, err := strconv.ParseUint(c.GetHeader("X-Test-Staff"), 10, 64); err == nil {
			c.Set("staff_id", uint(v))
		}
		c.Next()
	})
	r.POST("/b/:id/me/clock-in", h.ClockIn)
	r.POST("/b/:id/me/clock-out", h.ClockOut)
	r.POST("/b/:id/me/break", h.Break)
	r.GET("/b/:id/me/timesheet", h.GetMyTimesheet)
	r.GET("/b/:id/timesheets", h.ListTimesheets)
	r.POST("/b/:id/timesheets/:entryId/approve", h.ApproveTimesheet)
	r.POST("/b/:id/timesheets/:entryId/reject", h.RejectTimesheet)
	r.PATCH("/b/:id/timesheets/:entryId", h.EditTimesheet)
	r.POST("/b/:id/time-entries", h.CreateManualEntry)

	do = func(a tcTestActor, method, url string, body interface{}) *httptest.ResponseRecorder {
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

// TestTimeclockSelfRoutesRejectZeroStaff proves the punch + own-timesheet self
// routes reject a caller with no staff identity (staff_id==0, e.g. an owner
// authenticating by wallet) with 403, rather than opening punches / reading a
// timesheet orphaned to staff 0. The manager actor routes (approve / manual
// entry) intentionally still allow owner=0 and are deliberately not covered here.
func TestTimeclockSelfRoutesRejectZeroStaff(t *testing.T) {
	do, cleanup := newTimeclockTestServer(t)
	defer cleanup()
	seedTimeclockHandlerBiz(t)
	owner := tcTestActor{role: "manager", staffID: 0} // owner: no staff row
	cases := []struct{ method, url string }{
		{http.MethodPost, "/b/1/me/clock-in"},
		{http.MethodPost, "/b/1/me/clock-out"},
		{http.MethodPost, "/b/1/me/break"},
		{http.MethodGet, "/b/1/me/timesheet"},
	}
	for _, tc := range cases {
		w := do(owner, tc.method, tc.url, nil)
		require.Equal(t, http.StatusForbidden, w.Code, "%s %s must 403 without a staff identity", tc.method, tc.url)
	}
}

// seedTimeclockHandlerBiz inserts business 1 with a manager + a server.
func seedTimeclockHandlerBiz(t *testing.T) (mgrID, serverID uint) {
	t.Helper()
	require.NoError(t, database.GetDB().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	mgr := database.Staff{BusinessID: 1, Email: "m@b1.test", Name: "Mgr", Role: "manager", IsActive: true}
	require.NoError(t, database.GetDB().Create(&mgr).Error)
	s := database.Staff{BusinessID: 1, Email: "s@b1.test", Name: "Sam", Role: "server", IsActive: true}
	require.NoError(t, database.GetDB().Create(&s).Error)
	return mgr.ID, s.ID
}

func TestClockInOutBreakLifecycle(t *testing.T) {
	do, cleanup := newTimeclockTestServer(t)
	defer cleanup()
	_, serverID := seedTimeclockHandlerBiz(t)
	actor := tcTestActor{role: "server", staffID: serverID}

	// Clock in.
	w := do(actor, http.MethodPost, "/b/1/me/clock-in", map[string]interface{}{})
	require.Equal(t, http.StatusCreated, w.Code)
	var resp struct {
		Data timeEntryDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, database.TimeEntryStatusOpen, resp.Data.Status)
	require.Equal(t, serverID, resp.Data.StaffID)

	// Double clock-in => 409.
	w = do(actor, http.MethodPost, "/b/1/me/clock-in", map[string]interface{}{})
	require.Equal(t, http.StatusConflict, w.Code)

	// Clock out => pending_review.
	w = do(actor, http.MethodPost, "/b/1/me/clock-out", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, database.TimeEntryStatusPendingReview, resp.Data.Status)

	// Second clock-out with nothing open => 409.
	w = do(actor, http.MethodPost, "/b/1/me/clock-out", nil)
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestMyTimesheetMoneyIsolation(t *testing.T) {
	do, cleanup := newTimeclockTestServer(t)
	defer cleanup()
	mgrID, serverID := seedTimeclockHandlerBiz(t)
	actor := tcTestActor{role: "server", staffID: serverID}

	// Manager seeds an approved entry for the server with a real pay rate on the
	// position link — none of that may surface on the staff timesheet response.
	pos := database.StaffPosition{BusinessID: 1, StaffID: serverID, PositionID: 1, PayRateCents: 2500, IsPrimary: true}
	require.NoError(t, database.GetDB().Create(&pos).Error)

	in := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	out := in.Add(8 * time.Hour)
	manual := map[string]interface{}{
		"staff_id":      serverID,
		"clock_in_at":   in.Format(time.RFC3339),
		"clock_out_at":  out.Format(time.RFC3339),
		"break_minutes": 30,
		"note":          "shift",
	}
	w := do(tcTestActor{role: "manager", staffID: mgrID}, http.MethodPost, "/b/1/time-entries", manual)
	require.Equal(t, http.StatusCreated, w.Code)

	// Staff reads OWN timesheet.
	w = do(actor, http.MethodGet, "/b/1/me/timesheet", nil)
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Money isolation: the staff-facing timesheet must never carry a dollar sign,
	// a pay rate, or any cents field.
	require.NotContains(t, body, "$", "staff timesheet must not contain a dollar sign")
	require.NotContains(t, body, "rate", "staff timesheet must not contain a pay rate")
	require.NotContains(t, body, "pay", "staff timesheet must not contain a pay field")
	require.NotContains(t, body, "cents", "staff timesheet must not contain a cents field")

	// It DOES carry worked hours/minutes.
	var resp struct {
		Data []timeEntryDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	require.Equal(t, 450, resp.Data[0].WorkedMinutes) // 8h - 30m
	require.InDelta(t, 7.5, resp.Data[0].WorkedHours, 0.001)
}

func TestTimesheetReviewAndApprove(t *testing.T) {
	do, cleanup := newTimeclockTestServer(t)
	defer cleanup()
	mgrID, serverID := seedTimeclockHandlerBiz(t)
	mgr := tcTestActor{role: "manager", staffID: mgrID}
	server := tcTestActor{role: "server", staffID: serverID}

	// Server clocks in + out to produce a pending_review entry.
	require.Equal(t, http.StatusCreated, do(server, http.MethodPost, "/b/1/me/clock-in", map[string]interface{}{}).Code)
	w := do(server, http.MethodPost, "/b/1/me/clock-out", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var closeResp struct {
		Data timeEntryDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &closeResp))
	entryID := closeResp.Data.ID

	// Manager review queue shows the pending entry.
	w = do(mgr, http.MethodGet, "/b/1/timesheets", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Data []timeEntryDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Len(t, listResp.Data, 1)
	require.Equal(t, entryID, listResp.Data[0].ID)

	// Approve it.
	w = do(mgr, http.MethodPost, fmt.Sprintf("/b/1/timesheets/%d/approve", entryID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var approveResp struct {
		Data timeEntryDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &approveResp))
	require.Equal(t, database.TimeEntryStatusApproved, approveResp.Data.Status)

	// Re-approve => 409 illegal transition.
	w = do(mgr, http.MethodPost, fmt.Sprintf("/b/1/timesheets/%d/approve", entryID), nil)
	require.Equal(t, http.StatusConflict, w.Code)

	// Unknown entry => 404.
	w = do(mgr, http.MethodPost, "/b/1/timesheets/999999/approve", nil)
	require.Equal(t, http.StatusNotFound, w.Code)
}

// TestRejectAndEditTimesheetHandlers drives the correction flow end-to-end: a
// manager rejects a pending entry (with reason), then edits it back into review.
func TestRejectAndEditTimesheetHandlers(t *testing.T) {
	do, cleanup := newTimeclockTestServer(t)
	defer cleanup()
	mgrID, serverID := seedTimeclockHandlerBiz(t)
	staff := tcTestActor{role: "server", staffID: serverID}
	mgr := tcTestActor{role: "manager", staffID: mgrID}

	// The staffer clocks in + out → a pending_review entry.
	do(staff, http.MethodPost, "/b/1/me/clock-in", nil)
	w := do(staff, http.MethodPost, "/b/1/me/clock-out", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var closed struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &closed))
	entryID := closed.Data.ID

	// Manager rejects with a reason.
	w = do(mgr, http.MethodPost, fmt.Sprintf("/b/1/timesheets/%d/reject", entryID), map[string]interface{}{"reason": "wrong out time"})
	require.Equal(t, http.StatusOK, w.Code)
	var rejected struct {
		Data struct {
			Status string `json:"status"`
			Note   string `json:"note"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rejected))
	require.Equal(t, database.TimeEntryStatusRejected, rejected.Data.Status)
	require.Contains(t, rejected.Data.Note, "wrong out time")

	// Approving a rejected entry is a 409.
	w = do(mgr, http.MethodPost, fmt.Sprintf("/b/1/timesheets/%d/approve", entryID), nil)
	require.Equal(t, http.StatusConflict, w.Code)

	// Manager edits the rejected entry → back to pending_review, then approvable.
	in := time.Now().UTC().Add(-8 * time.Hour)
	out := in.Add(7 * time.Hour)
	w = do(mgr, http.MethodPatch, fmt.Sprintf("/b/1/timesheets/%d", entryID), map[string]interface{}{
		"clock_in_at": in, "clock_out_at": out, "break_minutes": 30, "note": "fixed",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var edited struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &edited))
	require.Equal(t, database.TimeEntryStatusPendingReview, edited.Data.Status)

	// Now it approves cleanly.
	w = do(mgr, http.MethodPost, fmt.Sprintf("/b/1/timesheets/%d/approve", entryID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	// Editing an approved entry is a 409 (locked).
	w = do(mgr, http.MethodPatch, fmt.Sprintf("/b/1/timesheets/%d", entryID), map[string]interface{}{
		"clock_in_at": in, "clock_out_at": out,
	})
	require.Equal(t, http.StatusConflict, w.Code)
}

// TestListTimesheetsPaginationBEFirst proves the BE-first contract: no
// offset/limit params → the legacy bare-array shape; with them → a paged envelope
// carrying an honest total.
func TestListTimesheetsPaginationBEFirst(t *testing.T) {
	do, cleanup := newTimeclockTestServer(t)
	defer cleanup()
	mgrID, serverID := seedTimeclockHandlerBiz(t)
	mgr := tcTestActor{role: "manager", staffID: mgrID}
	d := database.GetDBWrapper()

	// Seed relative to now: the paged path applies parseBusinessDayRange's default
	// "last 8 weeks" window, so a fixed calendar date silently ages out of it
	// (this test went red ~8 weeks after its hard-coded July date). Days -20..-6
	// sit well inside the window in any business timezone.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	base := today.AddDate(0, 0, -20).Add(9 * time.Hour)
	for i := 0; i < 15; i++ {
		clockIn := base.AddDate(0, 0, i)
		clockOut := clockIn.Add(time.Hour)
		_, err := d.CreateManualEntry(1, serverID, nil, clockIn, &clockOut, 0, "x")
		require.NoError(t, err)
	}

	// Legacy shape: no pagination params → bare array, no total field.
	w := do(mgr, http.MethodGet, "/b/1/timesheets", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var legacy struct {
		Data  []timeEntryDTO `json:"data"`
		Total *int64         `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &legacy))
	require.Len(t, legacy.Data, 15)
	require.Nil(t, legacy.Total, "legacy path omits total")

	// Paged shape: ?limit=5 → 5 rows + total=15.
	w = do(mgr, http.MethodGet, "/b/1/timesheets?limit=5", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var paged struct {
		Data   []timeEntryDTO `json:"data"`
		Total  int64          `json:"total"`
		Offset int            `json:"offset"`
		Limit  int            `json:"limit"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &paged))
	require.Len(t, paged.Data, 5)
	require.Equal(t, int64(15), paged.Total)
	require.Equal(t, 5, paged.Limit)

	// Second page.
	w = do(mgr, http.MethodGet, "/b/1/timesheets?limit=5&offset=5", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &paged))
	require.Len(t, paged.Data, 5)
	require.Equal(t, 5, paged.Offset)

	// Explicit from/to narrows the window: the first three seeded days only.
	from := base.Format("2006-01-02")
	to := base.AddDate(0, 0, 2).Format("2006-01-02")
	w = do(mgr, http.MethodGet, "/b/1/timesheets?limit=50&from="+from+"&to="+to, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &paged))
	require.Len(t, paged.Data, 3)
	require.Equal(t, int64(3), paged.Total)
}
