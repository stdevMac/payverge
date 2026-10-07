package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
)

type OperationalAlertHandlers struct {
	db  *gorm.DB
	svc *alerts.Service
}

func NewOperationalAlertHandlers(db *gorm.DB) *OperationalAlertHandlers {
	return &OperationalAlertHandlers{db: db, svc: alerts.NewService(db)}
}

func (h *OperationalAlertHandlers) List(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	statuses := parseAlertStatuses(c.Query("status"))
	types := parseAlertTypes(c.Query("types"))
	var items []database.OperationalAlert
	var err error
	if c.Query("recent") == "1" {
		since := time.Now().AddDate(0, 0, -7)
		items, err = h.svc.ListAlerts(c.Request.Context(), businessID, alerts.ListOptions{
			Statuses: statuses,
			Types:    types,
			Since:    &since,
			Limit:    100,
		})
	} else {
		items, err = h.svc.ListActiveAlerts(c.Request.Context(), businessID, statuses, types)
	}
	if err != nil {
		// FIND-060: never surface GORM/driver text to operators.
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not list alerts")
		return
	}
	c.JSON(http.StatusOK, gin.H{"alerts": items})
}

type claimAlertRequest struct {
	Source  string `json:"source"`
	StaffID *uint  `json:"staff_id"`
}

func (h *OperationalAlertHandlers) Claim(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	alertID, ok := alertIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alert id"})
		return
	}
	var req claimAlertRequest
	_ = c.ShouldBindJSON(&req)
	actor := h.actorFromContext(c)
	if req.StaffID != nil && *req.StaffID > 0 {
		routed, err := h.actorFromStaffID(businessID, *req.StaffID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid staff id"})
			return
		}
		actor = routed
	}
	alert, err := h.svc.ClaimAlertForBusiness(c.Request.Context(), businessID, alertID, actor, defaultAlertSource(req.Source))
	if err != nil {
		writeAlertError(c, err)
		return
	}
	c.JSON(http.StatusOK, alert)
}

type resolveAlertRequest struct {
	Reason string `json:"reason"`
}

func (h *OperationalAlertHandlers) Resolve(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	alertID, ok := alertIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alert id"})
		return
	}
	var req resolveAlertRequest
	_ = c.ShouldBindJSON(&req)
	alert, err := h.svc.ResolveAlertByIDForBusiness(c.Request.Context(), businessID, alertID, h.actorFromContext(c), strings.TrimSpace(req.Reason))
	if err != nil {
		writeAlertError(c, err)
		return
	}
	c.JSON(http.StatusOK, alert)
}

type snoozeAlertRequest struct {
	Until string `json:"until"`
}

func (h *OperationalAlertHandlers) Snooze(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	alertID, ok := alertIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alert id"})
		return
	}
	var req snoozeAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(req.Until))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid until"})
		return
	}
	alert, err := h.svc.SnoozeAlertForBusiness(c.Request.Context(), businessID, alertID, h.actorFromContext(c), until)
	if err != nil {
		writeAlertError(c, err)
		return
	}
	c.JSON(http.StatusOK, alert)
}

func (h *OperationalAlertHandlers) Events(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	alertID, ok := alertIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alert id"})
		return
	}
	items, err := h.svc.GetAlertEventsForBusiness(c.Request.Context(), businessID, alertID)
	if err != nil {
		writeAlertError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": items})
}

func (h *OperationalAlertHandlers) GetSettings(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	settings, err := h.svc.GetSettings(c.Request.Context(), businessID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not load alert settings")
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *OperationalAlertHandlers) UpdateSettings(c *gin.Context) {
	businessID, ok := businessIDFromCtx(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business id"})
		return
	}
	var req database.BusinessAlertSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	req.BusinessID = businessID
	settings, err := h.svc.UpdateSettings(c.Request.Context(), businessID, req, h.actorFromContext(c))
	if err != nil {
		writeAlertError(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *OperationalAlertHandlers) actorFromStaffID(businessID, staffID uint) (alerts.Actor, error) {
	var staff database.Staff
	err := h.db.Select("id", "name", "email").
		Where("id = ? AND business_id = ? AND is_active = ?", staffID, businessID, true).
		First(&staff).Error
	if err != nil {
		return alerts.Actor{}, err
	}
	name := strings.TrimSpace(staff.Name)
	if name == "" {
		name = strings.TrimSpace(staff.Email)
	}
	if name == "" {
		name = "staff:" + strconv.FormatUint(uint64(staff.ID), 10)
	}
	return alerts.Actor{StaffID: &staff.ID, Name: name}, nil
}

func (h *OperationalAlertHandlers) actorFromContext(c *gin.Context) alerts.Actor {
	if id, ok := contextUint(c, "staff_id"); ok && id > 0 {
		var staff database.Staff
		if err := h.db.Select("id", "name", "email").First(&staff, id).Error; err == nil {
			name := strings.TrimSpace(staff.Name)
			if name == "" {
				name = strings.TrimSpace(staff.Email)
			}
			return alerts.Actor{StaffID: &staff.ID, Name: name}
		}
		return alerts.Actor{StaffID: &id, Name: "staff:" + strconv.FormatUint(uint64(id), 10)}
	}
	if id, ok := contextUint(c, "user_id"); ok && id > 0 {
		name := "user:" + strconv.FormatUint(uint64(id), 10)
		if email, exists := c.Get("email"); exists {
			if emailStr, ok := email.(string); ok && strings.TrimSpace(emailStr) != "" {
				name = strings.TrimSpace(emailStr)
			}
		}
		return alerts.Actor{UserID: &id, Name: name}
	}
	if addr, exists := c.Get("address"); exists {
		if wallet, ok := addr.(string); ok && strings.TrimSpace(wallet) != "" {
			return alerts.Actor{Name: strings.TrimSpace(wallet)}
		}
	}
	return alerts.Actor{Name: "system"}
}

func alertIDFromCtx(c *gin.Context) (uint, bool) {
	raw := c.Param("alertId")
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint(n), true
}

func parseAlertStatuses(raw string) []database.OperationalAlertStatus {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]database.OperationalAlertStatus, 0, len(parts))
	for _, part := range parts {
		value := database.OperationalAlertStatus(strings.TrimSpace(part))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func parseAlertTypes(raw string) []database.OperationalAlertType {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]database.OperationalAlertType, 0, len(parts))
	for _, part := range parts {
		value := database.OperationalAlertType(strings.TrimSpace(part))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func defaultAlertSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "manual"
	}
	return source
}

func writeAlertError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, alerts.ErrAlertNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "alert not found"})
	case errors.Is(err, alerts.ErrAlertConflict):
		// State conflict (claim steal, claim/snooze of a resolved alert, …):
		// 409 with the current holder so the FE can show "already claimed by X".
		body := gin.H{"error": "alert state conflict"}
		var conflict *alerts.AlertConflictError
		if errors.As(err, &conflict) {
			body["status"] = conflict.Alert.Status
			body["claimed_by_name"] = conflict.Alert.ClaimedByName
		}
		c.JSON(http.StatusConflict, body)
	case errors.Is(err, alerts.ErrInvalidAlertInput), errors.Is(err, alerts.ErrInvalidAlertSetting):
		// Domain validation sentinels are product-safe.
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not update alert")
	}
}

// contextUint reads a numeric gin context value (JWT claims arrive as float64).
func contextUint(c *gin.Context, key string) (uint, bool) {
	value, exists := c.Get(key)
	if !exists {
		return 0, false
	}

	switch v := value.(type) {
	case float64:
		return uint(v), true
	case float32:
		return uint(v), true
	case uint:
		return v, true
	case uint64:
		return uint(v), true
	case int:
		return uint(v), true
	case int32:
		return uint(v), true
	case int64:
		return uint(v), true
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(v), 10, 32)
		if err != nil {
			return 0, false
		}
		return uint(parsed), true
	default:
		return 0, false
	}
}
