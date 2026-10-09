package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
)

// GetAdminEscalationDetail GET /api/v1/admin/escalations/:id
func GetAdminEscalationDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	row, err := database.GetEscalationByID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not load escalation"})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"escalation": row})
}

// PatchAdminEscalation PATCH /api/v1/admin/escalations/:id
func PatchAdminEscalation(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Status     *string `json:"status"`
		AdminNotes *string `json:"admin_notes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if body.Status == nil && body.AdminNotes == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status or admin_notes required"})
		return
	}
	row, err := database.UpdateEscalationAdmin(uint(id), body.Status, body.AdminNotes)
	if err != nil {
		if errors.Is(err, database.ErrInvalidEscalationStatus) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not update escalation"})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"escalation": row})
}

// GetAdminOpsThreadMessages GET /api/v1/admin/ops-threads/:business_id/:thread_id/messages
func GetAdminOpsThreadMessages(c *gin.Context) {
	businessID, err := strconv.ParseUint(c.Param("business_id"), 10, 64)
	if err != nil || businessID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid business_id"})
		return
	}
	threadID, err := strconv.ParseUint(c.Param("thread_id"), 10, 64)
	if err != nil || threadID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid thread_id"})
		return
	}
	msgs, err := database.ListOpsAssistantMessages(uint(businessID), uint(threadID), 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load transcript"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"messages":    msgs,
		"business_id": businessID,
		"thread_id":   threadID,
	})
}
