package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

func setupMissingTranslationTestDB(t *testing.T) {
	t.Helper()
	// Per-test in-memory DB (no cache=shared — that would leak rows across
	// tests in this package and break the dedup assertions).
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)

	require.NoError(t, gormDB.AutoMigrate(&database.MissingTranslation{}))
}

func newMissingTranslationRouter() (*gin.Engine, *MissingTranslationHandler) {
	gin.SetMode(gin.TestMode)
	h := NewMissingTranslationHandler()
	r := gin.New()
	r.POST("/api/v1/analytics/missing_translation", h.IngestMissing)
	r.GET("/api/v1/admin/analytics/missing-translations", h.ListMissing)
	r.PATCH("/api/v1/admin/analytics/missing-translations/:id/status", h.UpdateStatus)
	return r, h
}

func TestIngestMissing_PersistsBeaconPayload(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupMissingTranslationTestDB(t)
	r, _ := newMissingTranslationRouter()

	body, _ := json.Marshal(map[string]any{
		"page":          "/b/demo-ai-pro-business",
		"locale":        "es",
		"key":           "error.notFoundEyebrow",
		"fallback_used": "english",
		"timestamp":     "2026-05-23T12:00:00.000Z",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/missing_translation", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	rows, total, err := database.ListMissingTranslations(context.Background(), 10, 0, "", "")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	require.Equal(t, "es", rows[0].Locale)
	require.Equal(t, "error.notFoundEyebrow", rows[0].KeyPath)
	require.Equal(t, "/b/demo-ai-pro-business", rows[0].Page)
	require.Equal(t, "english", rows[0].FallbackUsed)
	require.EqualValues(t, 1, rows[0].OccurrenceCount)
}

func TestIngestMissing_AggregatesDuplicates(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupMissingTranslationTestDB(t)
	r, _ := newMissingTranslationRouter()

	body, _ := json.Marshal(map[string]any{
		"page":          "/dashboard",
		"locale":        "pt",
		"key":           "kitchen.allergenLegend",
		"fallback_used": "leaf",
	})
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/missing_translation", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	rows, total, err := database.ListMissingTranslations(context.Background(), 10, 0, "", "")
	require.NoError(t, err)
	require.EqualValues(t, 1, total, "duplicate tuples must dedupe into a single row")
	require.Len(t, rows, 1)
	require.EqualValues(t, 3, rows[0].OccurrenceCount)
}

func TestIngestMissing_NormalisesUnknownFallback(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupMissingTranslationTestDB(t)
	r, _ := newMissingTranslationRouter()

	body, _ := json.Marshal(map[string]any{
		"locale":        "fr",
		"key":           "checkout.split.title",
		"fallback_used": "garbage-value",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/missing_translation", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	rows, _, err := database.ListMissingTranslations(context.Background(), 10, 0, "", "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "leaf", rows[0].FallbackUsed, "unknown fallback values must be coerced to 'leaf'")
}

func TestIngestMissing_RejectsEmptyLocaleOrKey(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupMissingTranslationTestDB(t)
	r, _ := newMissingTranslationRouter()

	for _, tc := range []map[string]any{
		{"locale": "", "key": "x.y"},
		{"locale": "en", "key": ""},
	} {
		body, _ := json.Marshal(tc)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/missing_translation", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code, "payload %+v should 400", tc)
	}
}

// AITRANS-7: a POST whose locale is not in the canonical registry must be
// rejected (400) rather than stored as an unvalidated free-text value.
func TestIngestMissing_RejectsUnknownLocale(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupMissingTranslationTestDB(t)
	r, _ := newMissingTranslationRouter()

	body, _ := json.Marshal(map[string]any{
		"locale":        "klingon",
		"key":           "checkout.split.title",
		"fallback_used": "leaf",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/missing_translation", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "unknown locale must be rejected, not stored")

	rows, total, err := database.ListMissingTranslations(context.Background(), 10, 0, "", "")
	require.NoError(t, err)
	require.EqualValues(t, 0, total, "unknown-locale beacon must not persist any row")
	require.Len(t, rows, 0)
}

// AITRANS-7: the GET locale filter must be validated/normalized too — an
// unknown ?locale is ignored (treated as no filter) rather than passed through
// to the query unvalidated.
func TestListMissing_IgnoresUnknownLocaleFilter(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupMissingTranslationTestDB(t)
	r, _ := newMissingTranslationRouter()

	for _, p := range []map[string]any{
		{"locale": "es", "key": "a.b", "fallback_used": "english"},
		{"locale": "pt", "key": "c.d", "fallback_used": "leaf"},
	} {
		body, _ := json.Marshal(p)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/missing_translation", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/missing-translations?locale=klingon", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Rows  []*database.MissingTranslation `json:"rows"`
		Total int64                          `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 2, resp.Total, "unknown ?locale must be ignored (no filter), returning all rows")
}

func TestListMissing_FiltersByLocale(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupMissingTranslationTestDB(t)
	r, _ := newMissingTranslationRouter()

	for _, p := range []map[string]any{
		{"locale": "es", "key": "a.b", "fallback_used": "english"},
		{"locale": "pt", "key": "c.d", "fallback_used": "leaf"},
		{"locale": "es", "key": "e.f", "fallback_used": "leaf"},
	} {
		body, _ := json.Marshal(p)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/missing_translation", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/missing-translations?locale=es", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Rows  []*database.MissingTranslation `json:"rows"`
		Total int64                          `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 2, resp.Total)
	require.Len(t, resp.Rows, 2)
	for _, row := range resp.Rows {
		require.Equal(t, "es", row.Locale)
	}
}

func TestListMissing_ExposesHitCountForAdminClients(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupMissingTranslationTestDB(t)
	r, _ := newMissingTranslationRouter()

	body, _ := json.Marshal(map[string]any{
		"page":          "/business/demo/dashboard",
		"locale":        "en",
		"key":           "businessDashboard.dashboard.liveBills.loading",
		"fallback_used": "leaf",
	})
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/missing_translation", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/missing-translations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Rows []struct {
			KeyPath  string `json:"key_path"`
			HitCount int64  `json:"hit_count"`
		} `json:"rows"`
		Total int64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 1, resp.Total)
	require.Len(t, resp.Rows, 1)
	require.Equal(t, "businessDashboard.dashboard.liveBills.loading", resp.Rows[0].KeyPath)
	require.EqualValues(t, 3, resp.Rows[0].HitCount)
}

func TestMissingTranslationStatus_NewRowsAreOpenAndListFiltersStatus(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupMissingTranslationTestDB(t)
	r, _ := newMissingTranslationRouter()

	for _, key := range []string{"open.key", "resolved.key"} {
		body, _ := json.Marshal(map[string]any{
			"locale":        "es",
			"key":           key,
			"fallback_used": "leaf",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/missing_translation", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	rows, total, err := database.ListMissingTranslations(context.Background(), 10, 0, "", database.MissingTranslationStatusOpen)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, database.MissingTranslationStatusOpen, row.Status)
		require.Nil(t, row.StatusUpdatedAt)
	}

	var resolvedID uint
	for _, row := range rows {
		if row.KeyPath == "resolved.key" {
			resolvedID = row.ID
		}
	}
	require.NotZero(t, resolvedID)
	patchMissingTranslationStatus(t, r, resolvedID, database.MissingTranslationStatusResolved, http.StatusOK)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/missing-translations?status=resolved", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var filtered struct {
		Rows []struct {
			ID              uint       `json:"id"`
			Status          string     `json:"status"`
			StatusUpdatedAt *time.Time `json:"status_updated_at"`
		} `json:"rows"`
		Total int64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &filtered))
	require.EqualValues(t, 1, filtered.Total)
	require.Len(t, filtered.Rows, 1)
	require.Equal(t, resolvedID, filtered.Rows[0].ID)
	require.Equal(t, database.MissingTranslationStatusResolved, filtered.Rows[0].Status)
	require.NotNil(t, filtered.Rows[0].StatusUpdatedAt)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/missing-translations?status=not-a-status", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &filtered))
	require.EqualValues(t, 2, filtered.Total, "invalid status filters must be treated as no filter")
}

func TestUpdateMissingTranslationStatus_ValidatesStatusAndUnknownID(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupMissingTranslationTestDB(t)
	r, _ := newMissingTranslationRouter()

	patchMissingTranslationStatus(t, r, 999999, database.MissingTranslationStatusIgnored, http.StatusNotFound)
	patchMissingTranslationStatus(t, r, 999999, "closed", http.StatusBadRequest)
}

func TestRecordMissingTranslation_ResolvedRecurrenceReopensButIgnoredStaysIgnored(t *testing.T) {
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupMissingTranslationTestDB(t)
	r, _ := newMissingTranslationRouter()

	resolved := &database.MissingTranslation{Locale: "es", KeyPath: "resolved.again", FallbackUsed: "leaf"}
	require.NoError(t, database.RecordMissingTranslation(context.Background(), resolved))
	patchMissingTranslationStatus(t, r, resolved.ID, database.MissingTranslationStatusResolved, http.StatusOK)
	require.NoError(t, database.RecordMissingTranslation(context.Background(), &database.MissingTranslation{Locale: "es", KeyPath: "resolved.again", FallbackUsed: "leaf"}))

	openRows, total, err := database.ListMissingTranslations(context.Background(), 10, 0, "", database.MissingTranslationStatusOpen)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, openRows, 1)
	require.EqualValues(t, 2, openRows[0].OccurrenceCount)
	require.Nil(t, openRows[0].StatusUpdatedAt, "reopening must clear the prior resolution timestamp")

	ignored := &database.MissingTranslation{Locale: "pt", KeyPath: "ignored.again", FallbackUsed: "english"}
	require.NoError(t, database.RecordMissingTranslation(context.Background(), ignored))
	patchMissingTranslationStatus(t, r, ignored.ID, database.MissingTranslationStatusIgnored, http.StatusOK)
	require.NoError(t, database.RecordMissingTranslation(context.Background(), &database.MissingTranslation{Locale: "pt", KeyPath: "ignored.again", FallbackUsed: "english"}))

	ignoredRows, total, err := database.ListMissingTranslations(context.Background(), 10, 0, "", database.MissingTranslationStatusIgnored)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, ignoredRows, 1)
	require.EqualValues(t, 2, ignoredRows[0].OccurrenceCount)
	require.NotNil(t, ignoredRows[0].StatusUpdatedAt, "an ignored recurrence must preserve its lifecycle timestamp")
}

func patchMissingTranslationStatus(t *testing.T, r *gin.Engine, id uint, status string, expectedCode int) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"status": status})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/analytics/missing-translations/%d/status", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, expectedCode, w.Code, "response body: %s", w.Body.String())
}
