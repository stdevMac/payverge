package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupPageAnalyticsTestDB spins up an in-memory sqlite DB and creates the
// raw analytics tables (page_views + session_summaries) the handler writes to.
func setupPageAnalyticsTestDB(t *testing.T) *database.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	// Create the analytics tables with sqlite-compatible schema (no
	// BIGSERIAL). The handler uses raw SQL so we can't rely on GORM
	// auto-migration.
	require.NoError(t, gormDB.Exec(`
		CREATE TABLE IF NOT EXISTS page_views (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			page TEXT NOT NULL,
			referrer TEXT,
			user_agent TEXT,
			ip_address TEXT,
			country TEXT,
			city TEXT,
			device_type TEXT,
			browser TEXT,
			os TEXT,
			screen_width INTEGER,
			screen_height INTEGER,
			locale TEXT,
			timestamp TIMESTAMP,
			duration INTEGER DEFAULT 0
		)
	`).Error)
	require.NoError(t, gormDB.Exec(`
		CREATE TABLE IF NOT EXISTS user_interactions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			page TEXT NOT NULL,
			event_type TEXT NOT NULL,
			event_category TEXT,
			event_label TEXT,
			event_value TEXT,
			x_position INTEGER,
			y_position INTEGER,
			timestamp TIMESTAMP
		)
	`).Error)
	require.NoError(t, gormDB.Exec(`
		CREATE TABLE IF NOT EXISTS conversion_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			conversion_type TEXT NOT NULL,
			value REAL DEFAULT 0,
			metadata TEXT,
			timestamp TIMESTAMP
		)
	`).Error)
	require.NoError(t, gormDB.Exec(`
		CREATE TABLE IF NOT EXISTS session_summaries (
			session_id TEXT PRIMARY KEY,
			first_seen TIMESTAMP NOT NULL,
			last_seen TIMESTAMP NOT NULL,
			total_page_views INTEGER DEFAULT 0,
			total_interactions INTEGER DEFAULT 0,
			total_duration INTEGER DEFAULT 0,
			pages_visited TEXT,
			converted BOOLEAN DEFAULT 0,
			conversion_type TEXT,
			device_type TEXT,
			country TEXT
		)
	`).Error)

	database.SetTestDB(gormDB)
	return database.GetDBWrapper()
}

func TestTrackPageView_OverridesClientSuppliedIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPageAnalyticsTestDB(t)

	service := analytics.NewPageAnalyticsService(db)
	handler := NewPageAnalyticsHandler(service)

	router := gin.New()
	router.POST("/analytics/page-view", handler.TrackPageView)

	// Body pre-fills a spoofed IP — must be ignored in favor of c.ClientIP().
	body := map[string]any{
		"session_id": "sess-spoof-1",
		"page":       "/home",
		"ip_address": "1.2.3.4",
	}
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/analytics/page-view", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	// Force a known client IP; gin's default trust proxies look at RemoteAddr.
	req.RemoteAddr = "203.0.113.77:54321"

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	// Assert the stored row uses the server-derived IP, not the body value.
	var storedIP string
	require.NoError(t, db.GetGorm().Raw(
		"SELECT ip_address FROM page_views WHERE session_id = ?", "sess-spoof-1",
	).Row().Scan(&storedIP))

	assert.NotEqual(t, "1.2.3.4", storedIP, "server must discard body-supplied IP")
	assert.Equal(t, "203.0.113.77", storedIP, "server must use request remote IP")
}

func TestAdminSessionsLimit_Clamps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPageAnalyticsTestDB(t)

	// Insert 600 session summaries so we can observe the limit.
	now := time.Now()
	for i := 0; i < 600; i++ {
		require.NoError(t, db.GetGorm().Exec(`
			INSERT INTO session_summaries (
				session_id, first_seen, last_seen, total_page_views,
				total_interactions, total_duration, pages_visited,
				converted, conversion_type, device_type, country
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			fmt.Sprintf("sess-%04d", i),
			now.Add(time.Duration(i)*time.Second),
			now.Add(time.Duration(i)*time.Second),
			1, 0, 0, "[]", false, "", "desktop", "US",
		).Error)
	}

	service := analytics.NewPageAnalyticsService(db)
	handler := NewPageAnalyticsHandler(service)

	router := gin.New()
	router.GET("/admin/analytics/sessions", handler.GetRecentSessions)

	req := httptest.NewRequest(http.MethodGet, "/admin/analytics/sessions?limit=10000000", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var sessions []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sessions))

	assert.Equal(t, 500, len(sessions), "admin sessions limit must clamp to 500")
}
