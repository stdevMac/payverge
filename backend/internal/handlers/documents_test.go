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

func newDocumentTestServer(t *testing.T, staffID uint, role database.StaffRole) (
	func(method, url string, body interface{}) *httptest.ResponseRecorder, *database.DB, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Staff{}, &database.Position{}, &database.StaffPosition{}, &database.Document{}, &database.DocumentAck{}))
	database.SetTestDB(gormDB)
	d := database.GetDBWrapper()
	h := NewDocumentHandler(d)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if staffID != 0 {
			c.Set("staff_id", staffID)
			c.Set("staff_role", string(role))
		}
		c.Next()
	})
	r.GET("/b/:id/documents", h.List)
	r.POST("/b/:id/documents", h.Create)
	r.PUT("/b/:id/documents/:docId", h.Update)
	r.POST("/b/:id/documents/:docId/ack", h.Ack)
	do := func(method, url string, body interface{}) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			bb, _ := json.Marshal(body)
			rdr = bytes.NewReader(bb)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, url, rdr)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	return do, d, func() { _ = sqlDB.Close() }
}

// TestDocumentCreateRequiresContentOrURL: a doc with neither body nor link 400s.
func TestDocumentCreateRequiresContentOrURL(t *testing.T) {
	do, d, cleanup := newDocumentTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)

	w := do(http.MethodPost, "/b/1/documents", map[string]any{"title": "Handbook", "require_ack": true})
	require.Equal(t, http.StatusBadRequest, w.Code)

	w = do(http.MethodPost, "/b/1/documents", map[string]any{"title": "Handbook", "content": "be kind", "require_ack": true})
	require.Equal(t, http.StatusCreated, w.Code)
}

// TestDocumentAckRoundtrip: ack returns 200; unknown doc 404; no dollars leak.
func TestDocumentAckRoundtrip(t *testing.T) {
	do, d, cleanup := newDocumentTestServer(t, 7, database.StaffRoleServer)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	doc := &database.Document{BusinessID: 1, CreatedByStaffID: 1, Title: "Handbook", Content: "v1", Version: 1, RequireAck: true, AudienceFilter: "all", IsActive: true}
	require.NoError(t, d.GetGorm().Create(doc).Error)

	w := do(http.MethodPost, fmt.Sprintf("/b/1/documents/%d/ack", doc.ID), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code)

	w = do(http.MethodPost, "/b/1/documents/9999/ack", map[string]any{})
	require.Equal(t, http.StatusNotFound, w.Code)

	w = do(http.MethodGet, "/b/1/documents", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "dollar")
	require.NotContains(t, w.Body.String(), "amount")
}

// TestDocumentListReturnsAckedMap: GET /documents carries a per-caller `acked`
// map; ack flips the caller's entry to true; a version bump re-opens it (false).
func TestDocumentListReturnsAckedMap(t *testing.T) {
	do, d, cleanup := newDocumentTestServer(t, 7, database.StaffRoleServer)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	doc := &database.Document{BusinessID: 1, CreatedByStaffID: 1, Title: "Handbook", Content: "v1", Version: 1, RequireAck: true, AudienceFilter: "all", IsActive: true}
	require.NoError(t, d.GetGorm().Create(doc).Error)

	type listResp struct {
		Data  []struct{ ID uint } `json:"data"`
		Acked map[uint]bool       `json:"acked"`
	}
	parse := func(w *httptest.ResponseRecorder) listResp {
		var r listResp
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
		return r
	}

	w := do(http.MethodGet, "/b/1/documents", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, parse(w).Acked[doc.ID], "not yet acked")

	w = do(http.MethodPost, fmt.Sprintf("/b/1/documents/%d/ack", doc.ID), map[string]any{})
	require.Equal(t, http.StatusOK, w.Code)
	w = do(http.MethodGet, "/b/1/documents", nil)
	require.True(t, parse(w).Acked[doc.ID], "acked at current version")

	// Manager bumps the version via PUT → re-opens ack for the server caller.
	asMgr := documentRouterUpdateAs(2, database.StaffRoleManager)
	w = asMgr(http.MethodPut, fmt.Sprintf("/b/1/documents/%d", doc.ID), map[string]any{"title": "Handbook", "content": "v2"})
	require.Equal(t, http.StatusOK, w.Code)

	w = do(http.MethodGet, "/b/1/documents", nil)
	require.False(t, parse(w).Acked[doc.ID], "version bump re-opens ack")
}

// TestDocumentUpdateMissingIs404: PUT on an unknown doc 404s.
func TestDocumentUpdateMissingIs404(t *testing.T) {
	do, d, cleanup := newDocumentTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	w := do(http.MethodPut, "/b/1/documents/9999", map[string]any{"title": "X", "content": "y"})
	require.Equal(t, http.StatusNotFound, w.Code)
}

// documentRouterUpdateAs drives the PUT route as a different identity against the
// CURRENT test DB.
func documentRouterUpdateAs(staffID uint, role database.StaffRole) func(method, url string, body interface{}) *httptest.ResponseRecorder {
	h := NewDocumentHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("staff_id", staffID)
		c.Set("staff_role", string(role))
		c.Next()
	})
	r.PUT("/b/:id/documents/:docId", h.Update)
	return func(method, url string, body interface{}) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			bb, _ := json.Marshal(body)
			rdr = bytes.NewReader(bb)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, url, rdr)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
}

// TestDocumentCreateRejectsInvalidAudience: a malformed audience filter 400s.
func TestDocumentCreateRejectsInvalidAudience(t *testing.T) {
	do, d, cleanup := newDocumentTestServer(t, 1, database.StaffRoleManager)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)

	w := do(http.MethodPost, "/b/1/documents", map[string]any{"title": "Handbook", "content": "be kind", "audience_filter": "role:"})
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestDocumentListAudiencePrivacy: a line-staff caller sees only docs targeted at
// "all"/their role/their dept; a manager caller sees every doc.
func TestDocumentListAudiencePrivacy(t *testing.T) {
	do, d, cleanup := newDocumentTestServer(t, 7, database.StaffRoleServer)
	defer cleanup()
	require.NoError(t, d.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	for _, aud := range []string{"all", "role:server", "role:manager"} {
		require.NoError(t, d.GetGorm().Create(&database.Document{BusinessID: 1, CreatedByStaffID: 1, Title: "D", Content: "x", Version: 1, AudienceFilter: aud, IsActive: true}).Error)
	}

	w := do(http.MethodGet, "/b/1/documents", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []struct {
			AudienceFilter string `json:"audience_filter"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2, "server sees all + role:server, not role:manager")

	asMgr := documentRouterAs(2, database.StaffRoleManager)
	w = asMgr(http.MethodGet, "/b/1/documents", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 3, "manager sees every doc")
}

// documentRouterAs drives the document routes as a different identity against the
// CURRENT test DB.
func documentRouterAs(staffID uint, role database.StaffRole) func(method, url string, body interface{}) *httptest.ResponseRecorder {
	h := NewDocumentHandler(database.GetDBWrapper())
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("staff_id", staffID)
		c.Set("staff_role", string(role))
		c.Next()
	})
	r.GET("/b/:id/documents", h.List)
	return func(method, url string, body interface{}) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			bb, _ := json.Marshal(body)
			rdr = bytes.NewReader(bb)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(method, url, rdr)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
}
