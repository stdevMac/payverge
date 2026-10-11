package analytics

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/models"
)

type PageAnalyticsService struct {
	db *database.DB
}

func NewPageAnalyticsService(db *database.DB) *PageAnalyticsService {
	return &PageAnalyticsService{db: db}
}

// TrackPageView records a page view event
func (s *PageAnalyticsService) TrackPageView(pv *models.PageView) error {
	result := s.db.GetGorm().Exec(`
		INSERT INTO page_views (
			session_id, page, referrer, user_agent, ip_address, 
			country, city, device_type, browser, os, 
			screen_width, screen_height, locale, timestamp, duration
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		pv.SessionID, pv.Page, pv.Referrer, pv.UserAgent, pv.IPAddress,
		pv.Country, pv.City, pv.DeviceType, pv.Browser, pv.OS,
		pv.ScreenWidth, pv.ScreenHeight, pv.Locale, pv.Timestamp, pv.Duration,
	)

	if result.Error != nil {
		return fmt.Errorf("failed to track page view: %w", result.Error)
	}

	// Update session summary asynchronously via an O(1) incremental upsert
	// (no page_views rescan, no user_interactions COUNT).
	logger.SafeGo(func() {
		if err := s.updateSessionSummary(pv); err != nil {
			log.Printf("WARNING: failed to update session summary: %v", err)
		}
	})

	return nil
}

// TrackInteraction records a user interaction event
func (s *PageAnalyticsService) TrackInteraction(interaction *models.UserInteraction) error {
	result := s.db.GetGorm().Exec(`
		INSERT INTO user_interactions (
			session_id, page, event_type, event_category, event_label, 
			event_value, x_position, y_position, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		interaction.SessionID, interaction.Page, interaction.EventType,
		interaction.EventCategory, interaction.EventLabel, interaction.EventValue,
		interaction.XPosition, interaction.YPosition, interaction.Timestamp,
	)

	if result.Error != nil {
		return fmt.Errorf("failed to track interaction: %w", result.Error)
	}

	// Maintain the per-session interaction counter (and page_duration rollup)
	// incrementally (O(1)), mirroring the async pattern of the other Track* methods.
	logger.SafeGo(func() {
		if err := s.incrementSessionInteraction(interaction); err != nil {
			log.Printf("WARNING: failed to increment session interaction count: %v", err)
		}
		if err := s.applyPageDurationFromInteraction(interaction); err != nil {
			log.Printf("WARNING: failed to apply page_duration to session: %v", err)
		}
	})

	return nil
}

// applyPageDurationFromInteraction rolls a frontend page_duration interaction
// into session_summaries.total_duration. The frontend emits dwell time as
// event_type=page_duration with event_value={"duration":N} (seconds) because
// page-view rows never received an update path — without this consumer,
// AverageSessionTime is structurally zero platform-wide.
func (s *PageAnalyticsService) applyPageDurationFromInteraction(interaction *models.UserInteraction) error {
	if interaction == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(interaction.EventType), "page_duration") {
		return nil
	}
	seconds := parsePageDurationSeconds(interaction.EventValue)
	if seconds <= 0 {
		return nil
	}

	// Atomic upsert: create the summary if the duration beat the page view,
	// otherwise add to total_duration. total_page_views stays page-view-driven.
	result := s.db.GetGorm().Exec(`
		INSERT INTO session_summaries (
			session_id, first_seen, last_seen, total_duration
		) VALUES (?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			total_duration = session_summaries.total_duration + excluded.total_duration
	`,
		interaction.SessionID, interaction.Timestamp, interaction.Timestamp, seconds,
	)
	return result.Error
}

// parsePageDurationSeconds extracts a non-negative integer duration from the
// frontend event_value payload. Accepts {"duration":N}, {"duration":"N"}, or a
// bare numeric string. Returns 0 when unparseable.
func parsePageDurationSeconds(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	// Prefer the JSON shape the frontend emits: {"duration": 45}
	var payload struct {
		Duration json.Number `json:"duration"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		if n, err := payload.Duration.Int64(); err == nil && n > 0 {
			if n > 24*60*60 { // clamp to 24h to bound abuse
				return 24 * 60 * 60
			}
			return int(n)
		}
	}
	// Bare integer fallback.
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err == nil && n > 0 {
		if n > 24*60*60 {
			return 24 * 60 * 60
		}
		return n
	}
	return 0
}

// TrackConversion records a conversion event
func (s *PageAnalyticsService) TrackConversion(conversion *models.ConversionEvent) error {
	result := s.db.GetGorm().Exec(`
		INSERT INTO conversion_events (
			session_id, conversion_type, value, metadata, timestamp
		) VALUES (?, ?, ?, ?, ?)
	`,
		conversion.SessionID, conversion.ConversionType,
		conversion.Value, conversion.Metadata, conversion.Timestamp,
	)

	if result.Error != nil {
		return fmt.Errorf("failed to track conversion: %w", result.Error)
	}

	// Update session summary with conversion
	logger.SafeGo(func() {
		if err := s.markSessionConverted(conversion.SessionID, conversion.ConversionType); err != nil {
			log.Printf("WARNING: failed to mark session as converted: %v", err)
		}
	})

	return nil
}

// updateSessionSummary applies a single page view to the session summary via
// one atomic incremental upsert. It never rescans page_views or COUNTs
// user_interactions, so cost is O(1) per event rather than O(K) per session
// (DUP-01). On INSERT it seeds first_seen/last_seen/device_type/country from
// this (the first-processed) page view and a single-element pages_visited array;
// on CONFLICT it bumps last_seen, increments the page-view count, adds this
// page's duration, and appends the page to pages_visited with a dialect-aware
// JSON append. total_interactions is owned by incrementSessionInteraction and
// is intentionally left untouched here (it defaults to 0 on INSERT).
//
// Note: first_seen is now the first-PROCESSED page view's timestamp rather than
// a recomputed MIN(timestamp). With strictly-ordered async processing these are
// identical; only a rare async reorder could make first_seen reflect a slightly
// later page view. This matches the prior first-row semantics closely enough for
// the recent-sessions view and avoids a per-event rescan.
func (s *PageAnalyticsService) updateSessionSummary(pv *models.PageView) error {
	if pv == nil {
		return nil
	}

	// pages_visited starts as a JSON array of just this new page.
	pagesJSON, _ := json.Marshal([]string{pv.Page})

	// Dialect-aware append fragment for the CONFLICT path. The whole statement
	// stays a single atomic upsert (no read-modify-write race).
	//
	// In the ON CONFLICT DO UPDATE SET clause every reference to the EXISTING row
	// is table-qualified (session_summaries.col). On PostgreSQL both the target
	// table and the special `excluded` pseudo-relation are in scope, so a bare
	// column name that exists in both is ambiguous and Postgres rejects the whole
	// upsert with "column reference ... is ambiguous" (SQLSTATE 42702) — which
	// silently dropped every session-summary update in production. SQLite has no
	// such ambiguity but accepts the qualified form too, so we qualify uniformly.
	var appendFragment string
	switch s.db.GetGorm().Dialector.Name() {
	case "postgres":
		appendFragment = `pages_visited = (COALESCE(NULLIF(session_summaries.pages_visited, ''), '[]')::jsonb || to_jsonb(?::text))::text`
	default: // sqlite (and any json1-compatible engine)
		appendFragment = `pages_visited = json_insert(COALESCE(NULLIF(pages_visited, ''), '[]'), '$[#]', ?)`
	}

	sql := `
		INSERT INTO session_summaries (
			session_id, first_seen, last_seen, total_page_views,
			total_duration, pages_visited, device_type, country
		) VALUES (?, ?, ?, 1, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			last_seen = excluded.last_seen,
			total_page_views = session_summaries.total_page_views + 1,
			total_duration = session_summaries.total_duration + excluded.total_duration,
			` + appendFragment + `
	`

	result := s.db.GetGorm().Exec(sql,
		// INSERT row (proposed / excluded.*)
		pv.SessionID, pv.Timestamp, pv.Timestamp, pv.Duration,
		string(pagesJSON), pv.DeviceType, pv.Country,
		// CONFLICT append bind param
		pv.Page,
	)

	return result.Error
}

// incrementSessionInteraction maintains the per-session interaction counter with
// a single atomic upsert. If the summary row does not yet exist (interaction
// arrived before any page view) it is created with total_interactions=1 and the
// page-view counters defaulting to 0; a later page view then increments those
// without clobbering this count. On CONFLICT only total_interactions is bumped —
// first_seen/last_seen stay page-view-driven, preserving prior semantics.
func (s *PageAnalyticsService) incrementSessionInteraction(interaction *models.UserInteraction) error {
	if interaction == nil {
		return nil
	}

	// total_interactions on the RHS is table-qualified for the same reason as
	// updateSessionSummary: in ON CONFLICT DO UPDATE the existing row and
	// `excluded` are both in scope, so a bare column name is ambiguous on
	// PostgreSQL (SQLSTATE 42702) and would drop the interaction increment.
	result := s.db.GetGorm().Exec(`
		INSERT INTO session_summaries (
			session_id, first_seen, last_seen, total_interactions
		) VALUES (?, ?, ?, 1)
		ON CONFLICT(session_id) DO UPDATE SET
			total_interactions = session_summaries.total_interactions + 1
	`,
		interaction.SessionID, interaction.Timestamp, interaction.Timestamp,
	)

	return result.Error
}

// markSessionConverted marks a session as converted
func (s *PageAnalyticsService) markSessionConverted(sessionID, conversionType string) error {
	result := s.db.GetGorm().Exec(`
		UPDATE session_summaries 
		SET converted = true, conversion_type = ?
		WHERE session_id = ?
	`, conversionType, sessionID)

	return result.Error
}

// GetAnalyticsSummary returns aggregated analytics data for a date range
func (s *PageAnalyticsService) GetAnalyticsSummary(startDate, endDate time.Time) (*models.AnalyticsSummary, error) {
	summary := &models.AnalyticsSummary{
		DeviceBreakdown:  make(map[string]int64),
		CountryBreakdown: make(map[string]int64),
	}

	// Adjust endDate to include the entire day (add 1 day and use < instead of <=)
	endDateInclusive := endDate.AddDate(0, 0, 1)

	// Page-view scalar counts: total page views + distinct sessions in ONE
	// scan of page_views (DUP-03). Identical numbers to the prior two queries.
	var pageViewCounts struct {
		TotalPageViews int64
		TotalSessions  int64
	}
	s.db.GetGorm().Raw(`
		SELECT COUNT(*) AS total_page_views,
		       COUNT(DISTINCT session_id) AS total_sessions
		FROM page_views
		WHERE timestamp >= ? AND timestamp < ?
	`, startDate, endDateInclusive).Scan(&pageViewCounts)
	summary.TotalPageViews = pageViewCounts.TotalPageViews
	summary.TotalSessions = pageViewCounts.TotalSessions

	// Total interactions
	s.db.GetGorm().Raw(`
		SELECT COUNT(*) FROM user_interactions 
		WHERE timestamp >= ? AND timestamp < ?
	`, startDate, endDateInclusive).Scan(&summary.TotalInteractions)

	// Total conversions
	s.db.GetGorm().Raw(`
		SELECT COUNT(*) FROM conversion_events 
		WHERE timestamp >= ? AND timestamp < ?
	`, startDate, endDateInclusive).Scan(&summary.TotalConversions)

	// Session-summary scalar metrics: average duration + bounced-session count
	// (single-page-view sessions) in ONE conditional aggregate over
	// session_summaries (DUP-03). Identical numbers to the prior two queries.
	var sessionMetrics struct {
		AvgDuration     float64
		BouncedSessions int64
	}
	s.db.GetGorm().Raw(`
		SELECT COALESCE(AVG(total_duration), 0) AS avg_duration,
		       COALESCE(SUM(CASE WHEN total_page_views = 1 THEN 1 ELSE 0 END), 0) AS bounced_sessions
		FROM session_summaries
		WHERE first_seen >= ? AND first_seen < ?
	`, startDate, endDateInclusive).Scan(&sessionMetrics)
	summary.AverageSessionTime = sessionMetrics.AvgDuration
	if summary.TotalSessions > 0 {
		summary.BounceRate = float64(sessionMetrics.BouncedSessions) / float64(summary.TotalSessions) * 100
	}

	// Conversion rate
	if summary.TotalSessions > 0 {
		summary.ConversionRate = float64(summary.TotalConversions) / float64(summary.TotalSessions) * 100
	}

	// Top pages
	summary.TopPages = s.getTopPages(startDate, endDateInclusive)

	// Top interactions
	summary.TopInteractions = s.getTopInteractions(startDate, endDateInclusive)

	// Device breakdown
	type DeviceCount struct {
		DeviceType string
		Count      int64
	}
	var devices []DeviceCount
	s.db.GetGorm().Raw(`
		SELECT device_type, COUNT(*) as count 
		FROM session_summaries 
		WHERE first_seen >= ? AND first_seen < ? AND device_type IS NOT NULL AND device_type != ''
		GROUP BY device_type
	`, startDate, endDateInclusive).Scan(&devices)
	for _, d := range devices {
		summary.DeviceBreakdown[d.DeviceType] = d.Count
	}

	// Country breakdown
	type CountryCount struct {
		Country string
		Count   int64
	}
	var countries []CountryCount
	s.db.GetGorm().Raw(`
		SELECT country, COUNT(*) as count 
		FROM session_summaries 
		WHERE first_seen >= ? AND first_seen < ? AND country IS NOT NULL AND country != ''
		GROUP BY country
		ORDER BY count DESC
		LIMIT 10
	`, startDate, endDateInclusive).Scan(&countries)
	for _, c := range countries {
		summary.CountryBreakdown[c.Country] = c.Count
	}

	// Hourly traffic
	summary.HourlyTraffic = s.getHourlyTraffic(startDate, endDateInclusive)

	// Conversion funnel
	summary.ConversionFunnel = s.getConversionFunnel(startDate, endDateInclusive)

	return summary, nil
}

// getTopPages returns the most viewed pages
func (s *PageAnalyticsService) getTopPages(startDate, endDate time.Time) []models.PageStats {
	var stats []models.PageStats

	s.db.GetGorm().Raw(`
		SELECT 
			page,
			COUNT(*) as views,
			COUNT(DISTINCT session_id) as unique_visitors,
			COALESCE(AVG(duration), 0) as average_duration
		FROM page_views
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY page
		ORDER BY views DESC
		LIMIT 10
	`, startDate, endDate).Scan(&stats)

	return stats
}

// getTopInteractions returns the most common interactions
func (s *PageAnalyticsService) getTopInteractions(startDate, endDate time.Time) []models.InteractionStats {
	var stats []models.InteractionStats

	s.db.GetGorm().Raw(`
		SELECT 
			event_type,
			event_category,
			event_label,
			COUNT(*) as count,
			COUNT(DISTINCT session_id) as unique_sessions
		FROM user_interactions
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY event_type, event_category, event_label
		ORDER BY count DESC
		LIMIT 20
	`, startDate, endDate).Scan(&stats)

	return stats
}

// getHourlyTraffic returns traffic statistics by hour
func (s *PageAnalyticsService) getHourlyTraffic(startDate, endDate time.Time) []models.HourlyStats {
	var stats []models.HourlyStats

	s.db.GetGorm().Raw(`
		SELECT 
			EXTRACT(HOUR FROM timestamp)::INTEGER as hour,
			COUNT(*) as page_views,
			COUNT(DISTINCT session_id) as sessions
		FROM page_views
		WHERE timestamp >= ? AND timestamp < ?
		GROUP BY hour
		ORDER BY hour
	`, startDate, endDate).Scan(&stats)

	return stats
}

// getConversionFunnel returns independent distinct-session reach for the
// operator onboarding routes that record page views: the /dashboard sign-in
// and venue list, venue registration, and a venue's own dashboard. The
// self-hosted build has no marketing pages, so the old landing/how-it-works/
// pricing steps were always zero. Drop-off is omitted when the previous step
// has zero sessions or this step increased (#430).
func (s *PageAnalyticsService) getConversionFunnel(startDate, endDate time.Time) []models.FunnelStep {
	type funnelStepSpec struct {
		name      string
		predicate string
		arg       interface{}
	}
	// Patterns are bound, never inlined: a literal '?' inside a LIKE is eaten
	// as a GORM placeholder and shifts every later argument (#430).
	steps := []funnelStepSpec{
		{"Operator Sign-in", "page LIKE ?", "/dashboard%"},
		{"Venue Registration", "page LIKE ?", "/business/register%"},
		{"Venue Dashboard", "page LIKE ?", "/business/%/dashboard%"},
	}

	selectCols := make([]string, len(steps))
	args := make([]interface{}, 0, len(steps)+2)
	for i, step := range steps {
		selectCols[i] = fmt.Sprintf("COUNT(DISTINCT CASE WHEN %s THEN session_id END) AS step%d", step.predicate, i)
		if step.arg != nil {
			args = append(args, step.arg)
		}
	}
	args = append(args, startDate, endDate)

	query := fmt.Sprintf(`
		SELECT %s
		FROM page_views
		WHERE timestamp >= ? AND timestamp < ?
	`, strings.Join(selectCols, ",\n\t\t\t"))

	stepCounts := make([]int64, len(steps))
	if row := s.db.GetGorm().Raw(query, args...).Row(); row != nil {
		scanTargets := make([]interface{}, len(steps))
		for i := range steps {
			scanTargets[i] = &stepCounts[i]
		}
		_ = row.Scan(scanTargets...)
	}

	funnel := make([]models.FunnelStep, 0, len(steps))
	var previousSessions int64
	for i, step := range steps {
		sessions := stepCounts[i]
		var dropoff *float64
		if i > 0 && previousSessions > 0 && sessions <= previousSessions {
			rate := float64(previousSessions-sessions) / float64(previousSessions) * 100
			dropoff = &rate
		}
		funnel = append(funnel, models.FunnelStep{
			Step:        step.name,
			Sessions:    sessions,
			DropoffRate: dropoff,
		})
		previousSessions = sessions
	}

	return funnel
}

// GetRecentSessions returns recent session data
func (s *PageAnalyticsService) GetRecentSessions(limit int) ([]models.SessionSummary, error) {
	var sessions []models.SessionSummary

	err := s.db.GetGorm().Raw(`
		SELECT 
			session_id, first_seen, last_seen, total_page_views, 
			total_interactions, total_duration, pages_visited, 
			converted, COALESCE(conversion_type, '') as conversion_type, 
			COALESCE(device_type, '') as device_type, 
			COALESCE(country, '') as country
		FROM session_summaries
		ORDER BY last_seen DESC
		LIMIT ?
	`, limit).Scan(&sessions).Error

	return sessions, err
}
