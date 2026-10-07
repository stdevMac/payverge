package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type validatePromoRequest struct {
	Code string `json:"code" binding:"required"`
}

// ValidatePromoCode validates a guest-supplied promo code against active offers
// for the business that owns the table identified by the :code route param.
func ValidatePromoCode(c *gin.Context) {
	tableCode := c.Param("code")

	var req validatePromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	db := database.GetDB()

	var table database.Table
	if err := db.Where("table_code = ? AND is_active = ?", tableCode, true).First(&table).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	// Match sibling guest routes: don't serve promo lookups on a suspended or closed business.
	business, err := database.GetBusinessByID(table.BusinessID)
	if err != nil || business == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}
	if !database.IsBusinessOperational(business) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting orders",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	now := time.Now().UTC()
	code := strings.TrimSpace(strings.ToUpper(req.Code))

	var offer database.Offer
	result := db.Where(
		"business_id = ? AND UPPER(code) = ? AND is_active = ? AND (start_date IS NULL OR start_date <= ?) AND (end_date IS NULL OR end_date >= ?)",
		table.BusinessID, code, true, now, now,
	).First(&offer)

	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Invalid or expired code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"offer": gin.H{
			"id":             offer.ID,
			"name":           offer.Name,
			"discount_type":  offer.DiscountType,
			"discount_value": offer.DiscountValue,
			"applicable_to":  offer.ApplicableTo,
			"target_id":      offer.TargetID,
		},
	})
}
