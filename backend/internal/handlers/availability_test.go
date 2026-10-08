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
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// availTestActor identifies the simulated caller a request runs as.
type availTestActor struct {
	role    string
	staffID uint
}

// newAvailabilityTestServer wires an AvailabilityHandler against an isolated
// in-memory SQLite DB, initializes the global RBAC middleware (so the handler's
// row-scoping permission resolution works), and returns a do() helper. Like the
// schedule harness, routes carry NO RBAC middleware (enforced in main.go); a
// per-request middleware hydrates the staff gin-context from the actor so
// ResolveContextPermissions classifies approver vs plain staff exactly as in
// production.
func newAvailabilityTestServer(t *testing.T) (do func(a availTestActor, method, url string, body interface{}) *httptest.ResponseRecorder, cleanup func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}, &database.StaffPermissionDeny{}, &database.StaffAvailability{}, &database.TimeOffRequest{}, &database.RBACAuditLog{}))
	database.SetTestDB(gormDB)
	server.InitializeRBAC(database.GetDBWrapper())

	h := NewAvailabilityHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", c.GetHeader("X-Test-Role"))
		if v, err := strconv.ParseUint(c.GetHeader("X-Test-Staff"), 10, 64); err == nil {
			c.Set("staff_id", uint(v))
		}
		c.Next()
	})
	r.GET("/b/:id/me/availability", h.GetMyAvailability)
	r.PUT("/b/:id/me/availability", h.PutMyAvailability)
	r.GET("/b/:id/team-availability", h.GetTeamAvailability)
	r.POST("/b/:id/me/time-off", h.CreateMyTimeOff)
	r.GET("/b/:id/time-off", h.ListTimeOff)
	r.POST("/b/:id/time-off/:reqId/decision", h.DecideTimeOff)

	do = func(a availTestActor, method, url string, body interface{}) *httptest.ResponseRecorder {
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

// TestAvailabilitySelfRoutesRejectZeroStaff proves the "my" self routes reject a
// caller with no staff identity (staff_id==0, e.g. an owner authenticating by
// wallet) with 403, rather than reading/writing rows orphaned to staff 0. The
// manager actor route (DecideTimeOff) intentionally still allows owner=0 and is
// deliberately not covered here.
func TestAvailabilitySelfRoutesRejectZeroStaff(t *testing.T) {
	do, cleanup := newAvailabilityTestServer(t)
	defer cleanup()
	seedAvailHandlerBiz(t)
	owner := availTestActor{role: "manager", staffID: 0} // owner: no staff row
	cases := []struct{ method, url string }{
		{http.MethodGet, "/b/1/me/availability"},
		{http.MethodPut, "/b/1/me/availability"},
		{http.MethodPost, "/b/1/me/time-off"},
	}
	for _, tc := range cases {
		w := do(owner, tc.method, tc.url, nil)
		require.Equal(t, http.StatusForbidden, w.Code, "%s %s must 403 without a staff identity", tc.method, tc.url)
	}
}

// seedAvailHandlerBiz inserts business 1 with a manager + two servers and
// returns their staff ids.
func seedAvailHandlerBiz(t *testing.T) (mgrID, serverAID, serverBID uint) {
	t.Helper()
	require.NoError(t, database.GetDB().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	mgr := database.Staff{BusinessID: 1, Email: "m@b1.test", Name: "Mgr", Role: "manager", IsActive: true}
	require.NoError(t, database.GetDB().Create(&mgr).Error)
	a := database.Staff{BusinessID: 1, Email: "a@b1.test", Name: "A", Role: "server", IsActive: true}
	require.NoError(t, database.GetDB().Create(&a).Error)
	b := database.Staff{BusinessID: 1, Email: "b@b1.test", Name: "B", Role: "server", IsActive: true}
	require.NoError(t, database.GetDB().Create(&b).Error)
	return mgr.ID, a.ID, b.ID
}

func TestAvailabilityPutGetRoundTripAndScope(t *testing.T) {
	do, cleanup := newAvailabilityTestServer(t)
	defer cleanup()
	_, serverAID, serverBID := seedAvailHandlerBiz(t)
	actorA := availTestActor{role: "server", staffID: serverAID}
	actorB := availTestActor{role: "server", staffID: serverBID}

	// A PUTs two windows.
	w := do(actorA, http.MethodPut, "/b/1/me/availability", map[string]interface{}{
		"windows": []map[string]interface{}{
			{"weekday": 1, "start_min": 540, "end_min": 720, "kind": "preferred"},
			{"weekday": 1, "start_min": 840, "end_min": 960, "kind": "unavailable"},
		},
	})
	require.Equal(t, http.StatusOK, w.Code)
	var putResp struct {
		Data []database.StaffAvailability `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &putResp))
	require.Len(t, putResp.Data, 2)
	for _, win := range putResp.Data {
		require.Equal(t, serverAID, win.StaffID, "windows must be stamped with the caller's own staff_id")
	}

	// A GETs them back.
	w = do(actorA, http.MethodGet, "/b/1/me/availability", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var getResp struct {
		Data []database.StaffAvailability `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &getResp))
	require.Len(t, getResp.Data, 2)

	// B's availability is independent (and empty).
	w = do(actorB, http.MethodGet, "/b/1/me/availability", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &getResp))
	require.Empty(t, getResp.Data)

	// Validation: weekday out of range -> 400.
	w = do(actorA, http.MethodPut, "/b/1/me/availability", map[string]interface{}{
		"windows": []map[string]interface{}{{"weekday": 9, "start_min": 540, "end_min": 720, "kind": "preferred"}},
	})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Validation: bad kind -> 400.
	w = do(actorA, http.MethodPut, "/b/1/me/availability", map[string]interface{}{
		"windows": []map[string]interface{}{{"weekday": 1, "start_min": 540, "end_min": 720, "kind": "maybe"}},
	})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Validation: start >= end -> 400.
	w = do(actorA, http.MethodPut, "/b/1/me/availability", map[string]interface{}{
		"windows": []map[string]interface{}{{"weekday": 1, "start_min": 720, "end_min": 540, "kind": "preferred"}},
	})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// Empty windows clears availability.
	w = do(actorA, http.MethodPut, "/b/1/me/availability", map[string]interface{}{"windows": []map[string]interface{}{}})
	require.Equal(t, http.StatusOK, w.Code)
	w = do(actorA, http.MethodGet, "/b/1/me/availability", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &getResp))
	require.Empty(t, getResp.Data)
}

// TestTeamAvailabilityManagerOverlay proves the manager overlay endpoint returns
// every staff member's windows grouped by staff_id under the
// data.staff_availabilities envelope, so the schedule builder can render the
// availability overlay in one call (no per-staff N+1 from the client).
func TestTeamAvailabilityManagerOverlay(t *testing.T) {
	do, cleanup := newAvailabilityTestServer(t)
	defer cleanup()
	mgrID, serverAID, serverBID := seedAvailHandlerBiz(t)
	actorMgr := availTestActor{role: "manager", staffID: mgrID}
	actorA := availTestActor{role: "server", staffID: serverAID}
	actorB := availTestActor{role: "server", staffID: serverBID}

	// A and B each set their own availability.
	w := do(actorA, http.MethodPut, "/b/1/me/availability", map[string]interface{}{
		"windows": []map[string]interface{}{
			{"weekday": 1, "start_min": 540, "end_min": 720, "kind": "preferred"},
			{"weekday": 1, "start_min": 840, "end_min": 960, "kind": "unavailable"},
		},
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = do(actorB, http.MethodPut, "/b/1/me/availability", map[string]interface{}{
		"windows": []map[string]interface{}{
			{"weekday": 3, "start_min": 600, "end_min": 1020, "kind": "preferred"},
		},
	})
	require.Equal(t, http.StatusOK, w.Code)

	// Manager reads the whole team's availability in one call.
	w = do(actorMgr, http.MethodGet, "/b/1/team-availability", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct {
			StaffAvailabilities map[string][]database.StaffAvailability `json:"staff_availabilities"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data.StaffAvailabilities, 2, "one entry per staff with windows")
	require.Len(t, resp.Data.StaffAvailabilities[strconv.FormatUint(uint64(serverAID), 10)], 2)
	require.Len(t, resp.Data.StaffAvailabilities[strconv.FormatUint(uint64(serverBID), 10)], 1)

	// A manager with no staff row (owner authenticating by wallet) can still read
	// the overlay — the read is tenant-scoped, not staff-scoped.
	w = do(availTestActor{role: "manager", staffID: 0}, http.MethodGet, "/b/1/team-availability", nil)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestTimeOffListRowScopingByRole(t *testing.T) {
	do, cleanup := newAvailabilityTestServer(t)
	defer cleanup()
	mgrID, serverAID, serverBID := seedAvailHandlerBiz(t)
	actorMgr := availTestActor{role: "manager", staffID: mgrID}
	actorA := availTestActor{role: "server", staffID: serverAID}
	actorB := availTestActor{role: "server", staffID: serverBID}

	starts := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	ends := starts.AddDate(0, 0, 2)

	// A and B each file a request.
	w := do(actorA, http.MethodPost, "/b/1/me/time-off", map[string]interface{}{
		"starts_at": starts.Format(time.RFC3339), "ends_at": ends.Format(time.RFC3339), "reason": "a",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	w = do(actorB, http.MethodPost, "/b/1/me/time-off", map[string]interface{}{
		"starts_at": starts.Format(time.RFC3339), "ends_at": ends.Format(time.RFC3339), "reason": "b",
	})
	require.Equal(t, http.StatusCreated, w.Code)

	// A (plain staff) sees only A's own request.
	w = do(actorA, http.MethodGet, "/b/1/time-off", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Data []database.TimeOffRequest `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Len(t, listResp.Data, 1, "a plain staff member sees only their own requests")
	require.Equal(t, serverAID, listResp.Data[0].StaffID)

	// Manager (approver) sees the whole queue.
	w = do(actorMgr, http.MethodGet, "/b/1/time-off", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Len(t, listResp.Data, 2, "an approver sees all requests")
}

func TestTimeOffDecisionHappyAndIllegal(t *testing.T) {
	do, cleanup := newAvailabilityTestServer(t)
	defer cleanup()
	mgrID, serverAID, _ := seedAvailHandlerBiz(t)
	actorMgr := availTestActor{role: "manager", staffID: mgrID}
	actorA := availTestActor{role: "server", staffID: serverAID}

	starts := time.Now().UTC().Add(24 * time.Hour)
	ends := starts.AddDate(0, 0, 2)
	w := do(actorA, http.MethodPost, "/b/1/me/time-off", map[string]interface{}{
		"starts_at": starts.Format(time.RFC3339), "ends_at": ends.Format(time.RFC3339), "reason": "a",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var createResp struct {
		Data database.TimeOffRequest `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	reqID := createResp.Data.ID

	// Approve happy path.
	w = do(actorMgr, http.MethodPost, fmt.Sprintf("/b/1/time-off/%d/decision", reqID), map[string]interface{}{
		"approve": true, "reason": "ok",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var decideResp struct {
		Data database.TimeOffRequest `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decideResp))
	require.Equal(t, database.TimeOffStatusApproved, decideResp.Data.Status)

	// Audit row written.
	var audits int64
	require.NoError(t, database.GetDB().Model(&database.RBACAuditLog{}).
		Where("business_id = ? AND action = ?", 1, database.RBACActionTimeOffDecided).Count(&audits).Error)
	require.Equal(t, int64(1), audits)

	// Illegal transition: deciding the now-approved request -> 409.
	w = do(actorMgr, http.MethodPost, fmt.Sprintf("/b/1/time-off/%d/decision", reqID), map[string]interface{}{
		"approve": false, "reason": "again",
	})
	require.Equal(t, http.StatusConflict, w.Code)

	// Unknown request -> 404.
	w = do(actorMgr, http.MethodPost, "/b/1/time-off/999999/decision", map[string]interface{}{"approve": true})
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestTimeOffDecisionRejectsExpired(t *testing.T) {
	do, cleanup := newAvailabilityTestServer(t)
	defer cleanup()
	mgrID, serverAID, _ := seedAvailHandlerBiz(t)
	actorMgr := availTestActor{role: "manager", staffID: mgrID}
	actorA := availTestActor{role: "server", staffID: serverAID}

	starts := time.Now().UTC().Add(-48 * time.Hour)
	ends := time.Now().UTC().Add(-time.Hour)
	w := do(actorA, http.MethodPost, "/b/1/me/time-off", map[string]interface{}{
		"starts_at": starts.Format(time.RFC3339), "ends_at": ends.Format(time.RFC3339), "reason": "past",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var createResp struct {
		Data database.TimeOffRequest `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))

	w = do(actorMgr, http.MethodPost, fmt.Sprintf("/b/1/time-off/%d/decision", createResp.Data.ID), map[string]interface{}{
		"approve": true, "reason": "late",
	})
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), "time_off_expired")
}

func TestTimeOffSSEEmitted(t *testing.T) {
	do, cleanup := newAvailabilityTestServer(t)
	defer cleanup()
	mgrID, serverAID, _ := seedAvailHandlerBiz(t)
	actorMgr := availTestActor{role: "manager", staffID: mgrID}
	actorA := availTestActor{role: "server", staffID: serverAID}

	// Subscribe BEFORE the requests so we capture the live fan-out.
	ch, _, cancelSub := events.GetHub().SubscribeWithReplayTopic(1, 0, "timeoff.requested", "timeoff.decided")
	defer cancelSub()

	starts := time.Now().UTC().Add(24 * time.Hour)
	ends := starts.AddDate(0, 0, 2)
	w := do(actorA, http.MethodPost, "/b/1/me/time-off", map[string]interface{}{
		"starts_at": starts.Format(time.RFC3339), "ends_at": ends.Format(time.RFC3339), "reason": "a",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var createResp struct {
		Data database.TimeOffRequest `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))

	w = do(actorMgr, http.MethodPost, fmt.Sprintf("/b/1/time-off/%d/decision", createResp.Data.ID), map[string]interface{}{
		"approve": true,
	})
	require.Equal(t, http.StatusOK, w.Code)

	requested, decided := 0, 0
	deadline := time.After(500 * time.Millisecond)
	for done := false; !done; {
		select {
		case ev := <-ch:
			switch ev.Type {
			case "timeoff.requested":
				requested++
			case "timeoff.decided":
				decided++
			}
		case <-deadline:
			done = true
		}
		if requested >= 1 && decided >= 1 {
			break
		}
	}
	require.Equal(t, 1, requested, "exactly one timeoff.requested on create")
	require.Equal(t, 1, decided, "exactly one timeoff.decided on decision")
}
