package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
)

// guestOrderView is the safe subset of database.Order exposed on the
// unauthenticated guest-orders endpoint. Actor fields (created_by,
// approved_by, cancelled_by) expose staff email/wallet PII and are
// stripped. Currency is not a column on orders; it is resolved via
// Order.ResolvedCurrency() from the narrow Business currency preload
// (display → default → USD), matching Order.MarshalJSON on the operator
// path. The Business relation itself is never serialized. (ORD-BILL-2, #560)
type guestOrderView struct {
	ID           uint                 `json:"id"`
	BillID       uint                 `json:"bill_id"`
	BusinessID   uint                 `json:"business_id"`
	OrderNumber  string               `json:"order_number"`
	Status       database.OrderStatus `json:"status"`
	Currency     string               `json:"currency"`
	Notes        string               `json:"notes"`
	Items        string               `json:"items"`
	CancelReason string               `json:"cancel_reason"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	ApprovedAt   *time.Time           `json:"approved_at"`
	CancelledAt  *time.Time           `json:"cancelled_at"`
}

// guestOrdersResponse is the projection-safe version of OrdersResponse.
type guestOrdersResponse struct {
	Orders []guestOrderView `json:"orders"`
	Total  int64            `json:"total"`
}

func newGuestOrderView(o database.Order) guestOrderView {
	return guestOrderView{
		ID:           o.ID,
		BillID:       o.BillID,
		BusinessID:   o.BusinessID,
		OrderNumber:  o.OrderNumber,
		Status:       o.Status,
		Currency:     o.ResolvedCurrency(),
		Notes:        o.Notes,
		Items:        o.Items,
		CancelReason: o.CancelReason,
		CreatedAt:    o.CreatedAt,
		UpdatedAt:    o.UpdatedAt,
		ApprovedAt:   o.ApprovedAt,
		CancelledAt:  o.CancelledAt,
	}
}

// GetGuestOrdersByBillNumber retrieves orders for a specific bill (public endpoint for guests).
// The route param is the public_token capability (:bill_token).
func GetGuestOrdersByBillNumber(c *gin.Context) {
	token := strings.TrimSpace(c.Param("bill_token"))
	if token == "" {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	billID, err := database.GetGuestBillIDByToken(token)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	orders, err := database.GetOrdersByBillID(billID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to retrieve orders")
		return
	}

	views := make([]guestOrderView, 0, len(orders))
	for _, o := range orders {
		views = append(views, newGuestOrderView(o))
	}

	c.JSON(http.StatusOK, guestOrdersResponse{
		Orders: views,
		Total:  int64(len(views)),
	})
}
