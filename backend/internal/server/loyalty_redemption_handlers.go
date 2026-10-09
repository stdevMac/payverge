package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/loyalty"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/services/paidreceipt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type redeemPointsRequest struct {
	Points int `json:"points" binding:"required,min=1"`
}

// enqueueLoyaltyPaidReceipt handles the unusual paid transition where an
// already-partially-paid bill becomes fully covered because loyalty lowers its
// total. Receipt failures remain non-blocking, matching every money-settlement
// path, and the receipt domain service deduplicates by bill identity.
func enqueueLoyaltyPaidReceipt(db *gorm.DB, billID uint) {
	if db == nil || billID == 0 {
		return
	}
	var bill database.Bill
	if err := db.First(&bill, billID).Error; err != nil {
		logger.Logger.Warnf("load loyalty-settled bill %d for receipt: %v", billID, err)
		return
	}
	if bill.Status != database.BillStatusPaid {
		return
	}
	if _, err := paidreceipt.NewService(db).HandleBillPaid(
		context.Background(), &bill, "loyalty_redemption",
	); err != nil {
		logger.Logger.Warnf("enqueue loyalty-settled receipt for bill %d: %v", billID, err)
	}
}

// RedeemLoyaltyPoints applies a loyalty-point redemption to the open bill for
// the table identified by the :code route param. The caller must supply a valid
// customer JWT (CustomerAuthenticationMiddleware must run before this handler).
func RedeemLoyaltyPoints(c *gin.Context) {
	customerID, ok := contextUintFromValue(c.MustGet("customer_id"))
	if !ok || customerID == 0 {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid customer session")
		return
	}

	tableCode := c.Param("code")

	var req redeemPointsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	db := database.GetDB()

	// Resolve table → business
	var table database.Table
	if err := db.Select("id", "business_id", "is_active").
		Where("table_code = ? AND is_active = ?", tableCode, true).
		First(&table).Error; err != nil {
		RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Table not found")
		return
	}

	// Match sibling guest routes (GetLoyaltyRate, orders, promos): no mutations
	// on a suspended or closed business.
	business, err := database.GetBusinessByID(table.BusinessID)
	if err != nil || business == nil {
		RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Business not found")
		return
	}
	if !database.IsBusinessOperational(business) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting orders",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	// Find the open/partial bill for this table
	bill, err := database.GetOpenBillSummaryByTableID(table.ID)
	if err != nil {
		if errors.Is(err, database.ErrNoActiveBill) {
			RespondWithError(c, http.StatusNotFound, ErrCodeNoActiveBill, "No open bill found for this table")
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeServerError, "Failed to load bill")
		return
	}

	discountCents, pointsDeducted, remainingPoints, err := loyalty.RedeemPoints(db, customerID, bill.ID, table.BusinessID, req.Points)
	if err != nil {
		status, code, message := mapLoyaltyRedeemError(err)
		RespondWithError(c, status, code, message)
		return
	}
	enqueueLoyaltyPaidReceipt(db, bill.ID)

	// The redemption changed the bill's total — tell operator dashboards so
	// they refetch immediately. The summary row loaded above predates the
	// discount write, so publish ids only; no stale amounts in the payload.
	events.GetHub().PublishJSON(table.BusinessID, "bill.updated", gin.H{
		"bill_id": bill.ID,
		"source":  "loyalty_redemption",
	})

	rate := loyalty.RedemptionRateForBusiness(db, table.BusinessID)
	c.JSON(http.StatusOK, gin.H{
		"discount_cents":   discountCents,
		"discount":         float64(discountCents) / 100.0,
		"points_deducted":  pointsDeducted,
		"remaining_points": remainingPoints,
		// The rate is per unit of the VENUE's currency, which currency names.
		"points_per_currency_unit": rate,
		"currency":                 resolveBusinessCurrency(business),
	})
}

// GetLoyaltyRate returns the current redemption rate (points required for one
// unit of the business's own currency) for the business that owns the table.
// Loyalty earn and burn both run on business-currency amounts — nothing
// converts to USD — so on an ARS carta the rate is points per ARS 1.
// points_per_currency_unit + currency name what the number means (#895).
// The value comes from LoyaltyProgram.RedemptionPointsPerDollar, not the earn
// rate. Public endpoint — no auth required — so the guest UI can show an
// accurate discount estimate before the customer logs in.
func GetLoyaltyRate(c *gin.Context) {
	tableCode := c.Param("code")

	// Same cached, projected table+business load as sibling guest routes.
	// Do not call GetBusinessByID here: that is a SELECT * of the ~140-column
	// businesses row for lifecycle fields already on publicBusinessColumns.
	table, business, err := loadPublicGuestTableContext(tableCode)
	if err != nil {
		respondPublicGuestTableLookupError(c, err)
		return
	}
	if !database.IsBusinessOperational(business) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting orders",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	rate := loyalty.RedemptionRateForBusiness(database.GetDB(), table.BusinessID)
	c.JSON(http.StatusOK, gin.H{
		"points_per_currency_unit": rate,
		"currency":                 resolveBusinessCurrency(business),
	})
}

// UndoLoyaltyRedemption reverses a previously applied loyalty redemption on
// the open bill for the table identified by the :code route param.
func UndoLoyaltyRedemption(c *gin.Context) {
	customerID, ok := contextUintFromValue(c.MustGet("customer_id"))
	if !ok || customerID == 0 {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid customer session")
		return
	}

	tableCode := c.Param("code")

	db := database.GetDB()

	// Resolve table → business
	var table database.Table
	if err := db.Select("id", "business_id", "is_active").
		Where("table_code = ? AND is_active = ?", tableCode, true).
		First(&table).Error; err != nil {
		RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Table not found")
		return
	}

	// Match sibling guest routes: no mutations on a suspended or closed business.
	business, err := database.GetBusinessByID(table.BusinessID)
	if err != nil || business == nil {
		RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Business not found")
		return
	}
	if !database.IsBusinessOperational(business) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting orders",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	// Find the open/partial bill for this table
	bill, err := database.GetOpenBillSummaryByTableID(table.ID)
	if err != nil {
		if errors.Is(err, database.ErrNoActiveBill) {
			RespondWithError(c, http.StatusNotFound, ErrCodeNoActiveBill, "No open bill found for this table")
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeServerError, "Failed to load bill")
		return
	}

	if err := loyalty.UndoRedemption(db, customerID, bill.ID, table.BusinessID); err != nil {
		status, code, message := mapLoyaltyUndoError(err)
		RespondWithError(c, status, code, message)
		return
	}

	// The undo restored the bill's gross total — tell operator dashboards.
	events.GetHub().PublishJSON(table.BusinessID, "bill.updated", gin.H{
		"bill_id": bill.ID,
		"source":  "loyalty_redemption_undo",
	})

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetLoyaltyPointsEarned returns the points the authenticated customer earned
// on one settled bill plus their current balance at that business. Read-only
// companion to redeem/undo above; requires CustomerAuthenticationMiddleware.
// GET /api/v1/guest/table/:code/points-earned?bill_number=...
func GetLoyaltyPointsEarned(c *gin.Context) {
	customerID, ok := contextUintFromValue(c.MustGet("customer_id"))
	if !ok || customerID == 0 {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid customer session")
		return
	}

	tableCode := c.Param("code")
	billNumber := strings.TrimSpace(c.Query("bill_number"))
	if billNumber == "" {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "bill_number is required")
		return
	}

	db := database.GetDB()

	var table database.Table
	if err := db.Select("id", "business_id").
		Where("table_code = ? AND is_active = ?", tableCode, true).
		First(&table).Error; err != nil {
		RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Table not found")
		return
	}

	var bill database.Bill
	if err := db.Select("id").
		Where("bill_number = ? AND business_id = ?", billNumber, table.BusinessID).
		First(&bill).Error; err != nil {
		RespondWithError(c, http.StatusNotFound, ErrCodeNotFound, "Bill not found")
		return
	}

	var connection database.CustomerBusiness
	if err := db.Where("customer_id = ? AND business_id = ? AND is_active = ?", customerID, table.BusinessID, true).
		First(&connection).Error; err != nil {
		RespondWithError(c, http.StatusNotFound, ErrCodeNoLoyaltyConnection, "No loyalty membership at this business")
		return
	}

	var visit database.CustomerVisit
	if err := db.Where("customer_business_id = ? AND bill_id = ?", connection.ID, bill.ID).
		First(&visit).Error; err != nil {
		// The settlement visit is written after payment settles (payments.go →
		// RecordBillSettlementVisit); the guest UI polls briefly, so "not yet"
		// is a plain 404 with a stable code — not an error condition.
		RespondWithError(c, http.StatusNotFound, ErrCodeVisitNotRecorded, "Visit not recorded yet")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"points_earned": visit.PointsEarned,
		"total_points":  connection.LoyaltyPoints,
	})
}

func mapLoyaltyRedeemError(err error) (int, string, string) {
	switch {
	case errors.Is(err, loyalty.ErrInsufficientPoints):
		return http.StatusUnprocessableEntity, ErrCodeInsufficientPoints, err.Error()
	case errors.Is(err, gorm.ErrRecordNotFound):
		return http.StatusNotFound, ErrCodeBusinessNotFound, "Customer or bill not found"
	case errors.Is(err, loyalty.ErrDiscountAlreadyApplied):
		return http.StatusConflict, ErrCodeLoyaltyAlreadyApplied, err.Error()
	case errors.Is(err, loyalty.ErrNotConnected):
		return http.StatusNotFound, ErrCodeNoLoyaltyConnection, err.Error()
	case errors.Is(err, loyalty.ErrProgramNotEnabled):
		return http.StatusBadRequest, ErrCodeLoyaltyNotEnabled, err.Error()
	case errors.Is(err, loyalty.ErrRateNotConfigured):
		return http.StatusBadRequest, ErrCodeLoyaltyRateNotConfigured, err.Error()
	case errors.Is(err, loyalty.ErrPointsTooSmall), errors.Is(err, loyalty.ErrPointsMustBePositive):
		return http.StatusBadRequest, ErrCodeLoyaltyPointsTooSmall, err.Error()
	case errors.Is(err, loyalty.ErrBillNotOpen):
		return http.StatusNotFound, ErrCodeNoActiveBill, err.Error()
	default:
		// Do not collapse leftover redeem failures onto VALIDATION_INVALID_INPUT —
		// that catalogs as "Please check the form" and there is no form.
		return http.StatusBadRequest, "GENERIC_ERROR", "Could not apply loyalty discount"
	}
}

func mapLoyaltyUndoError(err error) (int, string, string) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return http.StatusNotFound, ErrCodeBusinessNotFound, "Bill or customer not found"
	case errors.Is(err, loyalty.ErrUndoNotOwner):
		return http.StatusForbidden, ErrCodeLoyaltyUndoNotOwner, err.Error()
	case errors.Is(err, loyalty.ErrNoDiscountToUndo):
		return http.StatusBadRequest, ErrCodeNoLoyaltyDiscount, err.Error()
	case errors.Is(err, loyalty.ErrNotConnected):
		return http.StatusNotFound, ErrCodeNoLoyaltyConnection, err.Error()
	case errors.Is(err, loyalty.ErrBillNotOpen):
		return http.StatusNotFound, ErrCodeNoActiveBill, err.Error()
	default:
		return http.StatusBadRequest, "GENERIC_ERROR", "Could not undo loyalty discount"
	}
}
