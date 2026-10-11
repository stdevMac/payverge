package runtimecontrol

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(c *gin.Context) {
	controls, err := h.service.ListControls(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "RUNTIME_CONTROL_UNAVAILABLE", "error": "Runtime controls are unavailable."})
		return
	}
	c.JSON(http.StatusOK, gin.H{"controls": controls})
}

func (h *Handler) Update(c *gin.Context) {
	var req struct {
		Enabled   bool      `json:"enabled"`
		Owner     string    `json:"owner" binding:"required"`
		Reason    string    `json:"reason" binding:"required"`
		ExpiresAt time.Time `json:"expires_at" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "VALIDATION_INVALID_INPUT", "error": "Owner, reason, and expiry are required."})
		return
	}
	err := h.service.SetControl(c.Request.Context(), SetControlInput{Key: ControlKey(c.Param("key")), Enabled: req.Enabled, Owner: req.Owner, Reason: req.Reason, ExpiresAt: req.ExpiresAt, Actor: actorFromContext(c)})
	if errors.Is(err, ErrInvalidControlChange) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "VALIDATION_INVALID_INPUT", "error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "RUNTIME_CONTROL_UNAVAILABLE", "error": "Runtime control could not be changed."})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) CreateInviteBatch(c *gin.Context) {
	var req struct {
		Name      string    `json:"name" binding:"required"`
		CohortCap int       `json:"cohort_cap" binding:"required,min=1"`
		Owner     string    `json:"owner" binding:"required"`
		Reason    string    `json:"reason" binding:"required"`
		ExpiresAt time.Time `json:"expires_at" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "VALIDATION_INVALID_INPUT", "error": "Name, positive cohort cap, owner, reason, and expiry are required."})
		return
	}
	batch, code, err := h.service.CreateInviteBatch(c.Request.Context(), CreateInviteBatchInput{Name: req.Name, CohortCap: req.CohortCap, Owner: req.Owner, Reason: req.Reason, ExpiresAt: req.ExpiresAt, Actor: actorFromContext(c)})
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, ErrInvalidControlChange) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"code": "RUNTIME_INVITE_CREATE_FAILED", "error": "Invite batch could not be created."})
		return
	}
	// The plaintext code is returned once and never persisted.
	c.JSON(http.StatusCreated, gin.H{"batch": batch, "invite_code": code})
}

func (h *Handler) Audit(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	events, err := h.service.ListAuditEvents(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "RUNTIME_CONTROL_UNAVAILABLE", "error": "Runtime control audit is unavailable."})
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": events})
}

func actorFromContext(c *gin.Context) string {
	if address := strings.TrimSpace(c.GetString("address")); address != "" {
		return address
	}
	if id, ok := c.Get("user_id"); ok {
		return fmt.Sprint(id)
	}
	return "authenticated-admin"
}
