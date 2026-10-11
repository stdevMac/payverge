package server

import (
	"net/http"
	"strconv"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
)

func adminPageLimit(c *gin.Context) (limit, offset int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return limit, (page - 1) * limit
}

// GetAdminEscalations GET /api/v1/admin/escalations — paginated, newest first.
func GetAdminEscalations(c *gin.Context) {
	limit, offset := adminPageLimit(c)
	rows, total, err := database.ListEscalations(limit, offset, c.Query("status"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not list escalations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"escalations": rows, "total": total})
}
