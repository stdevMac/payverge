package server

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/services"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"

	"github.com/gin-gonic/gin"
)

// cancelOrderRequest is the JSON body for operator cancel.
type cancelOrderRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// guestCancelOrderRequest is the JSON body for guest cancel (reason optional).
type guestCancelOrderRequest struct {
	Reason string `json:"reason"`
}

// cancelableByOperator lists statuses an operator may cancel from.
// Served/delivered and already-cancelled orders are terminal.
var cancelableByOperator = map[database.OrderStatus]bool{
	database.OrderStatusPending:    true,
	database.OrderStatusApproved:   true,
	database.OrderStatusInKitchen:  true,
	database.OrderStatusOrderReady: true,
}

// CancelOrder handles PATCH /inside/businesses/:id/orders/:orderId/cancel
// Operators (owner or staff with orders:status) may cancel any non-terminal order.
func CancelOrder(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	orderIDStr := c.Param("orderId")
	orderID, err := strconv.ParseUint(orderIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	var req cancelOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
		return
	}

	// Verify the order exists and belongs to this business.
	order, _, err := database.GetOrderByID(uint(orderID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	}
	if order.BusinessID != business.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	}

	// Validate the current status allows cancellation.
	if !cancelableByOperator[order.Status] {
		switch order.Status {
		case database.OrderStatusOrderCancelled:
			c.JSON(http.StatusConflict, gin.H{"error": "Order is already cancelled"})
		case database.OrderStatusOrderDelivered:
			c.JSON(http.StatusConflict, gin.H{"error": "Cannot cancel a delivered order"})
		default:
			c.JSON(http.StatusConflict, gin.H{"error": "Order cannot be cancelled in its current state"})
		}
		return
	}

	actor := resolveCancelActor(c)

	// Delivery-linked orders must cancel through the delivery lifecycle so the
	// delivery leg goes terminal, the driver is released, the unpaid bill is
	// closed, and the guest is emailed (B-2/X-3). Mirrors the PUT /status
	// handler's delivery branch (handlers/orders.go).
	if deliverySvc := GetDeliveryService(); deliverySvc != nil {
		delivery, derr := deliverySvc.DeliveryByOrderID(business.ID, uint(orderID))
		if derr == nil && delivery != nil {
			if rerr := deliverySvc.RejectDeliveryByOrder(business.ID, uint(orderID), actor, req.Reason); rerr != nil {
				// Log the raw error server-side; respond with a safe mapped
				// message via the shared chokepoint (MapDeliveryActionError) so
				// no DB/SQL detail leaks. Same mapper as handlers.UpdateOrderStatus.
				log.Printf("CancelOrder delivery reject failed: business_id=%d order_id=%d actor=%s error=%v", business.ID, orderID, actor, rerr)
				status, code, msg := MapDeliveryActionError(rerr)
				RespondWithError(c, status, code, msg)
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"message":  "Order cancelled",
				"order_id": uint(orderID),
			})
			return
		} else if derr != nil && !errors.Is(derr, services.ErrDeliveryOrderNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check delivery linkage"})
			return
		}
	}

	if err := database.UpdateOrderStatus(uint(orderID), database.OrderStatusOrderCancelled, actor, req.Reason); err != nil {
		log.Printf("CancelOrder failed: business_id=%d order_id=%d actor=%s error=%v", business.ID, orderID, actor, err)
		if errors.Is(err, database.ErrDeliveryLinkedCancel) {
			c.JSON(http.StatusConflict, gin.H{"error": "This order is part of a delivery and must be cancelled from the delivery queue"})
			return
		}
		if errors.Is(err, database.ErrInvalidStatusTransition) {
			c.JSON(http.StatusConflict, gin.H{"error": "Order cannot be cancelled in its current state"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel order"})
		return
	}

	// B-5: resolve the needs-approval operational alert raised at create time.
	alertSvc := operational_alerts.NewService(database.GetDB())
	if err := alertSvc.ResolveAlertForResource(c.Request.Context(), business.ID,
		database.OperationalAlertResourceTypeOrder, uint(orderID),
		operational_alerts.Actor{Name: actor}, string(database.OrderStatusOrderCancelled)); err != nil {
		log.Printf("failed to resolve order operational alert on cancel: business_id=%d order_id=%d error=%v",
			business.ID, orderID, err)
	}

	publishOrderCancelledEvent(business.ID, uint(orderID), order.BillID, req.Reason, actor)

	c.JSON(http.StatusOK, gin.H{
		"message":  "Order cancelled",
		"order_id": uint(orderID),
	})
}

func respondGuestCancelConflict(c *gin.Context, status database.OrderStatus) {
	if status == database.OrderStatusOrderCancelled {
		c.JSON(http.StatusConflict, gin.H{
			"error": "This order has already been cancelled",
			"code":  "order_already_cancelled",
		})
		return
	}
	c.JSON(http.StatusConflict, gin.H{
		"error": "This order has already been accepted by the kitchen",
		"code":  "order_already_accepted",
	})
}

// GuestCancelOrder handles POST /guest/table/:code/orders/:orderId/cancel
// Guests may only cancel while the order is still pending (before kitchen approval).
func GuestCancelOrder(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, business, err := loadPublicGuestTableContext(code)
	if err != nil {
		respondPublicGuestTableLookupError(c, err)
		return
	}

	// Match sibling guest routes (e.g. order creation): a suspended or closed
	// business must not keep serving guest actions off a scraped QR code.
	if !database.IsBusinessOperational(business) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting orders",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	orderIDStr := c.Param("orderId")
	orderID, err := strconv.ParseUint(orderIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	var req guestCancelOrderRequest
	// Reason is optional for guests — ignore bind errors.
	_ = c.ShouldBindJSON(&req)

	// Verify the order belongs to this table's business.
	order, _, err := database.GetOrderByID(uint(orderID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	}
	if order.BusinessID != table.BusinessID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	}
	if order.Bill.TableID != table.ID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	}

	// Guests may only cancel pending orders.
	if order.Status != database.OrderStatusPending {
		respondGuestCancelConflict(c, order.Status)
		return
	}

	now := time.Now().UTC()

	// Atomic conditional cancel + bill-history event in one tx (B-5/X-5);
	// stamps cancelled_by = "guest" without an authenticated actor string.
	updated, err := database.CancelPendingGuestOrder(order, req.Reason, now)
	if err != nil {
		log.Printf("GuestCancelOrder failed: table_code=%s order_id=%d error=%v", code, orderID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel order"})
		return
	}
	if !updated {
		if refreshed, _, refreshErr := database.GetOrderByID(uint(orderID)); refreshErr == nil && refreshed != nil {
			respondGuestCancelConflict(c, refreshed.Status)
			return
		}
		respondGuestCancelConflict(c, database.OrderStatusApproved)
		return
	}

	// B-5: resolve the needs-approval operational alert raised at create time.
	alertSvc := operational_alerts.NewService(database.GetDB())
	if err := alertSvc.ResolveAlertForResource(c.Request.Context(), table.BusinessID,
		database.OperationalAlertResourceTypeOrder, uint(orderID),
		operational_alerts.Actor{Name: "guest"}, string(database.OrderStatusOrderCancelled)); err != nil {
		log.Printf("failed to resolve order operational alert on guest cancel: business_id=%d order_id=%d error=%v",
			table.BusinessID, orderID, err)
	}

	publishOrderCancelledEvent(table.BusinessID, uint(orderID), order.BillID, req.Reason, "guest")

	c.JSON(http.StatusOK, gin.H{
		"message":  "Order cancelled",
		"order_id": uint(orderID),
	})
}

// resolveCancelActor derives an actor identifier from the Gin context for
// operator cancel actions, preferring staff ID over owner address.
func resolveCancelActor(c *gin.Context) string {
	if staffID, exists := c.Get("staff_id"); exists {
		if uid := toUint(staffID); uid > 0 {
			return "staff:" + strconv.FormatUint(uint64(uid), 10)
		}
	}
	if address, exists := c.Get("address"); exists {
		if addr, ok := address.(string); ok && addr != "" {
			return "owner:" + addr
		}
	}
	if userID, exists := c.Get("user_id"); exists {
		if uid := toUint(userID); uid > 0 {
			return "user:" + strconv.FormatUint(uint64(uid), 10)
		}
	}
	return "system"
}

func toUint(v interface{}) uint {
	switch n := v.(type) {
	case uint:
		return n
	case float64:
		return uint(n)
	case int:
		return uint(n)
	case int64:
		return uint(n)
	default:
		return 0
	}
}

// publishOrderCancelledEvent emits an SSE order.cancelled event to the
// business channel. It is a no-op if the hub is unavailable.
//
// X-7: the payload carries BOTH `order_id` and `id` (plus bill_id/status/
// reason/cancelled_by) so it matches the delivery lifecycle's emitter
// additively — subscribers keying on either shape see every cancellation.
func publishOrderCancelledEvent(businessID, orderID, billID uint, reason, cancelledBy string) {
	events.GetHub().PublishJSON(businessID, "order.cancelled", gin.H{
		"id":           orderID,
		"order_id":     orderID,
		"bill_id":      billID,
		"status":       "cancelled",
		"reason":       reason,
		"cancelled_by": cancelledBy,
	})
}
