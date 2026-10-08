package analytics

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// seedSummaryData populates the three raw analytics tables with a deterministic
// dataset so GetAnalyticsSummary's scalar/funnel numbers are hand-computable.
//
// Date range under test: [2026-06-15 00:00, 2026-06-15] -> endDateInclusive
// 2026-06-16 00:00. Anything outside that window must NOT be counted.
func seedSummaryData(t testing.TB, gormDB *gorm.DB) {
	t.Helper()

	inRange := func(h int) time.Time { return time.Date(2026, 6, 15, h, 0, 0, 0, time.UTC) }
	before := time.Date(2026, 6, 14, 23, 0, 0, 0, time.UTC)
	after := time.Date(2026, 6, 16, 1, 0, 0, 0, time.UTC)

	// --- page_views (drive TotalPageViews, TotalSessions, funnel) ---
	// session_id, page, timestamp
	pvs := []struct {
		sess string
		page string
		ts   time.Time
	}{
		// In-range page views.
		{"a", "/dashboard", inRange(1)},            // Operator Sign-in
		{"a", "/business/register", inRange(2)},    // Venue Registration
		{"a", "/business/7/dashboard", inRange(3)}, // Venue Dashboard
		{"b", "/dashboard", inRange(4)},            // Operator Sign-in
		{"b", "/business/register", inRange(5)},    // Venue Registration
		{"c", "/dashboard", inRange(6)},            // Operator Sign-in
		{"c", "/business/9/dashboard", inRange(7)}, // Venue Dashboard
		{"d", "/business/register", inRange(8)},    // Registration only (no sign-in view)
		{"e", "/dashboard?invite=x", inRange(9)},   // Operator Sign-in
		// Out-of-range page views (must be excluded everywhere).
		{"z", "/dashboard", before},
		{"z", "/business/register", after},
	}
	for _, pv := range pvs {
		require.NoError(t, gormDB.Exec(
			`INSERT INTO page_views (session_id, page, timestamp, duration) VALUES (?, ?, ?, 0)`,
			pv.sess, pv.page, pv.ts).Error)
	}
	// In-range page_views: 9 total, distinct sessions {a,b,c,d,e} = 5.
	//
	// Funnel uses the operator onboarding routes that record page views.
	//   step0 /dashboard%              -> a,b,c,e = 4
	//   step1 /business/register%      -> a,b,d   = 3
	//   step2 /business/%/dashboard%   -> a,c     = 2

	// --- session_summaries (drive avg duration, bounce, device, country) ---
	// session_id, first_seen, total_page_views, total_duration
	ss := []struct {
		sess     string
		first    time.Time
		pageView int
		duration int
	}{
		{"a", inRange(1), 3, 100}, // not bounced
		{"b", inRange(4), 1, 200}, // bounced (1 page view)
		{"c", inRange(6), 1, 300}, // bounced (1 page view)
		// Out-of-range summary rows (excluded).
		{"z1", before, 1, 9999},
		{"z2", after, 5, 8888},
	}
	for _, r := range ss {
		require.NoError(t, gormDB.Exec(
			`INSERT INTO session_summaries (session_id, first_seen, last_seen, total_page_views, total_duration, device_type, country)
			 VALUES (?, ?, ?, ?, ?, 'desktop', 'US')`,
			r.sess, r.first, r.first, r.pageView, r.duration).Error)
	}
	// In-range summaries: a,b,c.
	//   AVG(total_duration) = (100+200+300)/3 = 200
	//   bounced (total_page_views=1) = b,c = 2

	// --- conversion_events (drive TotalConversions) ---
	convs := []time.Time{inRange(2), inRange(5), before, after}
	for _, ts := range convs {
		require.NoError(t, gormDB.Exec(
			`INSERT INTO conversion_events (session_id, conversion_type, value, metadata, timestamp) VALUES ('a', 'signup', 0, '', ?)`,
			ts).Error)
	}
	// In-range conversions: 2.

	// --- user_interactions (drive TotalInteractions; untouched by this task) ---
	for _, ts := range []time.Time{inRange(1), inRange(2), inRange(3), before} {
		require.NoError(t, gormDB.Exec(
			`INSERT INTO user_interactions (session_id, page, event_type, timestamp) VALUES ('a', '/', 'click', ?)`,
			ts).Error)
	}
	// In-range interactions: 3.
}

// setupSummaryTestDB builds the conversion_events table on top of the shared
// page_views/user_interactions/session_summaries schema, then seeds it.
// It installs a SQL-capturing logger so callers can assert query shape.
func setupSummaryTestDB(t *testing.T) (*database.DB, *recordingAnalyticsLogger) {
	t.Helper()
	recorder := &recordingAnalyticsLogger{}
	db := setupSessionSummaryTestDB(t, recorder)
	gormDB := db.GetGorm()
	require.NoError(t, gormDB.Exec(`
		CREATE TABLE IF NOT EXISTS conversion_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			conversion_type TEXT,
			value REAL,
			metadata TEXT,
			timestamp TIMESTAMP
		)
	`).Error)
	seedSummaryData(t, gormDB)
	return db, recorder
}

// Expected hand-computed values for the seed in seedSummaryData over
// [2026-06-15, 2026-06-15].
const (
	expTotalPageViews  = int64(9)
	expTotalSessions   = int64(5)
	expTotalInteract   = int64(3)
	expTotalConvert    = int64(2)
	expAvgSessionTime  = float64(200)
	expBouncedSessions = int64(2)
)

// TestGetAnalyticsSummary_QueryShape asserts the three collapse targets:
//   - page_views scalar count+distinct = ONE statement (RED: 2)
//   - session_summaries avg+bounce     = ONE statement (RED: 2)
//   - conversion funnel                = ONE statement (RED: one per step)
func TestGetAnalyticsSummary_QueryShape(t *testing.T) {
	db, recorder := setupSummaryTestDB(t)
	s := NewPageAnalyticsService(db)

	start := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	recorder.Reset()
	_, err := s.GetAnalyticsSummary(start, end)
	require.NoError(t, err)
	sqls := recorder.SQLs()

	// (1) page_views scalar count+distinct: one combined statement that selects
	// both total_page_views and total_sessions in a single scan of page_views.
	pageViewScalar := countSQLsMatchingAll(sqls, "total_page_views", "total_sessions", "FROM page_views")
	require.Equal(t, 1, pageViewScalar,
		"page_views count+distinct must collapse to a single statement; got %d", pageViewScalar)

	// (2) session_summaries avg+bounce: one combined conditional aggregate.
	sessionScalar := countSQLsMatchingAll(sqls, "AVG(total_duration)", "bounced_sessions", "FROM session_summaries")
	require.Equal(t, 1, sessionScalar,
		"session_summaries avg+bounce must collapse to a single conditional aggregate; got %d", sessionScalar)

	// (3) conversion funnel: one combined conditional-distinct query.
	funnelStmts := countSQLsMatchingAll(sqls, "step0", "CASE WHEN page LIKE")
	require.Equal(t, 1, funnelStmts,
		"conversion funnel must collapse to a single conditional-distinct query; got %d", funnelStmts)

	// Defensive: no leftover standalone bounce-rate query of the old
	// "WHERE ... total_page_views = 1" shape (the new conditional aggregate
	// references total_page_views = 1 only inside a CASE WHEN, which is allowed).
	oldBounce := 0
	for _, sql := range sqls {
		if strings.Contains(sql, "total_page_views = 1") && !strings.Contains(sql, "CASE WHEN") {
			oldBounce++
		}
	}
	require.Equal(t, 0, oldBounce, "old standalone bounce query must be gone; got %d", oldBounce)
	// Defensive: no per-step funnel loop survives — every funnel statement must
	// be the single conditional-distinct query (which carries step0/CASE WHEN).
	oldFunnelLoop := 0
	for _, sql := range sqls {
		if strings.Contains(sql, "page LIKE") && strings.Contains(sql, "FROM page_views") && !strings.Contains(sql, "step0") {
			oldFunnelLoop++
		}
	}
	require.Equal(t, 0, oldFunnelLoop, "old per-step funnel query must be gone; got %d", oldFunnelLoop)
}

// TestGetAnalyticsSummary_NumericEquality asserts byte-identical numbers to the
// prior implementation, computed by hand from the seed.
func TestGetAnalyticsSummary_NumericEquality(t *testing.T) {
	db, _ := setupSummaryTestDB(t)
	s := NewPageAnalyticsService(db)

	start := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	summary, err := s.GetAnalyticsSummary(start, end)
	require.NoError(t, err)

	require.Equal(t, expTotalPageViews, summary.TotalPageViews, "TotalPageViews")
	require.Equal(t, expTotalSessions, summary.TotalSessions, "TotalSessions")
	require.Equal(t, expTotalInteract, summary.TotalInteractions, "TotalInteractions")
	require.Equal(t, expTotalConvert, summary.TotalConversions, "TotalConversions")
	require.InDelta(t, expAvgSessionTime, summary.AverageSessionTime, 1e-9, "AverageSessionTime")

	// BounceRate = bounced / TotalSessions * 100 = 2/5*100 = 40
	require.InDelta(t, float64(expBouncedSessions)/float64(expTotalSessions)*100, summary.BounceRate, 1e-9, "BounceRate")
	// ConversionRate = TotalConversions / TotalSessions * 100 = 2/5*100 = 40
	require.InDelta(t, float64(expTotalConvert)/float64(expTotalSessions)*100, summary.ConversionRate, 1e-9, "ConversionRate")

	// Funnel: sessions [4,3,2]; dropoff [nil, 25, 33.33]
	require.Len(t, summary.ConversionFunnel, 3)
	require.Equal(t, "Operator Sign-in", summary.ConversionFunnel[0].Step)
	require.Equal(t, int64(4), summary.ConversionFunnel[0].Sessions)
	require.Nil(t, summary.ConversionFunnel[0].DropoffRate)
	require.Equal(t, "Venue Registration", summary.ConversionFunnel[1].Step)
	require.Equal(t, int64(3), summary.ConversionFunnel[1].Sessions)
	require.NotNil(t, summary.ConversionFunnel[1].DropoffRate)
	require.InDelta(t, 25.0, *summary.ConversionFunnel[1].DropoffRate, 1e-9)
	require.Equal(t, "Venue Dashboard", summary.ConversionFunnel[2].Step)
	require.Equal(t, int64(2), summary.ConversionFunnel[2].Sessions)
	require.NotNil(t, summary.ConversionFunnel[2].DropoffRate)
	require.InDelta(t, 100.0/3.0, *summary.ConversionFunnel[2].DropoffRate, 1e-9)
}

func insertFunnelPageViews(t *testing.T, gormDB *gorm.DB, rows ...[2]string) {
	t.Helper()
	inRange := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	for _, r := range rows {
		require.NoError(t, gormDB.Exec(
			`INSERT INTO page_views (session_id, page, timestamp, duration) VALUES (?, ?, ?, 0)`,
			r[0], r[1], inRange).Error)
	}
}

func funnelSummaryForSeedDay(t *testing.T, db *database.DB) []int64 {
	t.Helper()
	s := NewPageAnalyticsService(db)
	day := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	summary, err := s.GetAnalyticsSummary(day, day)
	require.NoError(t, err)
	out := make([]int64, len(summary.ConversionFunnel))
	for i, step := range summary.ConversionFunnel {
		out[i] = step.Sessions
	}
	return out
}

func TestGetConversionFunnel_BindsEveryPattern(t *testing.T) {
	db := setupSessionSummaryTestDB(t, nil)
	insertFunnelPageViews(t, db.GetGorm(),
		[2]string{"signin", "/dashboard"},
		[2]string{"signin-q", "/dashboard?invite=abc"},
		[2]string{"reg", "/business/register"},
		[2]string{"venue", "/business/12/dashboard"},
	)
	require.Equal(t, []int64{2, 1, 1}, funnelSummaryForSeedDay(t, db),
		"query-string sign-in must count and later binds must not shift")
}

func TestGetConversionFunnel_IgnoresRemovedMarketingRoutesAndNullsZeroDropoff(t *testing.T) {
	db := setupSessionSummaryTestDB(t, nil)
	insertFunnelPageViews(t, db.GetGorm(),
		// The removed marketing pages and the storefront root count nowhere.
		[2]string{"mkt", "/"},
		[2]string{"mkt", "/how-it-works"},
		[2]string{"mkt", "/pricing"},
		// Direct-to-registration: zero prior steps, positive step.
		[2]string{"reg-only", "/business/register"},
		// Increase 1 -> 2 must not be a negative drop-off.
		[2]string{"dash-a", "/business/1/dashboard"},
		[2]string{"dash-b", "/business/2/dashboard"},
	)
	s := NewPageAnalyticsService(db)
	day := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	summary, err := s.GetAnalyticsSummary(day, day)
	require.NoError(t, err)
	require.Len(t, summary.ConversionFunnel, 3)
	require.Equal(t, int64(0), summary.ConversionFunnel[0].Sessions)
	require.Equal(t, int64(1), summary.ConversionFunnel[1].Sessions)
	require.Nil(t, summary.ConversionFunnel[1].DropoffRate, "increase from 0 must not be -0.0% dropoff")
	require.Equal(t, int64(2), summary.ConversionFunnel[2].Sessions)
	require.Nil(t, summary.ConversionFunnel[2].DropoffRate, "increase 1→2 must not be negative dropoff")
}

// countSQLsMatchingAll counts statements that contain ALL of the needles.
func countSQLsMatchingAll(sqls []string, needles ...string) int {
	count := 0
	for _, sql := range sqls {
		all := true
		for _, n := range needles {
			if !strings.Contains(sql, n) {
				all = false
				break
			}
		}
		if all {
			count++
		}
	}
	return count
}

// BenchmarkGetAnalyticsSummary measures one full admin summary over the seeded
// dataset. Query count drops from ~14 to ~9 after the three collapses.
func BenchmarkGetAnalyticsSummary(b *testing.B) {
	dsn := "file:bench_analytics_summary?mode=memory&cache=shared"
	// Silent logger: getHourlyTraffic uses Postgres-only EXTRACT and errors on
	// SQLite (pre-existing, swallowed); its log noise would otherwise corrupt
	// the benchmark output line.
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)

	// The cache=shared in-memory DB persists across -count runs in the same
	// process; drop+recreate so each setup starts from a clean slate (no
	// duplicate-PK on session_summaries).
	for _, tbl := range []string{"page_views", "user_interactions", "session_summaries", "conversion_events"} {
		mustExec(b, gormDB, "DROP TABLE IF EXISTS "+tbl)
	}

	mustExec(b, gormDB, `CREATE TABLE IF NOT EXISTS page_views (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, page TEXT, referrer TEXT, user_agent TEXT, ip_address TEXT, country TEXT, city TEXT, device_type TEXT, browser TEXT, os TEXT, screen_width INTEGER, screen_height INTEGER, locale TEXT, timestamp TIMESTAMP, duration INTEGER DEFAULT 0)`)
	mustExec(b, gormDB, `CREATE TABLE IF NOT EXISTS user_interactions (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, page TEXT, event_type TEXT, event_category TEXT, event_label TEXT, event_value TEXT, x_position INTEGER, y_position INTEGER, timestamp TIMESTAMP)`)
	mustExec(b, gormDB, `CREATE TABLE IF NOT EXISTS session_summaries (session_id TEXT PRIMARY KEY, first_seen TIMESTAMP NOT NULL, last_seen TIMESTAMP NOT NULL, total_page_views INTEGER DEFAULT 0, total_interactions INTEGER DEFAULT 0, total_duration INTEGER DEFAULT 0, pages_visited TEXT, converted BOOLEAN DEFAULT 0, conversion_type TEXT, device_type TEXT, country TEXT)`)
	mustExec(b, gormDB, `CREATE TABLE IF NOT EXISTS conversion_events (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT, conversion_type TEXT, value REAL, metadata TEXT, timestamp TIMESTAMP)`)

	database.SetTestDB(gormDB)
	db := database.GetDBWrapper()
	s := NewPageAnalyticsService(db)

	base := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	pages := []string{"/dashboard", "/business/register", "/business/7/dashboard", "/b/demo"}
	for i := 0; i < 200; i++ {
		sess := fmt.Sprintf("sess-%d", i)
		ts := base.Add(time.Duration(i) * time.Minute)
		page := pages[i%len(pages)]
		mustExec(b, gormDB, `INSERT INTO page_views (session_id, page, timestamp, duration) VALUES (?, ?, ?, ?)`, sess, page, ts, 10)
		mustExec(b, gormDB, `INSERT INTO session_summaries (session_id, first_seen, last_seen, total_page_views, total_duration, device_type, country) VALUES (?, ?, ?, ?, ?, 'desktop', 'US')`, sess, ts, ts, (i%3)+1, 30)
		mustExec(b, gormDB, `INSERT INTO user_interactions (session_id, page, event_type, timestamp) VALUES (?, ?, 'click', ?)`, sess, page, ts)
		if i%5 == 0 {
			mustExec(b, gormDB, `INSERT INTO conversion_events (session_id, conversion_type, value, metadata, timestamp) VALUES (?, 'signup', 0, '', ?)`, sess, ts)
		}
	}

	start := base
	end := base

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.GetAnalyticsSummary(start, end); err != nil {
			b.Fatal(err)
		}
	}
}
