package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	requestmiddleware "github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
	print "github.com/stdevmac/payverge/backend/internal/services/print"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// errOrderBillNotOpen is returned when the locked bill is not open or partial
// at insert time. The pre-check can race a close.
var errOrderBillNotOpen = errors.New("bill is not open for new orders")

// CreateOrderRequest represents the request to create a new order
type CreateOrderRequest struct {
	BillID uint                     `json:"bill_id" binding:"required"`
	Notes  string                   `json:"notes"`
	Items  []CreateOrderItemRequest `json:"items" binding:"required"`
}

// CreateOrderItemRequest represents individual items in the order
type CreateOrderItemRequest struct {
	MenuItemName    string                    `json:"menu_item_name" binding:"required"`
	MenuItemID      string                    `json:"menu_item_id"`
	Quantity        int                       `json:"quantity" binding:"required"`
	Price           float64                   `json:"price" binding:"gte=0"`
	Options         []database.MenuItemOption `json:"options"` // Add-ons/modifiers
	SpecialRequests string                    `json:"special_requests"`
	ItemType        string                    `json:"item_type"`
	BundleID        *uint                     `json:"bundle_id"`
	ParentBundleID  *uint                     `json:"parent_bundle_id"`
	SourceOfferID   *uint                     `json:"source_offer_id"`
}

// sharedPrintService lets the order-status handler enqueue kitchen tickets at
// approve time (X-2). Set once at startup via SetSharedPrintService; nil in
// deployments/tests that never wire printing.
var sharedPrintService *print.Service

// SetSharedPrintService wires the shared print service. Called from main.go.
func SetSharedPrintService(svc *print.Service) { sharedPrintService = svc }

// enqueueKitchenTicketForApprovedOrder fires the approve-time kitchen ticket
// (X-2, the missing IMP-04 caller): only for printer-enabled businesses, only
// once per order. Failures log and fall through to the orphan-sweep retry net.
func enqueueKitchenTicketForApprovedOrder(ctx context.Context, order *database.Order) {
	if sharedPrintService == nil || order == nil {
		return
	}
	db := database.GetDB()
	if !print.BusinessHasEnabledKitchenPrinter(db, order.BusinessID) {
		return
	}
	if print.OrderHasKitchenJob(db, order.ID) {
		return
	}
	orderID := order.ID
	if _, err := sharedPrintService.Enqueue(ctx, print.EnqueueParams{
		BusinessID: order.BusinessID,
		Kind:       database.PrintJobKindKitchen,
		SourceType: "order",
		SourceID:   orderID,
		OrderID:    &orderID,
		Language:   print.BusinessPrintLanguage(db, order.BusinessID),
		CreatedBy:  "order_approve",
	}); err != nil {
		log.Printf("kitchen ticket enqueue failed: business_id=%d order_id=%d error=%v", order.BusinessID, order.ID, err)
	}
}

// OrderHandler handles order-related requests
type OrderHandler struct {
	// WebSocket removed - using polling instead
}

// NewOrderHandler creates a new order handler
func NewOrderHandler(_ interface{}) *OrderHandler {
	return &OrderHandler{}
}

// UpdateOrderStatusRequest represents the request to update order status
type UpdateOrderStatusRequest struct {
	Status database.OrderStatus `json:"status" binding:"required"`
	Reason string               `json:"reason"`
}

// OrderResponse represents the response for orders
type OrderResponse struct {
	Order database.Order `json:"order"`
}

// OrdersResponse represents the response for multiple orders
type OrdersResponse struct {
	Orders []database.Order `json:"orders"`
	Total  int64            `json:"total"`
}

// PaginatedOrdersResponse includes pagination metadata.
type PaginatedOrdersResponse struct {
	Orders     []database.Order `json:"orders"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalPages int              `json:"total_pages"`
}

// parsePagination extracts page/page_size from query params.
func parsePagination(c *gin.Context) database.PaginationParams {
	p := database.DefaultPagination()
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			p.Page = n
		}
	}
	if v := c.Query("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			p.PageSize = n
		}
	}
	return p.Normalize()
}

// staffSafeOrderStatus maps UpdateOrderStatus failures to a safe HTTP status,
// stable error code (for FE localization), and English log/API message.
func staffSafeOrderStatus(err error) (status int, code string, message string) {
	if err == nil {
		return http.StatusInternalServerError, "", "Failed to update order status"
	}
	if errors.Is(err, database.ErrInvalidStatusTransition) {
		return http.StatusConflict, server.ErrCodeConflict, "Failed to update order status. This status change is not allowed."
	}
	if errors.Is(err, database.ErrDeliveryLinkedCancel) {
		return http.StatusConflict, server.ErrCodeConflict, "Failed to update order status. This order is part of a delivery — cancel it from the delivery queue."
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "inventory hard block"):
		return http.StatusConflict, server.ErrCodeInventoryInsufficientStock, "Insufficient stock to approve this order"
	case strings.Contains(lower, "bill is not open"):
		return http.StatusConflict, server.ErrCodeConflict, "Failed to update order status. The bill is no longer open."
	case strings.Contains(lower, "bill") && strings.Contains(lower, "relation"):
		return http.StatusInternalServerError, "", "Failed to update order status. Could not sync bill items. Please refresh and try again."
	case strings.Contains(lower, "bill") && strings.Contains(lower, "update"):
		return http.StatusInternalServerError, "", "Failed to update order status. Could not update bill totals. Please refresh and try again."
	case strings.Contains(lower, "transaction"):
		return http.StatusInternalServerError, "", "Failed to update order status. Transaction was not committed. Please retry."
	default:
		return http.StatusInternalServerError, "", "Failed to update order status. Please retry."
	}
}

// generateOrderNumber generates a unique order number for display
func generateOrderNumber(businessID uint) string {
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("O%d-%d", businessID, timestamp%100000000)
}

func getOrderActor(c *gin.Context) string {
	if staffEmail, exists := c.Get("staff_email"); exists {
		if actor, ok := staffEmail.(string); ok && actor != "" {
			return actor
		}
	}

	if email, exists := c.Get("email"); exists {
		if actor, ok := email.(string); ok && actor != "" {
			return actor
		}
	}

	if address, exists := c.Get("address"); exists {
		if actor, ok := address.(string); ok && actor != "" {
			return actor
		}
	}

	if tokenType, exists := c.Get("token_type"); exists {
		if actor, ok := tokenType.(string); ok && actor != "" {
			return actor
		}
	}

	return ""
}

// operatorOrderRequestID mirrors the guest helper: prefer the request-id the
// middleware validated/stored, fall back to the raw header for routes (and
// tests) without the middleware.
func operatorOrderRequestID(c *gin.Context) string {
	if requestID := strings.TrimSpace(c.GetString(requestmiddleware.RequestIDKey)); requestID != "" {
		return requestID
	}
	return strings.TrimSpace(c.GetHeader(requestmiddleware.RequestIDHeader))
}

func operatorOrderRequestPointer(requestID string) *string {
	trimmed := strings.TrimSpace(requestID)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func operatorOrderIsDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

// CreateOrder creates a new order (for guests)
func (oh *OrderHandler) CreateOrder(c *gin.Context) {
	c.Set("perf_flow", "checkout")
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if utf8.RuneCountInString(req.Notes) > services.MaxOrderTextLen {
		server.RespondWithError(c, http.StatusBadRequest, services.OrderErrCodeTextTooLong,
			fmt.Sprintf("Notes must be %d characters or fewer", services.MaxOrderTextLen))
		return
	}

	db := database.GetDB()
	if db == nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Database connection failed")
		return
	}

	// Verify the bill exists and belongs to this business
	var bill database.Bill
	if err := db.Where("id = ? AND business_id = ?", req.BillID, businessID).First(&bill).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
			return
		}
		log.Printf("CreateOrder bill lookup: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load bill")
		return
	}

	createdBy := getOrderActor(c)
	if createdBy == "" {
		createdBy = "guest"
	}

	// B-6: operator order creation honors the same X-Request-Id dedupe as the
	// guest path. The (bill_id, created_by, client_request_id) unique index
	// turns a staff double-click into a replay instead of a duplicate order.
	requestID := operatorOrderRequestID(c)
	if requestID != "" {
		existing, _, replayErr := database.GetOrderByRequestIdentity(req.BillID, createdBy, requestID)
		switch {
		case replayErr == nil:
			// The bill lookup above proves this request is scoped to the current
			// business. Still reject inconsistent cross-tenant data instead of
			// returning an order outside that scope.
			if existing.BusinessID != uint(businessID) || existing.BillID != req.BillID {
				server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to resolve existing order")
				return
			}
			c.JSON(http.StatusOK, gin.H{"order": existing, "duplicate": true})
			return
		case !errors.Is(replayErr, database.ErrOrderNotFound):
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to resolve existing order")
			return
		}
	}

	if bill.Status != database.BillStatusOpen && bill.Status != database.BillStatusPartial {
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Bill is not open for new orders")
		return
	}

	// Generate order number only after the replay-first path has missed.
	orderNumber := generateOrderNumber(uint(businessID))

	// Create the order
	order := database.Order{
		BillID:          req.BillID,
		BusinessID:      uint(businessID),
		OrderNumber:     orderNumber,
		Status:          database.OrderStatusPending,
		CreatedBy:       createdBy,
		ClientRequestID: operatorOrderRequestPointer(requestID),
		Notes:           req.Notes,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Resolve pricing from server-side menu and promotion data instead of
	// trusting client-submitted prices.
	promotionInput := make([]services.PromotionInputLine, 0, len(req.Items))
	for _, itemReq := range req.Items {
		promotionInput = append(promotionInput, services.PromotionInputLine{
			Name:            itemReq.MenuItemName,
			MenuItemID:      itemReq.MenuItemID,
			Quantity:        itemReq.Quantity,
			UnitPrice:       itemReq.Price,
			Options:         itemReq.Options,
			SpecialRequests: itemReq.SpecialRequests,
			ItemType:        itemReq.ItemType,
			BundleID:        itemReq.BundleID,
			ParentBundleID:  itemReq.ParentBundleID,
			SourceOfferID:   itemReq.SourceOfferID,
		})
	}

	pricedOrder, business, err := services.PriceOrderInputsByBusinessID(uint(businessID), promotionInput)
	if err != nil {
		var ve *services.OrderValidationError
		if errors.As(err, &ve) {
			server.RespondWithError(c, http.StatusBadRequest, ve.Code, ve.Message)
			return
		}
		if errors.Is(err, services.ErrPricingDataUnavailable) {
			log.Printf("CreateOrder pricing: %v", err)
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to price order")
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	_, categories, _, _, err := services.MenuDataForBusiness(uint(businessID))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to validate item availability")
		return
	}
	// Gate the Telegram outbox write before opening the write transaction (a
	// cached eligibility read) so the notification row can be enqueued INSIDE the
	// same transaction as the order — a true transactional outbox (N-1). A crash
	// between the order commit and a separate enqueue can no longer lose it.
	notifyTelegram := services.ShouldEnqueueTelegramNotification(order.BusinessID, services.PluginEventOrderCreated)

	// Create order with items (+ atomic outbox row when Telegram is wired).
	if err := db.Transaction(func(tx *gorm.DB) error {
		var locked database.Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "status").
			Where("id = ? AND business_id = ?", req.BillID, businessID).
			Take(&locked).Error; err != nil {
			return err
		}
		if locked.Status != database.BillStatusOpen && locked.Status != database.BillStatusPartial {
			return errOrderBillNotOpen
		}
		if err := services.ValidateOperatorOrderableLinesTx(tx, business, categories, pricedOrder.Lines); err != nil {
			return err
		}
		if err := database.CreateOrderTx(tx, &order, pricedOrder.Lines); err != nil {
			return err
		}
		if notifyTelegram {
			return enqueueTelegramOrderCreatedNotificationTx(tx, order, bill, business, pricedOrder.Lines, "staff")
		}
		return nil
	}); err != nil {
		if errors.Is(err, errOrderBillNotOpen) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Bill is not open for new orders")
			return
		}
		var unavailable *services.ItemNotOrderableError
		if errors.As(err, &unavailable) {
			server.RespondWithOrderabilityConflict(c, unavailable)
			return
		}
		var projectionErr *services.OperatorOrderabilityProjectionError
		if errors.As(err, &projectionErr) {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to validate item availability")
			return
		}
		if requestID != "" && operatorOrderIsDuplicateError(err) {
			if existing, _, derr := database.GetOrderByRequestIdentity(req.BillID, createdBy, requestID); derr == nil {
				c.JSON(http.StatusOK, gin.H{"order": existing, "duplicate": true})
				return
			}
		}
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to create order")
		return
	}

	// Hydrate associations from data already in memory rather than re-fetching
	// from Postgres. `bill` was loaded above; `business` was loaded by
	// PriceOrderInputsByBusinessID. This eliminates two extra round trips per
	// order create — the dominant allocation hotspot in BenchmarkOrderCreate.
	order.Bill = bill
	if business != nil {
		order.Business = *business
	}

	// Marshal the order once and reuse the bytes across SSE + HTTP response.
	// Previously each of c.JSON / events.PublishJSON ran an independent
	// json.Marshal on the same struct — accounting for ~230 MB cumulative
	// allocations in the v1 perf profile.
	if err := database.HydrateOrderSnapshotForWire(&order); err != nil {
		log.Printf("order.created snapshot hydrate failed: order_id=%d error=%v", order.ID, err)
		order.Items = ""
	}
	orderBytes, err := json.Marshal(order)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to encode order")
		return
	}

	events.GetHub().Publish(events.BusinessEvent{
		BusinessID: order.BusinessID,
		Type:       "order.created",
		Data:       orderBytes,
		Timestamp:  time.Now(),
	})
	if err := operational_alerts.NewService(database.GetDB()).CreateOrderNewAlert(c.Request.Context(), order); err != nil {
		log.Printf("failed to create order operational alert: business_id=%d order_id=%d error=%v", order.BusinessID, order.ID, err)
	}

	c.Status(http.StatusCreated)
	c.Header("Content-Type", "application/json; charset=utf-8")
	_, _ = c.Writer.Write([]byte(`{"order":`))
	_, _ = c.Writer.Write(orderBytes)
	_, _ = c.Writer.Write([]byte(`}`))
}

// enqueueTelegramOrderCreatedNotificationTx writes the order.created outbox row
// on the caller's order-create transaction (N-1 transactional outbox). The
// caller has already confirmed the business wants this notification
// (ShouldEnqueueTelegramNotification) before opening the transaction; the bill
// and business are passed explicitly so the payload never depends on the order's
// (unhydrated, in-transaction) relations. A failed insert rolls back the order,
// which is acceptable: both live in the same DB transaction, so the only
// realistic failure is a DB fault that would fail the order commit regardless.
//
// total_cents is this order's own payable total (items net of discounts, plus
// tax and service fee) — NOT bill.TotalAmount. Unlike the delivery checkout
// builder (delivery_v1.go enqueueDeliveryTelegramOrderCreatedTx), which can
// trust bill.TotalAmount because the delivery bill is created fresh with this
// order's totals already baked in inside the same transaction, `bill` here is
// loaded by CreateOrder BEFORE this transaction opens and is never
// re-synced with the new order's items: order approval — not creation — is
// what folds an order's items into bill.Items/bill.TotalAmount
// (database.updateOrderStatus, only on the OrderStatusApproved transition).
// A brand-new bill's TotalAmount at this point is 0 (or, for a bill with
// prior approved orders, some unrelated smaller total), so echoing it here
// would under-report the new order at least as badly as the items-only bug
// this replaces — and for a fresh bill it would show a flatly wrong "Total:
// $0.00" for an order that has real items. Mirrors the guest dine-in
// reference implementation (guest_checkout.go: quote.FinalTotalCents), which
// prices only the lines in the order just placed.
func enqueueTelegramOrderCreatedNotificationTx(tx *gorm.DB, order database.Order, bill database.Bill, business *database.Business, items []database.OrderItem, source string) error {
	itemCount, subtotalCents := summarizeOrderItemsForNotification(items)
	currency := ""
	var taxRate, serviceFeeRate float64
	if business != nil {
		currency = business.DisplayCurrency
		if currency == "" {
			currency = business.DefaultCurrency
		}
		taxRate = business.TaxRate
		serviceFeeRate = business.ServiceFeeRate
	}
	_, _, totalCents := services.BillTotalsCents(subtotalCents, taxRate, serviceFeeRate)
	payload := map[string]interface{}{
		"order_id":     order.ID,
		"order_number": order.OrderNumber,
		"bill_id":      order.BillID,
		"bill_number":  bill.BillNumber,
		"table_id":     bill.TableID,
		"table_name":   orderNotificationTableName(bill),
		"item_count":   itemCount,
		"total_cents":  totalCents,
		"currency":     currency,
		"notes":        order.Notes,
		"source":       source,
	}

	if _, _, err := services.EnqueuePluginNotificationTx(tx, services.PluginNotificationEvent{
		BusinessID: order.BusinessID,
		EventType:  services.PluginEventOrderCreated,
		EventID:    fmt.Sprintf("order:%d", order.ID),
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}, "telegram"); err != nil {
		return fmt.Errorf("enqueue telegram order notification (business_id=%d order_id=%d): %w", order.BusinessID, order.ID, err)
	}
	return nil
}

// summarizeOrderItemsForNotification returns the item count and the net
// items subtotal in cents (post-discount: PriceOrderInputsByBusinessID
// represents an applied offer as its own line with a negative Subtotal, so a
// plain sum already nets out discounts; bundle-child lines carry Subtotal 0
// by contract, so they cannot double-count against their bundle parent).
// This is NOT the order's payable total — callers must still add tax/service
// fee (see enqueueTelegramOrderCreatedNotificationTx).
func summarizeOrderItemsForNotification(items []database.OrderItem) (int, int64) {
	var itemCount int
	var subtotalCents int64
	for _, item := range items {
		itemCount += item.Quantity
		subtotalCents += int64(math.Round(item.Subtotal * 100))
	}
	return itemCount, subtotalCents
}

func orderNotificationTableName(bill database.Bill) string {
	if strings.TrimSpace(bill.Table.Name) != "" {
		return bill.Table.Name
	}
	if bill.TableID != 0 {
		return fmt.Sprintf("Table %d", bill.TableID)
	}
	return "Unassigned"
}

// GetOrders retrieves orders for a business. Supports pagination via ?page=&page_size= query params.
func GetOrders(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	status := c.Query("status")
	activeBillsOnly := false
	if raw := c.Query("active_bills_only"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid active_bills_only value")
			return
		}
		activeBillsOnly = parsed
	}

	// Optional sort=asc: absent/"desc" keeps the legacy newest-first order.
	// The kitchen board sends "asc" so the row cap drops the newest rather than
	// the oldest FIFO orders.
	sortAsc := false
	if raw := strings.ToLower(strings.TrimSpace(c.Query("sort"))); raw != "" {
		switch raw {
		case "asc":
			sortAsc = true
		case "desc":
			sortAsc = false
		default:
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid sort value")
			return
		}
	}

	result, err := database.GetOrdersByBusinessIDPaginated(uint(businessID), status, parsePagination(c), database.OrderListOptions{
		ActiveBillsOnly: activeBillsOnly,
		SortAsc:         sortAsc,
	})
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to retrieve orders")
		return
	}

	c.JSON(http.StatusOK, PaginatedOrdersResponse{
		Orders:     result.Data,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	})
}

// GetOrder retrieves a specific order
func GetOrder(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	orderIDStr := c.Param("orderId")
	orderID, err := strconv.ParseUint(orderIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid order ID")
		return
	}

	order, items, err := database.GetOrderByID(uint(orderID))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Order not found")
		return
	}

	// Verify order belongs to this business
	if order.BusinessID != uint(businessID) {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Order not found")
		return
	}

	// Add items to response (they're already parsed)
	_ = items // Items are already in the order.Items JSON field

	c.JSON(http.StatusOK, OrderResponse{Order: *order})
}

// UpdateOrderStatus updates the status of an order (for staff approval)
func UpdateOrderStatus(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	orderIDStr := c.Param("orderId")
	orderID, err := strconv.ParseUint(orderIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid order ID")
		return
	}

	var req UpdateOrderStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	if !req.Status.IsValid() {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid order status")
		return
	}

	// Verify order exists and belongs to this business without hydrating the
	// full order aggregate before the mutation transaction reloads it.
	orderBusinessID, err := database.GetOrderBusinessIDByID(uint(orderID))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Order not found")
		return
	}

	if orderBusinessID != uint(businessID) {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Order not found")
		return
	}

	actor := getOrderActor(c)
	if req.Status == database.OrderStatusApproved && actor == "" {
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeTokenInvalid, "Missing authenticated approver")
		return
	}

	// Delivery orders route through the linked lifecycle: approve must not
	// re-append bill items (it would double-bill and erase the delivery fee),
	// and prepay-mode approval waits for guest payment before starting the
	// kitchen.
	if sharedDeliveryService != nil && (req.Status == database.OrderStatusApproved || req.Status == database.OrderStatusOrderCancelled) {
		delivery, derr := sharedDeliveryService.DeliveryByOrderID(uint(businessID), uint(orderID))
		if derr == nil && delivery != nil {
			if req.Status == database.OrderStatusApproved {
				settings, serr := sharedDeliveryService.GetDeliverySettingsDTO(uint(businessID), false)
				if serr != nil {
					server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to load delivery settings")
					return
				}
				result, aerr := sharedDeliveryService.AcceptDeliveryByOrder(uint(businessID), uint(orderID), actor, database.DeliveryPaymentMode(settings.PaymentMode))
				if aerr != nil {
					status, code, msg := server.MapDeliveryActionError(aerr)
					if status == http.StatusInternalServerError {
						log.Printf("AcceptDeliveryByOrder failed: business_id=%d order_id=%d error=%v", businessID, orderID, aerr)
					}
					server.RespondWithError(c, status, code, msg)
					return
				}
				updatedOrder, _, gerr := database.GetOrderByID(uint(orderID))
				if gerr != nil {
					server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to retrieve updated order")
					return
				}
				deliveryAlertSvc := operational_alerts.NewService(database.GetDB())
				deliveryAlertActor := operational_alerts.Actor{Name: actor}
				if err := deliveryAlertSvc.ResolveAlertForResource(c.Request.Context(), updatedOrder.BusinessID, database.OperationalAlertResourceTypeOrder, updatedOrder.ID, deliveryAlertActor, string(updatedOrder.Status)); err != nil {
					log.Printf("failed to resolve order operational alert on delivery approve: business_id=%d order_id=%d error=%v", updatedOrder.BusinessID, updatedOrder.ID, err)
				}
				if !result.AwaitingPayment {
					if err := deliveryAlertSvc.CreateKitchenReadyAlert(c.Request.Context(), *updatedOrder); err != nil {
						log.Printf("failed to create kitchen alert for delivery order %d: %v", updatedOrder.ID, err)
					}
					enqueueKitchenTicketForApprovedOrder(c.Request.Context(), updatedOrder)
				}
				if herr := database.HydrateOrderSnapshotForWire(updatedOrder); herr != nil {
					log.Printf("order.updated snapshot hydrate failed: order_id=%d error=%v", updatedOrder.ID, herr)
					updatedOrder.Items = ""
				}
				events.GetHub().PublishJSON(updatedOrder.BusinessID, "order.updated", updatedOrder)
				c.JSON(http.StatusOK, gin.H{
					"order":            updatedOrder,
					"awaiting_payment": result.AwaitingPayment,
					"payment_mode":     string(result.PaymentMode),
				})
				return
			}
			// Cancel path.
			if rerr := sharedDeliveryService.RejectDeliveryByOrder(uint(businessID), uint(orderID), actor, req.Reason); rerr != nil {
				status, code, msg := server.MapDeliveryActionError(rerr)
				if status == http.StatusInternalServerError {
					log.Printf("RejectDeliveryByOrder failed: business_id=%d order_id=%d error=%v", businessID, orderID, rerr)
				}
				server.RespondWithError(c, status, code, msg)
				return
			}
			updatedOrder, _, gerr := database.GetOrderByID(uint(orderID))
			if gerr != nil {
				server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to retrieve updated order")
				return
			}
			c.JSON(http.StatusOK, OrderResponse{Order: *updatedOrder})
			return
		} else if derr != nil && !errors.Is(derr, services.ErrDeliveryOrderNotFound) {
			server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to check delivery linkage")
			return
		}
	}

	// Update order status
	if err := database.UpdateOrderStatus(uint(orderID), req.Status, actor, req.Reason); err != nil {
		log.Printf("UpdateOrderStatus failed: business_id=%d order_id=%d requested_status=%s error=%v", businessID, orderID, req.Status, err)
		status, code, msg := staffSafeOrderStatus(err)
		server.RespondWithError(c, status, code, msg)
		return
	}

	// Return updated order
	updatedOrder, _, err := database.GetOrderByID(uint(orderID))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to retrieve updated order")
		return
	}

	// Push SSE event for order status update
	if herr := database.HydrateOrderSnapshotForWire(updatedOrder); herr != nil {
		log.Printf("order.updated snapshot hydrate failed: order_id=%d error=%v", updatedOrder.ID, herr)
		updatedOrder.Items = ""
	}
	events.GetHub().PublishJSON(updatedOrder.BusinessID, "order.updated", updatedOrder)
	alertSvc := operational_alerts.NewService(database.GetDB())
	alertActor := operational_alerts.Actor{Name: actor}
	if updatedOrder.Status == database.OrderStatusApproved {
		if err := alertSvc.ResolveAlertForResource(c.Request.Context(), updatedOrder.BusinessID, database.OperationalAlertResourceTypeOrder, updatedOrder.ID, alertActor, string(updatedOrder.Status)); err != nil {
			log.Printf("failed to resolve order operational alert: business_id=%d order_id=%d error=%v", updatedOrder.BusinessID, updatedOrder.ID, err)
		}
		if err := alertSvc.CreateKitchenReadyAlert(c.Request.Context(), *updatedOrder); err != nil {
			log.Printf("failed to create kitchen operational alert: business_id=%d order_id=%d error=%v", updatedOrder.BusinessID, updatedOrder.ID, err)
		}
	}
	if updatedOrder.Status == database.OrderStatusInKitchen ||
		updatedOrder.Status == database.OrderStatusOrderReady ||
		updatedOrder.Status == database.OrderStatusOrderDelivered ||
		updatedOrder.Status == database.OrderStatusOrderCancelled {
		if err := alertSvc.ResolveAlertForResource(c.Request.Context(), updatedOrder.BusinessID, database.OperationalAlertResourceTypeOrder, updatedOrder.ID, alertActor, string(updatedOrder.Status)); err != nil {
			log.Printf("failed to resolve order operational alert: business_id=%d order_id=%d status=%s error=%v", updatedOrder.BusinessID, updatedOrder.ID, updatedOrder.Status, err)
		}
	}
	if updatedOrder.Status == database.OrderStatusApproved {
		enqueueTelegramInventoryLowStockAlerts(updatedOrder.BusinessID)
		enqueueKitchenTicketForApprovedOrder(c.Request.Context(), updatedOrder)
	}

	// Kitchen progress propagates to the delivery leg: order ready means the
	// delivery enters Dispatch's Ready column for driver assignment.
	if req.Status == database.OrderStatusOrderReady && sharedDeliveryService != nil {
		if deliveryForOrder, derr := sharedDeliveryService.DeliveryByOrderID(uint(businessID), uint(orderID)); derr == nil && deliveryForOrder != nil {
			if deliveryForOrder.Status == database.DeliveryStatusPreparing {
				if uerr := sharedDeliveryService.UpdateDeliveryStatus(deliveryForOrder.ID, database.DeliveryStatusReady, nil, actor); uerr != nil {
					log.Printf("order ready: delivery propagation failed (order=%d delivery=%d): %v", orderID, deliveryForOrder.ID, uerr)
				} else {
					// Reload the delivery so the SSE carries the post-mutation state;
					// UpdateDeliveryStatus does not emit SSE internally.
					if fresh, ferr := sharedDeliveryService.DeliveryByOrderID(uint(businessID), uint(orderID)); ferr == nil && fresh != nil {
						events.GetHub().PublishJSON(fresh.BusinessID, "delivery.updated", fresh)
					} else {
						events.GetHub().PublishJSON(deliveryForOrder.BusinessID, "delivery.updated", deliveryForOrder)
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, OrderResponse{Order: *updatedOrder})
}

func enqueueTelegramInventoryLowStockAlerts(businessID uint) {
	// Gating + enqueue now live in one shared helper so the order and
	// manual-adjustment paths can never drift apart (INV-L2).
	if _, err := services.MaybeEnqueueTelegramInventoryLowStockAlerts(businessID, time.Now().UTC()); err != nil {
		log.Printf("Failed to enqueue Telegram inventory low stock notification for business_id=%d: %v", businessID, err)
	}
}
