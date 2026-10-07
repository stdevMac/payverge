package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	demosvc "github.com/stdevmac/payverge/backend/internal/demo"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type AdminDemoService interface {
	SummaryForAdmin(ctx context.Context, adminUserID uint, ensure bool) (*demosvc.Summary, error)
	ResetForAdmin(ctx context.Context, adminUserID uint) (*database.DemoInstance, error)
	AppendDueDaysForAdmin(ctx context.Context, adminUserID uint) error
	VerifyForAdmin(ctx context.Context, adminUserID uint) (demosvc.VerificationResult, error)
}

type AdminDemoHandler struct {
	service AdminDemoService
}

func NewAdminDemoHandler(service AdminDemoService) *AdminDemoHandler {
	return &AdminDemoHandler{service: service}
}

func (h *AdminDemoHandler) Get(c *gin.Context) {
	h.respondSummary(c, true)
}

func (h *AdminDemoHandler) Ensure(c *gin.Context) {
	h.respondSummary(c, true)
}

func (h *AdminDemoHandler) Reset(c *gin.Context) {
	adminID, ok := adminDemoUserID(c)
	if !ok {
		return
	}
	if _, err := h.service.ResetForAdmin(c.Request.Context(), adminID); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to reset demo")
		return
	}
	h.respondSummaryForAdmin(c, adminID, false)
}

func (h *AdminDemoHandler) AppendDay(c *gin.Context) {
	adminID, ok := adminDemoUserID(c)
	if !ok {
		return
	}
	if err := h.service.AppendDueDaysForAdmin(c.Request.Context(), adminID); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to append demo day")
		return
	}
	h.respondSummaryForAdmin(c, adminID, false)
}

func (h *AdminDemoHandler) Verify(c *gin.Context) {
	adminID, ok := adminDemoUserID(c)
	if !ok {
		return
	}
	result, err := h.service.VerifyForAdmin(c.Request.Context(), adminID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to verify demo")
		return
	}
	c.JSON(http.StatusOK, gin.H{"verification": result})
}

func (h *AdminDemoHandler) respondSummary(c *gin.Context, ensure bool) {
	adminID, ok := adminDemoUserID(c)
	if !ok {
		return
	}
	h.respondSummaryForAdmin(c, adminID, ensure)
}

func (h *AdminDemoHandler) respondSummaryForAdmin(c *gin.Context, adminID uint, ensure bool) {
	summary, err := h.service.SummaryForAdmin(c.Request.Context(), adminID, ensure)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load demo")
		return
	}
	c.JSON(http.StatusOK, summary)
}

func adminDemoUserID(c *gin.Context) (uint, bool) {
	adminID, err := getAdminUserID(c)
	if err != nil {
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeNotAuthenticated, "Admin identity not available")
		return 0, false
	}
	return adminID, true
}
