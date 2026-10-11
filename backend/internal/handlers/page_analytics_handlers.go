package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/models"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type PageAnalyticsHandler struct {
	service *analytics.PageAnalyticsService
}

func NewPageAnalyticsHandler(service *analytics.PageAnalyticsService) *PageAnalyticsHandler {
	return &PageAnalyticsHandler{service: service}
}

// clampPageViewStrings bounds caller-supplied string fields on the public,
// unauthenticated page-view ingest to prevent storage/DoS abuse — mirroring the
// field-length limits the sibling public ingests (error logs, missing
// translations) already enforce. `truncate` is rune-safe, so multibyte values
// (page URLs, referrers, city names, user-agents) are never corrupted. Numeric
// fields are bounded by their type.
func clampPageViewStrings(pv *models.PageView) {
	pv.SessionID = truncate(pv.SessionID, 100)
	pv.Page = truncate(pv.Page, 1024)
	pv.Referrer = truncate(pv.Referrer, 1024)
	pv.UserAgent = truncate(pv.UserAgent, 512)
	pv.Country = truncate(pv.Country, 100)
	pv.City = truncate(pv.City, 200)
	pv.DeviceType = truncate(pv.DeviceType, 50)
	pv.Browser = truncate(pv.Browser, 100)
	pv.OS = truncate(pv.OS, 100)
	pv.Locale = truncate(pv.Locale, 32)
}

// clampInteractionStrings bounds caller-supplied string fields on the public
// interaction ingest (see clampPageViewStrings).
func clampInteractionStrings(in *models.UserInteraction) {
	in.SessionID = truncate(in.SessionID, 100)
	in.Page = truncate(in.Page, 1024)
	in.EventType = truncate(in.EventType, 100)
	in.EventCategory = truncate(in.EventCategory, 100)
	in.EventLabel = truncate(in.EventLabel, 255)
	in.EventValue = truncate(in.EventValue, 2048)
}

// clampConversionStrings bounds caller-supplied string fields on the public
// conversion ingest (see clampPageViewStrings).
func clampConversionStrings(ev *models.ConversionEvent) {
	ev.SessionID = truncate(ev.SessionID, 100)
	ev.ConversionType = truncate(ev.ConversionType, 100)
	ev.Metadata = truncate(ev.Metadata, 2048)
}

// countryFromCDNHeader accepts a CDN country header (Cloudflare CF-IPCountry).
// It returns the code only when it is exactly two ASCII letters and is not
// Cloudflare's unknown (XX) or Tor (T1) sentinel.
func countryFromCDNHeader(raw string) string {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if len(code) != 2 || code == "XX" || code == "T1" {
		return ""
	}
	if code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
		return ""
	}
	return code
}

// TrackPageView handles page view tracking
func (h *PageAnalyticsHandler) TrackPageView(c *gin.Context) {
	var pv models.PageView
	if err := c.ShouldBindJSON(&pv); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Bound attacker-controlled string fields before any persistence (public,
	// unauthenticated endpoint — see clampPageViewStrings).
	clampPageViewStrings(&pv)

	// Set timestamp if not provided
	if pv.Timestamp.IsZero() {
		pv.Timestamp = time.Now()
	}

	// Always derive IP address from the request — never trust a
	// body-supplied value. A malicious client could otherwise spoof
	// someone else's IP (or a private range) and poison analytics.
	// The address is stored and never logged or sent to a third party.
	pv.IPAddress = c.ClientIP()

	// Country comes from the CDN header when the client did not send one.
	// City stays whatever the client sent (already clamped).
	if pv.Country == "" {
		pv.Country = countryFromCDNHeader(c.GetHeader("CF-IPCountry"))
	}

	if err := h.service.TrackPageView(&pv); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to track page view")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "success"})
}

// TrackInteraction handles user interaction tracking
func (h *PageAnalyticsHandler) TrackInteraction(c *gin.Context) {
	var interaction models.UserInteraction
	if err := c.ShouldBindJSON(&interaction); err != nil {
		server.RespondBindError(c, err)
		return
	}

	clampInteractionStrings(&interaction)

	// Set timestamp if not provided
	if interaction.Timestamp.IsZero() {
		interaction.Timestamp = time.Now()
	}

	if err := h.service.TrackInteraction(&interaction); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to track interaction")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "success"})
}

// TrackConversion handles conversion event tracking
func (h *PageAnalyticsHandler) TrackConversion(c *gin.Context) {
	var conversion models.ConversionEvent
	if err := c.ShouldBindJSON(&conversion); err != nil {
		server.RespondBindError(c, err)
		return
	}

	clampConversionStrings(&conversion)

	// Set timestamp if not provided
	if conversion.Timestamp.IsZero() {
		conversion.Timestamp = time.Now()
	}

	if err := h.service.TrackConversion(&conversion); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to track conversion")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "success"})
}

// GetAnalyticsSummary returns aggregated analytics data
func (h *PageAnalyticsHandler) GetAnalyticsSummary(c *gin.Context) {
	// Parse date range from query parameters
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	var startDate, endDate time.Time
	var err error

	if startDateStr != "" {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid start_date format")
			return
		}
	} else {
		// Default to last 30 days
		startDate = time.Now().AddDate(0, 0, -30)
	}

	if endDateStr != "" {
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid end_date format")
			return
		}
	} else {
		endDate = time.Now()
	}

	summary, err := h.service.GetAnalyticsSummary(startDate, endDate)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get analytics summary")
		return
	}

	c.JSON(http.StatusOK, summary)
}

// GetRecentSessions returns recent session data
func (h *PageAnalyticsHandler) GetRecentSessions(c *gin.Context) {
	// Clamp to avoid a caller requesting millions of session rows.
	limit := ClampLimit(c.Query("limit"), 100, 500)

	sessions, err := h.service.GetRecentSessions(limit)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get recent sessions")
		return
	}

	c.JSON(http.StatusOK, sessions)
}
