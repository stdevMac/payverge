package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// MissingTranslationHandler ingests the IMP-31 sendBeacon telemetry posted by
// the guest- and operator-tier translation providers when a key falls through
// the localised manifest and lands on the English fallback or the
// sentence-cased leaf rendering.
//
// The endpoint is fire-and-forget: the frontend uses navigator.sendBeacon and
// cannot read the response, so we always return 200 with a small JSON ack and
// log soft failures rather than 5xx-ing.
type MissingTranslationHandler struct{}

func NewMissingTranslationHandler() *MissingTranslationHandler {
	return &MissingTranslationHandler{}
}

// fallback values accepted on the wire; anything else is normalised to
// "leaf" so the column never holds a free-text bogus value.
const (
	fallbackLayerEnglish = "english"
	fallbackLayerLeaf    = "leaf"
)

// IngestMissing handles POST /api/v1/analytics/missing_translation. Body
// fields mirror the frontend reporter's payload — see
// frontend/src/i18n/missingTranslationReporter.ts.
func (h *MissingTranslationHandler) IngestMissing(c *gin.Context) {
	var payload struct {
		Page         string `json:"page"`
		Locale       string `json:"locale"`
		Key          string `json:"key"`
		FallbackUsed string `json:"fallback_used"`
		Timestamp    string `json:"timestamp"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		server.RespondBindError(c, err)
		return
	}

	if payload.Locale == "" || payload.Key == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "locale and key are required")
		return
	}

	// Reject locales that are not part of the canonical registry. Without this
	// gate a typo'd or spoofed locale ("klingon") would be stored as a
	// free-text value and pollute the admin missing-translations surface.
	if !locales.IsSupported(payload.Locale) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "unknown locale")
		return
	}

	if payload.FallbackUsed != fallbackLayerEnglish && payload.FallbackUsed != fallbackLayerLeaf {
		payload.FallbackUsed = fallbackLayerLeaf
	}

	entry := &database.MissingTranslation{
		Locale:       truncate(payload.Locale, 32),
		KeyPath:      truncate(payload.Key, 255),
		Page:         truncate(payload.Page, 500),
		FallbackUsed: payload.FallbackUsed,
	}

	if err := database.RecordMissingTranslation(c.Request.Context(), entry); err != nil {
		logger.Logger.Warnf("[missing_translation] record failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"status": "dropped"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "logged"})
}

// ListMissing handles GET /api/v1/admin/analytics/missing-translations. Admin
// surface to scan which keys are falling through.
func (h *MissingTranslationHandler) ListMissing(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}
	// Validate the optional ?locale filter against the registry; an unknown
	// value is ignored (treated as "no filter") rather than forwarded to the
	// query as an unvalidated string.
	locale := c.Query("locale")
	if locale != "" && !locales.IsSupported(locale) {
		locale = ""
	}
	status := c.Query("status")
	if !database.IsValidMissingTranslationStatus(status) {
		status = ""
	}

	rows, total, err := database.ListMissingTranslations(c.Request.Context(), limit, offset, locale, status)
	if err != nil {
		logger.Logger.Warnf("[missing_translation] list failed: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "failed to list missing translations")
		return
	}

	responseRows := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		responseRows = append(responseRows, gin.H{
			"id":                row.ID,
			"locale":            row.Locale,
			"key_path":          row.KeyPath,
			"page":              row.Page,
			"fallback_used":     row.FallbackUsed,
			"hit_count":         row.OccurrenceCount,
			"occurrence_count":  row.OccurrenceCount,
			"first_seen_at":     row.FirstSeenAt,
			"last_seen_at":      row.LastSeenAt,
			"status":            row.Status,
			"status_updated_at": row.StatusUpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"rows":   responseRows,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// UpdateStatus handles PATCH
// /api/v1/admin/analytics/missing-translations/:id/status.
func (h *MissingTranslationHandler) UpdateStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "invalid missing translation id")
		return
	}

	var payload struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil || !database.IsValidMissingTranslationStatus(payload.Status) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "status must be open, resolved, or ignored")
		return
	}

	row, err := database.UpdateMissingTranslationStatus(c.Request.Context(), uint(id), payload.Status)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "missing translation not found")
		return
	}
	if err != nil {
		logger.Logger.Warnf("[missing_translation] status update failed: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "failed to update missing translation")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                row.ID,
		"status":            row.Status,
		"status_updated_at": row.StatusUpdatedAt,
	})
}
