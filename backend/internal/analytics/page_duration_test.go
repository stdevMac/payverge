package analytics

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/models"

	"github.com/stretchr/testify/require"
)

// page_duration is emitted by the frontend as an interaction event
// (event_type=page_duration, event_value={"duration":N}). It must roll into
// session_summaries.total_duration so AverageSessionTime is not structurally
// zero platform-wide.
func TestPageDurationRollsIntoSessionAverage(t *testing.T) {
	db := setupSessionSummaryTestDB(t, nil)
	s := NewPageAnalyticsService(db)

	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	// Seed a session via page view with zero duration (page-view rows never
	// carried dwell time — the duration travels on the interaction event).
	require.NoError(t, s.updateSessionSummary(mkPageView("sess-dur", "/pricing", base, 0, "desktop", "US")))

	ix := &models.UserInteraction{
		SessionID:     "sess-dur",
		Page:          "/pricing",
		EventType:     "page_duration",
		EventCategory: "engagement",
		EventLabel:    "/pricing",
		EventValue:    `{"duration": 45}`,
		Timestamp:     base.Add(45 * time.Second),
	}
	require.NoError(t, s.applyPageDurationFromInteraction(ix))

	// Second page dwell
	ix2 := &models.UserInteraction{
		SessionID:     "sess-dur",
		Page:          "/features",
		EventType:     "page_duration",
		EventCategory: "engagement",
		EventLabel:    "/features",
		EventValue:    `{"duration": 30}`,
		Timestamp:     base.Add(90 * time.Second),
	}
	require.NoError(t, s.applyPageDurationFromInteraction(ix2))

	var totalDuration int
	require.NoError(t, db.GetGorm().Raw(
		`SELECT total_duration FROM session_summaries WHERE session_id = ?`, "sess-dur",
	).Scan(&totalDuration).Error)
	require.Equal(t, 75, totalDuration, "page_duration events must accumulate on session total_duration")

	// Platform average session time must be non-zero once sessions have dwell.
	summary, err := s.GetAnalyticsSummary(base.Add(-time.Hour), base.Add(time.Hour))
	require.NoError(t, err)
	require.Greater(t, summary.AverageSessionTime, 0.0,
		"AverageSessionTime must roll up page_duration into a non-zero average")
	require.InDelta(t, 75.0, summary.AverageSessionTime, 1e-6)
}

func TestPageDurationIgnoresNonDurationEvents(t *testing.T) {
	db := setupSessionSummaryTestDB(t, nil)
	s := NewPageAnalyticsService(db)
	base := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	require.NoError(t, s.updateSessionSummary(mkPageView("sess-click", "/home", base, 0, "desktop", "US")))

	ix := &models.UserInteraction{
		SessionID:  "sess-click",
		Page:       "/home",
		EventType:  "click",
		EventValue: `{"duration": 99}`,
		Timestamp:  base,
	}
	require.NoError(t, s.applyPageDurationFromInteraction(ix))

	var totalDuration int
	require.NoError(t, db.GetGorm().Raw(
		`SELECT total_duration FROM session_summaries WHERE session_id = ?`, "sess-click",
	).Scan(&totalDuration).Error)
	require.Equal(t, 0, totalDuration)
}
