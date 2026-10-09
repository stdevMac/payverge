package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newPositionHandlerTestDB opens an isolated in-memory SQLite DB, migrates the
// tables exercised by PositionHandler, registers it as the package DB, and
// returns the wrapper. The database package's own helper is unexported, so we
// build our own using the exported SetTestDB/GetDBWrapper seam (Group A/B).
func newPositionHandlerTestDB(t *testing.T) *database.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}, &database.Position{}, &database.StaffPosition{}))
	require.NoError(t, gormDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_staff_one_primary ON staff_positions (business_id, staff_id) WHERE is_primary").Error)
	// Mirrors the genesis index. `lower(trim(name))` is the one spelling both
	// Postgres and SQLite accept, so the harness constrains rows exactly the
	// way production does — partial on is_active, since retiring a position is
	// a soft delete that must free the name for reuse.
	require.NoError(t, gormDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_positions_business_name_active ON positions (business_id, lower(trim(name))) WHERE is_active").Error)
	// No exported getter for the previous global; SetTestDB without restore is
	// acceptable here (handlers tests run in their own package).
	database.SetTestDB(gormDB)
	return database.GetDBWrapper()
}

// TestPositionHandlerHTTP drives the gin handlers against an in-memory DB,
// asserting create→list, the owner pay-rate round-trip, the SetRate money
// guards (negative/over-ceiling rejected without mutating the persisted value),
// the money-isolation guarantee (pay rate never on the assignment payload),
// Unassign and Delete (soft-retire drops from List), and the 404 sentinel
// mapping across assign/rate/update/delete/unassign. The router sets an owner
// context so unassign of the rated link is allowed; route-level RBAC stays in
// main.go.
func TestPositionHandlerHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPositionHandlerTestDB(t)
	// Seed a staff row directly via the gorm handle.
	require.NoError(t, db.GetGorm().Create(&database.Staff{
		ID: 7, BusinessID: 42, Email: "s@x.io", Name: "S", Role: database.StaffRoleServer, InvitedBy: "o",
	}).Error)

	h := NewPositionHandler(db)
	server.InitializeRBAC(db)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", uint(1))
		c.Set("business_owner_user_id", uint(1))
		c.Next()
	})
	r.GET("/b/:id/positions", h.List)
	r.POST("/b/:id/positions", h.Create)
	r.GET("/b/:id/staff/:staffId/positions", h.ListForStaff)
	r.POST("/b/:id/staff/:staffId/positions", h.Assign)
	r.GET("/b/:id/staff/:staffId/positions/:positionId/rate", h.GetRate)
	r.PUT("/b/:id/staff/:staffId/positions/:positionId/rate", h.SetRate)
	r.PATCH("/b/:id/positions/:positionId", h.Update)
	r.DELETE("/b/:id/positions/:positionId", h.Delete)
	r.DELETE("/b/:id/staff/:staffId/positions/:positionId", h.Unassign)

	do := func(method, url string, body interface{}) *httptest.ResponseRecorder {
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

	// 1. Create → List
	w := do(http.MethodPost, "/b/42/positions", map[string]interface{}{"name": "Server"})
	require.Equal(t, http.StatusCreated, w.Code, "create body: %s", w.Body.String())
	var created struct {
		Data struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.Equal(t, "Server", created.Data.Name)
	require.NotZero(t, created.Data.ID)
	posID := created.Data.ID

	w = do(http.MethodGet, "/b/42/positions", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"name":"Server"`)

	// 2. MONEY ISOLATION — assign, then prove pay rate never rides the payload.
	w = do(http.MethodPost, "/b/42/staff/7/positions", map[string]interface{}{"position_id": posID, "is_primary": true})
	require.Equal(t, http.StatusOK, w.Code, "assign body: %s", w.Body.String())

	w = do(http.MethodGet, "/b/42/staff/7/positions", nil)
	require.Equal(t, http.StatusOK, w.Code)
	listBody := w.Body.String()
	require.Contains(t, listBody, fmt.Sprintf("\"position_id\":%d", posID))
	require.NotContains(t, listBody, "pay_rate", "pay rate must never appear on the assignment payload")
	require.NotContains(t, listBody, "pay_rate_cents", "pay rate cents must never appear on the assignment payload")

	// 3. Owner rate round-trip (dollars on the wire, cents in DB).
	w = do(http.MethodPut, fmt.Sprintf("/b/42/staff/7/positions/%d/rate", posID), map[string]interface{}{"pay_rate": 18.5})
	require.Equal(t, http.StatusOK, w.Code, "set rate body: %s", w.Body.String())
	var rateResp struct {
		Data struct {
			PayRate float64 `json:"pay_rate"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rateResp))
	require.Equal(t, 18.5, rateResp.Data.PayRate)

	w = do(http.MethodGet, fmt.Sprintf("/b/42/staff/7/positions/%d/rate", posID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rateResp))
	require.Equal(t, 18.5, rateResp.Data.PayRate)

	// 3b. SetRate money guards — a negative rate and an over-ceiling rate are
	// each a clean 400, and neither rejected write may mutate the persisted 18.5.
	w = do(http.MethodPut, fmt.Sprintf("/b/42/staff/7/positions/%d/rate", posID), map[string]interface{}{"pay_rate": -1})
	require.Equal(t, http.StatusBadRequest, w.Code, "negative rate body: %s", w.Body.String())
	w = do(http.MethodPut, fmt.Sprintf("/b/42/staff/7/positions/%d/rate", posID), map[string]interface{}{"pay_rate": 100001})
	require.Equal(t, http.StatusBadRequest, w.Code, "over-ceiling rate body: %s", w.Body.String())
	w = do(http.MethodGet, fmt.Sprintf("/b/42/staff/7/positions/%d/rate", posID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rateResp))
	require.Equal(t, 18.5, rateResp.Data.PayRate, "rejected rate writes must not change the persisted value")

	// 3c. PHR-1 required-field guard — a PUT /rate with a bare `{}` body must 400
	// (pay_rate is required), not silently overwrite the wage to $0. This runs
	// against the live staff-7→posID link, so it's the required-guard firing, not
	// a 404. The persisted 18.5 must also survive the rejected write.
	w = do(http.MethodPut, fmt.Sprintf("/b/42/staff/7/positions/%d/rate", posID), map[string]interface{}{})
	require.Equal(t, http.StatusBadRequest, w.Code, "empty rate body must be a 400 (pay_rate required): %s", w.Body.String())
	w = do(http.MethodGet, fmt.Sprintf("/b/42/staff/7/positions/%d/rate", posID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rateResp))
	require.Equal(t, 18.5, rateResp.Data.PayRate, "an empty-body PUT must not overwrite the persisted rate")

	// 3d. MONEY ISOLATION, post-rate — now that staff 7 carries a NON-ZERO rate
	// (1850 cents / $18.5) on its posID link, re-fetch the assignment payload and
	// prove the wage still never rides it. We scrub the volatile created_at/
	// updated_at fields first: their RFC3339 sub-second fraction can coincidentally
	// render "...:18.5xx" (second 18, ms 500-599) and false-positive the dollar
	// check — that's noise, not a wage leak (a real json:"-" regression would emit
	// the cents int 1850 under the Go name PayRateCents, never a dollar float). The
	// stable fields are what must stay wage-free.
	w = do(http.MethodGet, "/b/42/staff/7/positions", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var postRate struct {
		Data []map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &postRate))
	for _, row := range postRate.Data {
		delete(row, "created_at")
		delete(row, "updated_at")
	}
	scrubbed, err := json.Marshal(postRate.Data)
	require.NoError(t, err)
	postRateBody := strings.ToLower(string(scrubbed))
	require.Contains(t, postRateBody, fmt.Sprintf("\"position_id\":%d", posID), "the rate-bearing link must be in the payload")
	require.NotContains(t, postRateBody, "payrate", "no pay-rate field (incl. the Go-name PayRateCents) may ride the assignment payload")
	require.NotContains(t, postRateBody, "18.5", "the dollar wage value must never appear on the assignment payload")
	require.NotContains(t, postRateBody, "1850", "the cents wage value must never appear on the assignment payload")

	// 4a. 404 mapping — assign to a non-existent staff id (ErrStaffNotFound → 404).
	w = do(http.MethodPost, "/b/42/staff/9999/positions", map[string]interface{}{"position_id": posID})
	require.Equal(t, http.StatusNotFound, w.Code, "missing staff body: %s", w.Body.String())

	// 4b. position_id required — 400 when omitted/zero.
	w = do(http.MethodPost, "/b/42/staff/7/positions", map[string]interface{}{"position_id": 0})
	require.Equal(t, http.StatusBadRequest, w.Code, "zero position body: %s", w.Body.String())

	// 4c. non-existent position — ErrPositionNotFound → 404.
	w = do(http.MethodPost, "/b/42/staff/7/positions", map[string]interface{}{"position_id": 999999})
	require.Equal(t, http.StatusNotFound, w.Code, "missing position body: %s", w.Body.String())

	// 5. Create with an invalid color is a clean 400 (not a Postgres 500).
	w = do(http.MethodPost, "/b/42/positions", map[string]interface{}{"name": "X", "color_hex": "red"})
	require.Equal(t, http.StatusBadRequest, w.Code, "invalid color body: %s", w.Body.String())

	// 6. Rate 404 arms — GET/PUT a rate for a position the staff is NOT assigned
	// to → ErrStaffPositionNotFound → 404 (not a silent 0 or a 500).
	w = do(http.MethodGet, "/b/42/staff/7/positions/999999/rate", nil)
	require.Equal(t, http.StatusNotFound, w.Code, "get unassigned rate body: %s", w.Body.String())
	w = do(http.MethodPut, "/b/42/staff/7/positions/999999/rate", map[string]interface{}{"pay_rate": 10})
	require.Equal(t, http.StatusNotFound, w.Code, "put unassigned rate body: %s", w.Body.String())

	// 7. Update 404 arm — PATCH a non-existent position → 404. The body carries a
	// real field so it clears the no-op guard and reaches the tenant lookup.
	w = do(http.MethodPatch, "/b/42/positions/999999", map[string]interface{}{"name": "Renamed"})
	require.Equal(t, http.StatusNotFound, w.Code, "patch missing position body: %s", w.Body.String())

	// 8. Unassign — the live link from step 2 deletes 200; the repeat (no link
	// left) → ErrStaffPositionNotFound → 404.
	w = do(http.MethodDelete, fmt.Sprintf("/b/42/staff/7/positions/%d", posID), nil)
	require.Equal(t, http.StatusOK, w.Code, "unassign body: %s", w.Body.String())
	w = do(http.MethodDelete, fmt.Sprintf("/b/42/staff/7/positions/%d", posID), nil)
	require.Equal(t, http.StatusNotFound, w.Code, "repeat unassign body: %s", w.Body.String())

	// 9. Delete (soft-retire) — retiring the position 200s, then it drops out of
	// List; deleting a non-existent position → 404.
	w = do(http.MethodDelete, fmt.Sprintf("/b/42/positions/%d", posID), nil)
	require.Equal(t, http.StatusOK, w.Code, "delete body: %s", w.Body.String())
	w = do(http.MethodGet, "/b/42/positions", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), fmt.Sprintf("\"id\":%d", posID), "retired position must not appear in List")
	w = do(http.MethodDelete, "/b/42/positions/999999", nil)
	require.Equal(t, http.StatusNotFound, w.Code, "delete missing position body: %s", w.Body.String())
}

// TestPositionHandlerPartialUpdate proves the I1 fix: a PATCH carrying only one
// field must not blank the others (the old full-replace handler set name/
// department to "" when they were absent), and blanking the name is rejected.
func TestPositionHandlerPartialUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPositionHandlerTestDB(t)
	h := NewPositionHandler(db)
	r := gin.New()
	r.POST("/b/:id/positions", h.Create)
	r.PATCH("/b/:id/positions/:positionId", h.Update)
	r.GET("/b/:id/positions", h.List)

	// create with a real name + department
	cw := httptest.NewRecorder()
	creq, _ := http.NewRequest(http.MethodPost, "/b/42/positions", bytes.NewBufferString(`{"name":"Server","department":"FOH"}`))
	creq.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(cw, creq)
	require.Equal(t, http.StatusCreated, cw.Code, "create body: %s", cw.Body.String())
	var created struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(cw.Body.Bytes(), &created))
	posID := created.Data.ID
	require.NotZero(t, posID)

	// PATCH only color_hex must NOT blank name/department
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("/b/42/positions/%d", posID), bytes.NewBufferString(`{"color_hex":"#ffffff"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// list still shows name "Server" and department "FOH"
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/b/42/positions", nil)
	r.ServeHTTP(w2, req2)
	body := w2.Body.String()
	require.Contains(t, body, "\"name\":\"Server\"")
	require.Contains(t, body, "\"department\":\"FOH\"")
	require.Contains(t, body, "#ffffff")

	// PATCH blanking the name is rejected
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("/b/42/positions/%d", posID), bytes.NewBufferString(`{"name":"  "}`))
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)
	require.Equal(t, http.StatusBadRequest, w3.Code)
}

// TestPositionCreateDuplicateName409 (L5-18): duplicate position names must
// 409 with the house conflict code (CONFLICT, not VALIDATION_INVALID_INPUT),
// the match must be case-insensitive, and a soft-retired position's name must
// be reusable (ListPositions is active-only, so retire frees the name).
func TestPositionCreateDuplicateName409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPositionHandlerTestDB(t)
	h := NewPositionHandler(db)
	r := gin.New()
	r.POST("/b/:id/positions", h.Create)
	r.DELETE("/b/:id/positions/:positionId", h.Delete)

	do := func(method, url, body string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(method, url, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// Seed the original.
	w := do(http.MethodPost, "/b/42/positions", `{"name":"Bartender"}`)
	require.Equal(t, http.StatusCreated, w.Code, "create body: %s", w.Body.String())
	var created struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	posID := created.Data.ID
	require.NotZero(t, posID)

	// Exact duplicate → 409 + house conflict code.
	w = do(http.MethodPost, "/b/42/positions", `{"name":"Bartender"}`)
	require.Equal(t, http.StatusConflict, w.Code, "duplicate body: %s", w.Body.String())
	var errResp struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	require.Equal(t, "CONFLICT", errResp.Code, "409 must carry ErrCodeConflict, body: %s", w.Body.String())

	// Case-insensitive (and surrounding-whitespace) duplicate → same 409/CONFLICT.
	w = do(http.MethodPost, "/b/42/positions", `{"name":"  bartender "}`)
	require.Equal(t, http.StatusConflict, w.Code, "case-insensitive duplicate body: %s", w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	require.Equal(t, "CONFLICT", errResp.Code)

	// Retire the original; the name becomes reusable (active-only uniqueness).
	w = do(http.MethodDelete, fmt.Sprintf("/b/42/positions/%d", posID), "")
	require.Equal(t, http.StatusOK, w.Code, "retire body: %s", w.Body.String())

	w = do(http.MethodPost, "/b/42/positions", `{"name":"Bartender"}`)
	require.Equal(t, http.StatusCreated, w.Code, "recreate after retire body: %s", w.Body.String())
}
