package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"payverge/internal/database"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CreateKitchenOrderRequest represents the request to create a new kitchen order
type CreateKitchenOrderRequest struct {
	BillID       *uint                        `json:"bill_id"`
	TableID      *uint                        `json:"table_id"`
	CustomerName string                       `json:"customer_name"`
	OrderType    database.KitchenOrderType    `json:"order_type"`
	Priority     database.OrderPriority       `json:"priority"`
	Notes        string                       `json:"notes"`
	Items        []CreateKitchenOrderItemRequest `json:"items"`
}

// CreateKitchenOrderItemRequest represents individual items in the order
type CreateKitchenOrderItemRequest struct {
	MenuItemName    string  `json:"menu_item_name"`
	Quantity        int     `json:"quantity"`
	Price           float64 `json:"price"`
	SpecialRequests string  `json:"special_requests"`
}

// UpdateKitchenOrderStatusRequest represents the request to update order status
type UpdateKitchenOrderStatusRequest struct {
	Status       database.KitchenOrderStatus `json:"status"`
	AssignedTo   string                      `json:"assigned_to"`
	EstimatedTime int                        `json:"estimated_time"`
	Notes        string                      `json:"notes"`
}

// UpdateKitchenOrderItemStatusRequest represents the request to update item status
type UpdateKitchenOrderItemStatusRequest struct {
	Status database.KitchenOrderItemStatus `json:"status"`
}

// KitchenOrderResponse represents the response for kitchen orders
type KitchenOrderResponse struct {
	Order database.KitchenOrder `json:"order"`
}

// KitchenOrdersResponse represents the response for multiple kitchen orders
type KitchenOrdersResponse struct {
	Orders []database.KitchenOrder `json:"orders"`
	Total  int64                   `json:"total"`
}

// generateOrderNumber generates a unique order number for the kitchen
func generateOrderNumber(businessID uint) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("K%d-%d", businessID, timestamp%10000)
}

// CreateKitchenOrder creates a new kitchen order
func CreateKitchenOrder(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	var req CreateKitchenOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate that at least one item is provided
	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one item is required"})
		return
	}

	// Get user address from context (set by auth middleware)
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	db := database.GetDB()

	// Validate business ownership
	var business database.Business
	if err := db.Where("id = ? AND owner_address = ?", businessID, userAddress).First(&business).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: business not found or not owned by user"})
		return
	}

	// Create the kitchen order
	order := database.KitchenOrder{
		BusinessID:   uint(businessID),
		BillID:       req.BillID,
		TableID:      req.TableID,
		OrderNumber:  generateOrderNumber(uint(businessID)),
		CustomerName: req.CustomerName,
		OrderType:    req.OrderType,
		Status:       database.OrderStatusIncoming,
		Priority:     req.Priority,
		Notes:        req.Notes,
		CreatedBy:    userAddress.(string),
	}

	// Start transaction
	tx := db.Begin()
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create kitchen order"})
		return
	}

	// Create order items
	for _, itemReq := range req.Items {
		item := database.KitchenOrderItem{
			KitchenOrderID:  order.ID,
			MenuItemName:    itemReq.MenuItemName,
			Quantity:        itemReq.Quantity,
			Price:           itemReq.Price,
			Status:          database.ItemStatusPending,
			SpecialRequests: itemReq.SpecialRequests,
		}

		if err := tx.Create(&item).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create order items"})
			return
		}
	}

	tx.Commit()

	// Load the complete order with relationships
	if err := db.Preload("Items").Preload("Business").Preload("Table").Preload("Bill").First(&order, order.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load created order"})
		return
	}

	c.JSON(http.StatusCreated, KitchenOrderResponse{Order: order})
}

// GetKitchenOrders retrieves kitchen orders for a business
func GetKitchenOrders(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Get user address from context (set by auth middleware)
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	db := database.GetDB()

	// Validate business ownership
	var business database.Business
	if err := db.Where("id = ? AND owner_address = ?", businessID, userAddress).First(&business).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: business not found or not owned by user"})
		return
	}

	// Parse query parameters
	status := c.Query("status")
	orderType := c.Query("order_type")
	limit := 50 // Default limit
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Build query
	query := db.Where("business_id = ?", businessID)
	
	if status != "" {
		query = query.Where("status = ?", status)
	}
	
	if orderType != "" {
		query = query.Where("order_type = ?", orderType)
	}

	// Get total count
	var total int64
	query.Model(&database.KitchenOrder{}).Count(&total)

	// Get orders with relationships
	var orders []database.KitchenOrder
	if err := query.
		Preload("Items").
		Preload("Business").
		Preload("Table").
		Preload("Bill").
		Order("created_at DESC").
		Limit(limit).
		Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve kitchen orders"})
		return
	}

	c.JSON(http.StatusOK, KitchenOrdersResponse{
		Orders: orders,
		Total:  total,
	})
}

// GetKitchenOrder retrieves a specific kitchen order
func GetKitchenOrder(c *gin.Context) {
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

	// Get user address from context (set by auth middleware)
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	db := database.GetDB()

	// Validate business ownership
	var business database.Business
	if err := db.Where("id = ? AND owner_address = ?", businessID, userAddress).First(&business).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: business not found or not owned by user"})
		return
	}

	var order database.KitchenOrder
	if err := db.
		Preload("Items").
		Preload("Business").
		Preload("Table").
		Preload("Bill").
		Where("id = ? AND business_id = ?", orderID, businessID).
		First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Kitchen order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve kitchen order"})
		return
	}

	c.JSON(http.StatusOK, KitchenOrderResponse{Order: order})
}

// UpdateKitchenOrderStatus updates the status of a kitchen order
func UpdateKitchenOrderStatus(c *gin.Context) {
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

	var req UpdateKitchenOrderStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user address from context (set by auth middleware)
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	db := database.GetDB()

	// Validate business ownership
	var business database.Business
	if err := db.Where("id = ? AND owner_address = ?", businessID, userAddress).First(&business).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: business not found or not owned by user"})
		return
	}

	// Find the order
	var order database.KitchenOrder
	if err := db.Where("id = ? AND business_id = ?", orderID, businessID).First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Kitchen order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find kitchen order"})
		return
	}

	// Update fields
	updates := map[string]interface{}{
		"status": req.Status,
	}

	if req.AssignedTo != "" {
		updates["assigned_to"] = req.AssignedTo
	}

	if req.EstimatedTime > 0 {
		updates["estimated_time"] = req.EstimatedTime
	}

	if req.Notes != "" {
		updates["notes"] = req.Notes
	}

	// Set timestamps based on status
	now := time.Now()
	switch req.Status {
	case database.OrderStatusReady:
		updates["ready_at"] = &now
		// Calculate actual time if order was in progress
		if order.Status == database.OrderStatusInProgress {
			actualTime := int(now.Sub(order.UpdatedAt).Minutes())
			updates["actual_time"] = actualTime
		}
	case database.OrderStatusDelivered:
		updates["delivered_at"] = &now
	}

	// Update the order
	if err := db.Model(&order).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update kitchen order"})
		return
	}

	// Load updated order with relationships
	if err := db.
		Preload("Items").
		Preload("Business").
		Preload("Table").
		Preload("Bill").
		First(&order, orderID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load updated order"})
		return
	}

	c.JSON(http.StatusOK, KitchenOrderResponse{Order: order})
}

// UpdateKitchenOrderItemStatus updates the status of a specific item in a kitchen order
func UpdateKitchenOrderItemStatus(c *gin.Context) {
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

	itemIDStr := c.Param("itemId")
	itemID, err := strconv.ParseUint(itemIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item ID"})
		return
	}

	var req UpdateKitchenOrderItemStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user address from context (set by auth middleware)
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	db := database.GetDB()

	// Validate business ownership
	var business database.Business
	if err := db.Where("id = ? AND owner_address = ?", businessID, userAddress).First(&business).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: business not found or not owned by user"})
		return
	}

	// Verify the item belongs to the order and business
	var item database.KitchenOrderItem
	if err := db.
		Joins("JOIN kitchen_orders ON kitchen_order_items.kitchen_order_id = kitchen_orders.id").
		Where("kitchen_order_items.id = ? AND kitchen_orders.id = ? AND kitchen_orders.business_id = ?", itemID, orderID, businessID).
		First(&item).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Kitchen order item not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find kitchen order item"})
		return
	}

	// Update item status with timestamps
	updates := map[string]interface{}{
		"status": req.Status,
	}

	now := time.Now()
	switch req.Status {
	case database.ItemStatusInProgress:
		updates["started_at"] = &now
	case database.ItemStatusReady:
		updates["ready_at"] = &now
	}

	if err := db.Model(&item).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update kitchen order item"})
		return
	}

	// Check if all items in the order are ready, and if so, update the order status
	var pendingItems int64
	db.Model(&database.KitchenOrderItem{}).
		Where("kitchen_order_id = ? AND status NOT IN ?", orderID, []string{"ready", "cancelled"}).
		Count(&pendingItems)

	if pendingItems == 0 {
		// All items are ready or cancelled, update order status to ready
		db.Model(&database.KitchenOrder{}).
			Where("id = ?", orderID).
			Updates(map[string]interface{}{
				"status":   database.OrderStatusReady,
				"ready_at": &now,
			})
	}

	c.JSON(http.StatusOK, gin.H{"message": "Kitchen order item updated successfully"})
}

// GetKitchenStats retrieves kitchen performance statistics
func GetKitchenStats(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Get user address from context (set by auth middleware)
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	db := database.GetDB()

	// Validate business ownership
	var business database.Business
	if err := db.Where("id = ? AND owner_address = ?", businessID, userAddress).First(&business).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: business not found or not owned by user"})
		return
	}

	// Get date range from query params (default to today)
	dateStr := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
	startDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format. Use YYYY-MM-DD"})
		return
	}
	endDate := startDate.Add(24 * time.Hour)

	// Calculate statistics
	var stats database.KitchenStats
	stats.BusinessID = uint(businessID)
	stats.Date = dateStr

	// Total orders
	var totalCount int64
	db.Model(&database.KitchenOrder{}).
		Where("business_id = ? AND created_at >= ? AND created_at < ?", businessID, startDate, endDate).
		Count(&totalCount)
	stats.TotalOrders = int(totalCount)

	// Completed orders
	var completedCount int64
	db.Model(&database.KitchenOrder{}).
		Where("business_id = ? AND status = ? AND created_at >= ? AND created_at < ?", 
			businessID, database.OrderStatusDelivered, startDate, endDate).
		Count(&completedCount)
	stats.CompletedOrders = int(completedCount)

	// Cancelled orders
	var cancelledCount int64
	db.Model(&database.KitchenOrder{}).
		Where("business_id = ? AND status = ? AND created_at >= ? AND created_at < ?", 
			businessID, database.OrderStatusCancelled, startDate, endDate).
		Count(&cancelledCount)
	stats.CancelledOrders = int(cancelledCount)

	// Average preparation time
	var avgTime sql.NullFloat64
	db.Model(&database.KitchenOrder{}).
		Select("AVG(actual_time)").
		Where("business_id = ? AND actual_time IS NOT NULL AND created_at >= ? AND created_at < ?", 
			businessID, startDate, endDate).
		Scan(&avgTime)
	
	if avgTime.Valid {
		stats.AverageTime = avgTime.Float64
	}

	// Total revenue (sum of all order items)
	var totalRevenue sql.NullFloat64
	db.Table("kitchen_order_items").
		Select("SUM(kitchen_order_items.price * kitchen_order_items.quantity)").
		Joins("JOIN kitchen_orders ON kitchen_order_items.kitchen_order_id = kitchen_orders.id").
		Where("kitchen_orders.business_id = ? AND kitchen_orders.created_at >= ? AND kitchen_orders.created_at < ?", 
			businessID, startDate, endDate).
		Scan(&totalRevenue)
	
	if totalRevenue.Valid {
		stats.TotalRevenue = totalRevenue.Float64
	}

	c.JSON(http.StatusOK, stats)
}
