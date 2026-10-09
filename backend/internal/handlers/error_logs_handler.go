package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type ErrorLogHandler struct{}

func NewErrorLogHandler() *ErrorLogHandler {
	return &ErrorLogHandler{}
}

// truncate returns s bounded to at most maxLen runes. It truncates on rune
// boundaries (never splitting a multibyte UTF-8 sequence) so the result is
// always valid UTF-8 — important because these bounded values are written to
// Postgres text columns, which reject invalid UTF-8 byte sequences (a mid-rune
// byte cut would otherwise 500 the INSERT for long multibyte input on these
// public, attacker-controlled ingest endpoints).
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen { // byte length ≤ maxLen ⇒ rune length ≤ maxLen
		return s
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen])
}

const (
	// MaxErrorLogBodyBytes bounds the whole public POST /logs/error body.
	MaxErrorLogBodyBytes int64 = 16 << 10
	// maxErrorLogAdditionalInfoBytes bounds the serialized additionalInfo
	// object that is persisted to error_logs (M-logs).
	maxErrorLogAdditionalInfoBytes = 8 << 10
	// ErrorLogSourceClientUntrusted marks rows ingested from the public
	// client endpoint. Callers must treat the message body as untrusted input.
	ErrorLogSourceClientUntrusted = "client_untrusted"
)

// boundAdditionalInfo returns info unchanged when its JSON encoding fits in
// maxErrorLogAdditionalInfoBytes, and otherwise a small marker object so an
// unauthenticated caller cannot persist arbitrarily large blobs.
func boundAdditionalInfo(info map[string]interface{}) map[string]interface{} {
	if len(info) == 0 {
		return info
	}
	encoded, err := json.Marshal(info)
	if err != nil {
		return map[string]interface{}{"truncated": true, "reason": "unencodable"}
	}
	if len(encoded) <= maxErrorLogAdditionalInfoBytes {
		return info
	}
	return map[string]interface{}{"truncated": true, "original_bytes": len(encoded)}
}

// IngestError handles POST /api/v1/logs/error from the frontend
func (h *ErrorLogHandler) IngestError(c *gin.Context) {
	if c.Request.ContentLength > MaxErrorLogBodyBytes {
		server.RespondWithError(c, http.StatusRequestEntityTooLarge, "", "error log payload too large")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxErrorLogBodyBytes)

	var payload struct {
		Timestamp      string                 `json:"timestamp"`
		Error          string                 `json:"error"`
		Component      string                 `json:"component"`
		Function       string                 `json:"function"`
		AdditionalInfo map[string]interface{} `json:"additionalInfo"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Enforce field-length limits to prevent abuse
	payload.Error = truncate(payload.Error, 2000)
	payload.Component = truncate(payload.Component, 200)
	payload.Function = truncate(payload.Function, 200)
	payload.AdditionalInfo = boundAdditionalInfo(payload.AdditionalInfo)

	// Parse the timestamp
	timestamp, err := time.Parse(time.RFC3339, payload.Timestamp)
	if err != nil {
		timestamp = time.Now()
	}

	// The client cannot choose the request id. Only the server middleware value is stored.
	requestID := truncate(c.GetString("request_id"), 100)

	logEntry := &database.ErrorLog{
		Timestamp:      timestamp,
		Source:         ErrorLogSourceClientUntrusted,
		Component:      payload.Component,
		Function:       payload.Function,
		Error:          payload.Error,
		Message:        payload.Error,
		RequestID:      requestID,
		AdditionalInfo: payload.AdditionalInfo,
	}

	if err := database.StoreErrorLog(c.Request.Context(), logEntry); err != nil {
		logger.Logger.Warnf("Failed to store error log: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "failed to store error log")
		return
	}

	logger.Logger.WithField("component", payload.Component).
		WithField("function", payload.Function).
		WithField("request_id", requestID).
		Infof("Frontend error logged: %s", payload.Error)

	c.JSON(http.StatusOK, gin.H{"status": "logged"})
}

// ListErrors handles GET /api/v1/admin/errors
// TODO(lane7): troubleshoot skill must label error logs as untrusted client input
func (h *ErrorLogHandler) ListErrors(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}
	source := c.Query("source")
	component := c.Query("component")

	// The MCP admin token must not receive rows the public client wrote.
	excludeSource := ""
	if c.GetString("token_type") == "admin_mcp" {
		excludeSource = ErrorLogSourceClientUntrusted
	}

	logs, total, err := database.GetErrorLogsPaginated(c.Request.Context(), limit, offset, source, component, excludeSource)
	if err != nil {
		logger.Logger.Warnf("Failed to retrieve error logs: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "failed to retrieve error logs")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"errors": logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}
