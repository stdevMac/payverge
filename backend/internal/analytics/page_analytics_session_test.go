package analytics

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupSessionSummaryTestDB spins up an in-memory sqlite DB and creates the
// three raw analytics tables (page_views, user_interactions, session_summaries)
// the service writes to via raw SQL. Mirrors handlers.setupPageAnalyticsTestDB
// but lives in the analytics package so we can call unexported helpers, and it
// installs a SQL-capturing GORM logger to assert on the query access-shape.
func setupSessionSummaryTestDB(t *testing.T, gormLogger logger.Interface) *database.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	config := &gorm.Config{}
	if gormLogger != nil {
		config.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), config)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

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

func mkPageView(sessionID, page string, ts time.Time, duration int, device, country string) *models.PageView {
	return &models.PageView{
		SessionID:  sessionID,
		Page:       page,
		Timestamp:  ts,
		Duration:   duration,
		DeviceType: device,
		Country:    country,
	}
}

// TestUpdateSessionSummary_NoPageViewRescanOrInteractionCount is the access-shape
// regression: across N incremental updates the new impl must NOT re-SELECT
// page_views or COUNT(*) user_interactions. RED under the old rescan impl
// (one "FROM page_views" SELECT per call); GREEN after the incremental rewrite.
func TestUpdateSessionSummary_NoPageViewRescanOrInteractionCount(t *testing.T) {
	recorder := &recordingAnalyticsLogger{}
	db := setupSessionSummaryTestDB(t, recorder)
	s := NewPageAnalyticsService(db)

	base := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	pages := []string{"a", "b", "c", "d", "e"}
	durations := []int{10, 20, 30, 40, 50}

	recorder.Reset()
	for i, p := range pages {
		ts := base.Add(time.Duration(i) * time.Minute)
		pv := mkPageView("sess-1", p, ts, durations[i], "desktop", "US")
		require.NoError(t, s.updateSessionSummary(pv))
	}

	sqls := recorder.SQLs()
	pageViewScans := countSQLsContaining(sqls, "FROM page_views")
	interactionCounts := countSQLsContaining(sqls, "COUNT(*) FROM user_interactions")
	require.Equal(t, 0, pageViewScans, "updateSessionSummary must not re-SELECT page_views; got %d scans across 5 calls", pageViewScans)
	require.Equal(t, 0, interactionCounts, "updateSessionSummary must not COUNT(*) user_interactions; got %d", interactionCounts)

	// Correctness of the aggregated row.
	var row models.SessionSummary
	require.NoError(t, db.GetGorm().Raw(`
		SELECT session_id, first_seen, last_seen, total_page_views, total_interactions,
			total_duration, pages_visited, COALESCE(device_type,'') as device_type,
			COALESCE(country,'') as country
		FROM session_summaries WHERE session_id = ?
	`, "sess-1").Scan(&row).Error)

	require.Equal(t, 5, row.TotalPageViews)
	require.Equal(t, 150, row.TotalDuration)
	require.Equal(t, "desktop", row.DeviceType)
	require.Equal(t, "US", row.Country)
	require.WithinDuration(t, base.Add(4*time.Minute), row.LastSeen, time.Second)

	var visited []string
	require.NoError(t, json.Unmarshal([]byte(row.PagesVisited), &visited))
	require.Equal(t, []string{"a", "b", "c", "d", "e"}, visited)
}

// TestTrackInteraction_IncrementsCounter verifies the interaction counter is
// maintained by TrackInteraction's own atomic upsert.
func TestTrackInteraction_IncrementsCounter(t *testing.T) {
	db := setupSessionSummaryTestDB(t, nil)
	s := NewPageAnalyticsService(db)

	base := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	// Seed a page view first so the row exists with page-view fields.
	require.NoError(t, s.updateSessionSummary(mkPageView("sess-2", "home", base, 5, "mobile", "AR")))

	for i := 0; i < 3; i++ {
		ix := &models.UserInteraction{
			SessionID: "sess-2",
			Page:      "home",
			EventType: "click",
			Timestamp: base.Add(time.Duration(i) * time.Second),
		}
		// Call the synchronous counter helper directly to avoid SafeGo races.
		require.NoError(t, s.incrementSessionInteraction(ix))
	}

	var row models.SessionSummary
	require.NoError(t, db.GetGorm().Raw(`
		SELECT session_id, total_page_views, total_interactions, last_seen
		FROM session_summaries WHERE session_id = ?
	`, "sess-2").Scan(&row).Error)
	require.Equal(t, 3, row.TotalInteractions)
	require.Equal(t, 1, row.TotalPageViews)
	// last_seen must stay page-view-driven (interactions do not touch it).
	require.WithinDuration(t, base, row.LastSeen, time.Second)
}

// TestInteractionBeforePageView verifies an interaction can create the summary
// row (total_interactions=1) and a subsequent page view increments page views
// without clobbering the interaction count.
func TestInteractionBeforePageView(t *testing.T) {
	db := setupSessionSummaryTestDB(t, nil)
	s := NewPageAnalyticsService(db)

	base := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)

	ix := &models.UserInteraction{
		SessionID: "sess-3",
		Page:      "landing",
		EventType: "scroll",
		Timestamp: base,
	}
	require.NoError(t, s.incrementSessionInteraction(ix))

	var afterIx models.SessionSummary
	require.NoError(t, db.GetGorm().Raw(`
		SELECT total_page_views, total_interactions, total_duration
		FROM session_summaries WHERE session_id = ?
	`, "sess-3").Scan(&afterIx).Error)
	require.Equal(t, 1, afterIx.TotalInteractions)
	require.Equal(t, 0, afterIx.TotalPageViews)
	require.Equal(t, 0, afterIx.TotalDuration)

	// Page view arrives after the interaction-created row.
	require.NoError(t, s.updateSessionSummary(mkPageView("sess-3", "landing", base.Add(time.Minute), 12, "desktop", "US")))

	var afterPV models.SessionSummary
	require.NoError(t, db.GetGorm().Raw(`
		SELECT total_page_views, total_interactions, total_duration, pages_visited
		FROM session_summaries WHERE session_id = ?
	`, "sess-3").Scan(&afterPV).Error)
	require.Equal(t, 1, afterPV.TotalPageViews, "page view must increment without clobbering")
	require.Equal(t, 1, afterPV.TotalInteractions, "interaction count must survive the page-view upsert")
	require.Equal(t, 12, afterPV.TotalDuration)

	var visited []string
	require.NoError(t, json.Unmarshal([]byte(afterPV.PagesVisited), &visited))
	require.Equal(t, []string{"landing"}, visited)
}

// TestGetRecentSessions_ShapePreserved verifies the admin recent-sessions
// read path returns the same shape after the incremental rewrite.
func TestGetRecentSessions_ShapePreserved(t *testing.T) {
	db := setupSessionSummaryTestDB(t, nil)
	s := NewPageAnalyticsService(db)

	base := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	for i, p := range []string{"x", "y", "z"} {
		require.NoError(t, s.updateSessionSummary(mkPageView("sess-4", p, base.Add(time.Duration(i)*time.Minute), 10*(i+1), "tablet", "BR")))
	}
	require.NoError(t, s.incrementSessionInteraction(&models.UserInteraction{
		SessionID: "sess-4", Page: "x", EventType: "click", Timestamp: base,
	}))

	sessions, err := s.GetRecentSessions(10)
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	got := sessions[0]
	require.Equal(t, "sess-4", got.SessionID)
	require.Equal(t, 3, got.TotalPageViews)
	require.Equal(t, 1, got.TotalInteractions)
	require.Equal(t, 60, got.TotalDuration)
	require.Equal(t, "tablet", got.DeviceType)
	require.Equal(t, "BR", got.Country)

	var visited []string
	require.NoError(t, json.Unmarshal([]byte(got.PagesVisited), &visited))
	require.Equal(t, []string{"x", "y", "z"}, visited)
}

// BenchmarkUpdateSessionSummary measures one more incremental update on a
// session that already has N=20 page views. The old rescan impl scales with N;
// the incremental impl is O(1) regardless of N.
func BenchmarkUpdateSessionSummary(b *testing.B) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", b.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	mustExec(b, gormDB, `CREATE TABLE IF NOT EXISTS page_views (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, page TEXT, referrer TEXT, user_agent TEXT, ip_address TEXT, country TEXT, city TEXT, device_type TEXT, browser TEXT, os TEXT, screen_width INTEGER, screen_height INTEGER, locale TEXT, timestamp TIMESTAMP, duration INTEGER DEFAULT 0)`)
	mustExec(b, gormDB, `CREATE TABLE IF NOT EXISTS user_interactions (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, page TEXT, event_type TEXT, event_category TEXT, event_label TEXT, event_value TEXT, x_position INTEGER, y_position INTEGER, timestamp TIMESTAMP)`)
	mustExec(b, gormDB, `CREATE TABLE IF NOT EXISTS session_summaries (session_id TEXT PRIMARY KEY, first_seen TIMESTAMP NOT NULL, last_seen TIMESTAMP NOT NULL, total_page_views INTEGER DEFAULT 0, total_interactions INTEGER DEFAULT 0, total_duration INTEGER DEFAULT 0, pages_visited TEXT, converted BOOLEAN DEFAULT 0, conversion_type TEXT, device_type TEXT, country TEXT)`)

	database.SetTestDB(gormDB)
	db := database.GetDBWrapper()
	s := NewPageAnalyticsService(db)

	base := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	const sessionID = "bench-sess"
	// Seed N=20 page views (both into page_views and the summary) so the old
	// rescan impl has 20 rows to re-aggregate on each update.
	for i := 0; i < 20; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		mustExec(b, gormDB, `INSERT INTO page_views (session_id, page, timestamp, duration, device_type, country) VALUES (?, ?, ?, ?, ?, ?)`,
			sessionID, fmt.Sprintf("p%d", i), ts, 10, "desktop", "US")
		_ = s.updateSessionSummary(mkPageView(sessionID, fmt.Sprintf("p%d", i), ts, 10, "desktop", "US"))
	}

	pv := mkPageView(sessionID, "p-extra", base.Add(21*time.Minute), 10, "desktop", "US")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.updateSessionSummary(pv); err != nil {
			b.Fatal(err)
		}
	}
}

func mustExec(tb testing.TB, db *gorm.DB, sql string, args ...interface{}) {
	tb.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		tb.Fatal(err)
	}
}
