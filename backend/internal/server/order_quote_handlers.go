package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type orderQuoteRequest struct {
	Items     []guestOrderItemRequest `json:"items" binding:"required"`
	PromoCode string                  `json:"promo_code"`
}

type orderQuoteLineResponse struct {
	Key          string                 `json:"key"`
	LineType     string                 `json:"line_type"`
	UnitPrice    float64                `json:"unit_price"`
	Quantity     int                    `json:"quantity"`
	Subtotal     float64                `json:"subtotal"`
	Orderability *services.Orderability `json:"orderability,omitempty"`
}

type orderQuoteResponse struct {
	Subtotal    float64                  `json:"subtotal"`
	Discount    float64                  `json:"discount"`
	NetSubtotal float64                  `json:"net_subtotal"`
	Tax         float64                  `json:"tax"`
	ServiceFee  float64                  `json:"service_fee"`
	Tip         float64                  `json:"tip"`
	Total       float64                  `json:"total"`
	Lines       []orderQuoteLineResponse `json:"lines"`
}

func quoteInputLines(items []guestOrderItemRequest) []services.PromotionInputLine {
	result := make([]services.PromotionInputLine, 0, len(items))
	for _, item := range items {
		result = append(result, services.PromotionInputLine{
			Name: item.MenuItemName, MenuItemID: item.MenuItemID, Quantity: item.Quantity,
			UnitPrice: item.Price, Options: item.Options, SpecialRequests: item.SpecialRequests,
			ItemType: item.ItemType, BundleID: item.BundleID, ParentBundleID: item.ParentBundleID,
			SourceOfferID: item.SourceOfferID,
		})
	}
	return result
}

func serializeOrderQuote(projection services.OrderQuoteProjection) orderQuoteResponse {
	netSubtotalCents := projection.Quote.NetSubtotalCents
	if netSubtotalCents == 0 && projection.Quote.TotalCents != 0 {
		netSubtotalCents = projection.Quote.TotalCents
	}
	finalTotalCents := projection.Quote.FinalTotalCents
	if finalTotalCents == 0 {
		finalTotalCents = netSubtotalCents + projection.Quote.TaxCents +
			projection.Quote.ServiceFeeCents + projection.Quote.TipCents
	}
	response := orderQuoteResponse{
		Subtotal:    float64(projection.Quote.SubtotalCents) / 100,
		Discount:    float64(projection.Quote.DiscountCents) / 100,
		NetSubtotal: float64(netSubtotalCents) / 100,
		Tax:         float64(projection.Quote.TaxCents) / 100,
		ServiceFee:  float64(projection.Quote.ServiceFeeCents) / 100,
		Tip:         float64(projection.Quote.TipCents) / 100,
		Total:       float64(finalTotalCents) / 100,
		Lines:       make([]orderQuoteLineResponse, 0, len(projection.Quote.Lines)),
	}
	for _, line := range projection.Quote.Lines {
		serialized := orderQuoteLineResponse{
			Key: line.Key, LineType: line.LineType, UnitPrice: float64(line.UnitPriceCents) / 100,
			Quantity: line.Quantity, Subtotal: float64(line.SubtotalCents) / 100,
		}
		if decision, ok := projection.Orderability[line.Key]; ok {
			copy := decision
			serialized.Orderability = &copy
		}
		response.Lines = append(response.Lines, serialized)
	}
	return response
}

func respondOrderQuoteError(c *gin.Context, err error) {
	var unavailable *services.ItemNotOrderableError
	if errors.As(err, &unavailable) {
		c.JSON(http.StatusConflict, gin.H{
			"code":    "item_not_orderable",
			"error":   "One or more items are unavailable",
			"message": "One or more items are unavailable",
			"details": gin.H{"items": unavailable.Items},
		})
		return
	}
	var validation *services.OrderValidationError
	if errors.As(err, &validation) {
		RespondWithError(c, http.StatusBadRequest, validation.Code, validation.Message)
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found") {
		RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Business or table not found")
		return
	}
	RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, err.Error())
}

// QuoteGuestOrder prices a table-code order without creating any records.
func QuoteGuestOrder(c *gin.Context) {
	_, business, err := loadPublicGuestTableContext(c.Param("code"))
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
	if !guestOrderingEnabled(business) {
		respondGuestOrderingDisabled(c)
		return
	}

	var req orderQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	projection, err := services.NewGuestCheckoutService(database.GetDB()).QuoteForTable(
		c.Request.Context(), c.Param("code"), quoteInputLines(req.Items), req.PromoCode,
	)
	if err != nil {
		respondOrderQuoteError(c, err)
		return
	}
	c.JSON(http.StatusOK, serializeOrderQuote(projection))
}

// QuoteBusinessOrder is the staff/operator counterpart to QuoteGuestOrder.
func QuoteBusinessOrder(c *gin.Context) {
	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || businessID == 0 {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Invalid business ID")
		return
	}
	var req orderQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	projection, err := services.NewGuestCheckoutService(database.GetDB()).QuoteForBusiness(
		c.Request.Context(), uint(businessID), quoteInputLines(req.Items), req.PromoCode,
	)
	if err != nil {
		respondOrderQuoteError(c, err)
		return
	}
	c.JSON(http.StatusOK, serializeOrderQuote(projection))
}
