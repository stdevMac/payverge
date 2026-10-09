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
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newLogbookTestServer wires LogbookHandler against in-memory SQLite. A tiny
// middleware injects staff_id like the staff-auth middleware does in prod (when
// staffID==0 it sets nothing, simulating a non-staff/expired session). Routes
// carry NO RBAC middleware — schedule:read is enforced in main.go.
func newLogbookTestServer(t *testing.T, staffID uint) (func(method, url string, body interface{}) *httptest.ResponseRecorder, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&database.Business{}, &database.Staff{}, &database.ShiftNote{}))
	database.SetTestDB(g)

	h := NewLogbookHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if staffID != 0 {
			c.Set("staff_id", staffID)
		}
		c.Next()
	})
	r.GET("/b/:id/shift-notes", h.List)
	r.POST("/b/:id/shift-notes", h.Create)

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
	return do, func() { _ = sqlDB.Close() }
}

func TestLogbookHandlerHTTP(t *testing.T) {
	do, cleanup := newLogbookTestServer(t, 7)
	defer cleanup()

	// 1. Create with a valid category → 201, author from context (not body).
	w := do(http.MethodPost, "/b/42/shift-notes", map[string]interface{}{
		"date": "2026-06-30", "category": "sales", "content": "Strong brunch",
		"author_staff_id": 999, // spoof attempt — must be ignored
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		Data struct {
			ID            uint   `json:"id"`
			AuthorStaffID uint   `json:"author_staff_id"`
			Category      string `json:"category"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.Equal(t, uint(7), created.Data.AuthorStaffID) // from ctx, not the spoofed 999
	require.NotZero(t, created.Data.ID)

	// 2. Unknown category → 400, nothing persisted.
	w = do(http.MethodPost, "/b/42/shift-notes", map[string]interface{}{
		"date": "2026-06-30", "category": "payroll", "content": "x"})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 3. Empty content → 400.
	w = do(http.MethodPost, "/b/42/shift-notes", map[string]interface{}{
		"date": "2026-06-30", "category": "other", "content": "   "})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 4. Bad date → 400.
	w = do(http.MethodPost, "/b/42/shift-notes", map[string]interface{}{
		"date": "06/30/2026", "category": "other", "content": "x"})
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 5. Second valid note same day for ordering check.
	w = do(http.MethodPost, "/b/42/shift-notes", map[string]interface{}{
		"date": "2026-06-30", "category": "maintenance", "content": "Fan rattling"})
	require.Equal(t, http.StatusCreated, w.Code)

	// 6. List that day → 2 notes, newest first.
	w = do(http.MethodGet, "/b/42/shift-notes?date=2026-06-30", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var list struct {
		Data []struct {
			Content  string `json:"content"`
			Category string `json:"category"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 2)
	require.Equal(t, "Fan rattling", list.Data[0].Content) // newest first

	// 7. A different day is empty.
	w = do(http.MethodGet, "/b/42/shift-notes?date=2026-07-01", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 0)

	// 8. Malformed ?date= → 400.
	w = do(http.MethodGet, "/b/42/shift-notes?date=not-a-date", nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLogbookCreateRequiresStaffSession(t *testing.T) {
	do, cleanup := newLogbookTestServer(t, 0) // no staff_id in ctx
	defer cleanup()
	w := do(http.MethodPost, "/b/42/shift-notes", map[string]interface{}{
		"date": "2026-06-30", "category": "sales", "content": "x"})
	require.Equal(t, http.StatusForbidden, w.Code)
}

// TestLogbookListEnrichesAuthorNames proves the operator-facing feed attaches an
// author_name resolved from the staff roster (accountability), and that a note
// from an unknown/removed author degrades to an empty name rather than erroring.
func TestLogbookListEnrichesAuthorNames(t *testing.T) {
	do, cleanup := newLogbookTestServer(t, 7)
	defer cleanup()
	g := database.GetDB()
	require.NoError(t, g.Create(&database.Business{ID: 42, BusinessId: "biz-42"}).Error)
	require.NoError(t, g.Create(&database.Staff{ID: 7, BusinessID: 42, Email: "ana@x.co", Name: "Ana", Role: database.StaffRoleManager, IsActive: true, InvitedBy: "o"}).Error)

	// Two notes: one by Ana (id 7), one by a stranger id 99 (no staff row).
	w := do(http.MethodPost, "/b/42/shift-notes", map[string]interface{}{
		"date": "2026-06-30", "category": "sales", "content": "Busy lunch"})
	require.Equal(t, http.StatusCreated, w.Code)
	require.NoError(t, g.Create(&database.ShiftNote{BusinessID: 42, ForDate: mustDay("2026-06-30"), AuthorStaffID: 99, Category: "other", Content: "ghost note"}).Error)

	w = do(http.MethodGet, "/b/42/shift-notes?date=2026-06-30", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var list struct {
		Data []struct {
			AuthorStaffID uint   `json:"author_staff_id"`
			AuthorName    string `json:"author_name"`
			Content       string `json:"content"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 2)
	byAuthor := map[uint]string{}
	for _, n := range list.Data {
		byAuthor[n.AuthorStaffID] = n.AuthorName
	}
	require.Equal(t, "Ana", byAuthor[7], "known author resolves to their name")
	require.Equal(t, "", byAuthor[99], "unknown/removed author degrades to empty, not an error")
}

func mustDay(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}
