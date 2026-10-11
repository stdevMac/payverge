package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
)

// newPositionNameRouter wires the three routes that can write a position name
// and returns a request helper, so each uniqueness case reads as one flow.
func newPositionNameRouter(t *testing.T, db *database.DB) func(method, url string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	h := NewPositionHandler(db)
	r := gin.New()
	r.GET("/b/:id/positions", h.List)
	r.POST("/b/:id/positions", h.Create)
	r.PATCH("/b/:id/positions/:positionId", h.Update)
	r.DELETE("/b/:id/positions/:positionId", h.Delete)

	return func(method, url string, body interface{}) *httptest.ResponseRecorder {
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
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
}

func createPositionForTest(t *testing.T, do func(string, string, interface{}) *httptest.ResponseRecorder, business, name string) uint {
	t.Helper()
	w := do(http.MethodPost, "/b/"+business+"/positions", map[string]interface{}{"name": name})
	require.Equal(t, http.StatusCreated, w.Code, "create %q body: %s", name, w.Body.String())
	var created struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotZero(t, created.Data.ID)
	return created.Data.ID
}

// L5-18. Create rejects a duplicate name, but Update never did — renaming
// "Host" to "Server" walked straight past the guard and produced exactly the
// duplicate the create path refuses. Rosters then show two identical roles and
// an operator has no way to tell which one the schedule points at.
func TestPositionUpdateRejectsARenameOntoAnExistingName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPositionHandlerTestDB(t)
	do := newPositionNameRouter(t, db)

	createPositionForTest(t, do, "42", "Server")
	hostID := createPositionForTest(t, do, "42", "Host")

	w := do(http.MethodPatch, fmt.Sprintf("/b/42/positions/%d", hostID), map[string]interface{}{"name": "server"})
	require.Equal(t, http.StatusConflict, w.Code, "rename body: %s", w.Body.String())

	w = do(http.MethodGet, "/b/42/positions", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"name":"Host"`, "the rejected rename must not have been applied")
}

// Re-casing or padding a position's own name is not a duplicate — the row
// collides only with itself.
func TestPositionUpdateAllowsRecasingItsOwnName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPositionHandlerTestDB(t)
	do := newPositionNameRouter(t, db)

	serverID := createPositionForTest(t, do, "42", "Server")

	w := do(http.MethodPatch, fmt.Sprintf("/b/42/positions/%d", serverID), map[string]interface{}{"name": "SERVER"})
	require.Equal(t, http.StatusOK, w.Code, "recase body: %s", w.Body.String())

	w = do(http.MethodGet, "/b/42/positions", nil)
	require.Contains(t, w.Body.String(), `"name":"SERVER"`)
}

// The name is unique per business, not globally: two restaurants both having a
// "Server" is the normal case.
func TestPositionNamesAreUniquePerBusinessNotGlobally(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPositionHandlerTestDB(t)
	do := newPositionNameRouter(t, db)

	createPositionForTest(t, do, "42", "Server")
	createPositionForTest(t, do, "43", "Server")
}

// Retiring is a soft delete (is_active=false) that keeps schedule history
// intact, so the name must come back up for reuse. A blanket unique index
// would break this working flow — the constraint has to be partial.
func TestPositionCreateReusesTheNameOfARetiredPosition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPositionHandlerTestDB(t)
	do := newPositionNameRouter(t, db)

	serverID := createPositionForTest(t, do, "42", "Server")
	w := do(http.MethodDelete, fmt.Sprintf("/b/42/positions/%d", serverID), nil)
	require.Equal(t, http.StatusOK, w.Code, "retire body: %s", w.Body.String())

	createPositionForTest(t, do, "42", "Server")
}

// The app-layer guard reads then writes, so two simultaneous creates can both
// pass the read. The database index is what actually makes it impossible.
func TestActivePositionNamesAreUniqueInTheDatabase(t *testing.T) {
	db := newPositionHandlerTestDB(t)
	gormDB := db.GetGorm()

	require.NoError(t, gormDB.Create(&database.Position{BusinessID: 42, Name: "Server", IsActive: true}).Error)

	err := gormDB.Create(&database.Position{BusinessID: 42, Name: " server ", IsActive: true}).Error
	require.Error(t, err, "a second active position with the same trimmed, lower-cased name must be rejected")
	require.True(t, isDuplicatePositionName(err), "expected a unique-violation, got: %v", err)

	// Retire-then-rename via the same map-update DeletePosition uses. Building
	// the row with IsActive:false would not prove anything: GORM skips a
	// zero-valued field that carries a `default` tag, so the insert would land
	// active and collide for the wrong reason.
	retired := database.Position{BusinessID: 42, Name: "Server (old)", IsActive: true}
	require.NoError(t, gormDB.Create(&retired).Error)
	require.NoError(t, gormDB.Model(&database.Position{}).Where("id = ?", retired.ID).
		Updates(map[string]interface{}{"is_active": false, "name": "Server"}).Error,
		"a retired row may share the name — the index is partial on is_active")
	require.NoError(t, gormDB.Create(&database.Position{BusinessID: 43, Name: "Server", IsActive: true}).Error,
		"another business may use the same name")
}

// The read-then-write race lands as a driver error, and an operator who lost
// that race deserves the same 409 the guard returns — not a 500 that reads
// like the platform broke.
func TestIsDuplicatePositionNameRecognizesBothDialects(t *testing.T) {
	require.True(t, isDuplicatePositionName(errors.New(
		`ERROR: duplicate key value violates unique constraint "idx_positions_business_name_active" (SQLSTATE 23505)`)))
	require.True(t, isDuplicatePositionName(errors.New(
		"UNIQUE constraint failed: index 'idx_positions_business_name_active'")))
	require.False(t, isDuplicatePositionName(errors.New("connection refused")))
	require.False(t, isDuplicatePositionName(nil))
}
