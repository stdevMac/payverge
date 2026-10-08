package models

import "time"

// PageView represents a page view event
type PageView struct {
	ID           int64     `json:"id" db:"id"`
	SessionID    string    `json:"session_id" db:"session_id"`
	Page         string    `json:"page" db:"page"`
	Referrer     string    `json:"referrer" db:"referrer"`
	UserAgent    string    `json:"user_agent" db:"user_agent"`
	IPAddress    string    `json:"ip_address" db:"ip_address"`
	Country      string    `json:"country" db:"country"`
	City         string    `json:"city" db:"city"`
	DeviceType   string    `json:"device_type" db:"device_type"`
	Browser      string    `json:"browser" db:"browser"`
	OS           string    `json:"os" db:"os"`
	ScreenWidth  int       `json:"screen_width" db:"screen_width"`
	ScreenHeight int       `json:"screen_height" db:"screen_height"`
	Locale       string    `json:"locale" db:"locale"`
	Timestamp    time.Time `json:"timestamp" db:"timestamp"`
	Duration     int       `json:"duration" db:"duration"` // Time spent on page in seconds
}

// UserInteraction represents a user interaction event
type UserInteraction struct {
	ID            int64     `json:"id" db:"id"`
	SessionID     string    `json:"session_id" db:"session_id"`
	Page          string    `json:"page" db:"page"`
	EventType     string    `json:"event_type" db:"event_type"`         // click, scroll, hover, form_submit, etc.
	EventCategory string    `json:"event_category" db:"event_category"` // button, link, form, section, etc.
	EventLabel    string    `json:"event_label" db:"event_label"`       // Specific element identifier
	EventValue    string    `json:"event_value" db:"event_value"`       // Additional data (JSON)
	XPosition     int       `json:"x_position" db:"x_position"`
	YPosition     int       `json:"y_position" db:"y_position"`
	Timestamp     time.Time `json:"timestamp" db:"timestamp"`
}

// ConversionEvent represents a conversion/goal completion
type ConversionEvent struct {
	ID             int64     `json:"id" db:"id"`
	SessionID      string    `json:"session_id" db:"session_id"`
	ConversionType string    `json:"conversion_type" db:"conversion_type"` // signup, business_registration, waitlist, etc.
	Value          float64   `json:"value" db:"value"`                     // Monetary value if applicable
	Metadata       string    `json:"metadata" db:"metadata"`               // Additional data (JSON)
	Timestamp      time.Time `json:"timestamp" db:"timestamp"`
}

// SessionSummary represents aggregated session data
type SessionSummary struct {
	SessionID         string    `json:"session_id" db:"session_id"`
	FirstSeen         time.Time `json:"first_seen" db:"first_seen"`
	LastSeen          time.Time `json:"last_seen" db:"last_seen"`
	TotalPageViews    int       `json:"total_page_views" db:"total_page_views"`
	TotalInteractions int       `json:"total_interactions" db:"total_interactions"`
	TotalDuration     int       `json:"total_duration" db:"total_duration"` // Total time in seconds
	PagesVisited      string    `json:"pages_visited" db:"pages_visited"`   // JSON array of pages
	Converted         bool      `json:"converted" db:"converted"`
	ConversionType    string    `json:"conversion_type" db:"conversion_type"`
	DeviceType        string    `json:"device_type" db:"device_type"`
	Country           string    `json:"country" db:"country"`
}

// AnalyticsSummary represents aggregated analytics data
type AnalyticsSummary struct {
	TotalPageViews     int64              `json:"total_page_views"`
	TotalSessions      int64              `json:"total_sessions"`
	TotalInteractions  int64              `json:"total_interactions"`
	TotalConversions   int64              `json:"total_conversions"`
	AverageSessionTime float64            `json:"average_session_time"`
	BounceRate         float64            `json:"bounce_rate"`
	ConversionRate     float64            `json:"conversion_rate"`
	TopPages           []PageStats        `json:"top_pages"`
	TopInteractions    []InteractionStats `json:"top_interactions"`
	DeviceBreakdown    map[string]int64   `json:"device_breakdown"`
	CountryBreakdown   map[string]int64   `json:"country_breakdown"`
	HourlyTraffic      []HourlyStats      `json:"hourly_traffic"`
	ConversionFunnel   []FunnelStep       `json:"conversion_funnel"`
}

// PageStats represents statistics for a specific page
type PageStats struct {
	Page            string  `json:"page"`
	Views           int64   `json:"views"`
	UniqueVisitors  int64   `json:"unique_visitors"`
	AverageDuration float64 `json:"average_duration"`
	BounceRate      float64 `json:"bounce_rate"`
}

// InteractionStats represents statistics for a specific interaction
type InteractionStats struct {
	EventType      string `json:"event_type"`
	EventCategory  string `json:"event_category"`
	EventLabel     string `json:"event_label"`
	Count          int64  `json:"count"`
	UniqueSessions int64  `json:"unique_sessions"`
}

// HourlyStats represents traffic statistics by hour
type HourlyStats struct {
	Hour         int   `json:"hour"`
	PageViews    int64 `json:"page_views"`
	Sessions     int64 `json:"sessions"`
	Interactions int64 `json:"interactions"`
}

// FunnelStep is independent route-reach for one marketing path.
// DropoffRate is nil when the previous step has zero sessions or this
// step has more sessions than the previous (not a sequential subset).
type FunnelStep struct {
	Step        string   `json:"step"`
	Sessions    int64    `json:"sessions"`
	DropoffRate *float64 `json:"dropoff_rate"`
}
