package events

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// syncRecorder guards httptest.ResponseRecorder.Body with a mutex so a test
// goroutine can safely poll streamed SSE output while the handler goroutine is
// still writing frames. Production SSE writes from a single goroutine to a real
// ResponseWriter with no concurrent reader — only these streaming tests observe
// the buffer concurrently, so the synchronization lives here, not in prod code.
type syncRecorder struct {
	*httptest.ResponseRecorder
	mu sync.Mutex
}

func newSyncRecorder() *syncRecorder {
	return &syncRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (r *syncRecorder) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ResponseRecorder.Write(b)
}

func (r *syncRecorder) WriteString(s string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ResponseRecorder.WriteString(s)
}

// body returns the streamed output under lock.
func (r *syncRecorder) body() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ResponseRecorder.Body.String()
}

// sseAuthzDBSeq makes each in-memory DSN unique per setup call so `go test
// -count=N` (and any repeat run) gets a FRESH database instead of reusing the
// same shared-cache DB keyed by t.Name() — which reran into unique-email
// conflicts on the second iteration.
var sseAuthzDBSeq atomic.Int64

func setupSSEAuthzDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:sse-authz-%s-%d?mode=memory&cache=shared", t.Name(), sseAuthzDBSeq.Add(1))
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}))
	return gormDB
}

func TestStaffSSEAuthzStillValid_AccessShapeSingleNarrowSelect(t *testing.T) {
	db := setupSSEAuthzDB(t)
	biz := createSSETestBusiness(t)
	staff := &database.Staff{
		BusinessID: biz.ID, Email: "sse-shape@example.com", Name: "Shape",
		Role: database.StaffRoleServer, IsActive: true, InvitedBy: "0xowner", AuthzVersion: 4,
	}
	require.NoError(t, db.Create(staff).Error)

	queryCount := 0
	var lastSQL string
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("sse_authz_shape_"+t.Name(), func(tx *gorm.DB) {
		queryCount++
		if tx.Statement != nil && tx.Statement.SQL.String() != "" {
			lastSQL = tx.Statement.SQL.String()
		}
	}))

	ok, err := staffSSEAuthzStillValid(staff.ID, 4)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 1, queryCount, "heartbeat re-check must issue exactly one query")
	// Prefer columns projection over SELECT * — tolerate dialects that omit table name.
	assert.NotContains(t, strings.ToLower(lastSQL), "select *", "must not SELECT * on hot path")

	// Stale version → invalid.
	ok, err = staffSSEAuthzStillValid(staff.ID, 1)
	require.NoError(t, err)
	assert.False(t, ok)

	// Deactivated → invalid even with matching version.
	require.NoError(t, db.Model(staff).Update("is_active", false).Error)
	ok, err = staffSSEAuthzStillValid(staff.ID, 4)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSSEHandler_StaleAuthzOnConnect_TerminalSessionRevoked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSSEAuthzDB(t)
	biz := createSSETestBusiness(t)
	staff := &database.Staff{
		BusinessID: biz.ID, Email: "sse-stale@example.com", Name: "Stale SSE",
		Role: database.StaffRoleServer, IsActive: true, InvitedBy: "0xowner", AuthzVersion: 3,
	}
	require.NoError(t, db.Create(staff).Error)

	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(biz.ID))+"/events", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(biz.ID))}}
	c.Request = req
	c.Set("token_type", "staff")
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", biz.ID)
	c.Set("staff_authz_version", 1) // stale vs DB version 3

	SSEHandler(c)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, `"code":"session_revoked"`)
	assert.NotContains(t, body, "event: connected")
}

func TestSSEHandler_HeartbeatDetectsAuthzBump(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSSEAuthzDB(t)
	biz := createSSETestBusiness(t)
	staff := &database.Staff{
		BusinessID: biz.ID, Email: "sse-hb@example.com", Name: "HB SSE",
		Role: database.StaffRoleServer, IsActive: true, InvitedBy: "0xowner", AuthzVersion: 1,
	}
	require.NoError(t, db.Create(staff).Error)

	// Speed up heartbeat for the test.
	prev := sseHeartbeatInterval
	sseHeartbeatInterval = 20 * time.Millisecond
	t.Cleanup(func() { sseHeartbeatInterval = prev })

	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(biz.ID))+"/events", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)

	w := newSyncRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(biz.ID))}}
	c.Request = req
	c.Set("token_type", "staff")
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", biz.ID)
	c.Set("staff_authz_version", 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		SSEHandler(c)
	}()

	// Wait until stream is connected, then bump authz_version so the next
	// heartbeat terminates the stream.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(w.body(), "event: connected") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.Contains(t, w.body(), "event: connected")

	require.NoError(t, db.Model(staff).UpdateColumn("authz_version", 2).Error)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("SSE handler did not terminate after authz_version bump on heartbeat")
	}

	body := w.body()
	assert.Contains(t, body, `"code":"session_revoked"`)
}

func TestSSEHandler_ProcessLocalRevocationEventTerminatesStaffStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSSEAuthzDB(t)
	biz := createSSETestBusiness(t)
	staff := &database.Staff{
		BusinessID: biz.ID, Email: "sse-evt@example.com", Name: "Evt SSE",
		Role: database.StaffRoleServer, IsActive: true, InvitedBy: "0xowner", AuthzVersion: 1,
	}
	require.NoError(t, db.Create(staff).Error)

	// Slow heartbeat so the process-local event is what terminates the stream.
	prev := sseHeartbeatInterval
	sseHeartbeatInterval = 30 * time.Second
	t.Cleanup(func() { sseHeartbeatInterval = prev })

	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(biz.ID))+"/events", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)

	w := newSyncRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(biz.ID))}}
	c.Request = req
	c.Set("token_type", "staff")
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", biz.ID)
	c.Set("staff_authz_version", 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		SSEHandler(c)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(w.body(), "event: connected") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.Contains(t, w.body(), "event: connected")

	PublishStaffAccessRevoked(biz.ID, staff.ID)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("SSE handler did not terminate after process-local revocation event")
	}

	assert.Contains(t, w.body(), `"code":"session_revoked"`)
}

func TestSSEHandler_HeartbeatDBErrorDoesNotEmitSessionRevoked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSSEAuthzDB(t)
	biz := createSSETestBusiness(t)
	staff := &database.Staff{
		BusinessID: biz.ID, Email: "sse-blip@example.com", Name: "Blip SSE",
		Role: database.StaffRoleServer, IsActive: true, InvitedBy: "0xowner", AuthzVersion: 1,
	}
	require.NoError(t, db.Create(staff).Error)

	prev := sseHeartbeatInterval
	sseHeartbeatInterval = 20 * time.Millisecond
	t.Cleanup(func() { sseHeartbeatInterval = prev })

	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(biz.ID))+"/events", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)

	w := newSyncRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(biz.ID))}}
	c.Request = req
	c.Set("token_type", "staff")
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", biz.ID)
	c.Set("staff_authz_version", 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		SSEHandler(c)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(w.body(), "event: connected") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.Contains(t, w.body(), "event: connected")

	// Transient failure after the stream is up: the staff table disappears.
	require.NoError(t, db.Migrator().DropTable(&database.Staff{}))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("SSE handler did not close after repeated authz check failures")
	}

	body := w.body()
	assert.NotContains(t, body, "session_revoked")
	assert.Contains(t, body, "event: ping")
}

func TestSSEHandler_AuthzDBErrorOnConnectReturns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSSEAuthzDB(t)
	biz := createSSETestBusiness(t)
	staff := &database.Staff{
		BusinessID: biz.ID, Email: "sse-down@example.com", Name: "Down SSE",
		Role: database.StaffRoleServer, IsActive: true, InvitedBy: "0xowner", AuthzVersion: 1,
	}
	require.NoError(t, db.Create(staff).Error)
	require.NoError(t, db.Migrator().DropTable(&database.Staff{}))

	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(biz.ID))+"/events", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(biz.ID))}}
	c.Request = req
	c.Set("token_type", "staff")
	c.Set("staff_id", staff.ID)
	c.Set("staff_business_id", biz.ID)
	c.Set("staff_authz_version", 1)

	SSEHandler(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"code":"authz_check_unavailable"`)
	assert.NotContains(t, body, "session_revoked")
}

func TestSSEHandler_DeletedOrInactiveStaffOnConnect_SessionRevoked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, db *gorm.DB, staff *database.Staff)
	}{
		{
			name: "deleted",
			mutate: func(t *testing.T, db *gorm.DB, staff *database.Staff) {
				require.NoError(t, db.Delete(staff).Error)
			},
		},
		{
			name: "inactive",
			mutate: func(t *testing.T, db *gorm.DB, staff *database.Staff) {
				require.NoError(t, db.Model(staff).Update("is_active", false).Error)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupSSEAuthzDB(t)
			biz := createSSETestBusiness(t)
			staff := &database.Staff{
				BusinessID: biz.ID, Email: "sse-revoked-" + tc.name + "@example.com", Name: "Revoked SSE",
				Role: database.StaffRoleServer, IsActive: true, InvitedBy: "0xowner", AuthzVersion: 2,
			}
			require.NoError(t, db.Create(staff).Error)
			tc.mutate(t, db, staff)

			req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(biz.ID))+"/events", nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(biz.ID))}}
			c.Request = req
			c.Set("token_type", "staff")
			c.Set("staff_id", staff.ID)
			c.Set("staff_business_id", biz.ID)
			c.Set("staff_authz_version", 2)

			SSEHandler(c)

			assert.Equal(t, http.StatusOK, w.Code)
			body := w.Body.String()
			assert.Contains(t, body, "event: error")
			assert.Contains(t, body, `"code":"session_revoked"`)
			assert.NotContains(t, body, "event: connected")
		})
	}
}
