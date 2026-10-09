package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetAdminFiscalSummary GET /api/v1/admin/fiscal/summary
func GetAdminFiscalSummary(c *gin.Context) {
	summary, err := database.GetAdminFiscalSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load fiscal summary"})
		return
	}
	c.JSON(http.StatusOK, summary)
}

// GetAdminFiscalJobs GET /api/v1/admin/fiscal/jobs
// Query: status, kind=real|demo|test|all (default real — Task 15 production queue).
func GetAdminFiscalJobs(c *gin.Context) {
	limit, offset := adminPageLimit(c)
	status := c.Query("status")
	kind := c.Query("kind")
	rows, total, err := database.ListAdminFiscalJobsWithKind(limit, offset, status, kind)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list fiscal jobs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": rows, "total": total})
}

// GetAdminFiscalReceipts GET /api/v1/admin/fiscal/receipts
// Query: status, kind=real|demo|test|all (default real).
func GetAdminFiscalReceipts(c *gin.Context) {
	limit, offset := adminPageLimit(c)
	status := c.Query("status")
	kind := c.Query("kind")
	rows, total, err := database.ListAdminFiscalReceiptsWithKind(limit, offset, status, kind)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list fiscal receipts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"receipts": rows, "total": total})
}

// PostAdminRequeueFiscalJob POST /api/v1/admin/fiscal/jobs/:id/requeue
// Body is optional: {"allow_permanent": bool}. An empty body is allowed.
func PostAdminRequeueFiscalJob(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	allowPermanent, err := readRequeueAllowPermanent(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	job, err := database.AdminRequeueFiscalJob(uint(id), allowPermanent)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if errors.Is(err, database.ErrFiscalJobNotRequeueable) {
			c.JSON(http.StatusConflict, gin.H{"error": "job cannot be requeued in its current state"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to requeue job"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"job": job})
}

// readRequeueAllowPermanent reads the optional requeue body.
// An empty body means allow_permanent is false. Malformed JSON is an error.
func readRequeueAllowPermanent(c *gin.Context) (bool, error) {
	if c.Request.Body == nil {
		return false, nil
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return false, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return false, nil
	}
	var body struct {
		AllowPermanent bool `json:"allow_permanent"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return false, err
	}
	return body.AllowPermanent, nil
}
