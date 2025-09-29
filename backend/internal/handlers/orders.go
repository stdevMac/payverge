package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"payverge/internal/database"
)

// CreateOrderRequest represents the request to create a new order
type CreateOrderRequest struct {
	BillID  uint                    `json:"bill_id" binding:"required"`
	Notes   string                  `json:"notes"`
	Items   []CreateOrderItemRequest `json:"items" binding:"required"`
}

// CreateOrderItemRequest represents individual items in the order
type CreateOrderItemRequest struct {
	MenuItemName    string  `json:"menu_item_name" binding:"required"`
	Quantity        int     `json:"quantity" binding:"required"`
	Price           float64 `json:"price" binding:"required"`
	SpecialRequests string  `json:"special_requests"`
}

// UpdateOrderStatusRequest represents the request to update order status
type UpdateOrderStatusRequest struct {
	Status     database.OrderStatus `json:"status" binding:"required"`
	ApprovedBy string               `json:"approved_by"`
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

// generateOrderNumber generates a unique order number for display
func generateOrderNumber(businessID uint) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("O%d-%d", businessID, timestamp%10000)
}

// CreateOrder creates a new order (for guests)
func CreateOrder(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db := database.GetDB()
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection failed"})
		return
	}

	// Verify the bill exists and belongs to this business
	var bill database.Bill
	if err := db.Where("id = ? AND business_id = ?", req.BillID, businessID).First(&bill).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Generate order number
	orderNumber := generateOrderNumber(uint(businessID))

	// Create the order
	order := database.Order{
		BillID:      req.BillID,
		BusinessID:  uint(businessID),
		OrderNumber: orderNumber,
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest", // TODO: Get from context if authenticated
		Notes:       req.Notes,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Convert request items to order items
	var orderItems []database.OrderItem
	for _, itemReq := range req.Items {
		orderItem := database.OrderItem{
			ID:              fmt.Sprintf("%s-%d", itemReq.MenuItemName, time.Now().UnixNano()),
			MenuItemName:    itemReq.MenuItemName,
			Quantity:        itemReq.Quantity,
			Price:           itemReq.Price,
			SpecialRequests: itemReq.SpecialRequests,
			Subtotal:        itemReq.Price * float64(itemReq.Quantity),
		}
		orderItems = append(orderItems, orderItem)
	}

	// Create order with items
	if err := database.CreateOrder(&order, orderItems); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create order"})
		return
	}

	// Load the complete order with relationships
	if err := db.
		Preload("Bill").
		Preload("Business").
		First(&order, order.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load order"})
		return
	}

	c.JSON(http.StatusCreated, OrderResponse{Order: order})
}

// GetOrders retrieves orders for a business
func GetOrders(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Get status filter from query params
	status := c.Query("status")

	orders, err := database.GetOrdersByBusinessID(uint(businessID), status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve orders"})
		return
	}

	c.JSON(http.StatusOK, OrdersResponse{
		Orders: orders,
		Total:  int64(len(orders)),
	})
}

// GetOrder retrieves a specific order
func GetOrder(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	orderIDStr := c.Param("orderId")
	orderID, err := strconv.ParseUint(orderIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	order, items, err := database.GetOrderByID(uint(orderID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	}

	// Verify order belongs to this business
	if order.BusinessID != uint(businessID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	orderIDStr := c.Param("orderId")
	orderID, err := strconv.ParseUint(orderIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid order ID"})
		return
	}

	var req UpdateOrderStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify order exists and belongs to this business
	order, _, err := database.GetOrderByID(uint(orderID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	}

	if order.BusinessID != uint(businessID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		return
	}

	// Update order status
	if err := database.UpdateOrderStatus(uint(orderID), req.Status, req.ApprovedBy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update order status"})
		return
	}

	// Return updated order
	updatedOrder, _, err := database.GetOrderByID(uint(orderID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve updated order"})
		return
	}

	c.JSON(http.StatusOK, OrderResponse{Order: *updatedOrder})
}

// GetOrdersByBillID retrieves orders for a specific bill
func GetOrdersByBillID(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	billIDStr := c.Param("billId")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	// Verify bill belongs to this business
	db := database.GetDB()
	var bill database.Bill
	if err := db.Where("id = ? AND business_id = ?", billID, businessID).First(&bill).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	orders, err := database.GetOrdersByBillID(uint(billID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve orders"})
		return
	}

	c.JSON(http.StatusOK, OrdersResponse{
		Orders: orders,
		Total:  int64(len(orders)),
	})
}
