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

// newScheduleSettingsTestServer wires a ScheduleSettingsHandler against an
// isolated in-memory SQLite DB (Business + BusinessScheduleSettings migrated),
// registers the GET/PUT routes with the `:id` param name parseBusinessID reads,
// and returns a do(method,path,body) JSON helper, a doRaw(method,path,rawBody)
// helper that sends bytes verbatim (for malformed-JSON cases), plus a cleanup.
// Routes carry NO middleware — RBAC (schedule:read/write) is enforced in
// main.go, not here.
func newScheduleSettingsTestServer(t *testing.T) (*ScheduleSettingsHandler, func(method, url string, body interface{}) *httptest.ResponseRecorder, func(method, url, rawBody string) *httptest.ResponseRecorder, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.BusinessScheduleSettings{}))
	database.SetTestDB(gormDB)

	h := NewScheduleSettingsHandler(database.GetDBWrapper())
	r := gin.New()
	r.GET("/b/:id/schedule/settings", h.Get)
	r.PUT("/b/:id/schedule/settings", h.Put)

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

	doRaw := func(method, url, rawBody string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(method, url, bytes.NewReader([]byte(rawBody)))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	cleanup := func() { _ = sqlDB.Close() }
	return h, do, doRaw, cleanup
}

func TestScheduleSettingsHandlerHTTP(t *testing.T) {
	_, do, doRaw, cleanup := newScheduleSettingsTestServer(t)
	defer cleanup()

	// 1. GET lazily creates defaults.
	w := do(http.MethodGet, "/b/42/schedule/settings", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var got struct {
		Data struct {
			BusinessID         uint `json:"business_id"`
			WeekStartDay       int  `json:"week_start_day"`
			PostedLeadDays     int  `json:"posted_lead_days"`
			MinorCutoffMin     *int `json:"minor_cutoff_min"`
			QuietHoursStartMin *int `json:"quiet_hours_start_min"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, uint(42), got.Data.BusinessID)
	require.Equal(t, 1, got.Data.WeekStartDay)
	require.Equal(t, 7, got.Data.PostedLeadDays)
	require.Nil(t, got.Data.MinorCutoffMin)

	// 2. PUT partial update persists and round-trips.
	w = do(http.MethodPut, "/b/42/schedule/settings", map[string]interface{}{
		"week_start_day":   0,
		"posted_lead_days": 14,
		"minor_cutoff_min": 1320,
	})
	require.Equal(t, http.StatusOK, w.Code)
	w = do(http.MethodGet, "/b/42/schedule/settings", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, 0, got.Data.WeekStartDay)
	require.Equal(t, 14, got.Data.PostedLeadDays)
	require.NotNil(t, got.Data.MinorCutoffMin)
	require.Equal(t, 1320, *got.Data.MinorCutoffMin)

	// 3. Validation: week_start_day out of 0..6 → 400.
	w = do(http.MethodPut, "/b/42/schedule/settings", map[string]interface{}{"week_start_day": 9})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 4. Validation: negative minutes → 400.
	w = do(http.MethodPut, "/b/42/schedule/settings", map[string]interface{}{"default_shift_minutes": -5})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 5. Validation: minor_cutoff_min out of 0..1439 → 400.
	w = do(http.MethodPut, "/b/42/schedule/settings", map[string]interface{}{"minor_cutoff_min": 5000})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 6. A rejected write must not have mutated the persisted value.
	w = do(http.MethodGet, "/b/42/schedule/settings", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, 0, got.Data.WeekStartDay) // still the value from step 2

	// 7. Clear round-trip: minor_cutoff_min was set to 1320 in step 2. The -1
	//    sentinel must clear it back to NULL end to end — this proves the
	//    service stages a nil map value that GORM writes as SQL NULL (a map
	//    Updates writes nil as NULL, unlike struct Updates which skip zeroes).
	w = do(http.MethodPut, "/b/42/schedule/settings", map[string]interface{}{"minor_cutoff_min": -1})
	require.Equal(t, http.StatusOK, w.Code)
	w = do(http.MethodGet, "/b/42/schedule/settings", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Nil(t, got.Data.MinorCutoffMin) // disabled again (SQL NULL), proven via reload

	// 8. Clear round-trip for quiet_hours_start_min: set 600, confirm, clear, confirm null.
	w = do(http.MethodPut, "/b/42/schedule/settings", map[string]interface{}{"quiet_hours_start_min": 600})
	require.Equal(t, http.StatusOK, w.Code)
	w = do(http.MethodGet, "/b/42/schedule/settings", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.NotNil(t, got.Data.QuietHoursStartMin)
	require.Equal(t, 600, *got.Data.QuietHoursStartMin)
	w = do(http.MethodPut, "/b/42/schedule/settings", map[string]interface{}{"quiet_hours_start_min": -1})
	require.Equal(t, http.StatusOK, w.Code)
	w = do(http.MethodGet, "/b/42/schedule/settings", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Nil(t, got.Data.QuietHoursStartMin) // cleared back to NULL

	// 9. Mixed valid+invalid body must reject the whole request before any DB
	//    write — the valid field must NOT partially apply. posted_lead_days is
	//    14 from step 2; this body pairs a valid posted_lead_days with an
	//    out-of-range minor_cutoff_min.
	w = do(http.MethodPut, "/b/42/schedule/settings", map[string]interface{}{
		"posted_lead_days": 21,
		"minor_cutoff_min": 5000,
	})
	require.Equal(t, http.StatusBadRequest, w.Code)
	w = do(http.MethodGet, "/b/42/schedule/settings", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, 14, got.Data.PostedLeadDays) // unchanged: no partial apply

	// 10. Empty body is a 200 no-op and leaves state untouched.
	w = do(http.MethodPut, "/b/42/schedule/settings", map[string]interface{}{})
	require.Equal(t, http.StatusOK, w.Code)
	w = do(http.MethodGet, "/b/42/schedule/settings", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, 0, got.Data.WeekStartDay)
	require.Equal(t, 14, got.Data.PostedLeadDays)
	require.Nil(t, got.Data.MinorCutoffMin)
	require.Nil(t, got.Data.QuietHoursStartMin)

	// 11. Malformed JSON is a 400.
	w = doRaw(http.MethodPut, "/b/42/schedule/settings", "{not json")
	require.Equal(t, http.StatusBadRequest, w.Code)
}
